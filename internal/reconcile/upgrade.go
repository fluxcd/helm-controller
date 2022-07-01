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

	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/logger"

	helmv2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
)

type Upgrade struct {
	configFactory *action.ConfigFactory
}

func (r *Upgrade) Reconcile(ctx context.Context, req *Request) error {
	var (
		cur    = req.Object.Status.Current.DeepCopy()
		logBuf = action.NewLogBuffer(action.NewDebugLog(ctrl.LoggerFrom(ctx).V(logger.InfoLevel)), 10)
		cfg    = r.configFactory.Build(logBuf.Log, observeRelease(req.Object))
	)

	// Run upgrade action.
	rls, err := action.Upgrade(ctx, cfg, req.Object, req.Chart, req.Values)
	if err != nil {
		// Mark failure on object.
		conditions.MarkFalse(req.Object, helmv2.ReleasedCondition, helmv2.UpgradeFailedReason, err.Error())
		req.Object.Status.Failures++

		// Return error if we did not store a release, as this does not
		// affect state and the caller should e.g. retry.
		if newCur := req.Object.Status.Current; newCur == nil || newCur == cur {
			return err
		}

		// Count upgrade failure on object, this is used to determine if
		// we should retry the upgrade and/or remediation. We only count
		// attempts which did cause a modification to the storage, as
		// without a new release in storage there is nothing to remediate,
		// and the action can be retried immediately without causing
		// storage drift.
		req.Object.Status.UpgradeFailures++
		return nil
	}

	// Mark success on object.
	conditions.MarkTrue(req.Object, helmv2.ReleasedCondition, helmv2.UpgradeSucceededReason, rls.Info.Description)
	return nil
}

func (r *Upgrade) Name() string {
	return "upgrade"
}

func (r *Upgrade) Type() ReconcilerType {
	return ReconcilerTypeRelease
}
