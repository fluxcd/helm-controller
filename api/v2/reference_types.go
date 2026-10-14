/*
Copyright 2024 The Flux authors

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

import (
	"fmt"

	"k8s.io/apimachinery/pkg/types"

	"github.com/fluxcd/pkg/apis/meta"
)

// CrossNamespaceObjectReference contains enough information to let you locate
// the typed referenced object at cluster level.
type CrossNamespaceObjectReference struct {
	// APIVersion of the referent.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the referent.
	// +kubebuilder:validation:Enum=HelmRepository;GitRepository;Bucket
	// +required
	Kind string `json:"kind,omitempty"`

	// Name of the referent.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// Namespace of the referent.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Optional
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// CrossNamespaceSourceReference contains enough information to let you locate
// the typed referenced object at cluster level.
type CrossNamespaceSourceReference struct {
	// APIVersion of the referent.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the referent.
	// +kubebuilder:validation:Enum=OCIRepository;HelmChart;ExternalArtifact
	// +required
	Kind string `json:"kind"`

	// Name of the referent.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// Namespace of the referent, defaults to the namespace of the Kubernetes
	// resource object that contains the reference.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Optional
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// HelmChartReference holds a typed and namespaced reference for a Helm chart.
type HelmChartReference struct {
	// Kind of the Helm chart.
	// +kubebuilder:validation:Enum=HelmChart;OCIRepository;ExternalArtifact
	// +required
	Kind string `json:"kind"`

	// Name of the Helm chart.
	// +required
	Name string `json:"name"`

	// Namespace of the Helm chart.
	// +required
	Namespace string `json:"namespace"`
}

func (in *HelmChartReference) String() string {
	if in == nil {
		return ""
	}
	return fmt.Sprintf("%s/%s/%s", in.Kind, in.Namespace, in.Name)
}

func (in *HelmChartReference) GetObjectKey() types.NamespacedName {
	return types.NamespacedName{
		Namespace: in.Namespace,
		Name:      in.Name,
	}
}

func (in *HelmChartReference) Matches(other *HelmChartReference) bool {
	if in == nil || other == nil {
		return false
	}
	return *in == *other
}

// DependencyReference defines a HelmRelease dependency on another HelmRelease resource.
type DependencyReference = meta.DependencyReference
