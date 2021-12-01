/*
Copyright 2020 The Flux authors

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

package controllers

import (
	"context"
	"testing"
	"time"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"
	"github.com/go-logr/logr"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
)

func TestHelmReleaseReconciler_reconcileChart(t *testing.T) {
	tests := []struct {
		name                  string
		hr                    *v2.HelmRelease
		hc                    *sourcev1.HelmChart
		expectHelmChartStatus string
		expectGC              bool
		expectErr             bool
	}{
		{
			name: "new HelmChart",
			hr: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-release",
					Namespace: "default",
				},
				Spec: v2.HelmReleaseSpec{
					Interval: metav1.Duration{Duration: time.Minute},
					Chart: v2.HelmChartTemplate{
						Spec: v2.HelmChartTemplateSpec{
							Chart: "chart",
							SourceRef: v2.CrossNamespaceObjectReference{
								Name: "test-repository",
								Kind: "HelmRepository",
							},
						},
					},
				},
			},
			hc:                    nil,
			expectHelmChartStatus: "default/default-test-release",
		},
		{
			name: "existing HelmChart",
			hr: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-release",
					Namespace: "default",
				},
				Spec: v2.HelmReleaseSpec{
					Interval: metav1.Duration{Duration: time.Minute},
					Chart: v2.HelmChartTemplate{
						Spec: v2.HelmChartTemplateSpec{
							Chart: "chart",
							SourceRef: v2.CrossNamespaceObjectReference{
								Name: "test-repository",
								Kind: "HelmRepository",
							},
						},
					},
				},
			},
			hc: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.HelmChartSpec{
					Chart: "chart",
					SourceRef: sourcev1.LocalHelmChartSourceReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
				},
			},
			expectHelmChartStatus: "default/default-test-release",
		},
		{
			name: "modified HelmChart",
			hr: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-release",
					Namespace: "default",
				},
				Spec: v2.HelmReleaseSpec{
					Interval: metav1.Duration{Duration: time.Minute},
					Chart: v2.HelmChartTemplate{
						Spec: v2.HelmChartTemplateSpec{
							Chart: "chart",
							SourceRef: v2.CrossNamespaceObjectReference{
								Name:      "test-repository",
								Kind:      "HelmRepository",
								Namespace: "cross",
							},
						},
					},
				},
				Status: v2.HelmReleaseStatus{
					HelmChart: "default/default-test-release",
				},
			},
			hc: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.HelmChartSpec{
					Chart: "chart",
					SourceRef: sourcev1.LocalHelmChartSourceReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
				},
			},
			expectHelmChartStatus: "cross/default-test-release",
			expectGC:              true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(v2.AddToScheme(scheme.Scheme)).To(Succeed())
			g.Expect(sourcev1.AddToScheme(scheme.Scheme)).To(Succeed())

			var c client.Client
			if tt.hc != nil {
				c = fake.NewFakeClientWithScheme(scheme.Scheme, tt.hc)
			} else {
				c = fake.NewFakeClientWithScheme(scheme.Scheme)
			}

			r := &HelmReleaseReconciler{
				Client: c,
			}

			hc, err := r.reconcileChart(logr.NewContext(context.TODO(), log.NullLogger{}), tt.hr)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(hc).To(BeNil())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(hc).NotTo(BeNil())
			}

			g.Expect(tt.hr.Status.HelmChart).To(Equal(tt.expectHelmChartStatus))

			if tt.expectGC {
				objKey := client.ObjectKeyFromObject(tt.hc)
				err = c.Get(context.TODO(), objKey, tt.hc.DeepCopy())
				g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
			}
		})
	}
}

func TestHelmReleaseReconciler_deleteHelmChart(t *testing.T) {
	tests := []struct {
		name                  string
		hc                    *sourcev1.HelmChart
		hr                    *v2.HelmRelease
		expectHelmChartStatus string
		expectErr             bool
	}{
		{
			name: "delete existing HelmChart",
			hc: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-chart",
					Namespace: "default",
				},
			},
			hr: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-release",
				},
				Status: v2.HelmReleaseStatus{
					HelmChart: "default/test-chart",
				},
			},
			expectHelmChartStatus: "",
			expectErr:             false,
		},
		{
			name: "delete already removed HelmChart",
			hc:   nil,
			hr: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-release",
				},
				Status: v2.HelmReleaseStatus{
					HelmChart: "default/test-chart",
				},
			},
			expectHelmChartStatus: "",
			expectErr:             false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			g.Expect(v2.AddToScheme(scheme.Scheme)).To(Succeed())
			g.Expect(sourcev1.AddToScheme(scheme.Scheme)).To(Succeed())

			var c client.Client
			if tt.hc != nil {
				c = fake.NewFakeClientWithScheme(scheme.Scheme, tt.hc)
			} else {
				c = fake.NewFakeClientWithScheme(scheme.Scheme)
			}

			r := &HelmReleaseReconciler{
				Client: c,
			}

			err := r.deleteHelmChart(context.TODO(), tt.hr)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(tt.hr.Status.HelmChart).To(Equal(tt.expectHelmChartStatus))
		})
	}
}

func Test_buildHelmChartFromTemplate(t *testing.T) {
	hrWithChartTemplate := v2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-release",
			Namespace: "default",
		},
		Spec: v2.HelmReleaseSpec{
			Interval: metav1.Duration{Duration: time.Minute},
			Chart: v2.HelmChartTemplate{
				Spec: v2.HelmChartTemplateSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: v2.CrossNamespaceObjectReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					Interval:    &metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles: []string{"values.yaml"},
				},
			},
		},
	}

	tests := []struct {
		name   string
		modify func(release *v2.HelmRelease)
		want   *sourcev1.HelmChart
	}{
		{
			name:   "builds HelmChart from HelmChartTemplate",
			modify: func(*v2.HelmRelease) {},
			want: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.HelmChartSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: sourcev1.LocalHelmChartSourceReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					Interval:    metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles: []string{"values.yaml"},
				},
			},
		},
		{
			name: "takes SourceRef namespace into account",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.Spec.SourceRef.Namespace = "cross"
			},
			want: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "cross",
				},
				Spec: sourcev1.HelmChartSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: sourcev1.LocalHelmChartSourceReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					Interval:    metav1.Duration{Duration: 2 * time.Minute},
					ValuesFiles: []string{"values.yaml"},
				},
			},
		},
		{
			name: "falls back to HelmRelease interval",
			modify: func(hr *v2.HelmRelease) {
				hr.Spec.Chart.Spec.Interval = nil
			},
			want: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "default-test-release",
					Namespace: "default",
				},
				Spec: sourcev1.HelmChartSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: sourcev1.LocalHelmChartSourceReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					Interval:    metav1.Duration{Duration: time.Minute},
					ValuesFiles: []string{"values.yaml"},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			hr := hrWithChartTemplate.DeepCopy()
			tt.modify(hr)
			g.Expect(buildHelmChartFromTemplate(hr)).To(Equal(tt.want))
		})
	}
}

func Test_helmChartRequiresUpdate(t *testing.T) {
	hrWithChartTemplate := v2.HelmRelease{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-release",
		},
		Spec: v2.HelmReleaseSpec{
			Interval: metav1.Duration{Duration: time.Minute},
			Chart: v2.HelmChartTemplate{
				Spec: v2.HelmChartTemplateSpec{
					Chart:   "chart",
					Version: "1.0.0",
					SourceRef: v2.CrossNamespaceObjectReference{
						Name: "test-repository",
						Kind: "HelmRepository",
					},
					Interval: &metav1.Duration{Duration: 2 * time.Minute},
				},
			},
		},
	}

	tests := []struct {
		name   string
		modify func(*v2.HelmRelease, *sourcev1.HelmChart)
		want   bool
	}{
		{
			name:   "detects no change",
			modify: func(*v2.HelmRelease, *sourcev1.HelmChart) {},
			want:   false,
		},
		{
			name: "detects chart change",
			modify: func(hr *v2.HelmRelease, hc *sourcev1.HelmChart) {
				hr.Spec.Chart.Spec.Chart = "new"
			},
			want: true,
		},
		{
			name: "detects version change",
			modify: func(hr *v2.HelmRelease, hc *sourcev1.HelmChart) {
				hr.Spec.Chart.Spec.Version = "2.0.0"
			},
			want: true,
		},
		{
			name: "detects chart source name change",
			modify: func(hr *v2.HelmRelease, hc *sourcev1.HelmChart) {
				hr.Spec.Chart.Spec.SourceRef.Name = "new"
			},
			want: true,
		},
		{
			name: "detects chart source kind change",
			modify: func(hr *v2.HelmRelease, hc *sourcev1.HelmChart) {
				hr.Spec.Chart.Spec.SourceRef.Kind = "GitRepository"
			},
			want: true,
		},
		{
			name: "detects interval change",
			modify: func(hr *v2.HelmRelease, hc *sourcev1.HelmChart) {
				hr.Spec.Chart.Spec.Interval = nil
			},
			want: true,
		},
		{
			name: "detects values files change",
			modify: func(hr *v2.HelmRelease, hc *sourcev1.HelmChart) {
				hr.Spec.Chart.Spec.ValuesFiles = []string{"values-prod.yaml"}
			},
			want: true,
		},
		{
			name: "detects values file change",
			modify: func(hr *v2.HelmRelease, hc *sourcev1.HelmChart) {
				hr.Spec.Chart.Spec.ValuesFile = "values-prod.yaml"
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			hr := hrWithChartTemplate.DeepCopy()
			hc := buildHelmChartFromTemplate(hr)
			g.Expect(helmChartRequiresUpdate(hr, hc)).To(Equal(false))

			tt.modify(hr, hc)
			g.Expect(helmChartRequiresUpdate(hr, hc)).To(Equal(tt.want))
		})
	}
}
