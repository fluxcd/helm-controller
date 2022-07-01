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

package action

import (
	helmaction "helm.sh/helm/v3/pkg/action"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
)

// Rollback runs the Helm rollback action with the provided config, using the
// v2beta2.HelmReleaseSpec of the given object to determine the target release
// and rollback configuration.
//
// It does not determine if there is a desire to perform the action, this is
// expected to be done by the caller. In addition, it does not take note of the
// action result. The caller is expected to listen to this using a
// storage.ObserveFunc, which provides superior access to Helm storage writes.
func Rollback(config *helmaction.Configuration, obj *v2.HelmRelease) error {
	rollback := newRollback(config, obj)
	return rollback.Run(obj.GetReleaseName())
}

func newRollback(config *helmaction.Configuration, rel *v2.HelmRelease) *helmaction.Rollback {
	rollback := helmaction.NewRollback(config)

	rollback.Timeout = rel.Spec.GetRollback().GetTimeout(rel.GetTimeout()).Duration
	rollback.Wait = !rel.Spec.GetRollback().DisableWait
	rollback.WaitForJobs = !rel.Spec.GetRollback().DisableWaitForJobs
	rollback.DisableHooks = rel.Spec.GetRollback().DisableHooks
	rollback.Force = rel.Spec.GetRollback().Force
	rollback.Recreate = rel.Spec.GetRollback().Recreate
	rollback.CleanupOnFail = rel.Spec.GetRollback().CleanupOnFail

	if prev := rel.Status.Previous; prev != nil && prev.Name == rel.GetReleaseName() && prev.Namespace == rel.GetReleaseNamespace() {
		rollback.Version = prev.Version
	}

	return rollback
}
