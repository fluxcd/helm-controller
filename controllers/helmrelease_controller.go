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

	"github.com/hashicorp/go-retryablehttp"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/strvals"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/ratelimiter"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	apiacl "github.com/fluxcd/pkg/apis/acl"
	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/acl"
	fluxClient "github.com/fluxcd/pkg/runtime/client"
	"github.com/fluxcd/pkg/runtime/metrics"
	"github.com/fluxcd/pkg/runtime/predicates"
	"github.com/fluxcd/pkg/runtime/transform"
	sourcev1 "github.com/fluxcd/source-controller/api/v1beta2"

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
	httpClient            *retryablehttp.Client
	Config                *rest.Config
	Scheme                *runtime.Scheme
	requeueDependency     time.Duration
	EventRecorder         kuberecorder.EventRecorder
	MetricsRecorder       *metrics.Recorder
	DefaultServiceAccount string
	NoCrossNamespaceRef   bool
	ClientOpts            fluxClient.Options
	KubeConfigOpts        fluxClient.KubeConfigOptions
}

func (r *HelmReleaseReconciler) SetupWithManager(mgr ctrl.Manager, opts HelmReleaseReconcilerOptions) error {
	// Index the HelmRelease by the HelmChart references they point at
	if err := mgr.GetFieldIndexer().IndexField(context.TODO(), &v2.HelmRelease{}, v2.SourceIndexKey,
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
	// By default it retries 10 times within a 3.5 minutes window.
	httpClient := retryablehttp.NewClient()
	httpClient.RetryWaitMin = 5 * time.Second
	httpClient.RetryWaitMax = 30 * time.Second
	httpClient.RetryMax = opts.HTTPRetry
	httpClient.Logger = nil
	r.httpClient = httpClient

	recoverPanic := true
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2.HelmRelease{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicates.ReconcileRequestedPredicate{}),
		)).
		Watches(
			&source.Kind{Type: &sourcev1.HelmChart{}},
			handler.EnqueueRequestsFromMapFunc(r.requestsForHelmChartChange),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: opts.MaxConcurrentReconciles,
			RateLimiter:             opts.RateLimiter,
			RecoverPanic:            &recoverPanic,
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

	// record suspension metrics
	defer r.recordSuspension(ctx, hr)

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
		return r.reconcileDelete(ctx, hr)
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

	// Record ready status
	r.recordReadiness(ctx, hr)

	// Log reconciliation duration
	durationMsg := fmt.Sprintf("reconcilation finished in %s", time.Now().Sub(start).String())
	if result.RequeueAfter > 0 {
		durationMsg = fmt.Sprintf("%s, next run in %s", durationMsg, result.RequeueAfter.String())
	}
	log.Info(durationMsg)

	return result, err
}

func (r *HelmReleaseReconciler) reconcile(ctx context.Context, hr v2.HelmRelease) (v2.HelmRelease, ctrl.Result, error) {
	reconcileStart := time.Now()
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
		// Record progressing status
		r.recordReadiness(ctx, hr)
	}

	// Record reconciliation duration
	if r.MetricsRecorder != nil {
		objRef, err := reference.GetReference(r.Scheme, &hr)
		if err != nil {
			return hr, ctrl.Result{Requeue: true}, err
		}
		defer r.MetricsRecorder.RecordDuration(*objRef, reconcileStart)
	}

	// Reconcile chart based on the HelmChartTemplate
	hc, reconcileErr := r.reconcileChart(ctx, &hr)
	if reconcileErr != nil {
		if acl.IsAccessDenied(reconcileErr) {
			log.Error(reconcileErr, "access denied to cross-namespace source")
			r.event(ctx, hr, hr.Status.LastAttemptedRevision, eventv1.EventSeverityError, reconcileErr.Error())
			return v2.HelmReleaseNotReady(hr, apiacl.AccessDeniedReason, reconcileErr.Error()),
				ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
		}

		msg := fmt.Sprintf("chart reconciliation failed: %s", reconcileErr.Error())
		r.event(ctx, hr, hr.Status.LastAttemptedRevision, eventv1.EventSeverityError, msg)
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, msg), ctrl.Result{Requeue: true}, reconcileErr
	}

	// Check chart readiness
	if hc.Generation != hc.Status.ObservedGeneration || !apimeta.IsStatusConditionTrue(hc.Status.Conditions, meta.ReadyCondition) {
		msg := fmt.Sprintf("HelmChart '%s/%s' is not ready", hc.GetNamespace(), hc.GetName())
		r.event(ctx, hr, hr.Status.LastAttemptedRevision, eventv1.EventSeverityInfo, msg)
		log.Info(msg)
		// Do not requeue immediately, when the artifact is created
		// the watcher should trigger a reconciliation.
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, msg), ctrl.Result{RequeueAfter: hc.Spec.Interval.Duration}, nil
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
	values, err := r.composeValues(ctx, hr)
	if err != nil {
		r.event(ctx, hr, hr.Status.LastAttemptedRevision, eventv1.EventSeverityError, err.Error())
		return v2.HelmReleaseNotReady(hr, v2.InitFailedReason, err.Error()), ctrl.Result{Requeue: true}, nil
	}

	// Load chart from artifact
	chart, err := r.loadHelmChart(hc)
	if err != nil {
		r.event(ctx, hr, hr.Status.LastAttemptedRevision, eventv1.EventSeverityError, err.Error())
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, err.Error()), ctrl.Result{Requeue: true}, nil
	}

	// Reconcile Helm release
	reconciledHr, reconcileErr := r.reconcileRelease(ctx, *hr.DeepCopy(), chart, values)
	if reconcileErr != nil {
		r.event(ctx, hr, hc.GetArtifact().Revision, eventv1.EventSeverityError,
			fmt.Sprintf("reconciliation failed: %s", reconcileErr.Error()))
	}
	return reconciledHr, ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, reconcileErr
}

type HelmReleaseReconcilerOptions struct {
	MaxConcurrentReconciles   int
	HTTPRetry                 int
	DependencyRequeueInterval time.Duration
	RateLimiter               ratelimiter.RateLimiter
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

	// Register the current release attempt.
	revision := chart.Metadata.Version
	releaseRevision := util.ReleaseRevision(rel)
	valuesChecksum := util.ValuesChecksum(values)
	hr, hasNewState := v2.HelmReleaseAttempted(hr, revision, releaseRevision, valuesChecksum)
	if hasNewState {
		hr = v2.HelmReleaseProgressing(hr)
		if updateStatusErr := r.patchStatus(ctx, &hr); updateStatusErr != nil {
			log.Error(updateStatusErr, "unable to update status after state update")
			return hr, updateStatusErr
		}
		// Record progressing status
		r.recordReadiness(ctx, hr)
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
		rel, err = run.Install(hr, chart, values)
		err = r.handleHelmActionResult(ctx, &hr, revision, err, deployAction.GetDescription(),
			v2.ReleasedCondition, v2.InstallSucceededReason, v2.InstallFailedReason)
	} else {
		r.event(ctx, hr, revision, eventv1.EventSeverityInfo, "Helm upgrade has started")
		deployAction = hr.Spec.GetUpgrade()
		rel, err = run.Upgrade(hr, chart, values)
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
				log.Info(fmt.Sprintf("skipping remediation, no new release revision created"))
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
	opts := []kube.ClientGetterOption{
		kube.WithClientOptions(r.ClientOpts),
		// When ServiceAccountName is empty, it will fall back to the configured default.
		// If this is not configured either, this option will result in a no-op.
		kube.WithImpersonate(hr.Spec.ServiceAccountName, hr.GetNamespace()),
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
		kubeConfig, err := kube.ConfigFromSecret(&secret, hr.Spec.KubeConfig.SecretRef.Key)
		if err != nil {
			return nil, err
		}
		opts = append(opts, kube.WithKubeConfig(kubeConfig, r.KubeConfigOpts))
	}
	return kube.BuildClientGetter(hr.GetReleaseNamespace(), opts...)
}

// composeValues attempts to resolve all v2beta1.ValuesReference resources
// and merges them as defined. Referenced resources are only retrieved once
// to ensure a single version is taken into account during the merge.
func (r *HelmReleaseReconciler) composeValues(ctx context.Context, hr v2.HelmRelease) (chartutil.Values, error) {
	result := chartutil.Values{}

	configMaps := make(map[string]*corev1.ConfigMap)
	secrets := make(map[string]*corev1.Secret)

	for _, v := range hr.Spec.ValuesFrom {
		namespacedName := types.NamespacedName{Namespace: hr.Namespace, Name: v.Name}
		var valuesData []byte

		switch v.Kind {
		case "ConfigMap":
			resource, ok := configMaps[namespacedName.String()]
			if !ok {
				// The resource may not exist, but we want to act on a single version
				// of the resource in case the values reference is marked as optional.
				configMaps[namespacedName.String()] = nil

				resource = &corev1.ConfigMap{}
				if err := r.Get(ctx, namespacedName, resource); err != nil {
					if apierrors.IsNotFound(err) {
						if v.Optional {
							(ctrl.LoggerFrom(ctx)).
								Info(fmt.Sprintf("could not find optional %s '%s'", v.Kind, namespacedName))
							continue
						}
						return nil, fmt.Errorf("could not find %s '%s'", v.Kind, namespacedName)
					}
					return nil, err
				}
				configMaps[namespacedName.String()] = resource
			}
			if resource == nil {
				if v.Optional {
					(ctrl.LoggerFrom(ctx)).Info(fmt.Sprintf("could not find optional %s '%s'", v.Kind, namespacedName))
					continue
				}
				return nil, fmt.Errorf("could not find %s '%s'", v.Kind, namespacedName)
			}
			if data, ok := resource.Data[v.GetValuesKey()]; !ok {
				return nil, fmt.Errorf("missing key '%s' in %s '%s'", v.GetValuesKey(), v.Kind, namespacedName)
			} else {
				valuesData = []byte(data)
			}
		case "Secret":
			resource, ok := secrets[namespacedName.String()]
			if !ok {
				// The resource may not exist, but we want to act on a single version
				// of the resource in case the values reference is marked as optional.
				secrets[namespacedName.String()] = nil

				resource = &corev1.Secret{}
				if err := r.Get(ctx, namespacedName, resource); err != nil {
					if apierrors.IsNotFound(err) {
						if v.Optional {
							(ctrl.LoggerFrom(ctx)).
								Info(fmt.Sprintf("could not find optional %s '%s'", v.Kind, namespacedName))
							continue
						}
						return nil, fmt.Errorf("could not find %s '%s'", v.Kind, namespacedName)
					}
					return nil, err
				}
				secrets[namespacedName.String()] = resource
			}
			if resource == nil {
				if v.Optional {
					(ctrl.LoggerFrom(ctx)).Info(fmt.Sprintf("could not find optional %s '%s'", v.Kind, namespacedName))
					continue
				}
				return nil, fmt.Errorf("could not find %s '%s'", v.Kind, namespacedName)
			}
			if data, ok := resource.Data[v.GetValuesKey()]; !ok {
				return nil, fmt.Errorf("missing key '%s' in %s '%s'", v.GetValuesKey(), v.Kind, namespacedName)
			} else {
				valuesData = data
			}
		default:
			return nil, fmt.Errorf("unsupported ValuesReference kind '%s'", v.Kind)
		}
		switch v.TargetPath {
		case "":
			values, err := chartutil.ReadValues(valuesData)
			if err != nil {
				return nil, fmt.Errorf("unable to read values from key '%s' in %s '%s': %w", v.GetValuesKey(), v.Kind, namespacedName, err)
			}
			result = transform.MergeMaps(result, values)
		default:
			// TODO(hidde): this is a bit of hack, as it mimics the way the option string is passed
			// 	to Helm from a CLI perspective. Given the parser is however not publicly accessible
			// 	while it contains all logic around parsing the target path, it is a fair trade-off.
			stringValuesData := string(valuesData)
			const singleQuote = "'"
			const doubleQuote = "\""
			var err error
			if (strings.HasPrefix(stringValuesData, singleQuote) && strings.HasSuffix(stringValuesData, singleQuote)) || (strings.HasPrefix(stringValuesData, doubleQuote) && strings.HasSuffix(stringValuesData, doubleQuote)) {
				stringValuesData = strings.Trim(stringValuesData, singleQuote+doubleQuote)
				singleValue := v.TargetPath + "=" + stringValuesData
				err = strvals.ParseIntoString(singleValue, result)
			} else {
				singleValue := v.TargetPath + "=" + stringValuesData
				err = strvals.ParseInto(singleValue, result)
			}
			if err != nil {
				return nil, fmt.Errorf("unable to merge value from key '%s' in %s '%s' into target path '%s': %w", v.GetValuesKey(), v.Kind, namespacedName, v.TargetPath, err)
			}
		}
	}
	return transform.MergeMaps(result, hr.GetValues()), nil
}

// reconcileDelete deletes the v1beta2.HelmChart of the v2beta1.HelmRelease,
// and uninstalls the Helm release if the resource has not been suspended.
func (r *HelmReleaseReconciler) reconcileDelete(ctx context.Context, hr v2.HelmRelease) (ctrl.Result, error) {
	r.recordReadiness(ctx, hr)

	// Delete the HelmChart that belongs to this resource.
	if err := r.deleteHelmChart(ctx, &hr); err != nil {
		return ctrl.Result{}, err
	}

	// Only uninstall the Helm Release if the resource is not suspended.
	if !hr.Spec.Suspend {
		getter, err := r.buildRESTClientGetter(ctx, hr)
		if err != nil {
			return ctrl.Result{}, err
		}
		run, err := runner.NewRunner(getter, hr.GetStorageNamespace(), ctrl.LoggerFrom(ctx))
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := run.Uninstall(hr); err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
			return ctrl.Result{}, err
		}
		ctrl.LoggerFrom(ctx).Info("uninstalled Helm release for deleted resource")

	} else {
		ctrl.LoggerFrom(ctx).Info("skipping Helm uninstall for suspended resource")
	}

	// Remove our finalizer from the list and update it.
	controllerutil.RemoveFinalizer(&hr, v2.HelmReleaseFinalizer)
	if err := r.Update(ctx, &hr); err != nil {
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
			msg = msg + "\n\nLast Helm logs:\n\n" + actionErr.CapturedLogs
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
	key := client.ObjectKeyFromObject(hr)
	latest := &v2.HelmRelease{}
	if err := r.Client.Get(ctx, key, latest); err != nil {
		return err
	}
	return r.Client.Status().Patch(ctx, hr, client.MergeFrom(latest))
}

func (r *HelmReleaseReconciler) requestsForHelmChartChange(o client.Object) []reconcile.Request {
	hc, ok := o.(*sourcev1.HelmChart)
	if !ok {
		panic(fmt.Sprintf("Expected a HelmChart, got %T", o))
	}
	// If we do not have an artifact, we have no requests to make
	if hc.GetArtifact() == nil {
		return nil
	}

	ctx := context.Background()
	var list v2.HelmReleaseList
	if err := r.List(ctx, &list, client.MatchingFields{
		v2.SourceIndexKey: client.ObjectKeyFromObject(hc).String(),
	}); err != nil {
		return nil
	}

	var reqs []reconcile.Request
	for _, i := range list.Items {
		// If the revision of the artifact equals to the last attempted revision,
		// we should not make a request for this HelmRelease
		if hc.GetArtifact().Revision == i.Status.LastAttemptedRevision {
			continue
		}
		reqs = append(reqs, reconcile.Request{NamespacedName: client.ObjectKeyFromObject(&i)})
	}
	return reqs
}

// event emits a Kubernetes event and forwards the event to notification controller if configured.
func (r *HelmReleaseReconciler) event(_ context.Context, hr v2.HelmRelease, revision, severity, msg string) {
	var meta map[string]string
	if revision != "" {
		meta = map[string]string{v2.GroupVersion.Group + "/revision": revision}
	}
	eventtype := "Normal"
	if severity == eventv1.EventSeverityError {
		eventtype = "Warning"
	}
	r.EventRecorder.AnnotatedEventf(&hr, meta, eventtype, severity, msg)
}

func (r *HelmReleaseReconciler) recordSuspension(ctx context.Context, hr v2.HelmRelease) {
	if r.MetricsRecorder == nil {
		return
	}
	log := ctrl.LoggerFrom(ctx)

	objRef, err := reference.GetReference(r.Scheme, &hr)
	if err != nil {
		log.Error(err, "unable to record suspended metric")
		return
	}

	if !hr.DeletionTimestamp.IsZero() {
		r.MetricsRecorder.RecordSuspend(*objRef, false)
	} else {
		r.MetricsRecorder.RecordSuspend(*objRef, hr.Spec.Suspend)
	}
}

func (r *HelmReleaseReconciler) recordReadiness(ctx context.Context, hr v2.HelmRelease) {
	if r.MetricsRecorder == nil {
		return
	}

	objRef, err := reference.GetReference(r.Scheme, &hr)
	if err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "unable to record readiness metric")
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
