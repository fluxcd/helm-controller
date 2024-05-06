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
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	helmaction "helm.sh/helm/v3/pkg/action"
	helmrelease "helm.sh/helm/v3/pkg/release"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	apierrutil "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	"github.com/fluxcd/cli-utils/pkg/object"
	"github.com/fluxcd/pkg/ssa"
	"github.com/fluxcd/pkg/ssa/jsondiff"
	ssanormalize "github.com/fluxcd/pkg/ssa/normalize"
	ssautil "github.com/fluxcd/pkg/ssa/utils"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/diff"
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
	objects, err := ssautil.ReadObjects(strings.NewReader(rls.Manifest))
	if err != nil {
		return nil, fmt.Errorf("failed to read objects from release manifest: %w", err)
	}
	if err = ssanormalize.UnstructuredListWithScheme(objects, c.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to normalize release objects: %w", err)
	}

	var (
		isNamespacedGVK = map[string]bool{}
		errs            []error
	)
	for _, obj := range objects {
		// Set the Helm metadata on the object which is normally set by Helm
		// during object creation.
		setHelmMetadata(obj, rls)

		// Set the namespace of the object if it is not set.
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
	return set, apierrutil.Reduce(apierrutil.Flatten(apierrutil.NewAggregate(errs)))
}

// ApplyDiff applies the changes described in the provided jsondiff.DiffSet to
// the Kubernetes cluster.
func ApplyDiff(ctx context.Context, config *helmaction.Configuration, diffSet jsondiff.DiffSet, fieldOwner string) (*ssa.ChangeSet, error) {
	cfg, err := config.RESTClientGetter.ToRESTConfig()
	if err != nil {
		return nil, err
	}
	c, err := client.New(cfg, client.Options{})
	if err != nil {
		return nil, err
	}

	var toCreate, toPatch sortableDiffs
	for _, d := range diffSet {
		switch d.Type {
		case jsondiff.DiffTypeCreate:
			toCreate = append(toCreate, d)
		case jsondiff.DiffTypeUpdate:
			toPatch = append(toPatch, d)
		}
	}

	var (
		changeSet = ssa.NewChangeSet()
		errs      []error
	)

	sort.Sort(toCreate)
	for _, d := range toCreate {
		obj := d.DesiredObject.DeepCopyObject().(client.Object)
		if err := c.Create(ctx, obj, client.FieldOwner(fieldOwner)); err != nil {
			errs = append(errs, fmt.Errorf("%s creation failure: %w", diff.ResourceName(obj), err))
			continue
		}
		changeSet.Add(objectToChangeSetEntry(obj, ssa.CreatedAction))
	}

	sort.Sort(toPatch)
	for _, d := range toPatch {
		data, err := json.Marshal(d.Patch)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s patch failure: %w", diff.ResourceName(d.DesiredObject), err))
			continue
		}

		obj := d.DesiredObject.DeepCopyObject().(client.Object)
		patch := client.RawPatch(types.JSONPatchType, data)
		if err := c.Patch(ctx, obj, patch, client.FieldOwner(fieldOwner)); err != nil {
			if obj.GetObjectKind().GroupVersionKind().Kind == "Secret" {
				err = maskSensitiveErrData(err)
			}
			errs = append(errs, fmt.Errorf("%s patch failure: %w", diff.ResourceName(obj), err))
			continue
		}
		changeSet.Add(objectToChangeSetEntry(obj, ssa.ConfiguredAction))
	}

	return changeSet, apierrutil.NewAggregate(errs)
}

const (
	appManagedByLabel              = "app.kubernetes.io/managed-by"
	appManagedByHelm               = "Helm"
	helmReleaseNameAnnotation      = "meta.helm.sh/release-name"
	helmReleaseNamespaceAnnotation = "meta.helm.sh/release-namespace"
)

// setHelmMetadata sets the metadata on the given object to indicate that it is
// managed by Helm. This is safe to do, because we apply it to objects that
// originate from the Helm release itself.
// xref: https://github.com/helm/helm/blob/v3.13.2/pkg/action/validate.go
// xref: https://github.com/helm/helm/blob/v3.13.2/pkg/action/rollback.go#L186-L191
func setHelmMetadata(obj client.Object, rls *helmrelease.Release) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string, 1)
	}
	labels[appManagedByLabel] = appManagedByHelm
	obj.SetLabels(labels)

	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string, 2)
	}
	annotations[helmReleaseNameAnnotation] = rls.Name
	annotations[helmReleaseNamespaceAnnotation] = rls.Namespace
	obj.SetAnnotations(annotations)
}

// objectToChangeSetEntry returns a ssa.ChangeSetEntry for the given object and
// action.
func objectToChangeSetEntry(obj client.Object, action ssa.Action) ssa.ChangeSetEntry {
	return ssa.ChangeSetEntry{
		ObjMetadata: object.ObjMetadata{
			GroupKind: obj.GetObjectKind().GroupVersionKind().GroupKind(),
			Name:      obj.GetName(),
			Namespace: obj.GetNamespace(),
		},
		GroupVersion: obj.GetObjectKind().GroupVersionKind().Version,
		Subject:      diff.ResourceName(obj),
		Action:       action,
	}
}

// maskSensitiveErrData masks potentially sensitive data from the error message
// returned by the Kubernetes API server.
// This avoids leaking any sensitive data in logs or other output when a patch
// operation fails.
func maskSensitiveErrData(err error) error {
	if apierrors.IsInvalid(err) {
		// The last part of the error message is the reason for the error.
		if i := strings.LastIndex(err.Error(), `:`); i != -1 {
			err = errors.New(strings.TrimSpace(err.Error()[i+1:]))
		}
	}
	return err
}

// sortableDiffs is a sortable slice of jsondiff.Diffs.
type sortableDiffs []*jsondiff.Diff

// Len returns the length of the slice.
func (s sortableDiffs) Len() int { return len(s) }

// Swap swaps the elements with indexes i and j.
func (s sortableDiffs) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

// Less returns true if the element with index i should sort before the element
// with index j.
// The elements are sorted by GroupKind, Namespace and Name.
func (s sortableDiffs) Less(i, j int) bool {
	iDiff, jDiff := s[i], s[j]

	if !ssa.Equals(iDiff.GroupVersionKind().GroupKind(), jDiff.GroupVersionKind().GroupKind()) {
		return ssa.IsLessThan(iDiff.GroupVersionKind().GroupKind(), jDiff.GroupVersionKind().GroupKind())
	}

	if iDiff.GetNamespace() != jDiff.GetNamespace() {
		return iDiff.GetNamespace() < jDiff.GetNamespace()
	}

	return iDiff.GetName() < jDiff.GetName()
}
