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

	helmrelease "helm.sh/helm/v3/pkg/release"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/runtime/logger"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
)

var (
	// ErrNoRetriesRemain is returned when there are no remaining retry
	// attempts for the provided release config.
	ErrNoRetriesRemain = errors.New("no retries remain")
)

// NextAction determines the action that should be performed for the release
// by verifying the integrity of the Helm storage and further state of the
// release, and comparing the Request.Chart and Request.Values to the latest
// release. It can be called repeatedly to step through the reconciliation
// process until it ends up in a state as desired by the Request.Object,
// or no retries remain.
func NextAction(ctx context.Context, cfg *action.ConfigFactory, recorder record.EventRecorder, req *Request) (ActionReconciler, error) {
	log := ctrl.LoggerFrom(ctx).V(logger.DebugLevel)
	config := cfg.Build(nil)
	cur := req.Object.GetCurrent().DeepCopy()

	// If we do not have a current release, we should either install or upgrade
	// the release depending on the state of the storage.
	if cur == nil {
		ok, err := action.IsInstalled(config, req.Object.GetReleaseName())
		if err != nil {
			return nil, fmt.Errorf("cannot confirm if release is already installed: %w", err)
		}
		if ok {
			return NewUpgrade(cfg, recorder), nil
		}
		return NewInstall(cfg, recorder), nil
	}

	// Verify the current release is still in storage and unmodified.
	rls, err := action.VerifyLastStorageItem(config, cur)
	switch err {
	case nil:
		// Noop
	case action.ErrReleaseNotFound, action.ErrReleaseDisappeared:
		log.Info(err.Error())
		return NewInstall(cfg, recorder), nil
	case action.ErrReleaseNotObserved, action.ErrReleaseDigest:
		log.Info(err.Error())
		return NewUpgrade(cfg, recorder), nil
	default:
		return nil, fmt.Errorf("cannot verify current release in storage: %w", err)
	}

	// If the release is in a pending state, the release process likely failed
	// unexpectedly. Unlock the release and e.g. retry again.
	if rls.Info.Status.IsPending() {
		log.Info("observed release is in stale pending state")
		return NewUnlock(cfg, recorder), nil
	}

	remediation := req.Object.GetActiveRemediation()

	// A release in a failed state is different from any of the other states in
	// that the action also needs to happen a last time when no retries remain.
	if rls.Info.Status == helmrelease.StatusFailed {
		if remediation.GetFailureCount(req.Object) <= 0 {
			// If the chart version and/or values have changed, the failure count(s)
			// are reset. This short circuits any remediation attempt to force an
			// upgrade with the new configuration instead.
			return NewUpgrade(cfg, recorder), nil
		}
		return rollbackOrUninstall(cfg, recorder, req)
	}

	// Short circuit if we are out of retries.
	if remediation.RetriesExhausted(req.Object) {
		return nil, fmt.Errorf("%w: ignoring release in %s state", ErrNoRetriesRemain, rls.Info.Status)
	}

	// Act on the state of the release.
	switch rls.Info.Status {
	case helmrelease.StatusUninstalled, helmrelease.StatusSuperseded:
		return NewInstall(cfg, recorder), nil
	case helmrelease.StatusDeployed:
		// Confirm the current release matches the desired config.
		if err = action.VerifyRelease(rls, cur, req.Chart.Metadata, req.Values); err != nil {
			switch err {
			case action.ErrChartChanged, action.ErrConfigDigest:
				return NewUpgrade(cfg, recorder), nil
			default:
				// Error out on any other error as we cannot determine what
				// the state and should e.g. retry.
				return nil, err
			}
		}

		// For the further determination of test results, we look at the
		// observed state of the object. As tests can be run manually by
		// users running e.g. `helm test`.
		if testSpec := req.Object.GetTest(); testSpec.Enable {
			// Confirm the release has been tested if enabled.
			if !req.Object.GetCurrent().HasBeenTested() {
				return NewTest(cfg, recorder), nil
			}
			// Act on any observed test failure.
			if !remediation.MustIgnoreTestFailures(req.Object.GetTest().IgnoreFailures) &&
				req.Object.GetCurrent().HasTestInPhase(helmrelease.HookPhaseFailed.String()) {
				return rollbackOrUninstall(cfg, recorder, req)
			}
		}
	}

	return nil, nil
}

// rollbackOrUninstall determines if the release should be rolled back or
// uninstalled based on the active remediation strategy. If the release
// must be rolled back, the target revision is verified to be in storage
// before returning the RollbackRemediation. If the verification fails,
// Upgrade is returned as a remediation action to ensure continuity.
func rollbackOrUninstall(cfg *action.ConfigFactory, recorder record.EventRecorder, req *Request) (ActionReconciler, error) {
	remediation := req.Object.GetActiveRemediation()
	if !remediation.RetriesExhausted(req.Object) || remediation.MustRemediateLastFailure() {
		switch remediation.GetStrategy() {
		case v2.RollbackRemediationStrategy:
			// Verify the previous release is still in storage and unmodified
			// before instructing to roll back to it.
			if _, err := action.VerifySnapshot(cfg.Build(nil), req.Object.GetPrevious()); err != nil {
				switch err {
				case action.ErrReleaseNotFound, action.ErrReleaseDisappeared,
					action.ErrReleaseNotObserved, action.ErrReleaseDigest:
					// If the rollback target is not found or is in any other
					// way corrupt, the most correct remediation is to reattempt
					// the upgrade.
					return NewUpgrade(cfg, recorder), nil
				default:
					return nil, err
				}
			}
			return NewRollbackRemediation(cfg, recorder), nil
		case v2.UninstallRemediationStrategy:
			return NewUninstallRemediation(cfg, recorder), nil
		}
	}
	return nil, fmt.Errorf("%w: can not remediate %s state", ErrNoRetriesRemain, req.Object.GetCurrent().Status)
}
