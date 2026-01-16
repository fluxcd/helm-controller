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
	"bytes"
	"fmt"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/fluxcd/cli-utils/pkg/object"
	ssautil "github.com/fluxcd/pkg/ssa/utils"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	helmchart "helm.sh/helm/v4/pkg/chart/v2"
	helmrelease "helm.sh/helm/v4/pkg/release/v1"
)

// New returns a new ResourceInventory with an empty Entries slice.
func New() *v2.ResourceInventory {
	return &v2.ResourceInventory{
		Entries: []v2.ResourceRef{},
	}
}

// AddManifest parses the manifest, complements namespaces, and adds the objects to the inventory.
func AddManifest(inv *v2.ResourceInventory, manifest string, releaseNamespace string, c client.Client) (warnings []string, err error) {
	objects, err := parseManifest(manifest)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	objects = slices.DeleteFunc(objects, func(obj *unstructured.Unstructured) bool {
		annotations := obj.GetAnnotations()
		if annotations == nil {
			return false
		}
		_, isHook := annotations[helmrelease.HookAnnotation]
		return isHook
	})

	warnings = setNamespaces(objects, releaseNamespace, c)

	for _, obj := range objects {
		objMeta := object.UnstructuredToObjMetadata(obj)
		inv.Entries = append(inv.Entries, v2.ResourceRef{
			ID:      objMeta.String(),
			Version: obj.GroupVersionKind().Version,
		})
	}
	return warnings, nil
}

func parseManifest(manifest string) ([]*unstructured.Unstructured, error) {
	return ssautil.ReadObjects(strings.NewReader(manifest))
}

// setNamespaces complements namespace for namespaced objects that don't have one set.
// This is necessary because Helm manifests don't include namespace for namespaced resources.
func setNamespaces(objects []*unstructured.Unstructured, releaseNamespace string, c client.Client) []string {
	var warnings []string
	isNamespacedGVK := map[schema.GroupVersionKind]bool{}

	for _, obj := range objects {
		if obj.GetNamespace() != "" {
			continue
		}

		objGVK := obj.GetObjectKind().GroupVersionKind()
		if _, ok := isNamespacedGVK[objGVK]; !ok {
			namespaced, err := apiutil.IsObjectNamespaced(obj, c.Scheme(), c.RESTMapper())
			if err != nil {
				warnings = append(warnings, fmt.Sprintf(
					"failed to determine if %s is namespace scoped, skipping namespace: %s",
					objGVK.Kind, err.Error()))
				continue
			}
			isNamespacedGVK[objGVK] = namespaced
		}

		if isNamespacedGVK[objGVK] {
			obj.SetNamespace(releaseNamespace)
		}
	}
	return warnings
}

// AddCRDs adds CRDs from the chart's crds/ directory to the inventory.
// CRDs are cluster-scoped, so no namespace complement is needed.
func AddCRDs(inv *v2.ResourceInventory, chart *helmchart.Chart) error {
	for _, crd := range chart.CRDObjects() {
		objects, err := ssautil.ReadObjects(bytes.NewBuffer(crd.File.Data))
		if err != nil {
			return fmt.Errorf("failed to parse CRD %s: %w", crd.Name, err)
		}

		for _, obj := range objects {
			objMeta := object.UnstructuredToObjMetadata(obj)
			inv.Entries = append(inv.Entries, v2.ResourceRef{
				ID:      objMeta.String(),
				Version: obj.GroupVersionKind().Version,
			})
		}
	}
	return nil
}
