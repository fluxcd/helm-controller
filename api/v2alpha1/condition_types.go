/*
Copyright 2020 The Flux CD contributors.

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

package v2alpha1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition contains condition information for a HelmRelease.
type Condition struct {
	// Type of the condition, one of ('Ready', 'Install', 'Upgrade', 'Test', 'Rollback', 'Uninstall').
	// +required
	Type string `json:"type"`

	// Status of the condition, one of ('True', 'False', 'Unknown').
	// +required
	Status corev1.ConditionStatus `json:"status"`

	// LastTransitionTime is the timestamp corresponding to the last status
	// change of this condition.
	// +required
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	// Reason is a brief machine readable explanation for the condition's last
	// transition.
	// +required
	Reason string `json:"reason,omitempty"`

	// Message is a human readable description of the details of the last
	// transition, complementing reason.
	// +optional
	Message string `json:"message,omitempty"`
}

const (
	// ReadyCondition represents the fact that the HelmRelease has been successfully reconciled.
	ReadyCondition string = "Ready"

	// InstalledCondition represents the fact that the HelmRelease has been successfully installed.
	InstalledCondition string = "Installed"

	// UpgradedCondition represents the fact that the HelmRelease has been successfully upgraded.
	UpgradedCondition string = "Upgraded"

	// TestedCondition represents the fact that the HelmRelease has been successfully tested.
	TestedCondition string = "Tested"

	// RolledBackCondition represents the fact that the HelmRelease has been successfully rolled back.
	RolledBackCondition string = "RolledBack"

	// UninstalledCondition represents the fact that the HelmRelease has been successfully uninstalled.
	UninstalledCondition string = "Uninstalled"
)

const (
	// ReconciliationSucceededReason represents the fact that the reconciliation of the HelmRelease has succeeded.
	ReconciliationSucceededReason string = "ReconciliationSucceeded"

	// ReconciliationFailedReason represents the fact that the reconciliation of the HelmRelease has failed.
	ReconciliationFailedReason string = "ReconciliationFailed"

	// InstallSucceededReason represents the fact that the Helm install for the HelmRelease succeeded.
	InstallSucceededReason string = "InstallSucceeded"

	// InstallFailedReason represents the fact that the Helm install for the HelmRelease failed.
	InstallFailedReason string = "InstallFailed"

	// UpgradeSucceededReason represents the fact that the Helm upgrade for the HelmRelease succeeded.
	UpgradeSucceededReason string = "UpgradeSucceeded"

	// UpgradeFailedReason represents the fact that the Helm upgrade for the HelmRelease failed.
	UpgradeFailedReason string = "UpgradeFailed"

	// TestSucceededReason represents the fact that the Helm test for the HelmRelease succeeded.
	TestSucceededReason string = "TestSucceeded"

	// TestFailedReason represents the fact that the Helm test for the HelmRelease failed.
	TestFailedReason string = "TestFailed"

	// RollbackSucceededReason represents the fact that the Helm rollback for the HelmRelease succeeded.
	RollbackSucceededReason string = "RollbackSucceeded"

	// RollbackFailedReason represents the fact that the Helm test for the HelmRelease failed.
	RollbackFailedReason string = "RollbackFailed"

	// UninstallSucceededReason represents the fact that the Helm uninstall for the HelmRelease succeeded.
	UninstallSucceededReason string = "UninstallSucceeded"

	// UninstallFailedReason represents the fact that the Helm uninstall for the HelmRelease failed.
	UninstallFailedReason string = "UninstallFailed"

	// ArtifactFailedReason represents the fact that the artifact download for the HelmRelease failed.
	ArtifactFailedReason string = "ArtifactFailed"

	// InitFailedReason represents the fact that the initialization of the Helm configuration failed.
	InitFailedReason string = "InitFailed"

	// ProgressingReason represents the fact that the reconciliation for the resource is underway.
	ProgressingReason string = "Progressing"

	// DependencyNotReadyReason represents the fact that the one of the dependencies is not ready.
	DependencyNotReadyReason string = "DependencyNotReady"

	// SuspendedReason represents the fact that the reconciliation of the HelmRelease is suspended.
	SuspendedReason string = "Suspended"
)
