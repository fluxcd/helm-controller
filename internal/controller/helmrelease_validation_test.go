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

package controller

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

// TestHelmReleaseChartTemplateValidation verifies the CEL expressions on
// HelmRelease.spec.chart, which enforce the union between the HelmChart and
// OCIRepository chart templates.
func TestHelmReleaseChartTemplateValidation(t *testing.T) {
	g := NewWithT(t)

	ns, err := testEnv.CreateNamespace(context.TODO(), "chart-template-validation")
	g.Expect(err).ToNot(HaveOccurred())
	t.Cleanup(func() {
		_ = testEnv.Delete(context.TODO(), ns)
	})

	helmSourceRef := &v2.CrossNamespaceObjectReference{
		Kind: sourcev1.HelmRepositoryKind,
		Name: "podinfo",
	}

	tests := []struct {
		name    string
		chart   v2.HelmChartTemplate
		wantErr string
	}{
		{
			name: "HelmChart template with chart and sourceRef",
			chart: v2.HelmChartTemplate{
				Spec: v2.HelmChartTemplateSpec{
					Chart:     "podinfo",
					SourceRef: helmSourceRef,
				},
			},
		},
		{
			name: "HelmChart kind explicitly set",
			chart: v2.HelmChartTemplate{
				Kind: sourcev1.HelmChartKind,
				Spec: v2.HelmChartTemplateSpec{
					Chart:     "podinfo",
					SourceRef: helmSourceRef,
				},
			},
		},
		{
			name: "HelmChart template without chart",
			chart: v2.HelmChartTemplate{
				Spec: v2.HelmChartTemplateSpec{
					SourceRef: helmSourceRef,
				},
			},
			wantErr: "chart.spec.chart and chart.spec.sourceRef must be set when chart.kind is not 'OCIRepository'",
		},
		{
			name: "HelmChart template without sourceRef",
			chart: v2.HelmChartTemplate{
				Spec: v2.HelmChartTemplateSpec{
					Chart: "podinfo",
				},
			},
			wantErr: "chart.spec.chart and chart.spec.sourceRef must be set when chart.kind is not 'OCIRepository'",
		},
		{
			name: "OCIRepository template with url and ref",
			chart: v2.HelmChartTemplate{
				Kind: sourcev1.OCIRepositoryKind,
				Spec: v2.HelmChartTemplateSpec{
					OCIRepositorySpec: sourcev1.OCIRepositorySpec{
						URL:       "oci://ghcr.io/stefanprodan/charts/podinfo",
						Reference: &sourcev1.OCIRepositoryRef{Tag: "6.6.0"},
					},
				},
			},
		},
		{
			name: "OCIRepository template without url",
			chart: v2.HelmChartTemplate{
				Kind: sourcev1.OCIRepositoryKind,
				Spec: v2.HelmChartTemplateSpec{},
			},
			wantErr: "chart.spec.url must be set when chart.kind is 'OCIRepository'",
		},
		{
			name: "OCIRepository template with chart",
			chart: v2.HelmChartTemplate{
				Kind: sourcev1.OCIRepositoryKind,
				Spec: v2.HelmChartTemplateSpec{
					OCIRepositorySpec: sourcev1.OCIRepositorySpec{
						URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
					},
					Chart: "podinfo",
				},
			},
			wantErr: "HelmChart-only fields in chart.spec cannot be set when chart.kind is 'OCIRepository'",
		},
		{
			name: "OCIRepository template with sourceRef",
			chart: v2.HelmChartTemplate{
				Kind: sourcev1.OCIRepositoryKind,
				Spec: v2.HelmChartTemplateSpec{
					OCIRepositorySpec: sourcev1.OCIRepositorySpec{
						URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
					},
					SourceRef: helmSourceRef,
				},
			},
			wantErr: "HelmChart-only fields in chart.spec cannot be set when chart.kind is 'OCIRepository'",
		},
		{
			name: "OCIRepository template with version",
			chart: v2.HelmChartTemplate{
				Kind: sourcev1.OCIRepositoryKind,
				Spec: v2.HelmChartTemplateSpec{
					OCIRepositorySpec: sourcev1.OCIRepositorySpec{
						URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
					},
					Version: "1.2.3",
				},
			},
			wantErr: "HelmChart-only fields in chart.spec cannot be set when chart.kind is 'OCIRepository'",
		},
		{
			name: "OCIRepository template with reconcileStrategy",
			chart: v2.HelmChartTemplate{
				Kind: sourcev1.OCIRepositoryKind,
				Spec: v2.HelmChartTemplateSpec{
					OCIRepositorySpec: sourcev1.OCIRepositorySpec{
						URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
					},
					ReconcileStrategy: sourcev1.ReconcileStrategyRevision,
				},
			},
			wantErr: "HelmChart-only fields in chart.spec cannot be set when chart.kind is 'OCIRepository'",
		},
		{
			name: "OCIRepository template with valuesFiles",
			chart: v2.HelmChartTemplate{
				Kind: sourcev1.OCIRepositoryKind,
				Spec: v2.HelmChartTemplateSpec{
					OCIRepositorySpec: sourcev1.OCIRepositorySpec{
						URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
					},
					ValuesFiles: []string{"values.yaml"},
				},
			},
			wantErr: "HelmChart-only fields in chart.spec cannot be set when chart.kind is 'OCIRepository'",
		},
		{
			name: "OCIRepository template with ignoreMissingValuesFiles",
			chart: v2.HelmChartTemplate{
				Kind: sourcev1.OCIRepositoryKind,
				Spec: v2.HelmChartTemplateSpec{
					OCIRepositorySpec: sourcev1.OCIRepositorySpec{
						URL: "oci://ghcr.io/stefanprodan/charts/podinfo",
					},
					IgnoreMissingValuesFiles: true,
				},
			},
			wantErr: "HelmChart-only fields in chart.spec cannot be set when chart.kind is 'OCIRepository'",
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			chart := tt.chart
			obj := &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("chart-template-%d", i),
					Namespace: ns.Name,
				},
				Spec: v2.HelmReleaseSpec{
					Interval: metav1.Duration{Duration: time.Minute},
					Chart:    &chart,
				},
			}

			err := testEnv.Create(context.TODO(), obj)
			if tt.wantErr == "" {
				g.Expect(err).ToNot(HaveOccurred())
				return
			}
			g.Expect(err).To(HaveOccurred())
			g.Expect(apierrors.IsInvalid(err)).To(BeTrue())
			g.Expect(err.Error()).To(ContainSubstring(tt.wantErr))
		})
	}
}
