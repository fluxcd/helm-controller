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

package controller_test

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
	"helm.sh/helm/v4/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	controllerLog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/fluxcd/pkg/apis/meta"
	rclient "github.com/fluxcd/pkg/runtime/client"
	helper "github.com/fluxcd/pkg/runtime/controller"
	"github.com/fluxcd/pkg/runtime/testenv"
	"github.com/fluxcd/pkg/testserver"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/controller"
	// +kubebuilder:scaffold:imports
)

var (
	controllerName = "helm-controller"
	testEnv        *testenv.Environment
	testServer     *testserver.HTTPServer
	k8sClient      client.Client
	reconciler     *controller.HelmReleaseReconciler
	kubeConfig     []byte

	testCtx = ctrl.SetupSignalHandler()
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz1234567890")

func randStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

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

	// Create a user for generating kubeconfig.
	user, err := testEnv.AddUser(envtest.User{
		Name:   "testenv-admin",
		Groups: []string{"system:masters"},
	}, nil)
	if err != nil {
		panic(fmt.Sprintf("Failed to create testenv-admin user: %v", err))
	}

	kubeConfig, err = user.KubeConfig()
	if err != nil {
		panic(fmt.Sprintf("Failed to create the testenv-admin user kubeconfig: %v", err))
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
	reconciler = &controller.HelmReleaseReconciler{
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
		DependencyRequeueInterval:  5 * time.Second,
		ArtifactFetchRetries:       1,
		DefaultServiceAccount:      "",
		DisableChartDigestTracking: false,
		AdditiveCELDependencyCheck: false,
		AllowExternalArtifact:      false,
	}

	watchConfigsPredicate, err := helper.GetWatchConfigsPredicate(helper.WatchOptions{})
	if err != nil {
		return err
	}

	return (reconciler).SetupWithManager(testCtx, testEnv, controller.HelmReleaseReconcilerOptions{
		WatchConfigsPredicate:      watchConfigsPredicate,
		WatchExternalArtifacts:     true,
		CancelHealthCheckOnRequeue: true,
	})
}

func createNamespace(name string) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	return k8sClient.Create(context.Background(), namespace)
}

func createKubeConfigSecret(namespace string) error {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "kubeconfig",
			Namespace: namespace,
		},
		Data: map[string][]byte{
			"value.yaml": kubeConfig,
		},
	}
	return k8sClient.Create(context.Background(), secret)
}

func applyHelmChart(objKey client.ObjectKey, artifact *meta.Artifact) error {
	chart := &sourcev1.HelmChart{
		TypeMeta: metav1.TypeMeta{
			Kind:       sourcev1.HelmChartKind,
			APIVersion: sourcev1.GroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      objKey.Name,
			Namespace: objKey.Namespace,
		},
		Spec: sourcev1.HelmChartSpec{
			Interval: metav1.Duration{Duration: time.Minute},
			Chart:    "test-chart",
			SourceRef: sourcev1.LocalHelmChartSourceReference{
				Kind: sourcev1.HelmRepositoryKind,
				Name: "test-repo",
			},
		},
	}

	status := sourcev1.HelmChartStatus{
		Conditions: []metav1.Condition{
			{
				Type:               meta.ReadyCondition,
				Status:             metav1.ConditionTrue,
				LastTransitionTime: metav1.Now(),
				Reason:             sourcev1.ChartPullSucceededReason,
			},
		},
		Artifact:           artifact,
		ObservedGeneration: 1,
	}

	patchOpts := []client.PatchOption{
		client.ForceOwnership,
		client.FieldOwner("helm-controller"),
	}

	if err := k8sClient.Patch(context.Background(), chart, client.Apply, patchOpts...); err != nil {
		return err
	}

	chart.ManagedFields = nil
	chart.Status = status

	statusOpts := &client.SubResourcePatchOptions{
		PatchOptions: client.PatchOptions{
			FieldManager: "source-controller",
		},
	}

	if err := k8sClient.Status().Patch(context.Background(), chart, client.Apply, statusOpts); err != nil {
		return err
	}
	return nil
}

func getEvents(objName string, annotations map[string]string) []corev1.Event {
	var result []corev1.Event
	events := &corev1.EventList{}
	_ = k8sClient.List(testCtx, events)
	for _, event := range events.Items {
		if event.InvolvedObject.Name == objName {
			if len(annotations) == 0 {
				result = append(result, event)
			} else {
				for ak, av := range annotations {
					if event.GetAnnotations()[ak] == av {
						result = append(result, event)
						break
					}
				}
			}
		}
	}
	return result
}
