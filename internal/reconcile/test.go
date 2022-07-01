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

	"github.com/fluxcd/pkg/runtime/logger"
	helmrelease "helm.sh/helm/v3/pkg/release"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/runtime/conditions"

	helmv2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
)

type Test struct {
	configFactory *action.ConfigFactory
}

func (r *Test) Reconcile(ctx context.Context, req *Request) error {
	var (
		cur    = req.Object.Status.Current.DeepCopy()
		logBuf = action.NewLogBuffer(action.NewDebugLog(ctrl.LoggerFrom(ctx).V(logger.InfoLevel)), 10)
	)

	// We only accept test results for the current release.
	if cur == nil {
		return fmt.Errorf("%w: required for test", ErrNoCurrent)
	}

	// Run tests.
	rls, err := action.Test(ctx, r.configFactory.Build(logBuf.Log, observeTest(req.Object)), req.Object)

	// The Helm test action does always target the latest release. Before
	// accepting results, we need to confirm this is actually the release we
	// have recorded as Current.
	if rls != nil && !release.ObserveRelease(rls).Targets(cur.Name, cur.Namespace, cur.Version) {
		err = fmt.Errorf("%w: tested release %s/%s with version %d != current release %s/%s with version %d",
			ErrReleaseMismatch, rls.Namespace, rls.Name, rls.Version, cur.Namespace, cur.Name, cur.Version)
	}

	// Something went wrong.
	if err != nil {
		req.Object.Status.Failures++
		conditions.MarkFalse(req.Object, helmv2.TestSuccessCondition, helmv2.TestFailedReason, err.Error())
		// If we failed to observe anything happened at all, we want to retry
		// and return the error to indicate this.
		if req.Object.Status.Current == cur {
			return err
		}
		return nil
	}

	// Compose success condition message.
	condMsg := "No test hooks."
	if hookLen := len(req.Object.Status.Current.TestHooks); hookLen > 0 {
		condMsg = fmt.Sprintf("%d test hook(s) completed successfully.", hookLen)
	}
	conditions.MarkTrue(req.Object, helmv2.TestSuccessCondition, helmv2.TestSucceededReason, condMsg)
	return nil
}

func (r *Test) Name() string {
	return "test"
}

func (r *Test) Type() ReconcilerType {
	return ReconcilerTypeTest
}

func observeTest(obj *helmv2.HelmRelease) storage.ObserveFunc {
	return func(rls *helmrelease.Release) {
		if cur := obj.Status.Current; cur != nil {
			obs := release.ObserveRelease(rls)
			if obs.Targets(cur.Name, cur.Namespace, cur.Version) {
				obj.Status.Current = release.ObservedToInfo(obs)
				if hooks := release.TestHooksFromRelease(rls); len(hooks) > 0 {
					obj.Status.Current.TestHooks = hooks
				}
			}
		}
	}
}
