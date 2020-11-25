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
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
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

// ConditionError represents an error with a status condition reason attached.
type ConditionError struct {
	Reason string
	Err    error
}

func (c ConditionError) Error() string {
	return c.Err.Error()
}

func (r *HelmReleaseReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	ctx := context.Background()
	start := time.Now()

	var hr v2.HelmRelease
	if err := r.Get(ctx, req.NamespacedName, &hr); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	log := r.Log.WithValues("controller", strings.ToLower(v2.HelmReleaseKind), "request", req.NamespacedName)

	// Examine if the object is under deletion
	if hr.ObjectMeta.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !containsString(hr.ObjectMeta.Finalizers, v2.HelmReleaseFinalizer) {
			hr.ObjectMeta.Finalizers = append(hr.ObjectMeta.Finalizers, v2.HelmReleaseFinalizer)
			if err := r.Update(ctx, &hr); err != nil {
				log.Error(err, "unable to register finalizer")
				return ctrl.Result{}, err
			}
		}
	} else {
		// The object is being deleted
		if containsString(hr.ObjectMeta.Finalizers, v2.HelmReleaseFinalizer) {
			// Our finalizer is still present, so lets handle garbage collection
			if err := r.garbageCollect(ctx, log, hr); err != nil {
				r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, err.Error())
				// Return the error so we retry the failed garbage collection
				return ctrl.Result{}, err
			}
			// Record deleted status
			r.recordReadiness(hr, true)
			// Remove our finalizer from the list and update it
			hr.ObjectMeta.Finalizers = removeString(hr.ObjectMeta.Finalizers, v2.HelmReleaseFinalizer)
			if err := r.Update(ctx, &hr); err != nil {
				return ctrl.Result{}, err
			}
			r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityInfo, "Helm uninstall for deleted resource succeeded")
		}
		// Stop reconciliation as the object is being deleted
		return ctrl.Result{}, nil
	}

	hr, result, err := r.reconcile(ctx, log, hr)

	// Update status after reconciliation.
	if updateStatusErr := r.patchStatus(ctx, &hr); updateStatusErr != nil {
		log.Error(updateStatusErr, "unable to update status after reconciliation")
		return ctrl.Result{Requeue: true}, updateStatusErr
	}

	// Record ready status
	r.recordReadiness(hr, false)

	// Log reconciliation duration
	durationMsg := fmt.Sprintf("reconcilation finished in %s", time.Now().Sub(start).String())
	if result.RequeueAfter > 0 {
		durationMsg = fmt.Sprintf("%s, next run in %s", durationMsg, result.RequeueAfter.String())
	}
	log.Info(durationMsg)

	return result, err
}

func (r *HelmReleaseReconciler) reconcile(ctx context.Context, log logr.Logger, hr v2.HelmRelease) (v2.HelmRelease, ctrl.Result, error) {
	reconcileStart := time.Now()
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
		r.recordReadiness(hr, false)
	}

	if hr.Spec.Suspend {
		msg := "HelmRelease is suspended, skipping reconciliation"
		log.Info(msg)
		return v2.HelmReleaseNotReady(hr, meta.SuspendedReason, msg), ctrl.Result{}, nil
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
		msg := fmt.Sprintf("chart reconciliation failed: %s", reconcileErr.Error())
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, msg)
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, msg), ctrl.Result{Requeue: true}, reconcileErr
	}

	// Check chart readiness
	if hc.Generation != hc.Status.ObservedGeneration || !apimeta.IsStatusConditionTrue(hc.Status.Conditions, meta.ReadyCondition) {
		msg := fmt.Sprintf("HelmChart '%s/%s' is not ready", hc.GetNamespace(), hc.GetName())
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityInfo, msg)
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
			r.event(hr, hc.GetArtifact().Revision, events.EventSeverityInfo, msg)
			log.Info(msg)

			// Exponential backoff would cause execution to be prolonged too much,
			// instead we requeue on a fixed interval.
			return v2.HelmReleaseNotReady(hr,
				meta.DependencyNotReadyReason, err.Error()), ctrl.Result{RequeueAfter: r.requeueDependency}, nil
		}
		log.Info("all dependencies are ready, proceeding with release")
	}

	// Compose values
	values, err := r.composeValues(ctx, hr)
	if err != nil {
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, err.Error())
		return v2.HelmReleaseNotReady(hr, v2.InitFailedReason, err.Error()), ctrl.Result{Requeue: true}, nil
	}

	// Load chart from artifact
	chart, err := r.loadHelmChart(hc)
	if err != nil {
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, err.Error())
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, err.Error()), ctrl.Result{Requeue: true}, nil
	}

	// Reconcile Helm release
	reconciledHr, reconcileErr := r.reconcileRelease(ctx, log, *hr.DeepCopy(), chart, values)
	if reconcileErr != nil {
		r.event(hr, hc.GetArtifact().Revision, events.EventSeverityError,
			fmt.Sprintf("reconciliation failed: %s", reconcileErr.Error()))
	}
	return reconciledHr, ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, reconcileErr
}

type HelmReleaseReconcilerOptions struct {
	MaxConcurrentReconciles   int
	DependencyRequeueInterval time.Duration
}

func (r *HelmReleaseReconciler) reconcileRelease(ctx context.Context, log logr.Logger,
	hr v2.HelmRelease, chart *chart.Chart, values chartutil.Values) (v2.HelmRelease, error) {
	// Initialize Helm action runner
	getter, err := r.getRESTClientGetter(ctx, hr)
	if err != nil {
		return v2.HelmReleaseNotReady(hr, v2.InitFailedReason, err.Error()), err
	}
	run, err := runner.NewRunner(getter, hr.GetNamespace(), r.Log)
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
		r.recordReadiness(hr, false)
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
			return v2.HelmReleaseNotReady(hr, released.Reason, released.Message), err
		}

		// Fail if there is a release and upgrade retries are exhausted.
		// This avoids failing after an upgrade uninstall remediation strategy.
		if rel != nil && hr.Spec.GetUpgrade().GetRemediation().RetriesExhausted(hr) {
			err = fmt.Errorf("upgrade retries exhausted")
			return v2.HelmReleaseNotReady(hr, released.Reason, released.Message), err
		}
	}

	// Deploy the release.
	var deployAction v2.DeploymentAction
	if rel == nil {
		deployAction = hr.Spec.GetInstall()
		rel, err = run.Install(hr, chart, values)
		err = r.handleHelmActionResult(&hr, revision, err, deployAction.GetDescription(), v2.ReleasedCondition, v2.InstallSucceededReason, v2.InstallFailedReason)
	} else {
		deployAction = hr.Spec.GetUpgrade()
		rel, err = run.Upgrade(hr, chart, values)
		err = r.handleHelmActionResult(&hr, revision, err, deployAction.GetDescription(), v2.ReleasedCondition, v2.UpgradeSucceededReason, v2.UpgradeFailedReason)
	}
	remediation := deployAction.GetRemediation()

	// If there is a new release revision...
	if util.ReleaseRevision(rel) > releaseRevision {
		// Ensure release is not marked remediated.
		apimeta.RemoveStatusCondition(&hr.Status.Conditions, v2.RemediatedCondition)

		// If new release revision is successful and tests are enabled, run them.
		if err == nil && hr.Spec.GetTest().Enable {
			_, testErr := run.Test(hr)
			testErr = r.handleHelmActionResult(&hr, revision, testErr, "test", v2.TestSuccessCondition, v2.TestSucceededReason, v2.TestFailedReason)

			// Propagate any test error if not marked ignored.
			if testErr != nil && !remediation.MustIgnoreTestFailures(hr.Spec.GetTest().IgnoreFailures) {
				testsPassing := apimeta.FindStatusCondition(hr.Status.Conditions, v2.TestSuccessCondition)
				meta.SetResourceCondition(&hr, v2.ReleasedCondition, metav1.ConditionFalse, testsPassing.Reason, testsPassing.Message)
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
					remediationErr = r.handleHelmActionResult(&hr, revision, rollbackErr, "rollback",
						v2.RemediatedCondition, v2.RollbackSucceededReason, v2.RollbackFailedReason)
				case v2.UninstallRemediationStrategy:
					uninstallErr := run.Uninstall(hr)
					remediationErr = r.handleHelmActionResult(&hr, revision, uninstallErr, "uninstall",
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
		reason := meta.ReconciliationFailedReason
		var cerr *ConditionError
		if errors.As(err, &cerr) {
			reason = cerr.Reason
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
		dName := types.NamespacedName(d)
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
							r.Log.Info("could not find optional %s '%s'", v.Kind, namespacedName)
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
					r.Log.Info("could not find optional %s '%s'", v.Kind, namespacedName)
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
							r.Log.Info("could not find optional %s '%s'", v.Kind, namespacedName)
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
					r.Log.Info("could not find optional %s '%s'", v.Kind, namespacedName)
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
			result = util.MergeMaps(result, values)
		default:
			// TODO(hidde): this is a bit of hack, as it mimics the way the option string is passed
			// 	to Helm from a CLI perspective. Given the parser is however not publicly accessible
			// 	while it contains all logic around parsing the target path, it is a fair trade-off.
			singleValue := v.TargetPath + "=" + string(valuesData)
			if err := strvals.ParseInto(singleValue, result); err != nil {
				return nil, fmt.Errorf("unable to merge value from key '%s' in %s '%s' into target path '%s': %w", v.GetValuesKey(), v.Kind, namespacedName, v.TargetPath, err)
			}
		}
	}
	return util.MergeMaps(result, hr.GetValues()), nil
}

// garbageCollect deletes the v1beta1.HelmChart of the v2beta1.HelmRelease,
// and uninstalls the Helm release if the resource has not been suspended.
func (r *HelmReleaseReconciler) garbageCollect(ctx context.Context, logger logr.Logger, hr v2.HelmRelease) error {
	if err := r.deleteHelmChart(ctx, &hr); err != nil {
		return err
	}
	if hr.Spec.Suspend {
		logger.Info("skipping garbage collection for suspended resource")
		return nil
	}
	return r.garbageCollectHelmRelease(logger, hr)
}

// garbageCollectHelmRelease uninstalls the deployed Helm release of
// the given v2beta1.HelmRelease.
func (r *HelmReleaseReconciler) garbageCollectHelmRelease(logger logr.Logger, hr v2.HelmRelease) error {
	getter, err := r.getRESTClientGetter(context.TODO(), hr)
	if err != nil {
		return err
	}
	run, err := runner.NewRunner(getter, hr.GetNamespace(), logger)
	if err != nil {
		return err
	}
	rel, err := run.ObserveLastRelease(hr)
	if err != nil {
		err = fmt.Errorf("failed to garbage collect resource: %w", err)
		return err
	}
	if rel == nil {
		return nil
	}
	err = run.Uninstall(hr)
	if err != nil {
		err = fmt.Errorf("failed to garbage collect resource: %w", err)
	}
	return err
}

func (r *HelmReleaseReconciler) handleHelmActionResult(hr *v2.HelmRelease, revision string, err error, action string, condition string, succeededReason string, failedReason string) error {
	if err != nil {
		msg := fmt.Sprintf("Helm %s failed: %s", action, err.Error())
		meta.SetResourceCondition(hr, condition, metav1.ConditionFalse, failedReason, msg)
		r.event(*hr, revision, events.EventSeverityError, msg)
		return &ConditionError{Reason: failedReason, Err: errors.New(msg)}
	} else {
		msg := fmt.Sprintf("Helm %s succeeded", action)
		meta.SetResourceCondition(hr, condition, metav1.ConditionTrue, succeededReason, msg)
		r.event(*hr, revision, events.EventSeverityInfo, msg)
		return nil
	}
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

func (r *HelmReleaseReconciler) requestsForHelmChartChange(obj handler.MapObject) []reconcile.Request {
	hc, ok := obj.Object.(*sourcev1.HelmChart)
	if !ok {
		panic(fmt.Sprintf("Expected a HelmChart, got %T", hc))
	}
	// If we do not have an artifact, we have no requests to make
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
		// we should not make a request for this HelmRelease
		if hc.GetArtifact().Revision == i.Status.LastAttemptedRevision {
			continue
		}
		req := reconcile.Request{NamespacedName: types.NamespacedName{Namespace: i.GetNamespace(), Name: i.GetName()}}
		reqs = append(reqs, req)
		r.Log.Info("requesting reconciliation due to GitRepository revision change",
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
		var meta map[string]string
		if revision != "" {
			meta = map[string]string{"revision": revision}
		}
		if err := r.ExternalEventRecorder.Eventf(*objRef, meta, severity, severity, msg); err != nil {
			r.Log.WithValues(
				"request",
				fmt.Sprintf("%s/%s", hr.GetNamespace(), hr.GetName()),
			).Error(err, "unable to send event")
			return
		}
	}
}

func (r *HelmReleaseReconciler) recordReadiness(hr v2.HelmRelease, deleted bool) {
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
		r.MetricsRecorder.RecordCondition(*objRef, *rc, deleted)
	} else {
		r.MetricsRecorder.RecordCondition(*objRef, metav1.Condition{
			Type:   meta.ReadyCondition,
			Status: metav1.ConditionUnknown,
		}, deleted)
	}
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) (result []string) {
	for _, item := range slice {
		if item == s {
			continue
		}
		result = append(result, item)
	}
	return
}
