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
	"strings"
	"testing"
	"time"

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

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/digest"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
	"github.com/fluxcd/helm-controller/internal/testutil"
	"github.com/fluxcd/pkg/chartutil"
)

func TestRollbackRemediation_Reconcile(t *testing.T) {
	var (
		mockCreateErr = fmt.Errorf("storage create error")
		mockUpdateErr = fmt.Errorf("storage update error")
	)

	tests := []struct {
		name string
		// driver allows for modifying the Helm storage driver.
		driver func(driver helmdriver.Driver) helmdriver.Driver
		// releases is the list of releases that are stored in the driver
		// before rollback.
		releases func(namespace string) []*helmrelease.Release
		// spec modifies the HelmRelease object's spec before rollback.
		spec func(spec *v2.HelmReleaseSpec)
		// status to configure on the HelmRelease before rollback.
		status func(releases []*helmrelease.Release) v2.HelmReleaseStatus
		// wantErr is the error that is expected to be returned.
		wantErr error
		// expectedConditions are the conditions that are expected to be set on
		// the HelmRelease after rolling back.
		expectConditions []metav1.Condition
		// expectHistory is the expected History on the HelmRelease after
		// rolling back.
		expectHistory func(releases []*helmrelease.Release) v2.Snapshots
		// expectFailures is the expected Failures count on the HelmRelease.
		expectFailures int64
		// expectInstallFailures is the expected InstallFailures count on the
		// HelmRelease.
		expectInstallFailures int64
		// expectUpgradeFailures is the expected UpgradeFailures count on the
		// HelmRelease.
		expectUpgradeFailures int64
	}{
		{
			name: "rollback",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusSuperseded,
						Namespace: namespace,
					}),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Version:   2,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusFailed,
						Namespace: namespace,
					}),
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.RollbackSucceededReason, "succeeded"),
				*conditions.TrueCondition(v2.RemediatedCondition, v2.RollbackSucceededReason, "succeeded"),
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[2])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
		},
		{
			name: "rollback without previous target release",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusSuperseded,
						Namespace: namespace,
					}),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Version:   2,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusFailed,
						Namespace: namespace,
					}),
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
					},
				}
			},
			wantErr: ErrMissingRollbackTarget,
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
				}
			},
		},
		{
			name: "rollback failure",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Version:   1,
						Chart:     testutil.BuildChart(testutil.ChartWithFailingHook()),
						Status:    helmrelease.StatusSuperseded,
						Namespace: namespace,
					}, testutil.ReleaseWithFailingHook()),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Version:   2,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusFailed,
						Namespace: namespace,
					}),
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.RollbackFailedReason,
					"timed out waiting for the condition"),
				*conditions.FalseCondition(v2.RemediatedCondition, v2.RollbackFailedReason,
					"timed out waiting for the condition"),
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[2])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			expectFailures: 1,
		},
		{
			name: "rollback with storage create error",
			driver: func(driver helmdriver.Driver) helmdriver.Driver {
				return &storage.Failing{
					Driver:    driver,
					CreateErr: mockCreateErr,
				}
			},
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusSuperseded,
						Namespace: namespace,
					}),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Version:   2,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusFailed,
						Namespace: namespace,
					}),
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			wantErr: mockCreateErr,
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.RollbackFailedReason,
					mockCreateErr.Error()),
				*conditions.FalseCondition(v2.RemediatedCondition, v2.RollbackFailedReason,
					mockCreateErr.Error()),
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			expectFailures: 1,
		},
		{
			name: "rollback with storage update error",
			driver: func(driver helmdriver.Driver) helmdriver.Driver {
				return &storage.Failing{
					Driver:    driver,
					UpdateErr: mockUpdateErr,
				}
			},
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusSuperseded,
						Namespace: namespace,
					}),
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Version:   2,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusFailed,
						Namespace: namespace,
					}),
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.RollbackFailedReason,
					"storage update error"),
				*conditions.FalseCondition(v2.RemediatedCondition, v2.RollbackFailedReason,
					"storage update error"),
			},
			expectHistory: func(releases []*helmrelease.Release) v2.Snapshots {
				return v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(releases[2])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
					release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
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

			obj := &v2.HelmRelease{
				Spec: v2.HelmReleaseSpec{
					ReleaseName:      mockReleaseName,
					TargetNamespace:  releaseNamespace,
					StorageNamespace: releaseNamespace,
					Timeout:          &metav1.Duration{Duration: 100 * time.Millisecond},
				},
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
			got := (NewRollbackRemediation(cfg, recorder)).Reconcile(context.TODO(), &Request{
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

func TestRollbackRemediation_failure(t *testing.T) {
	var (
		prev = testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:    mockReleaseName,
			Chart:   testutil.BuildChart(),
			Version: 4,
		})
		obj = &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					release.ObservedToSnapshot(release.ObserveRelease(prev)),
				},
			},
		}
		err = errors.New("rollback error")
	)

	t.Run("records failure", func(t *testing.T) {
		g := NewWithT(t)

		recorder := testutil.NewFakeRecorder(10, false)
		r := &RollbackRemediation{
			eventRecorder: recorder,
		}
		req := &Request{Object: obj.DeepCopy()}
		r.failure(req, release.ObservedToSnapshot(release.ObserveRelease(prev)), nil, err)

		expectMsg := fmt.Sprintf(fmtRollbackRemediationFailure,
			fmt.Sprintf("%s/%s.v%d", prev.Namespace, prev.Name, prev.Version),
			fmt.Sprintf("%s@%s", prev.Chart.Name(), prev.Chart.Metadata.Version),
			strings.TrimSpace(err.Error()))

		g.Expect(req.Object.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
			*conditions.FalseCondition(v2.RemediatedCondition, v2.RollbackFailedReason, expectMsg),
		}))
		g.Expect(req.Object.Status.Failures).To(Equal(int64(1)))
		g.Expect(recorder.GetEvents()).To(ConsistOf([]corev1.Event{
			{
				Type:    corev1.EventTypeWarning,
				Reason:  v2.RollbackFailedReason,
				Message: expectMsg,
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						eventMetaGroupKey(eventv1.MetaRevisionKey): prev.Chart.Metadata.Version,
						eventMetaGroupKey(metaAppVersionKey):       prev.Chart.Metadata.AppVersion,
						eventMetaGroupKey(eventv1.MetaTokenKey):    chartutil.DigestValues(digest.Canonical, req.Values).String(),
					},
				},
			},
		}))
	})

	t.Run("records failure with logs", func(t *testing.T) {
		g := NewWithT(t)

		recorder := testutil.NewFakeRecorder(10, false)
		r := &RollbackRemediation{
			eventRecorder: recorder,
		}
		req := &Request{Object: obj.DeepCopy()}
		r.failure(req, release.ObservedToSnapshot(release.ObserveRelease(prev)), mockLogBuffer(5, 10), err)

		expectSubStr := "Last Helm logs"
		g.Expect(conditions.IsFalse(req.Object, v2.RemediatedCondition)).To(BeTrue())
		g.Expect(conditions.GetMessage(req.Object, v2.RemediatedCondition)).ToNot(ContainSubstring(expectSubStr))

		events := recorder.GetEvents()
		g.Expect(events).To(HaveLen(1))
		g.Expect(events[0].Message).To(ContainSubstring(expectSubStr))
	})
}

func TestRollbackRemediation_success(t *testing.T) {
	g := NewWithT(t)

	var prev = testutil.BuildRelease(&helmrelease.MockReleaseOptions{
		Name:    mockReleaseName,
		Chart:   testutil.BuildChart(),
		Version: 4,
	})

	recorder := testutil.NewFakeRecorder(10, false)
	r := &RollbackRemediation{
		eventRecorder: recorder,
	}
	req := &Request{Object: &v2.HelmRelease{}, Values: map[string]interface{}{"foo": "bar"}}
	r.success(req, release.ObservedToSnapshot(release.ObserveRelease(prev)))

	expectMsg := fmt.Sprintf(fmtRollbackRemediationSuccess,
		fmt.Sprintf("%s/%s.v%d", prev.Namespace, prev.Name, prev.Version),
		fmt.Sprintf("%s@%s", prev.Chart.Name(), prev.Chart.Metadata.Version))

	g.Expect(req.Object.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
		*conditions.TrueCondition(v2.RemediatedCondition, v2.RollbackSucceededReason, expectMsg),
	}))
	g.Expect(req.Object.Status.Failures).To(Equal(int64(0)))
	g.Expect(recorder.GetEvents()).To(ConsistOf([]corev1.Event{
		{
			Type:    corev1.EventTypeNormal,
			Reason:  v2.RollbackSucceededReason,
			Message: expectMsg,
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					eventMetaGroupKey(eventv1.MetaRevisionKey): prev.Chart.Metadata.Version,
					eventMetaGroupKey(metaAppVersionKey):       prev.Chart.Metadata.AppVersion,
					eventMetaGroupKey(eventv1.MetaTokenKey):    chartutil.DigestValues(digest.Canonical, req.Values).String(),
				},
			},
		},
	}))
}

func Test_observeRollback(t *testing.T) {
	t.Run("rollback", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{}
		rls := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   2,
			Status:    helmrelease.StatusPendingRollback,
		})
		observeRollback(obj)(rls)
		expect := release.ObservedToSnapshot(release.ObserveRelease(rls))

		g.Expect(obj.Status.History).To(testutil.Equal(v2.Snapshots{
			expect,
		}))
	})

	t.Run("rollback with latest", func(t *testing.T) {
		g := NewWithT(t)

		latest := &v2.Snapshot{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   2,
			Status:    helmrelease.StatusFailed.String(),
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					latest,
				},
			},
		}
		rls := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      latest.Name,
			Namespace: latest.Namespace,
			Version:   latest.Version + 1,
			Status:    helmrelease.StatusPendingRollback,
		})
		expect := release.ObservedToSnapshot(release.ObserveRelease(rls))

		observeRollback(obj)(rls)
		g.Expect(obj.Status.History).To(testutil.Equal(v2.Snapshots{
			expect,
			latest,
		}))
	})

	t.Run("rollback with update to previous deployed", func(t *testing.T) {
		g := NewWithT(t)

		previous := &v2.Snapshot{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   2,
			Status:    helmrelease.StatusFailed.String(),
		}
		latest := &v2.Snapshot{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   3,
			Status:    helmrelease.StatusDeployed.String(),
		}

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					latest,
					previous,
				},
			},
		}
		rls := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      previous.Name,
			Namespace: previous.Namespace,
			Version:   previous.Version,
			Status:    helmrelease.StatusSuperseded,
		})
		expect := release.ObservedToSnapshot(release.ObserveRelease(rls))

		observeRollback(obj)(rls)
		g.Expect(obj.Status.History).To(testutil.Equal(v2.Snapshots{
			latest,
			expect,
		}))
	})

	t.Run("rollback with update to previous deployed copies existing test hooks", func(t *testing.T) {
		g := NewWithT(t)

		previous := &v2.Snapshot{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   2,
			Status:    helmrelease.StatusFailed.String(),
			TestHooks: &map[string]*v2.TestHookStatus{
				"test-hook": {
					Phase: helmrelease.HookPhaseSucceeded.String(),
				},
			},
		}
		latest := &v2.Snapshot{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   3,
			Status:    helmrelease.StatusDeployed.String(),
		}

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					latest,
					previous,
				},
			},
		}
		rls := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      previous.Name,
			Namespace: previous.Namespace,
			Version:   previous.Version,
			Status:    helmrelease.StatusSuperseded,
		})
		expect := release.ObservedToSnapshot(release.ObserveRelease(rls))
		expect.SetTestHooks(previous.GetTestHooks())

		observeRollback(obj)(rls)
		g.Expect(obj.Status.History).To(testutil.Equal(v2.Snapshots{
			latest,
			expect,
		}))
	})

	t.Run("rollback with update to previous deployed with OCI Digest", func(t *testing.T) {
		g := NewWithT(t)

		previous := &v2.Snapshot{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   2,
			Status:    helmrelease.StatusFailed.String(),
			OCIDigest: "sha256:fcdc2b0de1581a3633ada4afee3f918f6eaa5b5ab38c3fef03d5b48d3f85d9f6",
		}
		latest := &v2.Snapshot{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   3,
			Status:    helmrelease.StatusDeployed.String(),
			OCIDigest: "sha256:aedc2b0de1576a3633ada4afee3f918f6eaa5b5ab38c3fef03d5b48d3f85d9f6",
		}

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				History: v2.Snapshots{
					latest,
					previous,
				},
			},
		}
		rls := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      previous.Name,
			Namespace: previous.Namespace,
			Version:   previous.Version,
			Status:    helmrelease.StatusSuperseded,
		})
		obs := release.ObserveRelease(rls)
		obs.OCIDigest = "sha256:fcdc2b0de1581a3633ada4afee3f918f6eaa5b5ab38c3fef03d5b48d3f85d9f6"
		expect := release.ObservedToSnapshot(obs)

		observeRollback(obj)(rls)
		g.Expect(obj.Status.History).To(testutil.Equal(v2.Snapshots{
			latest,
			expect,
		}))
	})
}
