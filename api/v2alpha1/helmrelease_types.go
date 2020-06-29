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
	Values Values `json:"values,omitempty"`
}

type Values struct {
	// Data holds the configuration keys and values.
	// Work around for https://github.com/kubernetes-sigs/kubebuilder/issues/528
	Data map[string]interface{} `json:"-"`
}

// MarshalJSON marshals the Values data to a JSON blob.
func (in Values) MarshalJSON() ([]byte, error) {
	return json.Marshal(in.Data)
}

// UnmarshalJSON sets the Values to a copy of data.
func (in *Values) UnmarshalJSON(data []byte) error {
	var out map[string]interface{}
	err := json.Unmarshal(data, &out)
	if err != nil {
		return err
	}
	in.Data = out
	return nil
}

// DeepCopyInto is an deepcopy function, copying the receiver, writing
// into out. In must be non-nil. Declaring this here prevents it from
// being generated in zz_generated.deepcopy.go.
//
// This is defined here to work around https://github.com/kubernetes/code-generator/issues/50,
// and partially around https://github.com/kubernetes-sigs/controller-tools/pull/126
// and https://github.com/kubernetes-sigs/controller-tools/issues/294.
func (in *Values) DeepCopyInto(out *Values) {
	b, err := json.Marshal(in.Data)
	if err != nil {
		// The marshal should have been performed cleanly as otherwise
		// the resource would not have been created by the API server.
		panic(err)
	}
	var c map[string]interface{}
	err = json.Unmarshal(b, &c)
	if err != nil {
		panic(err)
	}
	out.Data = c
	return
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

func HelmReleaseNotReady(hr HelmRelease, reason, message string) HelmRelease {
	hr.Status.Conditions = []Condition{
		{
			Type:               ReadyCondition,
			Status:             corev1.ConditionFalse,
			LastTransitionTime: metav1.Now(),
			Reason:             reason,
			Message:            message,
		},
	}
	return hr
}

func HelmReleaseReady(hr HelmRelease, revision string, releaseRevision int, reason, message string) HelmRelease {
	hr.Status.Conditions = []Condition{
		{
			Type:               ReadyCondition,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             reason,
			Message:            message,
		},
	}
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
