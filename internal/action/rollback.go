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
	"fmt"

	helmaction "helm.sh/helm/v4/pkg/action"
	helmrelease "helm.sh/helm/v4/pkg/release/v1"

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

// Rollback runs the Helm rollback action with the provided config. Targeting
// a specific release or enabling dry-run is possible by providing
// RollbackToVersion as option.
//
// It does not determine if there is a desire to perform the action, this is
// expected to be done by the caller. In addition, it does not take note of the
// action result. The caller is expected to listen to this using a
// storage.ObserveFunc, which provides superior access to Helm storage writes.
func Rollback(config *helmaction.Configuration, obj *v2.HelmRelease, releaseName string, opts ...RollbackOption) error {
	rollback := newRollback(config, obj, opts)

	// Resolve "auto" server-side apply setting.
	// We need to copy this code from Helm because we need to set ForceConflicts
	// based on the resolved value, since we always force conflicts on server-side apply
	// (Helm does not).
	serverSideApply := rollback.ServerSideApply == "true"
	if rollback.ServerSideApply == "auto" {
		currentRelease, err := config.Releases.Last(releaseName)
		if err != nil {
			return err
		}
		currentReleaseTyped, ok := currentRelease.(*helmrelease.Release)
		if !ok {
			return fmt.Errorf("only the Chart API v2 is supported")
		}
		previousVersion := rollback.Version
		if rollback.Version == 0 {
			previousVersion = currentReleaseTyped.Version - 1
		}
		historyReleases, err := config.Releases.History(releaseName)
		if err != nil {
			return err
		}
		previousVersionExist := false
		for _, rlsr := range historyReleases {
			rlsrTyped, ok := rlsr.(*helmrelease.Release)
			if !ok {
				return fmt.Errorf("only the Chart API v2 is supported")
			}
			if previousVersion == rlsrTyped.Version {
				previousVersionExist = true
				break
			}
		}
		if !previousVersionExist {
			return fmt.Errorf("release has no %d version", previousVersion)
		}
		previousRelease, err := config.Releases.Get(releaseName, previousVersion)
		if err != nil {
			return err
		}
		previousReleaseTyped, ok := previousRelease.(*helmrelease.Release)
		if !ok {
			return fmt.Errorf("only the Chart API v2 is supported")
		}
		serverSideApply = previousReleaseTyped.ApplyMethod == "ssa"
		rollback.ServerSideApply = fmt.Sprint(serverSideApply)
	}
	rollback.ForceConflicts = serverSideApply // We always force conflicts on server-side apply.

	return rollback.Run(releaseName)
}

func newRollback(config *helmaction.Configuration, obj *v2.HelmRelease, opts []RollbackOption) *helmaction.Rollback {
	rollback := helmaction.NewRollback(config)
	rollback.ServerSideApply = "auto" // This must be the rollback default regardless of UseHelm3Defaults.
	if ssa := obj.GetRollback().ServerSideApply; ssa != "" {
		rollback.ServerSideApply = toHelmSSAValue(ssa)
	}

	rollback.Timeout = obj.GetRollback().GetTimeout(obj.GetTimeout()).Duration
	rollback.WaitStrategy = getWaitStrategy(obj.GetRollback())
	rollback.WaitForJobs = !obj.GetRollback().DisableWaitForJobs
	rollback.DisableHooks = obj.GetRollback().DisableHooks
	rollback.ForceReplace = obj.GetRollback().Force
	rollback.CleanupOnFail = obj.GetRollback().CleanupOnFail
	rollback.MaxHistory = obj.GetMaxHistory()

	for _, opt := range opts {
		opt(rollback)
	}

	return rollback
}
