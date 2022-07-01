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

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
)

type Uninstall struct {
	configFactory *action.ConfigFactory
}

func (r *Uninstall) Reconcile(ctx context.Context, req *Request) error {
	var (
		cur    = req.Object.Status.Current.DeepCopy()
		logBuf = action.NewLogBuffer(action.NewDebugLog(ctrl.LoggerFrom(ctx).V(logger.InfoLevel)), 10)
		cfg    = r.configFactory.Build(logBuf.Log, observeUninstall(req.Object))
	)

	// Require current to run uninstall.
	if cur == nil {
		return fmt.Errorf("%w: required to uninstall", ErrNoCurrent)
	}

	// Run the uninstall action.
	res, err := action.Uninstall(ctx, cfg, req.Object)

	// The Helm uninstall action does always target the latest release. Before
	// accepting results, we need to confirm this is actually the release we
	// have recorded as Current.
	if res != nil && !release.ObserveRelease(res.Release).Targets(cur.Name, cur.Namespace, cur.Version) {
		err = fmt.Errorf("%w: uninstalled release %s/%s with version %d != current release %s/%s with version %d",
			ErrReleaseMismatch, res.Release.Namespace, res.Release.Name, res.Release.Version, cur.Namespace, cur.Name,
			cur.Version)
	}

	// The Helm uninstall action may return without an error while the update
	// to the storage failed. Detect this and return an error.
	if err == nil && cur == req.Object.Status.Current {
		err = fmt.Errorf("uninstallation completed without updating Helm storage")
	}

	// Handle any error.
	if err != nil {
		req.Object.Status.Failures++
		conditions.MarkFalse(req.Object, v2.RemediatedCondition, v2.UninstallFailedReason, err.Error())
		if req.Object.Status.Current == cur {
			return err
		}
		return nil
	}

	// Mark success.
	conditions.MarkTrue(req.Object, v2.RemediatedCondition, v2.UninstallSucceededReason,
		res.Release.Info.Description)
	return nil
}

func (r *Uninstall) Name() string {
	return "uninstall"
}

func (r *Uninstall) Type() ReconcilerType {
	return ReconcilerTypeRemediate
}

// observeUninstall returns a storage.ObserveFunc that can be used to observe
// and record the result of an uninstall action in the status of the given
// release. It updates the Status.Current field of the release if it equals the
// uninstallation target, and version = Current.Version.
func observeUninstall(obj *v2.HelmRelease) storage.ObserveFunc {
	return func(rls *helmrelease.Release) {
		if cur := obj.Status.Current; cur != nil {
			if obs := release.ObserveRelease(rls); obs.Targets(cur.Name, cur.Namespace, cur.Version) {
				obj.Status.Current = release.ObservedToInfo(obs)
			}
		}
	}
}
