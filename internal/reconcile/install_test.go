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

	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/chart"
	helmchartutil "helm.sh/helm/v3/pkg/chartutil"
	helmrelease "helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/chartutil"
	"github.com/fluxcd/helm-controller/internal/digest"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

func TestInstall_Reconcile(t *testing.T) {
	tests := []struct {
		name string
		// driver allows for modifying the Helm storage driver.
		driver func(driver helmdriver.Driver) helmdriver.Driver
		// releases is the list of releases that are stored in the driver
		// before install.
		releases func(namespace string) []*helmrelease.Release
		// chart to install.
		chart *chart.Chart
		// values to use during install.
		values helmchartutil.Values
		// spec modifies the HelmRelease object spec before install.
		spec func(spec *v2.HelmReleaseSpec)
		// status to configure on the HelmRelease object before install.
		status func(releases []*helmrelease.Release) v2.HelmReleaseStatus
		// wantErr is the error that is expected to be returned.
		wantErr error
		// expectedConditions are the conditions that are expected to be set on
		// the HelmRelease after install.
		expectConditions []metav1.Condition
		// expectHistory is the expected History of the HelmRelease after
		// install.
		expectHistory func(releases []*helmrelease.Release) v2.Snapshots
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
			name:  "install success",
			chart: testutil.BuildChart(),
			expectConditions: []metav1.Condition{
				*conditions.TrueCondition(meta.ReadyCondition, v2.InstallSucceededReason,
					"Helm install succeeded"),
				*conditions.TrueCondition(v2.ReleasedCondition, v2.InstallSucceededReason,
					"Helm install succeeded"),
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name:  "install failure",
			chart: testutil.BuildChart(testutil.ChartWithFailingHook()),
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.InstallFailedReason,
					"failed post-install"),
				*conditions.FalseCondition(v2.ReleasedCondition, v2.InstallFailedReason,
					"failed post-install"),
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			expectFailures:        1,
			expectInstallFailures: 1,
		},
		{
			name: "install failure without storage update",
			driver: func(driver helmdriver.Driver) helmdriver.Driver {
				return &storage.Failing{
					Driver:    driver,
					CreateErr: fmt.Errorf("storage create error"),
				}
			},
			chart:   testutil.BuildChart(),
			wantErr: fmt.Errorf("storage create error"),
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.InstallFailedReason,
					"storage create error"),
				*conditions.FalseCondition(v2.ReleasedCondition, v2.InstallFailedReason,
					"storage create error"),
			},
			expectFailures:        1,
			expectInstallFailures: 0,
		},
		{
			name: "install with current",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Chart:     testutil.BuildChart(),
						Version:   1,
						Status:    helmrelease.StatusUninstalled,
					}),
				}
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Install = &v2.Install{
					Replace: true,
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart: testutil.BuildChart(),
			expectConditions: []metav1.Condition{
				*conditions.TrueCondition(meta.ReadyCondition, v2.InstallSucceededReason,
					"Helm install succeeded"),
				*conditions.TrueCondition(v2.ReleasedCondition, v2.InstallSucceededReason,
					"Helm install succeeded"),
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
				}
			},
		},
		{
			name: "install with stale current",
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(testutil.BuildRelease(&helmrelease.MockReleaseOptions{
							Name:      mockReleaseName,
							Namespace: "other",
							Version:   1,
							Status:    helmrelease.StatusUninstalled,
							Chart:     testutil.BuildChart(),
						}))),
					},
				}
			},
			chart: testutil.BuildChart(),
			expectConditions: []metav1.Condition{
				*conditions.TrueCondition(meta.ReadyCondition, v2.InstallSucceededReason,
					"Helm install succeeded"),
				*conditions.TrueCondition(v2.ReleasedCondition, v2.InstallSucceededReason,
					"Helm install succeeded"),
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "install with stale conditions",
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					Conditions: []metav1.Condition{
						*conditions.FalseCondition(v2.TestSuccessCondition, v2.TestFailedReason, ""),
						*conditions.TrueCondition(v2.RemediatedCondition, v2.UninstallSucceededReason, ""),
					},
				}
			},
			chart: testutil.BuildChart(),
			expectConditions: []metav1.Condition{
				*conditions.TrueCondition(meta.ReadyCondition, v2.InstallSucceededReason,
					"Helm install succeeded"),
				*conditions.TrueCondition(v2.ReleasedCondition, v2.InstallSucceededReason,
					"Helm install succeeded"),
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
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
				obj.Status = tt.status(releases)
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

			if tt.driver != nil {
				cfg.Driver = tt.driver(cfg.Driver)
			}

			recorder := new(record.FakeRecorder)
			got := (NewInstall(cfg, recorder)).Reconcile(context.TODO(), &Request{
				Object: obj,
				Chart:  tt.chart,
				Values: tt.values,
			})
			if tt.wantErr != nil {
				g.Expect(got).To(Equal(tt.wantErr))
			} else {
				g.Expect(got).ToNot(HaveOccurred())
			}

			g.Expect(obj.Status.Conditions).To(conditions.MatchConditions(tt.expectConditions))

			releases, _ = store.History(mockReleaseName)
			releaseutil.SortByRevision(releases)

			if tt.expectHistory != nil {
				g.Expect(obj.Status.History).To(testutil.Equal(tt.expectHistory(releases)))
			} else {
				g.Expect(obj.Status.History).To(BeEmpty(), "expected history to be empty")
			}

			g.Expect(obj.Status.Failures).To(Equal(tt.expectFailures))
			g.Expect(obj.Status.InstallFailures).To(Equal(tt.expectInstallFailures))
			g.Expect(obj.Status.UpgradeFailures).To(Equal(tt.expectUpgradeFailures))
		})
	}
}

func TestInstall_failure(t *testing.T) {
	var (
		obj = &v2.HelmRelease{
			Spec: v2.HelmReleaseSpec{
				ReleaseName:     mockReleaseName,
				TargetNamespace: mockReleaseNamespace,
			},
			Status: v2.HelmReleaseStatus{
				LastAttemptedRevisionDigest: "sha256:1234567890",
			},
		}
		chrt = testutil.BuildChart()
		err  = errors.New("installation error")
	)

	t.Run("records failure", func(t *testing.T) {
		g := NewWithT(t)

		recorder := testutil.NewFakeRecorder(10, false)
		r := &Install{
			eventRecorder: recorder,
		}

		req := &Request{Object: obj.DeepCopy(), Chart: chrt, Values: map[string]interface{}{"foo": "bar"}}
		r.failure(req, nil, err)

		expectMsg := fmt.Sprintf(fmtInstallFailure, mockReleaseNamespace, mockReleaseName, chrt.Name(),
			chrt.Metadata.Version, err.Error())

		g.Expect(req.Object.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.FalseCondition(v2.ReleasedCondition, v2.InstallFailedReason, expectMsg),
		}))
		g.Expect(req.Object.Status.Failures).To(Equal(int64(1)))
		g.Expect(recorder.GetEvents()).To(ConsistOf([]corev1.Event{
			{
				Type:    corev1.EventTypeWarning,
				Reason:  v2.InstallFailedReason,
				Message: expectMsg,
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						eventMetaGroupKey(metaOCIDigestKey):        obj.Status.LastAttemptedRevisionDigest,
						eventMetaGroupKey(eventv1.MetaRevisionKey): chrt.Metadata.Version,
						eventMetaGroupKey(metaAppVersionKey):       chrt.Metadata.AppVersion,
						eventMetaGroupKey(eventv1.MetaTokenKey):    chartutil.DigestValues(digest.Canonical, req.Values).String(),
					},
				},
			},
		}))
	})

	t.Run("records failure with logs", func(t *testing.T) {
		g := NewWithT(t)

		recorder := testutil.NewFakeRecorder(10, false)
		r := &Install{
			eventRecorder: recorder,
		}
		req := &Request{Object: obj.DeepCopy(), Chart: chrt}
		r.failure(req, mockLogBuffer(5, 10), err)

		expectSubStr := "Last Helm logs"
		g.Expect(conditions.IsFalse(req.Object, v2.ReleasedCondition)).To(BeTrue())
		g.Expect(conditions.GetMessage(req.Object, v2.ReleasedCondition)).ToNot(ContainSubstring(expectSubStr))

		events := recorder.GetEvents()
		g.Expect(events).To(HaveLen(1))
		g.Expect(events[0].Message).To(ContainSubstring(expectSubStr))
	})
}

func TestInstall_success(t *testing.T) {
	var (
		cur = testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Chart:     testutil.BuildChart(),
		})
		obj = &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(cur)),
				},
			},
		}
	)

	t.Run("records success", func(t *testing.T) {
		g := NewWithT(t)

		recorder := testutil.NewFakeRecorder(10, false)
		r := &Install{
			eventRecorder: recorder,
		}

		req := &Request{
			Object: obj.DeepCopy(),
		}
		r.success(req)

		expectMsg := fmt.Sprintf(fmtInstallSuccess,
			fmt.Sprintf("%s/%s.v%d", mockReleaseNamespace, mockReleaseName, obj.Status.History.Latest().Version),
			fmt.Sprintf("%s@%s", obj.Status.History.Latest().ChartName, obj.Status.History.Latest().ChartVersion))

		g.Expect(req.Object.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.TrueCondition(v2.ReleasedCondition, v2.InstallSucceededReason, expectMsg),
		}))
		g.Expect(recorder.GetEvents()).To(ConsistOf([]corev1.Event{
			{
				Type:    corev1.EventTypeNormal,
				Reason:  v2.InstallSucceededReason,
				Message: expectMsg,
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						eventMetaGroupKey(eventv1.MetaRevisionKey): obj.Status.History.Latest().ChartVersion,
						eventMetaGroupKey(metaAppVersionKey):       obj.Status.History.Latest().AppVersion,
						eventMetaGroupKey(eventv1.MetaTokenKey):    obj.Status.History.Latest().ConfigDigest,
					},
				},
			},
		}))
	})

	t.Run("records success with TestSuccess=False", func(t *testing.T) {
		g := NewWithT(t)

		recorder := testutil.NewFakeRecorder(10, false)
		r := &Install{
			eventRecorder: recorder,
		}

		obj := obj.DeepCopy()
		obj.Spec.Test = &v2.Test{Enable: true}

		req := &Request{Object: obj}
		r.success(req)

		g.Expect(conditions.IsTrue(req.Object, v2.ReleasedCondition)).To(BeTrue())

		cond := conditions.Get(req.Object, v2.TestSuccessCondition)
		g.Expect(cond).ToNot(BeNil())

		expectMsg := fmt.Sprintf(fmtTestPending,
			fmt.Sprintf("%s/%s.v%d", mockReleaseNamespace, mockReleaseName, obj.Status.History.Latest().Version),
			fmt.Sprintf("%s@%s", obj.Status.History.Latest().ChartName, obj.Status.History.Latest().ChartVersion))
		g.Expect(cond.Message).To(Equal(expectMsg))
	})
}
