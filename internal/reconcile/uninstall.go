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
	"errors"
	"fmt"
	"strings"

	helmrelease "helm.sh/helm/v3/pkg/release"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/logger"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
)

// Uninstall is an ActionReconciler which attempts to uninstall a Helm release
// based on the given Request data.
//
// The writes to the Helm storage during the uninstallation are observed, and
// update the Status.Current field.
//
// After a successful uninstall, the object is marked with Released=False and
// an event is emitted. When the uninstallation fails, the object is marked
// with Released=False and a warning event is emitted.
//
// When the Request.Object does not have a Status.Current, it returns an
// error of type ErrNoCurrent. If the uninstallation targeted a different
// release (version) than Status.Current, it returns an error of type
// ErrReleaseMismatch. In addition, it returns ErrNoStorageUpdate if the
// uninstallation completed without updating the Helm storage. In which case
// the resources for the release will be removed from the cluster, but the
// storage object remains in the cluster. Any other returned error indicates
// the caller should retry as it did not cause a change to the Helm storage or
// the cluster resources.
//
// At the end of the reconciliation, the Status.Conditions are summarized and
// propagated to the Ready condition on the Request.Object.
//
// This reconciler is different from UninstallRemediation, in that it makes
// observations to the Released condition type instead of Remediated. Use this
// reconciler to uninstall a release, and UninstallRemediation to remediate a
// release.
//
// The caller is assumed to have verified the integrity of Request.Object using
// e.g. action.VerifySnapshot before calling Reconcile.
type Uninstall struct {
	configFactory *action.ConfigFactory
	eventRecorder record.EventRecorder
}

// NewUninstall returns a new Uninstall reconciler configured with the provided
// values.
func NewUninstall(cfg *action.ConfigFactory, recorder record.EventRecorder) *Uninstall {
	return &Uninstall{configFactory: cfg, eventRecorder: recorder}
}

func (r *Uninstall) Reconcile(ctx context.Context, req *Request) error {
	var (
		cur    = req.Object.GetCurrent().DeepCopy()
		logBuf = action.NewLogBuffer(action.NewDebugLog(ctrl.LoggerFrom(ctx).V(logger.DebugLevel)), 10)
		cfg    = r.configFactory.Build(logBuf.Log, observeUninstall(req.Object))
	)

	defer summarize(req)

	// Require current to run uninstall.
	if cur == nil {
		return fmt.Errorf("%w: required to uninstall", ErrNoCurrent)
	}

	// Run the Helm uninstall action.
	res, err := action.Uninstall(ctx, cfg, req.Object, cur.Name)

	// When the release is not found, something else has already uninstalled
	// the release. As such, we can assume the release is uninstalled while
	// taking note that we did not do it.
	if errors.Is(err, helmdriver.ErrReleaseNotFound) {
		conditions.MarkFalse(req.Object, v2.ReleasedCondition, v2.UninstallSucceededReason,
			"Release %s was not found, assuming it is uninstalled", cur.FullReleaseName())
		return nil
	}

	// The Helm uninstall action does always target the latest release. Before
	// accepting results, we need to confirm this is actually the release we
	// have recorded as Current.
	if res != nil && !release.ObserveRelease(res.Release).Targets(cur.Name, cur.Namespace, cur.Version) {
		err = fmt.Errorf("%w: uninstalled release %s/%s.%d != current release %s",
			ErrReleaseMismatch, res.Release.Namespace, res.Release.Name, res.Release.Version, cur.FullReleaseName())
	}

	// The Helm uninstall action may return without an error while the update
	// to the storage failed. Detect this and return an error.
	if err == nil && cur.Digest == req.Object.GetCurrent().Digest {
		err = fmt.Errorf("uninstall completed with error: %w", ErrNoStorageUpdate)
	}

	// Handle any error.
	if err != nil {
		r.failure(req, logBuf, err)
		if req.Object.GetCurrent().Digest == cur.Digest {
			return err
		}
		return nil
	}

	// Mark success.
	r.success(req)
	return nil
}

func (r *Uninstall) Name() string {
	return "uninstall"
}

func (r *Uninstall) Type() ReconcilerType {
	return ReconcilerTypeRelease
}

const (
	// fmtUninstallFailed is the message format for an uninstall failure.
	fmtUninstallFailure = "Helm uninstall failed for release %s with chart %s: %s"
	// fmtUninstallSuccess is the message format for a successful uninstall.
	fmtUninstallSuccess = "Helm uninstall succeeded for release %s with chart %s"
)

// failure records the failure of a Helm uninstall action in the status of the
// given Request.Object by marking Released=False and emitting a warning
// event.
func (r *Uninstall) failure(req *Request, buffer *action.LogBuffer, err error) {
	// Compose success message.
	cur := req.Object.GetCurrent()
	msg := fmt.Sprintf(fmtUninstallFailure, cur.FullReleaseName(), cur.VersionedChartName(), strings.TrimSpace(err.Error()))

	// Mark remediation failure on object.
	req.Object.Status.Failures++
	conditions.MarkFalse(req.Object, v2.ReleasedCondition, v2.UninstallFailedReason, msg)

	// Record warning event, this message contains more data than the
	// Condition summary.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(cur.ChartVersion, cur.ConfigDigest),
		corev1.EventTypeWarning, v2.UninstallFailedReason,
		eventMessageWithLog(msg, buffer),
	)
}

// success records the success of a Helm uninstall action in the status of
// the given Request.Object by marking Released=False and emitting an
// event.
func (r *Uninstall) success(req *Request) {
	// Compose success message.
	cur := req.Object.GetCurrent()
	msg := fmt.Sprintf(fmtUninstallSuccess, cur.FullReleaseName(), cur.VersionedChartName())

	// Mark remediation success on object.
	conditions.MarkFalse(req.Object, v2.ReleasedCondition, v2.UninstallSucceededReason, msg)

	// Record warning event, this message contains more data than the
	// Condition summary.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(cur.ChartVersion, cur.ConfigDigest),
		corev1.EventTypeNormal,
		v2.UninstallSucceededReason,
		msg,
	)
}

// observeUninstall returns a storage.ObserveFunc that can be used to observe
// and record the result of an uninstall action in the status of the given
// release. It updates the Status.Current field of the release if it equals the
// uninstallation target, and version = Current.Version.
func observeUninstall(obj *v2.HelmRelease) storage.ObserveFunc {
	return func(rls *helmrelease.Release) {
		if cur := obj.GetCurrent(); cur != nil {
			if obs := release.ObserveRelease(rls); obs.Targets(cur.Name, cur.Namespace, cur.Version) {
				obj.Status.History.Current = release.ObservedToSnapshot(obs)
			}
		}
	}
}
