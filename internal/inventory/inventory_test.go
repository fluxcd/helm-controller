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

package inventory

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"helm.sh/helm/v4/pkg/chart/common"
	helmchart "helm.sh/helm/v4/pkg/chart/v2"
)

func TestNew(t *testing.T) {
	g := NewWithT(t)

	inv := New()

	g.Expect(inv).ToNot(BeNil())
	g.Expect(inv.Entries).To(BeEmpty())
}

func TestAddManifest(t *testing.T) {
	scheme := runtime.NewScheme()
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(rbacv1.AddToScheme(scheme))

	fakeMapper := newFakeRESTMapper()

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRESTMapper(fakeMapper).
		Build()

	tests := []struct {
		name             string
		manifest         string
		releaseNamespace string
		expectedLen      int
		expectedIDs      []string
		expectError      bool
	}{
		{
			name: "namespace specified in manifest",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: mycm
  namespace: default
`,
			releaseNamespace: "other",
			expectedLen:      1,
			expectedIDs:      []string{"default_mycm__ConfigMap"},
		},
		{
			name: "namespace complement for namespaced resource",
			manifest: `apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
`,
			releaseNamespace: "foo",
			expectedLen:      1,
			expectedIDs:      []string{"foo_myapp_apps_Deployment"},
		},
		{
			name: "cluster-scoped resource not complemented",
			manifest: `apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: myrole
`,
			releaseNamespace: "foo",
			expectedLen:      1,
			expectedIDs:      []string{"_myrole_rbac.authorization.k8s.io_ClusterRole"},
		},
		{
			name: "multiple documents",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: mycm
  namespace: default
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: myapp
`,
			releaseNamespace: "bar",
			expectedLen:      2,
			expectedIDs:      []string{"default_mycm__ConfigMap", "bar_myapp_apps_Deployment"},
		},
		{
			name: "hook resources excluded",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: hook
  namespace: default
  annotations:
    "helm.sh/hook": post-install
data:
  foo: bar
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: regular
  namespace: default
data:
  foo: bar
`,
			releaseNamespace: "default",
			expectedLen:      1,
			expectedIDs:      []string{"default_regular__ConfigMap"},
		},
		{
			name: "all hooks excluded results in empty inventory",
			manifest: `apiVersion: v1
kind: ConfigMap
metadata:
  name: pre-install-hook
  namespace: default
  annotations:
    "helm.sh/hook": pre-install
`,
			releaseNamespace: "default",
			expectedLen:      0,
			expectedIDs:      []string{},
		},
		{
			name:             "invalid YAML returns error",
			manifest:         "not: valid: yaml: content",
			releaseNamespace: "default",
			expectError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			inv := New()
			_, err := AddManifest(inv, tt.manifest, tt.releaseNamespace, fakeClient)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring("failed to parse manifest"))
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(inv.Entries).To(HaveLen(tt.expectedLen))
			for i, entry := range inv.Entries {
				g.Expect(entry.ID).To(Equal(tt.expectedIDs[i]))
			}
		})
	}
}

func TestAddManifest_RESTMapperError(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	emptyMapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{})
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRESTMapper(emptyMapper).
		Build()

	manifest := `apiVersion: v1
kind: ConfigMap
metadata:
  name: mycm
`

	inv := New()
	warnings, err := AddManifest(inv, manifest, "default", fakeClient)

	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(inv.Entries).To(HaveLen(1))
	g.Expect(inv.Entries[0].ID).To(Equal("_mycm__ConfigMap"))
	g.Expect(warnings).To(HaveLen(1))
	g.Expect(warnings[0]).To(ContainSubstring("failed to determine if ConfigMap is namespace scoped"))
}

// newFakeRESTMapper creates a RESTMapper with common Kubernetes resource types,
// correctly distinguishing between namespaced and cluster-scoped resources.
func newFakeRESTMapper() meta.RESTMapper {
	mapper := meta.NewDefaultRESTMapper([]schema.GroupVersion{
		corev1.SchemeGroupVersion,
		appsv1.SchemeGroupVersion,
		rbacv1.SchemeGroupVersion,
	})

	// Namespaced resources
	mapper.Add(corev1.SchemeGroupVersion.WithKind("ConfigMap"), meta.RESTScopeNamespace)
	mapper.Add(appsv1.SchemeGroupVersion.WithKind("Deployment"), meta.RESTScopeNamespace)

	// Cluster-scoped resources
	mapper.Add(rbacv1.SchemeGroupVersion.WithKind("ClusterRole"), meta.RESTScopeRoot)

	return mapper
}

func TestAddCRDs(t *testing.T) {
	modTime := time.Now()

	tests := []struct {
		name        string
		crdFiles    []*common.File
		expectedLen int
		expectedIDs []string
	}{
		{
			name: "CRD is added to inventory",
			crdFiles: []*common.File{
				{
					Name:    "crds/mycrd.yaml",
					ModTime: modTime,
					Data: []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: myresources.example.com
spec:
  group: example.com
  names:
    kind: MyResource
    plural: myresources
  scope: Namespaced
  versions:
  - name: v1
    served: true
    storage: true
`),
				},
			},
			expectedLen: 1,
			expectedIDs: []string{"_myresources.example.com_apiextensions.k8s.io_CustomResourceDefinition"},
		},
		{
			name: "multiple CRDs in single file",
			crdFiles: []*common.File{
				{
					Name:    "crds/crds.yaml",
					ModTime: modTime,
					Data: []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: foos.example.com
---
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: bars.example.com
`),
				},
			},
			expectedLen: 2,
			expectedIDs: []string{
				"_foos.example.com_apiextensions.k8s.io_CustomResourceDefinition",
				"_bars.example.com_apiextensions.k8s.io_CustomResourceDefinition",
			},
		},
		{
			name: "CRDs in multiple files",
			crdFiles: []*common.File{
				{
					Name:    "crds/crd1.yaml",
					ModTime: modTime,
					Data: []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: foos.example.com
`),
				},
				{
					Name:    "crds/crd2.yaml",
					ModTime: modTime,
					Data: []byte(`apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: bars.example.com
`),
				},
			},
			expectedLen: 2,
			expectedIDs: []string{
				"_foos.example.com_apiextensions.k8s.io_CustomResourceDefinition",
				"_bars.example.com_apiextensions.k8s.io_CustomResourceDefinition",
			},
		},
		{
			name:        "no CRDs",
			crdFiles:    []*common.File{},
			expectedLen: 0,
			expectedIDs: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			chart := &helmchart.Chart{
				Metadata: &helmchart.Metadata{
					Name:    "test-chart",
					Version: "1.0.0",
				},
				Files: tt.crdFiles,
			}

			inv := New()
			err := AddCRDs(inv, chart)

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(inv.Entries).To(HaveLen(tt.expectedLen))
			for i, entry := range inv.Entries {
				g.Expect(entry.ID).To(Equal(tt.expectedIDs[i]))
			}
		})
	}
}
