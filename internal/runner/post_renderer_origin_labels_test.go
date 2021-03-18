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

package runner

import (
	"bytes"
	"reflect"
	"testing"
)

const mixedResourceMock = `apiVersion: v1
kind: Pod
metadata:
  name: pod-without-labels
---
apiVersion: v1
kind: Service
metadata:
  name: service-with-labels
  labels:
    existing: label
`

func Test_postRendererOriginLabels_Run(t *testing.T) {
	tests := []struct {
		name              string
		renderedManifests string
		expectManifests   string
		expectErr         bool
	}{
		{
			name:              "labels",
			renderedManifests: mixedResourceMock,
			expectManifests: `apiVersion: v1
kind: Pod
metadata:
  labels:
    helm.toolkit.fluxcd.io/name: name
    helm.toolkit.fluxcd.io/namespace: namespace
  name: pod-without-labels
---
apiVersion: v1
kind: Service
metadata:
  labels:
    existing: label
    helm.toolkit.fluxcd.io/name: name
    helm.toolkit.fluxcd.io/namespace: namespace
  name: service-with-labels
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			k := &postRendererOriginLabels{
				name:      "name",
				namespace: "namespace",
			}
			gotModifiedManifests, err := k.Run(bytes.NewBufferString(tt.renderedManifests))
			if (err != nil) != tt.expectErr {
				t.Errorf("Run() error = %v, expectErr %v", err, tt.expectErr)
				return
			}
			if !reflect.DeepEqual(gotModifiedManifests, bytes.NewBufferString(tt.expectManifests)) {
				t.Errorf("Run() gotModifiedManifests = %v, want %v", gotModifiedManifests, tt.expectManifests)
			}
		})
	}
}
