//go:build gofuzz_libfuzzer
// +build gofuzz_libfuzzer

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

package controller

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
)

func FuzzHelmReleaseReconciler_reconcile(f *testing.F) {
	scheme := testScheme()
	tests := []struct {
		valuesKey  string
		hrValues   string
		secretData []byte
		configData string
	}{
		{
			valuesKey: "custom-values.yaml",
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
		},
	}

	for _, tt := range tests {
		f.Add(tt.valuesKey, tt.hrValues, tt.secretData, tt.configData)
	}

	f.Fuzz(func(t *testing.T,
		valuesKey, hrValues string, secretData []byte, configData string) {

		var values *apiextensionsv1.JSON
		if hrValues != "" {
			v, _ := yaml.YAMLToJSON([]byte(hrValues))
			values = &apiextensionsv1.JSON{Raw: v}
		}

		hr := v2.HelmRelease{
			Spec: v2.HelmReleaseSpec{
				Values: values,
			},
		}

		hc := sourcev1.HelmChart{}
		hc.ObjectMeta.Name = hr.GetHelmChartName()
		hc.ObjectMeta.Namespace = hr.Spec.Chart.GetNamespace(hr.Namespace)

		resources := []client.Object{
			valuesConfigMap("values", map[string]string{valuesKey: configData}),
			valuesSecret("values", map[string][]byte{valuesKey: secretData}),
			&hc,
		}

		c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(resources...).Build()
		r := &HelmReleaseReconciler{
			Client:        c,
			EventRecorder: &DummyRecorder{},
		}

		_, _, _ = r.reconcile(logr.NewContext(context.TODO(), logr.Discard()), hr)
	})
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

func testScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = v2.AddToScheme(scheme)
	_ = sourcev1.AddToScheme(scheme)
	return scheme
}

// DummyRecorder serves as a dummy for kuberecorder.EventRecorder.
type DummyRecorder struct{}

func (r *DummyRecorder) Event(object runtime.Object, eventtype, reason, message string) {
}

func (r *DummyRecorder) Eventf(object runtime.Object, eventtype, reason, messageFmt string, args ...interface{}) {
}

func (r *DummyRecorder) AnnotatedEventf(object runtime.Object, annotations map[string]string,
	eventtype, reason string, messageFmt string, args ...interface{}) {
}
