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
	"time"

	"helm.sh/helm/v3/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/logger"
	"github.com/fluxcd/pkg/runtime/patch"
	"github.com/fluxcd/pkg/ssa/jsondiff"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/diff"
	"github.com/fluxcd/helm-controller/internal/digest"
	interrors "github.com/fluxcd/helm-controller/internal/errors"
	"github.com/fluxcd/helm-controller/internal/postrender"
)

// OwnedConditions is a list of Condition types owned by the HelmRelease object.
var OwnedConditions = []string{
	v2.ReleasedCondition,
	v2.RemediatedCondition,
	v2.TestSuccessCondition,
	meta.ReconcilingCondition,
	meta.ReadyCondition,
	meta.StalledCondition,
}

var (
	// ErrExceededMaxRetries is returned when there are no remaining retry
	// attempts for the provided release config.
	ErrExceededMaxRetries = errors.New("exceeded maximum retries")

	// ErrMustRequeue is returned when the caller must requeue the object
	// to continue the reconciliation process.
	ErrMustRequeue = errors.New("must requeue")

	// ErrMissingRollbackTarget is returned when the rollback target is missing.
	ErrMissingRollbackTarget = errors.New("missing target release for rollback")

	// ErrUnknownReleaseStatus is returned when the release status is unknown
	// and cannot be acted upon.
	ErrUnknownReleaseStatus = errors.New("unknown release status")

	// ErrUnknownRemediationStrategy is returned when the remediation strategy
	// is unknown.
	ErrUnknownRemediationStrategy = errors.New("unknown remediation strategy")
)

// AtomicRelease is an ActionReconciler which implements an atomic release
// strategy similar to Helm's `--atomic`, but with more advanced state
// determination. It determines the next action to take based on the current
// state of the Request.Object and other data, and the state of the Helm
// release.
//
// This process will continue until an action is called multiple times, no
// action remains, or a remediation action is called. In which case, the process
// will stop to be resumed at a later time or be checked upon again, by e.g. a
// requeue.
//
// Before running the ActionReconciler for the next action, the object is
// marked with Reconciling=True and the status is patched.
// This condition is removed when the ActionReconciler process is done.
//
// When it determines the object is out of remediation retries, the object
// is marked with Stalled=True.
//
// The status conditions are summarized into a Ready condition when no actions
// to be run remain, to ensure any transient error is cleared.
//
// Any returned error other than ErrExceededMaxRetries should be retried by the
// caller as soon as possible, preferably with a backoff strategy. In case of
// ErrMustRequeue, it is advised to requeue the object outside the interval
// to ensure continued progress.
//
// The caller is expected to patch the object one last time with the
// Request.Object result to persist the final observation. As there is an
// expectation they will need to patch the object anyway to e.g. update the
// ObservedGeneration.
//
// For more information on the individual ActionReconcilers, refer to their
// documentation.
type AtomicRelease struct {
	patchHelper   *patch.SerialPatcher
	configFactory *action.ConfigFactory
	eventRecorder record.EventRecorder
	strategy      releaseStrategy
	fieldManager  string
}

// NewAtomicRelease returns a new AtomicRelease reconciler configured with the
// provided values.
func NewAtomicRelease(patchHelper *patch.SerialPatcher, cfg *action.ConfigFactory, recorder record.EventRecorder, fieldManager string) *AtomicRelease {
	return &AtomicRelease{
		patchHelper:   patchHelper,
		eventRecorder: recorder,
		configFactory: cfg,
		strategy:      &cleanReleaseStrategy{},
		fieldManager:  fieldManager,
	}
}

// releaseStrategy defines the continue-stop behavior of the reconcile loop.
type releaseStrategy interface {
	// MustContinue should be called before running the current action, and
	// returns true if the caller must proceed.
	MustContinue(current ReconcilerType, previous ReconcilerTypeSet) bool
	// MustStop should be called after running the current action, and returns
	// true if the caller must stop.
	MustStop(current ReconcilerType, previous ReconcilerTypeSet) bool
}

// cleanReleaseStrategy is a releaseStrategy which will only execute the
// (remaining) actions for a single release. Effectively, this means it will
// only run any action once during a reconcile attempt, and stops after running
// a remediation action.
type cleanReleaseStrategy ReconcilerTypeSet

// MustContinue returns if previous does not contain current.
func (cleanReleaseStrategy) MustContinue(current ReconcilerType, previous ReconcilerTypeSet) bool {
	return !previous.Contains(current)
}

// MustStop returns true if current equals ReconcilerTypeRemediate.
func (cleanReleaseStrategy) MustStop(current ReconcilerType, _ ReconcilerTypeSet) bool {
	switch current {
	case ReconcilerTypeRemediate:
		return true
	default:
		return false
	}
}

func (r *AtomicRelease) Reconcile(ctx context.Context, req *Request) error {
	log := ctrl.LoggerFrom(ctx).V(logger.InfoLevel)

	var (
		previous ReconcilerTypeSet
		next     ActionReconciler
	)
	for {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				// If the context is canceled, we still need to persist any
				// last observation before returning. If the patch fails, we
				// log the error and return the original context cancellation
				// error.
				if err := r.patchHelper.Patch(ctx, req.Object, patch.WithOwnedConditions{Conditions: OwnedConditions}, patch.WithFieldOwner(r.fieldManager)); err != nil {
					log.Error(err, "failed to patch HelmRelease after context cancellation")
				}
				cancel()
			}
			return fmt.Errorf("atomic release canceled: %w", ctx.Err())
		default:
			// Determine the current state of the Helm release.
			log.V(logger.DebugLevel).Info("determining current state of Helm release")
			state, err := DetermineReleaseState(ctx, r.configFactory, req)
			if err != nil {
				conditions.MarkFalse(req.Object, meta.ReadyCondition, "StateError", "Could not determine release state: %s", err)
				return fmt.Errorf("cannot determine release state: %w", err)
			}

			// Determine the next action to run based on the current state.
			log.V(logger.DebugLevel).Info("determining next Helm action based on current state")
			if next, err = r.actionForState(ctx, req, state); err != nil {
				if errors.Is(err, ErrExceededMaxRetries) {
					conditions.MarkStalled(req.Object, "RetriesExceeded", "Failed to %s after %d attempt(s)",
						req.Object.Status.LastAttemptedReleaseAction, req.Object.GetActiveRemediation().GetFailureCount(req.Object))
					return err
				}
				if errors.Is(err, ErrMissingRollbackTarget) {
					conditions.MarkStalled(req.Object, "MissingRollbackTarget", "Failed to perform remediation: %s", err)
					return err
				}
				return err
			}

			// If there is no next action, we are done.
			if next == nil {
				conditions.Delete(req.Object, meta.ReconcilingCondition)

				// Always summarize; this ensures we restore transient errors
				// written to Ready.
				summarize(req)

				// remove stale post-renderers digest on successful reconciliation.
				if conditions.IsReady(req.Object) {
					req.Object.Status.ObservedPostRenderersDigest = ""
					if req.Object.Spec.PostRenderers != nil {
						// Update the post-renderers digest if the post-renderers exist.
						req.Object.Status.ObservedPostRenderersDigest = postrender.Digest(digest.Canonical, req.Object.Spec.PostRenderers).String()
					}
				}

				return nil
			}

			// If we are not allowed to run the next action, we are done for now...
			if !r.strategy.MustContinue(next.Type(), previous) {
				log.V(logger.DebugLevel).Info(
					fmt.Sprintf("instructed to stop before running %s action reconciler %s", next.Type(), next.Name()),
				)

				if remediation := req.Object.GetActiveRemediation(); remediation == nil || !remediation.RetriesExhausted(req.Object) {
					conditions.MarkReconciling(req.Object, meta.ProgressingWithRetryReason, "%s", conditions.GetMessage(req.Object, meta.ReadyCondition))
					return ErrMustRequeue
				}

				conditions.Delete(req.Object, meta.ReconcilingCondition)
				return nil
			}

			// Mark the release as reconciling before we attempt to run the action.
			// This to show continuous progress, as Helm actions can be long-running.
			reconcilingMsg := fmt.Sprintf("Running '%s' action with timeout of %s",
				next.Name(), timeoutForAction(next, req.Object).String())
			conditions.MarkReconciling(req.Object, meta.ProgressingReason, "%s", reconcilingMsg)

			// If the next action is a release action, we can mark the release
			// as progressing in terms of readiness as well. Doing this for any
			// other action type is not useful, as it would potentially
			// overwrite more important failure state from an earlier action.
			if next.Type() == ReconcilerTypeRelease {
				conditions.MarkUnknown(req.Object, meta.ReadyCondition, meta.ProgressingReason, "%s", reconcilingMsg)
			}

			// Patch the object to reflect the new condition.
			if err = r.patchHelper.Patch(ctx, req.Object, patch.WithOwnedConditions{Conditions: OwnedConditions}, patch.WithFieldOwner(r.fieldManager)); err != nil {
				return err
			}

			// Run the action sub-reconciler.
			log.Info(fmt.Sprintf("running '%s' action with timeout of %s", next.Name(), timeoutForAction(next, req.Object).String()))
			if err = next.Reconcile(ctx, req); err != nil {
				if conditions.IsReady(req.Object) {
					conditions.MarkFalse(req.Object, meta.ReadyCondition, "ReconcileError", "%s", err)
				}
				return err
			}

			// If we must stop after running the action, we are done for now...
			if r.strategy.MustStop(next.Type(), previous) {
				log.V(logger.DebugLevel).Info(fmt.Sprintf(
					"instructed to stop after running %s action reconciler %s", next.Type(), next.Name()),
				)

				remediation := req.Object.GetActiveRemediation()
				if remediation == nil || !remediation.RetriesExhausted(req.Object) {
					conditions.MarkReconciling(req.Object, meta.ProgressingWithRetryReason, "%s", conditions.GetMessage(req.Object, meta.ReadyCondition))
					return ErrMustRequeue
				}
				// Check if retries have exhausted after remediation for early
				// stall condition detection.
				if remediation != nil && remediation.RetriesExhausted(req.Object) {
					conditions.MarkStalled(req.Object, "RetriesExceeded", "Failed to %s after %d attempt(s)",
						req.Object.Status.LastAttemptedReleaseAction, req.Object.GetActiveRemediation().GetFailureCount(req.Object))
					return ErrExceededMaxRetries
				}

				conditions.Delete(req.Object, meta.ReconcilingCondition)
				return nil
			}

			// Append the type to the set of action types we have performed.
			previous = append(previous, next.Type())

			// Patch the release to reflect progress.
			if err = r.patchHelper.Patch(ctx, req.Object, patch.WithOwnedConditions{Conditions: OwnedConditions}, patch.WithFieldOwner(r.fieldManager)); err != nil {
				return err
			}
		}
	}
}

// actionForState determines the next action to run based on the current state.
func (r *AtomicRelease) actionForState(ctx context.Context, req *Request, state ReleaseState) (ActionReconciler, error) {
	log := ctrl.LoggerFrom(ctx)

	// Determine whether we may need to force a release action.
	// We do this before determining the next action to run, as otherwise we may
	// end up running a Helm upgrade (due to e.g. ReleaseStatusUnmanaged) and
	// then forcing an upgrade (due to the release now being in
	// ReleaseStatusInSync with a yet unhandled force request).
	forceRequested := v2.ShouldHandleForceRequest(req.Object)

	switch state.Status {
	case ReleaseStatusInSync:
		log.Info("release in-sync with desired state")

		// Remove all history up to the previous release action.
		// We need to continue to hold on to the previous release result
		// to ensure we can e.g. roll back when tests are enabled without
		// any further changes to the release.
		ignoreFailures := req.Object.GetTest().IgnoreFailures
		if remediation := req.Object.GetActiveRemediation(); remediation != nil {
			ignoreFailures = remediation.MustIgnoreTestFailures(req.Object.GetTest().IgnoreFailures)
		}
		req.Object.Status.History.Truncate(ignoreFailures)

		if forceRequested {
			log.Info(msgWithReason("forcing upgrade for in-sync release", "force requested through annotation"))
			return NewUpgrade(r.configFactory, r.eventRecorder), nil
		}

		// Since the release is in-sync, remove any remediated condition if
		// present and replace it with upgrade succeeded condition.
		// This can happen when the current release, which is the result of a
		// rollback remediation, matches the new desired configuration due to
		// having the same chart version and values. As a result, we are already
		// in-sync without performing a release action.
		if conditions.IsTrue(req.Object, v2.RemediatedCondition) {
			cur := req.Object.Status.History.Latest()
			msg := fmt.Sprintf(fmtUpgradeSuccess, cur.FullReleaseName(), cur.VersionedChartName())
			replaceCondition(req.Object, v2.RemediatedCondition, v2.ReleasedCondition, v2.UpgradeSucceededReason, msg, metav1.ConditionTrue)
		}

		return nil, nil
	case ReleaseStatusLocked:
		log.Info(msgWithReason("release locked", state.Reason))
		return NewUnlock(r.configFactory, r.eventRecorder), nil
	case ReleaseStatusAbsent:
		log.Info(msgWithReason("release not installed", state.Reason))

		if req.Object.GetInstall().GetRemediation().RetriesExhausted(req.Object) {
			if forceRequested {
				log.Info(msgWithReason("forcing install while out of retries", "force requested through annotation"))
				return NewInstall(r.configFactory, r.eventRecorder), nil
			}

			return nil, fmt.Errorf("%w: cannot install release", ErrExceededMaxRetries)
		}

		return NewInstall(r.configFactory, r.eventRecorder), nil
	case ReleaseStatusUnmanaged:
		log.Info(msgWithReason("release not managed by controller", state.Reason))

		// Clear the history as we can no longer rely on it.
		req.Object.Status.ClearHistory()

		return NewUpgrade(r.configFactory, r.eventRecorder), nil
	case ReleaseStatusOutOfSync:
		log.Info(msgWithReason("release out-of-sync with desired state", state.Reason))

		if req.Object.GetUpgrade().GetRemediation().RetriesExhausted(req.Object) {
			if forceRequested {
				log.Info(msgWithReason("forcing upgrade while out of retries", "force requested through annotation"))
				return NewUpgrade(r.configFactory, r.eventRecorder), nil
			}

			return nil, fmt.Errorf("%w: cannot upgrade release", ErrExceededMaxRetries)
		}

		return NewUpgrade(r.configFactory, r.eventRecorder), nil
	case ReleaseStatusDrifted:
		log.Info(msgWithReason("detected changes in cluster state", diff.SummarizeDiffSetBrief(state.Diff)))
		for _, change := range state.Diff {
			switch change.Type {
			case jsondiff.DiffTypeCreate:
				log.V(logger.DebugLevel).Info("resource deleted",
					"resource", diff.ResourceName(change.DesiredObject))
			case jsondiff.DiffTypeUpdate:
				patch := change.Patch
				if change.DesiredObject.GetObjectKind().GroupVersionKind().Kind == "Secret" {
					patch = jsondiff.MaskSecretPatchData(change.Patch)
				}
				log.V(logger.DebugLevel).Info("resource modified",
					"resource", diff.ResourceName(change.DesiredObject),
					"patch", patch)
			}
		}

		r.eventRecorder.Eventf(req.Object, corev1.EventTypeWarning, "DriftDetected",
			"Cluster state of release %s has drifted from the desired state:\n%s",
			req.Object.Status.History.Latest().FullReleaseName(), diff.SummarizeDiffSet(state.Diff),
		)

		if req.Object.GetDriftDetection().GetMode() == v2.DriftDetectionEnabled {
			return NewCorrectClusterDrift(r.configFactory, r.eventRecorder, state.Diff, kube.ManagedFieldsManager), nil
		}

		return nil, nil
	case ReleaseStatusUntested:
		log.Info(msgWithReason("release has not been tested", state.Reason))

		// Since an untested release indicates that the release is already
		// in-sync, remove any remediated condition if present and replace it
		// with upgrade succeeded condition.
		// This can happen when an untested current release, which is the result
		// of a rollback remediation, matches the new desired configuration due
		// to having the same chart version and values, and has test enabled. As
		// a result, we are already in-sync without performing a release action,
		// the existing release needs to undergo testing.
		if conditions.IsTrue(req.Object, v2.RemediatedCondition) {
			cur := req.Object.Status.History.Latest()
			msg := fmt.Sprintf(fmtUpgradeSuccess, cur.FullReleaseName(), cur.VersionedChartName())
			replaceCondition(req.Object, v2.RemediatedCondition, v2.ReleasedCondition, v2.UpgradeSucceededReason, msg, metav1.ConditionTrue)
		}

		return NewTest(r.configFactory, r.eventRecorder), nil
	case ReleaseStatusFailed:
		log.Info(msgWithReason("release is in a failed state", state.Reason))

		remediation := req.Object.GetActiveRemediation()

		// If there is no active remediation strategy, we can only attempt to
		// upgrade the release to see if that fixes the problem.
		if remediation == nil {
			log.V(logger.DebugLevel).Info("no active remediation strategy")
			return NewUpgrade(r.configFactory, r.eventRecorder), nil
		}

		// If there is no failure count, the conditions under which the failure
		// occurred must have changed.
		// Attempt to upgrade the release to see if the problem is resolved.
		// This ensures that after a configuration change, the release is
		// attempted again.
		if remediation.GetFailureCount(req.Object) <= 0 {
			log.Info("release conditions have changed since last failure")
			return NewUpgrade(r.configFactory, r.eventRecorder), nil
		}

		// If the force annotation is set, we can attempt to upgrade the release
		// without any further checks.
		if forceRequested {
			log.Info(msgWithReason("forcing upgrade for failed release", "force requested through annotation"))
			return NewUpgrade(r.configFactory, r.eventRecorder), nil
		}

		// We have exhausted the number of retries for the remediation
		// strategy.
		if remediation.RetriesExhausted(req.Object) && !remediation.MustRemediateLastFailure() {
			return nil, fmt.Errorf("%w: cannot remediate failed release", ErrExceededMaxRetries)
		}

		// Reset the history up to the point where the failure occurred.
		// This ensures we do not accumulate a long history of failures.
		req.Object.Status.History.Truncate(remediation.MustIgnoreTestFailures(req.Object.GetTest().IgnoreFailures))

		switch remediation.GetStrategy() {
		case v2.RollbackRemediationStrategy:
			// Verify the previous release is still in storage and unmodified
			// before instructing to roll back to it.
			prev := req.Object.Status.History.Previous(remediation.MustIgnoreTestFailures(req.Object.GetTest().IgnoreFailures))
			if _, err := action.VerifySnapshot(r.configFactory.Build(nil), prev); err != nil {
				if errors.Is(err, action.ErrReleaseNotFound) {
					// If the rollback target is missing, we cannot roll back
					// to it and must fail.
					return nil, fmt.Errorf("%w: cannot remediate failed release", ErrMissingRollbackTarget)
				}

				if interrors.IsOneOf(err, action.ErrReleaseDisappeared, action.ErrReleaseNotObserved, action.ErrReleaseDigest) {
					// If the rollback target is in any way corrupt,
					// the most correct remediation is to reattempt the upgrade.
					log.Info(msgWithReason("unable to verify previous release in storage to roll back to", err.Error()))
					return NewUpgrade(r.configFactory, r.eventRecorder), nil
				}

				// This may be a temporary error, return it to retry.
				return nil, fmt.Errorf("cannot verify previous release to roll back to: %w", err)
			}
			return NewRollbackRemediation(r.configFactory, r.eventRecorder), nil
		case v2.UninstallRemediationStrategy:
			return NewUninstallRemediation(r.configFactory, r.eventRecorder), nil
		default:
			return nil, fmt.Errorf("%w: %s", ErrUnknownRemediationStrategy, remediation.GetStrategy())
		}
	default:
		return nil, fmt.Errorf("%w: %s", ErrUnknownReleaseStatus, state.Status)
	}
}

func (r *AtomicRelease) Name() string {
	return "atomic-release"
}

func (r *AtomicRelease) Type() ReconcilerType {
	return ReconcilerTypeRelease
}

func msgWithReason(msg, reason string) string {
	if reason != "" {
		return fmt.Sprintf("%s: %s", msg, reason)
	}
	return msg
}

func inStringSlice(ss []string, str string) (pos int, ok bool) {
	for k, s := range ss {
		if strings.EqualFold(s, str) {
			return k, true
		}
	}
	return -1, false
}

func timeoutForAction(action ActionReconciler, obj *v2.HelmRelease) time.Duration {
	switch action.(type) {
	case *Install:
		return obj.GetInstall().GetTimeout(obj.GetTimeout()).Duration
	case *Upgrade:
		return obj.GetUpgrade().GetTimeout(obj.GetTimeout()).Duration
	case *Test:
		return obj.GetTest().GetTimeout(obj.GetTimeout()).Duration
	case *RollbackRemediation:
		return obj.GetRollback().GetTimeout(obj.GetTimeout()).Duration
	case *UninstallRemediation:
		return obj.GetUninstall().GetTimeout(obj.GetTimeout()).Duration
	default:
		return obj.GetTimeout().Duration
	}
}

// replaceCondition replaces existing target condition with replacement
// condition, if present, for the given values, retaining the
// LastTransitionTime.
func replaceCondition(obj *v2.HelmRelease, target string, replacement string, reason string, msg string, status metav1.ConditionStatus) {
	c := conditions.Get(obj, target)
	if c != nil {
		// Remove any existing replacement condition to retain the
		// LastTransitionTime set here. If the state of the new condition
		// changes an existing condition, the LastTransitionTime is updated to
		// the current time.
		// Refer https://github.com/fluxcd/pkg/blob/runtime/v0.43.0/runtime/conditions/setter.go#L54-L55.
		conditions.Delete(obj, replacement)
		c.Status = status
		c.Type = replacement
		c.Reason = reason
		c.Message = msg
		conditions.Set(obj, c)
		conditions.Delete(obj, target)
	}
}
