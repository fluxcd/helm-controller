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

	"github.com/fluxcd/cli-utils/pkg/kstatus/polling/engine"
	"github.com/fluxcd/cli-utils/pkg/kstatus/polling/event"
	"github.com/fluxcd/cli-utils/pkg/kstatus/status"
	"github.com/fluxcd/cli-utils/pkg/object"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

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
