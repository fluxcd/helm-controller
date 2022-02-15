//go:build gofuzz
// +build gofuzz

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
package controllers

import (
	"context"
	"sync"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
)

var (
	doOnce     sync.Once
	fuzzScheme = runtime.NewScheme()
)

// An init function that is invoked by way of sync.Do
func initFunc() {
	_ = corev1.AddToScheme(fuzzScheme)
	_ = v2.AddToScheme(fuzzScheme)
	_ = sourcev1.AddToScheme(fuzzScheme)
}

// FuzzHelmreleaseComposeValues implements a fuzzer
// that targets HelmReleaseReconciler.composeValues().
func FuzzHelmreleaseComposeValues(data []byte) int {
	doOnce.Do(initFunc)

	f := fuzz.NewConsumer(data)

	resources, err := getResources(f)
	if err != nil {
		return 0
	}

	c := fake.NewFakeClientWithScheme(fuzzScheme, resources...)
	r := &HelmReleaseReconciler{Client: c}

	hr := v2.HelmRelease{}
	err = f.GenerateStruct(&hr)
	if err != nil {
		return 0
	}

	_, _ = r.composeValues(logr.NewContext(context.TODO(), logr.Discard()), hr)

	return 1
}

// FuzzHelmreleaseComposeValues implements a fuzzer
// that targets HelmReleaseReconciler.reconcile().
func FuzzHelmreleaseReconcile(data []byte) int {
	doOnce.Do(initFunc)

	f := fuzz.NewConsumer(data)

	resources, err := getResources(f)
	if err != nil {
		return 0
	}

	hr := v2.HelmRelease{}
	err = f.GenerateStruct(&hr)
	if err != nil {
		return 0
	}

	hc := sourcev1.HelmChart{}
	err = f.GenerateStruct(&hc)
	if err != nil {
		return 0
	}

	hc.ObjectMeta.Name = hr.GetHelmChartName()
	hc.ObjectMeta.Namespace = hr.Spec.Chart.GetNamespace(hr.Namespace)
	resources = append(resources, &hc)

	c := fake.NewFakeClientWithScheme(fuzzScheme, resources...)
	r := &HelmReleaseReconciler{Client: c}

	_, _, _ = r.reconcile(logr.NewContext(context.TODO(), logr.Discard()), hr)

	return 1
}

func getResources(f *fuzz.ConsumeFuzzer) ([]runtime.Object, error) {
	resources := make([]runtime.Object, 0)

	name, err := f.GetString()
	if err != nil {
		return nil, err
	}

	if createSecret, _ := f.GetBool(); createSecret {
		inputByte := make(map[string][]byte)
		f.FuzzMap(&inputByte) // ignore error, as empty is still valid
		resources = append(resources,
			&corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Data:       inputByte,
			})
	}

	if createConfigMap, _ := f.GetBool(); createConfigMap {
		inputString := make(map[string]string)
		f.FuzzMap(&inputString) // ignore error, as empty is still valid
		resources = append(resources,
			&corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Name: name},
				Data:       inputString,
			})
	}

	return resources, nil
}
