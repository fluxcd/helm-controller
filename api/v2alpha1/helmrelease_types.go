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
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HelmReleaseSpec defines the desired state of HelmRelease
type HelmReleaseSpec struct {
	// SourceRef of the HelmChart source.
	// +required
	SourceRef corev1.TypedLocalObjectReference `json:"sourceRef"`

	// Interval at which to reconcile the Helm release.
	// +required
	Interval metav1.Duration `json:"interval"`

	// Wait tells the reconciler to wait with marking a Helm action as
	// successful until all resources are in a ready state. When set, it will
	// wait for as long as 'Timeout'.
	// +optional
	Wait bool `json:"wait,omitempty"`

	// MaxHistory is the number of revisions saved by Helm for this release.
	// Use '0' for an unlimited number of revisions; defaults to '10'.
	// +optional
	MaxHistory *int `json:"maxHistory,omitempty"`

	// Values holds the values for this Helm release.
	// +optional
	Values apiextensionsv1.JSON `json:"values,omitempty"`
}

// HelmReleaseStatus defines the observed state of HelmRelease
type HelmReleaseStatus struct {
	// +optional
	Conditions []Condition `json:"conditions,omitempty"`

	// LastAppliedRevision is the revision of the last successfully applied source.
	// +optional
	LastAppliedRevision string `json:"lastAppliedRevision,omitempty"`

	// LastReleaseRevision is the revision of the last successfully Helm release.
	// +optional
	LastReleaseRevision int `json:"lastReleaseRevision,omitempty"`
}

// HelmReleaseProgressing resets the conditions of the given HelmRelease to a single
// ReadyCondition with status ConditionUnknown.
func HelmReleaseProgressing(hr HelmRelease) HelmRelease {
	hr.Status.Conditions = []Condition{
		{
			Type:               ReadyCondition,
			Status:             corev1.ConditionUnknown,
			LastTransitionTime: metav1.Now(),
			Reason:             ProgressingReason,
			Message:            "reconciliation in progress",
		},
	}
	return hr
}

// SetHelmReleaseCondition sets the given condition with the given status, reason and message
// on the HelmRelease.
func SetHelmReleaseCondition(hr *HelmRelease, condition string, status corev1.ConditionStatus, reason, message string) {
	hr.Status.Conditions = filterOutCondition(hr.Status.Conditions, condition)
	hr.Status.Conditions = append(hr.Status.Conditions, Condition{
		Type:               condition,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
}

// HelmReleaseNotReady sets the status of the ReadyCondition of the given HelmRelease to
// ConditionFalse including the given reason and message.
func HelmReleaseNotReady(hr HelmRelease, reason, message string) HelmRelease {
	hr.Status.Conditions = filterOutCondition(hr.Status.Conditions, ReadyCondition)
	hr.Status.Conditions = append(hr.Status.Conditions, Condition{
		Type:               ReadyCondition,
		Status:             corev1.ConditionFalse,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
	return hr
}

// HelmReleaseReady sets the status of the ReadyCondition of the given HelmRelease to
// ConditionTrue including the given reason and message, and sets the LastAppliedRevision
// and LastReleaseRevision to the given values.
func HelmReleaseReady(hr HelmRelease, revision string, releaseRevision int, reason, message string) HelmRelease {
	hr.Status.Conditions = filterOutCondition(hr.Status.Conditions, ReadyCondition)
	hr.Status.Conditions = append(hr.Status.Conditions, Condition{
		Type:               ReadyCondition,
		Status:             corev1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	})
	hr.Status.LastAppliedRevision = revision
	hr.Status.LastReleaseRevision = releaseRevision
	return hr
}

const (
	// ReconcileAtAnnotation is the annotation used for triggering a
	// reconciliation outside of the defined schedule.
	ReconcileAtAnnotation string = "helm.fluxcd.io/reconcileAt"

	// SourceIndexKey is the key used for indexing HelmReleases based on
	// their sources.
	SourceIndexKey string = ".metadata.source"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// HelmRelease is the Schema for the helmreleases API
type HelmRelease struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   HelmReleaseSpec   `json:"spec,omitempty"`
	Status HelmReleaseStatus `json:"status,omitempty"`
}

// GetValues unmarshals the raw values to a map[string]interface{}
// and returns the result.
func (in *HelmRelease) GetValues() map[string]interface{} {
	var values map[string]interface{}
	_ = json.Unmarshal(in.Spec.Values.Raw, &values)
	return values
}

// +kubebuilder:object:root=true

// HelmReleaseList contains a list of HelmRelease
type HelmReleaseList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HelmRelease `json:"items"`
}

func init() {
	SchemeBuilder.Register(&HelmRelease{}, &HelmReleaseList{})
}

// filterOutCondition returns a new slice of conditions without the
// condition of the given type.
func filterOutCondition(conditions []Condition, condition string) []Condition {
	var newConditions []Condition
	for _, c := range conditions {
		if c.Type == condition {
			continue
		}
		newConditions = append(newConditions, c)
	}
	return newConditions
}
