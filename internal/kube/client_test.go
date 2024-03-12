/*
Copyright 2023 The Flux authors

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

	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/pkg/runtime/client"
)

func TestWithNamespace(t *testing.T) {
	t.Run("sets the namespace", func(t *testing.T) {
		g := NewWithT(t)

		c := &MemoryRESTClientGetter{}
		WithNamespace("foo")(c)
		g.Expect(c.namespace).To(Equal("foo"))
	})
}

func TestWithImpersonate(t *testing.T) {
	t.Run("sets the impersonate", func(t *testing.T) {
		g := NewWithT(t)

		c := &MemoryRESTClientGetter{
			cfg: &rest.Config{
				Host: "https://example.com",
			},
		}
		WithImpersonate("foo", "bar")(c)
		g.Expect(c.impersonate).To(Equal(fmt.Sprintf(userNameFormat, "bar", "foo")))
		g.Expect(c.cfg.Impersonate.UserName).To(Equal(c.impersonate))
	})
}

func TestWithPersistent(t *testing.T) {
	t.Run("sets persistent flag", func(t *testing.T) {
		g := NewWithT(t)

		c := &MemoryRESTClientGetter{}
		WithPersistent(true)(c)
		g.Expect(c.persistent).To(BeTrue())

		WithPersistent(false)(c)
		g.Expect(c.persistent).To(BeFalse())
	})
}

func TestWithClientOptions(t *testing.T) {
	t.Run("sets the client options", func(t *testing.T) {
		g := NewWithT(t)

		c := &MemoryRESTClientGetter{
			cfg: &rest.Config{
				Host: "https://example.com",
			},
		}
		WithClientOptions(client.Options{
			Burst: 10,
			QPS:   5,
		})(c)
		g.Expect(c.cfg.Burst).To(Equal(10))
		g.Expect(c.cfg.QPS).To(Equal(float32(5)))
	})
}

func TestNewMemoryRESTClientGetter(t *testing.T) {
	t.Run("returns a new MemoryRESTClientGetter", func(t *testing.T) {
		g := NewWithT(t)

		c := NewMemoryRESTClientGetter(&rest.Config{
			Host: "https://example.com",
		})
		g.Expect(c).ToNot(BeNil())
		g.Expect(c.cfg).ToNot(BeNil())
		g.Expect(c.cfg.Host).To(Equal("https://example.com"))
	})

	t.Run("returns a new MemoryRESTClientGetter with default options", func(t *testing.T) {
		g := NewWithT(t)

		c := NewMemoryRESTClientGetter(&rest.Config{
			Host: "https://example.com",
		})
		g.Expect(c).ToNot(BeNil())
		g.Expect(c.cfg).ToNot(BeNil())
		g.Expect(c.namespace).To(Equal("default"))
	})

	t.Run("returns a new MemoryRESTClientGetter with options", func(t *testing.T) {
		g := NewWithT(t)

		c := NewMemoryRESTClientGetter(&rest.Config{
			Host: "https://example.com",
		}, WithNamespace("foo"))
		g.Expect(c).ToNot(BeNil())
		g.Expect(c.cfg).ToNot(BeNil())
		g.Expect(c.namespace).To(Equal("foo"))
	})
}

func TestNewInClusterMemoryRESTClientGetter(t *testing.T) {
	t.Cleanup(func() {
		cfg := ctrl.GetConfig
		ctrl.GetConfig = cfg
	})

	t.Run("discovers the in cluster config", func(t *testing.T) {
		g := NewWithT(t)

		mockCfg := &rest.Config{
			Host: "https://example.com",
		}
		ctrl.GetConfig = func() (*rest.Config, error) {
			return mockCfg, nil
		}

		c, err := NewInClusterMemoryRESTClientGetter()
		g.Expect(c).ToNot(BeNil())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.cfg).To(Equal(mockCfg))
	})

	t.Run("returns an error if the in cluster config cannot be discovered", func(t *testing.T) {
		g := NewWithT(t)

		ctrl.GetConfig = func() (*rest.Config, error) {
			return nil, fmt.Errorf("error")
		}

		c, err := NewInClusterMemoryRESTClientGetter()
		g.Expect(c).To(BeNil())
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("configures the client with options", func(t *testing.T) {
		g := NewWithT(t)

		mockCfg := &rest.Config{
			Host: "https://example.com",
		}
		ctrl.GetConfig = func() (*rest.Config, error) {
			return mockCfg, nil
		}

		c, err := NewInClusterMemoryRESTClientGetter(WithNamespace("foo"))
		g.Expect(c).ToNot(BeNil())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(c.cfg).To(Equal(mockCfg))
		g.Expect(c.namespace).To(Equal("foo"))
	})
}

func TestMemoryRESTClientGetter_ToRESTConfig(t *testing.T) {
	t.Run("returns a REST config", func(t *testing.T) {
		g := NewWithT(t)

		c := &MemoryRESTClientGetter{
			cfg: &rest.Config{
				Host: "https://example.com",
			},
		}
		cfg, err := c.ToRESTConfig()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(cfg).To(BeIdenticalTo(c.cfg))
	})

	t.Run("error on nil REST config", func(t *testing.T) {
		g := NewWithT(t)

		c := &MemoryRESTClientGetter{}
		cfg, err := c.ToRESTConfig()
		g.Expect(err).To(HaveOccurred())
		g.Expect(cfg).To(BeNil())
	})
}

func TestMemoryRESTClientGetter_ToDiscoveryClient(t *testing.T) {
	t.Run("returns a persistent discovery client", func(t *testing.T) {
		g := NewWithT(t)

		c := &MemoryRESTClientGetter{
			cfg: &rest.Config{
				Host: "https://example.com",
			},
			persistent: true,
		}
		dc, err := c.ToDiscoveryClient()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dc).ToNot(BeNil())

		// Calling it again should return the same instance.
		dc2, err := c.ToDiscoveryClient()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dc2).To(BeIdenticalTo(dc))
	})

	t.Run("returns a discovery client", func(t *testing.T) {
		g := NewWithT(t)

		c := &MemoryRESTClientGetter{
			cfg: &rest.Config{
				Host: "https://example.com",
			},
		}
		dc, err := c.ToDiscoveryClient()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dc).ToNot(BeNil())

		// Calling it again should return a new instance.
		dc2, err := c.ToDiscoveryClient()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(dc2).ToNot(BeIdenticalTo(dc))
	})
}

func TestMemoryRESTClientGetter_ToRESTMapper(t *testing.T) {
	t.Run("returns a persistent REST mapper", func(t *testing.T) {
		g := NewWithT(t)

		c := &MemoryRESTClientGetter{
			cfg: &rest.Config{
				Host: "https://example.com",
			},
			persistent: true,
		}
		rm, err := c.ToRESTMapper()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rm).ToNot(BeNil())

		// Calling it again should return the same instance.
		rm2, err := c.ToRESTMapper()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rm2).To(BeEquivalentTo(rm))
	})

	t.Run("returns a REST mapper", func(t *testing.T) {
		g := NewWithT(t)

		c := &MemoryRESTClientGetter{
			cfg: &rest.Config{
				Host: "https://example.com",
			},
		}
		rm, err := c.ToRESTMapper()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rm).ToNot(BeNil())

		// Calling it again should return a new instance.
		rm2, err := c.ToRESTMapper()
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(rm2).ToNot(BeEquivalentTo(rm))
	})
}

func TestMemoryRESTClientGetter_ToRawKubeConfigLoader(t *testing.T) {
	t.Run("returns a persistent client config", func(t *testing.T) {
		g := NewWithT(t)

		c := &MemoryRESTClientGetter{
			cfg: &rest.Config{
				Host: "https://example.com",
			},
			persistent: true,
		}
		cc := c.ToRawKubeConfigLoader()
		g.Expect(cc).ToNot(BeNil())

		// Calling it again should return the same instance.
		g.Expect(c.ToRawKubeConfigLoader()).To(BeIdenticalTo(cc))
	})

	t.Run("returns a client config", func(t *testing.T) {
		g := NewWithT(t)

		c := &MemoryRESTClientGetter{
			cfg: &rest.Config{
				Host: "https://example.com",
			},
		}
		cc := c.ToRawKubeConfigLoader()
		g.Expect(cc).ToNot(BeNil())

		// Calling it again should return the same instance.
		g.Expect(c.ToRawKubeConfigLoader()).ToNot(BeIdenticalTo(cc))
	})
}
