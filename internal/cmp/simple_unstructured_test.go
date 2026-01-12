/*
Copyright 2023 The Flux authors

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

package cmp

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

func TestSimpleYAMLReporter_Report(t *testing.T) {
	tests := []struct {
		name string
		x    string
		y    string
		want string
	}{
		{
			name: "added simple value",
			x: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec: {}`,
			y: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  replicas: 1`,
			want: `.spec.replicas
+1`,
		},
		{
			name: "removed simple value",
			x: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  replicas: 1`,
			y: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec: {}`,
			want: `.spec.replicas
-1`,
		},
		{
			name: "changed simple value",
			x: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  replicas: 1`,
			y: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  replicas: 2`,
			want: `.spec.replicas
-1
+2`,
		},
		{
			name: "added map with value",
			x: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  template:
    spec: {}`,
			y: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  template:
    spec:
      containers:
      - name: container`,
			want: `.spec.template.spec.containers
+[map[name:container]]`,
		},
		{
			name: "removed map with value",
			x: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  template:
    spec:
      containers:
      - name: container`,
			y: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  template:
    spec: {}`,
			want: `.spec.template.spec.containers
-[map[name:container]]`,
		},
		{
			name: "changed map with value",
			x: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  template:
    spec:
      containers:
      - name: container`,
			y: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  template:
    spec:
      containers:
      - name: container2`,
			want: `.spec.template.spec.containers[0].name
-container
+container2`,
		},
		{
			name: "added list item value",
			x: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  template:
    spec:
      containers:
      - name: container
        env: []`,
			y: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  template:
    spec:
      containers:
      - name: container
        env:
        - name: env`,
			want: `.spec.template.spec.containers[0].env[?->0]
+map[name:env]`,
		},
		{
			name: "removed list item value",
			x: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  template:
    spec:
      containers:
      - name: container
        env:
        - name: env`,
			y: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  template:
    spec:
      containers:
      - name: container
        env: []`,
			want: `.spec.template.spec.containers[0].env[0->?]
-map[name:env]`,
		},
		{
			name: "changed list item value",
			x: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  template:
    spec:
      containers:
      - name: container
        env:
        - name: env`,
			y: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
spec:
  template:
    spec:
      containers:
      - name: container
        env:
        - name: env2`,
			want: `.spec.template.spec.containers[0].env[0].name
-env
+env2`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			x, err := yamlToUnstructured(tt.x)
			if err != nil {
				t.Fatal("failed to parse x", err)
			}
			y, err := yamlToUnstructured(tt.y)
			if err != nil {
				t.Fatal("failed to parse y", err)
			}

			r := SimpleUnstructuredReporter{}
			_ = cmp.Diff(x, y, cmp.Reporter(&r))
			result := r.String()
			g.Expect(result).To(Equal(tt.want), result)
		})
	}
}

func TestSimpleYAMLReporter_String(t *testing.T) {
	tests := []struct {
		name  string
		diffs []string
		want  string
	}{
		{name: "trims space", diffs: []string{" at start", "in\nbetween", "at end\n"}, want: `at start
in
between
at end`},
		{name: "joins with newline", diffs: []string{"a", "b", "c"}, want: `a
b
c`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &SimpleUnstructuredReporter{
				diffs: tt.diffs,
			}
			if got := r.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func yamlToUnstructured(str string) (*unstructured.Unstructured, error) {
	var obj map[string]any
	if err := yaml.Unmarshal([]byte(str), &obj); err != nil {
		return nil, err
	}
	return &unstructured.Unstructured{Object: obj}, nil
}
