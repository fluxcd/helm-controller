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
	"errors"
	"log/slog"
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	helmkube "helm.sh/helm/v4/pkg/kube"
	helmrelease "helm.sh/helm/v4/pkg/release"
	helmdriver "helm.sh/helm/v4/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	cmdtest "k8s.io/kubectl/pkg/cmd/testing"

	"github.com/fluxcd/helm-controller/internal/kube"
	"github.com/fluxcd/helm-controller/internal/storage"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

// fakeSQLDriverPool returns a pool whose factory yields a memory-backed
// driver that reports its Name as SQL.
func fakeSQLDriverPool() *SQLDriverPool {
	pool := NewSQLDriverPool("postgres://user@example.com:5432/helm?sslmode=disable")
	pool.SetFactory(func(_ string, namespace string) (helmdriver.Driver, error) {
		d := helmdriver.NewMemory()
		d.SetNamespace(namespace)
		return fakeSQLDriver{Driver: d}, nil
	})
	return pool
}

type fakeSQLDriver struct {
	helmdriver.Driver
}

func (f fakeSQLDriver) Name() string {
	return helmdriver.SQLDriverName
}

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
				WithStorageLog(slog.DiscardHandler),
			},
			wantErr: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			factory, err := NewConfigFactory(tt.getter, tt.opts...)
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

func TestIsSupportedStorageDriver(t *testing.T) {
	g := NewWithT(t)

	g.Expect(IsSupportedStorageDriver("")).To(BeTrue())
	g.Expect(IsSupportedStorageDriver("secret")).To(BeTrue())
	g.Expect(IsSupportedStorageDriver("Secret")).To(BeTrue())
	g.Expect(IsSupportedStorageDriver("SECRET")).To(BeTrue())
	// Plural aliases match Helm CLI's HELM_DRIVER behaviour.
	g.Expect(IsSupportedStorageDriver("secrets")).To(BeTrue())
	g.Expect(IsSupportedStorageDriver("Secrets")).To(BeTrue())
	g.Expect(IsSupportedStorageDriver("configmap")).To(BeTrue())
	g.Expect(IsSupportedStorageDriver("ConfigMap")).To(BeTrue())
	g.Expect(IsSupportedStorageDriver("configmaps")).To(BeTrue())
	g.Expect(IsSupportedStorageDriver("memory")).To(BeTrue())
	g.Expect(IsSupportedStorageDriver("Memory")).To(BeTrue())
	g.Expect(IsSupportedStorageDriver("sql")).To(BeTrue())
	g.Expect(IsSupportedStorageDriver("SQL")).To(BeTrue())

	g.Expect(IsSupportedStorageDriver("postgres")).To(BeFalse())
	g.Expect(IsSupportedStorageDriver(" secret ")).To(BeFalse())
}

func TestSQLDriverPool(t *testing.T) {
	t.Run("caches one driver per namespace", func(t *testing.T) {
		g := NewWithT(t)

		var calls int
		pool := NewSQLDriverPool("ignored")
		pool.SetFactory(func(_ string, ns string) (helmdriver.Driver, error) {
			calls++
			d := helmdriver.NewMemory()
			d.SetNamespace(ns)
			return fakeSQLDriver{Driver: d}, nil
		})

		first, err := pool.DriverFor("foo")
		g.Expect(err).ToNot(HaveOccurred())
		again, err := pool.DriverFor("foo")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(again).To(BeIdenticalTo(first))
		g.Expect(calls).To(Equal(1))

		other, err := pool.DriverFor("bar")
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(other).ToNot(BeIdenticalTo(first))
		g.Expect(calls).To(Equal(2))
	})

	t.Run("propagates factory errors", func(t *testing.T) {
		g := NewWithT(t)

		pool := NewSQLDriverPool("ignored")
		pool.SetFactory(func(_ string, _ string) (helmdriver.Driver, error) {
			return nil, errors.New("boom")
		})

		d, err := pool.DriverFor("foo")
		g.Expect(err).To(MatchError("boom"))
		g.Expect(d).To(BeNil())
	})
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
				KubeClient: &Client{Client: helmkube.New(cmdtest.NewTestFactory())},
			},
			wantDriver: helmdriver.SecretsDriverName,
		},
		{
			name:       helmdriver.SecretsDriverName,
			driverName: helmdriver.SecretsDriverName,
			namespace:  "default",
			factory: ConfigFactory{
				KubeClient: &Client{Client: helmkube.New(cmdtest.NewTestFactory())},
			},
			wantDriver: helmdriver.SecretsDriverName,
		},
		{
			name:       "lowercase secret",
			driverName: strings.ToLower(helmdriver.SecretsDriverName),
			namespace:  "default",
			factory: ConfigFactory{
				KubeClient: &Client{Client: helmkube.New(cmdtest.NewTestFactory())},
			},
			wantDriver: helmdriver.SecretsDriverName,
		},
		{
			name:       "secrets alias",
			driverName: "secrets",
			namespace:  "default",
			factory: ConfigFactory{
				KubeClient: &Client{Client: helmkube.New(cmdtest.NewTestFactory())},
			},
			wantDriver: helmdriver.SecretsDriverName,
		},
		{
			name:       helmdriver.ConfigMapsDriverName,
			driverName: helmdriver.ConfigMapsDriverName,
			namespace:  "default",
			factory: ConfigFactory{
				KubeClient: &Client{Client: helmkube.New(cmdtest.NewTestFactory())},
			},
			wantDriver: helmdriver.ConfigMapsDriverName,
		},
		{
			name:       "lowercase configmap",
			driverName: strings.ToLower(helmdriver.ConfigMapsDriverName),
			namespace:  "default",
			factory: ConfigFactory{
				KubeClient: &Client{Client: helmkube.New(cmdtest.NewTestFactory())},
			},
			wantDriver: helmdriver.ConfigMapsDriverName,
		},
		{
			name:       "configmaps alias",
			driverName: "configmaps",
			namespace:  "default",
			factory: ConfigFactory{
				KubeClient: &Client{Client: helmkube.New(cmdtest.NewTestFactory())},
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
			name:       "uppercase sql",
			driverName: helmdriver.SQLDriverName,
			namespace:  "default",
			factory: ConfigFactory{
				SQLDriverPool: fakeSQLDriverPool(),
			},
			wantDriver: helmdriver.SQLDriverName,
		},
		{
			name:       "lowercase sql",
			driverName: strings.ToLower(helmdriver.SQLDriverName),
			namespace:  "default",
			factory: ConfigFactory{
				SQLDriverPool: fakeSQLDriverPool(),
			},
			wantDriver: helmdriver.SQLDriverName,
		},
		{
			name:       "invalid namespace",
			driverName: helmdriver.SecretsDriverName,
			namespace:  "",
			factory:    ConfigFactory{},
			wantErr:    errors.New("no namespace provided for Helm storage driver '" + helmdriver.SecretsDriverName + "'"),
		},
		{
			name:       "invalid namespace via alias",
			driverName: "secrets",
			namespace:  "",
			factory:    ConfigFactory{},
			// Error message is normalised to the canonical Helm constant.
			wantErr: errors.New("no namespace provided for Helm storage driver '" + helmdriver.SecretsDriverName + "'"),
		},
		{
			name:       "sql with empty namespace",
			driverName: helmdriver.SQLDriverName,
			namespace:  "",
			factory: ConfigFactory{
				SQLDriverPool: fakeSQLDriverPool(),
			},
			wantErr: errors.New("no namespace provided for Helm storage driver 'SQL'"),
		},
		{
			name:       "invalid driver",
			driverName: "invalid",
			namespace:  "default",
			factory:    ConfigFactory{},
			wantErr:    errors.New("unsupported Helm storage driver 'invalid'"),
		},
		{
			name:       "sql without pool",
			driverName: helmdriver.SQLDriverName,
			namespace:  "default",
			factory:    ConfigFactory{},
			wantErr:    errors.New("Helm storage driver 'SQL' requires a SQL driver pool"),
		},
		{
			name:       "sql with failing pool",
			driverName: helmdriver.SQLDriverName,
			namespace:  "default",
			factory: ConfigFactory{
				SQLDriverPool: func() *SQLDriverPool {
					p := NewSQLDriverPool("postgres://user:secret@host:5432/db")
					p.SetFactory(func(_ string, _ string) (helmdriver.Driver, error) {
						return nil, errors.New("dial error: postgres://user:secret@host:5432/db: refused")
					})
					return p
				}(),
			},
			// Underlying error must NOT be surfaced (it can echo the DSN).
			wantErr: errors.New("could not initialize Helm SQL storage driver: connection failed"),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			factory := tt.factory
			err := WithStorage(tt.driverName, tt.namespace)(&factory)
			if tt.wantErr != nil {
				g.Expect(err).To(MatchError(tt.wantErr.Error()))
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

func TestWithSQLDriverPool(t *testing.T) {
	g := NewWithT(t)

	factory := &ConfigFactory{}
	pool := NewSQLDriverPool("ignored")
	g.Expect(WithSQLDriverPool(pool)(factory)).NotTo(HaveOccurred())
	g.Expect(factory.SQLDriverPool).To(BeIdenticalTo(pool))
}

func TestStorageLog(t *testing.T) {
	g := NewWithT(t)

	factory := &ConfigFactory{}
	g.Expect(WithStorageLog(slog.DiscardHandler)(factory)).NotTo(HaveOccurred())
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

		obsFunc := func(rel helmrelease.Releaser) {}
		s := factory.NewStorage(obsFunc)
		g.Expect(s).ToNot(BeNil())
		g.Expect(s.Driver).To(BeAssignableToTypeOf(&storage.Observer{}))
	})

	t.Run("with storage log", func(t *testing.T) {
		g := NewWithT(t)

		log := &testutil.MockSLogHandler{}

		factory := &ConfigFactory{
			Driver:     helmdriver.NewMemory(),
			StorageLog: log,
		}

		s := factory.NewStorage()
		g.Expect(s).ToNot(BeNil())
		s.Logger().Info("test log")
		g.Expect(log.Called).To(BeTrue())
	})
}

func TestConfigFactory_Build(t *testing.T) {
	t.Run("build", func(t *testing.T) {
		g := NewWithT(t)

		getter := &kube.MemoryRESTClientGetter{}
		factory := &ConfigFactory{
			Getter:     getter,
			KubeClient: &Client{Client: helmkube.New(getter)},
		}

		cfg := factory.Build(nil)
		g.Expect(cfg).ToNot(BeNil())
		g.Expect(cfg.KubeClient).To(BeAssignableToTypeOf(&Client{}))
		g.Expect(cfg.RESTClientGetter).To(Equal(factory.Getter))
	})

	t.Run("with log", func(t *testing.T) {
		g := NewWithT(t)

		log := &testutil.MockSLogHandler{}
		cfg := (&ConfigFactory{}).Build(log)

		g.Expect(cfg).ToNot(BeNil())
		cfg.Logger().Info("test log")
		g.Expect(log.Called).To(BeTrue())
	})

	t.Run("with observe func", func(t *testing.T) {
		g := NewWithT(t)

		factory := &ConfigFactory{
			Driver: helmdriver.NewMemory(),
		}

		obsFunc := func(rel helmrelease.Releaser) {}
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
				Driver:     helmdriver.NewMemory(),
				Getter:     &kube.MemoryRESTClientGetter{},
				KubeClient: &Client{Client: helmkube.New(&kube.MemoryRESTClientGetter{})},
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
				Driver:     helmdriver.NewMemory(),
				KubeClient: &Client{Client: helmkube.New(&kube.MemoryRESTClientGetter{})},
			},
			wantErr: errors.New("no Kubernetes client and/or getter configured"),
		},
		{
			name: "no driver",
			factory: &ConfigFactory{
				KubeClient: &Client{Client: helmkube.New(&kube.MemoryRESTClientGetter{})},
				Getter:     &kube.MemoryRESTClientGetter{},
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
