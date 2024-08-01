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
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/openfluxcd/artifact/utils"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"
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
	// +kubebuilder:validation:Enum=OCIRepository;HelmChart
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

func (s *CrossNamespaceSourceReference) GetObjectKey() ctrlclient.ObjectKey {
	return ctrlclient.ObjectKey{
		Namespace: s.Namespace,
		Name:      s.Name,
	}
}

func (s *CrossNamespaceSourceReference) GetGroupKind() schema.GroupKind {
	if s.APIVersion == "" {
		return schema.GroupKind{
			Group: sourcev1.GroupVersion.Group,
			Kind:  s.Kind,
		}
	}

	return schema.GroupKind{
		Group: utils.ExtractGroupName(s.APIVersion),
		Kind:  s.Kind,
	}
}

func (s *CrossNamespaceSourceReference) GetName() string {
	return s.Name
}

func (s *CrossNamespaceSourceReference) GetNamespace() string {
	return s.Namespace
}

func (s *CrossNamespaceSourceReference) String() string {
	if s.GetNamespace() != "" {
		return fmt.Sprintf("%s/%s/%s/%s", s.GetGroupKind().Group, s.GetGroupKind().Kind, s.GetNamespace(), s.GetName())
	}
	return fmt.Sprintf("%s/%s/%s", s.GetGroupKind().Group, s.GetGroupKind().Kind, s.GetName())
}

// ValuesReference contains a reference to a resource containing Helm values,
// and optionally the key they can be found at.
type ValuesReference struct {
	// Kind of the values referent, valid values are ('Secret', 'ConfigMap').
	// +kubebuilder:validation:Enum=Secret;ConfigMap
	// +required
	Kind string `json:"kind"`

	// Name of the values referent. Should reside in the same namespace as the
	// referring resource.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// ValuesKey is the data key where the values.yaml or a specific value can be
	// found at. Defaults to 'values.yaml'.
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^[\-._a-zA-Z0-9]+$`
	// +optional
	ValuesKey string `json:"valuesKey,omitempty"`

	// TargetPath is the YAML dot notation path the value should be merged at. When
	// set, the ValuesKey is expected to be a single flat value. Defaults to 'None',
	// which results in the values getting merged at the root.
	// +kubebuilder:validation:MaxLength=250
	// +kubebuilder:validation:Pattern=`^([a-zA-Z0-9_\-.\\\/]|\[[0-9]{1,5}\])+$`
	// +optional
	TargetPath string `json:"targetPath,omitempty"`

	// Optional marks this ValuesReference as optional. When set, a not found error
	// for the values reference is ignored, but any ValuesKey, TargetPath or
	// transient error will still result in a reconciliation failure.
	// +optional
	Optional bool `json:"optional,omitempty"`
}

// GetValuesKey returns the defined ValuesKey, or the default ('values.yaml').
func (in ValuesReference) GetValuesKey() string {
	if in.ValuesKey == "" {
		return "values.yaml"
	}
	return in.ValuesKey
}
