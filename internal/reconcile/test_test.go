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
	"errors"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmreleaseutil "helm.sh/helm/v3/pkg/releaseutil"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fluxcd/pkg/runtime/conditions"

	helmv2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

// testHookFixtures is a list of release.Hook in every possible LastRun state.
var testHookFixtures = []*helmrelease.Hook{
	{
		Name:   "never-run-test",
		Events: []helmrelease.HookEvent{helmrelease.HookTest},
	},
	{
		Name:   "passing-test",
		Events: []helmrelease.HookEvent{helmrelease.HookTest},
		LastRun: helmrelease.HookExecution{
			StartedAt:   testutil.MustParseHelmTime("2006-01-02T15:04:05Z"),
			CompletedAt: testutil.MustParseHelmTime("2006-01-02T15:04:07Z"),
			Phase:       helmrelease.HookPhaseSucceeded,
		},
	},
	{
		Name:   "failing-test",
		Events: []helmrelease.HookEvent{helmrelease.HookTest},
		LastRun: helmrelease.HookExecution{
			StartedAt:   testutil.MustParseHelmTime("2006-01-02T15:10:05Z"),
			CompletedAt: testutil.MustParseHelmTime("2006-01-02T15:10:07Z"),
			Phase:       helmrelease.HookPhaseFailed,
		},
	},
	{
		Name:   "passing-pre-install",
		Events: []helmrelease.HookEvent{helmrelease.HookPreInstall},
		LastRun: helmrelease.HookExecution{
			StartedAt:   testutil.MustParseHelmTime("2006-01-02T15:00:05Z"),
			CompletedAt: testutil.MustParseHelmTime("2006-01-02T15:00:07Z"),
			Phase:       helmrelease.HookPhaseSucceeded,
		},
	},
}

func TestTest_Reconcile(t *testing.T) {
	tests := []struct {
		name string
		// driver allows for modifying the Helm storage driver.
		driver func(driver helmdriver.Driver) helmdriver.Driver
		// releases is the list of releases that are stored in the driver
		// before test.
		releases func(namespace string) []*helmrelease.Release
		// spec modifies the HelmRelease Object spec before test.
		spec func(spec *helmv2.HelmReleaseSpec)
		// status to configure on the HelmRelease Object before test.
		status func(releases []*helmrelease.Release) helmv2.HelmReleaseStatus
		// wantErr is the error that is expected to be returned.
		wantErr error
		// expectedConditions are the conditions that are expected to be set on
		// the HelmRelease after running rollback.
		expectConditions []metav1.Condition
		// expectCurrent is the expected Current release information in the
		// HelmRelease after install.
		expectCurrent func(releases []*helmrelease.Release) *helmv2.HelmReleaseInfo
		// expectPrevious returns the expected Previous release information of
		// the HelmRelease after install.
		expectPrevious func(releases []*helmrelease.Release) *helmv2.HelmReleaseInfo
		// expectFailures is the expected Failures count of the HelmRelease.
		expectFailures int64
		// expectInstallFailures is the expected InstallFailures count of the
		// HelmRelease.
		expectInstallFailures int64
		// expectUpgradeFailures is the expected UpgradeFailures count of the
		// HelmRelease.
		expectUpgradeFailures int64
	}{
		{
			name: "test success",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
						Status:    helmrelease.StatusDeployed,
					}, testutil.ReleaseWithTestHook()),
				}
			},
			status: func(releases []*helmrelease.Release) helmv2.HelmReleaseStatus {
				return helmv2.HelmReleaseStatus{
					Current: release.ObservedToInfo(release.ObserveRelease(releases[0])),
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.TrueCondition(helmv2.TestSuccessCondition, helmv2.TestSucceededReason,
					"1 test hook(s) completed successfully."),
			},
			expectCurrent: func(releases []*helmrelease.Release) *helmv2.HelmReleaseInfo {
				info := release.ObservedToInfo(release.ObserveRelease(releases[0]))
				info.TestHooks = release.TestHooksFromRelease(releases[0])
				return info
			},
		},
		{
			name: "test without hooks",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Status:    helmrelease.StatusDeployed,
						Chart:     testutil.BuildChart(),
					}),
				}
			},
			status: func(releases []*helmrelease.Release) helmv2.HelmReleaseStatus {
				return helmv2.HelmReleaseStatus{
					Current: release.ObservedToInfo(release.ObserveRelease(releases[0])),
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.TrueCondition(helmv2.TestSuccessCondition, helmv2.TestSucceededReason,
					"No test hooks."),
			},
			expectCurrent: func(releases []*helmrelease.Release) *helmv2.HelmReleaseInfo {
				info := release.ObservedToInfo(release.ObserveRelease(releases[0]))
				return info
			},
		},
		{
			name: "test failure",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Status:    helmrelease.StatusDeployed,
						Chart:     testutil.BuildChart(testutil.ChartWithFailingTestHook()),
					}, testutil.ReleaseWithFailingTestHook()),
				}
			},
			status: func(releases []*helmrelease.Release) helmv2.HelmReleaseStatus {
				return helmv2.HelmReleaseStatus{
					Current: release.ObservedToInfo(release.ObserveRelease(releases[0])),
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(helmv2.TestSuccessCondition, helmv2.TestFailedReason,
					"timed out waiting for the condition"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *helmv2.HelmReleaseInfo {
				info := release.ObservedToInfo(release.ObserveRelease(releases[0]))
				info.TestHooks = release.TestHooksFromRelease(releases[0])
				return info
			},
			expectFailures: 1,
		},
		{
			name: "test without current",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
						Status:    helmrelease.StatusDeployed,
					}, testutil.ReleaseWithTestHook()),
				}
			},
			expectConditions: []metav1.Condition{},
			wantErr:          ErrNoCurrent,
		},
		{
			name: "test with stale current",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
						Status:    helmrelease.StatusSuperseded,
					}, testutil.ReleaseWithTestHook()),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   2,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusDeployed,
					}),
				}
			},
			status: func(releases []*helmrelease.Release) helmv2.HelmReleaseStatus {
				return helmv2.HelmReleaseStatus{
					Current: release.ObservedToInfo(release.ObserveRelease(releases[0])),
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(helmv2.TestSuccessCondition, helmv2.TestFailedReason,
					ErrReleaseMismatch.Error()),
			},
			expectCurrent: func(releases []*helmrelease.Release) *helmv2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[0]))
			},
			expectFailures: 1,
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
				helmreleaseutil.SortByRevision(releases)
			}

			obj := &helmv2.HelmRelease{
				Spec: helmv2.HelmReleaseSpec{
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
				obj.Status = tt.status(releases)
			}

			getter, err := RESTClientGetterFromManager(testEnv.Manager, obj.GetReleaseNamespace())
			g.Expect(err).ToNot(HaveOccurred())

			cfg, err := action.NewConfigFactory(getter,
				action.WithStorage(action.DefaultStorageDriver, obj.GetStorageNamespace()),
				action.WithDebugLog(logr.Discard()),
			)
			g.Expect(err).ToNot(HaveOccurred())

			store := helmstorage.Init(cfg.Driver)
			for _, r := range releases {
				g.Expect(store.Create(r)).To(Succeed())
			}

			if tt.driver != nil {
				cfg.Driver = tt.driver(cfg.Driver)
			}

			got := (&Test{configFactory: cfg}).Reconcile(context.TODO(), &Request{
				Object: obj,
			})
			if tt.wantErr != nil {
				g.Expect(errors.Is(got, tt.wantErr)).To(BeTrue())
			} else {
				g.Expect(got).ToNot(HaveOccurred())
			}

			g.Expect(obj.Status.Conditions).To(conditions.MatchConditions(tt.expectConditions))

			releases, _ = store.History(mockReleaseName)
			helmreleaseutil.SortByRevision(releases)

			if tt.expectCurrent != nil {
				g.Expect(obj.Status.Current).To(testutil.Equal(tt.expectCurrent(releases)))
			} else {
				g.Expect(obj.Status.Current).To(BeNil(), "expected current to be nil")
			}

			if tt.expectPrevious != nil {
				g.Expect(obj.Status.Previous).To(testutil.Equal(tt.expectPrevious(releases)))
			} else {
				g.Expect(obj.Status.Previous).To(BeNil(), "expected previous to be nil")
			}

			g.Expect(obj.Status.Failures).To(Equal(tt.expectFailures))
			g.Expect(obj.Status.InstallFailures).To(Equal(tt.expectInstallFailures))
			g.Expect(obj.Status.UpgradeFailures).To(Equal(tt.expectUpgradeFailures))
		})
	}
}

func Test_observeTest(t *testing.T) {
	t.Run("test with current", func(t *testing.T) {
		g := NewWithT(t)

		obj := &helmv2.HelmRelease{
			Status: helmv2.HelmReleaseStatus{
				Current: &helmv2.HelmReleaseInfo{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
				},
			},
		}
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
		}, testutil.ReleaseWithHooks(testHookFixtures))

		expect := release.ObservedToInfo(release.ObserveRelease(rls))
		expect.TestHooks = release.TestHooksFromRelease(rls)

		observeTest(obj)(rls)
		g.Expect(obj.Status.Current).To(Equal(expect))
		g.Expect(obj.Status.Previous).To(BeNil())
	})

	t.Run("test with different current version", func(t *testing.T) {
		g := NewWithT(t)

		current := &helmv2.HelmReleaseInfo{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
		}
		obj := &helmv2.HelmRelease{
			Status: helmv2.HelmReleaseStatus{
				Current: current,
			},
		}
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   2,
		}, testutil.ReleaseWithHooks(testHookFixtures))

		observeTest(obj)(rls)
		g.Expect(obj.Status.Current).To(Equal(current))
		g.Expect(obj.Status.Previous).To(BeNil())
	})

	t.Run("test without current", func(t *testing.T) {
		g := NewWithT(t)

		obj := &helmv2.HelmRelease{}

		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   2,
		}, testutil.ReleaseWithHooks(testHookFixtures))

		observeTest(obj)(rls)
		g.Expect(obj.Status.Current).To(BeNil())
		g.Expect(obj.Status.Previous).To(BeNil())
	})
}
