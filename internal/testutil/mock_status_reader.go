/*
Copyright 2026 The Flux authors

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

package testutil

import (
	"context"
	"sync/atomic"

	"github.com/fluxcd/cli-utils/pkg/kstatus/polling"
	"github.com/fluxcd/cli-utils/pkg/kstatus/polling/engine"
	"github.com/fluxcd/cli-utils/pkg/kstatus/polling/event"
	"github.com/fluxcd/cli-utils/pkg/kstatus/status"
	"github.com/fluxcd/cli-utils/pkg/object"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/ssa"
)

// newStatusReaderFunc matches the action.NewStatusReaderFunc type alias
// without importing the action package (to avoid import cycles).
type newStatusReaderFunc = func(apimeta.RESTMapper) engine.StatusReader

// MockStatusReader is a mock implementation of engine.StatusReader that
// returns a healthy status for all resources and tracks method calls.
type MockStatusReader struct {
	supportsCalled            atomic.Int32
	readStatusForObjectCalled atomic.Int32
}

var _ engine.StatusReader = (*MockStatusReader)(nil)

// Supports returns true for all GroupKinds and records the call.
func (m *MockStatusReader) Supports(gk schema.GroupKind) bool {
	m.supportsCalled.Add(1)
	return true
}

// ReadStatus returns a Current status for any resource.
func (m *MockStatusReader) ReadStatus(ctx context.Context, reader engine.ClusterReader, resource object.ObjMetadata) (*event.ResourceStatus, error) {
	return &event.ResourceStatus{
		Identifier: resource,
		Status:     status.CurrentStatus,
		Message:    "Resource is current",
	}, nil
}

// ReadStatusForObject returns a Current status for any object and records the call.
func (m *MockStatusReader) ReadStatusForObject(ctx context.Context, reader engine.ClusterReader, obj *unstructured.Unstructured) (*event.ResourceStatus, error) {
	m.readStatusForObjectCalled.Add(1)
	return &event.ResourceStatus{
		Identifier: object.UnstructuredToObjMetadata(obj),
		Status:     status.CurrentStatus,
		Resource:   obj,
		Message:    "Resource is current",
	}, nil
}

// SupportsCalled returns the number of times Supports was called.
func (m *MockStatusReader) SupportsCalled() int {
	return int(m.supportsCalled.Load())
}

// ReadStatusForObjectCalled returns the number of times ReadStatusForObject was called.
func (m *MockStatusReader) ReadStatusForObjectCalled() int {
	return int(m.readStatusForObjectCalled.Load())
}

// NewResourceManagerFunc returns a function compatible with
// action.WithResourceManager that uses this MockStatusReader as a custom
// status reader in the returned ResourceManager.
func (m *MockStatusReader) NewResourceManagerFunc() func(sr ...newStatusReaderFunc) *ssa.ResourceManager {
	return m.NewResourceManagerFuncWithClient(nil, nil)
}

// NewResourceManagerFuncWithClient returns a function compatible with
// action.WithResourceManager that uses this MockStatusReader with a real
// client and REST mapper for the poller engine.
func (m *MockStatusReader) NewResourceManagerFuncWithClient(c client.Client, mapper apimeta.RESTMapper) func(sr ...newStatusReaderFunc) *ssa.ResourceManager {
	return func(sr ...newStatusReaderFunc) *ssa.ResourceManager {
		readers := []engine.StatusReader{m}
		for _, f := range sr {
			readers = append(readers, f(mapper))
		}
		poller := polling.NewStatusPoller(c, mapper, polling.Options{
			CustomStatusReaders: readers,
		})
		return ssa.NewResourceManager(c, poller, ssa.Owner{})
	}
}

// NewMockResourceManagerFunc returns a function compatible with
// action.WithResourceManager using a new MockStatusReader.
func NewMockResourceManagerFunc() func(sr ...newStatusReaderFunc) *ssa.ResourceManager {
	return (&MockStatusReader{}).NewResourceManagerFunc()
}
