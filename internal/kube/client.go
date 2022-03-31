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

package kube

import (
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/fluxcd/pkg/runtime/client"
)

func NewInClusterRESTClientGetter(cfg *rest.Config, namespace string) genericclioptions.RESTClientGetter {
	flags := genericclioptions.NewConfigFlags(false)
	flags.APIServer = &cfg.Host
	flags.BearerToken = &cfg.BearerToken
	flags.CAFile = &cfg.CAFile
	flags.Namespace = &namespace
	flags.WithDiscoveryBurst(cfg.Burst)
	flags.WithDiscoveryQPS(cfg.QPS)
	if sa := cfg.Impersonate.UserName; sa != "" {
		flags.Impersonate = &sa
	}

	return flags
}

// MemoryRESTClientGetter is an implementation of the genericclioptions.RESTClientGetter,
// capable of working with an in-memory kubeconfig file.
type MemoryRESTClientGetter struct {
	kubeConfig         []byte
	namespace          string
	impersonateAccount string
	qps                float32
	burst              int
	kubeConfigOpts     client.KubeConfigOptions
}

func NewMemoryRESTClientGetter(
	kubeConfig []byte,
	namespace string,
	impersonateAccount string,
	qps float32,
	burst int,
	kubeConfigOpts client.KubeConfigOptions) genericclioptions.RESTClientGetter {
	return &MemoryRESTClientGetter{
		kubeConfig:         kubeConfig,
		namespace:          namespace,
		impersonateAccount: impersonateAccount,
		qps:                qps,
		burst:              burst,
		kubeConfigOpts:     kubeConfigOpts,
	}
}

func (c *MemoryRESTClientGetter) ToRESTConfig() (*rest.Config, error) {
	cfg, err := clientcmd.RESTConfigFromKubeConfig(c.kubeConfig)
	if err != nil {
		return nil, err
	}
	cfg = client.KubeConfig(cfg, c.kubeConfigOpts)
	if c.impersonateAccount != "" {
		cfg.Impersonate = rest.ImpersonationConfig{UserName: c.impersonateAccount}
	}
	return cfg, nil
}

func (c *MemoryRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := c.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	if c.impersonateAccount != "" {
		config.Impersonate = rest.ImpersonationConfig{UserName: c.impersonateAccount}
	}

	config.QPS = c.qps
	config.Burst = c.burst

	discoveryClient, _ := discovery.NewDiscoveryClientForConfig(config)
	return memory.NewMemCacheClient(discoveryClient), nil
}

func (c *MemoryRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := c.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	expander := restmapper.NewShortcutExpander(mapper, discoveryClient)
	return expander, nil
}

func (c *MemoryRESTClientGetter) ToRawKubeConfigLoader() clientcmd.ClientConfig {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	// use the standard defaults for this client command
	// DEPRECATED: remove and replace with something more accurate
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig

	overrides := &clientcmd.ConfigOverrides{ClusterDefaults: clientcmd.ClusterDefaults}
	overrides.Context.Namespace = c.namespace

	if c.impersonateAccount != "" {
		overrides.AuthInfo.Impersonate = c.impersonateAccount
	}

	return clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
}
