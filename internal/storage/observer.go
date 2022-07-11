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

package storage

import (
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
)

// ObserverDriverName contains the string representation of Observer.
const ObserverDriverName = "observer"

// Observer is an observing Helm storage driver.
//
// It can be configured with a list of ObserveFunc functions that are called
// after a successful persistence operation to the underlying driver.
//
// This allows for observations on persisted state as performed by the driver,
// and works around the inconsistent behavior of some Helm actions that may
// return an object that was not actually persisted to the Helm storage
// (e.g. because a validation error occurred during a Helm upgrade).
type Observer struct {
	// driver holds the underlying driver.Driver implementation which is used
	// to persist data to, and retrieve from.
	driver helmdriver.Driver
	// observers holds a slice of ObserveFunc which are called after a
	// successful persistence of a release to storage driver.
	observers []ObserveFunc
}

// ObserveFunc observes a release which has been successfully persisted to
// storage.
// NOTE: while it takes a pointer, the caller is expected to perform a
// read-only operation.
type ObserveFunc func(rel *helmrelease.Release)

// NewObserver creates a new Observer for the given Helm storage driver.
func NewObserver(driver helmdriver.Driver, observers ...ObserveFunc) *Observer {
	return &Observer{
		driver:    driver,
		observers: observers,
	}
}

// Name returns the name of the driver.
func (o *Observer) Name() string {
	return ObserverDriverName
}

// Get returns the release named by key or returns ErrReleaseNotFound.
func (o *Observer) Get(key string) (*helmrelease.Release, error) {
	return o.driver.Get(key)
}

// List returns the list of all releases such that filter(release) == true.
func (o *Observer) List(filter func(*helmrelease.Release) bool) ([]*helmrelease.Release, error) {
	return o.driver.List(filter)
}

// Query returns the set of releases that match the provided set of labels.
func (o *Observer) Query(keyvals map[string]string) ([]*helmrelease.Release, error) {
	return o.driver.Query(keyvals)
}

// Create creates a new release or returns driver.ErrReleaseExists.
// It observes the release as provided after a successful creation.
func (o *Observer) Create(key string, rls *helmrelease.Release) error {
	if err := o.driver.Create(key, rls); err != nil {
		return err
	}
	for _, obs := range o.observers {
		obs(rls)
	}
	return nil
}

// Update updates a release or returns driver.ErrReleaseNotFound.
// After a successful update, it observes the release as provided.
func (o *Observer) Update(key string, rls *helmrelease.Release) error {
	if err := o.driver.Update(key, rls); err != nil {
		return err
	}
	for _, obs := range o.observers {
		obs(rls)
	}
	return nil
}

// Delete deletes a release or returns driver.ErrReleaseNotFound.
// After a successful deletion, it observes the release as returned by the
// embedded driver.Deletor.
func (o *Observer) Delete(key string) (*helmrelease.Release, error) {
	rls, err := o.driver.Delete(key)
	if err != nil {
		return nil, err
	}
	for _, obs := range o.observers {
		obs(rls)
	}
	return rls, nil
}
