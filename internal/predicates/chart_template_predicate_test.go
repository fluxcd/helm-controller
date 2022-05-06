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

package predicates

import (
	"testing"

	"github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/fluxcd/helm-controller/api/v2beta1"
)

func TestChartTemplateChangePredicate_Create(t *testing.T) {
	obj := &v2beta1.HelmRelease{Spec: v2beta1.HelmReleaseSpec{}}
	suspended := &v2beta1.HelmRelease{Spec: v2beta1.HelmReleaseSpec{Suspend: true}}
	not := &unstructured.Unstructured{}

	tests := []struct {
		name string
		obj  client.Object
		want bool
	}{
		{name: "new", obj: obj, want: true},
		{name: "suspended", obj: suspended, want: true},
		{name: "not a HelmRelease", obj: not, want: false},
		{name: "nil", obj: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			so := ChartTemplateChangePredicate{}
			e := event.CreateEvent{
				Object: tt.obj,
			}
			g.Expect(so.Create(e)).To(gomega.Equal(tt.want))
		})
	}
}

func TestChartTemplateChangePredicate_Update(t *testing.T) {
	templateA := &v2beta1.HelmRelease{Spec: v2beta1.HelmReleaseSpec{
		Chart: v2beta1.HelmChartTemplate{
			Spec: v2beta1.HelmChartTemplateSpec{
				Chart: "chart-name-a",
				SourceRef: v2beta1.CrossNamespaceObjectReference{
					Name: "repository",
					Kind: "HelmRepository",
				},
			},
		},
	}}
	templateB := &v2beta1.HelmRelease{Spec: v2beta1.HelmReleaseSpec{
		Chart: v2beta1.HelmChartTemplate{
			Spec: v2beta1.HelmChartTemplateSpec{
				Chart: "chart-name-b",
				SourceRef: v2beta1.CrossNamespaceObjectReference{
					Name: "repository",
					Kind: "HelmRepository",
				},
			},
		},
	}}
	templateWithMetaA := &v2beta1.HelmRelease{Spec: v2beta1.HelmReleaseSpec{
		Chart: v2beta1.HelmChartTemplate{
			ObjectMeta: &v2beta1.HelmChartTemplateObjectMeta{
				Labels: map[string]string{
					"key": "value",
				},
				Annotations: map[string]string{
					"key": "value",
				},
			},
			Spec: v2beta1.HelmChartTemplateSpec{
				Chart: "chart-name-a",
				SourceRef: v2beta1.CrossNamespaceObjectReference{
					Name: "repository",
					Kind: "HelmRepository",
				},
			},
		},
	}}
	templateWithMetaB := &v2beta1.HelmRelease{Spec: v2beta1.HelmReleaseSpec{
		Chart: v2beta1.HelmChartTemplate{
			ObjectMeta: &v2beta1.HelmChartTemplateObjectMeta{
				Labels: map[string]string{
					"key": "new-value",
				},
				Annotations: map[string]string{
					"key": "new-value",
				},
			},
		},
	}}
	empty := &v2beta1.HelmRelease{}
	suspended := &v2beta1.HelmRelease{Spec: v2beta1.HelmReleaseSpec{Suspend: true}}
	not := &unstructured.Unstructured{}

	tests := []struct {
		name string
		old  client.Object
		new  client.Object
		want bool
	}{
		{name: "same template", old: templateA, new: templateA, want: false},
		{name: "diff template", old: templateA, new: templateB, want: true},
		{name: "same template with meta", old: templateWithMetaA, new: templateWithMetaA, want: false},
		{name: "diff template with meta", old: templateWithMetaA, new: templateWithMetaB, want: true},
		{name: "new with template", old: empty, new: templateA, want: true},
		{name: "old with template", old: templateA, new: empty, want: true},
		{name: "new suspended", old: templateA, new: suspended, want: true},
		{name: "old suspended new template", old: suspended, new: templateA, want: true},
		{name: "old not a HelmRelease", old: not, new: templateA, want: false},
		{name: "new not a HelmRelease", old: templateA, new: not, want: false},
		{name: "old nil", old: nil, new: templateA, want: false},
		{name: "new nil", old: templateA, new: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			so := ChartTemplateChangePredicate{}
			e := event.UpdateEvent{
				ObjectOld: tt.old,
				ObjectNew: tt.new,
			}
			g.Expect(so.Update(e)).To(gomega.Equal(tt.want))
		})
	}
}

func TestChartTemplateChangePredicate_Delete(t *testing.T) {
	obj := &v2beta1.HelmRelease{Spec: v2beta1.HelmReleaseSpec{}}
	suspended := &v2beta1.HelmRelease{Spec: v2beta1.HelmReleaseSpec{Suspend: true}}
	not := &unstructured.Unstructured{}

	tests := []struct {
		name string
		obj  client.Object
		want bool
	}{
		{name: "object", obj: obj, want: true},
		{name: "suspended", obj: suspended, want: true},
		{name: "not a HelmRelease", obj: not, want: false},
		{name: "nil", obj: nil, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := gomega.NewWithT(t)

			so := ChartTemplateChangePredicate{}
			e := event.DeleteEvent{
				Object: tt.obj,
			}
			g.Expect(so.Delete(e)).To(gomega.Equal(tt.want))
		})
	}
}
