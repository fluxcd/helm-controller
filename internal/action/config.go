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

	helmaction "github.com/matheuscscp/helm/pkg/action"
	helmkube "github.com/matheuscscp/helm/pkg/kube"
	helmstorage "github.com/matheuscscp/helm/pkg/storage"
	helmdriver "github.com/matheuscscp/helm/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/fluxcd/helm-controller/internal/storage"
)

const (
	// DefaultStorageDriver is the default Helm storage driver.
	DefaultStorageDriver = helmdriver.SecretsDriverName
)

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
	// Driver to use for the Helm action.
	Driver helmdriver.Driver
	// StorageLog is the logger to use for the Helm storage driver.
	StorageLog helmaction.DebugLog
	// WaitContext ...
	WaitContext context.Context
}

// ConfigFactoryOption is a function that configures a ConfigFactory.
type ConfigFactoryOption func(*ConfigFactory) error

// NewConfigFactory returns a new ConfigFactory configured with the provided
// options.
func NewConfigFactory(getter genericclioptions.RESTClientGetter, waitContext context.Context, opts ...ConfigFactoryOption) (*ConfigFactory, error) {
	factory := &ConfigFactory{
		Getter:      getter,
		WaitContext: waitContext,
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
// It supports driver.ConfigMapsDriverName, driver.SecretsDriverName and
// driver.MemoryDriverName.
// It returns an error when the driver name is not supported, or the client
// configuration for the storage fails.
func WithStorage(driver, namespace string) ConfigFactoryOption {
	if driver == "" {
		driver = DefaultStorageDriver
	}

	return func(f *ConfigFactory) error {
		if namespace == "" {
			return fmt.Errorf("no namespace provided for '%s' storage driver", driver)
		}

		switch driver {
		case helmdriver.SecretsDriverName, helmdriver.ConfigMapsDriverName, "":
			clientSet, err := helmkube.New(f.Getter).Factory.KubernetesClientSet()
			if err != nil {
				return fmt.Errorf("could not get client set for '%s' storage driver: %w", driver, err)
			}
			if driver == helmdriver.ConfigMapsDriverName {
				f.Driver = helmdriver.NewConfigMaps(clientSet.CoreV1().ConfigMaps(namespace))
			}
			if driver == helmdriver.SecretsDriverName {
				f.Driver = helmdriver.NewSecrets(clientSet.CoreV1().Secrets(namespace))
			}
		case helmdriver.MemoryDriverName:
			driver := helmdriver.NewMemory()
			driver.SetNamespace(namespace)
			f.Driver = driver
		default:
			return fmt.Errorf("unsupported Helm storage driver '%s'", driver)
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

// WithStorageLog sets the ConfigFactory.StorageLog.
func WithStorageLog(log helmaction.DebugLog) ConfigFactoryOption {
	return func(f *ConfigFactory) error {
		f.StorageLog = log
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
	if c.StorageLog != nil {
		s.Log = c.StorageLog
	}
	return s
}

// Build returns a new Helm action.Configuration configured with the receiver
// values, and the provided logger and observer(s).
func (c *ConfigFactory) Build(log helmaction.DebugLog, observers ...storage.ObserveFunc) *helmaction.Configuration {
	client := helmkube.New(c.Getter)
	client.WaitContext = c.WaitContext
	if log != nil {
		client.Log = log
	}

	return &helmaction.Configuration{
		RESTClientGetter: c.Getter,
		Releases:         c.NewStorage(observers...),
		KubeClient:       client,
		Log:              log,
	}
}

// Valid returns an error if the ConfigFactory is missing configuration
// required to run a Helm action.
func (c *ConfigFactory) Valid() error {
	switch {
	case c == nil:
		return fmt.Errorf("ConfigFactory is nil")
	case c.Driver == nil:
		return fmt.Errorf("no Helm storage driver configured")
	case c.Getter == nil:
		return fmt.Errorf("no getter configured")
	}
	return nil
}
