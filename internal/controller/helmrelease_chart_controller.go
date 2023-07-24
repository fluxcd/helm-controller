/*
Copyright 2022 The Flux authors

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

package controller

import (
	"context"
	"fmt"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	kuberecorder "k8s.io/client-go/tools/record"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/apiutil"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/ratelimiter"

	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"
	"github.com/fluxcd/pkg/runtime/acl"
	helper "github.com/fluxcd/pkg/runtime/controller"
	"github.com/fluxcd/pkg/runtime/patch"
	"github.com/fluxcd/pkg/runtime/predicates"
	"github.com/fluxcd/pkg/ssa"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	intpredicates "github.com/fluxcd/helm-controller/internal/predicates"
)

type HelmReleaseChartReconciler struct {
	client.Client
	kuberecorder.EventRecorder
	helper.Metrics

	StatusPoller        *polling.StatusPoller
	FieldManager        string
	NoCrossNamespaceRef bool
}

type HelmReleaseChartReconcilerOptions struct {
	RateLimiter ratelimiter.RateLimiter
}

func (r *HelmReleaseChartReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.SetupWithManagerAndOptions(mgr, HelmReleaseChartReconcilerOptions{})
}

func (r *HelmReleaseChartReconciler) SetupWithManagerAndOptions(mgr ctrl.Manager, opts HelmReleaseChartReconcilerOptions) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2.HelmRelease{}).
		WithEventFilter(predicate.Or(intpredicates.ChartTemplateChangePredicate{}, predicates.ReconcileRequestedPredicate{})).
		WithOptions(controller.Options{
			RateLimiter: opts.RateLimiter,
		}).
		Complete(r)
}

func (r *HelmReleaseChartReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the HelmRelease
	obj := &v2.HelmRelease{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Record suspended status metric
	r.RecordSuspend(ctx, obj, obj.Spec.Suspend)

	// Initialize the patch helper with the current version of the object.
	patchHelper := patch.NewSerialPatcher(obj, r.Client)

	// Always attempt to patch the object after each reconciliation.
	defer func() {
		if err := patchHelper.Patch(ctx, obj, patch.WithFieldOwner(r.FieldManager)); err != nil {
			if retErr != nil {
				retErr = errors.NewAggregate([]error{retErr, err})
			} else {
				retErr = err
			}
		}
	}()

	// Examine if the object is under deletion.
	// NOTE: This needs to happen before the finalizer check, as
	if !obj.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, obj)
	}

	// Add finalizer first if not exist to avoid the race condition
	// between init and delete.
	// Note: Finalizers in general can only be added when the deletionTimestamp
	// is not set.
	if !controllerutil.ContainsFinalizer(obj, v2.ChartFinalizer) {
		controllerutil.AddFinalizer(obj, v2.ChartFinalizer)
		return ctrl.Result{Requeue: true}, nil
	}

	// Return early if the object is suspended
	if obj.Spec.Suspend {
		log.Info("reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	return r.reconcile(ctx, obj)
}

func (r *HelmReleaseChartReconciler) reconcile(ctx context.Context, obj *v2.HelmRelease) (ctrl.Result, error) {
	chartRef := types.NamespacedName{
		Namespace: obj.Spec.Chart.GetNamespace(obj.Namespace),
		Name:      obj.GetHelmChartName(),
	}

	// The HelmChart name and/or namespace diverges, delete first the current
	// and come back.
	if obj.Status.HelmChart != "" && obj.Status.HelmChart != chartRef.String() {
		return r.reconcileDelete(ctx, obj)
	}

	// Confirm we are allowed to fetch the HelmChart.
	if err := r.aclAllowAccessTo(obj, chartRef); err != nil {
		return ctrl.Result{}, err
	}

	// Build new HelmChart based on the declared template.
	newChart := buildHelmChartFromTemplate(obj)

	// Convert to an unstructured object to please the SSA library.
	uo, err := runtime.DefaultUnstructuredConverter.ToUnstructured(newChart.DeepCopy())
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to convert HelmChart to unstructured: %w", err)
	}
	u := &unstructured.Unstructured{Object: uo}

	// Get the GVK for the object according to the current scheme.
	gvk, err := apiutil.GVKForObject(newChart, r.Client.Scheme())
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("unable to get GVK for HelmChart: %w", err)
	}
	u.SetGroupVersionKind(gvk)

	rm := ssa.NewResourceManager(r.Client, r.StatusPoller, ssa.Owner{
		Group: v2.GroupVersion.Group,
		Field: r.FieldManager,
	})

	// Mark the object as owned by the HelmRelease.
	rm.SetOwnerLabels([]*unstructured.Unstructured{u}, obj.GetName(), obj.GetNamespace())

	// Run using server-side apply.
	entry, err := rm.Apply(ctx, u, ssa.DefaultApplyOptions())
	if err != nil {
		err = fmt.Errorf("failed to run server-side apply: %w", err)
		r.Eventf(obj, eventv1.EventTypeTrace, "HelmChartApplyFailed", err.Error())
		return ctrl.Result{}, err
	}

	// Consult the entry result and act accordingly.
	switch entry.Action {
	case ssa.CreatedAction, ssa.ConfiguredAction:
		msg := fmt.Sprintf("%s %s with SourceRef '%s/%s/%s'", entry.Action.String(), entry.Subject,
			newChart.Spec.SourceRef.Kind, newChart.GetNamespace(), newChart.Spec.SourceRef.Name)
		r.Eventf(obj, eventv1.EventTypeTrace, fmt.Sprintf("HelmChart%s", cases.Title(language.Und).String(entry.Action.String())), msg)
		ctrl.LoggerFrom(ctx).Info(msg)
	case ssa.UnchangedAction:
		msg := fmt.Sprintf("%s with SourceRef '%s/%s/%s' is in-sync with the declared state", entry.Subject,
			newChart.Spec.SourceRef.Kind, newChart.GetNamespace(), newChart.Spec.SourceRef.Name)
		r.Eventf(obj, eventv1.EventTypeTrace, "HelmChartUnchanged", msg)
		ctrl.LoggerFrom(ctx).Info(msg)
	default:
		err = fmt.Errorf("unexpected action '%s' for %s", entry.Action.String(), entry.Subject)
		return ctrl.Result{}, err
	}

	// From this moment on, we know the HelmChart spec is up-to-date.
	obj.Status.HelmChart = chartRef.String()

	// Requeue to ensure the state continues to be the same.
	return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
}

// reconcileDelete handles the garbage collection of the current HelmChart in
// the Status object of the given HelmRelease.
func (r *HelmReleaseChartReconciler) reconcileDelete(ctx context.Context, obj *v2.HelmRelease) (ctrl.Result, error) {
	if !obj.Spec.Suspend && obj.Status.HelmChart != "" {
		ns, name := obj.Status.GetHelmChart()
		namespacedName := types.NamespacedName{Namespace: ns, Name: name}

		// Confirm we are allowed to fetch the HelmChart.
		if err := r.aclAllowAccessTo(obj, namespacedName); err != nil {
			return ctrl.Result{}, err
		}

		// Fetch the HelmChart.
		var chart sourcev1.HelmChart
		err := r.Client.Get(ctx, namespacedName, &chart)
		if err != nil && !apierrors.IsNotFound(err) {
			// Return error to retry until we succeed.
			err = fmt.Errorf("failed to delete HelmChart '%s': %w", obj.Status.HelmChart, err)
			return ctrl.Result{}, err
		}
		if err == nil {
			// Delete the HelmChart.
			if err = r.Client.Delete(ctx, &chart); err != nil {
				err = fmt.Errorf("failed to delete HelmChart '%s': %w", obj.Status.HelmChart, err)
				return ctrl.Result{}, err
			}
			r.Eventf(obj, eventv1.EventTypeTrace, "HelmChartDeleted", "deleted HelmChart '%s'", obj.Status.HelmChart)
		}
		// Truncate the chart reference in the status object.
		obj.Status.HelmChart = ""
	}

	if obj.DeletionTimestamp != nil {
		// Remove our finalizer from the list.
		controllerutil.RemoveFinalizer(obj, v2.ChartFinalizer)

		// Stop reconciliation as the object is being deleted.
		return ctrl.Result{}, nil
	}
	return ctrl.Result{Requeue: true}, nil
}

// aclAllowAccessTo returns an acl.AccessDeniedError if the given v2beta1.HelmRelease
// object is not allowed to access the provided name.
func (r *HelmReleaseChartReconciler) aclAllowAccessTo(obj *v2.HelmRelease, name types.NamespacedName) error {
	if !r.NoCrossNamespaceRef {
		return nil
	}
	if obj.Namespace != name.Namespace {
		return acl.AccessDeniedError(fmt.Sprintf("can't access '%s/%s', cross-namespace references have been blocked",
			obj.Spec.Chart.Spec.SourceRef.Kind, types.NamespacedName{
				Namespace: obj.Spec.Chart.Spec.SourceRef.Namespace,
				Name:      obj.Spec.Chart.Spec.SourceRef.Name,
			},
		))
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
			Interval:          template.GetInterval(obj.Spec.Interval),
			ReconcileStrategy: template.Spec.ReconcileStrategy,
			ValuesFiles:       template.Spec.ValuesFiles,
			ValuesFile:        template.Spec.ValuesFile,
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
