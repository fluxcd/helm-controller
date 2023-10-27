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
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	extjsondiff "github.com/wI2L/jsondiff"
	helmaction "helm.sh/helm/v3/pkg/action"
	helmrelease "helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/fluxcd/pkg/apis/kustomize"
	"github.com/fluxcd/pkg/ssa"
	"github.com/fluxcd/pkg/ssa/jsondiff"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/kube"
)

func TestDiff(t *testing.T) {
	// Normally, we would create e.g. a `suite_test.go` file with a `TestMain`
	// function. But because this is the only test in this package which needs
	// a test cluster, we create it here instead.
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
		want          func(namepace string) jsondiff.DiffSet
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
			want: func(namespace string) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type: jsondiff.DiffTypeUpdate,
						GroupVersionKind: schema.GroupVersionKind{
							Version: "v1",
							Kind:    "ConfigMap",
						},
						Namespace: namespace,
						Name:      "changed",
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
						Type: jsondiff.DiffTypeCreate,
						GroupVersionKind: schema.GroupVersionKind{
							Version: "v1",
							Kind:    "ConfigMap",
						},
						Namespace: namespace,
						Name:      "deleted",
					},
					{
						Type: jsondiff.DiffTypeNone,
						GroupVersionKind: schema.GroupVersionKind{
							Version: "v1",
							Kind:    "ConfigMap",
						},
						Namespace: namespace,
						Name:      "unchanged",
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
			want: func(namespace string) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type: jsondiff.DiffTypeExclude,
						GroupVersionKind: schema.GroupVersionKind{
							Version: "v1",
							Kind:    "ConfigMap",
						},
						Namespace: namespace,
						Name:      "disabled",
					},
				}
			},
		},
		{
			name: "manifest with disabled annotation",
			manifest: fmt.Sprintf(`---
apiVersion: v1
kind: ConfigMap
metadata:
  name: disabled
  labels:
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
			want: func(namespace string) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type: jsondiff.DiffTypeExclude,
						GroupVersionKind: schema.GroupVersionKind{
							Version: "v1",
							Kind:    "ConfigMap",
						},
						Namespace: namespace,
						Name:      "disabled",
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
			want: func(namespace string) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type: jsondiff.DiffTypeExclude,
						GroupVersionKind: schema.GroupVersionKind{
							Version: "v1",
							Kind:    "ConfigMap",
						},
						Namespace: namespace,
						Name:      "fully-ignored",
					},
					{
						Type: jsondiff.DiffTypeUpdate,
						GroupVersionKind: schema.GroupVersionKind{
							Version: "v1",
							Kind:    "Secret",
						},
						Namespace: namespace,
						Name:      "partially-ignored",
						Patch: extjsondiff.Patch{
							{
								Type:     extjsondiff.OperationReplace,
								Path:     "/data/otherKey",
								OldValue: "*** (before)",
								Value:    "*** (after)",
							},
						},
					},
					{
						Type: jsondiff.DiffTypeNone,
						GroupVersionKind: schema.GroupVersionKind{
							Version: "v1",
							Kind:    "Secret",
						},
						Namespace: namespace,
						Name:      "globally-ignored",
					},
					{
						Type: jsondiff.DiffTypeUpdate,
						GroupVersionKind: schema.GroupVersionKind{
							Version: "v1",
							Kind:    "ConfigMap",
						},
						Namespace: namespace,
						Name:      "not-ignored",
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
			want: func(namespace string) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type: jsondiff.DiffTypeNone,
						GroupVersionKind: schema.GroupVersionKind{
							Version: "v1",
							Kind:    "ConfigMap",
						},
						Namespace: namespace,
						Name:      "without-namespace",
					},
					{
						Type: jsondiff.DiffTypeNone,
						GroupVersionKind: schema.GroupVersionKind{
							Version: "v1",
							Kind:    "ConfigMap",
						},
						Namespace: "diff-fixed-ns",
						Name:      "with-namespace",
					},
				}
			},
		},
		{
			name: "masks Secret data",
			manifest: `---
apiVersion: v1
kind: Secret
metadata:
  name: secret
stringData:
  key: value`,
			mutateCluster: func(objs []*unstructured.Unstructured, namespace string) ([]*unstructured.Unstructured, error) {
				var clusterObjs []*unstructured.Unstructured
				for _, obj := range objs {
					obj := obj.DeepCopy()
					obj.SetNamespace(namespace)
					if err := unstructured.SetNestedField(obj.Object, "changed", "stringData", "key"); err != nil {
						return nil, fmt.Errorf("failed to set nested field: %w", err)
					}
					clusterObjs = append(clusterObjs, obj)
				}
				return clusterObjs, nil
			},
			want: func(namespace string) jsondiff.DiffSet {
				return jsondiff.DiffSet{
					{
						Type: jsondiff.DiffTypeUpdate,
						GroupVersionKind: schema.GroupVersionKind{
							Version: "v1",
							Kind:    "Secret",
						},
						Namespace: namespace,
						Name:      "secret",
						Patch: extjsondiff.Patch{
							{
								Type:     extjsondiff.OperationReplace,
								Path:     "/data/key",
								OldValue: "*** (before)",
								Value:    "*** (after)",
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

			objs, err := ssa.ReadObjects(strings.NewReader(tt.manifest))
			if err != nil {
				t.Fatalf("Failed to read release objects: %v", err)
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
				if err = ssa.NormalizeUnstructured(obj); err != nil {
					t.Fatalf("Failed to normalize cluster manifest: %v", err)
				}
				if err := c.Create(ctx, obj, client.FieldOwner(testOwner)); err != nil {
					t.Fatalf("Failed to create object: %v", err)
				}
			}

			rls := &helmrelease.Release{Namespace: ns.Name, Manifest: tt.manifest}

			got, err := Diff(ctx, &helmaction.Configuration{RESTClientGetter: getter}, rls, testOwner, tt.ignoreRules...)
			if (err != nil) != tt.wantErr {
				t.Errorf("Diff() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			var want jsondiff.DiffSet
			if tt.want != nil {
				want = tt.want(ns.Name)
			}
			if diff := cmp.Diff(want, got, cmpopts.IgnoreUnexported(extjsondiff.Operation{})); diff != "" {
				t.Errorf("Diff() mismatch (-want +got):\n%s", diff)
			}
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
