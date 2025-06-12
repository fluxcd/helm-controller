/*
Copyright 2025 The Flux authors

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

package postrender

import (
	"bytes"
	"fmt"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

type OriginLabels struct {
	group     string
	name      string
	namespace string
}

type CommonRenderer struct {
	labels      map[string]string // Origin labels + Common labels to be applied to all resources
	annotations map[string]string // Common annotations to be applied to all resources
}

func NewOriginLabels(group, namespace, name string) *OriginLabels {
	return &OriginLabels{
		group:     group,
		name:      name,
		namespace: namespace,
	}
}

func (k *OriginLabels) Run(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error) {
	labels := originLabels(k.group, k.namespace, k.name)
	return transform(renderedManifests, labelTransformer(labels))
}

func NewCommonRenderer(commonMetadata *v2.CommonMetadata) *CommonRenderer {
	renderer := &CommonRenderer{}
	if commonMetadata.Labels != nil {
		renderer.labels = commonMetadata.Labels
	}
	if commonMetadata.Annotations != nil {
		renderer.annotations = commonMetadata.Annotations
	}
	return renderer
}

func (k *CommonRenderer) Run(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error) {
	var transformers []ResMapTransformer

	if k.labels != nil {
		transformers = append(transformers, labelTransformer(k.labels))
	}
	if k.annotations != nil {
		transformers = append(transformers, annotationTransformer(k.annotations))
	}

	if len(transformers) == 0 {
		return renderedManifests, nil
	}

	return transform(renderedManifests, transformers...)
}

func originLabels(group, namespace, name string) map[string]string {
	return map[string]string{
		fmt.Sprintf("%s/name", group):      name,
		fmt.Sprintf("%s/namespace", group): namespace,
	}
}
