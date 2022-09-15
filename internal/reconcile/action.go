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
	"errors"

	helmrelease "helm.sh/helm/v3/pkg/release"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
)

var (
	// ErrReconcileEnd is returned by NextAction when the reconciliation process
	// has reached an end state.
	ErrReconcileEnd = errors.New("abort reconcile")
)

// NextAction determines the action that should be performed for the release
// by verifying the integrity of the Helm storage and further state of the
// release, and comparing the Request.Chart and Request.Values to the latest
// release. It can be called repeatedly to step through the reconciliation
// process until it ends up in a state as desired by the Request.Object.
func NextAction(factory *action.ConfigFactory, req *Request) (ActionReconciler, error) {
	rls, err := action.VerifyStorage(factory.Build(nil), req.Object)
	if err != nil {
		switch err {
		case action.ErrReleaseNotFound, action.ErrReleaseDisappeared:
			return &Install{configFactory: factory}, nil
		case action.ErrReleaseNotObserved, action.ErrReleaseDigest:
			return &Upgrade{configFactory: factory}, nil
		default:
			return nil, err
		}
	}

	if rls.Info.Status.IsPending() {
		return &Unlock{configFactory: factory}, nil
	}

	remediation := req.Object.GetInstall().GetRemediation()
	if req.Object.Status.Previous != nil {
		remediation = req.Object.GetUpgrade().GetRemediation()
	}

	// TODO(hidde): the logic below lacks some implementation details. E.g.
	//  upgrading a failed release when a newer chart version appears.
	switch rls.Info.Status {
	case helmrelease.StatusFailed:
		return rollbackOrUninstall(factory, req)
	case helmrelease.StatusUninstalled:
		return &Install{configFactory: factory}, nil
	case helmrelease.StatusSuperseded:
		return &Install{configFactory: factory}, nil
	case helmrelease.StatusDeployed:
		if err = action.VerifyRelease(rls, req.Object, req.Chart.Metadata, req.Values); err != nil {
			switch err {
			case action.ErrChartChanged:
				return &Upgrade{configFactory: factory}, nil
			case action.ErrConfigDigest:
				return &Upgrade{configFactory: factory}, nil
			default:
				return nil, err
			}
		}

		if testSpec := req.Object.GetTest(); testSpec.Enable {
			if !release.HasBeenTested(rls) {
				return &Test{configFactory: factory}, nil
			}
			if release.HasFailedTests(rls) {
				if !remediation.MustIgnoreTestFailures(req.Object.GetTest().IgnoreFailures) {
					return rollbackOrUninstall(factory, req)
				}
			}
		}
	}
	return nil, ErrReconcileEnd
}

func rollbackOrUninstall(factory *action.ConfigFactory, req *Request) (ActionReconciler, error) {
	remediation := req.Object.GetInstall().GetRemediation()
	if req.Object.Status.Previous != nil {
		// TODO: determine if previous is still in storage and unmodified
		remediation = req.Object.GetUpgrade().GetRemediation()
	}
	// TODO: remove dependency on counter, as this shouldn't be used to determine
	//  if it's enabled.
	remediation.IncrementFailureCount(req.Object)
	if !remediation.RetriesExhausted(req.Object) || remediation.MustRemediateLastFailure() {
		switch remediation.GetStrategy() {
		case v2.RollbackRemediationStrategy:
			return &Rollback{configFactory: factory}, nil
		case v2.UninstallRemediationStrategy:
			return &Uninstall{configFactory: factory}, nil
		}
	}
	return nil, ErrReconcileEnd
}
