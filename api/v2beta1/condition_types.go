/*
Copyright 2020 The Flux authors

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

package v2beta1

const (
	// ReleasedCondition represents the status of the last release attempt
	// (install/upgrade/test) against the latest desired state.
	ReleasedCondition string = "Released"

	// TestSuccessCondition represents the status of the last test attempt against
	// the latest desired state.
	TestSuccessCondition string = "TestSuccess"

	// RemediatedCondition represents the status of the last remediation attempt
	// (uninstall/rollback) due to a failure of the last release attempt against the
	// latest desired state.
	RemediatedCondition string = "Remediated"

	// InitFailedCondtion represents the failure of the initial setup steps of the helm release.
	InitFailedCondition string = "InitFailed"

	// RetriesExhaustedCondition represents the failues when the maximum number
	// or retries has been reached.
	RetriesExhaustedCondition string = "RetriesExhausted"
)

const (
	// InstallSucceededReason represents the fact that the Helm install for the
	// HelmRelease succeeded.
	InstallSucceededReason string = "InstallSucceeded"

	// InstallFailedReason represents the fact that the Helm install for the
	// HelmRelease failed.
	InstallFailedReason string = "InstallFailed"

	// UpgradeSucceededReason represents the fact that the Helm upgrade for the
	// HelmRelease succeeded.
	UpgradeSucceededReason string = "UpgradeSucceeded"

	// UpgradeFailedReason represents the fact that the Helm upgrade for the
	// HelmRelease failed.
	UpgradeFailedReason string = "UpgradeFailed"

	// TestSucceededReason represents the fact that the Helm tests for the
	// HelmRelease succeeded.
	TestSucceededReason string = "TestSucceeded"

	// TestFailedReason represents the fact that the Helm tests for the HelmRelease
	// failed.
	TestFailedReason string = "TestFailed"

	// RollbackSucceededReason represents the fact that the Helm rollback for the
	// HelmRelease succeeded.
	RollbackSucceededReason string = "RollbackSucceeded"

	// RollbackFailedReason represents the fact that the Helm test for the
	// HelmRelease failed.
	RollbackFailedReason string = "RollbackFailed"

	// UninstallSucceededReason represents the fact that the Helm uninstall for the
	// HelmRelease succeeded.
	UninstallSucceededReason string = "UninstallSucceeded"

	// UninstallFailedReason represents the fact that the Helm uninstall for the
	// HelmRelease failed.
	UninstallFailedReason string = "UninstallFailed"

	// ArtifactFailedReason represents the fact that the artifact download for the
	// HelmRelease failed.
	ArtifactFailedReason string = "ArtifactFailed"

	// InitFailedReason represents the fact that the initialization of the Helm
	// configuration failed.
	InitFailedReason string = "InitFailed"

	// GetLastReleaseFailedReason represents the fact that observing the last
	// release failed.
	GetLastReleaseFailedReason string = "GetLastReleaseFailed"

	// GetHelmChartFailedReason represents the fact that the Helm chart for
	// the release could not be fetched.
	GetHelmChartFailedReason string = "GetHelmChartFailed"

	// DependencyFailedReason represents the fact that resources the Helm release
	// depends on are not ready.
	DependencyNotReadyReason string = "DependencyNotReady"
)
