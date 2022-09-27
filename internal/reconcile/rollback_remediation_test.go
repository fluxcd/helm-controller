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

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
	"github.com/fluxcd/helm-controller/internal/testutil"
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
		// expectCurrent is the expected Current release information on the
		// HelmRelease after rolling back.
		expectCurrent func(releases []*helmrelease.Release) *v2.HelmReleaseInfo
		// expectPrevious returns the expected Previous release information of
		// the HelmRelease after rolling back.
		expectPrevious func(releases []*helmrelease.Release) *v2.HelmReleaseInfo
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
					Current:  release.ObservedToInfo(release.ObserveRelease(releases[1])),
					Previous: release.ObservedToInfo(release.ObserveRelease(releases[0])),
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.RollbackSucceededReason, "Rolled back to"),
				*conditions.TrueCondition(v2.RemediatedCondition, v2.RollbackSucceededReason, "Rolled back to"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[2]))
			},
			expectPrevious: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[0]))
			},
		},
		{
			name: "rollback without previous",
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
					Current: release.ObservedToInfo(release.ObserveRelease(releases[1])),
				}
			},
			wantErr: ErrNoPrevious,
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[1]))
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
					Current:  release.ObservedToInfo(release.ObserveRelease(releases[1])),
					Previous: release.ObservedToInfo(release.ObserveRelease(releases[0])),
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.RollbackFailedReason,
					"timed out waiting for the condition"),
				*conditions.FalseCondition(v2.RemediatedCondition, v2.RollbackFailedReason,
					"timed out waiting for the condition"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[2]))
			},
			expectPrevious: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[0]))
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
					Current:  release.ObservedToInfo(release.ObserveRelease(releases[1])),
					Previous: release.ObservedToInfo(release.ObserveRelease(releases[0])),
				}
			},
			wantErr: mockCreateErr,
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.RollbackFailedReason,
					mockCreateErr.Error()),
				*conditions.FalseCondition(v2.RemediatedCondition, v2.RollbackFailedReason,
					mockCreateErr.Error()),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[1]))
			},
			expectPrevious: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[0]))
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
					Current:  release.ObservedToInfo(release.ObserveRelease(releases[1])),
					Previous: release.ObservedToInfo(release.ObserveRelease(releases[0])),
				}
			},
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(meta.ReadyCondition, v2.RollbackFailedReason,
					"storage update error"),
				*conditions.FalseCondition(v2.RemediatedCondition, v2.RollbackFailedReason,
					"storage update error"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[2]))
			},
			expectPrevious: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
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

func TestRollbackRemediation_failure(t *testing.T) {
	var (
		prev = testutil.BuildRelease(&helmrelease.MockReleaseOptions{
			Name:    mockReleaseName,
			Chart:   testutil.BuildChart(),
			Version: 4,
		})
		obj = &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				Previous: release.ObservedToInfo(release.ObserveRelease(prev)),
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
		r.failure(req, nil, err)

		expectMsg := fmt.Sprintf(fmtRollbackRemediationFailure,
			fmt.Sprintf("%s/%s.%d", prev.Namespace, prev.Name, prev.Version),
			fmt.Sprintf("%s@%s", prev.Chart.Name(), prev.Chart.Metadata.Version),
			err.Error())

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
						"revision": prev.Chart.Metadata.Version,
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
		r.failure(req, mockLogBuffer(5, 10), err)

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

	obj := &v2.HelmRelease{
		Status: v2.HelmReleaseStatus{
			Previous: release.ObservedToInfo(release.ObserveRelease(prev)),
		},
	}

	req := &Request{Object: obj}
	r.success(req)

	expectMsg := fmt.Sprintf(fmtRollbackRemediationSuccess,
		fmt.Sprintf("%s/%s.%d", prev.Namespace, prev.Name, prev.Version),
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
					"revision": prev.Chart.Metadata.Version,
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
		expect := release.ObservedToInfo(release.ObserveRelease(rls))

		g.Expect(obj.GetPrevious()).To(BeNil())
		g.Expect(obj.GetCurrent()).To(Equal(expect))
	})

	t.Run("rollback with current", func(t *testing.T) {
		g := NewWithT(t)

		current := &v2.HelmReleaseInfo{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   2,
			Status:    helmrelease.StatusFailed.String(),
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				Current: current,
			},
		}
		rls := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      current.Name,
			Namespace: current.Namespace,
			Version:   current.Version + 1,
			Status:    helmrelease.StatusPendingRollback,
		})
		expect := release.ObservedToInfo(release.ObserveRelease(rls))

		observeRollback(obj)(rls)
		g.Expect(obj.GetCurrent()).ToNot(BeNil())
		g.Expect(obj.GetCurrent()).To(Equal(expect))
		g.Expect(obj.GetPrevious()).To(BeNil())
	})

	t.Run("rollback with current with higher version", func(t *testing.T) {
		g := NewWithT(t)

		current := &v2.HelmReleaseInfo{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   2,
			Status:    helmrelease.StatusPendingRollback.String(),
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				Current: current,
			},
		}
		rls := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      current.Name,
			Namespace: current.Namespace,
			Version:   current.Version - 1,
			Status:    helmrelease.StatusSuperseded,
		})

		observeRollback(obj)(rls)
		g.Expect(obj.GetPrevious()).To(BeNil())
		g.Expect(obj.GetCurrent()).To(Equal(current))
	})

	t.Run("rollback with current with different name", func(t *testing.T) {
		g := NewWithT(t)

		current := &v2.HelmReleaseInfo{
			Name:      mockReleaseName + "-other",
			Namespace: mockReleaseNamespace,
			Version:   2,
			Status:    helmrelease.StatusFailed.String(),
		}
		obj := &v2.HelmRelease{
			Status: v2.HelmReleaseStatus{
				Current: current,
			},
		}
		rls := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusPendingRollback,
		})
		expect := release.ObservedToInfo(release.ObserveRelease(rls))

		observeRollback(obj)(rls)
		g.Expect(obj.GetPrevious()).To(BeNil())
		g.Expect(obj.GetCurrent()).To(Equal(expect))
	})
}
