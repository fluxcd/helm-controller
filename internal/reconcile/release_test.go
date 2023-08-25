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
	"testing"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	helmrelease "helm.sh/helm/v3/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
)

const (
	mockReleaseName      = "mock-release"
	mockReleaseNamespace = "mock-ns"
)

func Test_observeRelease(t *testing.T) {
	const (
		otherReleaseName      = "other"
		otherReleaseNamespace = "other-ns"
	)

	t.Run("release", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{}
		mock := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusPendingInstall,
		})
		expect := release.ObservedToSnapshot(release.ObserveRelease(mock))

		observeRelease(obj)(mock)

		g.Expect(obj.GetPrevious()).To(BeNil())
		g.Expect(obj.GetCurrent()).ToNot(BeNil())
		g.Expect(obj.GetCurrent()).To(Equal(expect))
	})

	t.Run("release with current", func(t *testing.T) {
		g := NewWithT(t)

		current := &v2.Snapshot{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.ReleaseHistory{
					Current: current,
				},
			},
		}
		mock := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      current.Name,
			Namespace: current.Namespace,
			Version:   current.Version + 1,
			Status:    helmrelease.StatusPendingInstall,
		})
		expect := release.ObservedToSnapshot(release.ObserveRelease(mock))

		observeRelease(obj)(mock)
		g.Expect(obj.GetPrevious()).ToNot(BeNil())
		g.Expect(obj.GetPrevious()).To(Equal(current))
		g.Expect(obj.GetCurrent()).ToNot(BeNil())
		g.Expect(obj.GetCurrent()).To(Equal(expect))
	})

	t.Run("release with current with different name", func(t *testing.T) {
		g := NewWithT(t)

		current := &v2.Snapshot{
			Name:      otherReleaseName,
			Namespace: otherReleaseNamespace,
			Version:   3,
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.ReleaseHistory{
					Current: current,
				},
			},
		}
		mock := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusPendingInstall,
		})
		expect := release.ObservedToSnapshot(release.ObserveRelease(mock))

		observeRelease(obj)(mock)
		g.Expect(obj.GetPrevious()).To(BeNil())
		g.Expect(obj.GetCurrent()).ToNot(BeNil())
		g.Expect(obj.GetCurrent()).To(Equal(expect))
	})

	t.Run("release with update to previous", func(t *testing.T) {
		g := NewWithT(t)

		previous := &v2.Snapshot{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusDeployed.String(),
		}
		current := &v2.Snapshot{
			Name:      previous.Name,
			Namespace: previous.Namespace,
			Version:   previous.Version + 1,
			Status:    helmrelease.StatusPendingInstall.String(),
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.ReleaseHistory{
					Current:  current,
					Previous: previous,
				},
			},
		}
		mock := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      previous.Name,
			Namespace: previous.Namespace,
			Version:   previous.Version,
			Status:    helmrelease.StatusSuperseded,
		})
		expect := release.ObservedToSnapshot(release.ObserveRelease(mock))

		observeRelease(obj)(mock)
		g.Expect(obj.GetPrevious()).ToNot(BeNil())
		g.Expect(obj.GetPrevious()).To(Equal(expect))
		g.Expect(obj.GetCurrent()).To(Equal(current))
	})
}

func Test_summarize(t *testing.T) {
	tests := []struct {
		name       string
		generation int64
		spec       *v2.HelmReleaseSpec
		conditions []metav1.Condition
		expect     []metav1.Condition
	}{
		{
			name:       "summarize conditions",
			generation: 1,
			conditions: []metav1.Condition{
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
			expect: []metav1.Condition{
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
		{
			name:       "with tests enabled",
			generation: 1,
			conditions: []metav1.Condition{
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
			spec: &v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			expect: []metav1.Condition{
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
		{
			name:       "with tests enabled and failure tests",
			generation: 1,
			conditions: []metav1.Condition{
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
			spec: &v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			expect: []metav1.Condition{
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
		{
			name: "with test hooks enabled and pending tests",
			conditions: []metav1.Condition{
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
					Reason:             "Pending",
					Message:            "Release is awaiting tests",
					ObservedGeneration: 1,
				},
			},
			spec: &v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			expect: []metav1.Condition{
				{
					Type:               meta.ReadyCondition,
					Status:             metav1.ConditionFalse,
					Reason:             "Pending",
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
					Reason:             "Pending",
					Message:            "Release is awaiting tests",
					ObservedGeneration: 1,
				},
			},
		},
		{
			name:       "with remediation failure",
			generation: 1,
			conditions: []metav1.Condition{
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
			spec: &v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			expect: []metav1.Condition{
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
		{
			name:       "with remediation success",
			generation: 1,
			conditions: []metav1.Condition{
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
			expect: []metav1.Condition{
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
		{
			name:       "with stale ready",
			generation: 1,
			conditions: []metav1.Condition{
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
			expect: []metav1.Condition{
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
		{
			name:       "with stale observed generation",
			generation: 5,
			spec: &v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			conditions: []metav1.Condition{
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
			expect: []metav1.Condition{
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
		{
			name: "with stale remediation",
			spec: &v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			conditions: []metav1.Condition{
				{
					Type:               v2.RemediatedCondition,
					Status:             metav1.ConditionTrue,
					Reason:             v2.RollbackSucceededReason,
					Message:            "Rollback finished",
					ObservedGeneration: 2,
				},
				{
					Type:               v2.ReleasedCondition,
					Status:             metav1.ConditionTrue,
					Reason:             v2.UpgradeSucceededReason,
					Message:            "Upgrade finished",
					ObservedGeneration: 2,
				},
				{
					Type:               v2.TestSuccessCondition,
					Status:             metav1.ConditionTrue,
					Reason:             v2.TestSucceededReason,
					Message:            "test hooks succeeded",
					ObservedGeneration: 2,
				},
			},
			expect: []metav1.Condition{
				{
					Type:               meta.ReadyCondition,
					Status:             metav1.ConditionTrue,
					Reason:             v2.TestSucceededReason,
					Message:            "test hooks succeeded",
					ObservedGeneration: 2,
				},
				{
					Type:               v2.ReleasedCondition,
					Status:             metav1.ConditionTrue,
					Reason:             v2.UpgradeSucceededReason,
					Message:            "Upgrade finished",
					ObservedGeneration: 2,
				},
				{
					Type:               v2.TestSuccessCondition,
					Status:             metav1.ConditionTrue,
					Reason:             v2.TestSucceededReason,
					Message:            "test hooks succeeded",
					ObservedGeneration: 2,
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
				Status: v2.HelmReleaseStatus{
					Conditions: tt.conditions,
				},
			}
			if tt.spec != nil {
				obj.Spec = *tt.spec.DeepCopy()
			}
			summarize(&Request{Object: obj})

			g.Expect(obj.Status.Conditions).To(conditions.MatchConditions(tt.expect))
		})
	}
}

func Test_conditionallyDeleteRemediated(t *testing.T) {
	tests := []struct {
		name         string
		spec         v2.HelmReleaseSpec
		conditions   []metav1.Condition
		expectDelete bool
	}{
		{
			name: "no Remediated condition",
			conditions: []metav1.Condition{
				*conditions.TrueCondition(v2.ReleasedCondition, v2.InstallSucceededReason, "Install finished"),
			},
			expectDelete: false,
		},
		{
			name: "no Released condition",
			conditions: []metav1.Condition{
				*conditions.TrueCondition(v2.RemediatedCondition, v2.RollbackSucceededReason, "Rollback finished"),
			},
			expectDelete: false,
		},
		{
			name: "Released=True without tests enabled",
			conditions: []metav1.Condition{
				*conditions.TrueCondition(v2.RemediatedCondition, v2.RollbackSucceededReason, "Rollback finished"),
				*conditions.TrueCondition(v2.ReleasedCondition, v2.UpgradeSucceededReason, "Upgrade finished"),
			},
			expectDelete: true,
		},
		{
			name: "Stale Released=True with newer Remediated",
			conditions: []metav1.Condition{
				*conditions.TrueCondition(v2.ReleasedCondition, v2.UpgradeSucceededReason, "Upgrade finished"),
				{
					Type:               v2.RemediatedCondition,
					Status:             metav1.ConditionTrue,
					Reason:             v2.RollbackSucceededReason,
					Message:            "Rollback finished",
					ObservedGeneration: 2,
				},
			},
			expectDelete: false,
		},
		{
			name: "Released=False",
			conditions: []metav1.Condition{
				*conditions.TrueCondition(v2.RemediatedCondition, v2.RollbackSucceededReason, "Rollback finished"),
				*conditions.FalseCondition(v2.ReleasedCondition, v2.UpgradeFailedReason, "Upgrade failed"),
			},
			expectDelete: false,
		},
		{
			name: "TestSuccess=True with tests enabled",
			spec: v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			conditions: []metav1.Condition{
				*conditions.TrueCondition(v2.RemediatedCondition, v2.RollbackSucceededReason, "Rollback finished"),
				*conditions.TrueCondition(v2.ReleasedCondition, v2.UpgradeSucceededReason, "Upgrade finished"),
				*conditions.TrueCondition(v2.TestSuccessCondition, v2.TestSucceededReason, "Test hooks succeeded"),
			},
			expectDelete: true,
		},
		{
			name: "TestSuccess=False with tests enabled",
			spec: v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			conditions: []metav1.Condition{
				*conditions.TrueCondition(v2.RemediatedCondition, v2.RollbackSucceededReason, "Rollback finished"),
				*conditions.TrueCondition(v2.ReleasedCondition, v2.UpgradeSucceededReason, "Upgrade finished"),
				*conditions.FalseCondition(v2.TestSuccessCondition, v2.TestSucceededReason, "Test hooks succeeded"),
			},
			expectDelete: false,
		},
		{
			name: "TestSuccess=False with tests ignored",
			spec: v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable:         true,
					IgnoreFailures: true,
				},
			},
			conditions: []metav1.Condition{
				*conditions.TrueCondition(v2.RemediatedCondition, v2.RollbackSucceededReason, "Rollback finished"),
				*conditions.TrueCondition(v2.ReleasedCondition, v2.UpgradeSucceededReason, "Upgrade finished"),
				*conditions.FalseCondition(v2.TestSuccessCondition, v2.TestFailedReason, "Test hooks failed"),
			},
			expectDelete: true,
		},
		{
			name: "Stale TestSuccess=True with newer Remediated",
			spec: v2.HelmReleaseSpec{
				Test: &v2.Test{
					Enable: true,
				},
			},
			conditions: []metav1.Condition{
				*conditions.TrueCondition(v2.ReleasedCondition, v2.UpgradeSucceededReason, "Upgrade finished"),
				*conditions.TrueCondition(v2.TestSuccessCondition, v2.TestSucceededReason, "Test hooks succeeded"),
				{
					Type:               v2.RemediatedCondition,
					Status:             metav1.ConditionTrue,
					Reason:             v2.RollbackSucceededReason,
					Message:            "Rollback finished",
					ObservedGeneration: 2,
				},
			},
			expectDelete: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			obj := &v2.HelmRelease{
				Spec: tt.spec,
				Status: v2.HelmReleaseStatus{
					Conditions: tt.conditions,
				},
			}
			isRemediated := conditions.Has(obj, v2.RemediatedCondition)

			conditionallyDeleteRemediated(&Request{Object: obj})

			if tt.expectDelete {
				g.Expect(isRemediated).ToNot(Equal(conditions.Has(obj, v2.RemediatedCondition)))
				return
			}

			g.Expect(conditions.Has(obj, v2.RemediatedCondition)).To(Equal(isRemediated))
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
