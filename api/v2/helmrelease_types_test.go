/*
Copyright 2026 The Flux authors

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

package v2

import (
	"reflect"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
)

func TestHelmReleaseStatus_SetChart(t *testing.T) {
	tests := []struct {
		name string
		ref  *HelmChartReference
		want string
	}{
		{
			name: "HelmChart omits the kind for backwards compatibility",
			ref:  &HelmChartReference{Kind: "HelmChart", Namespace: "default", Name: "podinfo"},
			want: "default/podinfo",
		},
		{
			name: "OCIRepository includes the kind",
			ref:  &HelmChartReference{Kind: "OCIRepository", Namespace: "default", Name: "podinfo"},
			want: "OCIRepository/default/podinfo",
		},
		{
			name: "nil clears the reference",
			ref:  nil,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var status HelmReleaseStatus
			status.SetChart(tt.ref)
			if status.HelmChart != tt.want {
				t.Errorf("SetChart() = %q, want %q", status.HelmChart, tt.want)
			}
		})
	}
}

func TestHelmReleaseStatus_GetHelmChartReference(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want *HelmChartReference
	}{
		{
			name: "legacy HelmChart reference",
			in:   "default/podinfo",
			want: &HelmChartReference{Kind: "HelmChart", Namespace: "default", Name: "podinfo"},
		},
		{
			name: "typed OCIRepository reference",
			in:   "OCIRepository/default/podinfo",
			want: &HelmChartReference{Kind: "OCIRepository", Namespace: "default", Name: "podinfo"},
		},
		{
			name: "empty",
			in:   "",
			want: nil,
		},
		{
			name: "unexpected format",
			in:   "default/podinfo/extra/segment",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status := HelmReleaseStatus{HelmChart: tt.in}
			got := status.GetHelmChartReference()
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetHelmChartReference() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestHelmChartTemplateSpec_GetVersion(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "defaults to latest",
			in:   "",
			want: "*",
		},
		{
			name: "returns the configured version",
			in:   "1.2.3",
			want: "1.2.3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := HelmChartTemplateSpec{Version: tt.in}
			if got := spec.GetVersion(); got != tt.want {
				t.Errorf("GetVersion() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHelmChartTemplateSpec_GetReconcileStrategy(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "defaults to ChartVersion",
			in:   "",
			want: "ChartVersion",
		},
		{
			name: "returns the configured strategy",
			in:   "Revision",
			want: "Revision",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			spec := HelmChartTemplateSpec{ReconcileStrategy: tt.in}
			if got := spec.GetReconcileStrategy(); got != tt.want {
				t.Errorf("GetReconcileStrategy() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHelmRelease_GetTemplateInterval(t *testing.T) {
	tests := []struct {
		name string
		obj  *HelmRelease
		want metav1.Duration
	}{
		{
			name: "no chart template falls back to the HelmRelease interval",
			obj: &HelmRelease{
				Spec: HelmReleaseSpec{
					Interval: metav1.Duration{Duration: time.Minute},
				},
			},
			want: metav1.Duration{Duration: time.Minute},
		},
		{
			name: "chart template without interval falls back to the HelmRelease interval",
			obj: &HelmRelease{
				Spec: HelmReleaseSpec{
					Interval: metav1.Duration{Duration: time.Minute},
					Chart:    &HelmChartTemplate{Spec: HelmChartTemplateSpec{}},
				},
			},
			want: metav1.Duration{Duration: time.Minute},
		},
		{
			name: "chart template interval takes precedence",
			obj: &HelmRelease{
				Spec: HelmReleaseSpec{
					Interval: metav1.Duration{Duration: time.Minute},
					Chart: &HelmChartTemplate{
						Spec: HelmChartTemplateSpec{
							OCIRepositorySpec: sourcev1.OCIRepositorySpec{
								Interval: &metav1.Duration{Duration: 2 * time.Minute},
							},
						},
					},
				},
			},
			want: metav1.Duration{Duration: 2 * time.Minute},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.obj.GetTemplateInterval(); got != tt.want {
				t.Errorf("GetTemplateInterval() = %v, want %v", got, tt.want)
			}
		})
	}
}
