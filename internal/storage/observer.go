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
	"crypto/sha256"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/mitchellh/copystructure"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	"helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
)

// ObserverDriverName contains the string representation of Observer.
const ObserverDriverName = "observer"

var (
	// ErrReleaseNotObserved indicates the release has not been observed by
	// the Observator.
	ErrReleaseNotObserved = errors.New("release: not observed")
)

// Observator reports about the write actions to a driver.Driver, recorded as
// ObservedRelease objects.
// Named to be inline with driver.Creator, driver.Updator, etc.
type Observator interface {
	// LastObservation returns the last observed release with the highest version,
	// or ErrReleaseNotObserved if there is no observed release with the provided
	// name.
	LastObservation(name string) (ObservedRelease, error)
	// GetObservedVersion returns the release with the given version if
	// observed, or ErrReleaseNotObserved.
	GetObservedVersion(name string, version int) (ObservedRelease, error)
	// ObserveLastRelease observes the release in with the highest version in
	// the embedded driver.Driver. It returns the driver.ErrReleaseNotFound is
	// returned if a release with the provided name does not exist.
	ObserveLastRelease(name string) (ObservedRelease, error)
}

// ObservedRelease is a copy of a release.Release as observed to be written to
// a Helm storage driver by an Observator. The object is detached from the Helm
// storage object, and mutations to it do not change the underlying release
// object.
type ObservedRelease struct {
	// Name of the release.
	Name string
	// Version of the release, at times also called revision.
	Version int
	// Info provides information about the release.
	Info release.Info
	// ChartMetadata contains the current Chartfile data of the release.
	ChartMetadata chart.Metadata
	// Config is the set of extra Values added to the chart.
	// These values override the default values inside the chart.
	Config map[string]interface{}
	// Manifest is the string representation of the rendered template.
	Manifest string
	// ManifestSHA256 is the string representation of the SHA256 sum of
	// Manifest.
	ManifestSHA256 string
	// Hooks are all the hooks declared for this release, and the current
	// state they are in.
	Hooks []release.Hook
	// Namespace is the Kubernetes namespace of the release.
	Namespace string
	// Labels of the release.
	Labels map[string]string
}

// DeepCopy deep copies the ObservedRelease, creating a new ObservedRelease.
func (in ObservedRelease) DeepCopy() ObservedRelease {
	out := ObservedRelease{}
	in.DeepCopyInto(&out)
	return out
}

// DeepCopyInto deep copies the ObservedRelease, writing it into out.
func (in ObservedRelease) DeepCopyInto(out *ObservedRelease) {
	if out == nil {
		return
	}

	out.Name = in.Name
	out.Version = in.Version
	out.Info = in.Info
	out.Manifest = in.Manifest
	out.ManifestSHA256 = in.ManifestSHA256
	out.Namespace = in.Namespace

	if v, err := copystructure.Copy(in.ChartMetadata); err == nil {
		out.ChartMetadata = v.(chart.Metadata)
	}

	if v, err := copystructure.Copy(in.Config); err == nil {
		out.Config = v.(map[string]interface{})
	}

	if len(in.Hooks) > 0 {
		out.Hooks = make([]release.Hook, len(in.Hooks))
		if v, err := copystructure.Copy(in.Hooks); err == nil {
			for i, h := range v.([]release.Hook) {
				out.Hooks[i] = h
			}
		}
	}

	if len(in.Labels) > 0 {
		out.Labels = make(map[string]string, len(in.Labels))
		for i, v := range in.Labels {
			out.Labels[i] = v
		}
	}
}

// NewObservedRelease deep copies the values from the provided release.Release
// into a new ObservedRelease while omitting all chart data except metadata.
func NewObservedRelease(rel *release.Release) ObservedRelease {
	if rel == nil {
		return ObservedRelease{}
	}

	obsRel := ObservedRelease{
		Name:      rel.Name,
		Version:   rel.Version,
		Config:    nil,
		Manifest:  rel.Manifest,
		Hooks:     nil,
		Namespace: rel.Namespace,
		Labels:    nil,
	}

	if rel.Info != nil {
		obsRel.Info = *rel.Info
	}

	if rel.Manifest != "" {
		obsRel.ManifestSHA256 = fmt.Sprintf("%x", sha256.Sum256([]byte(rel.Manifest)))
	}

	if rel.Chart != nil && rel.Chart.Metadata != nil {
		if v, err := copystructure.Copy(rel.Chart.Metadata); err == nil {
			obsRel.ChartMetadata = *v.(*chart.Metadata)
		}
	}

	if len(rel.Config) > 0 {
		if v, err := copystructure.Copy(rel.Config); err == nil {
			obsRel.Config = v.(map[string]interface{})
		}
	}

	if len(rel.Hooks) > 0 {
		obsRel.Hooks = make([]release.Hook, len(rel.Hooks))
		if v, err := copystructure.Copy(rel.Hooks); err == nil {
			for i, h := range v.([]*release.Hook) {
				obsRel.Hooks[i] = *h
			}
		}
	}

	if len(rel.Labels) > 0 {
		obsRel.Labels = make(map[string]string, len(rel.Labels))
		for i, v := range rel.Labels {
			obsRel.Labels[i] = v
		}
	}

	return obsRel
}

// Observer is a driver.Driver Observator.
//
// It observes the writes to the Helm storage driver it embeds, and caches
// persisted release.Release objects as an ObservedRelease by their Helm
// storage key.
//
// This allows for observations on persisted state as performed by the driver,
// and works around the inconsistent behavior of some Helm actions that may
// return an object that was not actually persisted to the Helm storage
// (e.g. because a validation error occurred during a Helm upgrade).
type Observer struct {
	// driver holds the underlying driver.Driver implementation which is used
	// to persist data to, and retrieve from.
	driver driver.Driver
	// releases contains a map of ObservedRelease objects indexed by makeKeyFunc
	// key.
	releases map[string]ObservedRelease
	// mu is a read-write lock for releases.
	mu sync.RWMutex
	// makeKeyFunc returns the expected Helm storage key for the given name and
	// version.
	// At present, the only implementation is makeKey, but to prevent
	// hard-coded assumptions and acknowledge the unexposed Helm API around it,
	// it can (theoretically) be configured.
	makeKeyFunc func(name string, version int) string
	// splitKeyFunc returns the name and version of a Helm storage key.
	// At present, the only implementation is splitKey, but to prevent
	// hard-coded assumptions and acknowledge the unexposed Helm API around it,
	// it can (theoretically) be configured.
	splitKeyFunc func(key string) (name string, version int)
}

// NewObserver creates a new observer for the given Helm storage driver.
func NewObserver(driver driver.Driver) *Observer {
	return &Observer{
		driver:       driver,
		makeKeyFunc:  makeKey,
		splitKeyFunc: splitKey,
		releases:     make(map[string]ObservedRelease),
	}
}

// Name returns the name of the driver.
func (o *Observer) Name() string {
	return ObserverDriverName
}

// Get returns the release named by key or returns ErrReleaseNotFound.
func (o *Observer) Get(key string) (*release.Release, error) {
	return o.driver.Get(key)
}

// List returns the list of all releases such that filter(release) == true.
func (o *Observer) List(filter func(*release.Release) bool) ([]*release.Release, error) {
	return o.driver.List(filter)
}

// Query returns the set of releases that match the provided set of labels.
func (o *Observer) Query(keyvals map[string]string) ([]*release.Release, error) {
	return o.driver.Query(keyvals)
}

// Create creates a new release or returns driver.ErrReleaseExists.
// It observes the release as provided after a successful creation.
func (o *Observer) Create(key string, rls *release.Release) error {
	defer unlock(o.wlock())
	if err := o.driver.Create(key, rls); err != nil {
		return err
	}
	o.releases[key] = NewObservedRelease(rls)
	return nil
}

// Update updates a release or returns driver.ErrReleaseNotFound.
// After a successful update, it observes the release as provided.
func (o *Observer) Update(key string, rls *release.Release) error {
	defer unlock(o.wlock())
	if err := o.driver.Update(key, rls); err != nil {
		return err
	}
	o.releases[key] = NewObservedRelease(rls)
	return nil
}

// Delete deletes a release or returns driver.ErrReleaseNotFound.
// After a successful deletion, it observes the release as returned by the
// embedded driver.Deletor.
func (o *Observer) Delete(key string) (*release.Release, error) {
	defer unlock(o.wlock())
	rel, err := o.driver.Delete(key)
	if err != nil {
		return nil, err
	}
	o.releases[key] = NewObservedRelease(rel)
	return rel, nil
}

// LastObservation returns the last observed release with the highest version,
// or ErrReleaseNotObserved if there is no observed release with the provided
// name.
func (o *Observer) LastObservation(name string) (ObservedRelease, error) {
	defer unlock(o.rlock())
	if len(o.releases) == 0 {
		return ObservedRelease{}, ErrReleaseNotObserved
	}
	var candidates []int
	for key := range o.releases {
		if n, ver := o.splitKeyFunc(key); n == name {
			candidates = append(candidates, ver)
		}
	}
	if len(candidates) == 0 {
		return ObservedRelease{}, ErrReleaseNotObserved
	}
	sort.Ints(candidates)
	return o.releases[o.makeKeyFunc(name, candidates[len(candidates)-1])].DeepCopy(), nil
}

// GetObservedVersion returns the observation for provided release name with
// the given version, or ErrReleaseNotObserved if it has not been observed.
func (o *Observer) GetObservedVersion(name string, version int) (ObservedRelease, error) {
	defer unlock(o.rlock())
	rls, ok := o.releases[o.makeKeyFunc(name, version)]
	if !ok {
		return ObservedRelease{}, ErrReleaseNotObserved
	}
	return rls.DeepCopy(), nil
}

// ObserveLastRelease observes the release with the highest version, or
// driver.ErrReleaseNotFound if a release with the provided name does not
// exist.
func (o *Observer) ObserveLastRelease(name string) (ObservedRelease, error) {
	defer unlock(o.wlock())
	rls, err := o.Query(map[string]string{"name": name, "owner": "helm"})
	if err != nil {
		return ObservedRelease{}, err
	}
	if len(rls) == 0 {
		return ObservedRelease{}, driver.ErrReleaseNotFound
	}
	releaseutil.Reverse(rls, releaseutil.SortByRevision)
	key := o.makeKeyFunc(rls[0].Name, rls[0].Version)
	o.releases[key] = NewObservedRelease(rls[0])
	return o.releases[key].DeepCopy(), nil
}

// wlock locks Observer for writing and returns a func to reverse the operation.
func (o *Observer) wlock() func() {
	o.mu.Lock()
	return func() { o.mu.Unlock() }
}

// rlock locks Observer for reading and returns a func to reverse the operation.
func (o *Observer) rlock() func() {
	o.mu.RLock()
	return func() { o.mu.RUnlock() }
}

// unlock calls fn which reverses an o.rlock or o.wlock. e.g:
// ```defer unlock(o.rlock())```, locks mem for reading at the
// call point of defer and unlocks upon exiting the block.
func unlock(fn func()) { fn() }

// makeKey mimics the Helm storage's internal makeKey method:
// https://github.com/helm/helm/blob/29d273f985306bc508b32455d77894f3b1eb8d4d/pkg/storage/storage.go#L251
func makeKey(name string, version int) string {
	return fmt.Sprintf("%s.%s.v%d", storage.HelmStorageType, name, version)
}

// splitKey is capable of splitting a Helm storage key into a name and version,
// if created using the makeKey logic.
func splitKey(key string) (name string, version int) {
	typeLessKey := strings.TrimPrefix(key, storage.HelmStorageType+".")
	split := strings.Split(typeLessKey, ".v")
	name = split[0]
	if len(split) > 1 {
		version, _ = strconv.Atoi(split[1])
	}
	return
}
