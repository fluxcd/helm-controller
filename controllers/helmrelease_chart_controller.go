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

package controllers

import (
	"context"
	"fmt"

	"github.com/google/go-cmp/cmp"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	kuberecorder "k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/ratelimiter"

	"github.com/fluxcd/pkg/runtime/acl"
	helper "github.com/fluxcd/pkg/runtime/controller"
	"github.com/fluxcd/pkg/runtime/events"
	"github.com/fluxcd/pkg/runtime/patch"
	"github.com/fluxcd/pkg/runtime/predicates"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"

	"github.com/fluxcd/helm-controller/api/v2beta1"
	intcmp "github.com/fluxcd/helm-controller/internal/cmp"
	intpredicates "github.com/fluxcd/helm-controller/internal/predicates"
)

type HelmReleaseChartReconciler struct {
	client.Client
	kuberecorder.EventRecorder
	helper.Metrics

	ControllerName      string
	NoCrossNamespaceRef bool
}

type HelmReleaseChartReconcilerOptions struct {
	MaxConcurrentReconciles int
	RateLimiter             ratelimiter.RateLimiter
}

func (r *HelmReleaseChartReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return r.SetupWithManagerAndOptions(mgr, HelmReleaseChartReconcilerOptions{})
}

func (r *HelmReleaseChartReconciler) SetupWithManagerAndOptions(mgr ctrl.Manager, opts HelmReleaseChartReconcilerOptions) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2beta1.HelmRelease{}).
		WithEventFilter(predicate.Or(intpredicates.ChartTemplateChangePredicate{}, predicates.ReconcileRequestedPredicate{})).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: opts.MaxConcurrentReconciles,
			RateLimiter:             opts.RateLimiter,
		}).
		Complete(r)
}

func (r *HelmReleaseChartReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	log := ctrl.LoggerFrom(ctx)

	// Fetch the HelmRelease
	obj := &v2beta1.HelmRelease{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Record suspended status metric
	r.RecordSuspend(ctx, obj, obj.Spec.Suspend)

	// Return early if the object is suspended
	if obj.Spec.Suspend {
		log.Info("reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper with the current version of the object.
	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always attempt to patch the object after each reconciliation.
	defer func() {
		if err = patchHelper.Patch(ctx, obj); err != nil {
			if retErr != nil {
				retErr = errors.NewAggregate([]error{retErr, err})
			} else {
				retErr = err
			}
		}
	}()

	// Add finalizer first if not exist to avoid the race condition
	// between init and delete.
	if !controllerutil.ContainsFinalizer(obj, v2beta1.ChartFinalizer) {
		controllerutil.AddFinalizer(obj, v2beta1.ChartFinalizer)
		return ctrl.Result{Requeue: true}, nil
	}

	// Examine if the object is under deletion.
	if !obj.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, obj)
	}

	return r.reconcile(ctx, obj)
}

func (r *HelmReleaseChartReconciler) reconcile(ctx context.Context, obj *v2beta1.HelmRelease) (ctrl.Result, error) {
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

	// Attempt to fetch the HelmChart, ignoring not found errors.
	var curChart sourcev1.HelmChart
	err := r.Client.Get(ctx, chartRef, &curChart)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, err
	}

	// Build new HelmChart based on the declared template.
	newChart := buildHelmChartFromTemplate(obj)

	// HelmChart was not found, create it.
	if apierrors.IsNotFound(err) {
		if err = r.Client.Create(ctx, newChart); err != nil {
			return ctrl.Result{}, err
		}
		obj.Status.HelmChart = chartRef.String()
		r.Eventf(obj, events.EventSeverityTrace, "HelmChartCreated", "created HelmChart '%s' with SourceRef '%s/%s/%s'",
			obj.Status.HelmChart, obj.Kind, obj.Namespace, obj.Name)
		return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
	}

	// Check if the current spec diverges from the newly calculated spec.
	if diff, eq := helmChartSpecDiff(curChart.Spec, newChart.Spec); !eq {
		r.Eventf(obj, events.EventSeverityTrace, "HelmChartDrifted", "drift detected in HelmChart '%s':\n\n%s",
			obj.Status.HelmChart, diff)

		// Update the spec of the HelmChart.
		curChart.Spec = newChart.Spec
		if err = r.Client.Update(ctx, &curChart); err != nil {
			return ctrl.Result{}, err
		}
	}

	// From this moment on, we know the HelmChart spec is up-to-date.
	obj.Status.HelmChart = chartRef.String()

	// Requeue to ensure the state continues to be the same.
	return ctrl.Result{RequeueAfter: obj.GetRequeueAfter()}, nil
}

// reconcileDelete handles the garbage collection of the current HelmChart in
// the Status object of the given HelmRelease.
func (r *HelmReleaseChartReconciler) reconcileDelete(ctx context.Context, obj *v2beta1.HelmRelease) (ctrl.Result, error) {
	if obj.Status.HelmChart != "" {
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
			r.Eventf(obj, events.EventSeverityTrace, "HelmChartDeleted", "deleted HelmChart '%s'", obj.Status.HelmChart)
		}
		// Truncate the chart reference in the status object.
		obj.Status.HelmChart = ""
	}

	if obj.DeletionTimestamp != nil {
		// Remove our finalizer from the list.
		controllerutil.RemoveFinalizer(obj, v2beta1.ChartFinalizer)

		// Stop reconciliation as the object is being deleted.
		return ctrl.Result{}, nil
	}

	return ctrl.Result{Requeue: true}, nil
}

// aclAllowAccessTo returns an acl.AccessDeniedError if the given v2beta1.HelmRelease
// object is not allowed to access the provided name.
func (r *HelmReleaseChartReconciler) aclAllowAccessTo(obj *v2beta1.HelmRelease, name types.NamespacedName) error {
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
func buildHelmChartFromTemplate(obj *v2beta1.HelmRelease) *sourcev1.HelmChart {
	template := obj.Spec.Chart
	return &sourcev1.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Name:      obj.GetHelmChartName(),
			Namespace: obj.Spec.Chart.GetNamespace(obj.Namespace),
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
}

// helmChartSpecDiff returns if the two v1beta1.HelmChartSpec differ.
func helmChartSpecDiff(cur, new sourcev1.HelmChartSpec) (diff string, eq bool) {
	r := intcmp.SimpleReporter{}
	if diff := cmp.Diff(cur, new, cmp.Reporter(&r)); diff != "" {
		return r.String(), false
	}
	return "", true
}
