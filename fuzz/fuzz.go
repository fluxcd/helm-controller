//go:build gofuzz
// +build gofuzz

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
	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sync"

	fuzz "github.com/AdaLogics/go-fuzz-headers"
)

var (
	initter sync.Once
	scheme  *runtime.Scheme
)

// An init function that is invoked by way of sync.Do
func initFunc() {
	scheme = runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	if err != nil {
		panic(err)
	}
	err = v2.AddToScheme(scheme)
	if err != nil {
		panic(err)
	}
}

// FuzzHelmreleaseComposeValues implements a fuzzer
// that targets HelmReleaseReconciler.composeValues()
func FuzzHelmreleaseComposeValues(data []byte) int {
	initter.Do(initFunc)

	f := fuzz.NewConsumer(data)

	hr := v2.HelmRelease{}
	err := f.GenerateStruct(&hr)
	if err != nil {
		return 0
	}

	r, err := createReconciler(f)
	if err != nil {
		return 0
	}

	_, _ = r.composeValues(logr.NewContext(context.TODO(), log.NullLogger{}), hr)
	return 1
}

// FuzzHelmreleasereconcile implements a fuzzer
// that targets HelmReleaseReconciler.reconcile()
func FuzzHelmreleasereconcile(data []byte) int {
	initter.Do(initFunc)

	f := fuzz.NewConsumer(data)

	hr := v2.HelmRelease{}
	err := f.GenerateStruct(&hr)
	if err != nil {
		return 0
	}

	r, err := createReconciler(f)
	if err != nil {
		return 0
	}

	_, _, _ = r.reconcile(logr.NewContext(context.TODO(), log.NullLogger{}), hr)
	return 1
}

// Allows the fuzzer to create a reconciler
func createReconciler(f *fuzz.ConsumeFuzzer) (*HelmReleaseReconciler, error) {
	// Get the type of object:
	var resources []runtime.Object
	r := &HelmReleaseReconciler{}
	getSecret, err := f.GetBool()
	if err != nil {
		return r, err
	}
	name, err := f.GetString()
	if err != nil {
		return r, err
	}
	if getSecret {
		inputMap := make(map[string][]byte)
		err = f.FuzzMap(&inputMap)
		if err != nil {
			return r, err
		}
		resources = []runtime.Object{
			valuesSecret(name, inputMap),
		}
	} else {
		inputMap := make(map[string]string)
		err = f.FuzzMap(&inputMap)
		if err != nil {
			return r, err
		}
		resources = []runtime.Object{
			valuesConfigMap(name, inputMap),
		}
	}
	c := fake.NewFakeClientWithScheme(scheme, resources...)
	r.Client = c
	return r, nil
}

// Taken from
// https://github.com/fluxcd/helm-controller/blob/main/controllers/helmrelease_controller_test.go#L282
func valuesSecret(name string, data map[string][]byte) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       data,
	}
}

// Taken from 
// https://github.com/fluxcd/helm-controller/blob/main/controllers/helmrelease_controller_test.go#L289
func valuesConfigMap(name string, data map[string]string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Data:       data,
	}
}
