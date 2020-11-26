/*
Copyright 2020 The Flux authors

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
	"sync"

	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

const ObserverDriverName = "observer"

// Observer observes the writes to the Helm storage driver it embeds
// and caches the persisted release.Release if its revision is higher
// or equal to the revision of the release.Release the cache holds
// at that moment.
//
// This allows for observations on persisted state that were the result
// of our own actions, and works around the inconsistent behavior of
// some of the Helm actions that may return an object that was not
// actually persisted to the Helm storage (e.g. because a validation
// error occurred during a Helm upgrade).
//
// NB: the implementation is simple, and while it does perform locking
// operations for cache writes, it was designed to only deal with a
// single Helm action at a time, including read operations on the
// cached release.
type Observer struct {
	sync.Mutex
	driver.Driver
	release *release.Release
}

// NewObserver creates a new observing Helm storage driver, the given
// release is the initial cached release.Release item and may be
// omitted.
func NewObserver(driver driver.Driver, rls *release.Release) *Observer {
	return &Observer{Driver: driver, release: rls}
}

// Name returns the name of the driver.
func (o *Observer) Name() string {
	return ObserverDriverName
}

// Get returns the release named by key or returns ErrReleaseNotFound.
func (o *Observer) Get(key string) (*release.Release, error) {
	return o.Driver.Get(key)
}

// List returns the list of all releases such that filter(release) == true
func (o *Observer) List(filter func(*release.Release) bool) ([]*release.Release, error) {
	return o.Driver.List(filter)
}

// Query returns the set of releases that match the provided set of labels
func (o *Observer) Query(keyvals map[string]string) ([]*release.Release, error) {
	return o.Driver.Query(keyvals)
}

// Create creates a new release or returns driver.ErrReleaseExists.
func (o *Observer) Create(key string, rls *release.Release) error {
	o.Lock()
	defer o.Unlock()
	if err := o.Driver.Create(key, rls); err != nil {
		return err
	}
	if hasNewerRevision(o.release, rls) {
		o.release = rls
	}
	return nil
}

// Update updates a release or returns driver.ErrReleaseNotFound.
func (o *Observer) Update(key string, rls *release.Release) error {
	o.Lock()
	defer o.Unlock()
	if err := o.Driver.Update(key, rls); err != nil {
		return err
	}
	if hasNewerRevision(o.release, rls) {
		o.release = rls
	}
	return nil
}

// Delete deletes a release or returns driver.ErrReleaseNotFound.
func (o *Observer) Delete(key string) (*release.Release, error) {
	return o.Driver.Delete(key)
}

// GetLastObservedRelease returns the last persisted release that was
// observed.
func (o *Observer) GetLastObservedRelease() *release.Release {
	return o.release
}

func hasNewerRevision(j, k *release.Release) bool {
	if j == nil {
		return true
	}
	return k.Version >= j.Version
}
