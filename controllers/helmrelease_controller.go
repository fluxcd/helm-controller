/*
Copyright 2020 The Flux authors

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
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	kuberecorder "k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/events"
	"github.com/fluxcd/pkg/runtime/metrics"
	"github.com/fluxcd/pkg/runtime/predicates"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta1"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/fluxcd/helm-controller/internal/kube"
	"github.com/fluxcd/helm-controller/internal/runner"
	"github.com/fluxcd/helm-controller/internal/util"
)

// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases/finalizers,verbs=get;create;update;patch;delete
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=helmcharts,verbs=get;list;watch
// +kubebuilder:rbac:groups=source.toolkit.fluxcd.io,resources=helmcharts/status,verbs=get
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// HelmReleaseReconciler reconciles a HelmRelease object
type HelmReleaseReconciler struct {
	client.Client
	Config                *rest.Config
	Log                   logr.Logger
	Scheme                *runtime.Scheme
	requeueDependency     time.Duration
	EventRecorder         kuberecorder.EventRecorder
	ExternalEventRecorder *events.Recorder
	MetricsRecorder       *metrics.Recorder
}

type HelmReleaseReconcilerOptions struct {
	MaxConcurrentReconciles   int
	DependencyRequeueInterval time.Duration
}

func (r *HelmReleaseReconciler) SetupWithManager(mgr ctrl.Manager, opts HelmReleaseReconcilerOptions) error {
	// Index the HelmRelease by the HelmChart references they point at
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &v2.HelmRelease{}, v2.SourceIndexKey,
		func(rawObj runtime.Object) []string {
			hr := rawObj.(*v2.HelmRelease)
			return []string{fmt.Sprintf("%s/%s", hr.Spec.Chart.GetNamespace(hr.Namespace), hr.GetHelmChartName())}
		},
	); err != nil {
		return err
	}

	r.requeueDependency = opts.DependencyRequeueInterval
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2.HelmRelease{}, builder.WithPredicates(predicates.ChangePredicate{})).
		Watches(
			&source.Kind{Type: &sourcev1.HelmChart{}},
			&handler.EnqueueRequestsFromMapFunc{ToRequests: handler.ToRequestsFunc(r.requestsForHelmChartChange)},
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: opts.MaxConcurrentReconciles}).
		Complete(r)
}

func (r *HelmReleaseReconciler) Reconcile(req ctrl.Request) (_ ctrl.Result, retErr error) {
	ctx := context.Background()
	reconcileStart := time.Now()

	hr := &v2.HelmRelease{}
	if err := r.Get(ctx, req.NamespacedName, hr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log := r.Log.WithValues("controller", strings.ToLower(v2.HelmReleaseKind), "request", req.NamespacedName)

	// Return early if the HelmRelease is suspended.
	if hr.Spec.Suspend {
		log.Info("Reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	// Always record the readiness.
	defer r.recordReadiness(*hr)

	// Add our finalizer if it does not exist.
	if !controllerutil.ContainsFinalizer(hr, v2.HelmReleaseFinalizer) {
		controllerutil.AddFinalizer(hr, v2.HelmReleaseFinalizer)
		if err := r.Client.Update(ctx, hr); err != nil {
			log.Error(err, "unable to register finalizer")
			return ctrl.Result{}, err
		}
	}

	// Get the REST client for the Helm operations performed for this release.
	getter, err := r.getRESTClientGetter(ctx, *hr)
	if err != nil {
		v2.HelmReleaseNotReady(hr, v2.InitFailedReason, err.Error())
		if patchErr := r.patchStatus(ctx, hr); patchErr != nil {
			err = utilerrors.NewAggregate([]error{err, patchErr})
		}
		return ctrl.Result{}, err
	}

	// Initialize the Helm action runner.
	run, err := runner.NewRunner(getter, hr, log)
	if err != nil {
		v2.HelmReleaseNotReady(hr, v2.InitFailedReason, err.Error())
		if patchErr := r.patchStatus(ctx, hr); patchErr != nil {
			err = utilerrors.NewAggregate([]error{err, patchErr})
		}
		return ctrl.Result{}, err
	}

	// Examine if our object is under deletion.
	if !hr.ObjectMeta.DeletionTimestamp.IsZero() {
		result, err := r.reconcileDelete(ctx, run, hr)
		if err != nil {
			return result, err
		}
		if err := r.Client.Update(ctx, hr); err != nil {
			return result, err
		}
		return result, retErr
	}

	// Record reconciliation duration.
	if r.MetricsRecorder != nil {
		objRef, err := reference.GetReference(r.Scheme, hr)
		if err != nil {
			return ctrl.Result{}, err
		}
		defer r.MetricsRecorder.RecordDuration(*objRef, reconcileStart)
	}

	defer func() {
		// Record the value of the reconciliation request, this can be used
		// by consumers to observe their request has been handled.
		if v, ok := meta.ReconcileAnnotationValue(hr.GetAnnotations()); ok {
			hr.Status.LastHandledReconcileAt = v
		}

		// Record the observed generation.
		hr.Status.ObservedGeneration = hr.Generation

		// Always attempt to patch the status after each reconciliation.
		if err := r.patchStatus(ctx, hr); err != nil {
			retErr = utilerrors.NewAggregate([]error{retErr, err})
		}
	}()

	// Reconcile the chart for this resource.
	if result, err := r.reconcileChart(ctx, hr); err != nil {
		msg := fmt.Sprintf("HelmChart reconciliation failed: %s", err.Error())
		v2.HelmReleaseNotReady(hr, meta.ReconciliationFailedReason, msg)
		r.event(*hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, msg)
		return result, err
	}

	// Reconcile dependencies before proceeding.
	if result, err := r.reconcileDependencies(hr); err != nil {
		v2.HelmReleaseNotReady(hr, meta.DependencyNotReadyReason, err.Error())
		r.event(*hr, hr.Status.LastAttemptedRevision, events.EventSeverityInfo,
			fmt.Sprintf("%s (retrying in %s)", err.Error(), result.RequeueAfter.String()))
		// We do not return the error here on purpose, as this would
		// result in a back-off strategy.
		return result, nil
	}

	return r.reconcile(ctx, run, hr)
}

func (r *HelmReleaseReconciler) reconcile(ctx context.Context, run *runner.Runner, hr *v2.HelmRelease) (ctrl.Result, error) {
	// Step through the release phases.
	steps := []func(context.Context, *runner.Runner, *v2.HelmRelease) (ctrl.Result, error){
		r.reconcileRelease,
		r.reconcileTest,
		r.reconcileRemediation,
	}
	result := ctrl.Result{}
	var errs []error
	for _, step := range steps {
		stepResult, err := step(ctx, run, hr)
		if err != nil {
			errs = append(errs, err)
		}
		if len(errs) > 0 {
			continue
		}
		result = util.LowestNonZeroResult(result, stepResult)
	}
	return result, utilerrors.NewAggregate(errs)
}

func (r *HelmReleaseReconciler) reconcileDependencies(hr *v2.HelmRelease) (ctrl.Result, error) {
	for _, d := range hr.Spec.DependsOn {
		if d.Namespace == "" {
			d.Namespace = hr.GetNamespace()
		}
		dName := types.NamespacedName(d)
		var dHr v2.HelmRelease
		err := r.Get(context.Background(), dName, &dHr)
		if err != nil {
			if apierrors.IsNotFound(err) {
				return ctrl.Result{RequeueAfter: r.requeueDependency}, fmt.Errorf("'%s' dependency does not exist", dName)
			}
			return ctrl.Result{RequeueAfter: r.requeueDependency}, fmt.Errorf("unable to get '%s' dependency: %w", dName, err)
		}

		if len(dHr.Status.Conditions) == 0 || dHr.Generation != dHr.Status.ObservedGeneration {
			return ctrl.Result{RequeueAfter: r.requeueDependency}, fmt.Errorf("dependency '%s' is not ready", dName)
		}

		if !apimeta.IsStatusConditionTrue(dHr.Status.Conditions, meta.ReadyCondition) {
			return ctrl.Result{RequeueAfter: r.requeueDependency}, fmt.Errorf("dependency '%s' is not ready", dName)
		}
	}
	return ctrl.Result{RequeueAfter: r.requeueDependency}, nil
}

func (r *HelmReleaseReconciler) reconcileDelete(ctx context.Context, run *runner.Runner, hr *v2.HelmRelease) (ctrl.Result, error) {
	if err := run.Uninstall(*hr); err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return ctrl.Result{}, err
	}
	if err := r.deleteHelmChart(ctx, hr); err != nil {
		return ctrl.Result{}, err
	}
	controllerutil.RemoveFinalizer(hr, v2.HelmReleaseFinalizer)
	return ctrl.Result{}, nil
}

func (r *HelmReleaseReconciler) patchStatus(ctx context.Context, hr *v2.HelmRelease) error {
	key, err := client.ObjectKeyFromObject(hr)
	if err != nil {
		return err
	}
	latest := &v2.HelmRelease{}
	if err := r.Client.Get(ctx, key, latest); err != nil {
		return err
	}
	return r.Client.Status().Patch(ctx, hr, client.MergeFrom(latest))
}

func (r *HelmReleaseReconciler) getRESTClientGetter(ctx context.Context, hr v2.HelmRelease) (genericclioptions.RESTClientGetter, error) {
	if hr.Spec.KubeConfig == nil {
		// impersonate service account if specified
		if hr.Spec.ServiceAccountName != "" {
			token, err := r.getServiceAccountToken(ctx, hr)
			if err != nil {
				return nil, fmt.Errorf("could not impersonate ServiceAccount '%s': %w", hr.Spec.ServiceAccountName, err)
			}

			config := *r.Config
			config.BearerToken = token
			return kube.NewInClusterRESTClientGetter(&config, hr.GetReleaseNamespace()), nil
		}

		return kube.NewInClusterRESTClientGetter(r.Config, hr.GetReleaseNamespace()), nil
	}
	secretName := types.NamespacedName{
		Namespace: hr.GetNamespace(),
		Name:      hr.Spec.KubeConfig.SecretRef.Name,
	}
	var secret corev1.Secret
	if err := r.Get(ctx, secretName, &secret); err != nil {
		return nil, fmt.Errorf("could not find KubeConfig secret '%s': %w", secretName, err)
	}
	kubeConfig, ok := secret.Data["value"]
	if !ok {
		return nil, fmt.Errorf("KubeConfig secret '%s' does not contain a 'value' key", secretName)
	}
	return kube.NewMemoryRESTClientGetter(kubeConfig, hr.GetReleaseNamespace()), nil
}

func (r *HelmReleaseReconciler) getServiceAccountToken(ctx context.Context, hr v2.HelmRelease) (string, error) {
	namespacedName := types.NamespacedName{
		Namespace: hr.Namespace,
		Name:      hr.Spec.ServiceAccountName,
	}

	var serviceAccount corev1.ServiceAccount
	err := r.Client.Get(ctx, namespacedName, &serviceAccount)
	if err != nil {
		return "", err
	}

	secretName := types.NamespacedName{
		Namespace: hr.Namespace,
		Name:      hr.Spec.ServiceAccountName,
	}

	for _, secret := range serviceAccount.Secrets {
		if strings.HasPrefix(secret.Name, fmt.Sprintf("%s-token", serviceAccount.Name)) {
			secretName.Name = secret.Name
			break
		}
	}

	var secret corev1.Secret
	err = r.Client.Get(ctx, secretName, &secret)
	if err != nil {
		return "", err
	}

	var token string
	if data, ok := secret.Data["token"]; ok {
		token = string(data)
	} else {
		return "", fmt.Errorf("the service account secret '%s' does not containt a token", secretName.String())
	}

	return token, nil
}

func (r *HelmReleaseReconciler) requestsForHelmChartChange(obj handler.MapObject) []reconcile.Request {
	hc, ok := obj.Object.(*sourcev1.HelmChart)
	if !ok {
		panic(fmt.Sprintf("Expected a HelmChart, got %T", hc))
	}
	// If we do not have an artifact, we have no requests to make.
	if hc.GetArtifact() == nil {
		return nil
	}

	ctx := context.Background()
	var list v2.HelmReleaseList
	if err := r.List(ctx, &list, client.MatchingFields{
		v2.SourceIndexKey: fmt.Sprintf("%s/%s", obj.Meta.GetNamespace(), obj.Meta.GetName()),
	}); err != nil {
		r.Log.Error(err, "failed to list HelmReleases for HelmChart")
		return nil
	}

	var reqs []reconcile.Request
	for _, i := range list.Items {
		// If the revision of the artifact equals to the last attempted revision,
		// we should not make a request for this HelmRelease.
		if hc.GetArtifact().Revision == i.Status.LastAttemptedRevision {
			continue
		}
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: i.GetNamespace(), Name: i.GetName()}}
		reqs = append(reqs, req)
		r.Log.Info("requesting reconciliation due to HelmChart revision change",
			strings.ToLower(v2.HelmReleaseKind), &req,
			"revision", hc.GetArtifact().Revision)
	}
	return reqs
}

// event emits a Kubernetes event and forwards the event to notification controller if configured.
func (r *HelmReleaseReconciler) event(hr v2.HelmRelease, revision, severity, msg string) {
	r.EventRecorder.Event(&hr, "Normal", severity, msg)
	objRef, err := reference.GetReference(r.Scheme, &hr)
	if err != nil {
		r.Log.WithValues(
			"request",
			fmt.Sprintf("%s/%s", hr.GetNamespace(), hr.GetName()),
		).Error(err, "unable to send event")
		return
	}

	if r.ExternalEventRecorder != nil {
		var metadata map[string]string
		if revision != "" {
			metadata = map[string]string{"revision": revision}
		}
		if err := r.ExternalEventRecorder.Eventf(*objRef, metadata, severity, severity, msg); err != nil {
			r.Log.WithValues(
				"request",
				fmt.Sprintf("%s/%s", hr.GetNamespace(), hr.GetName()),
			).Error(err, "unable to send event")
			return
		}
	}
}

func (r *HelmReleaseReconciler) recordReadiness(hr v2.HelmRelease) {
	if r.MetricsRecorder == nil {
		return
	}

	objRef, err := reference.GetReference(r.Scheme, &hr)
	if err != nil {
		r.Log.WithValues(
			strings.ToLower(hr.Kind),
			fmt.Sprintf("%s/%s", hr.GetNamespace(), hr.GetName()),
		).Error(err, "unable to record readiness metric")
		return
	}
	if rc := apimeta.FindStatusCondition(hr.Status.Conditions, meta.ReadyCondition); rc != nil {
		r.MetricsRecorder.RecordCondition(*objRef, *rc, !hr.DeletionTimestamp.IsZero())
	} else {
		r.MetricsRecorder.RecordCondition(*objRef, metav1.Condition{
			Type:   meta.ReadyCondition,
			Status: metav1.ConditionUnknown,
		}, !hr.DeletionTimestamp.IsZero())
	}
}
