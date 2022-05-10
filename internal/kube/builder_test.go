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
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"testing"

	"github.com/fluxcd/pkg/runtime/client"
	. "github.com/onsi/gomega"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestBuildClientGetter(t *testing.T) {
	t.Run("with namespace and retrieved config", func(t *testing.T) {
		g := NewWithT(t)
		cfg := &rest.Config{Host: "https://example.com"}
		ctrl.GetConfig = func() (*rest.Config, error) {
			return cfg, nil
		}

		namespace := "a-namespace"
		getter, err := BuildClientGetter(namespace)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(getter).To(BeAssignableToTypeOf(&genericclioptions.ConfigFlags{}))

		flags := getter.(*genericclioptions.ConfigFlags)
		g.Expect(flags.Namespace).ToNot(BeNil())
		g.Expect(*flags.Namespace).To(Equal(namespace))
		g.Expect(flags.APIServer).ToNot(BeNil())
		g.Expect(*flags.APIServer).To(Equal(cfg.Host))
	})

	t.Run("with kubeconfig, impersonate and client options", func(t *testing.T) {
		g := NewWithT(t)
		ctrl.GetConfig = mockGetConfig

		namespace := "a-namespace"
		cfg := []byte(`apiVersion: v1
clusters:
- cluster:
    server: https://example.com
  name: example-cluster
contexts:
- context:
    cluster: example-cluster
    namespace: flux-system
kind: Config
preferences: {}
users:`)
		clientOpts := client.Options{QPS: 600, Burst: 1000}
		cfgOpts := client.KubeConfigOptions{InsecureTLS: true}
		impersonate := "jane"

		getter, err := BuildClientGetter(namespace, WithClientOptions(clientOpts), WithKubeConfig(cfg, cfgOpts), WithImpersonate(impersonate))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(getter).To(BeAssignableToTypeOf(&MemoryRESTClientGetter{}))

		got := getter.(*MemoryRESTClientGetter)
		g.Expect(got.namespace).To(Equal(namespace))
		g.Expect(got.kubeConfig).To(Equal(cfg))
		g.Expect(got.clientOpts).To(Equal(clientOpts))
		g.Expect(got.kubeConfigOpts).To(Equal(cfgOpts))
		g.Expect(got.impersonateAccount).To(Equal(impersonate))
	})

	t.Run("with impersonate account", func(t *testing.T) {
		g := NewWithT(t)
		ctrl.GetConfig = mockGetConfig

		namespace := "a-namespace"
		impersonate := "frank"
		getter, err := BuildClientGetter(namespace, WithImpersonate(impersonate))
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(getter).To(BeAssignableToTypeOf(&genericclioptions.ConfigFlags{}))

		flags := getter.(*genericclioptions.ConfigFlags)
		g.Expect(flags.Namespace).ToNot(BeNil())
		g.Expect(*flags.Namespace).To(Equal(namespace))
		g.Expect(flags.Impersonate).ToNot(BeNil())
		g.Expect(*flags.Impersonate).To(Equal("system:serviceaccount:a-namespace:frank"))
	})

	t.Run("with DefaultServiceAccount", func(t *testing.T) {
		g := NewWithT(t)
		ctrl.GetConfig = mockGetConfig

		namespace := "a-namespace"
		DefaultServiceAccountName = "frank"
		getter, err := BuildClientGetter(namespace)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(getter).To(BeAssignableToTypeOf(&genericclioptions.ConfigFlags{}))

		flags := getter.(*genericclioptions.ConfigFlags)
		g.Expect(flags.Namespace).ToNot(BeNil())
		g.Expect(*flags.Namespace).To(Equal(namespace))
		g.Expect(flags.Impersonate).ToNot(BeNil())
		g.Expect(*flags.Impersonate).To(Equal("system:serviceaccount:a-namespace:frank"))
	})
}

func mockGetConfig() (*rest.Config, error) {
	return &rest.Config{}, nil
}
