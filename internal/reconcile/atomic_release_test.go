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
	"context"
	"testing"
	"time"

	"github.com/fluxcd/pkg/runtime/patch"
	. "github.com/onsi/gomega"
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/kube"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

func TestReleaseStrategy_CleanRelease_MustContinue(t *testing.T) {
	tests := []struct {
		name     string
		current  ReconcilerType
		previous ReconcilerTypeSet
		want     bool
	}{
		{
			name:    "continue if not in previous",
			current: ReconcilerTypeRemediate,
			previous: []ReconcilerType{
				ReconcilerTypeRelease,
			},
			want: true,
		},
		{
			name:    "do not continue if in previous",
			current: ReconcilerTypeRemediate,
			previous: []ReconcilerType{
				ReconcilerTypeRemediate,
			},
			want: false,
		},
		{
			name:     "do continue on nil",
			current:  ReconcilerTypeRemediate,
			previous: nil,
			want:     true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := &cleanReleaseStrategy{}
			if got := at.MustContinue(tt.current, tt.previous); got != tt.want {
				g := NewWithT(t)
				g.Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func TestReleaseStrategy_CleanRelease_MustStop(t *testing.T) {
	tests := []struct {
		name     string
		current  ReconcilerType
		previous ReconcilerTypeSet
		want     bool
	}{
		{
			name:    "stop if current is remediate",
			current: ReconcilerTypeRemediate,
			want:    true,
		},
		{
			name:    "do not stop if current is not remediate",
			current: ReconcilerTypeRelease,
			want:    false,
		},
		{
			name:    "do not stop if current is not remediate",
			current: ReconcilerTypeUnlock,
			want:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := &cleanReleaseStrategy{}
			if got := at.MustStop(tt.current, tt.previous); got != tt.want {
				g := NewWithT(t)
				g.Expect(got).To(Equal(tt.want))
			}
		})
	}
}

func TestAtomicRelease_Reconcile(t *testing.T) {
	t.Run("runs a series of actions", func(t *testing.T) {
		g := NewWithT(t)

		namedNS, err := testEnv.CreateNamespace(context.TODO(), mockReleaseNamespace)
		g.Expect(err).NotTo(HaveOccurred())
		t.Cleanup(func() {
			_ = testEnv.Delete(context.TODO(), namedNS)
		})
		releaseNamespace := namedNS.Name

		obj := &v2.HelmRelease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      mockReleaseName,
				Namespace: releaseNamespace,
			},
			Spec: v2.HelmReleaseSpec{
				ReleaseName:     mockReleaseName,
				TargetNamespace: releaseNamespace,
				Test: &v2.Test{
					Enable: true,
				},
				StorageNamespace: releaseNamespace,
				Timeout:          &metav1.Duration{Duration: 100 * time.Millisecond},
			},
		}

		getter, err := RESTClientGetterFromManager(testEnv.Manager, obj.GetReleaseNamespace())
		g.Expect(err).ToNot(HaveOccurred())

		cfg, err := action.NewConfigFactory(getter,
			action.WithStorage(action.DefaultStorageDriver, obj.GetStorageNamespace()),
		)
		g.Expect(err).ToNot(HaveOccurred())

		// We use a fake client here to allow us to work with a minimal release
		// object mock. As the fake client does not perform any validation.
		// However, for the Helm storage driver to work, we need a real client
		// which is therefore initialized separately above.
		client := fake.NewClientBuilder().
			WithScheme(testEnv.Scheme()).
			WithObjects(obj).
			WithStatusSubresource(&v2.HelmRelease{}).
			Build()
		patchHelper := patch.NewSerialPatcher(obj, client)
		recorder := new(record.FakeRecorder)

		req := &Request{
			Object: obj,
			Chart:  testutil.BuildChart(testutil.ChartWithTestHook()),
			Values: nil,
		}
		g.Expect(NewAtomicRelease(patchHelper, cfg, recorder, "helm-controller").Reconcile(context.TODO(), req)).ToNot(HaveOccurred())

		g.Expect(obj.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			{
				Type:    meta.ReadyCondition,
				Status:  metav1.ConditionTrue,
				Reason:  v2.TestSucceededReason,
				Message: "test hook completed successfully",
			},
			{
				Type:    v2.ReleasedCondition,
				Status:  metav1.ConditionTrue,
				Reason:  v2.InstallSucceededReason,
				Message: "Helm install succeeded",
			},
			{
				Type:    v2.TestSuccessCondition,
				Status:  metav1.ConditionTrue,
				Reason:  v2.TestSucceededReason,
				Message: "test hook completed successfully",
			},
		}))
		g.Expect(obj.GetCurrent()).ToNot(BeNil(), "expected current to not be nil")
		g.Expect(obj.GetPrevious()).To(BeNil(), "expected previous to be nil")

		g.Expect(obj.Status.Failures).To(BeZero())
		g.Expect(obj.Status.InstallFailures).To(BeZero())
		g.Expect(obj.Status.UpgradeFailures).To(BeZero())

		endState, err := DetermineReleaseState(cfg, req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(endState).To(Equal(ReleaseState{Status: ReleaseStatusInSync}))
	})
}

func TestAtomicRelease_actionForState(t *testing.T) {
	tests := []struct {
		name     string
		releases []*helmrelease.Release
		spec     func(spec *v2.HelmReleaseSpec)
		status   func(releases []*helmrelease.Release) v2.HelmReleaseStatus
		state    ReleaseState
		want     ActionReconciler
		wantErr  error
	}{
		{
			name:  "in-sync release does not trigger any action",
			state: ReleaseState{Status: ReleaseStatusInSync},
			want:  nil,
		},
		{
			name:  "locked release triggers unlock action",
			state: ReleaseState{Status: ReleaseStatusLocked},
			want:  &Unlock{},
		},
		{
			name:  "absent release triggers install action",
			state: ReleaseState{Status: ReleaseStatusAbsent},
			want:  &Install{},
		},
		{
			name:  "absent release without remaining retries returns error",
			state: ReleaseState{Status: ReleaseStatusAbsent},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					InstallFailures: 1,
				}
			},
			wantErr: ErrExceededMaxRetries,
		},
		{
			name:  "unmanaged release triggers upgrade",
			state: ReleaseState{Status: ReleaseStatusUnmanaged},
			want:  &Upgrade{},
		},
		{
			name: "out-of-sync release triggers upgrade",
			state: ReleaseState{
				Status: ReleaseStatusOutOfSync,
			},
			want: &Upgrade{},
		},
		{
			name: "out-of-sync release with no remaining retries returns error",
			state: ReleaseState{
				Status: ReleaseStatusOutOfSync,
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					UpgradeFailures: 1,
				}
			},
			wantErr: ErrExceededMaxRetries,
		},
		{
			name:  "untested release triggers test action",
			state: ReleaseState{Status: ReleaseStatusUntested},
			want:  &Test{},
		},
		{
			name:  "failed release without active remediation triggers upgrade",
			state: ReleaseState{Status: ReleaseStatusFailed},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					LastAttemptedReleaseAction: "",
					InstallFailures:            1,
				}
			},
			want: &Upgrade{},
		},
		{
			name:  "failed release without failure count triggers upgrade",
			state: ReleaseState{Status: ReleaseStatusFailed},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					LastAttemptedReleaseAction: v2.ReleaseActionUpgrade,
					UpgradeFailures:            0,
				}
			},
			want: &Upgrade{},
		},
		{
			name:  "failed release with exhausted retries returns error",
			state: ReleaseState{Status: ReleaseStatusFailed},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					LastAttemptedReleaseAction: v2.ReleaseActionUpgrade,
					UpgradeFailures:            1,
				}
			},
			wantErr: ErrExceededMaxRetries,
		},
		{
			name:  "failed release with active install remediation triggers uninstall",
			state: ReleaseState{Status: ReleaseStatusFailed},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Install = &v2.Install{
					Remediation: &v2.InstallRemediation{
						Retries: 3,
					},
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					LastAttemptedReleaseAction: v2.ReleaseActionInstall,
					InstallFailures:            2,
				}
			},
			want: &UninstallRemediation{},
		},
		{
			name:  "failed release with active upgrade remediation triggers rollback",
			state: ReleaseState{Status: ReleaseStatusFailed},
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusSuperseded,
					Chart:     testutil.BuildChart(),
				}),
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   2,
					Status:    helmrelease.StatusFailed,
					Chart:     testutil.BuildChart(),
				}),
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Upgrade = &v2.Upgrade{
					Remediation: &v2.UpgradeRemediation{
						Retries: 2,
					},
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Previous: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
					LastAttemptedReleaseAction: v2.ReleaseActionUpgrade,
					UpgradeFailures:            1,
				}
			},
			want: &RollbackRemediation{},
		},
		{
			name:  "failed release with active upgrade remediation and unverified previous triggers upgrade",
			state: ReleaseState{Status: ReleaseStatusFailed},
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   2,
					Status:    helmrelease.StatusSuperseded,
					Chart:     testutil.BuildChart(),
				}),
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Upgrade = &v2.Upgrade{
					Remediation: &v2.UpgradeRemediation{
						Retries: 2,
					},
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Previous: release.ObservedToSnapshot(release.ObserveRelease(
							testutil.BuildRelease(&helmrelease.MockReleaseOptions{
								Name:      mockReleaseName,
								Namespace: mockReleaseNamespace,
								Version:   1,
								Status:    helmrelease.StatusSuperseded,
								Chart:     testutil.BuildChart(),
							}),
						)),
					},
					LastAttemptedReleaseAction: v2.ReleaseActionUpgrade,
					UpgradeFailures:            1,
				}
			},
			want: &Upgrade{},
		},
		{
			name: "unknown remediation strategy returns error",
			state: ReleaseState{
				Status: ReleaseStatusFailed,
			},
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusSuperseded,
					Chart:     testutil.BuildChart(),
				}),
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				strategy := v2.RemediationStrategy("invalid")
				spec.Upgrade = &v2.Upgrade{
					Remediation: &v2.UpgradeRemediation{
						Strategy: &strategy,
						Retries:  2,
					},
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Previous: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
					LastAttemptedReleaseAction: v2.ReleaseActionUpgrade,
					UpgradeFailures:            1,
				}
			},
			wantErr: ErrUnknownRemediationStrategy,
		},
		{
			name:    "invalid release status returns error",
			state:   ReleaseState{Status: "invalid"},
			wantErr: ErrUnknownReleaseStatus,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			obj := &v2.HelmRelease{
				Spec: v2.HelmReleaseSpec{
					ReleaseName:      mockReleaseName,
					TargetNamespace:  mockReleaseNamespace,
					StorageNamespace: mockReleaseNamespace,
				},
			}
			if tt.spec != nil {
				tt.spec(&obj.Spec)
			}
			if tt.status != nil {
				obj.Status = tt.status(tt.releases)
			}

			cfg, err := action.NewConfigFactory(&kube.MemoryRESTClientGetter{},
				action.WithStorage(helmdriver.MemoryDriverName, mockReleaseNamespace),
			)
			g.Expect(err).ToNot(HaveOccurred())

			if len(tt.releases) > 0 {
				store := helmstorage.Init(cfg.Driver)
				for _, i := range tt.releases {
					g.Expect(store.Create(i)).To(Succeed())
				}
			}

			r := &AtomicRelease{configFactory: cfg}
			got, err := r.actionForState(context.TODO(), &Request{Object: obj}, tt.state)

			if tt.wantErr != nil {
				g.Expect(got).To(BeNil())
				g.Expect(err).To(MatchError(tt.wantErr))
				return
			}
			g.Expect(err).ToNot(HaveOccurred())

			want := BeAssignableToTypeOf(tt.want)
			if tt.want == nil {
				want = BeNil()
			}
			g.Expect(got).To(want)
		})
	}
}
