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

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/logger"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
)

var (
	ErrNoStorageUpdate = errors.New("release not updated in Helm storage")
)

// UninstallRemediation is an ActionReconciler which attempts to remediate a
// failed Helm release for the given Request data by uninstalling it.
//
// The writes to the Helm storage during the uninstallation are observed, and
// update the Status.Current field.
//
// After a successful uninstall, the object is marked with Remediated=True and
// an event is emitted. When the uninstallation fails, the object is marked
// with Remediated=False and a warning event is emitted.
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
// This reconciler is different from Uninstall, in that it makes observations
// to the Remediated condition type instead of Released. Use this reconciler
// to remediate a failed release, and Uninstall to uninstall a release.
//
// The caller is assumed to have verified the integrity of Request.Object using
// e.g. action.VerifySnapshot before calling Reconcile.
type UninstallRemediation struct {
	configFactory *action.ConfigFactory
	eventRecorder record.EventRecorder
}

// NewUninstallRemediation returns a new UninstallRemediation reconciler
// configured with the provided values.
func NewUninstallRemediation(cfg *action.ConfigFactory, recorder record.EventRecorder) *UninstallRemediation {
	return &UninstallRemediation{configFactory: cfg, eventRecorder: recorder}
}

func (r *UninstallRemediation) Reconcile(ctx context.Context, req *Request) error {
	var (
		cur    = req.Object.GetCurrent().DeepCopy()
		logBuf = action.NewLogBuffer(action.NewDebugLog(ctrl.LoggerFrom(ctx).V(logger.InfoLevel)), 10)
		cfg    = r.configFactory.Build(logBuf.Log, observeUninstall(req.Object))
	)

	// Require current to run uninstall.
	if cur == nil {
		return fmt.Errorf("%w: required to uninstall", ErrNoCurrent)
	}

	// Run the Helm uninstall action.
	res, err := action.Uninstall(ctx, cfg, req.Object, cur.Name)

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
		if cur.Digest == req.Object.GetCurrent().Digest {
			return err
		}
		return nil
	}

	// Mark success.
	r.success(req)
	return nil
}

func (r *UninstallRemediation) Name() string {
	return "uninstall"
}

func (r *UninstallRemediation) Type() ReconcilerType {
	return ReconcilerTypeRemediate
}

const (
	// fmtUninstallRemediationFailure is the message format for an uninstall
	// remediation failure.
	fmtUninstallRemediationFailure = "Helm uninstall remediation for release %s with chart %s failed: %s"
	// fmtUninstallRemediationSuccess is the message format for a successful
	// uninstall remediation.
	fmtUninstallRemediationSuccess = "Helm uninstall remediation for release %s with chart %s succeeded"
)

// success records the success of a Helm uninstall remediation action in the
// status of the given Request.Object by marking Remediated=False and emitting
// a warning event.
func (r *UninstallRemediation) failure(req *Request, buffer *action.LogBuffer, err error) {
	// Compose success message.
	cur := req.Object.GetCurrent()
	msg := fmt.Sprintf(fmtUninstallRemediationFailure, cur.FullReleaseName(), cur.VersionedChartName(), strings.TrimSpace(err.Error()))

	// Mark uninstall failure on object.
	req.Object.Status.Failures++
	conditions.MarkFalse(req.Object, v2.RemediatedCondition, v2.UninstallFailedReason, msg)

	// Record warning event, this message contains more data than the
	// Condition summary.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(cur.ChartVersion, cur.ConfigDigest),
		corev1.EventTypeWarning,
		v2.UninstallFailedReason,
		eventMessageWithLog(msg, buffer),
	)
}

// success records the success of a Helm uninstall remediation action in the
// status of the given Request.Object by marking Remediated=True and emitting
// an event.
func (r *UninstallRemediation) success(req *Request) {
	// Compose success message.
	cur := req.Object.GetCurrent()
	msg := fmt.Sprintf(fmtUninstallRemediationSuccess, cur.FullReleaseName(), cur.VersionedChartName())

	// Mark remediation success on object.
	conditions.MarkTrue(req.Object, v2.RemediatedCondition, v2.UninstallSucceededReason, msg)

	// Record event.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(cur.ChartVersion, cur.ConfigDigest),
		corev1.EventTypeNormal,
		v2.UninstallSucceededReason,
		msg,
	)
}
