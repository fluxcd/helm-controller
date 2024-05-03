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

package reconcile

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"helm.sh/helm/v3/pkg/kube"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/fluxcd/pkg/runtime/testenv"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1beta2 "github.com/fluxcd/source-controller/api/v1beta2"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

const testFieldManager = "helm-controller"

var (
	ctx     = ctrl.SetupSignalHandler()
	testEnv *testenv.Environment
)

func NewTestScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(s))
	utilruntime.Must(apiextensionsv1.AddToScheme(s))
	utilruntime.Must(sourcev1.AddToScheme(s))
	utilruntime.Must(sourcev1beta2.AddToScheme(s))
	utilruntime.Must(v2.AddToScheme(s))
	return s
}

func TestMain(m *testing.M) {
	testEnv = testenv.New(
		testenv.WithCRDPath(
			filepath.Join("..", "..", "build", "config", "crd", "bases"),
			filepath.Join("..", "..", "config", "crd", "bases"),
		),
		testenv.WithScheme(NewTestScheme()),
	)

	go func() {
		fmt.Println("Starting the test environment")
		if err := testEnv.Start(ctx); err != nil {
			panic(fmt.Sprintf("Failed to start the test environment manager: %v", err))
		}
	}()
	<-testEnv.Manager.Elected()

	// Globally configure field manager for all tests.
	kube.ManagedFieldsManager = "reconciler-tests"

	code := m.Run()

	fmt.Println("Stopping the test environment")
	if err := testEnv.Stop(); err != nil {
		panic(fmt.Sprintf("Failed to stop the test environment: %v", err))
	}
	os.Exit(code)
}

type managerRESTClientGetter struct {
	restConfig      *rest.Config
	discoveryClient discovery.CachedDiscoveryInterface
	restMapper      meta.RESTMapper
	namespaceConfig clientcmd.ClientConfig
}

func RESTClientGetterFromManager(mgr manager.Manager, ns string) (genericclioptions.RESTClientGetter, error) {
	cfg := mgr.GetConfig()
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	cdc := memory.NewMemCacheClient(dc)
	rm := mgr.GetRESTMapper()
	return &managerRESTClientGetter{
		restConfig:      cfg,
		discoveryClient: cdc,
		restMapper:      rm,
		namespaceConfig: &namespaceClientConfig{ns},
	}, nil
}

func (c *managerRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	return c.restConfig, nil
}

func (c *managerRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	return c.discoveryClient, nil
}

func (c *managerRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	return c.restMapper, nil
}

func (c *managerRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	return c.namespaceConfig
}

var _ clientcmd.ClientConfig = &namespaceClientConfig{}

type namespaceClientConfig struct {
	namespace string
}

func (c namespaceClientConfig) RawConfig() (clientcmdapi.Config, error) {
	return clientcmdapi.Config{}, nil
}

func (c namespaceClientConfig) ClientConfig() (*rest.Config, error) {
	return nil, nil
}

func (c namespaceClientConfig) Namespace() (string, bool, error) {
	return c.namespace, false, nil
}

func (c namespaceClientConfig) ConfigAccess() clientcmd.ConfigAccess {
	return nil
}
