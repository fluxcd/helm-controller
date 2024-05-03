/*
Copyright 2024 The Flux authors

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

package v2

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fluxcd/pkg/apis/meta"
)

func TestShouldHandleResetRequest(t *testing.T) {
	t.Run("should handle reset request", func(t *testing.T) {
		obj := &HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					meta.ReconcileRequestAnnotation: "b",
					ResetRequestAnnotation:          "b",
				},
			},
			Status: HelmReleaseStatus{
				LastHandledResetAt: "a",
				ReconcileRequestStatus: meta.ReconcileRequestStatus{
					LastHandledReconcileAt: "a",
				},
			},
		}

		if !ShouldHandleResetRequest(obj) {
			t.Error("ShouldHandleResetRequest() = false")
		}

		if obj.Status.LastHandledResetAt != "b" {
			t.Error("ShouldHandleResetRequest did not update LastHandledResetAt")
		}
	})
}

func TestShouldHandleForceRequest(t *testing.T) {
	t.Run("should handle force request", func(t *testing.T) {
		obj := &HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					meta.ReconcileRequestAnnotation: "b",
					ForceRequestAnnotation:          "b",
				},
			},
			Status: HelmReleaseStatus{
				LastHandledForceAt: "a",
				ReconcileRequestStatus: meta.ReconcileRequestStatus{
					LastHandledReconcileAt: "a",
				},
			},
		}

		if !ShouldHandleForceRequest(obj) {
			t.Error("ShouldHandleForceRequest() = false")
		}

		if obj.Status.LastHandledForceAt != "b" {
			t.Error("ShouldHandleForceRequest did not update LastHandledForceAt")
		}
	})
}

func Test_handleRequest(t *testing.T) {
	const requestAnnotation = "requestAnnotation"

	tests := []struct {
		name                     string
		annotations              map[string]string
		lastHandledReconcile     string
		lastHandledRequest       string
		want                     bool
		expectLastHandledRequest string
	}{
		{
			name: "valid request and reconcile annotations",
			annotations: map[string]string{
				meta.ReconcileRequestAnnotation: "b",
				requestAnnotation:               "b",
			},
			want:                     true,
			expectLastHandledRequest: "b",
		},
		{
			name: "mismatched annotations",
			annotations: map[string]string{
				meta.ReconcileRequestAnnotation: "b",
				requestAnnotation:               "c",
			},
			want:                     false,
			expectLastHandledRequest: "c",
		},
		{
			name: "reconcile matches previous request",
			annotations: map[string]string{
				meta.ReconcileRequestAnnotation: "b",
				requestAnnotation:               "b",
			},
			lastHandledReconcile:     "a",
			lastHandledRequest:       "b",
			want:                     false,
			expectLastHandledRequest: "b",
		},
		{
			name: "request matches previous reconcile",
			annotations: map[string]string{
				meta.ReconcileRequestAnnotation: "b",
				requestAnnotation:               "b",
			},
			lastHandledReconcile:     "b",
			lastHandledRequest:       "a",
			want:                     false,
			expectLastHandledRequest: "b",
		},
		{
			name:                     "missing annotations",
			annotations:              map[string]string{},
			lastHandledRequest:       "a",
			want:                     false,
			expectLastHandledRequest: "a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := &HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
				Status: HelmReleaseStatus{
					ReconcileRequestStatus: meta.ReconcileRequestStatus{
						LastHandledReconcileAt: tt.lastHandledReconcile,
					},
				},
			}

			lastHandled := tt.lastHandledRequest
			result := handleRequest(obj, requestAnnotation, &lastHandled)

			if result != tt.want {
				t.Errorf("handleRequest() = %v, want %v", result, tt.want)
			}
			if lastHandled != tt.expectLastHandledRequest {
				t.Errorf("lastHandledRequest = %v, want %v", lastHandled, tt.expectLastHandledRequest)
			}
		})
	}
}
