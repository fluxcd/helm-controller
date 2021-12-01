/*
Copyright 2020 The Flux authors

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

package controllers

import (
	"context"
	"reflect"
	"testing"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/chartutil"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/yaml"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
)

func TestHelmReleaseReconciler_composeValues(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v2.AddToScheme(scheme)

	tests := []struct {
		name       string
		resources  []runtime.Object
		references []v2.ValuesReference
		values     string
		want       chartutil.Values
		wantErr    bool
	}{
		{
			name: "merges",
			resources: []runtime.Object{
				valuesConfigMap("values", map[string]string{
					"values.yaml": `flat: value
nested:
  configuration: value
`,
				}),
				valuesSecret("values", map[string][]byte{
					"values.yaml": []byte(`flat:
  nested: value
nested: value
`),
				}),
			},
			references: []v2.ValuesReference{
				{
					Kind: "ConfigMap",
					Name: "values",
				},
				{
					Kind: "Secret",
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
			name: "target path",
			resources: []runtime.Object{
				valuesSecret("values", map[string][]byte{"single": []byte("value")}),
			},
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
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
			name: "target path with boolean value",
			resources: []runtime.Object{
				valuesSecret("values", map[string][]byte{"single": []byte("true")}),
			},
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					ValuesKey:  "single",
					TargetPath: "merge.at.specific.path",
				},
			},
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
			name: "target path with set-string behavior",
			resources: []runtime.Object{
				valuesSecret("values", map[string][]byte{"single": []byte("\"true\"")}),
			},
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					ValuesKey:  "single",
					TargetPath: "merge.at.specific.path",
				},
			},
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
			name: "values reference to non existing secret",
			references: []v2.ValuesReference{
				{
					Kind: "Secret",
					Name: "missing",
				},
			},
			wantErr: true,
		},
		{
			name: "optional values reference to non existing secret",
			references: []v2.ValuesReference{
				{
					Kind:     "Secret",
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
					Kind: "ConfigMap",
					Name: "missing",
				},
			},
			wantErr: true,
		},
		{
			name: "optional values reference to non existing config map",
			references: []v2.ValuesReference{
				{
					Kind:     "ConfigMap",
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
				valuesSecret("values", nil),
			},
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: "nonexisting",
				},
			},
			wantErr: true,
		},
		{
			name: "missing config map key",
			resources: []runtime.Object{
				valuesConfigMap("values", nil),
			},
			references: []v2.ValuesReference{
				{
					Kind:      "ConfigMap",
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
				valuesConfigMap("values", map[string]string{
					"values.yaml": `
invalid`,
				}),
			},
			references: []v2.ValuesReference{
				{
					Kind: "ConfigMap",
					Name: "values",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := fake.NewFakeClientWithScheme(scheme, tt.resources...)
			r := &HelmReleaseReconciler{Client: c}
			var values *apiextensionsv1.JSON
			if tt.values != "" {
				v, _ := yaml.YAMLToJSON([]byte(tt.values))
				values = &apiextensionsv1.JSON{Raw: v}
			}
			hr := &v2.HelmRelease{
				Spec: v2.HelmReleaseSpec{
					ValuesFrom: tt.references,
					Values:     values,
				},
			}
			got, err := r.composeValues(logr.NewContext(context.TODO(), log.NullLogger{}), hr)
			if (err != nil) != tt.wantErr {
				t.Errorf("composeValues() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("composeValues() got = %v, want %v", got, tt.want)
			}
		})
	}
}

func valuesSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       data,
	}
}

func valuesConfigMap(name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       data,
	}
}
