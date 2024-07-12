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

	"github.com/fluxcd/pkg/runtime/logger"
	helmrelease "helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
)

// Test is an ActionReconciler which attempts to perform a Helm test for
// the latest release of the Request.Object.
//
// The writes to the Helm storage during testing are observed, which causes the
// TestHooks field of the latest Snapshot in the Status.History to be updated
// if it matches the target of the test.
//
// When all test hooks for the release succeed, the object is marked with
// TestSuccess=True and an event is emitted. When one of the test hooks fails,
// Helm stops running the remaining tests, and the object is marked with
// TestSuccess=False and a warning event is emitted. If test failures are not
// ignored, the failure count for the active remediation strategy is
// incremented.
//
// When the Request.Object does not have a latest release, it returns an
// error of type ErrNoLatest. In addition, it returns ErrReleaseMismatch
// if the test ran for a different release target than the latest release.
// Any other returned error indicates the caller should retry as it did not cause
// a change to the Helm storage.
//
// At the end of the reconciliation, the Status.Conditions are summarized and
// propagated to the Ready condition on the Request.Object.
//
// The caller is assumed to have verified the integrity of Request.Object using
// e.g. action.VerifySnapshot before calling Reconcile.
type Test struct {
	configFactory *action.ConfigFactory
	eventRecorder record.EventRecorder
}

// NewTest returns a new Test reconciler configured with the provided values.
func NewTest(cfg *action.ConfigFactory, recorder record.EventRecorder) *Test {
	return &Test{configFactory: cfg, eventRecorder: recorder}
}

func (r *Test) Reconcile(ctx context.Context, req *Request) error {
	var (
		cur = req.Object.Status.History.Latest().DeepCopy()
		cfg = r.configFactory.Build(action.NewDebugLog(ctrl.LoggerFrom(ctx).V(logger.DebugLevel)), observeTest(req.Object))
	)

	defer summarize(req)

	// We only accept test results for the current release.
	if cur == nil {
		return fmt.Errorf("%w: required for test", ErrNoLatest)
	}

	// Run the Helm test action.
	rls, err := action.Test(ctx, cfg, req.Object)

	// The Helm test action does always target the latest release. Before
	// accepting results, we need to confirm this is actually the release we
	// have recorded as latest.
	if rls != nil && !release.ObserveRelease(rls).Targets(cur.Name, cur.Namespace, cur.Version) {
		err = fmt.Errorf("%w: tested release %s/%s.v%d != current release %s/%s.v%d",
			ErrReleaseMismatch, rls.Namespace, rls.Name, rls.Version, cur.Namespace, cur.Name, cur.Version)
	}

	// Something went wrong.
	if err != nil {
		r.failure(req, err)

		// If we failed to observe anything happened at all, we want to retry
		// and return the error to indicate this.
		if !req.Object.Status.History.Latest().HasBeenTested() {
			return err
		}
		return nil
	}

	r.success(req)
	return nil
}

func (r *Test) Name() string {
	return "test"
}

func (r *Test) Type() ReconcilerType {
	return ReconcilerTypeTest
}

const (
	// fmtTestPending is the message format used when awaiting tests to be run.
	fmtTestPending = "Helm release %s with chart %s is awaiting tests"
	// fmtTestFailure is the message format for a test failure.
	fmtTestFailure = "Helm test failed for release %s with chart %s: %s"
	// fmtTestSuccess is the message format for a successful test.
	fmtTestSuccess = "Helm test succeeded for release %s with chart %s: %s"
)

// failure records the failure of a Helm test action in the status of the given
// Request.Object by marking TestSuccess=False and increasing the failure
// counter. In addition, it emits a warning event for the Request.Object.
// The active remediation failure count is only incremented if test failures
// are not ignored.
func (r *Test) failure(req *Request, err error) {
	// Compose failure message.
	cur := req.Object.Status.History.Latest()
	msg := fmt.Sprintf(fmtTestFailure, cur.FullReleaseName(), cur.VersionedChartName(), strings.TrimSpace(err.Error()))

	// Mark test failure on object.
	req.Object.Status.Failures++
	conditions.MarkFalse(req.Object, v2.TestSuccessCondition, v2.TestFailedReason, "%s", msg)

	// Record warning event, this message contains more data than the
	// Condition summary.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(cur.ChartVersion, cur.ConfigDigest, addAppVersion(cur.AppVersion), addOCIDigest(cur.OCIDigest)),
		corev1.EventTypeWarning,
		v2.TestFailedReason,
		msg,
	)

	if req.Object.Status.History.Latest().HasBeenTested() {
		// Count the failure of the test for the active remediation strategy if enabled.
		remediation := req.Object.GetActiveRemediation()
		if remediation != nil && !remediation.MustIgnoreTestFailures(req.Object.GetTest().IgnoreFailures) {
			remediation.IncrementFailureCount(req.Object)
		}
	}
}

// success records the failure of a Helm test action in the status of the given
// Request.Object by marking TestSuccess=True and emitting an event.
func (r *Test) success(req *Request) {
	// Compose success message.
	cur := req.Object.Status.History.Latest()
	var hookMsg = "no test hooks"
	if l := len(cur.GetTestHooks()); l > 0 {
		h := "hook"
		if l > 1 {
			h += "s"
		}
		hookMsg = fmt.Sprintf("%d test %s completed successfully", l, h)
	}
	msg := fmt.Sprintf(fmtTestSuccess, cur.FullReleaseName(), cur.VersionedChartName(), hookMsg)

	// Mark test success on object.
	conditions.MarkTrue(req.Object, v2.TestSuccessCondition, v2.TestSucceededReason, "%s", msg)

	// Record event.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(cur.ChartVersion, cur.ConfigDigest, addAppVersion(cur.AppVersion), addOCIDigest(cur.OCIDigest)),
		corev1.EventTypeNormal,
		v2.TestSucceededReason,
		msg,
	)
}

// observeTest returns a storage.ObserveFunc to track test results of a
// HelmRelease.
// It only accepts test results for the latest release and updates the
// latest snapshot with the observed test results.
func observeTest(obj *v2.HelmRelease) storage.ObserveFunc {
	return func(rls *helmrelease.Release) {
		// Only accept test results for the latest release.
		if !obj.Status.History.Latest().Targets(rls.Name, rls.Namespace, rls.Version) {
			return
		}

		// Update the latest snapshot with the test result.
		latest := obj.Status.History.Latest()
		tested := release.ObservedToSnapshot(releaseToObservation(rls, latest))
		tested.SetTestHooks(release.TestHooksFromRelease(rls))
		obj.Status.History[0] = tested
	}
}
