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
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

const (
	// FailingDriverName is the name of the failing driver.
	FailingDriverName = "failing"
)

// Failing is a failing Helm storage driver that returns the configured errors.
type Failing struct {
	driver.Driver

	// GetErr is returned by Get if configured. If not set, the embedded driver
	// result is returned.
	GetErr error
	// ListErr is returned by List if configured. If not set, the embedded
	// driver result is returned.
	ListErr error
	// QueryErr is returned by Query if configured. If not set, the embedded
	// driver result is returned.
	QueryErr error
	// CreateErr is returned by Create if configured. If not set, the embedded
	// driver result is returned.
	CreateErr error
	// UpdateErr is returned by Update if configured. If not set, the embedded
	// driver result is returned.
	UpdateErr error
	// DeleteErr is returned by Delete if configured. If not set, the embedded
	// driver result is returned.
	DeleteErr error
}

// Name returns the name of the driver.
func (o *Failing) Name() string {
	return FailingDriverName
}

// Get returns GetErr, or the embedded driver result.
func (o *Failing) Get(key string) (*release.Release, error) {
	if o.GetErr != nil {
		return nil, o.GetErr
	}
	return o.Driver.Get(key)
}

// List returns ListErr, or the embedded driver result.
func (o *Failing) List(filter func(*release.Release) bool) ([]*release.Release, error) {
	if o.ListErr != nil {
		return nil, o.ListErr
	}
	return o.Driver.List(filter)
}

// Query returns QueryErr, or the embedded driver result.
func (o *Failing) Query(keyvals map[string]string) ([]*release.Release, error) {
	if o.QueryErr != nil {
		return nil, o.QueryErr
	}
	return o.Driver.Query(keyvals)
}

// Create returns CreateErr, or the embedded driver result.
func (o *Failing) Create(key string, rls *release.Release) error {
	if o.CreateErr != nil {
		return o.CreateErr
	}
	return o.Driver.Create(key, rls)
}

// Update returns UpdateErr, or the embedded driver result.
func (o *Failing) Update(key string, rls *release.Release) error {
	if o.UpdateErr != nil {
		return o.UpdateErr
	}
	return o.Driver.Update(key, rls)
}

// Delete returns DeleteErr, or the embedded driver result.
func (o *Failing) Delete(key string) (*release.Release, error) {
	if o.DeleteErr != nil {
		return nil, o.DeleteErr
	}
	return o.Driver.Delete(key)
}
