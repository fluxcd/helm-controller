//go:build gofuzz_libfuzzer
// +build gofuzz_libfuzzer

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
	"helm.sh/helm/v3/pkg/chartutil"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
)

func FuzzChartValuesFromReferences(f *testing.F) {
	scheme := testScheme()

	tests := []struct {
		targetPath   string
		valuesKey    string
		hrValues     string
		createObject bool
		secretData   []byte
		configData   string
	}{
		{
			targetPath: "flat",
			valuesKey:  "custom-values.yaml",
			secretData: []byte(`flat:
  nested: value
nested: value
`),
			configData: `flat: value
nested:
  configuration: value
`,
			hrValues: `
other: values
`,
			createObject: true,
		},
		{
			targetPath: "'flat'",
			valuesKey:  "custom-values.yaml",
			secretData: []byte(`flat:
  nested: value
nested: value
`),
			configData: `flat: value
nested:
  configuration: value
`,
			hrValues: `
other: values
`,
			createObject: true,
		},
		{
			targetPath: "flat[0]",
			secretData: []byte(``),
			configData: `flat: value`,
			hrValues: `
other: values
`,
			createObject: true,
		},
		{
			secretData: []byte(`flat:
  nested: value
nested: value
`),
			configData: `flat: value
nested:
  configuration: value
`,
			hrValues: `
other: values
`,
			createObject: true,
		},
		{
			targetPath: "some-value",
			hrValues: `
other: values
`,
			createObject: false,
		},
	}

	for _, tt := range tests {
		f.Add(tt.targetPath, tt.valuesKey, tt.hrValues, tt.createObject, tt.secretData, tt.configData)
	}

	f.Fuzz(func(t *testing.T,
		targetPath, valuesKey, hrValues string, createObject bool, secretData []byte, configData string) {

		// objectName and objectNamespace represent a name reference to a core
		// Kubernetes object upstream (Secret/ConfigMap) which is validated upstream,
		// and also validated by us in the OpenAPI-based validation set in
		// v2.ValuesReference. Therefore, a static value here suffices, and instead
		// we just play with the objects presence/absence.
		objectName := "values"
		objectNamespace := "default"
		var resources []runtime.Object

		if createObject {
			resources = append(resources,
				mockConfigMap(objectName, map[string]string{valuesKey: configData}),
				mockSecret(objectName, map[string][]byte{valuesKey: secretData}),
			)
		}

		references := []v2.ValuesReference{
			{
				Kind:       kindConfigMap,
				Name:       objectName,
				ValuesKey:  valuesKey,
				TargetPath: targetPath,
			},
			{
				Kind:       kindSecret,
				Name:       objectName,
				ValuesKey:  valuesKey,
				TargetPath: targetPath,
			},
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(resources...)
		var values chartutil.Values
		if hrValues != "" {
			values, _ = chartutil.ReadValues([]byte(hrValues))
		}

		_, _ = ChartValuesFromReferences(logr.NewContext(context.TODO(), logr.Discard()), c.Build(), objectNamespace, values, references...)
	})
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
