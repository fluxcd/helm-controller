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

	"github.com/fluxcd/pkg/runtime/logger"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
)

type Install struct {
	configFactory *action.ConfigFactory
}

func (r *Install) Reconcile(ctx context.Context, req *Request) error {
	var (
		cur    = req.Object.Status.Current.DeepCopy()
		logBuf = action.NewLogBuffer(action.NewDebugLog(ctrl.LoggerFrom(ctx).V(logger.InfoLevel)), 10)
		cfg    = r.configFactory.Build(logBuf.Log, observeRelease(req.Object))
	)

	// Run install action.
	rls, err := action.Install(ctx, cfg, req.Object, req.Chart, req.Values)
	if err != nil {
		// Mark failure on object.
		req.Object.Status.Failures++
		conditions.MarkFalse(req.Object, v2.ReleasedCondition, v2.InstallFailedReason, err.Error())

		// Return error if we did not store a release, as this does not
		// require remediation and the caller should e.g. retry.
		if newCur := req.Object.Status.Current; newCur == nil || cur == newCur {
			return err
		}

		// Count install failure on object, this is used to determine if
		// we should retry the install and/or remediation. We only count
		// attempts which did cause a modification to the storage, as
		// without a new release in storage there is nothing to remediate,
		// and the action can be retried immediately without causing
		// storage drift.
		req.Object.Status.InstallFailures++
		return nil
	}

	// Mark release success and delete any test success, as the current release
	// isn't tested (yet).
	conditions.MarkTrue(req.Object, v2.ReleasedCondition, v2.InstallSucceededReason, rls.Info.Description)
	conditions.Delete(req.Object, v2.TestSuccessCondition)
	return nil
}

func (r *Install) Name() string {
	return "install"
}

func (r *Install) Type() ReconcilerType {
	return ReconcilerTypeRelease
}
