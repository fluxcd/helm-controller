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

package testutil

import (
	"fmt"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	_ "k8s.io/client-go/tools/record"
)

// FakeRecorder is used as a fake during tests.
//
// It was invented to be used in tests which require more precise control over
// e.g. assertions of specific event fields like Reason. For which string
// comparisons on the concentrated event message using record.FakeRecorder is
// not sufficient.
//
// To empty the Events channel into a slice of the recorded events, use
// GetEvents(). Not initializing Events will cause the recorder to not record
// any messages.
type FakeRecorder struct {
	Events        chan corev1.Event
	IncludeObject bool
}

// NewFakeRecorder creates new fake event recorder with an Events channel with
// the given size. Setting includeObject to true will cause the recorder to
// include the object reference in the events.
//
// To initialize a recorder which does not record any events, simply use:
//
//	recorder := new(FakeRecorder)
func NewFakeRecorder(bufferSize int, includeObject bool) *FakeRecorder {
	return &FakeRecorder{
		Events:        make(chan corev1.Event, bufferSize),
		IncludeObject: includeObject,
	}
}

// Event emits an event with the given message.
func (f *FakeRecorder) Event(obj runtime.Object, eventType, reason, message string) {
	f.Eventf(obj, eventType, reason, message)
}

// Eventf emits an event with the given message.
func (f *FakeRecorder) Eventf(obj runtime.Object, eventType, reason, message string, args ...any) {
	if f.Events != nil {
		f.Events <- f.generateEvent(obj, nil, eventType, reason, message, args...)
	}
}

// AnnotatedEventf emits an event with annotations.
func (f *FakeRecorder) AnnotatedEventf(obj runtime.Object, annotations map[string]string, eventType, reason, message string, args ...any) {
	if f.Events != nil {
		f.Events <- f.generateEvent(obj, annotations, eventType, reason, message, args...)
	}
}

// GetEvents empties the Events channel and returns a slice of recorded events.
// If the Events channel is nil, it returns nil.
func (f *FakeRecorder) GetEvents() (events []corev1.Event) {
	if f.Events != nil {
		for {
			select {
			case e := <-f.Events:
				events = append(events, e)
			default:
				return events
			}
		}
	}
	return nil
}

// generateEvent generates a new mocked event with the given parameters.
func (f *FakeRecorder) generateEvent(obj runtime.Object, annotations map[string]string, eventType, reason, message string, args ...any) corev1.Event {
	event := corev1.Event{
		InvolvedObject: objectReference(obj, f.IncludeObject),
		Type:           eventType,
		Reason:         reason,
		Message:        fmt.Sprintf(message, args...),
	}
	if annotations != nil {
		event.ObjectMeta.Annotations = annotations
	}
	return event
}

// objectReference returns an object reference for the given object with the
// kind and (group) API version set.
func objectReference(obj runtime.Object, includeObject bool) corev1.ObjectReference {
	if !includeObject {
		return corev1.ObjectReference{}
	}

	return corev1.ObjectReference{
		Kind:       obj.GetObjectKind().GroupVersionKind().Kind,
		APIVersion: obj.GetObjectKind().GroupVersionKind().GroupVersion().String(),
	}
}
