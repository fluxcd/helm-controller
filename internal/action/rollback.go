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

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

// RollbackOption can be used to modify Helm's action.Rollback after the
// instructions from the v2.HelmRelease have been applied. This is for
// example useful to enable the dry-run setting as a CLI.
type RollbackOption func(*helmaction.Rollback)

// RollbackToVersion returns a RollbackOption which sets the version to
// roll back to.
func RollbackToVersion(version int) RollbackOption {
	return func(rollback *helmaction.Rollback) {
		rollback.Version = version
	}
}

// RollbackDryRun returns a RollbackOption which enables the dry-run setting.
func RollbackDryRun() RollbackOption {
	return func(rollback *helmaction.Rollback) {
		rollback.DryRun = true
	}
}

// Rollback runs the Helm rollback action with the provided config. Targeting
// a specific release or enabling dry-run is possible by providing
// RollbackToVersion and/or RollbackDryRun as options.
//
// It does not determine if there is a desire to perform the action, this is
// expected to be done by the caller. In addition, it does not take note of the
// action result. The caller is expected to listen to this using a
// storage.ObserveFunc, which provides superior access to Helm storage writes.
func Rollback(config *helmaction.Configuration, obj *v2.HelmRelease, releaseName string, opts ...RollbackOption) error {
	rollback := newRollback(config, obj, opts)
	return rollback.Run(releaseName)
}

func newRollback(config *helmaction.Configuration, obj *v2.HelmRelease, opts []RollbackOption) *helmaction.Rollback {
	rollback := helmaction.NewRollback(config)

	rollback.Timeout = obj.GetRollback().GetTimeout(obj.GetTimeout()).Duration
	rollback.Wait = !obj.GetRollback().DisableWait
	rollback.WaitForJobs = !obj.GetRollback().DisableWaitForJobs
	rollback.DisableHooks = obj.GetRollback().DisableHooks
	rollback.Force = obj.GetRollback().Force
	rollback.Recreate = obj.GetRollback().Recreate
	rollback.CleanupOnFail = obj.GetRollback().CleanupOnFail

	for _, opt := range opts {
		opt(rollback)
	}

	return rollback
}
