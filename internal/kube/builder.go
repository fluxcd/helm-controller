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

package kube

import (
	"github.com/fluxcd/pkg/runtime/client"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

const (
	// DefaultKubeConfigSecretKey is the default data key ConfigFromSecret
	// looks at when no data key is provided.
	DefaultKubeConfigSecretKey = "value"
	// DefaultKubeConfigSecretKeyExt is the default data key ConfigFromSecret
	// looks at when no data key is provided, and DefaultKubeConfigSecretKey
	// does not exist.
	DefaultKubeConfigSecretKeyExt = DefaultKubeConfigSecretKey + ".yaml"
)

// clientGetterOptions used to BuildClientGetter.
type clientGetterOptions struct {
	namespace          string
	kubeConfig         []byte
	impersonateAccount string
	clientOptions      client.Options
	kubeConfigOptions  client.KubeConfigOptions
}

// ClientGetterOption configures a genericclioptions.RESTClientGetter.
type ClientGetterOption func(o *clientGetterOptions)

// WithKubeConfig creates a MemoryRESTClientGetter configured with the provided
// KubeConfig and other values.
func WithKubeConfig(kubeConfig []byte, opts client.KubeConfigOptions) func(o *clientGetterOptions) {
	return func(o *clientGetterOptions) {
		o.kubeConfig = kubeConfig
		o.kubeConfigOptions = opts
	}
}

// WithClientOptions configures the genericclioptions.RESTClientGetter with
// provided options.
func WithClientOptions(opts client.Options) func(o *clientGetterOptions) {
	return func(o *clientGetterOptions) {
		o.clientOptions = opts
	}
}

// WithImpersonate configures the genericclioptions.RESTClientGetter to
// impersonate the provided account name.
func WithImpersonate(accountName string) func(o *clientGetterOptions) {
	return func(o *clientGetterOptions) {
		o.impersonateAccount = accountName
	}
}

// BuildClientGetter builds a genericclioptions.RESTClientGetter based on the
// provided options and returns the result. Namespace is not expected to be
// empty. In case it fails to construct using NewInClusterRESTClientGetter, it
// returns an error.
func BuildClientGetter(namespace string, opts ...ClientGetterOption) (genericclioptions.RESTClientGetter, error) {
	o := &clientGetterOptions{
		namespace: namespace,
	}
	for _, opt := range opts {
		opt(o)
	}
	if len(o.kubeConfig) > 0 {
		return NewMemoryRESTClientGetter(o.kubeConfig, namespace, o.impersonateAccount, o.clientOptions, o.kubeConfigOptions), nil
	}
	return NewInClusterRESTClientGetter(namespace, o.impersonateAccount, &o.clientOptions)
}
