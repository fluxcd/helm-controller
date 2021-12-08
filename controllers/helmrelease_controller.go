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
	"github.com/hashicorp/go-retryablehttp"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/strvals"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	kerrors "k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	helper "github.com/fluxcd/pkg/runtime/controller"
	"github.com/fluxcd/pkg/runtime/events"
	"github.com/fluxcd/pkg/runtime/patch"
	"github.com/fluxcd/pkg/runtime/predicates"
	"github.com/fluxcd/pkg/runtime/transform"
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
	helper.Events
	helper.Metrics

	Config            *rest.Config
	httpClient        *retryablehttp.Client
	requeueDependency time.Duration
}

type HelmReleaseReconcilerOptions struct {
	MaxConcurrentReconciles   int
	HTTPRetry                 int
	DependencyRequeueInterval time.Duration
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

	return ctrl.NewControllerManagedBy(mgr).
		For(&v2.HelmRelease{}, builder.WithPredicates(
			predicate.Or(predicate.GenerationChangedPredicate{}, predicates.ReconcileRequestedPredicate{}),
		)).
		Watches(
			&source.Kind{Type: &sourcev1.HelmChart{}},
			handler.EnqueueRequestsFromMapFunc(r.requestsForHelmChartChange),
			builder.WithPredicates(SourceRevisionChangePredicate{}),
		).
		WithOptions(controller.Options{MaxConcurrentReconciles: opts.MaxConcurrentReconciles}).
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

func (r *HelmReleaseReconciler) Reconcile(ctx context.Context, req ctrl.Request) (result ctrl.Result, retErr error) {
	start := time.Now()
	log := logr.FromContext(ctx)

	obj := &v2.HelmRelease{}
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	r.RecordSuspend(ctx, obj, obj.Spec.Suspend)

	// Return early if the HelmRelease is suspended.
	if obj.Spec.Suspend {
		log.Info("Reconciliation is suspended for this object")
		return ctrl.Result{}, nil
	}

	// Initialize the patch helper
	patchHelper, err := patch.NewHelper(obj, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always attempt to patch the object and status after each reconciliation
	defer func() {
		// Record the value of the reconciliation request, if any
		if v, ok := meta.ReconcileAnnotationValue(obj.GetAnnotations()); ok {
			obj.Status.SetLastHandledReconcileRequest(v)
		}

		// TODO: Handle test condition when tests are enabled
		// Summarize Ready condition
		conditions.SetSummary(obj,
			meta.ReadyCondition,
			conditions.WithConditions(
				v2.ReleasedCondition,
				//v2.TestSuccessCondition,
				v2.RemediatedCondition,
				v2.InitFailedCondition,
				v2.RetriesExhaustedCondition,
			),
			conditions.WithNegativePolarityConditions(
				v2.RemediatedCondition,
				v2.InitFailedCondition,
				v2.RetriesExhaustedCondition,
			),
		)

		// Patch the object, ignoring conflicts on the conditions owned by this controller
		patchOpts := []patch.Option{
			patch.WithOwnedConditions{
				Conditions: []string{
					v2.ReleasedCondition,
					v2.TestSuccessCondition,
					v2.RemediatedCondition,
					v2.InitFailedCondition,
					v2.RetriesExhaustedCondition,
					meta.ReadyCondition,
					meta.ReconcilingCondition,
					meta.StalledCondition,
				},
			},
		}

		// Determine if the resource is still being reconciled, or if it has stalled, and record this observation
		if retErr == nil && (result.IsZero() || !result.Requeue) {
			// We are no longer reconciling
			conditions.Delete(obj, meta.ReconcilingCondition)

			// We have now observed this generation
			patchOpts = append(patchOpts, patch.WithStatusObservedGeneration{})

			readyCondition := conditions.Get(obj, meta.ReadyCondition)
			switch readyCondition.Status {
			case metav1.ConditionFalse:
				// As we are no longer reconciling and the end-state
				// is not ready, the reconciliation has stalled
				conditions.MarkStalled(obj, readyCondition.Reason, readyCondition.Message)
			case metav1.ConditionTrue:
				// As we are no longer reconciling and the end-state
				// is ready, the reconciliation is no longer stalled
				conditions.Delete(obj, meta.StalledCondition)
			}
		}

		// Finally, patch the resource
		if err := patchHelper.Patch(ctx, obj, patchOpts...); err != nil {
			retErr = kerrors.NewAggregate([]error{retErr, err})
		}

		// Always record readiness and duration metrics
		r.Metrics.RecordReadiness(ctx, obj)
		r.Metrics.RecordDuration(ctx, obj, start)
	}()

	// Add finalizer first if not exit to avoid the race condition
	// between init and delete
	if !controllerutil.ContainsFinalizer(obj, v2.HelmReleaseFinalizer) {
		controllerutil.AddFinalizer(obj, v2.HelmReleaseFinalizer)
		return ctrl.Result{Requeue: true}, nil
	}

	// Examine if the object is under deletion
	if !obj.ObjectMeta.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, obj)
	}

	// Reconcile actual object
	return r.reconcile(ctx, obj)
}

func (r *HelmReleaseReconciler) reconcile(ctx context.Context, obj *v2.HelmRelease) (ctrl.Result, error) {
	log := logr.FromContext(ctx)

	// Mark the resource as under reconciliation
	conditions.MarkReconciling(obj, meta.ProgressingReason, "")

	// Remove InitFailedCondition
	conditions.Delete(obj, v2.InitFailedCondition)

	// Reconcile chart based on the HelmChartTemplate
	hc, reconcileErr := r.reconcileChart(ctx, obj)
	if reconcileErr != nil {
		obj.IncrementFailureCounter()
		conditions.MarkTrue(obj, v2.InitFailedCondition, v2.ArtifactFailedReason, "Chart reconcilliation failed: %s", reconcileErr.Error())
		r.Eventf(ctx, obj, events.EventSeverityError, v2.ArtifactFailedReason, "Chart reconcilliation failed: %s", reconcileErr.Error())
		return ctrl.Result{Requeue: true}, reconcileErr
	}

	if !conditions.IsReady(hc) {
		msg := fmt.Sprintf("HelmChart '%s/%s' is not ready", hc.GetNamespace(), hc.GetName())
		obj.IncrementFailureCounter()
		conditions.MarkTrue(obj, v2.InitFailedCondition, v2.ArtifactFailedReason, msg)
		r.Eventf(ctx, obj, events.EventSeverityError, v2.ArtifactFailedReason, msg)
		log.Info(msg)
		// Do not requeue immediately, when the artifact is created
		// the watcher should trigger a reconciliation.
		return ctrl.Result{RequeueAfter: hc.Spec.Interval.Duration}, nil
	}

	// Check dependencies
	if len(obj.Spec.DependsOn) > 0 {
		if err := r.checkDependencies(ctx, obj); err != nil {
			msg := fmt.Sprintf("dependencies do not meet ready condition (%s), retrying in %s", err.Error(), r.requeueDependency.String())
			obj.IncrementFailureCounter()
			conditions.MarkTrue(obj, v2.InitFailedCondition, v2.DependencyNotReadyReason, msg)
			r.Eventf(ctx, obj, events.EventSeverityError, v2.DependencyNotReadyReason, msg)
			log.Info(msg)
			// Exponential backoff would cause execution to be prolonged too much,
			// instead we requeue on a fixed interval.
			return ctrl.Result{RequeueAfter: r.requeueDependency}, nil
		}
		log.Info("all dependencies are ready, proceeding with release")
	}

	// Compose values
	values, err := r.composeValues(ctx, obj)
	if err != nil {
		obj.IncrementFailureCounter()
		conditions.MarkTrue(obj, v2.InitFailedCondition, v2.InitFailedReason, "could not get chart values: %s", err.Error())
		r.Eventf(ctx, obj, events.EventSeverityError, v2.InitFailedReason, "could not get chart values: %s", err.Error())
		return ctrl.Result{Requeue: true}, nil
	}

	// Load chart from artifact
	chart, err := r.loadHelmChart(hc)
	if err != nil {
		obj.IncrementFailureCounter()
		conditions.MarkTrue(obj, v2.InitFailedCondition, v2.ArtifactFailedReason, "could not load chart: %s", err.Error())
		r.Eventf(ctx, obj, events.EventSeverityError, v2.ArtifactFailedReason, "could not load chart: %s", err.Error())
		return ctrl.Result{Requeue: true}, nil
	}

	// Reconcile Helm release
	if result, err := r.reconcileRelease(ctx, obj, chart, values); err != nil {
		obj.IncrementFailureCounter()
		// TODO: Set a better reason
		r.Eventf(ctx, obj, events.EventSeverityError, v2.ArtifactFailedReason, "reconciliation failed: %s", err.Error())
		return result, err
	}
	return ctrl.Result{RequeueAfter: obj.Spec.Interval.Duration}, nil
}

func (r *HelmReleaseReconciler) reconcileRelease(ctx context.Context, hr *v2.HelmRelease, chart *chart.Chart, values chartutil.Values) (ctrl.Result, error) {
	log := logr.FromContext(ctx)

	// Initialize Helm action runner
	getter, err := r.getRESTClientGetter(ctx, hr)
	if err != nil {
		hr.IncrementFailureCounter()
		conditions.MarkTrue(hr, v2.InitFailedCondition, v2.InitFailedReason, err.Error())
		r.Eventf(ctx, hr, events.EventSeverityError, v2.InitFailedReason, err.Error())
		return ctrl.Result{}, err
	}
	run, err := runner.NewRunner(getter, hr.GetStorageNamespace(), log)
	if err != nil {
		hr.IncrementFailureCounter()
		conditions.MarkTrue(hr, v2.InitFailedCondition, v2.InitFailedReason, "failed to initialize Helm action runner: %s", err)
		r.Eventf(ctx, hr, events.EventSeverityError, v2.InitFailedReason, "failed to initialize Helm action runner: %s", err)
		return ctrl.Result{}, err
	}

	// Determine last release revision.
	rel, observeLastReleaseErr := run.ObserveLastRelease(*hr)
	if observeLastReleaseErr != nil {
		hr.IncrementFailureCounter()
		err = fmt.Errorf("failed to get last release revision: %w", observeLastReleaseErr)
		conditions.MarkTrue(hr, v2.InitFailedCondition, v2.GetLastReleaseFailedReason, err.Error())
		r.Eventf(ctx, hr, events.EventSeverityError, v2.GetLastReleaseFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// Register the current release attempt.
	revision := chart.Metadata.Version
	releaseRevision := util.ReleaseRevision(rel)
	valuesChecksum := util.ValuesChecksum(values)
	hr, hasNewState := v2.HelmReleaseAttempted(hr, revision, releaseRevision, valuesChecksum)
	if hasNewState {
		conditions.MarkUnknown(hr, v2.ReleasedCondition, meta.ProgressingReason, "Reconciliation in progress")
		hr.ResetFailureCounter()
	}

	// Check status of any previous release attempt.
	released := conditions.Get(hr, v2.ReleasedCondition)
	if released != nil {
		switch released.Status {
		// Succeed if the previous release attempt succeeded.
		case metav1.ConditionTrue:
			return ctrl.Result{}, nil
		case metav1.ConditionFalse:
			// Fail if the previous release attempt remediation failed.
			remediated := conditions.Get(hr, v2.RemediatedCondition)
			if remediated != nil && remediated.Status == metav1.ConditionFalse {
				err = fmt.Errorf("previous release attempt remediation failed")
				return ctrl.Result{}, err
			}
		}

		// Fail if install retries are exhausted.
		if hr.Spec.GetInstall().GetRemediation().RetriesExhausted(hr) {
			hr.IncrementFailureCounter()
			err = fmt.Errorf("install retries exhausted")
			conditions.MarkTrue(hr, v2.RetriesExhaustedCondition, released.Reason, err.Error())
			r.Eventf(ctx, hr, events.EventSeverityError, released.Reason, err.Error())
			return ctrl.Result{}, err
		}

		// Fail if there is a release and upgrade retries are exhausted.
		// This avoids failing after an upgrade uninstall remediation strategy.
		if rel != nil && hr.Spec.GetUpgrade().GetRemediation().RetriesExhausted(hr) {
			hr.IncrementFailureCounter()
			err = fmt.Errorf("upgrade retries exhausted")
			conditions.MarkTrue(hr, v2.RetriesExhaustedCondition, released.Reason, err.Error())
			r.Eventf(ctx, hr, events.EventSeverityError, released.Reason, err.Error())
			return ctrl.Result{}, err
		}
	}
	conditions.Delete(hr, v2.RetriesExhaustedCondition)

	// Deploy the release.
	var deployAction v2.DeploymentAction
	if rel == nil {
		r.Event(ctx, hr, events.EventSeverityInfo, "Helm install has started", revision)
		deployAction = hr.Spec.GetInstall()
		rel, err = run.Install(*hr, chart, values)
		err = r.handleHelmActionResult(ctx, hr, revision, err, deployAction.GetDescription(),
			v2.ReleasedCondition, v2.InstallSucceededReason, v2.InstallFailedReason)
	} else {
		r.Event(ctx, hr, events.EventSeverityInfo, "Helm upgrade has started", revision)
		deployAction = hr.Spec.GetUpgrade()
		rel, err = run.Upgrade(*hr, chart, values)
		err = r.handleHelmActionResult(ctx, hr, revision, err, deployAction.GetDescription(),
			v2.ReleasedCondition, v2.UpgradeSucceededReason, v2.UpgradeFailedReason)
	}
	remediation := deployAction.GetRemediation()

	// If there is a new release revision...
	if util.ReleaseRevision(rel) > releaseRevision {
		// Ensure release is not marked remediated.
		conditions.Delete(hr, v2.RemediatedCondition)

		// If new release revision is successful and tests are enabled, run them.
		if err == nil && hr.Spec.GetTest().Enable {
			_, testErr := run.Test(*hr)
			testErr = r.handleHelmActionResult(ctx, hr, revision, testErr, "test",
				v2.TestSuccessCondition, v2.TestSucceededReason, v2.TestFailedReason)

			// Propagate any test error if not marked ignored.
			if testErr != nil && !remediation.MustIgnoreTestFailures(hr.Spec.GetTest().IgnoreFailures) {
				testsPassing := conditions.Get(hr, v2.TestSuccessCondition)
				if testsPassing != nil {
					hr.IncrementFailureCounter()
					conditions.MarkFalse(hr, v2.ReleasedCondition, testsPassing.Reason, testsPassing.Message)
					err = testErr
				}
			}
		}
	}

	if err != nil {
		// Increment failure count for deployment action.
		remediation.IncrementFailureCount(hr)
		// Remediate deployment failure if necessary.
		if !remediation.RetriesExhausted(hr) || remediation.MustRemediateLastFailure() {
			if util.ReleaseRevision(rel) <= releaseRevision {
				log.Info("skipping remediation, no new release revision created")
			} else {
				var remediationErr error
				switch remediation.GetStrategy() {
				case v2.RollbackRemediationStrategy:
					rollbackErr := run.Rollback(*hr)
					remediationErr = r.handleHelmActionResult(ctx, hr, revision, rollbackErr, "rollback",
						v2.RemediatedCondition, v2.RollbackSucceededReason, v2.RollbackFailedReason)
				case v2.UninstallRemediationStrategy:
					uninstallErr := run.Uninstall(*hr)
					remediationErr = r.handleHelmActionResult(ctx, hr, revision, uninstallErr, "uninstall",
						v2.RemediatedCondition, v2.UninstallSucceededReason, v2.UninstallFailedReason)
				}
				if remediationErr != nil {
					err = remediationErr
				}
			}

			// Determine release after remediation.
			rel, observeLastReleaseErr = run.ObserveLastRelease(*hr)
			if observeLastReleaseErr != nil {
				err = &ConditionError{
					Reason: v2.GetLastReleaseFailedReason,
					Err:    errors.New("failed to get last release revision after remediation"),
				}
			}
		}
	}

	hr.Status.LastReleaseRevision = util.ReleaseRevision(rel)

	// TODO: This should be replaced as part of issue #324
	if err != nil {
		// TODO: Is this the correct reason?
		reason := v2.InstallFailedReason
		if condErr := (*ConditionError)(nil); errors.As(err, &condErr) {
			reason = condErr.Reason
		}
		hr.IncrementFailureCounter()
		conditions.MarkFalse(hr, v2.ReleasedCondition, reason, err.Error())
		return ctrl.Result{}, err
	}

	hr.ResetFailureCounter()
	return ctrl.Result{}, nil
}

func (r *HelmReleaseReconciler) checkDependencies(ctx context.Context, hr *v2.HelmRelease) error {
	for _, d := range hr.Spec.DependsOn {
		if d.Namespace == "" {
			d.Namespace = hr.GetNamespace()
		}
		dName := types.NamespacedName{Namespace: d.Namespace, Name: d.Name}
		var dHr v2.HelmRelease
		err := r.Get(ctx, dName, &dHr)
		if err != nil {
			return fmt.Errorf("unable to get '%s' dependency: %w", dName, err)
		}
		if len(dHr.Status.Conditions) == 0 || dHr.Generation != dHr.Status.ObservedGeneration {
			return fmt.Errorf("dependency '%s' is not ready", dName)
		}
		if !conditions.IsReady(&dHr) {
			return fmt.Errorf("dependency '%s' is not ready", dName)
		}
	}
	return nil
}

func (r *HelmReleaseReconciler) getRESTClientGetter(ctx context.Context, hr *v2.HelmRelease) (genericclioptions.RESTClientGetter, error) {
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

	var kubeConfig []byte
	for k := range secret.Data {
		if k == "value" || k == "value.yaml" {
			kubeConfig = secret.Data[k]
			break
		}
	}

	if len(kubeConfig) == 0 {
		return nil, fmt.Errorf("KubeConfig secret '%s' does not contain a 'value' key", secretName)
	}
	return kube.NewMemoryRESTClientGetter(kubeConfig, hr.GetReleaseNamespace()), nil
}

func (r *HelmReleaseReconciler) getServiceAccountToken(ctx context.Context, hr *v2.HelmRelease) (string, error) {
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

// composeValues attempts to resolve all v2beta1.ValuesReference resources
// and merges them as defined. Referenced resources are only retrieved once
// to ensure a single version is taken into account during the merge.
func (r *HelmReleaseReconciler) composeValues(ctx context.Context, hr *v2.HelmRelease) (chartutil.Values, error) {
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
							(logr.FromContext(ctx)).
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
					(logr.FromContext(ctx)).Info(fmt.Sprintf("could not find optional %s '%s'", v.Kind, namespacedName))
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
							(logr.FromContext(ctx)).
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
					(logr.FromContext(ctx)).Info(fmt.Sprintf("could not find optional %s '%s'", v.Kind, namespacedName))
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

// reconcileDelete deletes the v1beta1.HelmChart of the v2beta1.HelmRelease,
// and uninstalls the Helm release if the resource has not been suspended.
func (r *HelmReleaseReconciler) reconcileDelete(ctx context.Context, hr *v2.HelmRelease) (ctrl.Result, error) {
	// Delete the HelmChart that belongs to this resource.
	if err := r.deleteHelmChart(ctx, hr); err != nil {
		return ctrl.Result{}, err
	}

	// Only uninstall the Helm Release if the resource is not suspended.
	if !hr.Spec.Suspend {
		getter, err := r.getRESTClientGetter(ctx, hr)
		if err != nil {
			return ctrl.Result{}, err
		}
		run, err := runner.NewRunner(getter, hr.GetStorageNamespace(), logr.FromContext(ctx))
		if err != nil {
			return ctrl.Result{}, err
		}
		if err := run.Uninstall(*hr); err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
			return ctrl.Result{}, err
		}
		logr.FromContext(ctx).Info("uninstalled Helm release for deleted resource")
	} else {
		logr.FromContext(ctx).Info("skipping Helm uninstall for suspended resource")
	}

	// Remove our finalizer from the list
	controllerutil.RemoveFinalizer(hr, v2.HelmReleaseFinalizer)
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
		hr.IncrementFailureCounter()
		conditions.MarkFalse(hr, condition, failedReason, msg)
		r.Eventf(ctx, hr, events.EventSeverityError, failedReason, msg)
		return &ConditionError{Reason: failedReason, Err: err}
	} else {
		conditions.MarkTrue(hr, condition, succeededReason, "Helm %s succeeded", action)
		r.Eventf(ctx, hr, events.EventSeverityInfo, succeededReason, "Helm %s succeeded", action)
		return nil
	}
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
