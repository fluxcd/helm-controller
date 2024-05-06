/*
Copyright 2023 The Flux authors

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

package reconcile

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	extjsondiff "github.com/wI2L/jsondiff"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	apierrutil "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/ssa"
	"github.com/fluxcd/pkg/ssa/jsondiff"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

func TestCorrectClusterDrift_Reconcile(t *testing.T) {
	mockStatus := v2.HelmReleaseStatus{
		History: v2.Snapshots{
			{
				Version:   2,
				Name:      mockReleaseName,
				Namespace: mockReleaseNamespace,
			},
		},
	}

	tests := []struct {
		name      string
		obj       *v2.HelmRelease
		diff      func(namespace string) jsondiff.DiffSet
		wantEvent bool
	}{
		{
			name: "corrects cluster drift",
			obj: &v2.HelmRelease{
				Spec: v2.HelmReleaseSpec{
					DriftDetection: &v2.DriftDetection{
						Mode: v2.DriftDetectionEnabled,
					},
				},
				Status: *mockStatus.DeepCopy(),
			},
			diff: func(namespace string) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type: jsondiff.DiffTypeCreate,
						DesiredObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "Secret",
								"metadata": map[string]interface{}{
									"name":      "secret",
									"namespace": namespace,
								},
							},
						},
					},
					{
						Type: jsondiff.DiffTypeUpdate,
						DesiredObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name":      "configmap",
									"namespace": namespace,
								},
								"data": map[string]interface{}{
									"key": "value",
								},
							},
						},
						ClusterObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name":      "configmap",
									"namespace": namespace,
								},
							},
						},
						Patch: extjsondiff.Patch{
							{
								Type: extjsondiff.OperationAdd,
								Path: "/data",
								Value: map[string]interface{}{
									"key": "value",
								},
							},
						},
					},
				}
			},
			wantEvent: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			namedNS, err := testEnv.CreateNamespace(context.TODO(), mockReleaseNamespace)
			g.Expect(err).NotTo(HaveOccurred())
			t.Cleanup(func() {
				_ = testEnv.Delete(context.TODO(), namedNS)
			})

			diff := tt.diff(namedNS.Name)
			for _, diff := range diff {
				if diff.ClusterObject != nil {
					obj := diff.ClusterObject.DeepCopyObject()
					g.Expect(testEnv.Create(context.TODO(), obj.(client.Object))).To(Succeed())
				}
			}

			getter, err := RESTClientGetterFromManager(testEnv.Manager, namedNS.Name)
			g.Expect(err).ToNot(HaveOccurred())

			cfg, err := action.NewConfigFactory(getter, action.WithStorage(action.DefaultStorageDriver, namedNS.Name))
			g.Expect(err).ToNot(HaveOccurred())

			recorder := testutil.NewFakeRecorder(10, false)

			r := NewCorrectClusterDrift(cfg, recorder, tt.diff(namedNS.Name), testFieldManager)
			g.Expect(r.Reconcile(context.TODO(), &Request{
				Object: tt.obj,
			})).ToNot(HaveOccurred())

			if tt.wantEvent {
				g.Expect(recorder.GetEvents()).To(HaveLen(1))
			} else {
				g.Expect(recorder.GetEvents()).To(BeEmpty())
			}

			g.Expect(tt.obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
				*conditions.UnknownCondition(meta.ReadyCondition, meta.ProgressingReason, "correcting cluster drift"),
			}))
		})
	}
}

func TestCorrectClusterDrift_report(t *testing.T) {
	mockObj := &v2.HelmRelease{
		Status: v2.HelmReleaseStatus{
			History: v2.Snapshots{
				{
					Version:   3,
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
				},
			},
		},
	}

	tests := []struct {
		name      string
		obj       *v2.HelmRelease
		changeSet *ssa.ChangeSet
		err       error
		wantEvent []corev1.Event
	}{
		{
			name: "with multiple changes",
			obj:  mockObj.DeepCopy(),
			changeSet: &ssa.ChangeSet{
				Entries: []ssa.ChangeSetEntry{
					{
						Subject: "Secret/namespace/name",
						Action:  ssa.CreatedAction,
					},
					{
						Subject: "Deployment/namespace/name",
						Action:  ssa.ConfiguredAction,
					},
				},
			},
			wantEvent: []corev1.Event{
				{
					Type:   corev1.EventTypeNormal,
					Reason: "DriftCorrected",
					Message: `Cluster state of release mock-ns/mock-release.v3 has been corrected:
Secret/namespace/name created
Deployment/namespace/name configured`,
				},
			},
		},
		{
			name: "with multiple changes and errors",
			obj:  mockObj.DeepCopy(),
			changeSet: &ssa.ChangeSet{
				Entries: []ssa.ChangeSetEntry{
					{
						Subject: "Secret/namespace/name",
						Action:  ssa.CreatedAction,
					},
					{
						Subject: "Deployment/namespace/name",
						Action:  ssa.ConfiguredAction,
					},
					{
						Subject: "ConfigMap/namespace/name",
						Action:  ssa.ConfiguredAction,
					},
				},
			},
			err: apierrutil.NewAggregate([]error{
				errors.New("error 1"),
				errors.New("error 2"),
			}),
			wantEvent: []corev1.Event{
				{
					Type:   corev1.EventTypeWarning,
					Reason: "DriftCorrectionFailed",
					Message: `Failed to partially correct cluster state of release mock-ns/mock-release.v3:
error 1
error 2

Successful corrections:
Secret/namespace/name created
Deployment/namespace/name configured
ConfigMap/namespace/name configured`,
				},
			},
		},
		{
			name: "with multiple errors",
			obj:  mockObj.DeepCopy(),
			err: apierrutil.NewAggregate([]error{
				errors.New("error 1"),
				errors.New("error 2"),
			}),
			wantEvent: []corev1.Event{
				{
					Type:   corev1.EventTypeWarning,
					Reason: "DriftCorrectionFailed",
					Message: `Failed to correct cluster state of release mock-ns/mock-release.v3:
error 1
error 2`,
				},
			},
		},
		{
			name: "with single change",
			obj:  mockObj.DeepCopy(),
			changeSet: &ssa.ChangeSet{
				Entries: []ssa.ChangeSetEntry{
					{
						Subject: "Secret/namespace/name",
						Action:  ssa.CreatedAction,
					},
				},
			},
			wantEvent: []corev1.Event{
				{
					Type:   corev1.EventTypeNormal,
					Reason: "DriftCorrected",
					Message: `Cluster state of release mock-ns/mock-release.v3 has been corrected:
Secret/namespace/name created`,
				},
			},
		},
		{
			name: "with single error",
			obj:  mockObj.DeepCopy(),
			err:  errors.New("error 1"),
			wantEvent: []corev1.Event{
				{
					Type:   corev1.EventTypeWarning,
					Reason: "DriftCorrectionFailed",
					Message: `Failed to correct cluster state of release mock-ns/mock-release.v3:
error 1`,
				},
			},
		},
		{
			name:      "empty change set",
			obj:       mockObj.DeepCopy(),
			wantEvent: []corev1.Event{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			recorder := testutil.NewFakeRecorder(10, false)
			r := &CorrectClusterDrift{
				eventRecorder: recorder,
			}

			r.report(tt.obj, tt.changeSet, tt.err)
			g.Expect(recorder.GetEvents()).To(ConsistOf(tt.wantEvent))
		})
	}
}
