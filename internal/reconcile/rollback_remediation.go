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

	helmrelease "helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/logger"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/chartutil"
	"github.com/fluxcd/helm-controller/internal/digest"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
)

// RollbackRemediation is an ActionReconciler which attempts to roll back
// a Request.Object to the version specified in Status.Previous.
//
// The writes to the Helm storage during the rollback are observed, and update
// the Status.Current field. On an upgrade reattempt, the UpgradeReconciler
// will move the Status.Current field to Status.Previous, effectively updating
// the previous version to the version which was rolled back to. Ensuring
// repetitive failures continue to be able to roll back to a version existing
// in the storage when the history is pruned.
//
// After a successful rollback, the object is marked with Remediated=True and
// an event is emitted. When the rollback fails, the object is marked with
// Remediated=False and a warning event is emitted.
//
// When the Request.Object does not have a Status.Previous, it returns an
// error of type ErrNoPrevious. In addition, it returns ErrReleaseMismatch
// if the name and/or namespace of Status.Current and Status.Previous point
// towards a different release. Any other returned error indicates the caller
// should retry as it did not cause a change to the Helm storage.
//
// At the end of the reconciliation, the Status.Conditions are summarized and
// propagated to the Ready condition on the Request.Object.
//
// The caller is assumed to have verified the integrity of Request.Object using
// e.g. action.VerifyReleaseInfo before calling Reconcile.
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
		cur    = req.Object.GetCurrent().DeepCopy()
		logBuf = action.NewLogBuffer(action.NewDebugLog(ctrl.LoggerFrom(ctx).V(logger.InfoLevel)), 10)
		cfg    = r.configFactory.Build(logBuf.Log, observeRollback(req.Object))
	)

	defer summarize(req)

	// Previous is required to determine what version to roll back to.
	if !req.Object.HasPrevious() {
		return fmt.Errorf("%w: required to rollback", ErrNoPrevious)
	}

	// Confirm previous and current point to the same release.
	prev := req.Object.GetPrevious()
	if prev.Name != cur.Name || prev.Namespace != cur.Namespace {
		return fmt.Errorf("%w: previous release name or namespace %s does not match current %s",
			ErrReleaseMismatch, prev.FullReleaseName(), cur.FullReleaseName())
	}

	// Run the Helm rollback action.
	if err := action.Rollback(cfg, req.Object, prev.Name, action.RollbackToVersion(prev.Version)); err != nil {
		r.failure(req, logBuf, err)

		// Return error if we did not store a release, as this does not
		// affect state and the caller should e.g. retry.
		if newCur := req.Object.GetCurrent(); newCur == nil || (cur != nil && newCur.Digest == cur.Digest) {
			return err
		}

		return nil
	}

	r.success(req)
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
	fmtRollbackRemediationFailure = "Rollback to previous release %s with chart %s failed: %s"
	// fmtRollbackRemediationSuccess is the message format for a successful
	// rollback remediation.
	fmtRollbackRemediationSuccess = "Rolled back to previous release %s with chart %s"
)

// failure records the failure of a Helm rollback action in the status of the
// given Request.Object by marking Remediated=False and emitting a warning
// event.
func (r *RollbackRemediation) failure(req *Request, buffer *action.LogBuffer, err error) {
	// Compose failure message.
	prev := req.Object.GetPrevious()
	msg := fmt.Sprintf(fmtRollbackRemediationFailure, prev.FullReleaseName(), prev.VersionedChartName(), err.Error())

	// Mark remediation failure on object.
	req.Object.Status.Failures++
	conditions.MarkFalse(req.Object, v2.RemediatedCondition, v2.RollbackFailedReason, msg)

	// Record warning event, this message contains more data than the
	// Condition summary.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(prev.ChartVersion, chartutil.DigestValues(digest.Canonical, req.Values).String()),
		corev1.EventTypeWarning,
		v2.RollbackFailedReason,
		eventMessageWithLog(msg, buffer),
	)
}

// success records the success of a Helm rollback action in the status of the
// given Request.Object by marking Remediated=True and emitting an event.
func (r *RollbackRemediation) success(req *Request) {
	// Compose success message.
	prev := req.Object.GetPrevious()
	msg := fmt.Sprintf(fmtRollbackRemediationSuccess, prev.FullReleaseName(), prev.VersionedChartName())

	// Mark remediation success on object.
	conditions.MarkTrue(req.Object, v2.RemediatedCondition, v2.RollbackSucceededReason, msg)

	// Record event.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(prev.ChartVersion, chartutil.DigestValues(digest.Canonical, req.Values).String()),
		corev1.EventTypeNormal,
		v2.RollbackSucceededReason,
		msg,
	)
}

// observeRollback returns a storage.ObserveFunc that can be used to observe
// and record the result of a rollback action in the status of the given release.
// It updates the Status.Current field of the release if it equals the target
// of the rollback action, and version >= Current.Version.
func observeRollback(obj *v2.HelmRelease) storage.ObserveFunc {
	return func(rls *helmrelease.Release) {
		cur := obj.GetCurrent().DeepCopy()
		obs := release.ObserveRelease(rls)
		if cur == nil || !obs.Targets(cur.Name, cur.Namespace, 0) || obs.Version >= cur.Version {
			// Overwrite current with newer release, or update it.
			obj.Status.Current = release.ObservedToInfo(obs)
		}
	}
}
