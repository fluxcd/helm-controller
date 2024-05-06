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

package action

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	. "github.com/onsi/gomega"
	extjsondiff "github.com/wI2L/jsondiff"
	helmaction "helm.sh/helm/v3/pkg/action"
	helmrelease "helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	apierrutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/fluxcd/pkg/apis/kustomize"
	"github.com/fluxcd/pkg/ssa"
	"github.com/fluxcd/pkg/ssa/jsondiff"
	ssanormalize "github.com/fluxcd/pkg/ssa/normalize"
	ssautil "github.com/fluxcd/pkg/ssa/utils"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/kube"
)

func TestDiff(t *testing.T) {
	// Normally, we would create e.g. a `suite_test.go` file with a `TestMain`
	// function. As this is one of the few tests in this package which needs a
	// test cluster, we create it here instead.
	config, cleanup := newTestCluster(t)
	t.Cleanup(func() {
		t.Log("Stopping the test environment")
		if err := cleanup(); err != nil {
			t.Logf("Failed to stop the test environment: %v", err)
		}
	})

	// Construct a REST client getter for Helm's action configuration.
	getter := kube.NewMemoryRESTClientGetter(config)

	// Construct a client for to be able to mutate the cluster.
	c, err := client.New(config, client.Options{})
	if err != nil {
		t.Fatalf("Failed to create client for test environment: %v", err)
	}

	const testOwner = "helm-controller"

	tests := []struct {
		name          string
		manifest      string
		ignoreRules   []v2.IgnoreRule
		mutateCluster func(objs []*unstructured.Unstructured, namespace string) ([]*unstructured.Unstructured, error)
		want          func(namespace string, desired, cluster []*unstructured.Unstructured) jsondiff.DiffSet
		wantErr       bool
	}{
		{
			name: "detects drift",
			manifest: `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: changed
data:
  key: value
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: deleted
data:
  key: value
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: unchanged
data:
  key: value`,
			mutateCluster: func(objs []*unstructured.Unstructured, namespace string) ([]*unstructured.Unstructured, error) {
				var clusterObjs []*unstructured.Unstructured
				for _, obj := range objs {
					if obj.GetName() == "deleted" {
						continue
					}

					obj := obj.DeepCopy()
					obj.SetNamespace(namespace)

					if obj.GetName() == "changed" {
						if err := unstructured.SetNestedField(obj.Object, "changed", "data", "key"); err != nil {
							return nil, fmt.Errorf("failed to set nested field: %w", err)
						}
					}

					clusterObjs = append(clusterObjs, obj)
				}
				return clusterObjs, nil
			},
			want: func(namespace string, desired, cluster []*unstructured.Unstructured) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type:          jsondiff.DiffTypeUpdate,
						DesiredObject: namespacedUnstructured(desired[0], namespace),
						ClusterObject: cluster[0],
						Patch: extjsondiff.Patch{
							{
								Type:     extjsondiff.OperationReplace,
								OldValue: "changed",
								Value:    "value",
								Path:     "/data/key",
							},
						},
					},
					{
						Type:          jsondiff.DiffTypeCreate,
						DesiredObject: namespacedUnstructured(desired[1], namespace),
					},
					{
						Type:          jsondiff.DiffTypeNone,
						DesiredObject: namespacedUnstructured(desired[2], namespace),
						ClusterObject: cluster[1],
					},
				}
			},
		},
		{
			name:     "empty release manifest",
			manifest: "",
		},
		{
			name: "manifest with disabled annotation",
			manifest: fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: disabled
  annotations:
    %[1]s: %[2]s
data:
  key: value`, v2.DriftDetectionMetadataKey, v2.DriftDetectionDisabledValue),
			mutateCluster: func(objs []*unstructured.Unstructured, namespace string) ([]*unstructured.Unstructured, error) {
				var clusterObjs []*unstructured.Unstructured
				for _, obj := range objs {
					obj := obj.DeepCopy()
					obj.SetNamespace(namespace)
					if err := unstructured.SetNestedField(obj.Object, "changed", "data", "key"); err != nil {
						return nil, fmt.Errorf("failed to set nested field: %w", err)
					}
					clusterObjs = append(clusterObjs, obj)
				}
				return clusterObjs, nil
			},
			want: func(namespace string, desired, cluster []*unstructured.Unstructured) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type:          jsondiff.DiffTypeExclude,
						DesiredObject: namespacedUnstructured(desired[0], namespace),
					},
				}
			},
		},
		{
			name: "adheres to ignore rules",
			manifest: `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: fully-ignored
data:
  key: value
---
apiVersion: v1
kind: Secret
metadata:
  name: partially-ignored
stringData:
  key: value
  otherKey: otherValue
---
apiVersion: v1
kind: Secret
metadata:
  name: globally-ignored
stringData:
  globalKey: globalValue
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: not-ignored
data:
  key: value`,
			mutateCluster: func(objs []*unstructured.Unstructured, namespace string) ([]*unstructured.Unstructured, error) {
				var clusterObjs []*unstructured.Unstructured
				for _, obj := range objs {
					obj := obj.DeepCopy()
					obj.SetNamespace(namespace)

					switch obj.GetName() {
					case "fully-ignored", "not-ignored":
						if err := unstructured.SetNestedField(obj.Object, "changed", "data", "key"); err != nil {
							return nil, fmt.Errorf("failed to set nested field: %w", err)
						}
					case "partially-ignored":
						if err := unstructured.SetNestedField(obj.Object, "changed", "stringData", "key"); err != nil {
							return nil, fmt.Errorf("failed to set nested field: %w", err)
						}
						if err := unstructured.SetNestedField(obj.Object, "changed", "stringData", "otherKey"); err != nil {
							return nil, fmt.Errorf("failed to set nested field: %w", err)
						}
					case "globally-ignored":
						if err := unstructured.SetNestedField(obj.Object, "changed", "stringData", "globalKey"); err != nil {
							return nil, fmt.Errorf("failed to set nested field: %w", err)
						}
					}
					clusterObjs = append(clusterObjs, obj)
				}
				return clusterObjs, nil
			},
			ignoreRules: []v2.IgnoreRule{
				{Target: &kustomize.Selector{Name: "fully-ignored"}, Paths: []string{""}},
				{Target: &kustomize.Selector{Name: "partially-ignored"}, Paths: []string{"/data/key"}},
				{Paths: []string{"/data/globalKey"}},
			},
			want: func(namespace string, desired, cluster []*unstructured.Unstructured) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type:          jsondiff.DiffTypeExclude,
						DesiredObject: namespacedUnstructured(desired[0], namespace),
					},
					{
						Type:          jsondiff.DiffTypeUpdate,
						DesiredObject: namespacedUnstructured(desired[1], namespace),
						ClusterObject: cluster[1],
						Patch: extjsondiff.Patch{
							{
								Type:     extjsondiff.OperationReplace,
								Path:     "/data/otherKey",
								OldValue: base64.StdEncoding.EncodeToString([]byte("changed")),
								Value:    base64.StdEncoding.EncodeToString([]byte("otherValue")),
							},
						},
					},
					{
						Type:          jsondiff.DiffTypeNone,
						DesiredObject: namespacedUnstructured(desired[2], namespace),
						ClusterObject: cluster[2],
					},
					{
						Type:          jsondiff.DiffTypeUpdate,
						DesiredObject: namespacedUnstructured(desired[3], namespace),
						ClusterObject: cluster[3],
						Patch: extjsondiff.Patch{
							{
								Type:     extjsondiff.OperationReplace,
								Path:     "/data/key",
								OldValue: "changed",
								Value:    "value",
							},
						},
					},
				}
			},
		},
		{
			name: "configures namespace",
			manifest: `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: without-namespace
data:
  key: value
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: with-namespace
  namespace: diff-fixed-ns
data:
  key: value`,
			mutateCluster: func(objs []*unstructured.Unstructured, namespace string) ([]*unstructured.Unstructured, error) {
				var clusterObjs []*unstructured.Unstructured

				otherNS := unstructured.Unstructured{Object: map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "Namespace",
					"metadata": map[string]interface{}{
						"name": "diff-fixed-ns",
					},
				}}
				clusterObjs = append(clusterObjs, &otherNS)

				for _, obj := range objs {
					obj := obj.DeepCopy()
					if obj.GetNamespace() == "" {
						obj.SetNamespace(namespace)
					}
					clusterObjs = append(clusterObjs, obj)
				}
				return clusterObjs, nil
			},
			want: func(namespace string, desired, cluster []*unstructured.Unstructured) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type:          jsondiff.DiffTypeNone,
						DesiredObject: namespacedUnstructured(desired[0], namespace),
						ClusterObject: cluster[1],
					},
					{
						Type:          jsondiff.DiffTypeNone,
						DesiredObject: namespacedUnstructured(desired[1], desired[1].GetNamespace()),
						ClusterObject: cluster[2],
					},
				}
			},
		},
		{
			name: "configures Helm metadata",
			manifest: `---
apiVersion: v1
kind: ConfigMap
metadata:
  name: without-helm-metadata
data:
  key: value`,
			mutateCluster: func(objs []*unstructured.Unstructured, namespace string) ([]*unstructured.Unstructured, error) {
				var clusterObjs []*unstructured.Unstructured
				for _, obj := range objs {
					obj := obj.DeepCopy()
					if obj.GetNamespace() == "" {
						obj.SetNamespace(namespace)
					}
					obj.SetAnnotations(nil)
					obj.SetLabels(nil)
					clusterObjs = append(clusterObjs, obj)
				}
				return clusterObjs, nil
			},
			want: func(namespace string, desired, cluster []*unstructured.Unstructured) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type:          jsondiff.DiffTypeUpdate,
						DesiredObject: namespacedUnstructured(desired[0], namespace),
						ClusterObject: cluster[0],
						Patch: extjsondiff.Patch{
							{
								Type: extjsondiff.OperationAdd,
								Path: "/metadata",
								Value: map[string]interface{}{
									"labels": map[string]interface{}{
										appManagedByLabel: appManagedByHelm,
									},
									"annotations": map[string]interface{}{
										helmReleaseNameAnnotation:      "configures Helm metadata",
										helmReleaseNamespaceAnnotation: namespace,
									},
								},
							},
						},
					},
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			t.Cleanup(cancel)

			ns, err := generateNamespace(ctx, c, "diff-action")
			if err != nil {
				t.Fatalf("Failed to generate namespace: %v", err)
			}
			t.Cleanup(func() {
				if err := c.Delete(context.Background(), ns); client.IgnoreNotFound(err) != nil {
					t.Logf("Failed to delete generated namespace: %v", err)
				}
			})

			rls := &helmrelease.Release{Name: tt.name, Namespace: ns.Name, Manifest: tt.manifest}

			objs, err := ssautil.ReadObjects(strings.NewReader(tt.manifest))
			if err != nil {
				t.Fatalf("Failed to read release objects: %v", err)
			}
			for _, obj := range objs {
				setHelmMetadata(obj, rls)
			}

			clusterObjs := objs
			if tt.mutateCluster != nil {
				if clusterObjs, err = tt.mutateCluster(objs, ns.Name); err != nil {
					t.Fatalf("Failed to modify cluster resource: %v", err)
				}
			}

			t.Cleanup(func() {
				for _, obj := range clusterObjs {
					if err := c.Delete(context.Background(), obj); client.IgnoreNotFound(err) != nil {
						t.Logf("Failed to delete object: %v", err)
					}
				}
			})

			for _, obj := range clusterObjs {
				if err = ssanormalize.Unstructured(obj); err != nil {
					t.Fatalf("Failed to normalize cluster manifest: %v", err)
				}
				if err := c.Create(ctx, obj, client.FieldOwner(testOwner)); err != nil {
					t.Fatalf("Failed to create object: %v", err)
				}
			}

			got, err := Diff(ctx, &helmaction.Configuration{RESTClientGetter: getter}, rls, testOwner, tt.ignoreRules...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Diff() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			var want jsondiff.DiffSet
			if tt.want != nil {
				want = tt.want(ns.Name, objs, clusterObjs)
			}
			if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(extjsondiff.Operation{})); diff != "" {
				t.Errorf("Diff() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestApplyDiff(t *testing.T) {
	// Normally, we would create e.g. a `suite_test.go` file with a `TestMain`
	// function. As this is one of the few tests in this package which needs a
	// test cluster, we create it here instead.
	config, cleanup := newTestCluster(t)
	t.Cleanup(func() {
		t.Log("Stopping the test environment")
		if err := cleanup(); err != nil {
			t.Logf("Failed to stop the test environment: %v", err)
		}
	})

	// Construct a REST client getter for Helm's action configuration.
	getter := kube.NewMemoryRESTClientGetter(config)

	// Construct a client for to be able to mutate the cluster.
	c, err := client.New(config, client.Options{})
	if err != nil {
		t.Fatalf("Failed to create client for test environment: %v", err)
	}

	const testOwner = "helm-controller"

	tests := []struct {
		name    string
		diffSet func(namespace string) jsondiff.DiffSet
		expect  func(g *GomegaWithT, namespace string, got *ssa.ChangeSet, err error)
	}{
		{
			name: "creates and updates resources",
			diffSet: func(namespace string) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type: jsondiff.DiffTypeCreate,
						DesiredObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "Secret",
								"metadata": map[string]interface{}{
									"name":      "test-secret",
									"namespace": namespace,
								},
								"stringData": map[string]interface{}{
									"key": "value",
								},
							},
						},
					},
					{
						Type: jsondiff.DiffTypeUpdate,
						DesiredObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name":      "test-cm",
									"namespace": namespace,
								},
								"data": map[string]interface{}{
									"key": "value",
								},
							},
						},
						ClusterObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name":      "test-cm",
									"namespace": namespace,
								},
								"data": map[string]interface{}{
									"key": "changed",
								},
							},
						},
						Patch: extjsondiff.Patch{
							{
								Type:  extjsondiff.OperationReplace,
								Path:  "/data/key",
								Value: "value",
							},
						},
					},
				}
			},
			expect: func(g *GomegaWithT, namespace string, got *ssa.ChangeSet, err error) {
				g.THelper()

				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(got).NotTo(BeNil())
				g.Expect(got.Entries).To(HaveLen(2))

				g.Expect(got.Entries[0].Subject).To(Equal("Secret/" + namespace + "/test-secret"))
				g.Expect(got.Entries[0].Action).To(Equal(ssa.CreatedAction))
				g.Expect(c.Get(context.TODO(), types.NamespacedName{
					Namespace: namespace,
					Name:      "test-secret",
				}, &corev1.Secret{})).To(Succeed())

				g.Expect(got.Entries[1].Subject).To(Equal("ConfigMap/" + namespace + "/test-cm"))
				g.Expect(got.Entries[1].Action).To(Equal(ssa.ConfiguredAction))
				cm := &corev1.ConfigMap{}
				g.Expect(c.Get(context.TODO(), types.NamespacedName{
					Namespace: namespace,
					Name:      "test-cm",
				}, cm)).To(Succeed())
				g.Expect(cm.Data).To(HaveKeyWithValue("key", "value"))
			},
		},
		{
			name: "continues on error",
			diffSet: func(namespace string) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type: jsondiff.DiffTypeCreate,
						DesiredObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "Secret",
								"metadata": map[string]interface{}{
									"name":      "invalid-test-secret",
									"namespace": namespace,
								},
								"data": map[string]interface{}{
									// Illegal base64 encoded data.
									"key": "secret value",
								},
							},
						},
					},
					{
						Type: jsondiff.DiffTypeCreate,
						DesiredObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name":      "test-cm",
									"namespace": namespace,
								},
							},
						},
					},
					{
						Type: jsondiff.DiffTypeUpdate,
						DesiredObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "Secret",
								"metadata": map[string]interface{}{
									"name":      "invalid-test-secret-update",
									"namespace": namespace,
								},
								"data": map[string]interface{}{
									// Illegal base64 encoded data.
									"key": "secret value2",
								},
							},
						},
						ClusterObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "Secret",
								"metadata": map[string]interface{}{
									"name":      "invalid-test-secret-update",
									"namespace": namespace,
								},
								"stringData": map[string]interface{}{
									"key": "value",
								},
							},
						},
						Patch: extjsondiff.Patch{
							{
								Type: extjsondiff.OperationReplace,
								Path: "/data/key",
								// Illegal base64 encoded data.
								Value: "value",
							},
						},
					},
					{
						Type: jsondiff.DiffTypeUpdate,
						DesiredObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name":      "test-cm-2",
									"namespace": namespace,
								},
								"data": map[string]interface{}{
									"key": "value",
								},
							},
						},
						ClusterObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name":      "test-cm-2",
									"namespace": namespace,
								},
								"data": map[string]interface{}{
									"key": "changed",
								},
							},
						},
						Patch: extjsondiff.Patch{
							{
								Type:  extjsondiff.OperationReplace,
								Path:  "/data/key",
								Value: "value",
							},
						},
					},
				}
			},
			expect: func(g *GomegaWithT, namespace string, got *ssa.ChangeSet, err error) {
				g.THelper()

				g.Expect(err).To(HaveOccurred())
				g.Expect(err.(apierrutil.Aggregate).Errors()).To(HaveLen(2))
				g.Expect(err.Error()).To(ContainSubstring("invalid-test-secret creation failure"))
				g.Expect(err.Error()).To(ContainSubstring("invalid-test-secret-update patch failure"))

				// Verify that the error message does not contain the secret data.
				g.Expect(err.Error()).ToNot(ContainSubstring("secret value"))
				g.Expect(err.Error()).ToNot(ContainSubstring("secret value2"))

				g.Expect(got).NotTo(BeNil())
				g.Expect(got.Entries).To(HaveLen(2))

				g.Expect(got.Entries[0].Subject).To(Equal("ConfigMap/" + namespace + "/test-cm"))
				g.Expect(got.Entries[0].Action).To(Equal(ssa.CreatedAction))
				g.Expect(c.Get(context.TODO(), types.NamespacedName{
					Namespace: namespace,
					Name:      "test-cm",
				}, &corev1.ConfigMap{})).To(Succeed())

				g.Expect(got.Entries[1].Subject).To(Equal("ConfigMap/" + namespace + "/test-cm-2"))
				g.Expect(got.Entries[1].Action).To(Equal(ssa.ConfiguredAction))

				cm2 := &corev1.ConfigMap{}
				g.Expect(c.Get(context.TODO(), types.NamespacedName{
					Namespace: namespace,
					Name:      "test-cm-2",
				}, cm2)).To(Succeed())
				g.Expect(cm2.Data).To(HaveKeyWithValue("key", "value"))
			},
		},
		{
			name: "creates namespace before dependent resources",
			diffSet: func(namespace string) jsondiff.DiffSet {
				otherNS := generateName("test-ns")

				return jsondiff.DiffSet{
					{
						Type: jsondiff.DiffTypeCreate,
						DesiredObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "ConfigMap",
								"metadata": map[string]interface{}{
									"name":      "test-cm",
									"namespace": otherNS,
								},
							},
						},
					},
					{
						Type: jsondiff.DiffTypeCreate,
						DesiredObject: &unstructured.Unstructured{
							Object: map[string]interface{}{
								"apiVersion": "v1",
								"kind":       "Namespace",
								"metadata": map[string]interface{}{
									"name": otherNS,
								},
							},
						},
					},
				}
			},
			expect: func(g *GomegaWithT, namespace string, got *ssa.ChangeSet, err error) {
				g.THelper()

				g.Expect(err).NotTo(HaveOccurred())

				g.Expect(got).NotTo(BeNil())
				g.Expect(got.Entries).To(HaveLen(2))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			t.Cleanup(cancel)

			ns, err := generateNamespace(ctx, c, "diff-action")
			if err != nil {
				t.Fatalf("Failed to generate namespace: %v", err)
			}
			t.Cleanup(func() {
				if err := c.Delete(context.Background(), ns); client.IgnoreNotFound(err) != nil {
					t.Logf("Failed to delete generated namespace: %v", err)
				}
			})

			diff := tt.diffSet(ns.Name)

			for _, d := range diff {
				if d.ClusterObject != nil {
					if err := c.Create(ctx, d.ClusterObject, client.FieldOwner(testOwner)); err != nil {
						t.Fatalf("Failed to create cluster object: %v", err)
					}
					t.Cleanup(func() {
						if err := c.Delete(ctx, d.ClusterObject); client.IgnoreNotFound(err) != nil {
							t.Logf("Failed to delete cluster object: %v", err)
						}
					})
				}
			}

			got, err := ApplyDiff(context.Background(), &helmaction.Configuration{RESTClientGetter: getter}, diff, testOwner)
			tt.expect(g, ns.Name, got, err)
		})
	}
}

// newTestCluster creates a new test cluster and returns a rest.Config and a
// function to stop the test cluster.
func newTestCluster(t *testing.T) (*rest.Config, func() error) {
	t.Helper()

	testEnv := &envtest.Environment{}

	t.Log("Starting the test environment")
	if _, err := testEnv.Start(); err != nil {
		t.Fatalf("Failed to start the test environment: %v", err)
	}

	return testEnv.Config, testEnv.Stop
}

// generateNamespace creates a new namespace with the given generateName and
// returns the namespace object.
func generateNamespace(ctx context.Context, c client.Client, generateName string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-", generateName),
		},
	}
	if err := c.Create(ctx, ns); err != nil {
		return nil, err
	}
	return ns, nil
}

// generateName generates a name with the given name and a random suffix.
func generateName(name string) string {
	return fmt.Sprintf("%s-%s", name, rand.String(5))
}

func namespacedUnstructured(obj *unstructured.Unstructured, namespace string) *unstructured.Unstructured {
	obj = obj.DeepCopy()
	obj.SetNamespace(namespace)
	_ = ssanormalize.Unstructured(obj)
	return obj
}
