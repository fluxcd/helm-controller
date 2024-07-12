/*
Copyright 2022 The Flux authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package reconcile

import (
	"context"
	"fmt"
	"strings"

	helmrelease "helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/logger"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/chartutil"
	"github.com/fluxcd/helm-controller/internal/digest"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
)

// RollbackRemediation is an ActionReconciler which attempts to roll back
// a Request.Object to a previous successful deployed release in the
// Status.History.
//
// The writes to the Helm storage during the rollback are observed, and update
// the Status.History field.
//
// After a successful rollback, the object is marked with Remediated=True and
// an event is emitted. When the rollback fails, the object is marked with
// Remediated=False and a warning event is emitted.
//
// When the Request.Object does not have a (successful) previous deployed
// release, it returns an error of type ErrMissingRollbackTarget. In addition,
// it returns ErrReleaseMismatch if the name and/or namespace of the latest and
// previous release do not match. Any other returned error indicates the caller
// should retry as it did not cause a change to the Helm storage.
//
// At the end of the reconciliation, the Status.Conditions are summarized and
// propagated to the Ready condition on the Request.Object.
//
// The caller is assumed to have verified the integrity of Request.Object using
// e.g. action.VerifySnapshot before calling Reconcile.
type RollbackRemediation struct {
	configFactory *action.ConfigFactory
	eventRecorder record.EventRecorder
}

// NewRollbackRemediation returns a new RollbackRemediation reconciler
// configured with the provided values.
func NewRollbackRemediation(configFactory *action.ConfigFactory, eventRecorder record.EventRecorder) *RollbackRemediation {
	return &RollbackRemediation{
		configFactory: configFactory,
		eventRecorder: eventRecorder,
	}
}

func (r *RollbackRemediation) Reconcile(ctx context.Context, req *Request) error {
	var (
		cur    = req.Object.Status.History.Latest().DeepCopy()
		logBuf = action.NewLogBuffer(action.NewDebugLog(ctrl.LoggerFrom(ctx).V(logger.DebugLevel)), 10)
		cfg    = r.configFactory.Build(logBuf.Log, observeRollback(req.Object))
	)

	defer summarize(req)

	// Previous is required to determine what version to roll back to.
	prev := req.Object.Status.History.Previous(req.Object.GetUpgrade().GetRemediation().MustIgnoreTestFailures(req.Object.GetTest().IgnoreFailures))
	if prev == nil {
		return fmt.Errorf("%w: required to rollback", ErrMissingRollbackTarget)
	}

	// Confirm previous and current point to the same release.
	if prev.Name != cur.Name || prev.Namespace != cur.Namespace {
		return fmt.Errorf("%w: previous release name or namespace %s does not match current %s",
			ErrReleaseMismatch, prev.FullReleaseName(), cur.FullReleaseName())
	}

	// Run the Helm rollback action.
	if err := action.Rollback(cfg, req.Object, prev.Name, action.RollbackToVersion(prev.Version)); err != nil {
		r.failure(req, prev, logBuf, err)

		// Return error if we did not store a release, as this does not
		// affect state and the caller should e.g. retry.
		if newCur := req.Object.Status.History.Latest(); newCur == nil || (cur != nil && newCur.Digest == cur.Digest) {
			return err
		}

		return nil
	}

	r.success(req, prev)
	return nil
}

func (r *RollbackRemediation) Name() string {
	return "rollback"
}

func (r *RollbackRemediation) Type() ReconcilerType {
	return ReconcilerTypeRemediate
}

const (
	// fmtRollbackRemediationFailure is the message format for a rollback
	// remediation failure.
	fmtRollbackRemediationFailure = "Helm rollback to previous release %s with chart %s failed: %s"
	// fmtRollbackRemediationSuccess is the message format for a successful
	// rollback remediation.
	fmtRollbackRemediationSuccess = "Helm rollback to previous release %s with chart %s succeeded"
)

// failure records the failure of a Helm rollback action in the status of the
// given Request.Object by marking Remediated=False and emitting a warning
// event.
func (r *RollbackRemediation) failure(req *Request, prev *v2.Snapshot, buffer *action.LogBuffer, err error) {
	// Compose failure message.
	msg := fmt.Sprintf(fmtRollbackRemediationFailure, prev.FullReleaseName(), prev.VersionedChartName(), strings.TrimSpace(err.Error()))

	// Mark remediation failure on object.
	req.Object.Status.Failures++
	conditions.MarkFalse(req.Object, v2.RemediatedCondition, v2.RollbackFailedReason, "%s", msg)

	// Record warning event, this message contains more data than the
	// Condition summary.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(prev.ChartVersion, chartutil.DigestValues(digest.Canonical, req.Values).String(),
			addAppVersion(prev.AppVersion), addOCIDigest(prev.OCIDigest)),
		corev1.EventTypeWarning,
		v2.RollbackFailedReason,
		eventMessageWithLog(msg, buffer),
	)
}

// success records the success of a Helm rollback action in the status of the
// given Request.Object by marking Remediated=True and emitting an event.
func (r *RollbackRemediation) success(req *Request, prev *v2.Snapshot) {
	// Compose success message.
	msg := fmt.Sprintf(fmtRollbackRemediationSuccess, prev.FullReleaseName(), prev.VersionedChartName())

	// Mark remediation success on object.
	conditions.MarkTrue(req.Object, v2.RemediatedCondition, v2.RollbackSucceededReason, "%s", msg)

	// Record event.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(prev.ChartVersion, chartutil.DigestValues(digest.Canonical, req.Values).String(),
			addAppVersion(prev.AppVersion), addOCIDigest(prev.OCIDigest)),
		corev1.EventTypeNormal,
		v2.RollbackSucceededReason,
		msg,
	)
}

// observeRollback returns a storage.ObserveFunc to track the rollback history
// of a HelmRelease.
// It observes the rollback action of a Helm release by comparing the release
// history with the recorded snapshots.
// If the rolled-back release matches a snapshot, it updates the snapshot with
// the observed release data.
// If no matching snapshot is found, it creates a new snapshot and prepends it
// to the release history.
func observeRollback(obj *v2.HelmRelease) storage.ObserveFunc {
	return func(rls *helmrelease.Release) {
		for i := range obj.Status.History {
			snap := obj.Status.History[i]
			if snap.Targets(rls.Name, rls.Namespace, rls.Version) {
				newSnap := release.ObservedToSnapshot(releaseToObservation(rls, snap))
				newSnap.SetTestHooks(snap.GetTestHooks())
				obj.Status.History[i] = newSnap
				return
			}
		}

		obs := release.ObserveRelease(rls)
		obj.Status.History = append(v2.Snapshots{release.ObservedToSnapshot(obs)}, obj.Status.History...)
	}
}
