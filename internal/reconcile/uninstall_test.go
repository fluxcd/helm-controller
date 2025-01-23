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
	"github.com/fluxcd/helm-controller/internal/digest"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
	"github.com/fluxcd/helm-controller/internal/testutil"
	"github.com/fluxcd/pkg/chartutil"
)

func TestUninstall_Reconcile(t *testing.T) {
	mockUpdateErr := errors.New("mock update error")

	tests := []struct {
		name string
		// driver allows for modifying the Helm storage driver.
		driver func(helmdriver.Driver) helmdriver.Driver
		// releases is the list of releases that are stored in the driver
		// before uninstall.
		releases func(namespace string) []*helmrelease.Release
		// spec modifies the HelmRelease Object spec before uninstall.
		spec func(spec *v2.HelmReleaseSpec)
		// status to configure on the HelmRelease Object before uninstall.
		status func(releases []*helmrelease.Release) v2.HelmReleaseStatus
		// wantErr is the error that is expected to be returned.
		wantErr error
		// wantErrString is the error string that is expected to be in the
		// returned error. This is used for scenarios that return
		// untyped/unwrapped error that can't be asserted for their value.
		wantErrString string
		// expectedConditions are the conditions that are expected to be set on
		// the HelmRelease after running rollback.
		expectConditions []metav1.Condition
		// expectHistory is the expected History of the HelmRelease after
		// uninstall.
		expectHistory func(namespace string, releases []*helmrelease.Release) v2.Snapshots
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
			name: "uninstall success",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
						Status:    helmrelease.StatusDeployed,
					}),
				}
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Uninstall = &v2.Uninstall{
					KeepHistory: true,
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallSucceededReason,
					"succeeded"),
				*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallSucceededReason,
					"succeeded"),
			},
			expectHistory: func(namespace string, releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "uninstall failure",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Status:    helmrelease.StatusDeployed,
						Chart:     testutil.BuildChart(testutil.ChartWithFailingHook()),
					}, testutil.ReleaseWithFailingHook()),
				}
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Uninstall = &v2.Uninstall{
					KeepHistory: true,
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallFailedReason,
					"uninstallation completed with 1 error(s): 1 error occurred:\n\t* timed out waiting for the condition"),
				*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallFailedReason,
					"uninstallation completed with 1 error(s): 1 error occurred:\n\t* timed out waiting for the condition"),
			},
			expectHistory: func(namespace string, releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			expectFailures: 1,
			wantErrString:  "timed out waiting",
		},
		{
			name: "uninstall failure without storage update",
			driver: func(driver helmdriver.Driver) helmdriver.Driver {
				return &storage.Failing{
					// Explicitly inherit the driver, as we want to rely on the
					// Secret storage, as the memory storage does not detach
					// objects from the release action. Causing writes post-persist
					// to leak to the stored release object.
					// xref: https://github.com/helm/helm/issues/11304
					Driver:    driver,
					UpdateErr: mockUpdateErr,
				}
			},
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
						Status:    helmrelease.StatusDeployed,
					}),
				}
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Uninstall = &v2.Uninstall{
					KeepHistory: true,
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallFailedReason,
					ErrNoStorageUpdate.Error()),
				*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallFailedReason,
					ErrNoStorageUpdate.Error()),
			},
			expectHistory: func(namespace string, releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			expectFailures: 1,
			wantErr:        ErrNoStorageUpdate,
		},
		{
			name: "uninstall failure without storage delete",
			driver: func(driver helmdriver.Driver) helmdriver.Driver {
				return &storage.Failing{
					// Explicitly inherit the driver, as we want to rely on the
					// Secret storage, as the memory storage does not detach
					// objects from the release action. Causing writes post-persist
					// to leak to the stored release object.
					// xref: https://github.com/helm/helm/issues/11304
					Driver:    driver,
					DeleteErr: fmt.Errorf("delete error"),
				}
			},
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
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallFailedReason,
					"delete error"),
				*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallFailedReason,
					"delete error"),
			},
			expectHistory: func(namespace string, releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			expectFailures: 1,
			wantErrString:  "Failed to purge the release",
		},
		{
			name: "uninstall without current",
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
			expectConditions: []metav1.Condition{},
			wantErr:          ErrNoLatest,
		},
		{
			name: "uninstall with stale current",
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
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Uninstall = &v2.Uninstall{
					KeepHistory: true,
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallFailedReason,
					ErrReleaseMismatch.Error()),
				*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallFailedReason,
					ErrReleaseMismatch.Error()),
			},
			expectHistory: func(namespace string, releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			expectFailures: 1,
			wantErr:        ErrReleaseMismatch,
		},
		{
			name: "uninstall already deleted release",
			driver: func(driver helmdriver.Driver) helmdriver.Driver {
				return &storage.Failing{
					// Explicitly inherit the driver, as we want to rely on the
					// Secret storage, as the memory storage does not detach
					// objects from the release action. Causing writes post-persist
					// to leak to the stored release object.
					// xref: https://github.com/helm/helm/issues/11304
					Driver:   driver,
					QueryErr: helmdriver.ErrReleaseNotFound,
				}
			},
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
						Status:    helmrelease.StatusDeployed,
					}),
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallSucceededReason,
					"assuming it is uninstalled"),
				*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallSucceededReason,
					"assuming it is uninstalled"),
			},
			expectHistory: func(namespace string, releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "already uninstalled without keep history",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
						Status:    helmrelease.StatusUninstalled,
					}),
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallSucceededReason,
					"succeeded"),
				*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallSucceededReason,
					"succeeded"),
			},
			expectHistory: func(namespace string, releases []*helmrelease.Release) v2.Snapshots {
				rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: namespace,
					Version:   1,
					Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
					Status:    helmrelease.StatusUninstalled,
				})
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(rls)),
				}
			},
		},
		{
			name: "already uninstalled with keep history",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(testutil.ChartWithTestHook()),
						Status:    helmrelease.StatusUninstalled,
					}),
				}
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Uninstall = &v2.Uninstall{
					KeepHistory: true,
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.UninstallSucceededReason,
					"was already uninstalled"),
				*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallSucceededReason,
					"was already uninstalled"),
			},
			expectHistory: func(namespace string, releases []*helmrelease.Release) v2.Snapshots {
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
			got := NewUninstall(cfg, recorder).Reconcile(context.TODO(), &Request{
				Object: obj,
			})
			if tt.wantErr != nil {
				g.Expect(errors.Is(got, tt.wantErr)).To(BeTrue())
			} else if tt.wantErrString != "" {
				g.Expect(got.Error()).To(ContainSubstring(tt.wantErrString))
			} else {
				g.Expect(got).ToNot(HaveOccurred())
			}

			g.Expect(obj.Status.Conditions).To(conditions.MatchConditions(tt.expectConditions))

			releases, _ = store.History(mockReleaseName)
			releaseutil.SortByRevision(releases)

			if tt.expectHistory != nil {
				g.Expect(obj.Status.History).To(testutil.Equal(tt.expectHistory(releaseNamespace, releases)))
			} else {
				g.Expect(obj.Status.History).To(BeEmpty(), "expected history to be empty")
			}

			g.Expect(obj.Status.Failures).To(Equal(tt.expectFailures))
			g.Expect(obj.Status.InstallFailures).To(Equal(tt.expectInstallFailures))
			g.Expect(obj.Status.UpgradeFailures).To(Equal(tt.expectUpgradeFailures))
		})
	}
}

func TestUninstall_failure(t *testing.T) {
	var (
		cur = testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:    mockReleaseName,
			Chart:   testutil.BuildChart(),
			Version: 4,
		})
		obj = &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(cur)),
				},
			},
		}
		err = errors.New("uninstall error")
	)

	t.Run("records failure", func(t *testing.T) {
		g := NewWithT(t)

		recorder := testutil.NewFakeRecorder(10, false)
		r := &Uninstall{
			eventRecorder: recorder,
		}

		req := &Request{Object: obj.DeepCopy()}
		r.failure(req, nil, err)

		expectMsg := fmt.Sprintf(fmtUninstallFailure,
			fmt.Sprintf("%s/%s.v%d", cur.Namespace, cur.Name, cur.Version),
			fmt.Sprintf("%s@%s", cur.Chart.Name(), cur.Chart.Metadata.Version),
			err.Error())

		g.Expect(req.Object.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallFailedReason, expectMsg),
		}))
		g.Expect(req.Object.Status.Failures).To(Equal(int64(1)))
		g.Expect(recorder.GetEvents()).To(ConsistOf([]corev1.Event{
			{
				Type:    corev1.EventTypeWarning,
				Reason:  v2.UninstallFailedReason,
				Message: expectMsg,
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						eventMetaGroupKey(eventv1.MetaRevisionKey): cur.Chart.Metadata.Version,
						eventMetaGroupKey(metaAppVersionKey):       cur.Chart.Metadata.AppVersion,
						eventMetaGroupKey(eventv1.MetaTokenKey):    chartutil.DigestValues(digest.Canonical, cur.Config).String(),
					},
				},
			},
		}))
	})

	t.Run("records failure with logs", func(t *testing.T) {
		g := NewWithT(t)

		recorder := testutil.NewFakeRecorder(10, false)
		r := &Uninstall{
			eventRecorder: recorder,
		}
		req := &Request{Object: obj.DeepCopy()}
		r.failure(req, mockLogBuffer(5, 10), err)

		expectSubStr := "Last Helm logs"
		g.Expect(conditions.IsFalse(req.Object, v2.ReleasedCondition)).To(BeTrue())
		g.Expect(conditions.GetMessage(req.Object, v2.ReleasedCondition)).ToNot(ContainSubstring(expectSubStr))

		events := recorder.GetEvents()
		g.Expect(events).To(HaveLen(1))
		g.Expect(events[0].Message).To(ContainSubstring(expectSubStr))
	})
}

func TestUninstall_success(t *testing.T) {
	g := NewWithT(t)

	var cur = testutil.BuildRelease(&helmrelease.MockReleaseOptions{
		Name:      mockReleaseName,
		Namespace: mockReleaseNamespace,
		Chart:     testutil.BuildChart(),
		Version:   4,
	})

	recorder := testutil.NewFakeRecorder(10, false)
	r := &Uninstall{
		eventRecorder: recorder,
	}

	obj := &v2.HelmRelease{
		Status: v2.HelmReleaseStatus{
			History: v2.Snapshots{
				release.ObservedToSnapshot(release.ObserveRelease(cur)),
			},
		},
	}
	req := &Request{Object: obj}
	r.success(req)

	expectMsg := fmt.Sprintf(fmtUninstallSuccess,
		fmt.Sprintf("%s/%s.v%d", cur.Namespace, cur.Name, cur.Version),
		fmt.Sprintf("%s@%s", cur.Chart.Name(), cur.Chart.Metadata.Version))

	g.Expect(req.Object.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
		*conditions.FalseCondition(v2.ReleasedCondition, v2.UninstallSucceededReason, expectMsg),
	}))
	g.Expect(req.Object.Status.Failures).To(Equal(int64(0)))
	g.Expect(recorder.GetEvents()).To(ConsistOf([]corev1.Event{
		{
			Type:    corev1.EventTypeNormal,
			Reason:  v2.UninstallSucceededReason,
			Message: expectMsg,
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					eventMetaGroupKey(eventv1.MetaRevisionKey): cur.Chart.Metadata.Version,
					eventMetaGroupKey(metaAppVersionKey):       cur.Chart.Metadata.AppVersion,
					eventMetaGroupKey(eventv1.MetaTokenKey):    chartutil.DigestValues(digest.Canonical, cur.Config).String(),
				},
			},
		},
	}))
}

func Test_observeUninstall(t *testing.T) {
	t.Run("uninstall of current", func(t *testing.T) {
		g := NewWithT(t)

		current := &v2.Snapshot{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusDeployed.String(),
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					current,
				},
			},
		}
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      current.Name,
			Namespace: current.Namespace,
			Version:   current.Version,
			Status:    helmrelease.StatusUninstalled,
		})
		expect := release.ObservedToSnapshot(release.ObserveRelease(rls))

		observeUninstall(obj)(rls)
		g.Expect(obj.Status.History).To(testutil.Equal(v2.Snapshots{
			expect,
		}))
	})

	t.Run("uninstall without current", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: nil,
			},
		}
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusUninstalling,
		})

		observeUninstall(obj)(rls)
		g.Expect(obj.Status.History).To(BeNil())
	})

	t.Run("uninstall of different version than current", func(t *testing.T) {
		g := NewWithT(t)

		current := &v2.Snapshot{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusDeployed.String(),
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					current,
				},
			},
		}
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      current.Name,
			Namespace: current.Namespace,
			Version:   current.Version + 1,
			Status:    helmrelease.StatusUninstalled,
		})

		observeUninstall(obj)(rls)
		g.Expect(obj.Status.History).To(testutil.Equal(v2.Snapshots{
			current,
		}))
	})
	t.Run("uninstall of current with OCI Digest", func(t *testing.T) {
		g := NewWithT(t)

		current := &v2.Snapshot{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusDeployed.String(),
			OCIDigest: "sha256:fcdc2b0de1581a3633ada4afee3f918f6eaa5b5ab38c3fef03d5b48d3f85d9f6",
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					current,
				},
			},
		}
		rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:      current.Name,
			Namespace: current.Namespace,
			Version:   current.Version,
			Status:    helmrelease.StatusUninstalled,
		})
		obs := release.ObserveRelease(rls)
		obs.OCIDigest = "sha256:fcdc2b0de1581a3633ada4afee3f918f6eaa5b5ab38c3fef03d5b48d3f85d9f6"
		expect := release.ObservedToSnapshot(obs)

		observeUninstall(obj)(rls)
		g.Expect(obj.Status.History).To(testutil.Equal(v2.Snapshots{
			expect,
		}))
	})
}
