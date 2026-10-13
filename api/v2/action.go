/*
Copyright 2026 The Flux authors

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

package v2

// Action describes an observable stage of the reconcile loop, from checking
// dependencies and resolving the chart source and values, through performing
// Helm operations (install, upgrade, test, rollback, uninstall), to drift
// detection, health checking and unlocking stale releases.
type Action string

// String returns the string representation of the Action.
func (a Action) String() string {
	return string(a)
}

const (
	// ActionReconcile denotes the overall outcome of the reconcile loop,
	// emitted once per run to report that reconciliation finished or failed.
	ActionReconcile Action = "Reconcile"

	// ActionCheckDependencies verifies that every HelmRelease referenced by
	// spec.dependsOn is ready before reconciliation proceeds.
	ActionCheckDependencies Action = "CheckDependencies"

	// ActionResolveSource resolves the chart source reference, enforcing
	// cross-namespace access controls, and loads the chart artifact.
	ActionResolveSource Action = "ResolveSource"

	// ActionResolveValues composes the chart values from the spec and the
	// referenced ConfigMaps and Secrets.
	ActionResolveValues Action = "ResolveValues"

	// ActionHealthCheck waits on the health of the release resources and
	// cancels in-flight checks when a new revision is observed.
	ActionHealthCheck Action = "HealthCheck"

	// ActionDriftDetection detects and corrects drift between the cluster
	// state and the manifests persisted in Helm storage.
	ActionDriftDetection Action = "DriftDetection"

	// ActionHelmInstall installs the Helm release.
	ActionHelmInstall Action = "HelmInstall"

	// ActionHelmUpgrade upgrades the Helm release.
	ActionHelmUpgrade Action = "HelmUpgrade"

	// ActionHelmRollback rolls back the Helm release to the last deployed
	// revision.
	ActionHelmRollback Action = "HelmRollback"

	// ActionHelmTest runs the test hooks of the Helm release.
	ActionHelmTest Action = "HelmTest"

	// ActionHelmUninstall uninstalls the Helm release.
	ActionHelmUninstall Action = "HelmUninstall"

	// ActionUnlock unlocks a Helm release left in a pending state.
	ActionUnlock Action = "Unlock"
)
