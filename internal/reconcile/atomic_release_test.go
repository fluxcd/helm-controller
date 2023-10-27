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
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	extjsondiff "github.com/wI2L/jsondiff"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmrelease "helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/runtime/patch"
	"github.com/fluxcd/pkg/ssa/jsondiff"

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
		g.Expect(NewAtomicRelease(patchHelper, cfg, recorder, testFieldManager).Reconcile(context.TODO(), req)).ToNot(HaveOccurred())

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
		g.Expect(obj.Status.History.Latest()).ToNot(BeNil(), "expected current to not be nil")
		g.Expect(obj.Status.History.Previous(false)).To(BeNil(), "expected previous to be nil")

		g.Expect(obj.Status.Failures).To(BeZero())
		g.Expect(obj.Status.InstallFailures).To(BeZero())
		g.Expect(obj.Status.UpgradeFailures).To(BeZero())

		endState, err := DetermineReleaseState(ctx, cfg, req)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(endState).To(Equal(ReleaseState{Status: ReleaseStatusInSync}))
	})
}

func TestAtomicRelease_Reconcile_Scenarios(t *testing.T) {
	tests := []struct {
		name          string
		releases      func(namespace string) []*helmrelease.Release
		spec          func(spec *v2.HelmReleaseSpec)
		status        func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus
		chart         *helmchart.Chart
		values        map[string]interface{}
		expectHistory func(releases []*helmrelease.Release) v2.Snapshots
		wantErr       error
	}{
		{
			name: "release is in-sync",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}, testutil.ReleaseWithConfig(nil)),
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: nil,
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "release is out-of-sync (chart)",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}, testutil.ReleaseWithConfig(nil)),
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(testutil.ChartWithVersion("0.2.0")),
			values: nil,
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "release is out-of-sync (values)",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "baz"},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "release is locked (pending-install)",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusPendingInstall,
					}, testutil.ReleaseWithConfig(nil)),
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: nil,
				}
			},
			chart: testutil.BuildChart(),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[len(releases)-1])),
				}
			},
		},
		{
			name: "release is locked (pending-upgrade)",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}, testutil.ReleaseWithConfig(nil)),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   2,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusPendingUpgrade,
					}, testutil.ReleaseWithConfig(nil)),
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart: testutil.BuildChart(),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[len(releases)-1])),
				}
			},
		},
		{
			name: "release is locked (pending-rollback)",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}, testutil.ReleaseWithConfig(nil)),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   2,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusFailed,
					}, testutil.ReleaseWithConfig(nil)),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   3,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusPendingRollback,
					}, testutil.ReleaseWithConfig(nil)),
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart: testutil.BuildChart(),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[len(releases)-1])),
				}
			},
		},
		{
			name:  "release is not installed",
			chart: testutil.BuildChart(),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "release exists but is not managed",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}),
				}
			},
			chart: testutil.BuildChart(),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
				}
			},
		},
		{
			name: "release was upgraded outside of the reconciler",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   3,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   2,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusSuperseded,
					}),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusSuperseded,
					}),
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				previousDeployed := release.ObserveRelease(releases[1])
				previousDeployed.Info.Status = helmrelease.StatusDeployed

				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(previousDeployed),
					},
				}
			},
			chart: testutil.BuildChart(),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[len(releases)-1])),
				}
			},
		},
		{
			name: "release was rolled back outside of the reconciler",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   3,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   2,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusSuperseded,
					}),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusSuperseded,
					}),
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				modifiedRelease := release.ObserveRelease(releases[1])
				modifiedRelease.Info.Status = helmrelease.StatusFailed

				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(modifiedRelease),
					},
				}
			},
			chart: testutil.BuildChart(),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[len(releases)-1])),
				}
			},
		},
		{
			name: "release was deleted outside of the reconciler",
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(
							testutil.BuildRelease(&helmrelease.MockReleaseOptions{
								Name:      mockReleaseName,
								Namespace: namespace,
								Version:   1,
								Chart:     testutil.BuildChart(),
								Status:    helmrelease.StatusDeployed,
							}),
						)),
					},
				}
			},
			chart: testutil.BuildChart(),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "part of the release history was deleted outside of the reconciler",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusSuperseded,
					}),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   2,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusSuperseded,
					}),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   3,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}),
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				deletedRelease := release.ObservedToSnapshot(release.ObserveRelease(
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   4,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusFailed,
					}),
				))

				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						deletedRelease,
						release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
					},
				}
			},
			chart: testutil.BuildChart(),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[len(releases)-1])),
				}
			},
		},
		{
			name:  "install failure",
			chart: testutil.BuildChart(testutil.ChartWithFailingHook()),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			wantErr: ErrExceededMaxRetries,
		},
		{
			name: "install failure with remediation",
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Install = &v2.Install{
					Remediation: &v2.InstallRemediation{
						RemediateLastFailure: pointer.Bool(true),
					},
				}
				spec.Uninstall = &v2.Uninstall{
					KeepHistory: true,
				}
			},
			chart: testutil.BuildChart(testutil.ChartWithFailingHook()),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "install test failure with remediation",
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Install = &v2.Install{
					Remediation: &v2.InstallRemediation{
						RemediateLastFailure: pointer.Bool(true),
					},
				}
				spec.Test = &v2.Test{
					Enable: true,
				}
				spec.Uninstall = &v2.Uninstall{
					KeepHistory: true,
				}
			},
			chart: testutil.BuildChart(testutil.ChartWithFailingTestHook()),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				snap := release.ObservedToSnapshot(release.ObserveRelease(releases[0]))
				snap.SetTestHooks(release.TestHooksFromRelease(releases[0]))

				return v2.Snapshots{
					snap,
				}
			},
		},
		{
			name: "install test failure with test ignore",
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Test = &v2.Test{
					Enable:         true,
					IgnoreFailures: true,
				}
			},
			chart: testutil.BuildChart(testutil.ChartWithFailingTestHook()),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				snap := release.ObservedToSnapshot(release.ObserveRelease(releases[0]))
				snap.SetTestHooks(release.TestHooksFromRelease(releases[0]))

				return v2.Snapshots{
					snap,
				}
			},
		},
		{
			name: "install with exhausted retries after remediation",
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(
							testutil.BuildRelease(&helmrelease.MockReleaseOptions{
								Name:      mockReleaseName,
								Namespace: namespace,
								Version:   1,
								Chart:     testutil.BuildChart(),
								Status:    helmrelease.StatusUninstalling,
							}),
						)),
					},
					LastAttemptedReleaseAction: v2.ReleaseActionInstall,
					Failures:                   1,
					InstallFailures:            1,
				}
			},
			wantErr: ErrExceededMaxRetries,
		},
		{
			name: "upgrade failure",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}),
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart: testutil.BuildChart(testutil.ChartWithFailingHook()),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			wantErr: ErrExceededMaxRetries,
		},
		{
			name: "upgrade failure with rollback remediation",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}),
				}
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Upgrade = &v2.Upgrade{
					Remediation: &v2.UpgradeRemediation{
						RemediateLastFailure: pointer.Bool(true),
					},
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart: testutil.BuildChart(testutil.ChartWithFailingHook()),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[2])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "upgrade failure with uninstall remediation",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}),
				}
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				strategy := v2.UninstallRemediationStrategy
				spec.Upgrade = &v2.Upgrade{
					Remediation: &v2.UpgradeRemediation{
						Strategy:             &strategy,
						RemediateLastFailure: pointer.Bool(true),
					},
				}
				spec.Uninstall = &v2.Uninstall{
					KeepHistory: true,
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart: testutil.BuildChart(testutil.ChartWithFailingHook()),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "upgrade test failure with remediation",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}),
				}
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Upgrade = &v2.Upgrade{
					Remediation: &v2.UpgradeRemediation{
						RemediateLastFailure: pointer.Bool(true),
					},
				}
				spec.Test = &v2.Test{
					Enable: true,
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart: testutil.BuildChart(testutil.ChartWithFailingTestHook()),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				testedSnap := release.ObservedToSnapshot(release.ObserveRelease(releases[1]))
				testedSnap.SetTestHooks(release.TestHooksFromRelease(releases[1]))

				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[2])),
					testedSnap,
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "upgrade test failure with test ignore",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}),
				}
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Test = &v2.Test{
					Enable:         true,
					IgnoreFailures: true,
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart: testutil.BuildChart(testutil.ChartWithFailingTestHook()),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				testedSnap := release.ObservedToSnapshot(release.ObserveRelease(releases[1]))
				testedSnap.SetTestHooks(release.TestHooksFromRelease(releases[1]))

				return v2.Snapshots{
					testedSnap,
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "upgrade with exhausted retries after remediation",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusSuperseded,
					}),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   2,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusSuperseded,
					}),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   3,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}),
				}
			},
			status: func(namespace string, releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[2])),
						release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
					LastAttemptedReleaseAction: v2.ReleaseActionUpgrade,
					Failures:                   1,
					UpgradeFailures:            1,
				}
			},
			chart: testutil.BuildChart(),
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[2])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			wantErr: ErrExceededMaxRetries,
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
			releaseNamespace := namedNS.Name

			var releases []*helmrelease.Release
			if tt.releases != nil {
				releases = tt.releases(releaseNamespace)
				releaseutil.SortByRevision(releases)
			}

			obj := &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      mockReleaseName,
					Namespace: releaseNamespace,
				},
				Spec: v2.HelmReleaseSpec{
					ReleaseName:      mockReleaseName,
					TargetNamespace:  releaseNamespace,
					StorageNamespace: releaseNamespace,
					Timeout:          &metav1.Duration{Duration: 100 * time.Millisecond},
				},
			}
			if tt.spec != nil {
				tt.spec(&obj.Spec)
			}
			if tt.status != nil {
				obj.Status = tt.status(releaseNamespace, releases)
			}

			getter, err := RESTClientGetterFromManager(testEnv.Manager, obj.GetReleaseNamespace())
			g.Expect(err).ToNot(HaveOccurred())

			cfg, err := action.NewConfigFactory(getter,
				action.WithStorage(action.DefaultStorageDriver, obj.GetStorageNamespace()),
			)
			g.Expect(err).ToNot(HaveOccurred())

			store := helmstorage.Init(cfg.Driver)
			for _, r := range releases {
				g.Expect(store.Create(r)).To(Succeed())
			}

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
				Chart:  tt.chart,
				Values: tt.values,
			}

			err = NewAtomicRelease(patchHelper, cfg, recorder, testFieldManager).Reconcile(context.TODO(), req)
			wantErr := BeNil()
			if tt.wantErr != nil {
				wantErr = MatchError(tt.wantErr)
			}
			g.Expect(err).To(wantErr)

			if tt.expectHistory != nil {
				history, _ := store.History(mockReleaseName)
				releaseutil.SortByRevision(history)

				g.Expect(req.Object.Status.History).To(testutil.Equal(tt.expectHistory(history)))
			}
		})
	}
}

func TestAtomicRelease_actionForState(t *testing.T) {
	tests := []struct {
		name      string
		releases  []*helmrelease.Release
		spec      func(spec *v2.HelmReleaseSpec)
		status    func(releases []*helmrelease.Release) v2.HelmReleaseStatus
		state     ReleaseState
		want      ActionReconciler
		wantEvent *corev1.Event
		wantErr   error
	}{
		{
			name: "in-sync release does not trigger any action",
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						{Version: 1},
					},
				}
			},
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
			name: "drifted release triggers upgrade if enabled",
			state: ReleaseState{Status: ReleaseStatusDrifted, Diff: jsondiff.DiffSet{
				{
					Type: jsondiff.DiffTypeCreate,
					GroupVersionKind: schema.GroupVersionKind{
						Group:   "apps",
						Kind:    "Deployment",
						Version: "v1",
					},
					Name:      "mock",
					Namespace: "something",
				},
			}},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.DriftDetection = &v2.DriftDetection{
					Mode: v2.DriftDetectionEnabled,
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						{
							Name:      mockReleaseName,
							Namespace: mockReleaseNamespace,
							Version:   1,
						},
					},
				}
			},
			want: &Upgrade{},
			wantEvent: &corev1.Event{
				Reason: "DriftDetected",
				Type:   corev1.EventTypeWarning,
				Message: fmt.Sprintf(
					"Cluster state of release %s has drifted from the desired state:\n%s",
					mockReleaseNamespace+"/"+mockReleaseName+".v1",
					"Deployment/something/mock removed",
				),
			},
		},
		{
			name: "drifted release only triggers event if mode is warn",
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.DriftDetection = &v2.DriftDetection{
					Mode: v2.DriftDetectionDisabled,
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						{
							Name:      mockReleaseName,
							Namespace: mockReleaseNamespace,
							Version:   1,
						},
					},
				}
			},
			state: ReleaseState{Status: ReleaseStatusDrifted, Diff: jsondiff.DiffSet{
				{
					Type: jsondiff.DiffTypeUpdate,
					GroupVersionKind: schema.GroupVersionKind{
						Group:   "apps",
						Kind:    "Deployment",
						Version: "v1",
					},
					Name:      "mock",
					Namespace: "something",
					Patch: extjsondiff.Patch{
						{
							Type:     extjsondiff.OperationReplace,
							Path:     "/spec/replicas",
							OldValue: 1,
							Value:    2,
						},
					},
				},
			}},
			want:    nil,
			wantErr: nil,
			wantEvent: &corev1.Event{
				Reason: "DriftDetected",
				Type:   corev1.EventTypeWarning,
				Message: fmt.Sprintf(
					"Cluster state of release %s has drifted from the desired state:\n%s",
					mockReleaseNamespace+"/"+mockReleaseName+".v1",
					"Deployment/something/mock changed (0 additions, 1 changes, 0 removals)",
				),
			},
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
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
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
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(
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
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
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

			recorder := testutil.NewFakeRecorder(1, false)
			r := &AtomicRelease{configFactory: cfg, eventRecorder: recorder}
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

			if tt.wantEvent != nil {
				g.Expect(recorder.GetEvents()).To(ConsistOf([]corev1.Event{*tt.wantEvent}))
			} else {
				g.Expect(recorder.GetEvents()).To(BeEmpty())
			}
		})
	}
}
