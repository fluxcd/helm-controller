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
	"fmt"
	"testing"
	"time"

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmreleaseutil "helm.sh/helm/v3/pkg/releaseutil"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/chartutil"
	"github.com/fluxcd/helm-controller/internal/digest"
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
		spec func(spec *v2.HelmReleaseSpec)
		// status to configure on the HelmRelease Object before test.
		status func(releases []*helmrelease.Release) v2.HelmReleaseStatus
		// wantErr is the error that is expected to be returned.
		wantErr error
		// expectedConditions are the conditions that are expected to be set on
		// the HelmRelease after running rollback.
		expectConditions []metav1.Condition
		// expectCurrent is the expected Current release information in the
		// HelmRelease after install.
		expectCurrent func(releases []*helmrelease.Release) *v2.HelmReleaseInfo
		// expectPrevious returns the expected Previous release information of
		// the HelmRelease after install.
		expectPrevious func(releases []*helmrelease.Release) *v2.HelmReleaseInfo
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
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					Current: release.ObservedToInfo(release.ObserveRelease(releases[0])),
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.TrueCondition(meta.ReadyCondition, v2.TestSucceededReason,
					"1 test hook completed successfully"),
				*conditions.TrueCondition(v2.TestSuccessCondition, v2.TestSucceededReason,
					"1 test hook completed successfully"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				info := release.ObservedToInfo(release.ObserveRelease(releases[0]))
				info.SetTestHooks(release.TestHooksFromRelease(releases[0]))
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
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					Current: release.ObservedToInfo(release.ObserveRelease(releases[0])),
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.TrueCondition(meta.ReadyCondition, v2.TestSucceededReason,
					"no test hooks"),
				*conditions.TrueCondition(v2.TestSuccessCondition, v2.TestSucceededReason,
					"no test hooks"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				info := release.ObservedToInfo(release.ObserveRelease(releases[0]))
				info.SetTestHooks(release.TestHooksFromRelease(releases[0]))
				return info
			},
		},
		{
			name: "test install failure",
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
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					Current:         release.ObservedToInfo(release.ObserveRelease(releases[0])),
					InstallFailures: 0,
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.TestFailedReason,
					"timed out waiting for the condition"),
				*conditions.FalseCondition(v2.TestSuccessCondition, v2.TestFailedReason,
					"timed out waiting for the condition"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				info := release.ObservedToInfo(release.ObserveRelease(releases[0]))
				info.SetTestHooks(release.TestHooksFromRelease(releases[0]))
				return info
			},
			expectFailures:        1,
			expectInstallFailures: 1,
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
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					Current: release.ObservedToInfo(release.ObserveRelease(releases[0])),
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.TestFailedReason,
					ErrReleaseMismatch.Error()),
				*conditions.FalseCondition(v2.TestSuccessCondition, v2.TestFailedReason,
					ErrReleaseMismatch.Error()),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[0]))
			},
			expectFailures: 1,
			wantErr:        ErrReleaseMismatch,
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

			obj := &v2.HelmRelease{
				Spec: v2.HelmReleaseSpec{
					ReleaseName:      mockReleaseName,
					TargetNamespace:  releaseNamespace,
					StorageNamespace: releaseNamespace,
					Timeout:          &metav1.Duration{Duration: 100 * time.Millisecond},
					Test: &v2.Test{
						Enable: true,
					},
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

			recorder := new(record.FakeRecorder)
			got := (NewTest(cfg, recorder)).Reconcile(context.TODO(), &Request{
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
				g.Expect(obj.GetCurrent()).To(testutil.Equal(tt.expectCurrent(releases)))
			} else {
				g.Expect(obj.GetCurrent()).To(BeNil(), "expected current to be nil")
			}

			if tt.expectPrevious != nil {
				g.Expect(obj.GetPrevious()).To(testutil.Equal(tt.expectPrevious(releases)))
			} else {
				g.Expect(obj.GetPrevious()).To(BeNil(), "expected previous to be nil")
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

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				Current: &v2.HelmReleaseInfo{
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
		expect.SetTestHooks(release.TestHooksFromRelease(rls))

		observeTest(obj)(rls)
		g.Expect(obj.GetCurrent()).To(Equal(expect))
		g.Expect(obj.GetPrevious()).To(BeNil())
	})

	t.Run("test with different current version", func(t *testing.T) {
		g := NewWithT(t)

		current := &v2.HelmReleaseInfo{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				Current: current,
			},
		}
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   2,
		}, testutil.ReleaseWithHooks(testHookFixtures))

		observeTest(obj)(rls)
		g.Expect(obj.GetCurrent()).To(Equal(current))
		g.Expect(obj.GetPrevious()).To(BeNil())
	})

	t.Run("test without current", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{}

		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   2,
		}, testutil.ReleaseWithHooks(testHookFixtures))

		observeTest(obj)(rls)
		g.Expect(obj.GetCurrent()).To(BeNil())
		g.Expect(obj.GetPrevious()).To(BeNil())
	})
}

func TestTest_failure(t *testing.T) {
	var (
		cur = testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Chart:     testutil.BuildChart(),
			Version:   4,
		})
		obj = &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				Current: release.ObservedToInfo(release.ObserveRelease(cur)),
			},
		}
		err = errors.New("test error")
	)

	t.Run("records failure", func(t *testing.T) {
		g := NewWithT(t)

		recorder := testutil.NewFakeRecorder(10, false)
		r := &Test{
			eventRecorder: recorder,
		}

		req := &Request{Object: obj.DeepCopy()}
		r.failure(req, nil, err)

		expectMsg := fmt.Sprintf(fmtTestFailure,
			fmt.Sprintf("%s/%s.%d", cur.Namespace, cur.Name, cur.Version),
			fmt.Sprintf("%s@%s", cur.Chart.Name(), cur.Chart.Metadata.Version),
			err.Error())

		g.Expect(req.Object.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.FalseCondition(v2.TestSuccessCondition, v2.TestFailedReason, expectMsg),
		}))
		g.Expect(req.Object.Status.Failures).To(Equal(int64(1)))
		g.Expect(req.Object.Status.InstallFailures).To(BeZero())
		g.Expect(req.Object.Status.UpgradeFailures).To(BeZero())
		g.Expect(recorder.GetEvents()).To(ConsistOf([]corev1.Event{
			{
				Type:    corev1.EventTypeWarning,
				Reason:  v2.TestFailedReason,
				Message: expectMsg,
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						eventMetaGroupKey(eventv1.MetaRevisionKey): cur.Chart.Metadata.Version,
						eventMetaGroupKey(eventv1.MetaTokenKey):    chartutil.DigestValues(digest.Canonical, cur.Config).String(),
					},
				},
			},
		}))
	})

	t.Run("records failure with logs", func(t *testing.T) {
		g := NewWithT(t)

		recorder := testutil.NewFakeRecorder(10, false)
		r := &Test{
			eventRecorder: recorder,
		}
		req := &Request{Object: obj.DeepCopy()}
		r.failure(req, mockLogBuffer(5, 10), err)

		expectSubStr := "Last Helm logs"
		g.Expect(conditions.IsFalse(req.Object, v2.TestSuccessCondition)).To(BeTrue())
		g.Expect(conditions.GetMessage(req.Object, v2.TestSuccessCondition)).ToNot(ContainSubstring(expectSubStr))

		events := recorder.GetEvents()
		g.Expect(events).To(HaveLen(1))
		g.Expect(events[0].Message).To(ContainSubstring(expectSubStr))
	})

	t.Run("increases remediation failure count", func(t *testing.T) {
		g := NewWithT(t)

		recorder := testutil.NewFakeRecorder(10, false)
		r := &Test{
			eventRecorder: recorder,
		}

		obj := obj.DeepCopy()
		obj.Status.Current.SetTestHooks(map[string]*v2.HelmReleaseTestHook{})
		req := &Request{Object: obj}
		r.failure(req, nil, err)

		g.Expect(req.Object.Status.InstallFailures).To(Equal(int64(1)))
	})

	t.Run("follows ignore failure instructions", func(t *testing.T) {
		g := NewWithT(t)

		recorder := testutil.NewFakeRecorder(10, false)
		r := &Test{
			eventRecorder: recorder,
		}

		obj := obj.DeepCopy()
		obj.Spec.Test = &v2.Test{IgnoreFailures: true}
		obj.Status.Current.SetTestHooks(map[string]*v2.HelmReleaseTestHook{})
		req := &Request{Object: obj}
		r.failure(req, nil, err)

		g.Expect(req.Object.Status.InstallFailures).To(BeZero())
	})
}

func TestTest_success(t *testing.T) {
	g := NewWithT(t)

	var (
		cur = testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Chart:     testutil.BuildChart(),
			Version:   4,
		})
		obj = &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				Current: release.ObservedToInfo(release.ObserveRelease(cur)),
			},
		}
	)

	t.Run("records success", func(t *testing.T) {
		recorder := testutil.NewFakeRecorder(10, false)
		r := &Test{
			eventRecorder: recorder,
		}

		obj := obj.DeepCopy()
		obj.Status.Current.SetTestHooks(map[string]*v2.HelmReleaseTestHook{
			"test": {
				Phase: helmrelease.HookPhaseSucceeded.String(),
			},
			"test-2": {
				Phase: helmrelease.HookPhaseSucceeded.String(),
			},
		})
		req := &Request{Object: obj}
		r.success(req)

		expectMsg := fmt.Sprintf(fmtTestSuccess,
			fmt.Sprintf("%s/%s.%d", cur.Namespace, cur.Name, cur.Version),
			fmt.Sprintf("%s@%s", cur.Chart.Name(), cur.Chart.Metadata.Version),
			"2 test hooks completed successfully")

		g.Expect(req.Object.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(v2.TestSuccessCondition, v2.TestSucceededReason, expectMsg),
		}))
		g.Expect(req.Object.Status.Failures).To(Equal(int64(0)))
		g.Expect(recorder.GetEvents()).To(ConsistOf([]corev1.Event{
			{
				Type:    corev1.EventTypeNormal,
				Reason:  v2.TestSucceededReason,
				Message: expectMsg,
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						eventMetaGroupKey(eventv1.MetaRevisionKey): cur.Chart.Metadata.Version,
						eventMetaGroupKey(eventv1.MetaTokenKey):    chartutil.DigestValues(digest.Canonical, cur.Config).String(),
					},
				},
			},
		}))
	})

	t.Run("records success without hooks", func(t *testing.T) {
		r := &Test{
			eventRecorder: new(testutil.FakeRecorder),
		}

		obj := obj.DeepCopy()
		obj.Status.Current.SetTestHooks(map[string]*v2.HelmReleaseTestHook{})
		req := &Request{Object: obj}
		r.success(req)

		g.Expect(conditions.IsTrue(req.Object, v2.TestSuccessCondition)).To(BeTrue())
		g.Expect(req.Object.Status.Conditions[0].Message).To(ContainSubstring("no test hooks"))
	})
}
