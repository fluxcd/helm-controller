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

	"github.com/fluxcd/pkg/runtime/conditions"
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
)

type Unlock struct {
	configFactory *action.ConfigFactory
}

func (r *Unlock) Reconcile(_ context.Context, req *Request) error {
	// We can only unlock a release if we have a current.
	cur := req.Object.Status.Current.DeepCopy()
	if cur == nil {
		return fmt.Errorf("%w: required for unlock", ErrNoCurrent)
	}

	// Build action configuration to gain access to Helm storage.
	cfg := r.configFactory.Build(nil, observeUnlock(req.Object))

	// Retrieve last release object.
	rls, err := cfg.Releases.Last(req.Object.GetReleaseName())
	if err != nil {
		// Ignore not found error. Assume caller will decide what to do
		// when it re-assess state to determine the next action.
		if errors.Is(err, helmdriver.ErrReleaseNotFound) {
			return nil
		}
		// Return any other error to retry.
		return err
	}

	// Ensure latest is still same as current.
	obs := release.ObserveRelease(rls)
	if obs.Targets(cur.Name, cur.Namespace, cur.Version) {
		if status := rls.Info.Status; status.IsPending() {
			// Update pending status to failed and persist.
			rls.SetStatus(helmrelease.StatusFailed, fmt.Sprintf("Release unlocked from stale '%s' state",
				status.String()))
			if err = cfg.Releases.Update(rls); err != nil {
				req.Object.Status.Failures++
				conditions.MarkFalse(req.Object, v2.ReleasedCondition, "StalePending",
					"Failed to unlock release from stale '%s' state: %s", status.String(), err.Error())
				return err
			}
			conditions.MarkFalse(req.Object, v2.ReleasedCondition, "StalePending", rls.Info.Description)
		}
	}
	return nil
}

func (r *Unlock) Name() string {
	return "unlock"
}

func (r *Unlock) Type() ReconcilerType {
	return ReconcilerTypeUnlock
}

// observeUnlock returns a storage.ObserveFunc that can be used to observe and
// record the result of an unlock action in the status of the given release.
// It updates the Status.Current field of the release if it equals the target
// of the unlock action.
func observeUnlock(obj *v2.HelmRelease) storage.ObserveFunc {
	return func(rls *helmrelease.Release) {
		if cur := obj.Status.Current; cur != nil {
			obs := release.ObserveRelease(rls)
			if obs.Targets(cur.Name, cur.Namespace, cur.Version) {
				obj.Status.Current = release.ObservedToInfo(obs)
			}
		}
	}
}
