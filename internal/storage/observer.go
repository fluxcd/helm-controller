package storage

import (
	"sync"

	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

const ObserverDriverName = "observer"

// Observer is a wrapper around a Helm storage driver to observe the
// last release that has been persisted to storage.
type Observer struct {
	sync.Mutex
	driver.Driver
	newest *release.Release
}

func NewObserver(driver driver.Driver, rls *release.Release) *Observer {
	return &Observer{Driver: driver, newest: rls}
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

// Create creates a new release or returns ErrReleaseExists.
func (o *Observer) Create(key string, rls *release.Release) error {
	o.Lock()
	defer o.Unlock()
	err := o.Driver.Create(key, rls)
	if err == nil && isNewerRevision(o.newest, rls) {
		o.newest = rls
	}
	return err
}

// Update updates a release or returns ErrReleaseNotFound.
func (o *Observer) Update(key string, rls *release.Release) error {
	o.Lock()
	defer o.Unlock()
	err := o.Driver.Update(key, rls)
	if err == nil && isNewerRevision(o.newest, rls) {
		o.newest = rls
	}
	return err
}

// Delete deletes a release or returns ErrReleaseNotFound.
func (o *Observer) Delete(key string) (*release.Release, error) {
	return o.Driver.Delete(key)
}

// GetLastStoredRelease returns the newest stored release.
func (o *Observer) GetLastStoredRelease() *release.Release {
	return o.newest
}

func isNewerRevision(j, k *release.Release) bool {
	if j == nil {
		return true
	}
	return j.Version <= k.Version
}
