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
	"github.com/fluxcd/helm-controller/internal/storage"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

func TestUnlock_Reconcile(t *testing.T) {
	var (
		mockQueryErr  = errors.New("storage query error")
		mockUpdateErr = errors.New("storage update error")
	)

	tests := []struct {
		name string
		// driver allows for modifying the Helm storage driver.
		driver func(helmdriver.Driver) helmdriver.Driver
		// releases is the list of releases that are stored in the driver
		// before unlock.
		releases func(namespace string) []*helmrelease.Release
		// spec modifies the HelmRelease Object spec before unlock.
		spec func(spec *v2.HelmReleaseSpec)
		// status to configure on the HelmRelease object before unlock.
		status func(releases []*helmrelease.Release) v2.HelmReleaseStatus
		// wantErr is the error that is expected to be returned.
		wantErr error
		// expectedConditions are the conditions that are expected to be set on
		// the HelmRelease after running rollback.
		expectConditions []metav1.Condition
		// expectCurrent is the expected Current release information in the
		// HelmRelease after unlock.
		expectCurrent func(releases []*helmrelease.Release) *v2.Snapshot
		// expectPrevious returns the expected Previous release information of
		// the HelmRelease after unlock.
		expectPrevious func(releases []*helmrelease.Release) *v2.Snapshot
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
			name: "unlock success",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusPendingInstall,
					}),
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, "PendingRelease", "Unlocked release"),
				*conditions.FalseCondition(v2.ReleasedCondition, "PendingRelease", "Unlocked release"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.Snapshot {
				return release.ObservedToSnapshot(release.ObserveRelease(releases[0]))
			},
		},
		{
			name: "unlock failure",
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
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusPendingRollback,
					}),
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
				}
			},
			wantErr: mockUpdateErr,
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, "PendingRelease", "Unlock of release"),
				*conditions.FalseCondition(v2.ReleasedCondition, "PendingRelease", "Unlock of release"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.Snapshot {
				return release.ObservedToSnapshot(release.ObserveRelease(releases[0]))
			},
			expectFailures: 1,
		},
		{
			name: "unlock without pending status",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusFailed,
					}),
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					Current: &v2.Snapshot{
						Name:      mockReleaseName,
						Namespace: releases[0].Namespace,
						Version:   1,
						Status:    helmrelease.StatusFailed.String(),
					},
				}
			},
			expectConditions: []metav1.Condition{},
			expectCurrent: func(releases []*helmrelease.Release) *v2.Snapshot {
				return &v2.Snapshot{
					Name:      mockReleaseName,
					Namespace: releases[0].Namespace,
					Version:   1,
					Status:    helmrelease.StatusFailed.String(),
				}
			},
		},
		{
			name: "unlock without current",
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{}
			},
			wantErr:          ErrNoCurrent,
			expectConditions: []metav1.Condition{},
		},
		{
			name: "unlock with stale current",
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
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
					Current: &v2.Snapshot{
						Name:      mockReleaseName,
						Namespace: releases[0].Namespace,
						Version:   releases[0].Version - 1,
						Status:    helmrelease.StatusPendingInstall.String(),
					},
				}
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.Snapshot {
				return &v2.Snapshot{
					Name:      mockReleaseName,
					Namespace: releases[0].Namespace,
					Version:   releases[0].Version - 1,
					Status:    helmrelease.StatusPendingInstall.String(),
				}
			},
		},
		{
			name: "unlock without latest",
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					Current: &v2.Snapshot{
						Name:    mockReleaseName,
						Version: 1,
						Status:  helmrelease.StatusFailed.String(),
					},
				}
			},
			expectConditions: []metav1.Condition{},
			expectCurrent: func(releases []*helmrelease.Release) *v2.Snapshot {
				return &v2.Snapshot{
					Name:    mockReleaseName,
					Version: 1,
					Status:  helmrelease.StatusFailed.String(),
				}
			},
		},
		{
			name: "unlock with storage query error",
			driver: func(driver helmdriver.Driver) helmdriver.Driver {
				return &storage.Failing{
					Driver: driver,
					GetErr: mockQueryErr,
				}
			},
			releases: func(namespace string) []*helmrelease.Release {
				return []*helmrelease.Release{
					testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: namespace,
						Version:   1,
						Chart:     testutil.BuildChart(),
						Status:    helmrelease.StatusPendingInstall,
					}),
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					Current: &v2.Snapshot{
						Name:    mockReleaseName,
						Version: 1,
						Status:  helmrelease.StatusFailed.String(),
					},
				}
			},
			wantErr:          mockQueryErr,
			expectConditions: []metav1.Condition{},
			expectCurrent: func(releases []*helmrelease.Release) *v2.Snapshot {
				return &v2.Snapshot{
					Name:    mockReleaseName,
					Version: 1,
					Status:  helmrelease.StatusFailed.String(),
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
			got := NewUnlock(cfg, recorder).Reconcile(context.TODO(), &Request{
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

func TestUnlock_failure(t *testing.T) {
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
				Current: release.ObservedToSnapshot(release.ObserveRelease(cur)),
			},
		}
		status = helmrelease.StatusPendingInstall
		err    = fmt.Errorf("unlock error")
	)

	recorder := testutil.NewFakeRecorder(10, false)
	r := &Unlock{
		eventRecorder: recorder,
	}

	req := &Request{Object: obj}
	r.failure(req, status, err)

	expectMsg := fmt.Sprintf(fmtUnlockFailure,
		fmt.Sprintf("%s/%s.%d", cur.Namespace, cur.Name, cur.Version),
		fmt.Sprintf("%s@%s", cur.Chart.Name(), cur.Chart.Metadata.Version),
		status, err.Error())

	g.Expect(req.Object.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
		*conditions.FalseCondition(v2.ReleasedCondition, "PendingRelease", expectMsg),
	}))
	g.Expect(req.Object.Status.Failures).To(Equal(int64(1)))
	g.Expect(recorder.GetEvents()).To(ConsistOf([]corev1.Event{
		{
			Type:    corev1.EventTypeWarning,
			Reason:  "PendingRelease",
			Message: expectMsg,
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					eventMetaGroupKey(eventv1.MetaRevisionKey): cur.Chart.Metadata.Version,
					eventMetaGroupKey(eventv1.MetaTokenKey):    chartutil.DigestValues(digest.Canonical, cur.Config).String(),
				},
			},
		},
	}))
}

func TestUnlock_success(t *testing.T) {
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
				Current: release.ObservedToSnapshot(release.ObserveRelease(cur)),
			},
		}
		status = helmrelease.StatusPendingInstall
	)

	recorder := testutil.NewFakeRecorder(10, false)
	r := &Unlock{
		eventRecorder: recorder,
	}

	req := &Request{Object: obj}
	r.success(req, status)

	expectMsg := fmt.Sprintf(fmtUnlockSuccess,
		fmt.Sprintf("%s/%s.%d", cur.Namespace, cur.Name, cur.Version),
		fmt.Sprintf("%s@%s", cur.Chart.Name(), cur.Chart.Metadata.Version),
		status)

	g.Expect(req.Object.Status.Conditions).To(conditions.MatchConditions([]metav1.Condition{
		*conditions.FalseCondition(v2.ReleasedCondition, "PendingRelease", expectMsg),
	}))
	g.Expect(req.Object.Status.Failures).To(Equal(int64(0)))
	g.Expect(recorder.GetEvents()).To(ConsistOf([]corev1.Event{
		{
			Type:    corev1.EventTypeNormal,
			Reason:  "PendingRelease",
			Message: expectMsg,
			ObjectMeta: metav1.ObjectMeta{
				Annotations: map[string]string{
					eventMetaGroupKey(eventv1.MetaRevisionKey): cur.Chart.Metadata.Version,
					eventMetaGroupKey(eventv1.MetaTokenKey):    chartutil.DigestValues(digest.Canonical, cur.Config).String(),
				},
			},
		},
	}))
}

func Test_observeUnlock(t *testing.T) {
	t.Run("unlock", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				Current: &v2.Snapshot{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusPendingRollback.String(),
				},
			},
		}
		rls := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusFailed,
		})
		expect := release.ObservedToSnapshot(release.ObserveRelease(rls))
		observeUnlock(obj)(rls)

		g.Expect(obj.GetPrevious()).To(BeNil())
		g.Expect(obj.GetCurrent()).To(Equal(expect))
	})

	t.Run("unlock without current", func(t *testing.T) {
		g := NewWithT(t)

		obj := &v2.HelmRelease{}
		rls := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusFailed,
		})
		observeUnlock(obj)(rls)

		g.Expect(obj.GetPrevious()).To(BeNil())
		g.Expect(obj.GetCurrent()).To(BeNil())
	})
}
