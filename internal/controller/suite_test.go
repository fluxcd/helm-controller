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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/runtime/testenv"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
	// +kubebuilder:scaffold:imports
)

var (
	testScheme = runtime.NewScheme()

	testEnv *testenv.Environment

	testClient client.Client

	testCtx = ctrl.SetupSignalHandler()
)

func TestMain(m *testing.M) {
	utilruntime.Must(scheme.AddToScheme(testScheme))
	utilruntime.Must(sourcev1.AddToScheme(testScheme))
	utilruntime.Must(v2.AddToScheme(testScheme))

	testEnv = testenv.New(
		testenv.WithCRDPath(
			filepath.Join("..", "..", "build", "config", "crd", "bases"),
			filepath.Join("..", "..", "config", "crd", "bases"),
		),
		testenv.WithScheme(testScheme),
	)

	go func() {
		fmt.Println("Starting the test environment")
		if err := testEnv.Start(testCtx); err != nil {
			panic(fmt.Sprintf("Failed to start the test environment manager: %v", err))
		}
	}()
	<-testEnv.Manager.Elected()

	// Client with caching disabled.
	var err error
	testClient, err = client.New(testEnv.Config, client.Options{Scheme: testScheme})
	if err != nil {
		panic(fmt.Sprintf("Failed to create cacheless Kubernetes client: %v", err))
	}

	code := m.Run()

	fmt.Println("Stopping the test environment")
	if err := testEnv.Stop(); err != nil {
		panic(fmt.Sprintf("Failed to stop the test environment: %v", err))
	}

	os.Exit(code)
}
