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

	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmchartutil "helm.sh/helm/v3/pkg/chartutil"
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/client-go/tools/record"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/kube"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

func Test_NextAction(t *testing.T) {
	tests := []struct {
		name     string
		releases []*helmrelease.Release
		spec     func(spec *v2.HelmReleaseSpec)
		status   func(releases []*helmrelease.Release) v2.HelmReleaseStatus
		chart    *helmchart.Chart
		values   helmchartutil.Values
		want     ActionReconciler
		wantErr  bool
	}{
		{
			name: "up-to-date release returns no action",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusDeployed,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
		},
		{
			name:     "no release in storage requires install",
			releases: nil,
			want:     &Install{},
		},
		{
			name: "disappeared release from storage requires install",
			status: func(_ []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(testutil.BuildRelease(&helmrelease.MockReleaseOptions{
							Name:      mockReleaseName,
							Namespace: mockReleaseNamespace,
							Version:   1,
							Status:    helmrelease.StatusDeployed,
							Chart:     testutil.BuildChart(),
						}))),
					},
				}
			},
			want: &Install{},
		},
		{
			name: "existing release without current requires upgrade",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusDeployed,
					Chart:     testutil.BuildChart(),
				}),
			},
			want: &Upgrade{},
		},
		{
			name: "release digest parse error requires upgrade",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusDeployed,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				cur := release.ObservedToSnapshot(release.ObserveRelease(releases[0]))
				cur.Digest = "sha256:invalid"
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: cur,
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want:   &Upgrade{},
		},
		{
			name: "release digest mismatch requires upgrade",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusDeployed,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				cur := release.ObservedToSnapshot(release.ObserveRelease(releases[0]))
				// Digest for empty string is always mismatch
				cur.Digest = "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: cur,
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want:   &Upgrade{},
		},
		{
			name: "verified release with pending state requires unlock",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusPendingInstall,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want:   &Unlock{},
		},
		{
			name: "deployed release requires test when enabled",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusDeployed,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Test = &v2.Test{
					Enable: true,
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want:   &Test{},
		},
		{
			name: "failure test requires rollback when enabled",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusSuperseded,
					Chart:     testutil.BuildChart(),
				}),
				testutil.BuildRelease(
					&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: mockReleaseNamespace,
						Version:   2,
						Status:    helmrelease.StatusDeployed,
						Chart:     testutil.BuildChart(),
					},
					testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"}),
					testutil.ReleaseWithHookExecution("failure-tests", []helmrelease.HookEvent{helmrelease.HookTest},
						helmrelease.HookPhaseFailed),
				),
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Test = &v2.Test{
					Enable: true,
				}
				spec.Upgrade = &v2.Upgrade{
					Remediation: &v2.UpgradeRemediation{
						Retries: 1,
					},
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				cur := release.ObservedToSnapshot(release.ObserveRelease(releases[1]))
				cur.SetTestHooks(release.TestHooksFromRelease(releases[1]))

				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current:  cur,
						Previous: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want:   &RollbackRemediation{},
		},
		{
			name: "failure test requires uninstall when enabled",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(
					&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: mockReleaseNamespace,
						Version:   2,
						Status:    helmrelease.StatusDeployed,
						Chart:     testutil.BuildChart(),
					},
					testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"}),
					testutil.ReleaseWithHookExecution("failure-tests", []helmrelease.HookEvent{helmrelease.HookTest},
						helmrelease.HookPhaseFailed),
				),
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Test = &v2.Test{
					Enable: true,
				}
				spec.Install = &v2.Install{
					Remediation: &v2.InstallRemediation{
						Retries: 1,
					},
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				cur := release.ObservedToSnapshot(release.ObserveRelease(releases[0]))
				cur.SetTestHooks(release.TestHooksFromRelease(releases[0]))

				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: cur,
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want:   &UninstallRemediation{},
		},
		{
			name: "failure test is ignored when ignore failures is set",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(
					&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: mockReleaseNamespace,
						Version:   2,
						Status:    helmrelease.StatusDeployed,
						Chart:     testutil.BuildChart(),
					},
					testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"}),
					testutil.ReleaseWithHookExecution("failure-tests", []helmrelease.HookEvent{helmrelease.HookTest},
						helmrelease.HookPhaseFailed),
				),
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Test = &v2.Test{
					Enable:         true,
					IgnoreFailures: true,
				}
				spec.Install = &v2.Install{
					Remediation: &v2.InstallRemediation{
						Retries: 1,
					},
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				cur := release.ObservedToSnapshot(release.ObserveRelease(releases[0]))
				cur.SetTestHooks(release.TestHooksFromRelease(releases[0]))

				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: cur,
					},
				}
			},
		},
		{
			name: "failure test is ignored when not made by controller",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(
					&helmrelease.MockReleaseOptions{
						Name:      mockReleaseName,
						Namespace: mockReleaseNamespace,
						Version:   2,
						Status:    helmrelease.StatusDeployed,
						Chart:     testutil.BuildChart(),
					},
					testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"}),
					testutil.ReleaseWithHookExecution("failure-tests", []helmrelease.HookEvent{helmrelease.HookTest},
						helmrelease.HookPhaseFailed),
				),
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Test = &v2.Test{
					Enable: true,
				}
				spec.Install = &v2.Install{
					Remediation: &v2.InstallRemediation{
						Retries: 1,
					},
				}
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			want: &Test{},
		},
		{
			name: "failure release requires rollback when enabled",
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
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Upgrade = &v2.Upgrade{
					Remediation: &v2.UpgradeRemediation{
						Retries: 1,
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current:  release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
						Previous: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
					UpgradeFailures: 1,
				}
			},
			want: &RollbackRemediation{},
		},
		{
			name: "failure release requires uninstall when enabled",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusFailed,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Install = &v2.Install{
					Remediation: &v2.InstallRemediation{
						Retries: 1,
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
					InstallFailures: 1,
				}
			},
			want: &UninstallRemediation{},
		},
		{
			name: "failure release is ignored when no remediation strategy is configured",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusFailed,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
					InstallFailures: 1,
				}
			},
			wantErr: true,
		},
		{
			name: "failure release without install failure count requires upgrade",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusFailed,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
					UpgradeFailures: 1,
				}
			},
			want: &Upgrade{},
		},
		{
			name: "failure release without upgrade failure count requires upgrade",
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
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Upgrade = &v2.Upgrade{
					Remediation: &v2.UpgradeRemediation{
						Retries: 1,
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current:  release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
						Previous: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			want: &Upgrade{},
		},
		{
			name: "failure release with disappeared previous release requires upgrade",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   2,
					Status:    helmrelease.StatusFailed,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Upgrade = &v2.Upgrade{
					Remediation: &v2.UpgradeRemediation{
						Retries: 1,
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				prev := *releases[0]
				prev.Version = 1
				return v2.HelmReleaseStatus{
					UpgradeFailures: 1,
					History: v2.ReleaseHistory{
						Current:  release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
						Previous: release.ObservedToSnapshot(release.ObserveRelease(&prev)),
					},
				}
			},
			want: &Upgrade{},
		},
		{
			name: "superseded release requires install",
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
				spec.Upgrade = &v2.Upgrade{
					Remediation: &v2.UpgradeRemediation{
						Retries: 1,
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			want: &Install{},
		},
		{
			name: "exhausted install retries",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusFailed,
					Chart:     testutil.BuildChart(),
				}),
			},
			spec: func(spec *v2.HelmReleaseSpec) {
				spec.Install = &v2.Install{
					Remediation: &v2.InstallRemediation{
						Retries: 2,
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
					InstallFailures: 3,
				}
			},
			wantErr: true,
		},
		{
			name: "exhausted upgrade retries",
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
					Status:    helmrelease.StatusDeployed,
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
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current:  release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
						Previous: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
					UpgradeFailures: 3,
				}
			},
			wantErr: true,
		},
		{
			name: "uninstalled release requires install",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusUninstalled,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			want: &Install{},
		},
		{
			name: "chart change requires upgrade",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusDeployed,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(testutil.ChartWithName("other-name")),
			values: map[string]interface{}{"foo": "bar"},
			want:   &Upgrade{},
		},
		{
			name: "values diff requires upgrade",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusDeployed,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.ReleaseHistory{
						Current: release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"bar": "foo"},
			want:   &Upgrade{},
		},
		// {
		//	name: "manifestTmpl diff requires upgrade (or apply?) when enabled",
		// },
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
				action.WithDebugLog(logr.Discard()))
			g.Expect(err).ToNot(HaveOccurred())

			if len(tt.releases) > 0 {
				store := helmstorage.Init(cfg.Driver)
				for _, i := range tt.releases {
					g.Expect(store.Create(i)).To(Succeed())
				}
			}

			recorder := new(record.FakeRecorder)
			got, err := NextAction(context.TODO(), cfg, recorder, &Request{
				Object: obj,
				Chart:  tt.chart,
				Values: tt.values,
			})
			if tt.wantErr {
				g.Expect(got).To(BeNil())
				g.Expect(err).To(HaveOccurred())
				return
			}

			want := BeAssignableToTypeOf(tt.want)
			if tt.want == nil {
				want = BeNil()
			}

			g.Expect(got).To(want)
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}
