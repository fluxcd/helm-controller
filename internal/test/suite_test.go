/*
Copyright 2025 The Flux authors

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

package test

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	rclient "github.com/fluxcd/pkg/runtime/client"
	helper "github.com/fluxcd/pkg/runtime/controller"
	"github.com/fluxcd/pkg/runtime/testenv"
	"github.com/fluxcd/pkg/testserver"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"go.uber.org/zap/zapcore"
	"helm.sh/helm/v3/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	controllerLog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/controller"
	// +kubebuilder:scaffold:imports
)

var (
	controllerName = "helm-controller"
	testEnv        *testenv.Environment
	testServer     *testserver.HTTPServer
	k8sClient      client.Client

	testCtx = ctrl.SetupSignalHandler()
)

func NewTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(apiextensionsv1.AddToScheme(s))
	utilruntime.Must(sourcev1.AddToScheme(s))
	utilruntime.Must(v2.AddToScheme(s))
	return s
}

func TestMain(m *testing.M) {
	// Set logs on development mode and debug level.
	controllerLog.SetLogger(zap.New(zap.WriteTo(os.Stderr), zap.UseDevMode(true), zap.Level(zapcore.DebugLevel)))

	// Set the managedFields owner for resources reconciled from Helm charts.
	kube.ManagedFieldsManager = controllerName

	// Initialize the test environment with source and helm controller CRDs.
	testEnv = testenv.New(
		testenv.WithCRDPath(
			filepath.Join("..", "..", "build", "config", "crd", "bases"),
			filepath.Join("..", "..", "config", "crd", "bases"),
		),
		testenv.WithScheme(NewTestScheme()),
	)

	// Start a local test HTTP server to serve chart and artifact files.
	var err error
	if testServer, err = testserver.NewTempHTTPServer(); err != nil {
		panic(fmt.Sprintf("Failed to create a temporary storage server: %v", err))
	}
	fmt.Println("Starting the test storage server")
	testServer.Start()

	// Start the test environment.
	go func() {
		fmt.Println("Starting the test environment")
		if err := testEnv.Start(testCtx); err != nil {
			panic(fmt.Sprintf("Failed to start the test environment manager: %v", err))
		}
	}()
	<-testEnv.Manager.Elected()

	// Client with caching disabled.
	k8sClient, err = client.New(testEnv.Config, client.Options{Scheme: NewTestScheme()})
	if err != nil {
		panic(fmt.Sprintf("Failed to create client: %v", err))
	}

	// Start the HelmRelease controller.
	if err := StartController(); err != nil {
		panic(fmt.Sprintf("Failed to start HelmRelease controller: %v", err))
	}

	code := m.Run()

	fmt.Println("Stopping the test environment")
	if err := testEnv.Stop(); err != nil {
		panic(fmt.Sprintf("Failed to stop the test environment: %v", err))
	}

	fmt.Println("Stopping the test storage server")
	testServer.Stop()
	if err := os.RemoveAll(testServer.Root()); err != nil {
		panic(fmt.Sprintf("Failed to remove storage server dir: %v", err))
	}

	os.Exit(code)
}

// GetTestClusterConfig returns a copy of the test cluster config.
func GetTestClusterConfig() (*rest.Config, error) {
	return rest.CopyConfig(testEnv.GetConfig()), nil
}

// StartController adds the HelmReleaseReconciler to the test controller manager
// and starts the reconciliation loops.
func StartController() error {
	timeout := time.Second * 10
	reconciler := &controller.HelmReleaseReconciler{
		Client:           testEnv,
		EventRecorder:    testEnv.GetEventRecorderFor(controllerName),
		Metrics:          helper.Metrics{},
		GetClusterConfig: GetTestClusterConfig,
		ClientOpts: rclient.Options{
			QPS:   50.0,
			Burst: 300,
		},
		KubeConfigOpts: rclient.KubeConfigOptions{
			InsecureExecProvider: false,
			InsecureTLS:          false,
			UserAgent:            controllerName,
			Timeout:              &timeout,
		},
		APIReader:                  testEnv,
		TokenCache:                 nil,
		FieldManager:               controllerName,
		DefaultServiceAccount:      "",
		DisableChartDigestTracking: false,
		AdditiveCELDependencyCheck: false,
	}

	watchConfigsPredicate, err := helper.GetWatchConfigsPredicate(helper.WatchOptions{})
	if err != nil {
		return err
	}

	return (reconciler).SetupWithManager(testCtx, testEnv, controller.HelmReleaseReconcilerOptions{
		WatchConfigsPredicate:     watchConfigsPredicate,
		DependencyRequeueInterval: 5 * time.Second,
		HTTPRetry:                 1,
	})
}
