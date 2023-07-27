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

package controller

import (
	"context"
	"github.com/fluxcd/helm-controller/internal/acl"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
)

func TestHelmReleaseReconciler_getHelmChart(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(v2.AddToScheme(scheme)).To(Succeed())
	g.Expect(sourcev1.AddToScheme(scheme)).To(Succeed())

	chart := &sourcev1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "some-namespace",
			Name:      "some-chart-name",
		},
	}

	tests := []struct {
		name            string
		rel             *v2.HelmRelease
		chart           *sourcev1.HelmChart
		expectChart     bool
		wantErr         bool
		disallowCrossNS bool
	}{
		{
			name: "retrieves HelmChart object from Status",
			rel: &v2.HelmRelease{
				Status: v2.HelmReleaseStatus{
					HelmChart: "some-namespace/some-chart-name",
				},
			},
			chart:       chart,
			expectChart: true,
		},
		{
			name: "no HelmChart found",
			rel: &v2.HelmRelease{
				Status: v2.HelmReleaseStatus{
					HelmChart: "some-namespace/some-chart-name",
				},
			},
			chart:       nil,
			expectChart: false,
			wantErr:     true,
		},
		{
			name: "no HelmChart in Status",
			rel: &v2.HelmRelease{
				Status: v2.HelmReleaseStatus{
					HelmChart: "",
				},
			},
			chart:       chart,
			expectChart: false,
			wantErr:     true,
		},
		{
			name: "ACL disallows cross namespace",
			rel: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Status: v2.HelmReleaseStatus{
					HelmChart: "some-namespace/some-chart-name",
				},
			},
			chart:           chart,
			expectChart:     false,
			wantErr:         true,
			disallowCrossNS: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder()
			builder.WithScheme(scheme)
			if tt.chart != nil {
				builder.WithObjects(tt.chart)
			}

			r := &HelmReleaseReconciler{
				Client:        builder.Build(),
				EventRecorder: record.NewFakeRecorder(32),
			}

			curAllow := acl.AllowCrossNamespaceRef
			acl.AllowCrossNamespaceRef = !tt.disallowCrossNS
			t.Cleanup(func() { acl.AllowCrossNamespaceRef = !curAllow })

			got, err := r.getHelmChart(context.TODO(), tt.rel)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			expect := g.Expect(got.ObjectMeta)
			if tt.expectChart {
				expect.To(BeEquivalentTo(tt.chart.ObjectMeta))
			} else {
				expect.To(BeNil())
			}
		})
	}
}

func TestValuesReferenceValidation(t *testing.T) {
	tests := []struct {
		name       string
		references []v2.ValuesReference
		wantErr    bool
	}{
		{
			name: "valid ValuesKey",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: "any-key_na.me",
				},
			},
			wantErr: false,
		},
		{
			name: "valid ValuesKey: empty",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: "",
				},
			},
			wantErr: false,
		},
		{
			name: "valid ValuesKey: long",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: strings.Repeat("a", 253),
				},
			},
			wantErr: false,
		},
		{
			name: "invalid ValuesKey",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: "a($&^%b",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid ValuesKey: too long",
			references: []v2.ValuesReference{
				{
					Kind:      "Secret",
					Name:      "values",
					ValuesKey: strings.Repeat("a", 254),
				},
			},
			wantErr: true,
		},
		{
			name: "valid target path: empty",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					TargetPath: "",
				},
			},
			wantErr: false,
		},
		{
			name: "valid target path",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					TargetPath: "list_with.nested-values.and.index[0]",
				},
			},
			wantErr: false,
		},
		{
			name: "valid target path: long",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					TargetPath: strings.Repeat("a", 250),
				},
			},
			wantErr: false,
		},
		{
			name: "invalid target path: too long",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					TargetPath: strings.Repeat("a", 251),
				},
			},
			wantErr: true,
		},
		{
			name: "invalid target path: opened index",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					ValuesKey:  "single",
					TargetPath: "a[",
				},
			},
			wantErr: true,
		},
		{
			name: "invalid target path: incorrect index syntax",
			references: []v2.ValuesReference{
				{
					Kind:       "Secret",
					Name:       "values",
					ValuesKey:  "single",
					TargetPath: "a]0[",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var values *apiextensionsv1.JSON
			v, _ := yaml.YAMLToJSON([]byte("values"))
			values = &apiextensionsv1.JSON{Raw: v}

			hr := v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test",
					Namespace: "default",
				},
				Spec: v2.HelmReleaseSpec{
					Interval: metav1.Duration{Duration: 5 * time.Minute},
					Chart: v2.HelmChartTemplate{
						Spec: v2.HelmChartTemplateSpec{
							Chart: "mychart",
							SourceRef: v2.CrossNamespaceObjectReference{
								Name: "something",
							},
						},
					},
					ValuesFrom: tt.references,
					Values:     values,
				},
			}

			err := testEnv.Create(context.TODO(), &hr, client.DryRunAll)
			if (err != nil) != tt.wantErr {
				t.Errorf("composeValues() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
		})
	}
}
