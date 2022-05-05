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
	"k8s.io/utils/pointer"

	"github.com/fluxcd/pkg/runtime/client"
)

// NewInClusterRESTClientGetter creates a new genericclioptions.RESTClientGetter
// using genericclioptions.NewConfigFlags, and configures it with the server,
// authentication, impersonation, and burst and QPS settings, and the provided
// namespace.
func NewInClusterRESTClientGetter(cfg *rest.Config, namespace string) genericclioptions.RESTClientGetter {
	flags := genericclioptions.NewConfigFlags(false)
	flags.APIServer = pointer.String(cfg.Host)
	flags.BearerToken = pointer.String(cfg.BearerToken)
	flags.CAFile = pointer.String(cfg.CAFile)
	flags.Namespace = pointer.String(namespace)
	flags.WithDiscoveryBurst(cfg.Burst)
	flags.WithDiscoveryQPS(cfg.QPS)
	if sa := cfg.Impersonate.UserName; sa != "" {
		flags.Impersonate = pointer.String(sa)
	}
	return flags
}

// MemoryRESTClientGetter is an implementation of the genericclioptions.RESTClientGetter,
// capable of working with an in-memory kubeconfig file.
type MemoryRESTClientGetter struct {
	// kubeConfig used to load a rest.Config, after being sanitized.
	kubeConfig []byte
	// kubeConfigOpts control the sanitization of the kubeConfig.
	kubeConfigOpts client.KubeConfigOptions
	// namespace specifies the namespace the client is configured to.
	namespace string
	// impersonateAccount configures the rest.ImpersonationConfig account name.
	impersonateAccount string
	// qps configures the QPS on the discovery.DiscoveryClient.
	qps float32
	// burst configures the burst on the discovery.DiscoveryClient.
	burst int
}

// NewMemoryRESTClientGetter returns a MemoryRESTClientGetter configured with
// the provided values and client.KubeConfigOptions. The provided KubeConfig is
// sanitized, configure the settings for this using client.KubeConfigOptions.
func NewMemoryRESTClientGetter(
	kubeConfig []byte,
	namespace string,
	impersonate string,
	qps float32,
	burst int,
	kubeConfigOpts client.KubeConfigOptions) genericclioptions.RESTClientGetter {
	return &MemoryRESTClientGetter{
		kubeConfig:         kubeConfig,
		namespace:          namespace,
		impersonateAccount: impersonate,
		qps:                qps,
		burst:              burst,
		kubeConfigOpts:     kubeConfigOpts,
	}
}

// ToRESTConfig creates a rest.Config with the rest.ImpersonationConfig configured
// with to the impersonation account. It loads the config the KubeConfig bytes and
// sanitizes it using the client.KubeConfigOptions.
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

// ToDiscoveryClient returns a discovery.CachedDiscoveryInterface configured
// with ToRESTConfig, and the QPS and Burst settings.
func (c *MemoryRESTClientGetter) ToDiscoveryClient() (discovery.CachedDiscoveryInterface, error) {
	config, err := c.ToRESTConfig()
	if err != nil {
		return nil, err
	}

	config.QPS = c.qps
	config.Burst = c.burst

	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return nil, err
	}
	return memory.NewMemCacheClient(discoveryClient), nil
}

// ToRESTMapper returns a RESTMapper constructed from ToDiscoveryClient.
func (c *MemoryRESTClientGetter) ToRESTMapper() (meta.RESTMapper, error) {
	discoveryClient, err := c.ToDiscoveryClient()
	if err != nil {
		return nil, err
	}

	mapper := restmapper.NewDeferredDiscoveryRESTMapper(discoveryClient)
	expander := restmapper.NewShortcutExpander(mapper, discoveryClient)
	return expander, nil
}

// ToRawKubeConfigLoader returns a clientcmd.ClientConfig using
// clientcmd.DefaultClientConfig. With clientcmd.ClusterDefaults, namespace, and
// impersonate configured as overwrites.
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
