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
	"fmt"
	"testing"

	"github.com/fluxcd/pkg/runtime/client"
	. "github.com/onsi/gomega"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var cfg = []byte(`current-context: federal-context
apiVersion: v1
clusters:
- cluster:
    api-version: v1
    server: http://cow.org:8080
    insecure-skip-tls-verify: true
  name: cow-cluster
contexts:
- context:
    cluster: cow-cluster
    user: blue-user
  name: federal-context
kind: Config
users:
- name: blue-user
  user:
    token: foo`)

func TestNewInClusterRESTClientGetter(t *testing.T) {
	t.Run("discover config", func(t *testing.T) {
		g := NewWithT(t)

		cfg := &rest.Config{
			Host:        "https://example.com",
			BearerToken: "chase-the-honey",
			TLSClientConfig: rest.TLSClientConfig{
				CAFile: "afile",
			},
		}
		ctrl.GetConfig = func() (*rest.Config, error) {
			return cfg, nil
		}
		got, err := NewInClusterRESTClientGetter("", "", "", nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(BeAssignableToTypeOf(&genericclioptions.ConfigFlags{}))

		flags := got.(*genericclioptions.ConfigFlags)
		fields := map[*string]*string{
			flags.APIServer:   &cfg.Host,
			flags.BearerToken: &cfg.BearerToken,
			flags.CAFile:      &cfg.CAFile,
		}
		for f, ff := range fields {
			g.Expect(f).ToNot(BeNil())
			g.Expect(f).To(Equal(ff))
			g.Expect(f).ToNot(BeIdenticalTo(ff))
		}
	})

	t.Run("config retrieval error", func(t *testing.T) {
		g := NewWithT(t)

		ctrl.GetConfig = func() (*rest.Config, error) {
			return nil, fmt.Errorf("error")
		}
		got, err := NewInClusterRESTClientGetter("", "", "", nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("failed to get config for in-cluster REST client"))
		g.Expect(got).To(BeNil())
	})

	t.Run("namespace", func(t *testing.T) {
		g := NewWithT(t)

		ctrl.GetConfig = mockGetConfig
		namespace := "a-space"
		got, err := NewInClusterRESTClientGetter(namespace, "", "", nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(BeAssignableToTypeOf(&genericclioptions.ConfigFlags{}))

		flags := got.(*genericclioptions.ConfigFlags)
		g.Expect(flags.Namespace).ToNot(BeNil())
		g.Expect(*flags.Namespace).To(Equal(namespace))
	})

	t.Run("impersonation", func(t *testing.T) {
		g := NewWithT(t)

		ctrl.GetConfig = mockGetConfig
		ns := "a-namespace"
		accountName := "foo"
		accountNamespace := "another-namespace"
		got, err := NewInClusterRESTClientGetter(ns, accountName, accountNamespace, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).To(BeAssignableToTypeOf(&genericclioptions.ConfigFlags{}))

		flags := got.(*genericclioptions.ConfigFlags)
		g.Expect(flags.Impersonate).ToNot(BeNil())
		g.Expect(*flags.Impersonate).To(Equal(fmt.Sprintf("system:serviceaccount:%s:%s", accountNamespace, accountName)))
	})
}

func TestMemoryRESTClientGetter_ToRESTConfig(t *testing.T) {
	t.Run("loads REST config from KubeConfig", func(t *testing.T) {
		g := NewWithT(t)
		getter := NewMemoryRESTClientGetter(cfg, "", "", "", client.Options{}, client.KubeConfigOptions{})
		got, err := getter.ToRESTConfig()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got.Host).To(Equal("http://cow.org:8080"))
		g.Expect(got.TLSClientConfig.Insecure).To(BeFalse())
	})

	t.Run("sets ImpersonationConfig", func(t *testing.T) {
		g := NewWithT(t)
		getter := NewMemoryRESTClientGetter(cfg, "", "someone", "a-namespace", client.Options{}, client.KubeConfigOptions{})

		got, err := getter.ToRESTConfig()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got.Impersonate.UserName).To(Equal("system:serviceaccount:a-namespace:someone"))
	})

	t.Run("uses KubeConfigOptions", func(t *testing.T) {
		g := NewWithT(t)

		agent := "a static string forever," +
			"but static strings can have dreams and hope too"
		getter := NewMemoryRESTClientGetter(cfg, "", "someone", "", client.Options{QPS: 400, Burst: 800}, client.KubeConfigOptions{
			UserAgent: agent,
		})

		got, err := getter.ToRESTConfig()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got.UserAgent).To(Equal(agent))
	})

	t.Run("invalid config", func(t *testing.T) {
		g := NewWithT(t)

		getter := NewMemoryRESTClientGetter([]byte("invalid"), "", "", "", client.Options{QPS: 400, Burst: 800}, client.KubeConfigOptions{})
		got, err := getter.ToRESTConfig()
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(BeNil())
	})
}

func TestMemoryRESTClientGetter_ToDiscoveryClient(t *testing.T) {
	g := NewWithT(t)

	getter := NewMemoryRESTClientGetter(cfg, "", "", "", client.Options{QPS: 400, Burst: 800}, client.KubeConfigOptions{})
	got, err := getter.ToDiscoveryClient()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).ToNot(BeNil())
}

func TestMemoryRESTClientGetter_ToRESTMapper(t *testing.T) {
	g := NewWithT(t)

	getter := NewMemoryRESTClientGetter(cfg, "", "", "", client.Options{QPS: 400, Burst: 800}, client.KubeConfigOptions{})
	got, err := getter.ToRESTMapper()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(got).ToNot(BeNil())
}

func TestMemoryRESTClientGetter_ToRawKubeConfigLoader(t *testing.T) {
	g := NewWithT(t)

	getter := NewMemoryRESTClientGetter(cfg, "a-namespace", "impersonate", "other-namespace", client.Options{QPS: 400, Burst: 800}, client.KubeConfigOptions{})
	got := getter.ToRawKubeConfigLoader()
	g.Expect(got).ToNot(BeNil())

	cfg, err := got.ClientConfig()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(cfg.Impersonate.UserName).To(Equal("impersonate"))
	ns, _, err := got.Namespace()
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(ns).To(Equal("a-namespace"))
}
