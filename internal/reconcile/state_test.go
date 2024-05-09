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
	"strings"
	"testing"

	. "github.com/onsi/gomega"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmchartutil "helm.sh/helm/v3/pkg/chartutil"
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/ssa/jsondiff"
	ssanormalize "github.com/fluxcd/pkg/ssa/normalize"
	ssautil "github.com/fluxcd/pkg/ssa/utils"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/digest"
	"github.com/fluxcd/helm-controller/internal/kube"
	"github.com/fluxcd/helm-controller/internal/postrender"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

func Test_DetermineReleaseState(t *testing.T) {
	tests := []struct {
		name     string
		releases []*helmrelease.Release
		spec     func(spec *v2.HelmReleaseSpec)
		status   func(releases []*helmrelease.Release) v2.HelmReleaseStatus
		chart    *helmchart.Chart
		values   helmchartutil.Values
		want     ReleaseState
		wantErr  bool
	}{
		{
			name: "in-sync release",
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
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want: ReleaseState{
				Status: ReleaseStatusInSync,
			},
		},
		{
			name:     "no release in storage",
			releases: nil,
			want: ReleaseState{
				Status: ReleaseStatusAbsent,
			},
		},
		{
			name: "release disappeared from storage",
			status: func(_ []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(testutil.BuildRelease(&helmrelease.MockReleaseOptions{
							Name:      mockReleaseName,
							Namespace: mockReleaseNamespace,
							Version:   1,
							Status:    helmrelease.StatusDeployed,
							Chart:     testutil.BuildChart(),
						}))),
					},
				}
			},
			want: ReleaseState{
				Status: ReleaseStatusAbsent,
			},
		},
		{
			name: "existing release without current",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusDeployed,
					Chart:     testutil.BuildChart(),
				}),
			},
			want: ReleaseState{
				Status: ReleaseStatusUnmanaged,
			},
		},
		{
			name: "release digest parse error",
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
					History: v2.Snapshots{
						cur,
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want: ReleaseState{
				Status: ReleaseStatusUnmanaged,
			},
		},
		{
			name: "release digest mismatch",
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
					History: v2.Snapshots{
						cur,
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want: ReleaseState{
				Status: ReleaseStatusUnmanaged,
			},
		},
		{
			name: "release in pending state",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusPendingInstall,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want: ReleaseState{
				Status: ReleaseStatusLocked,
			},
		},
		{
			name: "untested release",
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
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want: ReleaseState{
				Status: ReleaseStatusUntested,
			},
		},
		{
			name: "failed test",
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
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				cur := release.ObservedToSnapshot(release.ObserveRelease(releases[1]))
				cur.SetTestHooks(release.TestHooksFromRelease(releases[1]))

				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						cur,
					},
					LastAttemptedReleaseAction: v2.ReleaseActionUpgrade,
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want: ReleaseState{
				Status: ReleaseStatusFailed,
			},
		},
		{
			name: "failed test with ignore failures set",
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
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				cur := release.ObservedToSnapshot(release.ObserveRelease(releases[0]))
				cur.SetTestHooks(release.TestHooksFromRelease(releases[0]))

				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						cur,
					},
					LastAttemptedReleaseAction: v2.ReleaseActionInstall,
				}
			},
			want: ReleaseState{
				Status: ReleaseStatusInSync,
			},
		},
		{
			name: "failed test is ignored when not made by controller",
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
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			want: ReleaseState{
				Status: ReleaseStatusUntested,
			},
		},
		{
			name: "failed release",
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
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[1])),
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			want: ReleaseState{
				Status: ReleaseStatusFailed,
			},
		},
		{
			name: "uninstalled release",
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
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			want: ReleaseState{
				Status: ReleaseStatusAbsent,
			},
		},
		{
			name: "uninstalled release without current",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      mockReleaseName,
					Namespace: mockReleaseNamespace,
					Version:   1,
					Status:    helmrelease.StatusUninstalled,
					Chart:     testutil.BuildChart(),
				}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})),
			},
			want: ReleaseState{
				Status: ReleaseStatusAbsent,
			},
		},
		{
			name: "chart changed",
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
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(testutil.ChartWithName("other-name")),
			values: map[string]interface{}{"foo": "bar"},
			want: ReleaseState{
				Status: ReleaseStatusOutOfSync,
			},
		},
		{
			name: "values changed",
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
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"bar": "foo"},
			want: ReleaseState{
				Status: ReleaseStatusOutOfSync,
			},
		},
		{
			name: "postRenderers changed",
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
				spec.PostRenderers = postRenderers2
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
					ObservedPostRenderersDigest: postrender.Digest(digest.Canonical, postRenderers).String(),
					Conditions: []metav1.Condition{
						{
							Type:               meta.ReadyCondition,
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 1,
						},
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want: ReleaseState{
				Status: ReleaseStatusOutOfSync,
			},
		},
		{
			name: "postRenderers mismatch ignored for processed generation",
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
				spec.PostRenderers = postRenderers2
			},
			status: func(releases []*helmrelease.Release) v2.HelmReleaseStatus {
				return v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(releases[0])),
					},
					ObservedPostRenderersDigest: postrender.Digest(digest.Canonical, postRenderers).String(),
					Conditions: []metav1.Condition{
						{
							Type:               meta.ReadyCondition,
							Status:             metav1.ConditionTrue,
							ObservedGeneration: 2,
						},
					},
				}
			},
			chart:  testutil.BuildChart(),
			values: map[string]interface{}{"foo": "bar"},
			want: ReleaseState{
				Status: ReleaseStatusInSync,
			},
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
			// Set a non-zero generation so that old observations can be set on
			// the object status.
			obj.Generation = 2

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

			got, err := DetermineReleaseState(context.TODO(), cfg, &Request{
				Object: obj,
				Chart:  tt.chart,
				Values: tt.values,
			})
			if tt.wantErr {
				g.Expect(got).To(BeNil())
				g.Expect(err).To(HaveOccurred())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got.Status).To(Equal(tt.want.Status))
			g.Expect(got.Reason).To(ContainSubstring(tt.want.Reason))
		})
	}
}

func TestDetermineReleaseState_DriftDetection(t *testing.T) {
	tests := []struct {
		name          string
		driftMode     v2.DriftDetectionMode
		applyManifest bool
		want          func(namespace string) ReleaseState
	}{
		{
			name:      "with drift and detection mode enabled",
			driftMode: v2.DriftDetectionEnabled,
			want: func(namespace string) ReleaseState {
				return ReleaseState{
					Status: ReleaseStatusDrifted,
					Diff: jsondiff.DiffSet{
						{
							Type: jsondiff.DiffTypeCreate,
							DesiredObject: &unstructured.Unstructured{
								Object: map[string]interface{}{
									"apiVersion": "v1",
									"kind":       "Secret",
									"metadata": map[string]interface{}{
										"name":              "fixture",
										"namespace":         namespace,
										"creationTimestamp": nil,
										"labels": map[string]interface{}{
											"app.kubernetes.io/managed-by": "Helm",
										},
										"annotations": map[string]interface{}{
											"meta.helm.sh/release-name":      mockReleaseName,
											"meta.helm.sh/release-namespace": namespace,
										},
									},
								},
							},
						},
					},
				}
			},
		},
		{
			name:          "without drift and detection mode enabled",
			driftMode:     v2.DriftDetectionEnabled,
			applyManifest: true,
			want: func(_ string) ReleaseState {
				return ReleaseState{Status: ReleaseStatusInSync}
			},
		},
		{
			name:      "with drift and detection mode warn",
			driftMode: v2.DriftDetectionWarn,
			want: func(namespace string) ReleaseState {
				return ReleaseState{
					Status: ReleaseStatusDrifted,
					Diff: jsondiff.DiffSet{
						{
							Type: jsondiff.DiffTypeCreate,
							DesiredObject: &unstructured.Unstructured{
								Object: map[string]interface{}{
									"apiVersion": "v1",
									"kind":       "Secret",
									"metadata": map[string]interface{}{
										"name":              "fixture",
										"namespace":         namespace,
										"creationTimestamp": nil,
										"labels": map[string]interface{}{
											"app.kubernetes.io/managed-by": "Helm",
										},
										"annotations": map[string]interface{}{
											"meta.helm.sh/release-name":      mockReleaseName,
											"meta.helm.sh/release-namespace": namespace,
										},
									},
								},
							},
						},
					},
				}
			},
		},
		{
			name:          "without drift and detection mode warn",
			applyManifest: true,
			driftMode:     v2.DriftDetectionWarn,
			want: func(_ string) ReleaseState {
				return ReleaseState{Status: ReleaseStatusInSync}
			},
		},
		{
			name:      "drift detection mode disabled",
			driftMode: v2.DriftDetectionDisabled,
			want: func(_ string) ReleaseState {
				return ReleaseState{Status: ReleaseStatusInSync}
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

			chart := testutil.BuildChart()

			rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
				Name:      mockReleaseName,
				Namespace: releaseNamespace,
				Version:   1,
				Status:    helmrelease.StatusDeployed,
				Chart:     chart,
			})

			if tt.applyManifest {
				objs, err := ssautil.ReadObjects(strings.NewReader(rls.Manifest))
				g.Expect(err).ToNot(HaveOccurred())

				for _, obj := range objs {
					g.Expect(ssanormalize.Unstructured(obj)).To(Succeed())
					obj.SetNamespace(releaseNamespace)
					obj.SetLabels(map[string]string{
						"app.kubernetes.io/managed-by": "Helm",
					})
					obj.SetAnnotations(map[string]string{
						"meta.helm.sh/release-name":      rls.Name,
						"meta.helm.sh/release-namespace": rls.Namespace,
					})
					g.Expect(testEnv.Create(context.Background(), obj)).To(Succeed())
				}
			}

			obj := &v2.HelmRelease{
				Spec: v2.HelmReleaseSpec{
					ReleaseName:      mockReleaseName,
					TargetNamespace:  releaseNamespace,
					StorageNamespace: releaseNamespace,
					DriftDetection: &v2.DriftDetection{
						Mode: tt.driftMode,
					},
				},
				Status: v2.HelmReleaseStatus{
					History: v2.Snapshots{
						release.ObservedToSnapshot(release.ObserveRelease(rls)),
					},
				},
			}

			getter, err := RESTClientGetterFromManager(testEnv.Manager, obj.GetReleaseNamespace())
			g.Expect(err).ToNot(HaveOccurred())

			cfg, err := action.NewConfigFactory(getter,
				action.WithStorage(action.DefaultStorageDriver, obj.GetStorageNamespace()),
			)
			g.Expect(err).ToNot(HaveOccurred())

			store := helmstorage.Init(cfg.Driver)
			g.Expect(store.Create(rls)).To(Succeed())

			got, err := DetermineReleaseState(context.TODO(), cfg, &Request{
				Object: obj,
				Chart:  testutil.BuildChart(),
				Values: rls.Config,
			})
			g.Expect(err).ToNot(HaveOccurred())

			want := tt.want(releaseNamespace)
			g.Expect(got).To(Equal(want))
		})
	}
}
