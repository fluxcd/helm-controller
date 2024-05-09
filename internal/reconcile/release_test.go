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

package reconcile

import (
	"fmt"
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/chart"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fluxcd/pkg/apis/kustomize"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/action"
)

const (
	mockReleaseName      = "mock-release"
	mockReleaseNamespace = "mock-ns"
)

var (
	postRenderers = []v2.PostRenderer{
		{
			Kustomize: &v2.Kustomize{
				Patches: []kustomize.Patch{
					{
						Target: &kustomize.Selector{
							Kind: "Deployment",
							Name: "test",
						},
						Patch: `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
spec:
  replicas: 2
`,
					},
				},
			},
		},
	}

	postRenderers2 = []v2.PostRenderer{
		{
			Kustomize: &v2.Kustomize{
				Patches: []kustomize.Patch{
					{
						Target: &kustomize.Selector{
							Kind: "Deployment",
							Name: "test",
						},
						Patch: `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test
spec:
  replicas: 3
`,
					},
				},
			},
		},
	}
)

func Test_summarize(t *testing.T) {
	tests := []struct {
		name           string
		generation     int64
		spec           *v2.HelmReleaseSpec
		status         v2.HelmReleaseStatus
		expectedStatus *v2.HelmReleaseStatus
	}{
		{
			name:       "summarize conditions",
			generation: 1,
			status: v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.InstallSucceededReason,
						Message:            "Install complete",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.TestSuccessCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.TestFailedReason,
						Message:            "test hook(s) failure",
						ObservedGeneration: 1,
					},
				},
			},
			expectedStatus: &v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               meta.ReadyCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.InstallSucceededReason,
						Message:            "Install complete",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.InstallSucceededReason,
						Message:            "Install complete",
						ObservedGeneration: 1,
					},
				},
			},
		},
		{
			name:       "with tests enabled",
			generation: 1,
			status: v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.InstallSucceededReason,
						Message:            "Install complete",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.TestSuccessCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.TestSucceededReason,
						Message:            "test hook(s) succeeded",
						ObservedGeneration: 1,
					},
				},
			},
			spec: &v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			expectedStatus: &v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               meta.ReadyCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.TestSucceededReason,
						Message:            "test hook(s) succeeded",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.InstallSucceededReason,
						Message:            "Install complete",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.TestSuccessCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.TestSucceededReason,
						Message:            "test hook(s) succeeded",
						ObservedGeneration: 1,
					},
				},
			},
		},
		{
			name:       "with tests enabled and failure tests",
			generation: 1,
			status: v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.InstallSucceededReason,
						Message:            "Install complete",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.TestSuccessCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.TestFailedReason,
						Message:            "test hook(s) failure",
						ObservedGeneration: 1,
					},
				},
			},
			spec: &v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			expectedStatus: &v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               meta.ReadyCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.TestFailedReason,
						Message:            "test hook(s) failure",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.InstallSucceededReason,
						Message:            "Install complete",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.TestSuccessCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.TestFailedReason,
						Message:            "test hook(s) failure",
						ObservedGeneration: 1,
					},
				},
			},
		},
		{
			name: "with test hooks enabled and pending tests",
			status: v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.InstallSucceededReason,
						Message:            "Install complete",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.TestSuccessCondition,
						Status:             metav1.ConditionUnknown,
						Reason:             "AwaitingTests",
						Message:            "Release is awaiting tests",
						ObservedGeneration: 1,
					},
				},
			},
			spec: &v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			expectedStatus: &v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               meta.ReadyCondition,
						Status:             metav1.ConditionUnknown,
						Reason:             "AwaitingTests",
						Message:            "Release is awaiting tests",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.InstallSucceededReason,
						Message:            "Install complete",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.TestSuccessCondition,
						Status:             metav1.ConditionUnknown,
						Reason:             "AwaitingTests",
						Message:            "Release is awaiting tests",
						ObservedGeneration: 1,
					},
				},
			},
		},
		{
			name:       "with remediation failure",
			generation: 1,
			status: v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.InstallSucceededReason,
						Message:            "Install complete",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.TestSuccessCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.TestFailedReason,
						Message:            "test hook(s) failure",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.RemediatedCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.UninstallFailedReason,
						Message:            "Uninstall failure",
						ObservedGeneration: 1,
					},
				},
			},
			spec: &v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			expectedStatus: &v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               meta.ReadyCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.UninstallFailedReason,
						Message:            "Uninstall failure",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.InstallSucceededReason,
						Message:            "Install complete",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.TestSuccessCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.TestFailedReason,
						Message:            "test hook(s) failure",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.RemediatedCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.UninstallFailedReason,
						Message:            "Uninstall failure",
						ObservedGeneration: 1,
					},
				},
			},
		},
		{
			name:       "with remediation success",
			generation: 1,
			status: v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.UpgradeFailedReason,
						Message:            "Upgrade failure",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.RemediatedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.RollbackSucceededReason,
						Message:            "Uninstall complete",
						ObservedGeneration: 1,
					},
				},
			},
			expectedStatus: &v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               meta.ReadyCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.RollbackSucceededReason,
						Message:            "Uninstall complete",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.UpgradeFailedReason,
						Message:            "Upgrade failure",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.RemediatedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.RollbackSucceededReason,
						Message:            "Uninstall complete",
						ObservedGeneration: 1,
					},
				},
			},
		},
		{
			name:       "with stale ready",
			generation: 1,
			status: v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:    meta.ReadyCondition,
						Status:  metav1.ConditionFalse,
						Reason:  "ChartNotFound",
						Message: "chart not found",
					},
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.UpgradeSucceededReason,
						Message:            "Upgrade finished",
						ObservedGeneration: 1,
					},
				},
			},
			expectedStatus: &v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               meta.ReadyCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.UpgradeSucceededReason,
						Message:            "Upgrade finished",
						ObservedGeneration: 1,
					},
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.UpgradeSucceededReason,
						Message:            "Upgrade finished",
						ObservedGeneration: 1,
					},
				},
			},
		},
		{
			name:       "with stale observed generation",
			generation: 5,
			spec: &v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			status: v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.UpgradeSucceededReason,
						Message:            "Upgrade finished",
						ObservedGeneration: 4,
					},
					{
						Type:               v2.RemediatedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.RollbackSucceededReason,
						Message:            "Rollback finished",
						ObservedGeneration: 3,
					},
					{
						Type:               v2.TestSuccessCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.TestFailedReason,
						Message:            "test hook(s) failure",
						ObservedGeneration: 2,
					},
				},
			},
			expectedStatus: &v2.HelmReleaseStatus{
				Conditions: []metav1.Condition{
					{
						Type:               meta.ReadyCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.UpgradeSucceededReason,
						Message:            "Upgrade finished",
						ObservedGeneration: 5,
					},
					{
						Type:               v2.ReleasedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.UpgradeSucceededReason,
						Message:            "Upgrade finished",
						ObservedGeneration: 4,
					},
					{
						Type:               v2.RemediatedCondition,
						Status:             metav1.ConditionTrue,
						Reason:             v2.RollbackSucceededReason,
						Message:            "Rollback finished",
						ObservedGeneration: 3,
					},
					{
						Type:               v2.TestSuccessCondition,
						Status:             metav1.ConditionFalse,
						Reason:             v2.TestFailedReason,
						Message:            "test hook(s) failure",
						ObservedGeneration: 2,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			obj := &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Generation: tt.generation,
				},
				Status: tt.status,
			}
			if tt.spec != nil {
				obj.Spec = *tt.spec.DeepCopy()
			}
			summarize(&Request{Object: obj})

			g.Expect(obj.Status.Conditions).To(conditions.MatchConditions(tt.expectedStatus.Conditions))
			g.Expect(obj.Status.ObservedPostRenderersDigest).To(Equal(tt.expectedStatus.ObservedPostRenderersDigest))
		})
	}
}

func mockLogBuffer(size int, lines int) *action.LogBuffer {
	log := action.NewLogBuffer(action.NewDebugLog(logr.Discard()), size)
	for i := 0; i < lines; i++ {
		log.Log("line %d", i+1)
	}
	return log
}

func Test_RecordOnObject(t *testing.T) {
	tests := []struct {
		name     string
		obj      *v2.HelmRelease
		r        observedReleases
		mutate   bool
		testFunc func(*v2.HelmRelease) error
	}{
		{
			name: "record observed releases",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
				},
			},
			r: observedReleases{
				1: {
					Name:    mockReleaseName,
					Version: 1,
					ChartMetadata: chart.Metadata{
						Name:    mockReleaseName,
						Version: "1.0.0",
					},
				},
			},
			testFunc: func(obj *v2.HelmRelease) error {
				if len(obj.Status.History) != 1 {
					return fmt.Errorf("history length is not 1")
				}
				if obj.Status.History[0].Name != mockReleaseName {
					return fmt.Errorf("release name is not %s", mockReleaseName)
				}
				return nil
			},
		},
		{
			name: "record observed releases with multiple versions",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
				},
			},
			r: observedReleases{
				1: {
					Name:    mockReleaseName,
					Version: 1,
					ChartMetadata: chart.Metadata{
						Name:    mockReleaseName,
						Version: "1.0.0",
					},
				},
				2: {
					Name:    mockReleaseName,
					Version: 2,
					ChartMetadata: chart.Metadata{
						Name:    mockReleaseName,
						Version: "2.0.0",
					},
				},
			},
			testFunc: func(obj *v2.HelmRelease) error {
				if len(obj.Status.History) != 1 {
					return fmt.Errorf("want history length 1, got %d", len(obj.Status.History))
				}
				if obj.Status.History[0].Name != mockReleaseName {
					return fmt.Errorf("release name is not %s", mockReleaseName)
				}
				if obj.Status.History[0].ChartVersion != "2.0.0" {
					return fmt.Errorf("want chart version %s, got %s", "2.0.0", obj.Status.History[0].ChartVersion)
				}
				return nil
			},
		},
		{
			name: "record observed releases with status digest",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
				},
				Status: v2.HelmReleaseStatus{
					LastAttemptedRevisionDigest: "sha256:123456",
				},
			},
			r: observedReleases{
				1: {
					Name:    mockReleaseName,
					Version: 1,
					ChartMetadata: chart.Metadata{
						Name:    mockReleaseName,
						Version: "1.0.0",
					},
				},
			},
			mutate: true,
			testFunc: func(obj *v2.HelmRelease) error {
				h := obj.Status.History.Latest()
				if h.Name != mockReleaseName {
					return fmt.Errorf("release name is not %s", mockReleaseName)
				}
				if h.ChartVersion != "1.0.0" {
					return fmt.Errorf("want chart version %s, got %s", "1.0.0", h.ChartVersion)
				}
				if h.OCIDigest != obj.Status.LastAttemptedRevisionDigest {
					return fmt.Errorf("want digest %s, got %s", obj.Status.LastAttemptedRevisionDigest, h.OCIDigest)
				}
				return nil
			},
		},
		{
			name: "record observed releases with multiple versions and status digest",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
				},
				Status: v2.HelmReleaseStatus{
					LastAttemptedRevisionDigest: "sha256:123456",
				},
			},
			r: observedReleases{
				1: {
					Name:    mockReleaseName,
					Version: 1,
					ChartMetadata: chart.Metadata{
						Name:    mockReleaseName,
						Version: "1.0.0",
					},
				},
				2: {
					Name:    mockReleaseName,
					Version: 2,
					ChartMetadata: chart.Metadata{
						Name:    mockReleaseName,
						Version: "2.0.0",
					},
				},
			},
			mutate: true,
			testFunc: func(obj *v2.HelmRelease) error {
				if len(obj.Status.History) != 1 {
					return fmt.Errorf("want history length 1, got %d", len(obj.Status.History))
				}
				h := obj.Status.History.Latest()
				if h.Name != mockReleaseName {
					return fmt.Errorf("release name is not %s", mockReleaseName)
				}
				if h.ChartVersion != "2.0.0" {
					return fmt.Errorf("want chart version %s, got %s", "2.0.0", h.ChartVersion)
				}
				if h.OCIDigest != obj.Status.LastAttemptedRevisionDigest {
					return fmt.Errorf("want digest %s, got %s", obj.Status.LastAttemptedRevisionDigest, h.OCIDigest)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			if tt.mutate {
				tt.r.recordOnObject(tt.obj, mutateOCIDigest)
			} else {
				tt.r.recordOnObject(tt.obj)
			}
			err := tt.testFunc(tt.obj)
			g.Expect(err).ToNot(HaveOccurred())
		})
	}

}
