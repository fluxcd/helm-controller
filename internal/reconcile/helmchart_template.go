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

package reconcile

import (
	"context"
	"fmt"

	"helm.sh/helm/v4/pkg/registry"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	eventv1 "github.com/fluxcd/pkg/apis/event/v1"
	"github.com/fluxcd/pkg/runtime/events"
	"github.com/fluxcd/pkg/ssa"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/acl"
	"github.com/fluxcd/helm-controller/internal/strings"
)

// HelmChartTemplate attempts to create, update or delete a v1.HelmChart or
// v1.OCIRepository based on the given Request data.
//
// It does this by building the chart from the template declared in the
// v2.HelmRelease, and then reconciling it using a server-side apply.
//
// When the server-side apply succeeds, the typed and namespaced name of the
// chart is written to the Status.HelmChart field of the v2.HelmRelease. If
// the server-side apply fails, the error is returned to the caller and
// indicates they should retry.
//
// When at the beginning of the reconciliation the deletion timestamp is set
// on the v2.HelmRelease, or the Status.HelmChart differs from the reference
// of the chart to be applied, the existing chart is deleted. The deletion is
// observed, and when it completes, the Status.HelmChart is cleared. If the
// deletion fails, the error is returned to the caller and indicates they
// should retry.
//
// In case the v2.HelmRelease is marked for deletion, the reconciler will
// not continue to attempt to create or update the chart.
type HelmChartTemplate struct {
	client        client.Client
	eventRecorder events.Recorder
	fieldManager  string
}

// NewHelmChartTemplate returns a new HelmChartTemplate reconciler configured
// with the provided values.
func NewHelmChartTemplate(client client.Client, recorder events.Recorder, fieldManager string) *HelmChartTemplate {
	return &HelmChartTemplate{
		client:        client,
		eventRecorder: recorder,
		fieldManager:  fieldManager,
	}
}

func (r *HelmChartTemplate) Reconcile(ctx context.Context, req *Request) error {
	obj := req.Object
	ref := obj.GetHelmChartTemplateReference()

	// The chart reference diverges or the HelmRelease is being deleted,
	// delete the chart.
	if (obj.Status.HasChart() && !obj.Status.GetHelmChartReference().Matches(ref)) || !obj.DeletionTimestamp.IsZero() {
		// If the HelmRelease is being deleted, we need to short-circuit to
		// avoid recreating the chart.
		if err := r.reconcileDelete(ctx, obj); err != nil || !obj.DeletionTimestamp.IsZero() {
			return err
		}
	}

	if mustCleanDeployedChart(obj) {
		// If the HelmRelease has a ChartRef and no Chart template, but a
		// chart is present in the status, we need to clean it up.
		if err := r.reconcileDelete(ctx, obj); err != nil {
			return err
		}
		return nil
	}

	if obj.HasChartRef() {
		// if a chartRef is present, we do not need to reconcile the chart from the template.
		return nil
	}

	// Confirm we are allowed to fetch the chart.
	if err := acl.AllowsAccessTo(obj, ref); err != nil {
		return err
	}

	// Build a new chart based on the declared template.
	var newChart client.Object
	var newChartDeepCopy any
	var newChartWithSourceRef string
	switch ref.Kind {
	case sourcev1.HelmChartKind:
		hc := buildHelmChartFromTemplate(obj, ref)
		newChart = hc
		newChartDeepCopy = hc.DeepCopy()
		newChartWithSourceRef = fmt.Sprintf(" with SourceRef '%s/%s/%s'",
			hc.Spec.SourceRef.Kind, hc.GetNamespace(), hc.Spec.SourceRef.Name)
	case sourcev1.OCIRepositoryKind:
		or := buildOCIRepositoryFromTemplate(obj, ref)
		newChart = or
		newChartDeepCopy = or.DeepCopy()
		newChartWithSourceRef = ""
	}

	// Convert to an unstructured object to please the SSA library.
	uo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(newChartDeepCopy)
	if err != nil {
		return fmt.Errorf("failed to convert %s to unstructured: %w", ref.Kind, err)
	}
	u := &unstructured.Unstructured{Object: uo}

	// Get the GVK for the object according to the current scheme.
	gvk, err := apiutil.GVKForObject(newChart, r.client.Scheme())
	if err != nil {
		return fmt.Errorf("unable to get GVK for %s: %w", ref.Kind, err)
	}
	u.SetGroupVersionKind(gvk)

	rm := ssa.NewResourceManager(r.client, nil, ssa.Owner{
		Group: v2.GroupVersion.Group,
		Field: r.fieldManager,
	})

	// Mark the object as owned by the HelmRelease.
	rm.SetOwnerLabels([]*unstructured.Unstructured{u}, obj.GetName(), obj.GetNamespace())

	// Run using server-side apply.
	entry, err := rm.Apply(ctx, u, ssa.DefaultApplyOptions())
	if err != nil {
		err = fmt.Errorf("failed to run server-side apply: %w", err)
		reason := fmt.Sprintf("%sSyncErr", ref.Kind)
		r.eventRecorder.Eventf(obj, nil, eventv1.EventTypeTrace, reason, "%s", err.Error())
		return err
	}

	// Consult the entry result and act accordingly.
	switch entry.Action {
	case ssa.CreatedAction, ssa.ConfiguredAction:
		msg := strings.Normalize(fmt.Sprintf("%s %s%s",
			entry.Action.String(), entry.Subject, newChartWithSourceRef))

		ctrl.LoggerFrom(ctx).Info(msg)
		r.eventRecorder.Eventf(obj, nil, eventv1.EventTypeTrace,
			fmt.Sprintf("%s%s", ref.Kind, strings.Title(entry.Action.String())), "%s", msg)
	case ssa.UnchangedAction:
		msg := fmt.Sprintf("%s%s is in-sync", entry.Subject, newChartWithSourceRef)

		ctrl.LoggerFrom(ctx).Info(msg)
	default:
		err = fmt.Errorf("unexpected action '%s' for %s", entry.Action.String(), entry.Subject)
		return err
	}

	// From this moment on, we know the chart spec is up-to-date.
	obj.Status.SetChart(ref)

	return nil
}

// reconcileDelete handles the garbage collection of the current chart
// referenced in the Status object of the given HelmRelease.
func (r *HelmChartTemplate) reconcileDelete(ctx context.Context, obj *v2.HelmRelease) error {
	if !obj.Spec.Suspend && obj.Status.HasChart() {
		ref := obj.Status.GetHelmChartReference()

		// Confirm we are allowed to fetch the chart.
		if err := acl.AllowsAccessTo(obj, ref); err != nil {
			return err
		}

		// Fetch the chart.
		var chart client.Object
		if ref.Kind == sourcev1.OCIRepositoryKind {
			chart = &sourcev1.OCIRepository{}
		} else {
			chart = &sourcev1.HelmChart{}
		}
		err := r.client.Get(ctx, ref.GetObjectKey(), chart)
		if err != nil && !apierrors.IsNotFound(err) {
			// Return error to retry until we succeed.
			err = fmt.Errorf("failed to get '%s': %w", ref, err)
			return err
		}
		if err == nil {
			// Delete the chart.
			if err = r.client.Delete(ctx, chart); client.IgnoreNotFound(err) != nil {
				err = fmt.Errorf("failed to delete '%s': %w", ref, err)
				return err
			}
			reason := fmt.Sprintf("%sDeleted", ref.Kind)
			r.eventRecorder.Eventf(obj, nil, eventv1.EventTypeTrace, reason, "deleted '%s'", ref.String())
		}

		// Truncate the chart reference in the status object.
		obj.Status.SetChart(nil)
	}

	return nil
}

func buildHelmChartFromTemplate(obj *v2.HelmRelease, ref *v2.HelmChartReference) *sourcev1.HelmChart {
	template := obj.Spec.Chart.DeepCopy()
	result := &sourcev1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Name,
			Namespace: ref.Namespace,
		},
		Spec: sourcev1.HelmChartSpec{
			Chart:                    template.Spec.Chart,
			Version:                  template.Spec.GetVersion(),
			Interval:                 obj.GetTemplateInterval(),
			ReconcileStrategy:        template.Spec.GetReconcileStrategy(),
			ValuesFiles:              template.Spec.ValuesFiles,
			IgnoreMissingValuesFiles: template.Spec.IgnoreMissingValuesFiles,
		},
	}
	if sourceRef := template.Spec.SourceRef; sourceRef != nil {
		result.Spec.SourceRef = sourcev1.LocalHelmChartSourceReference{
			Name: sourceRef.Name,
			Kind: sourceRef.Kind,
		}
	}
	if verifyTpl := template.Spec.Verify; verifyTpl != nil {
		result.Spec.Verify = &sourcev1.HelmChartVerification{
			Provider:  verifyTpl.Provider,
			SecretRef: verifyTpl.SecretRef,
		}
	}
	if metaTpl := obj.Spec.Chart.ObjectMeta; metaTpl != nil {
		result.SetAnnotations(metaTpl.Annotations)
		result.SetLabels(metaTpl.Labels)
	}
	return result
}

func buildOCIRepositoryFromTemplate(obj *v2.HelmRelease, ref *v2.HelmChartReference) *sourcev1.OCIRepository {
	template := obj.Spec.Chart
	interval := obj.GetTemplateInterval()
	spec := template.Spec.OCIRepositorySpec.DeepCopy()
	spec.Interval = &interval
	// If the layer selector is not explicitly specified, default to
	// selecting the Helm chart layer and copying it as-is.
	if spec.LayerSelector == nil {
		spec.LayerSelector = &sourcev1.OCILayerSelector{
			MediaType: registry.ChartLayerMediaType,
			Operation: sourcev1.OCILayerCopy,
		}
	}
	result := &sourcev1.OCIRepository{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ref.Name,
			Namespace: ref.Namespace,
		},
		Spec: *spec,
	}
	if metaTpl := obj.Spec.Chart.ObjectMeta; metaTpl != nil {
		result.SetAnnotations(metaTpl.Annotations)
		result.SetLabels(metaTpl.Labels)
	}
	return result
}

func mustCleanDeployedChart(obj *v2.HelmRelease) bool {
	return obj.HasChartRef() && !obj.HasChartTemplate() && obj.Status.HasChart()
}
