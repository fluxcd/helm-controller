/*
Copyright 2023 The Flux authors

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

package diff

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/fluxcd/cli-utils/pkg/kstatus/polling"
	"github.com/fluxcd/cli-utils/pkg/object"
	runtimeClient "github.com/fluxcd/pkg/runtime/client"
	"github.com/fluxcd/pkg/ssa"

	"helm.sh/helm/v3/pkg/release"
)

func TestDiffer_Diff(t *testing.T) {
	scheme, mapper := testSchemeWithMapper()

	// We do not test all the possible scenarios here, as the ssa package is
	// already tested in depth. We only test the integration with the ssa package.
	tests := []struct {
		name      string
		client    client.Client
		rel       *release.Release
		want      *ssa.ChangeSet
		wantDrift bool
		wantErr   string
	}{
		{
			name:   "manifest read error",
			client: fake.NewClientBuilder().Build(),
			rel: &release.Release{
				Manifest: "invalid",
			},
			wantErr: "failed to read objects from release manifest",
		},
		{
			name:   "error on failure to determine namespace scope",
			client: fake.NewClientBuilder().Build(),
			rel: &release.Release{
				Namespace: "release",
				Manifest: `apiVersion: v1
kind: Secret
metadata:
  name: test
stringData:
  foo: bar
`,
			},
			wantErr: "failed to determine if Secret is namespace scoped",
		},
		{
			name: "detects changes",
			client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithRESTMapper(mapper).
				Build(),
			rel: &release.Release{
				Namespace: "release",
				Manifest: `---
apiVersion: v1
kind: Secret
metadata:
  name: test
stringData:
  foo: bar
---
apiVersion: v1
kind: Secret
metadata:
  name: test-ns
  namespace: other
stringData:
  foo: bar
`,
			},
			want: &ssa.ChangeSet{
				Entries: []ssa.ChangeSetEntry{
					{
						ObjMetadata: object.ObjMetadata{
							Namespace: "release",
							Name:      "test",
							GroupKind: schema.GroupKind{
								Kind: "Secret",
							},
						},
						GroupVersion: "v1",
						Subject:      "Secret/release/test",
						Action:       ssa.CreatedAction,
					},
					{
						ObjMetadata: object.ObjMetadata{
							Namespace: "other",
							Name:      "test-ns",
							GroupKind: schema.GroupKind{
								Kind: "Secret",
							},
						},
						GroupVersion: "v1",
						Subject:      "Secret/other/test-ns",
						Action:       ssa.CreatedAction,
					},
				},
			},
			wantDrift: true,
		},
		{
			name: "ignores exclusions",
			client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithRESTMapper(mapper).
				Build(),
			rel: &release.Release{
				Namespace: "release",
				Manifest: fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: test
  labels:
    %[1]s: %[2]s
stringData:
    foo: bar
---
apiVersion: v1
kind: Secret
metadata:
  name: test2
stringData:
  foo: bar
`, MetadataKey, MetadataDisabledValue),
			},
			want: &ssa.ChangeSet{
				Entries: []ssa.ChangeSetEntry{
					{
						ObjMetadata: object.ObjMetadata{
							Namespace: "release",
							Name:      "test",
							GroupKind: schema.GroupKind{
								Kind: "Secret",
							},
						},
						GroupVersion: "v1",
						Subject:      "Secret/release/test",
						Action:       ssa.SkippedAction,
					},
					{
						ObjMetadata: object.ObjMetadata{
							Namespace: "release",
							Name:      "test2",
							GroupKind: schema.GroupKind{
								Kind: "Secret",
							},
						},
						GroupVersion: "v1",
						Subject:      "Secret/release/test2",
						Action:       ssa.CreatedAction,
					},
				},
			},
			wantDrift: true,
		},
		{
			name: "ignores exclusions (without diff)",
			client: fake.NewClientBuilder().
				WithScheme(scheme).
				WithRESTMapper(mapper).
				Build(),
			rel: &release.Release{
				Namespace: "release",
				Manifest: fmt.Sprintf(`---
apiVersion: v1
kind: Secret
metadata:
  name: test
  labels:
    %[1]s: %[2]s
stringData:
    foo: bar`, MetadataKey, MetadataDisabledValue),
			},
			want: &ssa.ChangeSet{
				Entries: []ssa.ChangeSetEntry{
					{
						ObjMetadata: object.ObjMetadata{
							Namespace: "release",
							Name:      "test",
							GroupKind: schema.GroupKind{
								Kind: "Secret",
							},
						},
						GroupVersion: "v1",
						Subject:      "Secret/release/test",
						Action:       ssa.SkippedAction,
					},
				},
			},
			wantDrift: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			d := NewDiffer(runtimeClient.NewImpersonator(tt.client, nil, polling.Options{}, nil, runtimeClient.KubeConfigOptions{}, "", "", ""), "test-controller")
			got, drift, err := d.Diff(context.TODO(), tt.rel)

			if tt.wantErr != "" {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.wantErr))
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}

			g.Expect(got).To(Equal(tt.want))
			g.Expect(drift).To(Equal(tt.wantDrift))
		})
	}
}

func testSchemeWithMapper() (*runtime.Scheme, meta.RESTMapper) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{corev1.SchemeGroupVersion})
	mapper.Add(corev1.SchemeGroupVersion.WithKind("Secret"), meta.RESTScopeNamespace)
	return scheme, mapper
}
