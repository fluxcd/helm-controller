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

package controller

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	kuberecorder "k8s.io/client-go/tools/record"
	"sigs.k8s.io/cli-utils/pkg/kstatus/polling"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/ratelimiter"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apiacl "github.com/fluxcd/pkg/apis/acl"
	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/acl"
	runtimeClient "github.com/fluxcd/pkg/runtime/client"
	"github.com/fluxcd/pkg/runtime/conditions"
	helper "github.com/fluxcd/pkg/runtime/controller"
	"github.com/fluxcd/pkg/runtime/jitter"
	"github.com/fluxcd/pkg/runtime/predicates"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
	intchartutil "github.com/fluxcd/helm-controller/internal/chartutil"
	"github.com/fluxcd/helm-controller/internal/diff"
	"github.com/fluxcd/helm-controller/internal/features"
	"github.com/fluxcd/helm-controller/internal/kube"
	"github.com/fluxcd/helm-controller/internal/loader"
	intpredicates "github.com/fluxcd/helm-controller/internal/predicates"
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
	helper.Metrics

	Config              *rest.Config
	Scheme              *runtime.Scheme
	EventRecorder       kuberecorder.EventRecorder
	NoCrossNamespaceRef bool
	ClientOpts          runtimeClient.Options
	KubeConfigOpts      runtimeClient.KubeConfigOptions
	StatusPoller        *polling.StatusPoller
	PollingOpts         polling.Options
	ControllerName      string

	httpClient        *retryablehttp.Client
	requeueDependency time.Duration
}

type HelmReleaseReconcilerOptions struct {
	HTTPRetry                 int
	DependencyRequeueInterval time.Duration
	RateLimiter               ratelimiter.RateLimiter
}

func (r *HelmReleaseReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager, opts HelmReleaseReconcilerOptions) error {
	// Index the HelmRelease by the HelmChart references they point at
	if err := mgr.GetFieldIndexer().IndexField(ctx, &v2.HelmRelease{}, v2.SourceIndexKey,
		func(o client.Object) []string {
			hr := o.(*v2.HelmRelease)
			return []string{
				fmt.Sprintf("%s/%s", hr.Spec.Chart.GetNamespace(hr.GetNamespace()), hr.GetHelmChartName()),
			}
		},
	); err != nil {
		return err
	}

	r.requeueDependency = opts.DependencyRequeueInterval

	// Configure the retryable http client used for fetching artifacts.
	// By default, it retries 10 times within a 3.5 minutes window.
	httpClient := retryablehttp.NewClient()
	httpClient.RetryWaitMin = 5 * time.Second
	httpClient.RetryWaitMax = 30 * time.Second
	httpClient.RetryMax = opts.HTTPRetry
	httpClient.Logger = nil
	r.httpClient = httpClient

	return ctrl.NewControllerManagedBy(mgr).
		For(&v2.HelmRelease{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicates.ReconcileRequestedPredicate{}),
		)).
		Watches(
			&sourcev1.HelmChart{},
			handler.EnqueueRequestsFromMapFunc(r.requestsForHelmChartChange),
			builder.WithPredicates(intpredicates.SourceRevisionChangePredicate{}),
		).
		WithOptions(controller.Options{
			RateLimiter: opts.RateLimiter,
		}).
		Complete(r)
}

// ConditionError represents an error with a status condition reason attached.
type ConditionError struct {
	Reason string
	Err    error
}

func (c ConditionError) Error() string {
	return c.Err.Error()
}

func (r *HelmReleaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	log := ctrl.LoggerFrom(ctx)

	var hr v2.HelmRelease
	if err := r.Get(ctx, req.NamespacedName, &hr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	defer func() {
		// Always record metrics.
		r.Metrics.RecordSuspend(ctx, &hr, hr.Spec.Suspend)
		r.Metrics.RecordReadiness(ctx, &hr)
		r.Metrics.RecordDuration(ctx, &hr, start)
	}()

	// Add our finalizer if it does not exist
	if !controllerutil.ContainsFinalizer(&hr, v2.HelmReleaseFinalizer) {
		patch := client.MergeFrom(hr.DeepCopy())
		controllerutil.AddFinalizer(&hr, v2.HelmReleaseFinalizer)
		if err := r.Patch(ctx, &hr, patch); err != nil {
			log.Error(err, "unable to register finalizer")
			return ctrl.Result{}, err
		}
	}

	// Examine if the object is under deletion
	if !hr.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &hr)
	}

	// Return early if the HelmRelease is suspended.
	if hr.Spec.Suspend {
		log.Info("Reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	hr, result, err := r.reconcile(ctx, hr)

	// Update status after reconciliation.
	if updateStatusErr := r.patchStatus(ctx, &hr); updateStatusErr != nil {
		log.Error(updateStatusErr, "unable to update status after reconciliation")
		return ctrl.Result{Requeue: true}, updateStatusErr
	}

	// Log reconciliation duration
	durationMsg := fmt.Sprintf("reconciliation finished in %s", time.Since(start).String())
	if result.RequeueAfter > 0 {
		durationMsg = fmt.Sprintf("%s, next run in %s", durationMsg, result.RequeueAfter.String())
	}
	log.Info(durationMsg)

	return result, err
}

func (r *HelmReleaseReconciler) reconcile(ctx context.Context, hr v2.HelmRelease) (v2.HelmRelease, ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)
	// Record the value of the reconciliation request, if any
	if v, ok := meta.ReconcileAnnotationValue(hr.GetAnnotations()); ok {
		hr.Status.SetLastHandledReconcileRequest(v)
	}

	// Observe HelmRelease generation.
	if hr.Status.ObservedGeneration != hr.Generation {
		hr.Status.ObservedGeneration = hr.Generation
		hr = v2.HelmReleaseProgressing(hr)
		if updateStatusErr := r.patchStatus(ctx, &hr); updateStatusErr != nil {
			log.Error(updateStatusErr, "unable to update status after generation update")
			return hr, ctrl.Result{Requeue: true}, updateStatusErr
		}
	}

	// Get HelmChart object for release
	hc, err := r.getHelmChart(ctx, &hr)
	if err != nil {
		if acl.IsAccessDenied(err) {
			log.Error(err, "access denied to cross-namespace source")
			r.event(ctx, hr, hr.Status.LastAttemptedRevision, eventv1.EventSeverityError, err.Error())
			return v2.HelmReleaseNotReady(hr, apiacl.AccessDeniedReason, err.Error()),
				jitter.JitteredRequeueInterval(ctrl.Result{RequeueAfter: hr.GetRequeueAfter()}), nil
		}

		msg := fmt.Sprintf("chart reconciliation failed: %s", err.Error())
		r.event(ctx, hr, hr.Status.LastAttemptedRevision, eventv1.EventSeverityError, msg)
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, msg), ctrl.Result{Requeue: true}, err
	}

	// Check chart readiness
	if hc.Generation != hc.Status.ObservedGeneration || !apimeta.IsStatusConditionTrue(hc.Status.Conditions, meta.ReadyCondition) {
		msg := fmt.Sprintf("HelmChart '%s/%s' is not ready", hc.GetNamespace(), hc.GetName())
		r.event(ctx, hr, hr.Status.LastAttemptedRevision, eventv1.EventSeverityInfo, msg)
		log.Info(msg)
		// Do not requeue immediately, when the artifact is created
		// the watcher should trigger a reconciliation.
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, msg), jitter.JitteredRequeueInterval(ctrl.Result{RequeueAfter: hr.GetRequeueAfter()}), nil
	}

	// Check dependencies
	if len(hr.Spec.DependsOn) > 0 {
		if err := r.checkDependencies(hr); err != nil {
			msg := fmt.Sprintf("dependencies do not meet ready condition (%s), retrying in %s",
				err.Error(), r.requeueDependency.String())
			r.event(ctx, hr, hc.GetArtifact().Revision, eventv1.EventSeverityInfo, msg)
			log.Info(msg)

			// Exponential backoff would cause execution to be prolonged too much,
			// instead we requeue on a fixed interval.
			return v2.HelmReleaseNotReady(hr,
				v2.DependencyNotReadyReason, err.Error()), ctrl.Result{RequeueAfter: r.requeueDependency}, nil
		}
		log.Info("all dependencies are ready, proceeding with release")
	}

	// Compose values
	values, err := intchartutil.ChartValuesFromReferences(ctx, r.Client, hr.Namespace, hr.GetValues(), hr.Spec.ValuesFrom...)
	if err != nil {
		r.event(ctx, hr, hr.Status.LastAttemptedRevision, eventv1.EventSeverityError, err.Error())
		return v2.HelmReleaseNotReady(hr, v2.InitFailedReason, err.Error()), ctrl.Result{Requeue: true}, nil
	}

	// Load chart from artifact
	loadedChart, err := loader.SecureLoadChartFromURL(r.httpClient, hc.GetArtifact().URL, hc.GetArtifact().Digest)
	if err != nil {
		r.event(ctx, hr, hr.Status.LastAttemptedRevision, eventv1.EventSeverityError, err.Error())
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, err.Error()), ctrl.Result{Requeue: true}, nil
	}

	// Reconcile Helm release
	reconciledHr, reconcileErr := r.reconcileRelease(ctx, *hr.DeepCopy(), loadedChart, values)
	if reconcileErr != nil {
		r.event(ctx, hr, hc.GetArtifact().Revision, eventv1.EventSeverityError,
			fmt.Sprintf("reconciliation failed: %s", reconcileErr.Error()))
	}
	return reconciledHr, jitter.JitteredRequeueInterval(ctrl.Result{RequeueAfter: hr.GetRequeueAfter()}), reconcileErr
}

func (r *HelmReleaseReconciler) reconcileRelease(ctx context.Context,
	hr v2.HelmRelease, chart *chart.Chart, values chartutil.Values) (v2.HelmRelease, error) {
	log := ctrl.LoggerFrom(ctx)

	// Initialize Helm action runner
	getter, err := r.buildRESTClientGetter(ctx, hr)
	if err != nil {
		return v2.HelmReleaseNotReady(hr, v2.InitFailedReason, err.Error()), err
	}
	run, err := runner.NewRunner(getter, hr.GetStorageNamespace(), log)
	if err != nil {
		return v2.HelmReleaseNotReady(hr, v2.InitFailedReason, "failed to initialize Helm action runner"), err
	}

	// Determine last release revision.
	rel, observeLastReleaseErr := run.ObserveLastRelease(hr)
	if observeLastReleaseErr != nil {
		err = fmt.Errorf("failed to get last release revision: %w", observeLastReleaseErr)
		return v2.HelmReleaseNotReady(hr, v2.GetLastReleaseFailedReason, "failed to get last release revision"), err
	}

	// Detect divergence between release in storage and HelmRelease spec.
	revision := chart.Metadata.Version
	releaseRevision := util.ReleaseRevision(rel)
	// TODO: deprecate "unordered" checksum.
	valuesChecksum := util.OrderedValuesChecksum(values)
	hasNewState := v2.HelmReleaseChanged(hr, revision, releaseRevision, util.ValuesChecksum(values), valuesChecksum)

	// Register the current release attempt.
	v2.HelmReleaseRecordAttempt(&hr, revision, releaseRevision, valuesChecksum)

	// Run diff against current cluster state.
	if !hasNewState && rel != nil {
		if ok, _ := features.Enabled(features.DetectDrift); ok {
			differ := diff.NewDiffer(runtimeClient.NewImpersonator(
				r.Client,
				r.StatusPoller,
				r.PollingOpts,
				hr.Spec.KubeConfig,
				r.KubeConfigOpts,
				kube.DefaultServiceAccountName,
				hr.Spec.ServiceAccountName,
				hr.GetNamespace(),
			), r.ControllerName)

			changeSet, drift, err := differ.Diff(ctx, rel)
			if err != nil {
				if changeSet == nil {
					msg := "failed to diff release against cluster resources"
					r.event(ctx, hr, rel.Chart.Metadata.Version, eventv1.EventSeverityError, err.Error())
					return v2.HelmReleaseNotReady(hr, "DiffFailed", fmt.Sprintf("%s: %s", msg, err.Error())), err
				}
				log.Error(err, "diff of release against cluster resources finished with error")
			}

			msg := "no diff in cluster resources compared to release"
			if drift {
				msg = "diff in cluster resources compared to release"
				hasNewState, _ = features.Enabled(features.CorrectDrift)
			}
			if changeSet != nil {
				msg = fmt.Sprintf("%s:\n\n%s", msg, changeSet.String())
				r.event(ctx, hr, rel.Chart.Metadata.Version, eventv1.EventSeverityInfo, msg)
			}
			log.Info(msg)
		}
	}

	if hasNewState {
		hr = v2.HelmReleaseProgressing(hr)
		if updateStatusErr := r.patchStatus(ctx, &hr); updateStatusErr != nil {
			log.Error(updateStatusErr, "unable to update status after state update")
			return hr, updateStatusErr
		}
	}

	// Check status of any previous release attempt.
	released := apimeta.FindStatusCondition(hr.Status.Conditions, v2.ReleasedCondition)
	if released != nil {
		switch released.Status {
		// Succeed if the previous release attempt succeeded.
		case metav1.ConditionTrue:
			return v2.HelmReleaseReady(hr), nil
		case metav1.ConditionFalse:
			// Fail if the previous release attempt remediation failed.
			remediated := apimeta.FindStatusCondition(hr.Status.Conditions, v2.RemediatedCondition)
			if remediated != nil && remediated.Status == metav1.ConditionFalse {
				err = fmt.Errorf("previous release attempt remediation failed")
				return v2.HelmReleaseNotReady(hr, remediated.Reason, remediated.Message), err
			}
		}

		// Fail if install retries are exhausted.
		if hr.Spec.GetInstall().GetRemediation().RetriesExhausted(hr) {
			err = fmt.Errorf("install retries exhausted")
			return v2.HelmReleaseNotReady(hr, released.Reason, err.Error()), err
		}

		// Fail if there is a release and upgrade retries are exhausted.
		// This avoids failing after an upgrade uninstall remediation strategy.
		if rel != nil && hr.Spec.GetUpgrade().GetRemediation().RetriesExhausted(hr) {
			err = fmt.Errorf("upgrade retries exhausted")
			return v2.HelmReleaseNotReady(hr, released.Reason, err.Error()), err
		}
	}

	// Deploy the release.
	var deployAction v2.DeploymentAction
	if rel == nil {
		r.event(ctx, hr, revision, eventv1.EventSeverityInfo, "Helm install has started")
		deployAction = hr.Spec.GetInstall()
		rel, err = run.Install(ctx, hr, chart, values)
		err = r.handleHelmActionResult(ctx, &hr, revision, err, deployAction.GetDescription(),
			v2.ReleasedCondition, v2.InstallSucceededReason, v2.InstallFailedReason)
	} else {
		r.event(ctx, hr, revision, eventv1.EventSeverityInfo, "Helm upgrade has started")
		deployAction = hr.Spec.GetUpgrade()
		rel, err = run.Upgrade(ctx, hr, chart, values)
		err = r.handleHelmActionResult(ctx, &hr, revision, err, deployAction.GetDescription(),
			v2.ReleasedCondition, v2.UpgradeSucceededReason, v2.UpgradeFailedReason)
	}
	remediation := deployAction.GetRemediation()

	// If there is a new release revision...
	if util.ReleaseRevision(rel) > releaseRevision {
		// Ensure release is not marked remediated.
		apimeta.RemoveStatusCondition(&hr.Status.Conditions, v2.RemediatedCondition)

		// If new release revision is successful and tests are enabled, run them.
		if err == nil && hr.Spec.GetTest().Enable {
			_, testErr := run.Test(hr)
			testErr = r.handleHelmActionResult(ctx, &hr, revision, testErr, "test",
				v2.TestSuccessCondition, v2.TestSucceededReason, v2.TestFailedReason)

			// Propagate any test error if not marked ignored.
			if testErr != nil && !remediation.MustIgnoreTestFailures(hr.Spec.GetTest().IgnoreFailures) {
				testsPassing := apimeta.FindStatusCondition(hr.Status.Conditions, v2.TestSuccessCondition)
				newCondition := metav1.Condition{
					Type:    v2.ReleasedCondition,
					Status:  metav1.ConditionFalse,
					Reason:  testsPassing.Reason,
					Message: testsPassing.Message,
				}
				apimeta.SetStatusCondition(hr.GetStatusConditions(), newCondition)
				err = testErr
			}
		}
	}

	if err != nil {
		// Increment failure count for deployment action.
		remediation.IncrementFailureCount(&hr)
		// Remediate deployment failure if necessary.
		if !remediation.RetriesExhausted(hr) || remediation.MustRemediateLastFailure() {
			if util.ReleaseRevision(rel) <= releaseRevision {
				log.Info("skipping remediation, no new release revision created")
			} else {
				var remediationErr error
				switch remediation.GetStrategy() {
				case v2.RollbackRemediationStrategy:
					rollbackErr := run.Rollback(hr)
					remediationErr = r.handleHelmActionResult(ctx, &hr, revision, rollbackErr, "rollback",
						v2.RemediatedCondition, v2.RollbackSucceededReason, v2.RollbackFailedReason)
				case v2.UninstallRemediationStrategy:
					uninstallErr := run.Uninstall(hr)
					remediationErr = r.handleHelmActionResult(ctx, &hr, revision, uninstallErr, "uninstall",
						v2.RemediatedCondition, v2.UninstallSucceededReason, v2.UninstallFailedReason)
				}
				if remediationErr != nil {
					err = remediationErr
				}
			}

			// Determine release after remediation.
			rel, observeLastReleaseErr = run.ObserveLastRelease(hr)
			if observeLastReleaseErr != nil {
				err = &ConditionError{
					Reason: v2.GetLastReleaseFailedReason,
					Err:    errors.New("failed to get last release revision after remediation"),
				}
			}
		}
	}

	hr.Status.LastReleaseRevision = util.ReleaseRevision(rel)
	if updateStatusErr := r.patchStatus(ctx, &hr); updateStatusErr != nil {
		log.Error(updateStatusErr, "unable to update status after state update")
		return hr, updateStatusErr
	}

	if err != nil {
		reason := v2.ReconciliationFailedReason
		if condErr := (*ConditionError)(nil); errors.As(err, &condErr) {
			reason = condErr.Reason
		}
		return v2.HelmReleaseNotReady(hr, reason, err.Error()), err
	}
	return v2.HelmReleaseReady(hr), nil
}

func (r *HelmReleaseReconciler) checkDependencies(hr v2.HelmRelease) error {
	for _, d := range hr.Spec.DependsOn {
		if d.Namespace == "" {
			d.Namespace = hr.GetNamespace()
		}
		dName := types.NamespacedName{
			Namespace: d.Namespace,
			Name:      d.Name,
		}
		var dHr v2.HelmRelease
		err := r.Get(context.Background(), dName, &dHr)
		if err != nil {
			return fmt.Errorf("unable to get '%s' dependency: %w", dName, err)
		}

		if len(dHr.Status.Conditions) == 0 || dHr.Generation != dHr.Status.ObservedGeneration {
			return fmt.Errorf("dependency '%s' is not ready", dName)
		}

		if !apimeta.IsStatusConditionTrue(dHr.Status.Conditions, meta.ReadyCondition) {
			return fmt.Errorf("dependency '%s' is not ready", dName)
		}
	}
	return nil
}

func (r *HelmReleaseReconciler) buildRESTClientGetter(ctx context.Context, hr v2.HelmRelease) (genericclioptions.RESTClientGetter, error) {
	opts := []kube.Option{
		kube.WithNamespace(hr.GetReleaseNamespace()),
		kube.WithClientOptions(r.ClientOpts),
		// When ServiceAccountName is empty, it will fall back to the configured default.
		// If this is not configured either, this option will result in a no-op.
		kube.WithImpersonate(hr.Spec.ServiceAccountName, hr.GetNamespace()),
		kube.WithPersistent(hr.UsePersistentClient()),
	}
	if hr.Spec.KubeConfig != nil {
		secretName := types.NamespacedName{
			Namespace: hr.GetNamespace(),
			Name:      hr.Spec.KubeConfig.SecretRef.Name,
		}
		var secret corev1.Secret
		if err := r.Get(ctx, secretName, &secret); err != nil {
			return nil, fmt.Errorf("could not find KubeConfig secret '%s': %w", secretName, err)
		}
		kubeConfig, err := kube.ConfigFromSecret(&secret, hr.Spec.KubeConfig.SecretRef.Key, r.KubeConfigOpts)
		if err != nil {
			return nil, err
		}
		return kube.NewMemoryRESTClientGetter(kubeConfig, opts...), nil
	}
	return kube.NewInClusterMemoryRESTClientGetter(opts...)
}

// getHelmChart retrieves the v1beta2.HelmChart for the given
// v2beta1.HelmRelease using the name that is advertised in the status
// object. It returns the v1beta2.HelmChart, or an error.
func (r *HelmReleaseReconciler) getHelmChart(ctx context.Context, hr *v2.HelmRelease) (*sourcev1.HelmChart, error) {
	namespace, name := hr.Status.GetHelmChart()
	chartName := types.NamespacedName{Namespace: namespace, Name: name}
	if r.NoCrossNamespaceRef && chartName.Namespace != hr.Namespace {
		return nil, acl.AccessDeniedError(fmt.Sprintf("can't access '%s/%s', cross-namespace references have been blocked",
			hr.Spec.Chart.Spec.SourceRef.Kind, types.NamespacedName{
				Namespace: hr.Spec.Chart.Spec.SourceRef.Namespace,
				Name:      hr.Spec.Chart.Spec.SourceRef.Name,
			}))
	}
	hc := sourcev1.HelmChart{}
	if err := r.Client.Get(ctx, chartName, &hc); err != nil {
		return nil, err
	}
	return &hc, nil
}

// reconcileDelete deletes the v1beta2.HelmChart of the v2beta1.HelmRelease,
// and uninstalls the Helm release if the resource has not been suspended.
// It only performs a Helm uninstall if the ServiceAccount to be impersonated
// exists.
func (r *HelmReleaseReconciler) reconcileDelete(ctx context.Context, hr *v2.HelmRelease) (ctrl.Result, error) {
	log := ctrl.LoggerFrom(ctx)

	// Only uninstall the Helm Release if the resource is not suspended.
	if !hr.Spec.Suspend {
		impersonator := runtimeClient.NewImpersonator(
			r.Client,
			r.StatusPoller,
			r.PollingOpts,
			hr.Spec.KubeConfig,
			r.KubeConfigOpts,
			kube.DefaultServiceAccountName,
			hr.Spec.ServiceAccountName,
			hr.GetNamespace(),
		)

		if impersonator.CanImpersonate(ctx) {
			getter, err := r.buildRESTClientGetter(ctx, *hr)
			if err != nil {
				return ctrl.Result{}, err
			}
			run, err := runner.NewRunner(getter, hr.GetStorageNamespace(), ctrl.LoggerFrom(ctx))
			if err != nil {
				return ctrl.Result{}, err
			}
			if err := run.Uninstall(*hr); err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
				return ctrl.Result{}, err
			}
			log.Info("uninstalled Helm release for deleted resource")
		} else {
			err := fmt.Errorf("failed to find service account to impersonate")
			msg := "skipping Helm uninstall"
			log.Error(err, msg)
			r.event(ctx, *hr, hr.Status.LastAppliedRevision, eventv1.EventSeverityError, fmt.Sprintf("%s: %s", msg, err.Error()))
		}
	} else {
		ctrl.LoggerFrom(ctx).Info("skipping Helm uninstall for suspended resource")
	}

	// Remove our finalizer from the list and update it.
	controllerutil.RemoveFinalizer(hr, v2.HelmReleaseFinalizer)
	if err := r.Update(ctx, hr); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *HelmReleaseReconciler) handleHelmActionResult(ctx context.Context,
	hr *v2.HelmRelease, revision string, err error, action string, condition string, succeededReason string, failedReason string) error {
	if err != nil {
		err = fmt.Errorf("Helm %s failed: %w", action, err)
		msg := err.Error()
		if actionErr := (*runner.ActionError)(nil); errors.As(err, &actionErr) {
			msg = strings.TrimSpace(msg) + "\n\nLast Helm logs:\n\n" + actionErr.CapturedLogs
		}
		newCondition := metav1.Condition{
			Type:    condition,
			Status:  metav1.ConditionFalse,
			Reason:  failedReason,
			Message: msg,
		}
		apimeta.SetStatusCondition(hr.GetStatusConditions(), newCondition)
		r.event(ctx, *hr, revision, eventv1.EventSeverityError, msg)
		return &ConditionError{Reason: failedReason, Err: err}
	} else {
		msg := fmt.Sprintf("Helm %s succeeded", action)
		newCondition := metav1.Condition{
			Type:    condition,
			Status:  metav1.ConditionTrue,
			Reason:  succeededReason,
			Message: msg,
		}
		apimeta.SetStatusCondition(hr.GetStatusConditions(), newCondition)
		r.event(ctx, *hr, revision, eventv1.EventSeverityInfo, msg)
		return nil
	}
}

func (r *HelmReleaseReconciler) patchStatus(ctx context.Context, hr *v2.HelmRelease) error {
	latest := &v2.HelmRelease{}
	if err := r.Client.Get(ctx, client.ObjectKeyFromObject(hr), latest); err != nil {
		return err
	}
	patch := client.MergeFrom(latest.DeepCopy())
	latest.Status = hr.Status
	return r.Client.Status().Patch(ctx, latest, patch, client.FieldOwner(r.ControllerName))
}

func (r *HelmReleaseReconciler) requestsForHelmChartChange(ctx context.Context, o client.Object) []reconcile.Request {
	hc, ok := o.(*sourcev1.HelmChart)
	if !ok {
		err := fmt.Errorf("expected a HelmChart, got %T", o)
		ctrl.LoggerFrom(ctx).Error(err, "failed to get requests for HelmChart change")
		return nil
	}
	// If we do not have an artifact, we have no requests to make
	if hc.GetArtifact() == nil {
		return nil
	}

	var list v2.HelmReleaseList
	if err := r.List(ctx, &list, client.MatchingFields{
		v2.SourceIndexKey: client.ObjectKeyFromObject(hc).String(),
	}); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "failed to list HelmReleases for HelmChart change")
		return nil
	}

	var reqs []reconcile.Request
	for i, hr := range list.Items {
		// If the HelmRelease is ready and the revision of the artifact equals to the
		// last attempted revision, we should not make a request for this HelmRelease
		if conditions.IsReady(&list.Items[i]) && hc.GetArtifact().HasRevision(hr.Status.LastAttemptedRevision) {
			continue
		}
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&list.Items[i])})
	}
	return reqs
}

// event emits a Kubernetes event and forwards the event to notification controller if configured.
func (r *HelmReleaseReconciler) event(_ context.Context, hr v2.HelmRelease, revision, severity, msg string) {
	var eventMeta map[string]string

	if revision != "" || hr.Status.LastAttemptedValuesChecksum != "" {
		eventMeta = make(map[string]string)
		if revision != "" {
			eventMeta[v2.GroupVersion.Group+"/"+eventv1.MetaRevisionKey] = revision
		}
		if hr.Status.LastAttemptedValuesChecksum != "" {
			eventMeta[v2.GroupVersion.Group+"/"+eventv1.MetaTokenKey] = hr.Status.LastAttemptedValuesChecksum
		}
	}

	eventType := corev1.EventTypeNormal
	if severity == eventv1.EventSeverityError {
		eventType = corev1.EventTypeWarning
	}
	r.EventRecorder.AnnotatedEventf(&hr, eventMeta, eventType, severity, msg)
}
