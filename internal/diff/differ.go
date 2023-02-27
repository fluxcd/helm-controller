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
	"strings"

	"helm.sh/helm/v3/pkg/release"
	"k8s.io/apimachinery/pkg/util/errors"

	"github.com/fluxcd/pkg/runtime/client"
	"github.com/fluxcd/pkg/ssa"

	"github.com/fluxcd/helm-controller/internal/util"
)

type Differ struct {
	impersonator   *client.Impersonator
	controllerName string
}

func NewDiffer(impersonator *client.Impersonator, controllerName string) *Differ {
	return &Differ{
		impersonator:   impersonator,
		controllerName: controllerName,
	}
}

// Manager returns a new ssa.ResourceManager constructed using the client.Impersonator.
func (d *Differ) Manager(ctx context.Context) (*ssa.ResourceManager, error) {
	c, poller, err := d.impersonator.GetClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get client to configure resource manager: %w", err)
	}
	owner := ssa.Owner{
		Field: d.controllerName,
	}
	return ssa.NewResourceManager(c, poller, owner), nil
}

func (d *Differ) Diff(ctx context.Context, rel *release.Release) (*ssa.ChangeSet, bool, error) {
	objects, err := ssa.ReadObjects(strings.NewReader(rel.Manifest))
	if err != nil {
		return nil, false, fmt.Errorf("failed to read objects from release manifest: %w", err)
	}

	if err := ssa.SetNativeKindsDefaults(objects); err != nil {
		return nil, false, fmt.Errorf("failed to set native kind defaults on release objects: %w", err)
	}

	resourceManager, err := d.Manager(ctx)
	if err != nil {
		return nil, false, err
	}

	var (
		changeSet       = ssa.NewChangeSet()
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
				namespaced, err := util.IsAPINamespaced(obj, resourceManager.Client().Scheme(), resourceManager.Client().RESTMapper())
				if err != nil {
					errs = append(errs, fmt.Errorf("failed to determine if %s is namespace scoped: %w",
						obj.GetObjectKind().GroupVersionKind().Kind, err))
					continue
				}
				// Cache the result, so we don't have to do this for every object
				isNamespacedGVK[objGVK] = namespaced
			}
			if isNamespacedGVK[objGVK] {
				obj.SetNamespace(rel.Namespace)
			}
		}

		entry, _, _, err := resourceManager.Diff(ctx, obj, ssa.DiffOptions{})
		if err != nil {
			errs = append(errs, err)
		}
		if entry != nil && (entry.Action == string(ssa.CreatedAction) || entry.Action == string(ssa.ConfiguredAction)) {
			changeSet.Add(*entry)
		}
	}

	err = errors.Reduce(errors.Flatten(errors.NewAggregate(errs)))
	if len(changeSet.Entries) == 0 {
		return nil, false, err
	}
	return changeSet, true, err
}
