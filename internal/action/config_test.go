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

package action

import (
	"context"
	"errors"
	"testing"

	helmaction "github.com/matheuscscp/helm/pkg/action"
	helmrelease "github.com/matheuscscp/helm/pkg/release"
	helmdriver "github.com/matheuscscp/helm/pkg/storage/driver"
	. "github.com/onsi/gomega"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdtest "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/fluxcd/helm-controller/internal/kube"
	"github.com/fluxcd/helm-controller/internal/storage"
)

func TestNewConfigFactory(t *testing.T) {
	tests := []struct {
		name    string
		getter  genericclioptions.RESTClientGetter
		opts    []ConfigFactoryOption
		wantErr error
	}{
		{
			name:   "constructs config factory",
			getter: &kube.MemoryRESTClientGetter{},
			opts: []ConfigFactoryOption{
				WithStorage(helmdriver.MemoryDriverName, "default"),
			},
			wantErr: nil,
		},
		{
			name:    "invalid config",
			getter:  &kube.MemoryRESTClientGetter{},
			wantErr: errors.New("no Helm storage driver configured"),
		},
		{
			name:   "multiple options",
			getter: &kube.MemoryRESTClientGetter{},
			opts: []ConfigFactoryOption{
				WithDriver(helmdriver.NewMemory()),
				WithStorageLog(func(format string, v ...interface{}) {
					// noop
				}),
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			factory, err := NewConfigFactory(tt.getter, context.Background(), tt.opts...)
			if tt.wantErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(factory).To(BeNil())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(factory).ToNot(BeNil())
		})
	}
}

func TestWithStorage(t *testing.T) {
	tests := []struct {
		name       string
		factory    ConfigFactory
		driverName string
		namespace  string
		wantErr    error
		wantDriver string
	}{
		{
			name:      "default_" + DefaultStorageDriver,
			namespace: "default",
			factory: ConfigFactory{
				Getter: cmdtest.NewTestFactory(),
			},
			wantDriver: helmdriver.SecretsDriverName,
		},
		{
			name:       helmdriver.SecretsDriverName,
			driverName: helmdriver.SecretsDriverName,
			namespace:  "default",
			factory: ConfigFactory{
				Getter: cmdtest.NewTestFactory(),
			},
			wantDriver: helmdriver.SecretsDriverName,
		},
		{
			name:       helmdriver.ConfigMapsDriverName,
			driverName: helmdriver.ConfigMapsDriverName,
			namespace:  "default",
			factory: ConfigFactory{
				Getter: cmdtest.NewTestFactory(),
			},
			wantDriver: helmdriver.ConfigMapsDriverName,
		},
		{
			name:       helmdriver.MemoryDriverName,
			driverName: helmdriver.MemoryDriverName,
			namespace:  "default",
			factory:    ConfigFactory{},
			wantDriver: helmdriver.MemoryDriverName,
		},
		{
			name:       "invalid namespace",
			driverName: helmdriver.SecretsDriverName,
			namespace:  "",
			factory:    ConfigFactory{},
			wantErr:    errors.New("no namespace provided for Helm storage driver 'secrets'"),
		},
		{
			name:       "invalid driver",
			driverName: "invalid",
			namespace:  "default",
			factory:    ConfigFactory{},
			wantErr:    errors.New("unsupported Helm storage driver 'invalid'"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			factory := tt.factory
			err := WithStorage(tt.driverName, tt.namespace)(&factory)
			if tt.wantErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(factory.Driver).To(BeNil())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(factory.Driver).ToNot(BeNil())
			g.Expect(factory.Driver.Name()).To(Equal(tt.wantDriver))
		})
	}
}

func TestWithDriver(t *testing.T) {
	g := NewWithT(t)

	factory := &ConfigFactory{}
	driver := helmdriver.NewMemory()
	g.Expect(WithDriver(driver)(factory)).NotTo(HaveOccurred())
	g.Expect(factory.Driver).To(Equal(driver))
}

func TestStorageLog(t *testing.T) {
	g := NewWithT(t)

	factory := &ConfigFactory{}
	log := helmaction.DebugLog(func(format string, v ...interface{}) {
		// noop
	})
	g.Expect(WithStorageLog(log)(factory)).NotTo(HaveOccurred())
	g.Expect(factory.StorageLog).ToNot(BeNil())
}

func TestConfigFactory_NewStorage(t *testing.T) {
	t.Run("without observers", func(t *testing.T) {
		g := NewWithT(t)

		factory := &ConfigFactory{
			Driver: helmdriver.NewMemory(),
		}

		s := factory.NewStorage()
		g.Expect(s).ToNot(BeNil())
		g.Expect(s.Driver).To(BeAssignableToTypeOf(factory.Driver))
	})

	t.Run("with observers", func(t *testing.T) {
		g := NewWithT(t)

		factory := &ConfigFactory{
			Driver: helmdriver.NewMemory(),
		}

		obsFunc := func(rel *helmrelease.Release) {}
		s := factory.NewStorage(obsFunc)
		g.Expect(s).ToNot(BeNil())
		g.Expect(s.Driver).To(BeAssignableToTypeOf(&storage.Observer{}))
	})

	t.Run("with storage log", func(t *testing.T) {
		g := NewWithT(t)

		var called bool
		log := func(fmt string, v ...interface{}) {
			called = true
		}

		factory := &ConfigFactory{
			Driver:     helmdriver.NewMemory(),
			StorageLog: log,
		}

		s := factory.NewStorage()
		g.Expect(s).ToNot(BeNil())
		s.Log("test")
		g.Expect(called).To(BeTrue())
	})
}

func TestConfigFactory_Build(t *testing.T) {
	t.Run("build", func(t *testing.T) {
		g := NewWithT(t)

		getter := &kube.MemoryRESTClientGetter{}
		factory := &ConfigFactory{
			Getter: getter,
		}

		cfg := factory.Build(nil)
		g.Expect(cfg).ToNot(BeNil())
		g.Expect(cfg.RESTClientGetter).To(Equal(factory.Getter))
	})

	t.Run("with log", func(t *testing.T) {
		g := NewWithT(t)

		var called bool
		log := func(fmt string, v ...interface{}) {
			called = true
		}
		cfg := (&ConfigFactory{}).Build(log)

		g.Expect(cfg).ToNot(BeNil())
		cfg.Log("")
		g.Expect(called).To(BeTrue())
	})

	t.Run("with observe func", func(t *testing.T) {
		g := NewWithT(t)

		factory := &ConfigFactory{
			Driver: helmdriver.NewMemory(),
		}

		obsFunc := func(rel *helmrelease.Release) {}
		cfg := factory.Build(nil, obsFunc)

		g.Expect(cfg).To(Not(BeNil()))
		g.Expect(cfg.Releases).ToNot(BeNil())
		g.Expect(cfg.Releases.Driver).To(BeAssignableToTypeOf(&storage.Observer{}))
	})
}

func TestConfigFactory_Valid(t *testing.T) {
	tests := []struct {
		name    string
		factory *ConfigFactory
		wantErr error
	}{
		{
			name: "valid",
			factory: &ConfigFactory{
				Driver: helmdriver.NewMemory(),
				Getter: &kube.MemoryRESTClientGetter{},
			},
			wantErr: nil,
		},
		{
			name: "no Kubernetes client",
			factory: &ConfigFactory{
				Driver: helmdriver.NewMemory(),
				Getter: &kube.MemoryRESTClientGetter{},
			},
			wantErr: errors.New("no Kubernetes client and/or getter configured"),
		},
		{
			name: "no Kubernetes getter",
			factory: &ConfigFactory{
				Driver: helmdriver.NewMemory(),
			},
			wantErr: errors.New("no Kubernetes client and/or getter configured"),
		},
		{
			name: "no driver",
			factory: &ConfigFactory{
				Getter: &kube.MemoryRESTClientGetter{},
			},
			wantErr: errors.New("no Helm storage driver configured"),
		},
		{
			name:    "nil factory",
			factory: nil,
			wantErr: errors.New("ConfigFactory is nil"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := tt.factory.Valid()
			if tt.wantErr == nil {
				g.Expect(err).To(BeNil())
				return
			}
			g.Expect(tt.factory.Valid()).To(Equal(tt.wantErr))
		})
	}
}
