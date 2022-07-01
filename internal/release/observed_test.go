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

package release

import (
	"bytes"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	helmrelease "helm.sh/helm/v3/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

func TestIgnoreHookTestEvents(t *testing.T) {
	// testHookFixtures is a list of release.Hook in every possible LastRun state.
	var testHookFixtures = []helmrelease.Hook{
		{
			Name:   "never-run-test",
			Events: []helmrelease.HookEvent{helmrelease.HookTest},
		},
		{
			Name:   "passing-test",
			Events: []helmrelease.HookEvent{helmrelease.HookTest},
			LastRun: helmrelease.HookExecution{
				Phase: helmrelease.HookPhaseSucceeded,
			},
		},
		{
			Name:   "failing-test",
			Events: []helmrelease.HookEvent{helmrelease.HookTest},
			LastRun: helmrelease.HookExecution{
				Phase: helmrelease.HookPhaseFailed,
			},
		},
		{
			Name:   "passing-pre-install",
			Events: []helmrelease.HookEvent{helmrelease.HookPreInstall},
			LastRun: helmrelease.HookExecution{
				Phase: helmrelease.HookPhaseSucceeded,
			},
		},
	}

	tests := []struct {
		name  string
		hooks []helmrelease.Hook
		want  []helmrelease.Hook
	}{
		{
			name:  "ignores test hooks",
			hooks: testHookFixtures,
			want: []helmrelease.Hook{
				testHookFixtures[3],
			},
		},
		{
			name:  "no hooks",
			hooks: []helmrelease.Hook{},
			want:  []helmrelease.Hook{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			obs := ObservedRelease{
				Hooks: tt.hooks,
			}
			IgnoreHookTestEvents(&obs)
			g.Expect(obs.Hooks).To(Equal(tt.want))

		})
	}
}

func TestObservedRelease_Targets(t *testing.T) {
	tests := []struct {
		name            string
		obs             ObservedRelease
		targetName      string
		targetNamespace string
		targetVersion   int
		want            bool
	}{
		{
			name: "matching name, namespace and version",
			obs: ObservedRelease{
				Name:      "foo",
				Namespace: "bar",
				Version:   2,
			},
			targetName:      "foo",
			targetNamespace: "bar",
			targetVersion:   2,
			want:            true,
		},
		{
			name: "matching name and namespace with version set to 0",
			obs: ObservedRelease{
				Name:      "foo",
				Namespace: "bar",
				Version:   2,
			},
			targetName:      "foo",
			targetNamespace: "bar",
			targetVersion:   0,
			want:            true,
		},
		{
			name: "name mismatch",
			obs: ObservedRelease{
				Name:      "baz",
				Namespace: "bar",
				Version:   2,
			},
			targetName:      "foo",
			targetNamespace: "bar",
			targetVersion:   2,
		},
		{
			name: "namespace mismatch",
			obs: ObservedRelease{
				Name:      "foo",
				Namespace: "baz",
				Version:   2,
			},
			targetName:      "foo",
			targetNamespace: "bar",
			targetVersion:   2,
		},
		{
			name: "matching name, namespace and version",
			obs: ObservedRelease{
				Name:      "foo",
				Namespace: "bar",
				Version:   2,
			},
			targetName:      "foo",
			targetNamespace: "bar",
			targetVersion:   3,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(tt.obs.Targets(tt.targetName, tt.targetNamespace, tt.targetVersion)).To(Equal(tt.want))
		})
	}
}

func TestObservedRelease_Encode(t *testing.T) {
	g := NewWithT(t)

	o := ObservedRelease{
		Name:      "foo",
		Namespace: "bar",
		Version:   2,
	}
	w := &bytes.Buffer{}
	g.Expect(o.Encode(w)).ToNot(HaveOccurred())
	g.Expect(w.String()).ToNot(BeEmpty())
}

func TestObserveRelease(t *testing.T) {
	var (
		testReleaseWithConfig = testutil.BuildRelease(
			&helmrelease.MockReleaseOptions{
				Name:      "foo",
				Namespace: "namespace",
				Version:   1,
				Chart:     testutil.BuildChart(),
			},
			testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"}),
		)
		testReleaseWithLabels = testutil.BuildRelease(
			&helmrelease.MockReleaseOptions{
				Name:      "foo",
				Namespace: "namespace",
				Version:   1,
				Chart:     testutil.BuildChart(),
			},
			testutil.ReleaseWithLabels(map[string]string{"foo": "bar"}),
		)
	)

	tests := []struct {
		name    string
		release *helmrelease.Release
		filters []DataFilter
		want    ObservedRelease
	}{
		{
			name:    "observes release",
			release: smallRelease,
			want: ObservedRelease{
				Name:          smallRelease.Name,
				Namespace:     smallRelease.Namespace,
				Version:       smallRelease.Version,
				Info:          *smallRelease.Info,
				ChartMetadata: *smallRelease.Chart.Metadata,
				Manifest:      smallRelease.Manifest,
				Hooks:         nil,
				Labels:        smallRelease.Labels,
				Config:        smallRelease.Config,
			},
		},
		{
			name:    "observes with filters overwrite",
			release: midRelease,
			filters: []DataFilter{},
			want: ObservedRelease{
				Name:          midRelease.Name,
				Namespace:     midRelease.Namespace,
				Version:       midRelease.Version,
				Info:          *midRelease.Info,
				ChartMetadata: *midRelease.Chart.Metadata,
				Manifest:      midRelease.Manifest,
				Hooks: func() []helmrelease.Hook {
					var hooks []helmrelease.Hook
					for _, h := range midRelease.Hooks {
						hooks = append(hooks, *h)
					}
					return hooks
				}(),
				Labels: midRelease.Labels,
				Config: midRelease.Config,
			},
		},
		{
			name:    "observes config",
			release: testReleaseWithConfig,
			want: ObservedRelease{
				Name:          testReleaseWithConfig.Name,
				Namespace:     testReleaseWithConfig.Namespace,
				Version:       testReleaseWithConfig.Version,
				Info:          *testReleaseWithConfig.Info,
				ChartMetadata: *testReleaseWithConfig.Chart.Metadata,
				Config:        testReleaseWithConfig.Config,
				Manifest:      testReleaseWithConfig.Manifest,
				Hooks: []helmrelease.Hook{
					*testReleaseWithConfig.Hooks[0],
				},
			},
		},
		{
			name:    "observes labels",
			release: testReleaseWithLabels,
			want: ObservedRelease{
				Name:          testReleaseWithLabels.Name,
				Namespace:     testReleaseWithLabels.Namespace,
				Version:       testReleaseWithLabels.Version,
				Info:          *testReleaseWithLabels.Info,
				ChartMetadata: *testReleaseWithLabels.Chart.Metadata,
				Config:        testReleaseWithLabels.Config,
				Labels:        testReleaseWithLabels.Labels,
				Manifest:      testReleaseWithLabels.Manifest,
				Hooks: []helmrelease.Hook{
					*testReleaseWithLabels.Hooks[0],
				},
			},
		},
		{
			name:    "empty release",
			release: &helmrelease.Release{},
			want:    ObservedRelease{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(ObserveRelease(tt.release, tt.filters...)).To(testutil.Equal(tt.want))
		})
	}
}

func TestObservedToInfo(t *testing.T) {
	g := NewWithT(t)

	obs := ObserveRelease(testutil.BuildRelease(&helmrelease.MockReleaseOptions{
		Name:      "foo",
		Namespace: "namespace",
		Version:   1,
		Chart:     testutil.BuildChart(),
	}, testutil.ReleaseWithConfig(map[string]interface{}{"foo": "bar"})))

	got := ObservedToInfo(obs)

	g.Expect(got.Name).To(Equal(obs.Name))
	g.Expect(got.Namespace).To(Equal(obs.Namespace))
	g.Expect(got.Version).To(Equal(obs.Version))
	g.Expect(got.ChartName).To(Equal(obs.ChartMetadata.Name))
	g.Expect(got.ChartVersion).To(Equal(obs.ChartMetadata.Version))
	g.Expect(got.Status).To(BeEquivalentTo(obs.Info.Status))

	g.Expect(obs.Info.FirstDeployed.Time.Equal(got.FirstDeployed.Time)).To(BeTrue())
	g.Expect(obs.Info.LastDeployed.Time.Equal(got.LastDeployed.Time)).To(BeTrue())
	g.Expect(obs.Info.Deleted.Time.Equal(got.Deleted.Time)).To(BeTrue())

	g.Expect(got.Digest).ToNot(BeEmpty())
	g.Expect(digest.Digest(got.Digest).Validate()).To(Succeed())

	g.Expect(got.ConfigDigest).ToNot(BeEmpty())
	g.Expect(digest.Digest(got.ConfigDigest).Validate()).To(Succeed())
}

func TestTestHooksFromRelease(t *testing.T) {
	g := NewWithT(t)

	hooks := []*helmrelease.Hook{
		{
			Name:   "never-run-test",
			Events: []helmrelease.HookEvent{helmrelease.HookTest},
		},
		{
			Name:   "passing-test",
			Events: []helmrelease.HookEvent{helmrelease.HookTest},
			LastRun: helmrelease.HookExecution{
				Phase: helmrelease.HookPhaseSucceeded,
			},
		},
		{
			Name:   "failing-test",
			Events: []helmrelease.HookEvent{helmrelease.HookTest},
			LastRun: helmrelease.HookExecution{
				Phase: helmrelease.HookPhaseFailed,
			},
		},
		{
			Name:   "passing-pre-install",
			Events: []helmrelease.HookEvent{helmrelease.HookPreInstall},
			LastRun: helmrelease.HookExecution{
				Phase: helmrelease.HookPhaseSucceeded,
			},
		},
	}
	rls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
		Name:      "foo",
		Namespace: "namespace",
		Version:   1,
		Chart:     testutil.BuildChart(),
	}, testutil.ReleaseWithHooks(hooks))

	g.Expect(TestHooksFromRelease(rls)).To(testutil.Equal(map[string]*v2.HelmReleaseTestHook{
		hooks[0].Name: {},
		hooks[1].Name: {
			LastStarted:   metav1.Time{Time: hooks[1].LastRun.StartedAt.Time},
			LastCompleted: metav1.Time{Time: hooks[1].LastRun.CompletedAt.Time},
			Phase:         hooks[1].LastRun.Phase.String(),
		},
		hooks[2].Name: {
			LastStarted:   metav1.Time{Time: hooks[2].LastRun.StartedAt.Time},
			LastCompleted: metav1.Time{Time: hooks[2].LastRun.CompletedAt.Time},
			Phase:         hooks[2].LastRun.Phase.String(),
		},
	}))
}
