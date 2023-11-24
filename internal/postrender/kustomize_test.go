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
	"encoding/json"
	"testing"

	. "github.com/onsi/gomega"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/yaml"

	"github.com/fluxcd/pkg/apis/kustomize"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
)

const replaceImageMock = `apiVersion: v1
kind: Pod
metadata:
  name: image
spec:
  containers:
  - image: repository/image:tag
`

const json6902Mock = `apiVersion: v1
kind: Pod
metadata:
  annotations:
    c: foo
  name: json6902
`

const strategicMergeMock = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
spec:
  template:
    spec:
      containers:
        - name: nginx
          image: nignx:v1.0.0
`

func Test_postRendererKustomize_Run(t *testing.T) {
	tests := []struct {
		name                  string
		renderedManifests     string
		patches               string
		patchesStrategicMerge string
		patchesJson6902       string
		images                string
		expectManifests       string
		expectErr             bool
	}{
		{
			name:              "image tag",
			renderedManifests: replaceImageMock,
			images: `
- name: repository/image
  newTag: 0.1.0
`,
			expectManifests: `apiVersion: v1
kind: Pod
metadata:
  name: image
spec:
  containers:
  - image: repository/image:0.1.0
`,
		},
		{
			name:              "image name",
			renderedManifests: replaceImageMock,
			images: `
- name: repository/image
  newName: repository/new-image
`,
			expectManifests: `apiVersion: v1
kind: Pod
metadata:
  name: image
spec:
  containers:
  - image: repository/new-image:tag
`,
		},
		{
			name:              "image digest",
			renderedManifests: replaceImageMock,
			images: `
- name: repository/image
  digest: sha256:24a0c4b4a4c0eb97a1aabb8e29f18e917d05abfe1b7a7c07857230879ce7d3d3
`,
			expectManifests: `apiVersion: v1
kind: Pod
metadata:
  name: image
spec:
  containers:
  - image: repository/image@sha256:24a0c4b4a4c0eb97a1aabb8e29f18e917d05abfe1b7a7c07857230879ce7d3d3
`,
		},
		{
			name:              "json 6902",
			renderedManifests: json6902Mock,
			patchesJson6902: `
- target:
    version: v1
    kind: Pod
    name: json6902
  patch:
    - op: test
      path: /metadata/annotations/c
      value: foo
    - op: remove
      path: /metadata/annotations/c
    - op: add
      path: /metadata/annotations/c
      value: [ "foo", "bar" ]
    - op: replace
      path: /metadata/annotations/c
      value: 42
    - op: move
      from: /metadata/annotations/c
      path: /metadata/annotations/d
    - op: copy
      from: /metadata/annotations/d
      path: /metadata/annotations/e
`,
			expectManifests: `apiVersion: v1
kind: Pod
metadata:
  annotations:
    d: "42"
    e: "42"
  name: json6902
`,
		},
		{
			name:              "targeted json 6902",
			renderedManifests: json6902Mock,
			patches: `
- target:
    version: v1
    kind: Pod
    name: json6902
  patch: |
    - op: test
      path: /metadata/annotations/c
      value: foo
    - op: remove
      path: /metadata/annotations/c
    - op: add
      path: /metadata/annotations/c
      value: [ "foo", "bar" ]
    - op: replace
      path: /metadata/annotations/c
      value: 42
    - op: move
      from: /metadata/annotations/c
      path: /metadata/annotations/d
    - op: copy
      from: /metadata/annotations/d
      path: /metadata/annotations/e
`,
			expectManifests: `apiVersion: v1
kind: Pod
metadata:
  annotations:
    d: "42"
    e: "42"
  name: json6902
`,
		},
		{
			name:              "strategic merge test",
			renderedManifests: strategicMergeMock,
			patchesStrategicMerge: `
- apiVersion: apps/v1
  kind: Deployment
  metadata:
    name: nginx
  spec:
    template:
      spec:
        containers:
          - name: nginx
            image: nignx:latest
`,
			expectManifests: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
spec:
  template:
    spec:
      containers:
      - image: nignx:latest
        name: nginx
`,
		},
		{
			name:              "targeted strategic merge test",
			renderedManifests: strategicMergeMock,
			patches: `
- target:
    group: apps
    version: v1
    kind: Deployment
    name: nginx
  patch: |
    apiVersion: apps/v1
    kind: Deployment
    metadata:
      name: nginx
    spec:
      template:
        spec:
          containers:
            - name: nginx
              image: nignx:latest
`,
			expectManifests: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
spec:
  template:
    spec:
      containers:
      - image: nignx:latest
        name: nginx
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			spec, err := mockKustomize(tt.patches, tt.patchesStrategicMerge, tt.patchesJson6902, tt.images)
			g.Expect(err).ToNot(HaveOccurred())

			k := &Kustomize{
				Patches:               spec.Patches,
				PatchesStrategicMerge: spec.PatchesStrategicMerge,
				PatchesJSON6902:       spec.PatchesJSON6902,
				Images:                spec.Images,
			}
			gotModifiedManifests, err := k.Run(bytes.NewBufferString(tt.renderedManifests))
			if tt.expectErr {
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(gotModifiedManifests.String()).To(BeEmpty())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(gotModifiedManifests).To(Equal(bytes.NewBufferString(tt.expectManifests)))
		})
	}
}

func mockKustomize(patches, patchesStrategicMerge, patchesJson6902, images string) (*v2.Kustomize, error) {
	var targeted []kustomize.Patch
	if err := yaml.Unmarshal([]byte(patches), &targeted); err != nil {
		return nil, err
	}
	b, err := yaml.YAMLToJSON([]byte(patchesStrategicMerge))
	if err != nil {
		return nil, err
	}
	var strategicMerge []v1.JSON
	if err := json.Unmarshal(b, &strategicMerge); err != nil {
		return nil, err
	}
	var json6902 []kustomize.JSON6902Patch
	if err := yaml.Unmarshal([]byte(patchesJson6902), &json6902); err != nil {
		return nil, err
	}
	var imgs []kustomize.Image
	if err := yaml.Unmarshal([]byte(images), &imgs); err != nil {
		return nil, err
	}
	return &v2.Kustomize{
		Patches:               targeted,
		PatchesStrategicMerge: strategicMerge,
		PatchesJSON6902:       json6902,
		Images:                imgs,
	}, nil
}
