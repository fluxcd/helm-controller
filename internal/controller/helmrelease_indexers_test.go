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
	"testing"

	"github.com/google/go-cmp/cmp"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/fluxcd/pkg/apis/meta"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

func TestHelmReleaseReconciler_requestsForSourceChange_dependencyOrder(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		source   client.Object
		requests func(*HelmReleaseReconciler, context.Context, client.Object) []reconcile.Request
	}{
		{
			name: "HelmChart",
			kind: sourcev1.HelmChartKind,
			source: &sourcev1.HelmChart{
				ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "default"},
				Status: sourcev1.HelmChartStatus{
					Artifact: &meta.Artifact{Revision: "1.0.0"},
				},
			},
			requests: (*HelmReleaseReconciler).requestsForHelmChartChange,
		},
		{
			name: "OCIRepository",
			kind: sourcev1.OCIRepositoryKind,
			source: &sourcev1.OCIRepository{
				ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "default"},
				Status: sourcev1.OCIRepositoryStatus{
					Artifact: &meta.Artifact{Revision: "1.0.0@sha256:0123456789abcdef"},
				},
			},
			requests: (*HelmReleaseReconciler).requestsForOCIRepositoryChange,
		},
		{
			name: "ExternalArtifact",
			kind: sourcev1.ExternalArtifactKind,
			source: &sourcev1.ExternalArtifact{
				ObjectMeta: metav1.ObjectMeta{Name: "shared", Namespace: "default"},
				Status: sourcev1.ExternalArtifactStatus{
					Artifact: &meta.Artifact{Revision: "1.0.0"},
				},
			},
			requests: (*HelmReleaseReconciler).requestsForExternalArtifactChange,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dependent := &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{Name: "a-dependent", Namespace: "default"},
				Spec: v2.HelmReleaseSpec{
					ChartRef:  &v2.CrossNamespaceSourceReference{Kind: tt.kind, Name: "shared"},
					DependsOn: []v2.DependencyReference{{Name: "z-dependency"}},
				},
			}
			dependency := &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{Name: "z-dependency", Namespace: "default"},
				Spec: v2.HelmReleaseSpec{
					ChartRef: &v2.CrossNamespaceSourceReference{Kind: tt.kind, Name: "shared"},
				},
			}

			c := fake.NewClientBuilder().
				WithScheme(NewTestScheme()).
				WithObjects(dependent, dependency).
				WithIndex(&v2.HelmRelease{}, v2.SourceIndexKey, func(o client.Object) []string {
					hr := o.(*v2.HelmRelease)
					return []string{hr.Spec.ChartRef.Kind + "/" + hr.Namespace + "/" + hr.Spec.ChartRef.Name}
				}).
				Build()
			r := &HelmReleaseReconciler{Client: c}

			got := tt.requests(r, context.Background(), tt.source)
			want := []reconcile.Request{
				{NamespacedName: client.ObjectKey{Namespace: "default", Name: "z-dependency"}},
				{NamespacedName: client.ObjectKey{Namespace: "default", Name: "a-dependent"}},
			}
			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("requestsForSourceChange(%s) mismatch (-want +got):\n%s", tt.kind, diff)
			}
		})
	}
}
