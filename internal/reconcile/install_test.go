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

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	helmrelease "helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/releaseutil"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
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
		values chartutil.Values
		// spec modifies the HelmRelease object spec before install.
		spec func(spec *v2.HelmReleaseSpec)
		// status to configure on the HelmRelease object before install.
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
			name:  "install success",
			chart: testutil.BuildChart(),
			expectConditions: []metav1.Condition{
				*conditions.TrueCondition(v2.ReleasedCondition, v2.InstallSucceededReason,
					"Install complete"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[0]))
			},
		},
		{
			name:  "install failure",
			chart: testutil.BuildChart(testutil.ChartWithFailingHook()),
			expectConditions: []metav1.Condition{
				*conditions.FalseCondition(v2.ReleasedCondition, v2.InstallFailedReason,
					"failed post-install"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[0]))
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
					Current: release.ObservedToInfo(release.ObserveRelease(releases[0])),
				}
			},
			chart: testutil.BuildChart(),
			expectConditions: []metav1.Condition{
				*conditions.TrueCondition(v2.ReleasedCondition, v2.InstallSucceededReason,
					"Install complete"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[1]))
			},
			expectPrevious: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[0]))
			},
		},
		{
			name: "install with stale current",
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					Current: release.ObservedToInfo(release.ObserveRelease(testutil.BuildRelease(&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: "other",
						Version:   1,
						Status:    helmrelease.StatusUninstalled,
						Chart:     testutil.BuildChart(),
					}))),
				}
			},
			chart: testutil.BuildChart(),
			expectConditions: []metav1.Condition{
				*conditions.TrueCondition(v2.ReleasedCondition, v2.InstallSucceededReason,
					"Install complete"),
			},
			expectCurrent: func(releases []*helmrelease.Release) *v2.HelmReleaseInfo {
				return release.ObservedToInfo(release.ObserveRelease(releases[0]))
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

			got := (&Install{configFactory: cfg}).Reconcile(context.TODO(), &Request{
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
