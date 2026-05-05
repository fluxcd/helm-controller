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
	"fmt"
	"log/slog"
	"strings"
	"sync"

	helmaction "helm.sh/helm/v4/pkg/action"
	helmstorage "helm.sh/helm/v4/pkg/storage"
	helmdriver "helm.sh/helm/v4/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/fluxcd/pkg/ssa"

	"github.com/fluxcd/helm-controller/internal/storage"
)

// DefaultStorageDriver is the default Helm storage driver.
const DefaultStorageDriver = helmdriver.SecretsDriverName

// NormalizeStorageDriverName maps a user-supplied storage driver name to its
// canonical Helm constant. The accepted forms mirror the Helm CLI / SDK
// behaviour (see helm.sh/helm/v4/pkg/action.(*Configuration).Init), allowing
// both "secret"/"secrets" and "configmap"/"configmaps" interchangeably and
// matching case-insensitively. An empty name maps to DefaultStorageDriver.
// The boolean return is false when name is not a recognised driver.
func NormalizeStorageDriverName(name string) (string, bool) {
	switch strings.ToLower(name) {
	case "":
		return DefaultStorageDriver, true
	case strings.ToLower(helmdriver.SecretsDriverName), "secrets":
		return helmdriver.SecretsDriverName, true
	case strings.ToLower(helmdriver.ConfigMapsDriverName), "configmaps":
		return helmdriver.ConfigMapsDriverName, true
	case strings.ToLower(helmdriver.MemoryDriverName):
		return helmdriver.MemoryDriverName, true
	case strings.ToLower(helmdriver.SQLDriverName):
		return helmdriver.SQLDriverName, true
	}
	return "", false
}

// IsSupportedStorageDriver reports whether name is a recognised storage driver
// name. The accepted forms match those of the Helm CLI's HELM_DRIVER variable,
// matched case-insensitively. An empty name is treated as the default driver.
func IsSupportedStorageDriver(name string) bool {
	_, ok := NormalizeStorageDriverName(name)
	return ok
}

// SQLDriverPool returns Helm SQL storage drivers keyed by storage namespace,
// reusing the underlying database handle across reconciliations to avoid
// opening a fresh connection pool on every action.
//
// Helm v4 does not expose a Close method on storage.Driver, so the connection
// pools opened by helmdriver.NewSQL are released only when the controller
// process exits. The pool therefore caches drivers per storage namespace,
// bounding the resource usage to the set of namespaces actually in use.
type SQLDriverPool struct {
	connectionString string
	factory          func(connection, namespace string) (helmdriver.Driver, error)

	mu      sync.Mutex
	drivers map[string]helmdriver.Driver
}

// NewSQLDriverPool returns an SQLDriverPool that lazily opens a Helm SQL driver
// per namespace using the given connection string.
func NewSQLDriverPool(connectionString string) *SQLDriverPool {
	return &SQLDriverPool{
		connectionString: connectionString,
		factory: func(connection, namespace string) (helmdriver.Driver, error) {
			return helmdriver.NewSQL(connection, namespace)
		},
		drivers: make(map[string]helmdriver.Driver),
	}
}

// SetFactory replaces the driver constructor; intended for tests. It is not
// safe to call concurrently with DriverFor.
func (p *SQLDriverPool) SetFactory(f func(connection, namespace string) (helmdriver.Driver, error)) {
	p.factory = f
}

// DriverFor returns the cached SQL driver for the given storage namespace,
// constructing a new one on first use.
func (p *SQLDriverPool) DriverFor(namespace string) (helmdriver.Driver, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if d, ok := p.drivers[namespace]; ok {
		return d, nil
	}
	d, err := p.factory(p.connectionString, namespace)
	if err != nil {
		return nil, err
	}
	p.drivers[namespace] = d
	return d, nil
}

// ConfigFactory is a factory for the Helm action configuration of a (series
// of) Helm action(s). It allows for sharing Kubernetes client(s) and the
// Helm storage driver between actions, where possible.
//
// To get a Helm action.Configuration for an action, use the Build method on an
// initialized factory.
type ConfigFactory struct {
	// Getter is the RESTClientGetter used to get the RESTClient for the
	// Kubernetes API.
	Getter genericclioptions.RESTClientGetter
	// KubeClient is the (wrapped) Helm Kubernetes client, it is Helm-specific and
	// contains a factory used for lazy-loading.
	KubeClient *Client
	// Driver to use for the Helm action.
	Driver helmdriver.Driver
	// SQLDriverPool, when set, supplies SQL storage drivers per namespace and
	// is consulted by WithStorage when the SQL driver is requested.
	SQLDriverPool *SQLDriverPool
	// StorageLog is the logger to use for the Helm storage driver.
	StorageLog slog.Handler
	// NewResourceManager is the resource manager used to evaluate custom health checks.
	NewResourceManager func(sr ...NewStatusReaderFunc) *ssa.ResourceManager
	// WaitContext is the context used for waiting operations in the Helm
	// Kubernetes client.
	WaitContext context.Context
}

// ConfigFactoryOption is a function that configures a ConfigFactory.
type ConfigFactoryOption func(*ConfigFactory) error

// NewConfigFactory returns a new ConfigFactory configured with the provided
// options.
func NewConfigFactory(getter genericclioptions.RESTClientGetter, opts ...ConfigFactoryOption) (*ConfigFactory, error) {
	kubeClient := NewClient(getter)
	factory := &ConfigFactory{
		Getter:     getter,
		KubeClient: kubeClient,
	}
	for _, opt := range opts {
		if err := opt(factory); err != nil {
			return nil, err
		}
	}
	if err := factory.Valid(); err != nil {
		return nil, err
	}
	return factory, nil
}

// WithStorage configures the ConfigFactory.Driver by constructing a new Helm
// driver.Driver using the provided driver name and namespace.
//
// Driver names are matched case-insensitively and accept the same aliases as
// the Helm CLI: "secret"/"secrets", "configmap"/"configmaps", "memory", "sql".
// An empty driver falls back to DefaultStorageDriver.
//
// The SQL driver requires a SQLDriverPool to have been set on the
// ConfigFactory via WithSQLDriverPool. The Memory driver creates a fresh
// store on every call and is therefore only suitable for tests or
// short-lived processes.
//
// It returns an error when the driver name is not supported, the namespace
// is empty, or the underlying client/storage configuration fails.
func WithStorage(driver, namespace string) ConfigFactoryOption {
	canonical, ok := NormalizeStorageDriverName(driver)

	return func(f *ConfigFactory) error {
		if !ok {
			return fmt.Errorf("unsupported Helm storage driver '%s'", driver)
		}
		if namespace == "" {
			return fmt.Errorf("no namespace provided for Helm storage driver '%s'", canonical)
		}

		switch canonical {
		case helmdriver.SecretsDriverName, helmdriver.ConfigMapsDriverName:
			clientSet, err := f.KubeClient.Factory.KubernetesClientSet()
			if err != nil {
				return fmt.Errorf("could not get client set for '%s' storage driver: %w", canonical, err)
			}
			if canonical == helmdriver.ConfigMapsDriverName {
				f.Driver = helmdriver.NewConfigMaps(clientSet.CoreV1().ConfigMaps(namespace))
			} else {
				f.Driver = helmdriver.NewSecrets(clientSet.CoreV1().Secrets(namespace))
			}
		case helmdriver.MemoryDriverName:
			d := helmdriver.NewMemory()
			d.SetNamespace(namespace)
			f.Driver = d
		case helmdriver.SQLDriverName:
			if f.SQLDriverPool == nil {
				return fmt.Errorf("Helm storage driver '%s' requires a SQL driver pool", canonical)
			}
			sqlDriver, err := f.SQLDriverPool.DriverFor(namespace)
			if err != nil {
				// Underlying errors from helmdriver.NewSQL / sqlx.Connect can
				// echo the connection string; surface a generic message and
				// keep the detail off the HelmRelease status condition.
				return fmt.Errorf("could not initialize Helm SQL storage driver: connection failed")
			}
			f.Driver = sqlDriver
		}
		return nil
	}
}

// WithDriver sets the ConfigFactory.Driver.
func WithDriver(driver helmdriver.Driver) ConfigFactoryOption {
	return func(f *ConfigFactory) error {
		f.Driver = driver
		return nil
	}
}

// WithSQLDriverPool sets the ConfigFactory.SQLDriverPool. The pool is consulted
// by WithStorage when the SQL driver is requested.
func WithSQLDriverPool(pool *SQLDriverPool) ConfigFactoryOption {
	return func(f *ConfigFactory) error {
		f.SQLDriverPool = pool
		return nil
	}
}

// WithStorageLog sets the ConfigFactory.StorageLog.
func WithStorageLog(log slog.Handler) ConfigFactoryOption {
	return func(f *ConfigFactory) error {
		f.StorageLog = log
		return nil
	}
}

// WithResourceManager sets the ConfigFactory.ResourceManager.
func WithResourceManager(mgr func(sr ...NewStatusReaderFunc) *ssa.ResourceManager) ConfigFactoryOption {
	return func(f *ConfigFactory) error {
		f.NewResourceManager = mgr
		return nil
	}
}

// WithWaitContext sets the context used for waiting operations in the Helm
// Kubernetes client.
func WithWaitContext(ctx context.Context) ConfigFactoryOption {
	return func(f *ConfigFactory) error {
		f.WaitContext = ctx
		return nil
	}
}

// NewStorage returns a new Helm storage.Storage configured with any
// observer(s) and the Driver configured on the ConfigFactory.
func (c *ConfigFactory) NewStorage(observers ...storage.ObserveFunc) *helmstorage.Storage {
	driver := c.Driver
	if len(observers) > 0 {
		driver = storage.NewObserver(driver, observers...)
	}
	s := helmstorage.Init(driver)
	s.SetLogger(c.StorageLog)
	return s
}

// Build returns a new Helm action.Configuration configured with the receiver
// values, and the provided logger and observer(s).
func (c *ConfigFactory) Build(log slog.Handler, observers ...storage.ObserveFunc) *helmaction.Configuration {
	client := NewClient(c.Getter)
	client.newResourceManager = c.NewResourceManager
	client.waitContext = c.WaitContext

	var opts []helmaction.ConfigurationOption
	if log != nil {
		client.SetLogger(log)
		opts = append(opts, helmaction.ConfigurationSetLogger(log))
	}

	conf := helmaction.NewConfiguration(opts...)
	conf.RESTClientGetter = c.Getter
	conf.Releases = c.NewStorage(observers...)
	conf.KubeClient = client
	return conf
}

// Valid returns an error if the ConfigFactory is missing configuration
// required to run a Helm action.
func (c *ConfigFactory) Valid() error {
	switch {
	case c == nil:
		return fmt.Errorf("ConfigFactory is nil")
	case c.Driver == nil:
		return fmt.Errorf("no Helm storage driver configured")
	case c.KubeClient == nil, c.Getter == nil:
		return fmt.Errorf("no Kubernetes client and/or getter configured")
	}
	return nil
}
