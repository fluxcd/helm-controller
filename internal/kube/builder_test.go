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
	"testing"

	"github.com/fluxcd/pkg/runtime/client"
	. "github.com/onsi/gomega"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

func TestBuildClientGetter(t *testing.T) {
	t.Run("with config and namespace", func(t *testing.T) {
		g := NewWithT(t)

		cfg := &rest.Config{
			BearerToken: "a-token",
		}
		namespace := "a-namespace"
		getter := BuildClientGetter(cfg, namespace)
		g.Expect(getter).To(BeAssignableToTypeOf(&genericclioptions.ConfigFlags{}))

		flags := getter.(*genericclioptions.ConfigFlags)
		g.Expect(flags.BearerToken).ToNot(BeNil())
		g.Expect(*flags.BearerToken).To(Equal(cfg.BearerToken))
		g.Expect(flags.Namespace).ToNot(BeNil())
		g.Expect(*flags.Namespace).To(Equal(namespace))
	})

	t.Run("with kubeconfig and impersonate", func(t *testing.T) {
		g := NewWithT(t)

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
		qps := float32(600)
		burst := 1000
		cfgOpts := client.KubeConfigOptions{InsecureTLS: true}

		impersonate := "jane"

		getter := BuildClientGetter(&rest.Config{}, namespace, WithKubeConfig(cfg, qps, burst, cfgOpts), WithImpersonate(impersonate))
		g.Expect(getter).To(BeAssignableToTypeOf(&MemoryRESTClientGetter{}))

		got := getter.(*MemoryRESTClientGetter)
		g.Expect(got.namespace).To(Equal(namespace))
		g.Expect(got.kubeConfig).To(Equal(cfg))
		g.Expect(got.qps).To(Equal(qps))
		g.Expect(got.burst).To(Equal(burst))
		g.Expect(got.kubeConfigOpts).To(Equal(cfgOpts))
		g.Expect(got.impersonateAccount).To(Equal(impersonate))
	})

	t.Run("with config and impersonate account", func(t *testing.T) {
		g := NewWithT(t)

		namespace := "a-namespace"
		impersonate := "frank"
		getter := BuildClientGetter(&rest.Config{}, namespace, WithImpersonate(impersonate))
		g.Expect(getter).To(BeAssignableToTypeOf(&genericclioptions.ConfigFlags{}))

		flags := getter.(*genericclioptions.ConfigFlags)
		g.Expect(flags.Namespace).ToNot(BeNil())
		g.Expect(*flags.Namespace).To(Equal(namespace))
		g.Expect(flags.Impersonate).ToNot(BeNil())
		g.Expect(*flags.Impersonate).To(Equal("system:serviceaccount:a-namespace:frank"))
	})

	t.Run("with config and DefaultServiceAccount", func(t *testing.T) {
		g := NewWithT(t)

		namespace := "a-namespace"
		DefaultServiceAccountName = "frank"
		getter := BuildClientGetter(&rest.Config{}, namespace)
		g.Expect(getter).To(BeAssignableToTypeOf(&genericclioptions.ConfigFlags{}))

		flags := getter.(*genericclioptions.ConfigFlags)
		g.Expect(flags.Namespace).ToNot(BeNil())
		g.Expect(*flags.Namespace).To(Equal(namespace))
		g.Expect(flags.Impersonate).ToNot(BeNil())
		g.Expect(*flags.Impersonate).To(Equal("system:serviceaccount:a-namespace:frank"))
	})
}
