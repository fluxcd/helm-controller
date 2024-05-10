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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"

	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"
	"github.com/fluxcd/pkg/ssa"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/acl"
	"github.com/fluxcd/helm-controller/internal/strings"
)

// HelmChartTemplate attempts to create, update or delete a v1beta2.HelmChart
// based on the given Request data.
//
// It does this by building a v1beta2.HelmChart from the template declared in
// the v2.HelmRelease, and then reconciling that v1beta2.HelmChart using
// a server-side apply.
//
// When the server-side apply succeeds, the namespaced name of the chart is
// written to the Status.HelmChart field of the v2.HelmRelease. If the
// server-side apply fails, the error is returned to the caller and indicates
// they should retry.
//
// When at the beginning of the reconciliation the deletion timestamp is set
// on the v2.HelmRelease, or the Status.HelmChart differs from the
// namespaced name of the chart to be applied, the existing chart is deleted.
// The deletion is observed, and when it completes, the Status.HelmChart is
// cleared. If the deletion fails, the error is returned to the caller and
// indicates they should retry.
//
// In case the v2.HelmRelease is marked for deletion, the reconciler will
// not continue to attempt to create or update the v1beta2.HelmChart.
type HelmChartTemplate struct {
	client        client.Client
	eventRecorder record.EventRecorder
	fieldManager  string
}

// NewHelmChartTemplate returns a new HelmChartTemplate reconciler configured
// with the provided values.
func NewHelmChartTemplate(client client.Client, recorder record.EventRecorder, fieldManager string) *HelmChartTemplate {
	return &HelmChartTemplate{
		client:        client,
		eventRecorder: recorder,
		fieldManager:  fieldManager,
	}
}

func (r *HelmChartTemplate) Reconcile(ctx context.Context, req *Request) error {
	var (
		obj      = req.Object
		chartRef = types.NamespacedName{}
	)

	if obj.Spec.Chart != nil {
		chartRef.Name = obj.GetHelmChartName()
		chartRef.Namespace = obj.Spec.Chart.GetNamespace(obj.Namespace)
	}

	// The HelmChart name and/or namespace diverges or the HelmRelease is
	// being deleted, delete the HelmChart.
	if (obj.Status.HelmChart != "" && obj.Status.HelmChart != chartRef.String()) || !obj.DeletionTimestamp.IsZero() {
		// If the HelmRelease is being deleted, we need to short-circuit to
		// avoid recreating the HelmChart.
		if err := r.reconcileDelete(ctx, req.Object); err != nil || !obj.DeletionTimestamp.IsZero() {
			return err
		}
	}

	if mustCleanDeployedChart(obj) {
		// If the HelmRelease has a ChartRef and no Chart template, and the
		// HelmChart is present, we need to clean it up.
		if err := r.reconcileDelete(ctx, req.Object); err != nil {
			return err
		}
		return nil
	}

	if obj.HasChartRef() {
		// if a chartRef is present, we do not need to reconcile the HelmChart from the template.
		return nil
	}

	// Confirm we are allowed to fetch the HelmChart.
	if err := acl.AllowsAccessTo(req.Object, sourcev1.HelmChartKind, chartRef); err != nil {
		return err
	}

	// Build new HelmChart based on the declared template.
	newChart := buildHelmChartFromTemplate(req.Object)

	// Convert to an unstructured object to please the SSA library.
	uo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(newChart.DeepCopy())
	if err != nil {
		return fmt.Errorf("failed to convert HelmChart to unstructured: %w", err)
	}
	u := &unstructured.Unstructured{Object: uo}

	// Get the GVK for the object according to the current scheme.
	gvk, err := apiutil.GVKForObject(newChart, r.client.Scheme())
	if err != nil {
		return fmt.Errorf("unable to get GVK for HelmChart: %w", err)
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
		r.eventRecorder.Eventf(req.Object, eventv1.EventTypeTrace, "HelmChartSyncErr", err.Error())
		return err
	}

	// Consult the entry result and act accordingly.
	switch entry.Action {
	case ssa.CreatedAction, ssa.ConfiguredAction:
		msg := strings.Normalize(fmt.Sprintf(
			"%s %s with SourceRef '%s/%s/%s'", entry.Action.String(), entry.Subject,
			newChart.Spec.SourceRef.Kind, newChart.GetNamespace(), newChart.Spec.SourceRef.Name,
		))

		ctrl.LoggerFrom(ctx).Info(msg)
		r.eventRecorder.Eventf(req.Object, eventv1.EventTypeTrace,
			fmt.Sprintf("HelmChart%s", strings.Title(entry.Action.String())), msg)
	case ssa.UnchangedAction:
		msg := fmt.Sprintf("%s with SourceRef '%s/%s/%s' is in-sync", entry.Subject,
			newChart.Spec.SourceRef.Kind, newChart.GetNamespace(), newChart.Spec.SourceRef.Name)

		ctrl.LoggerFrom(ctx).Info(msg)
	default:
		err = fmt.Errorf("unexpected action '%s' for %s", entry.Action.String(), entry.Subject)
		return err
	}

	// From this moment on, we know the HelmChart spec is up-to-date.
	obj.Status.HelmChart = chartRef.String()

	return nil
}

// reconcileDelete handles the garbage collection of the current HelmChart in
// the Status object of the given HelmRelease.
func (r *HelmChartTemplate) reconcileDelete(ctx context.Context, obj *v2.HelmRelease) error {
	if !obj.Spec.Suspend && obj.Status.HelmChart != "" {
		ns, name := obj.Status.GetHelmChart()
		namespacedName := types.NamespacedName{Namespace: ns, Name: name}

		// Confirm we are allowed to fetch the HelmChart.
		if err := acl.AllowsAccessTo(obj, sourcev1.HelmChartKind, namespacedName); err != nil {
			return err
		}

		// Fetch the HelmChart.
		var chart sourcev1.HelmChart
		err := r.client.Get(ctx, namespacedName, &chart)
		if err != nil && !apierrors.IsNotFound(err) {
			// Return error to retry until we succeed.
			err = fmt.Errorf("failed to delete HelmChart '%s': %w", obj.Status.HelmChart, err)
			return err
		}
		if err == nil {
			// Delete the HelmChart.
			if err = r.client.Delete(ctx, &chart); err != nil {
				err = fmt.Errorf("failed to delete HelmChart '%s': %w", obj.Status.HelmChart, err)
				return err
			}
			r.eventRecorder.Eventf(obj, eventv1.EventTypeTrace, "HelmChartDeleted", "deleted HelmChart '%s'", obj.Status.HelmChart)
		}

		// Truncate the chart reference in the status object.
		obj.Status.HelmChart = ""
	}

	return nil
}

// buildHelmChartFromTemplate builds a v1beta2.HelmChart from the
// v2beta1.HelmChartTemplate of the given v2beta1.HelmRelease.
func buildHelmChartFromTemplate(obj *v2.HelmRelease) *sourcev1.HelmChart {
	template := obj.Spec.Chart.DeepCopy()
	result := &sourcev1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.GetHelmChartName(),
			Namespace: template.GetNamespace(obj.Namespace),
		},
		Spec: sourcev1.HelmChartSpec{
			Chart:   template.Spec.Chart,
			Version: template.Spec.Version,
			SourceRef: sourcev1.LocalHelmChartSourceReference{
				Name: template.Spec.SourceRef.Name,
				Kind: template.Spec.SourceRef.Kind,
			},
			Interval:                 template.GetInterval(obj.Spec.Interval),
			ReconcileStrategy:        template.Spec.ReconcileStrategy,
			ValuesFiles:              template.Spec.ValuesFiles,
			IgnoreMissingValuesFiles: template.Spec.IgnoreMissingValuesFiles,
		},
	}
	if verifyTpl := template.Spec.Verify; verifyTpl != nil {
		result.Spec.Verify = &sourcev1.OCIRepositoryVerification{
			Provider:  verifyTpl.Provider,
			SecretRef: verifyTpl.SecretRef,
		}
	}
	if metaTpl := template.ObjectMeta; metaTpl != nil {
		result.SetAnnotations(metaTpl.Annotations)
		result.SetLabels(metaTpl.Labels)
	}
	return result
}

func mustCleanDeployedChart(obj *v2.HelmRelease) bool {
	if obj.HasChartRef() && !obj.HasChartTemplate() {
		if obj.Status.HelmChart != "" {
			return true
		}
	}

	return false
}
