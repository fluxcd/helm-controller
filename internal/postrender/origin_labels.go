/*
Copyright 2021 The Flux authors

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

	"sigs.k8s.io/kustomize/api/builtins"
	"sigs.k8s.io/kustomize/api/provider"
	"sigs.k8s.io/kustomize/api/resmap"
	kustypes "sigs.k8s.io/kustomize/api/types"
)

func NewOriginLabels(group, namespace, name string) *OriginLabels {
	return &OriginLabels{
		group:     group,
		name:      name,
		namespace: namespace,
	}
}

type OriginLabels struct {
	group     string
	name      string
	namespace string
}

func (k *OriginLabels) Run(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error) {
	resFactory := provider.NewDefaultDepProvider().GetResourceFactory()
	resMapFactory := resmap.NewFactory(resFactory)

	resMap, err := resMapFactory.NewResMapFromBytes(renderedManifests.Bytes())
	if err != nil {
		return nil, err
	}

	labelTransformer := builtins.LabelTransformerPlugin{
		Labels: originLabels(k.group, k.namespace, k.name),
		FieldSpecs: []kustypes.FieldSpec{
			{Path: "metadata/labels", CreateIfNotPresent: true},
		},
	}
	if err := labelTransformer.Transform(resMap); err != nil {
		return nil, err
	}

	yaml, err := resMap.AsYaml()
	if err != nil {
		return nil, err
	}

	return bytes.NewBuffer(yaml), nil
}

func originLabels(group, namespace, name string) map[string]string {
	return map[string]string{
		fmt.Sprintf("%s/name", group):      name,
		fmt.Sprintf("%s/namespace", group): namespace,
	}
}
