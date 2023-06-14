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

	"github.com/fluxcd/pkg/runtime/logger"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/chartutil"
	"github.com/fluxcd/helm-controller/internal/digest"
)

// Install is an ActionReconciler which attempts to install a Helm release
// based on the given Request data.
//
// The writes to the Helm storage during the installation process are
// observed, and updates the Status.Current (and possibly Status.Previous)
// field(s).
//
// On installation success, the object is marked with Released=True and emits
// an event. In addition, the object is marked with TestSuccess=False if tests
// are enabled to indicate we are awaiting the results.
// On failure, the object is marked with Released=False and emits a warning
// event. Only an error which resulted in a modification to the Helm storage
// counts towards a failure for the active remediation strategy.
//
// At the end of the reconciliation, the Status.Conditions are summarized and
// propagated to the Ready condition on the Request.Object.
//
// The caller is assumed to have verified the integrity of Request.Object using
// e.g. action.VerifyReleaseInfo before calling Reconcile.
type Install struct {
	configFactory *action.ConfigFactory
	eventRecorder record.EventRecorder
}

// NewInstall returns a new Install reconciler configured with the provided
// values.
func NewInstall(cfg *action.ConfigFactory, recorder record.EventRecorder) *Install {
	return &Install{configFactory: cfg, eventRecorder: recorder}
}

func (r *Install) Reconcile(ctx context.Context, req *Request) error {
	var (
		cur    = req.Object.GetCurrent().DeepCopy()
		logBuf = action.NewLogBuffer(action.NewDebugLog(ctrl.LoggerFrom(ctx).V(logger.InfoLevel)), 10)
		cfg    = r.configFactory.Build(logBuf.Log, observeRelease(req.Object))
	)

	defer summarize(req)

	// Run the Helm install action.
	_, err := action.Install(ctx, cfg, req.Object, req.Chart, req.Values)
	if err != nil {
		r.failure(req, logBuf, err)

		// Return error if we did not store a release, as this does not
		// require remediation and the caller should e.g. retry.
		if newCur := req.Object.GetCurrent(); newCur == nil || (cur != nil && cur.Digest == newCur.Digest) {
			return err
		}

		// Count install failure on object, this is used to determine if
		// we should retry the install and/or remediation. We only count
		// attempts which did cause a modification to the storage, as
		// without a new release in storage there is nothing to remediate,
		// and the action can be retried immediately without causing
		// storage drift.
		req.Object.GetActiveRemediation().IncrementFailureCount(req.Object)
		return nil
	}

	r.success(req)
	return nil
}

func (r *Install) Name() string {
	return "install"
}

func (r *Install) Type() ReconcilerType {
	return ReconcilerTypeRelease
}

const (
	// fmtInstallFailure is the message format for an installation failure.
	fmtInstallFailure = "Install of release %s/%s with chart %s@%s failed: %s"
	// fmtInstallSuccess is the message format for a successful installation.
	fmtInstallSuccess = "Installed release %s with chart %s"
)

// failure records the failure of a Helm installation action in the status of
// the given Request.Object by marking ReleasedCondition=False and increasing
// the failure counter. In addition, it emits a warning event for the
// Request.Object.
//
// Increase of the failure counter for the active remediation strategy should
// be done conditionally by the caller after verifying the failed action has
// modified the Helm storage. This to avoid counting failures which do not
// result in Helm storage drift.
func (r *Install) failure(req *Request, buffer *action.LogBuffer, err error) {
	// Compose failure message.
	msg := fmt.Sprintf(fmtInstallFailure, req.Object.GetReleaseNamespace(), req.Object.GetReleaseName(), req.Chart.Name(),
		req.Chart.Metadata.Version, err.Error())

	// Mark install failure on object.
	req.Object.Status.Failures++
	conditions.MarkFalse(req.Object, v2.ReleasedCondition, v2.InstallFailedReason, msg)

	// Record warning event, this message contains more data than the
	// Condition summary.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(req.Chart.Metadata.Version, chartutil.DigestValues(digest.Canonical, req.Values).String()),
		corev1.EventTypeWarning,
		v2.InstallFailedReason,
		eventMessageWithLog(msg, buffer),
	)
}

// success records the success of a Helm installation action in the status of
// the given Request.Object by marking ReleasedCondition=True and emitting an
// event. In addition, it marks TestSuccessCondition=False when tests are
// enabled to indicate we are awaiting test results after having made the
// release.
func (r *Install) success(req *Request) {
	// Compose success message.
	cur := req.Object.GetCurrent()
	msg := fmt.Sprintf(fmtInstallSuccess, cur.FullReleaseName(), cur.VersionedChartName())

	// Mark install success on object.
	conditions.MarkTrue(req.Object, v2.ReleasedCondition, v2.InstallSucceededReason, msg)
	if req.Object.GetTest().Enable && !cur.HasBeenTested() {
		conditions.MarkFalse(req.Object, v2.TestSuccessCondition, "Pending", fmtTestPending, cur.FullReleaseName(), cur.VersionedChartName())
	}

	// Record event.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(cur.ChartVersion, cur.ConfigDigest),
		corev1.EventTypeNormal,
		v2.InstallSucceededReason,
		msg,
	)
}
