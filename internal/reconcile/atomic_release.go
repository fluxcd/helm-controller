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

	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/logger"
	"github.com/fluxcd/pkg/runtime/patch"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
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

// AtomicRelease is an ActionReconciler which implements an atomic release
// strategy similar to Helm's `--atomic`, but with more advanced state
// determination. It determines the NextAction to take based on the current
// state of the Request.Object and other data, and the state of the Helm
// release.
//
// This process will continue until an action is called multiple times, no
// action remains, or a remediation action is called. In which case the process
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
// Any returned error other than ErrNoRetriesRemain should be retried by the
// caller as soon as possible, preferably with a backoff strategy.
//
// The caller is expected to patch the object one last time with the
// Request.Object result to persist the final observation. As there is an
// expectation they will need to patch the object anyway to e.g. update the
// ObservedGeneration.
//
// For more information on the individual ActionReconcilers, refer to their
// documentation.
type AtomicRelease struct {
	kubeClient    client.Client
	configFactory *action.ConfigFactory
	eventRecorder record.EventRecorder
	strategy      releaseStrategy
}

// NewAtomicRelease returns a new AtomicRelease reconciler configured with the
// provided values.
func NewAtomicRelease(client client.Client, cfg *action.ConfigFactory, recorder record.EventRecorder) *AtomicRelease {
	return &AtomicRelease{
		kubeClient:    client,
		eventRecorder: recorder,
		configFactory: cfg,
		strategy:      &cleanReleaseStrategy{},
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
	log := ctrl.LoggerFrom(ctx).V(logger.DebugLevel)
	patchHelper := patch.NewSerialPatcher(req.Object, r.kubeClient)

	var (
		previous ReconcilerTypeSet
		next     ActionReconciler
		err      error
	)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			// Determine the next action to run based on the current state.
			log.Info("determining next Helm action based on current state")
			if next, err = NextAction(ctx, r.configFactory, r.eventRecorder, req); err != nil {
				log.Error(err, "cannot determine next action")
				if errors.Is(err, ErrNoRetriesRemain) {
					conditions.MarkStalled(req.Object, "NoRemainingRetries", "Attempted %d times but failed", req.Object.GetActiveRemediation().GetRetries())
				} else {
					conditions.MarkFalse(req.Object, meta.ReadyCondition, "ActionPlanError", fmt.Sprintf("Could not determine Helm action: %s", err.Error()))
				}
				return err
			}

			// Nothing to do...
			if next == nil {
				log.Info("release in-sync")

				// If we are in-sync, we are no longer reconciling
				conditions.Delete(req.Object, meta.ReconcilingCondition)

				// Always summarize, this ensures we e.g. restore transient errors written to Ready
				summarize(req)

				return nil
			}

			// If we are not allowed to run the next action, we are done for now...
			if !r.strategy.MustContinue(next.Type(), previous) {
				log.Info("instructed to stop before running %s action reconciler %s", next.Type(), next.Name())
				conditions.Delete(req.Object, meta.ReconcilingCondition)
				return nil
			}

			// Mark the release as reconciling before we attempt to run the action.
			// This to show continuous progress, as Helm actions can be long-running.
			conditions.MarkTrue(req.Object, meta.ReconcilingCondition, "Progressing", "Running '%s' %s action with timeout of %s",
				next.Name(), next.Type(), timeoutForAction(next, req.Object).String())
			// Patch the object to reflect the new condition.
			if err = patchHelper.Patch(ctx, req.Object, patch.WithOwnedConditions{Conditions: OwnedConditions}, patch.WithFieldOwner("helm-controller")); err != nil {
				return err
			}

			// Run the action sub-reconciler.
			if err = next.Reconcile(ctx, req); err != nil {
				if conditions.IsReady(req.Object) {
					conditions.MarkFalse(req.Object, meta.ReadyCondition, "ReconcileError", err.Error())
				}
				return err
			}

			// If we must stop after running the action, we are done for now...
			if r.strategy.MustStop(next.Type(), previous) {
				log.Info("instructed to stop after running %s action reconciler %s", next.Type(), next.Name())
				conditions.Delete(req.Object, meta.ReconcilingCondition)
				return nil
			}

			// Append the type to the set of action types we have performed.
			previous = append(previous, next.Type())

			// Patch the release to reflect progress.
			if err = patchHelper.Patch(ctx, req.Object, patch.WithOwnedConditions{Conditions: OwnedConditions}, patch.WithFieldOwner("helm-controller")); err != nil {
				return err
			}
		}
	}
}

func (r *AtomicRelease) Name() string {
	return "atomic-release"
}

func (r *AtomicRelease) Type() ReconcilerType {
	return ReconcilerTypeRelease
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
