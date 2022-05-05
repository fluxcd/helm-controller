/*
Copyright 2022 The Flux authors

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

package chartutil

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/chartutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
)

func TestChartValuesFromReferences(t *testing.T) {
	scheme := testScheme()

	tests := []struct {
		name       string
		resources  []runtime.Object
		namespace  string
		references []v2.ValuesReference
		values     string
		want       chartutil.Values
		wantErr    bool
	}{
		{
			name: "merges",
			resources: []runtime.Object{
				mockConfigMap("values", map[string]string{
					"values.yaml": `flat: value
nested:
  configuration: value
`,
				}),
				mockSecret("values", map[string][]byte{
					"values.yaml": []byte(`flat:
  nested: value
nested: value
`),
				}),
			},
			references: []v2.ValuesReference{
				{
					Kind: kindConfigMap,
					Name: "values",
				},
				{
					Kind: kindSecret,
					Name: "values",
				},
			},
			values: `
other: values
`,
			want: chartutil.Values{
				"flat": map[string]interface{}{
					"nested": "value",
				},
				"nested": "value",
				"other":  "values",
			},
		},
		{
			name: "with target path",
			resources: []runtime.Object{
				mockSecret("values", map[string][]byte{"single": []byte("value")}),
			},
			references: []v2.ValuesReference{
				{
					Kind:       kindSecret,
					Name:       "values",
					ValuesKey:  "single",
					TargetPath: "merge.at.specific.path",
				},
			},
			want: chartutil.Values{
				"merge": map[string]interface{}{
					"at": map[string]interface{}{
						"specific": map[string]interface{}{
							"path": "value",
						},
					},
				},
			},
		},
		{
			name: "target path for string type array item",
			resources: []runtime.Object{
				mockConfigMap("values", map[string]string{
					"values.yaml": `flat: value
nested:
  configuration:
  - list
  - item
  - option
`,
				}),
				mockSecret("values", map[string][]byte{
					"values.yaml": []byte(`foo`),
				}),
			},
			references: []v2.ValuesReference{
				{
					Kind: kindConfigMap,
					Name: "values",
				},
				{
					Kind:       kindSecret,
					Name:       "values",
					TargetPath: "nested.configuration[1]",
				},
			},
			values: `
other: values
`,
			want: chartutil.Values{
				"flat": "value",
				"nested": map[string]interface{}{
					"configuration": []interface{}{"list", "foo", "option"},
				},
				"other": "values",
			},
		},
		{
			name: "values reference to non existing secret",
			references: []v2.ValuesReference{
				{
					Kind: kindSecret,
					Name: "missing",
				},
			},
			wantErr: true,
		},
		{
			name: "optional values reference to non existing secret",
			references: []v2.ValuesReference{
				{
					Kind:     kindSecret,
					Name:     "missing",
					Optional: true,
				},
			},
			want:    chartutil.Values{},
			wantErr: false,
		},
		{
			name: "values reference to non existing config map",
			references: []v2.ValuesReference{
				{
					Kind: kindConfigMap,
					Name: "missing",
				},
			},
			wantErr: true,
		},
		{
			name: "optional values reference to non existing config map",
			references: []v2.ValuesReference{
				{
					Kind:     kindConfigMap,
					Name:     "missing",
					Optional: true,
				},
			},
			want:    chartutil.Values{},
			wantErr: false,
		},
		{
			name: "missing secret key",
			resources: []runtime.Object{
				mockSecret("values", nil),
			},
			references: []v2.ValuesReference{
				{
					Kind:      kindSecret,
					Name:      "values",
					ValuesKey: "nonexisting",
				},
			},
			wantErr: true,
		},
		{
			name: "missing config map key",
			resources: []runtime.Object{
				mockConfigMap("values", nil),
			},
			references: []v2.ValuesReference{
				{
					Kind:      kindConfigMap,
					Name:      "values",
					ValuesKey: "nonexisting",
				},
			},
			wantErr: true,
		},
		{
			name: "unsupported values reference kind",
			references: []v2.ValuesReference{
				{
					Kind: "Unsupported",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid values",
			resources: []runtime.Object{
				mockConfigMap("values", map[string]string{
					"values.yaml": `
invalid`,
				}),
			},
			references: []v2.ValuesReference{
				{
					Kind: kindConfigMap,
					Name: "values",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(tt.resources...)
			var values map[string]interface{}
			if tt.values != "" {
				m, err := chartutil.ReadValues([]byte(tt.values))
				g.Expect(err).ToNot(HaveOccurred())
				values = m
			}
			ctx := logr.NewContext(context.TODO(), logr.Discard())
			got, err := ChartValuesFromReferences(ctx, c.Build(), tt.namespace, values, tt.references...)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

// This tests compatability with the formats described in:
// https://helm.sh/docs/intro/using_helm/#the-format-and-limitations-of---set
func TestReplacePathValue(t *testing.T) {
	tests := []struct {
		name    string
		value   []byte
		path    string
		want    map[string]interface{}
		wantErr bool
	}{
		{
			name:  "outer inner",
			value: []byte("value"),
			path:  "outer.inner",
			want: map[string]interface{}{
				"outer": map[string]interface{}{
					"inner": "value",
				},
			},
		},
		{
			name:  "inline list",
			value: []byte("{a,b,c}"),
			path:  "name",
			want: map[string]interface{}{
				// TODO(hidde): figure out why the cap is off by len+1
				"name": append(make([]interface{}, 0, 4), []interface{}{"a", "b", "c"}...),
			},
		},
		{
			name:  "with escape",
			value: []byte(`value1\,value2`),
			path:  "name",
			want: map[string]interface{}{
				"name": "value1,value2",
			},
		},
		{
			name:  "target path with boolean value",
			value: []byte("true"),
			path:  "merge.at.specific.path",
			want: chartutil.Values{
				"merge": map[string]interface{}{
					"at": map[string]interface{}{
						"specific": map[string]interface{}{
							"path": true,
						},
					},
				},
			},
		},
		{
			name:  "target path with set-string behavior",
			value: []byte(`"true"`),
			path:  "merge.at.specific.path",
			want: chartutil.Values{
				"merge": map[string]interface{}{
					"at": map[string]interface{}{
						"specific": map[string]interface{}{
							"path": "true",
						},
					},
				},
			},
		},
		{
			name:  "target path with array item",
			value: []byte("value"),
			path:  "merge.at[2]",
			want: chartutil.Values{
				"merge": map[string]interface{}{
					"at": []interface{}{nil, nil, "value"},
				},
			},
		},
		{
			name:  "dot sequence escaping path",
			value: []byte("master"),
			path:  `nodeSelector.kubernetes\.io/role`,
			want: map[string]interface{}{
				"nodeSelector": map[string]interface{}{
					"kubernetes.io/role": "master",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			values := map[string]interface{}{}
			err := ReplacePathValue(values, tt.path, string(tt.value))
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(values).To(BeNil())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(values).To(Equal(tt.want))
		})
	}
}

func mockSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		TypeMeta: metav1.TypeMeta{
			Kind:       kindSecret,
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       data,
	}
}

func mockConfigMap(name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       kindConfigMap,
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       data,
	}
}

func testScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v2.AddToScheme(scheme)
	return scheme
}
