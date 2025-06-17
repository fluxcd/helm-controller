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

	"sigs.k8s.io/kustomize/api/builtins"
	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/kustomize/api/resmap"
	kustypes "sigs.k8s.io/kustomize/api/types"
)

type ResMapTransformer interface {
	Transform(resmap.ResMap) error
}

func transform(renderedManifests *bytes.Buffer, transformers ...ResMapTransformer) (*bytes.Buffer, error) {
	resFactory := provider.NewDefaultDepProvider().GetResourceFactory()
	resMapFactory := resmap.NewFactory(resFactory)

	resMap, err := resMapFactory.NewResMapFromBytes(renderedManifests.Bytes())
	if err != nil {
		return nil, err
	}

	for _, t := range transformers {
		if err := t.Transform(resMap); err != nil {
			return nil, err
		}
	}

	yaml, err := resMap.AsYaml()
	if err != nil {
		return nil, err
	}

	return bytes.NewBuffer(yaml), nil
}

func labelTransformer(labels map[string]string) ResMapTransformer {
	return &builtins.LabelTransformerPlugin{
		Labels: labels,
		FieldSpecs: []kustypes.FieldSpec{
			{Path: "metadata/labels", CreateIfNotPresent: true},
		},
	}
}

func annotationTransformer(annotations map[string]string) ResMapTransformer {
	return &builtins.AnnotationsTransformerPlugin{
		Annotations: annotations,
		FieldSpecs: []kustypes.FieldSpec{
			{Path: "metadata/annotations", CreateIfNotPresent: true},
		},
	}
}
