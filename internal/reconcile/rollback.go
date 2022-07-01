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

	helmrelease "helm.sh/helm/v3/pkg/release"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/logger"

	helmv2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
)

type Rollback struct {
	configFactory *action.ConfigFactory
}

func (r *Rollback) Name() string {
	return "rollback"
}

func (r *Rollback) Type() ReconcilerType {
	return ReconcilerTypeRemediate
}

func (r *Rollback) Reconcile(ctx context.Context, req *Request) error {
	var (
		cur    = req.Object.Status.Current.DeepCopy()
		logBuf = action.NewLogBuffer(action.NewDebugLog(ctrl.LoggerFrom(ctx).V(logger.InfoLevel)), 10)
	)

	// Previous is required to determine what version to roll back to.
	if req.Object.Status.Previous == nil {
		return fmt.Errorf("%w: required to rollback", ErrNoPrevious)
	}

	// Run rollback action.
	if err := action.Rollback(r.configFactory.Build(logBuf.Log, observeRollback(req.Object)), req.Object); err != nil {
		// Mark failure on object.
		req.Object.Status.Failures++
		conditions.MarkFalse(req.Object, helmv2.RemediatedCondition, helmv2.RollbackFailedReason, err.Error())

		// Return error if we did not store a release, as this does not
		// affect state and the caller should e.g. retry.
		if newCur := req.Object.Status.Current; newCur == nil || newCur == cur {
			return err
		}
		return nil
	}

	// Mark remediation success.
	condMsg := "Rolled back to previous version"
	if prev := req.Object.Status.Previous; prev != nil {
		condMsg = fmt.Sprintf("Rolled back to version %d", prev.Version)
	}
	conditions.MarkTrue(req.Object, helmv2.RemediatedCondition, helmv2.RollbackSucceededReason, condMsg)
	return nil
}

// observeRollback returns a storage.ObserveFunc that can be used to observe
// and record the result of a rollback action in the status of the given release.
// It updates the Status.Current field of the release if it equals the target
// of the rollback action, and version >= Current.Version.
func observeRollback(obj *helmv2.HelmRelease) storage.ObserveFunc {
	return func(rls *helmrelease.Release) {
		cur := obj.Status.Current.DeepCopy()
		obs := release.ObserveRelease(rls)
		if cur == nil || !obs.Targets(cur.Name, cur.Namespace, 0) || obs.Version >= cur.Version {
			// Overwrite current with newer release, or update it.
			obj.Status.Current = release.ObservedToInfo(obs)
		}
	}
}
