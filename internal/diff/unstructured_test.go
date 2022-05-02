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

package diff

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestWithoutStatus(t *testing.T) {
	g := NewWithT(t)

	u := unstructured.Unstructured{
		Object: map[string]interface{}{
			"status": "test",
		},
	}
	WithoutStatus()(&u)
	g.Expect(u.Object["status"]).To(BeNil())
}

func TestUnstructured(t *testing.T) {
	tests := []struct {
		name  string
		x     *unstructured.Unstructured
		y     *unstructured.Unstructured
		opts  []CompareOption
		want  string
		equal bool
	}{
		{
			name: "equal objects",
			x: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(4),
				},
				"status": map[string]interface{}{
					"readyReplicas": int64(4),
				},
			}},
			y: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(4),
				},
				"status": map[string]interface{}{
					"readyReplicas": int64(4),
				},
			}},
			want:  "",
			equal: true,
		},
		{
			name: "added simple value",
			x: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(1),
				},
				"status": map[string]interface{}{},
			}},
			y: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(1),
				},
				"status": map[string]interface{}{
					"readyReplicas": int64(1),
				},
			}},
			want: `.status.readyReplicas
+1`,
			equal: false,
		},
		{
			name: "removed simple value",
			x: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(1),
				},
				"status": map[string]interface{}{
					"readyReplicas": int64(4),
				},
			}},
			y: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{},
				"status": map[string]interface{}{
					"readyReplicas": int64(4),
				},
			}},
			want: `.spec.replicas
-1`,
			equal: false,
		},
		{
			name: "changed simple value",
			x: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(3),
				},
				"status": map[string]interface{}{
					"readyReplicas": int64(1),
				},
			}},
			y: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(3),
				},
				"status": map[string]interface{}{
					"readyReplicas": int64(3),
				},
			}},
			want: `.status.readyReplicas
-1
+3`,
			equal: false,
		},
		{
			name: "with options",
			opts: []CompareOption{WithoutStatus()},
			x: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(3),
				},
				"status": map[string]interface{}{
					"readyReplicas": int64(4),
				},
			}},
			y: &unstructured.Unstructured{Object: map[string]interface{}{
				"spec": map[string]interface{}{
					"replicas": int64(3),
				},
				"status": map[string]interface{}{
					"readyReplicas": int64(1),
				},
			}},
			equal: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, equal := Unstructured(tt.x, tt.y, tt.opts...)
			g.Expect(got).To(Equal(tt.want))
			g.Expect(equal).To(Equal(tt.equal))
		})
	}
}
