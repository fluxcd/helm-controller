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

	helmaction "helm.sh/helm/v3/pkg/action"
	helmrelease "helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/fluxcd/pkg/ssa"
	"github.com/fluxcd/pkg/ssa/jsondiff"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
)

// Diff returns a jsondiff.DiffSet of the changes between the state of the
// cluster and the Helm release.Release manifest.
func Diff(ctx context.Context, config *helmaction.Configuration, rls *helmrelease.Release, fieldOwner string, ignore ...v2.IgnoreRule) (jsondiff.DiffSet, error) {
	// Create a dry-run only client to use solely for diffing.
	cfg, err := config.RESTClientGetter.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	c, err := client.New(cfg, client.Options{DryRun: ptr.To(true)})
	if err != nil {
		return nil, err
	}

	// Read the release manifest and normalize the objects.
	objects, err := ssa.ReadObjects(strings.NewReader(rls.Manifest))
	if err != nil {
		return nil, fmt.Errorf("failed to read objects from release manifest: %w", err)
	}
	if err = ssa.NormalizeUnstructuredListWithScheme(objects, c.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to normalize release objects: %w", err)
	}

	var (
		isNamespacedGVK = map[string]bool{}
		errs            []error
	)
	for _, obj := range objects {
		if obj.GetNamespace() == "" {
			// Manifest does not contain the namespace of the release.
			// Figure out if the object is namespaced if the namespace is not
			// explicitly set, and configure the namespace accordingly.
			objGVK := obj.GetObjectKind().GroupVersionKind().String()
			if _, ok := isNamespacedGVK[objGVK]; !ok {
				namespaced, err := apiutil.IsObjectNamespaced(obj, c.Scheme(), c.RESTMapper())
				if err != nil {
					errs = append(errs, fmt.Errorf("failed to determine if %s is namespace scoped: %w",
						obj.GetObjectKind().GroupVersionKind().Kind, err))
					continue
				}
				// Cache the result, so we don't have to do this for every object
				isNamespacedGVK[objGVK] = namespaced
			}
			if isNamespacedGVK[objGVK] {
				obj.SetNamespace(rls.Namespace)
			}
		}
	}

	// Base configuration for the diffing of the object.
	diffOpts := []jsondiff.ListOption{
		jsondiff.FieldOwner(fieldOwner),
		jsondiff.ExclusionSelector{v2.DriftDetectionMetadataKey: v2.DriftDetectionDisabledValue},
		jsondiff.MaskSecrets(true),
		jsondiff.Rationalize(true),
		jsondiff.Graceful(true),
	}

	// Add ignore rules to the diffing configuration.
	var ignoreRules jsondiff.IgnoreRules
	for _, rule := range ignore {
		r := jsondiff.IgnoreRule{
			Paths: rule.Paths,
		}
		if rule.Target != nil {
			r.Selector = &jsondiff.Selector{
				Group:              rule.Target.Group,
				Version:            rule.Target.Version,
				Kind:               rule.Target.Kind,
				Name:               rule.Target.Name,
				Namespace:          rule.Target.Namespace,
				AnnotationSelector: rule.Target.AnnotationSelector,
				LabelSelector:      rule.Target.LabelSelector,
			}
		}
		ignoreRules = append(ignoreRules, r)
	}
	if len(ignoreRules) > 0 {
		diffOpts = append(diffOpts, ignoreRules)
	}

	// Actually diff the objects.
	set, err := jsondiff.UnstructuredList(ctx, c, objects, diffOpts...)
	if err != nil {
		errs = append(errs, err)
	}
	return set, errors.Reduce(errors.Flatten(errors.NewAggregate(errs)))
}
