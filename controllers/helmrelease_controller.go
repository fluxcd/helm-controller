/*
Copyright 2020 The Flux CD contributors.

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
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/storage/driver"
	"helm.sh/helm/v3/pkg/strvals"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	kuberecorder "k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/fluxcd/pkg/recorder"
	consts "github.com/fluxcd/pkg/runtime"
	"github.com/fluxcd/pkg/runtime/predicates"
	sourcev1 "github.com/fluxcd/source-controller/api/v1alpha1"

	v2 "github.com/fluxcd/helm-controller/api/v2alpha1"
	"github.com/fluxcd/helm-controller/internal/runner"
	"github.com/fluxcd/helm-controller/internal/util"
)

// HelmReleaseReconciler reconciles a HelmRelease object
type HelmReleaseReconciler struct {
	client.Client
	Config                *rest.Config
	Log                   logr.Logger
	Scheme                *runtime.Scheme
	requeueDependency     time.Duration
	EventRecorder         kuberecorder.EventRecorder
	ExternalEventRecorder *recorder.EventRecorder
}

// ConditionError represents an error with a status condition reason attached.
type ConditionError struct {
	Reason string
	Err    error
}

func (c ConditionError) Error() string {
	return c.Err.Error()
}

// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases/status,verbs=get;update;patch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

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
				r.event(hr, hr.Status.LastAttemptedRevision, recorder.EventSeverityError, err.Error())
				// Return the error so we retry the failed garbage collection
				return ctrl.Result{}, err
			}

			// Remove our finalizer from the list and update it
			hr.ObjectMeta.Finalizers = removeString(hr.ObjectMeta.Finalizers, v2.HelmReleaseFinalizer)
			if err := r.Update(ctx, &hr); err != nil {
				return ctrl.Result{}, err
			}
			r.event(hr, hr.Status.LastAttemptedRevision, recorder.EventSeverityInfo, "Helm uninstall for deleted resource succeeded")
		}
		// Stop reconciliation as the object is being deleted
		return ctrl.Result{}, nil
	}

	hr, result, err := r.reconcile(ctx, log, hr)

	// Update status after reconciliation.
	if updateStatusErr := r.updateStatus(ctx, &hr); updateStatusErr != nil {
		log.Error(updateStatusErr, "unable to update status after reconciliation")
		return ctrl.Result{Requeue: true}, updateStatusErr
	}

	// Log reconciliation duration
	log.Info(fmt.Sprintf("reconcilation finished in %s, next run in %s",
		time.Now().Sub(start).String(),
		hr.Spec.Interval.Duration.String(),
	))

	return result, err
}

func (r *HelmReleaseReconciler) reconcile(ctx context.Context, log logr.Logger, hr v2.HelmRelease) (v2.HelmRelease, ctrl.Result, error) {
	// Observe HelmRelease generation.
	if hr.Status.ObservedGeneration != hr.Generation {
		hr.Status.ObservedGeneration = hr.Generation
		hr = v2.HelmReleaseProgressing(hr)
		if updateStatusErr := r.updateStatus(ctx, &hr); updateStatusErr != nil {
			log.Error(updateStatusErr, "unable to update status after generation update")
			return hr, ctrl.Result{Requeue: true}, updateStatusErr
		}
	}

	if hr.Spec.Suspend {
		msg := "HelmRelease is suspended, skipping reconciliation"
		log.Info(msg)
		return v2.HelmReleaseNotReady(hr, v2.SuspendedReason, msg), ctrl.Result{}, nil
	}

	// Reconcile chart based on the HelmChartTemplate
	hc, ok, reconcileErr := r.reconcileChart(ctx, &hr)
	if !ok {
		var msg string
		if reconcileErr != nil {
			msg = fmt.Sprintf("chart reconciliation failed: %s", reconcileErr.Error())
			r.event(hr, hr.Status.LastAttemptedRevision, recorder.EventSeverityError, msg)
		} else {
			msg = "HelmChart is not ready"
			r.event(hr, hr.Status.LastAttemptedRevision, recorder.EventSeverityInfo, msg)
		}
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, msg), ctrl.Result{}, reconcileErr
	}

	// Check chart artifact readiness
	if hc.GetArtifact() == nil {
		msg := "HelmChart is not ready"
		r.event(hr, hr.Status.LastAttemptedRevision, recorder.EventSeverityInfo, msg)
		log.Info(msg)
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, msg), ctrl.Result{}, nil
	}

	// Check dependencies
	if len(hr.Spec.DependsOn) > 0 {
		if err := r.checkDependencies(hr); err != nil {
			msg := fmt.Sprintf("dependencies do not meet ready condition (%s), retrying in %s",
				err.Error(), r.requeueDependency.String())
			r.event(hr, hc.GetArtifact().Revision, recorder.EventSeverityInfo, msg)
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
		r.event(hr, hr.Status.LastAttemptedRevision, recorder.EventSeverityError, err.Error())
		return v2.HelmReleaseNotReady(hr, v2.InitFailedReason, err.Error()), ctrl.Result{}, nil
	}

	// Load chart from artifact
	chart, err := r.loadHelmChart(hc)
	if err != nil {
		r.event(hr, hr.Status.LastAttemptedRevision, recorder.EventSeverityError, err.Error())
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, err.Error()), ctrl.Result{}, nil
	}

	// Reconcile Helm release
	reconciledHr, reconcileErr := r.reconcileRelease(ctx, log, *hr.DeepCopy(), chart, values)
	if reconcileErr != nil {
		r.event(hr, hc.GetArtifact().Revision, recorder.EventSeverityError,
			fmt.Sprintf("reconciliation failed: %s", reconcileErr.Error()))
	}
	return reconciledHr, ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, reconcileErr
}

type HelmReleaseReconcilerOptions struct {
	MaxConcurrentReconciles   int
	DependencyRequeueInterval time.Duration
}

func (r *HelmReleaseReconciler) SetupWithManager(mgr ctrl.Manager, opts HelmReleaseReconcilerOptions) error {
	r.requeueDependency = opts.DependencyRequeueInterval
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2.HelmRelease{}).
		WithEventFilter(predicates.ChangePredicate{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: opts.MaxConcurrentReconciles}).
		Complete(r)
}

func (r *HelmReleaseReconciler) reconcileChart(ctx context.Context, hr *v2.HelmRelease) (*sourcev1.HelmChart, bool, error) {
	chartName := types.NamespacedName{
		Namespace: hr.Spec.Chart.GetNamespace(hr.Namespace),
		Name:      hr.GetHelmChartName(),
	}

	// Garbage collect the previous HelmChart if the namespace named changed.
	if hr.Status.HelmChart != "" && hr.Status.HelmChart != chartName.String() {
		if err := r.garbageCollectHelmChart(ctx, *hr); err != nil {
			return nil, false, err
		}
	}

	// Continue with the reconciliation of the current template.
	var helmChart sourcev1.HelmChart
	err := r.Client.Get(ctx, chartName, &helmChart)
	if err != nil && !apierrors.IsNotFound(err) {
		return nil, false, err
	}
	hc := helmChartFromTemplate(*hr)
	switch {
	case apierrors.IsNotFound(err):
		if err = r.Client.Create(ctx, hc); err != nil {
			return nil, false, err
		}
		hr.Status.HelmChart = chartName.String()
		return nil, false, nil
	case helmChartRequiresUpdate(*hr, helmChart):
		r.Log.Info("chart diverged from template", strings.ToLower(sourcev1.HelmChartKind), chartName.String())
		helmChart.Spec = hc.Spec
		if err = r.Client.Update(ctx, &helmChart); err != nil {
			return nil, false, err
		}
		hr.Status.HelmChart = chartName.String()
		return nil, false, nil
	}

	return &helmChart, true, nil
}

func (r *HelmReleaseReconciler) reconcileRelease(ctx context.Context, log logr.Logger,
	hr v2.HelmRelease, chart *chart.Chart, values chartutil.Values) (v2.HelmRelease, error) {
	// Initialize Helm action runner
	run, err := runner.NewRunner(r.Config, hr.Namespace, r.Log)
	if err != nil {
		return v2.HelmReleaseNotReady(hr, v2.InitFailedReason, "failed to initialize Helm action runner"), err
	}

	// Determine last release revision.
	rel, observeLastReleaseErr := run.ObserveLastRelease(hr)
	if observeLastReleaseErr != nil {
		return v2.HelmReleaseNotReady(hr, v2.GetLastReleaseFailedReason, "failed to get last release revision"), err
	}

	// Register the current release attempt.
	revision := chart.Metadata.Version
	releaseRevision := util.ReleaseRevision(rel)
	valuesChecksum := util.ValuesChecksum(values)
	hr, hasNewState := v2.HelmReleaseAttempted(hr, revision, releaseRevision, valuesChecksum)
	if hasNewState {
		hr = v2.HelmReleaseProgressing(hr)
		if updateStatusErr := r.updateStatus(ctx, &hr); updateStatusErr != nil {
			log.Error(updateStatusErr, "unable to update status after state update")
			return hr, updateStatusErr
		}
	}

	// Check status of any previous release attempt.
	released := v2.GetHelmReleaseCondition(hr, v2.ReleasedCondition)
	if released != nil {
		switch released.Status {
		// Succeed if the previous release attempt succeeded.
		case corev1.ConditionTrue:
			return v2.HelmReleaseReady(hr), nil
		case corev1.ConditionFalse:
			// Fail if the previous release attempt remediation failed.
			remediated := v2.GetHelmReleaseCondition(hr, v2.RemediatedCondition)
			if remediated != nil && remediated.Status == corev1.ConditionFalse {
				err = fmt.Errorf("previous release attempt remediation failed")
				return v2.HelmReleaseNotReady(hr, remediated.Reason, remediated.Message), err
			}
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
		v2.DeleteHelmReleaseCondition(&hr, v2.RemediatedCondition)

		// If new release revision is successful and tests are enabled, run them.
		if err == nil && hr.Spec.GetTest().Enable {
			_, testErr := run.Test(hr)
			testErr = r.handleHelmActionResult(&hr, revision, testErr, "test", v2.TestSuccessCondition, v2.TestSucceededReason, v2.TestFailedReason)

			// Propagate any test error if not marked ignored.
			if testErr != nil && !remediation.MustIgnoreTestFailures(hr.Spec.GetTest().IgnoreFailures) {
				testsPassing := v2.GetHelmReleaseCondition(hr, v2.TestSuccessCondition)
				v2.SetHelmReleaseCondition(&hr, v2.ReleasedCondition, corev1.ConditionFalse, testsPassing.Reason, testsPassing.Message)
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
		reason := v2.ReconciliationFailedReason
		var cerr *ConditionError
		if errors.As(err, &cerr) {
			reason = cerr.Reason
		}
		return v2.HelmReleaseNotReady(hr, reason, err.Error()), err
	}
	return v2.HelmReleaseReady(hr), nil
}

func (r *HelmReleaseReconciler) updateStatus(ctx context.Context, hr *v2.HelmRelease) error {
	hr.Status.LastObservedTime = v1.Now()
	return r.Status().Update(ctx, hr)
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

		for _, condition := range dHr.Status.Conditions {
			if condition.Type == consts.ReadyCondition && condition.Status != corev1.ConditionTrue {
				return fmt.Errorf("dependency '%s' is not ready", dName)
			}
		}
	}
	return nil
}

func (r *HelmReleaseReconciler) composeValues(ctx context.Context, hr v2.HelmRelease) (chartutil.Values, error) {
	var result chartutil.Values
	for _, v := range hr.Spec.ValuesFrom {
		namespacedName := types.NamespacedName{Namespace: hr.Namespace, Name: v.Name}
		var valsData []byte
		switch v.Kind {
		case "ConfigMap":
			var resource corev1.ConfigMap
			if err := r.Get(ctx, namespacedName, &resource); err != nil {
				if apierrors.IsNotFound(err) {
					if v.Optional {
						r.Log.Info("could not find optional %s '%s'", v.Kind, namespacedName)
						continue
					}
					return nil, fmt.Errorf("could not find %s '%s'", v.Kind, namespacedName)
				}
				return nil, err
			}
			if data, ok := resource.Data[v.GetValuesKey()]; !ok {
				return nil, fmt.Errorf("missing key '%s' in %s '%s'", v.GetValuesKey(), v.Kind, namespacedName)
			} else {
				valsData = []byte(data)
			}
		case "Secret":
			var resource corev1.Secret
			if err := r.Get(ctx, namespacedName, &resource); err != nil {
				if apierrors.IsNotFound(err) {
					if v.Optional {
						r.Log.Info("could not find optional %s '%s'", v.Kind, namespacedName)
						continue
					}
					return nil, fmt.Errorf("could not find %s '%s'", v.Kind, namespacedName)
				}
				return nil, err
			}
			if data, ok := resource.Data[v.GetValuesKey()]; !ok {
				return nil, fmt.Errorf("missing key '%s' in %s '%s'", v.GetValuesKey(), v.Kind, namespacedName)
			} else {
				valsData = data
			}
		default:
			return nil, fmt.Errorf("unsupported ValuesReference kind '%s'", v.Kind)
		}
		switch v.TargetPath {
		case "":
			values, err := chartutil.ReadValues(valsData)
			if err != nil {
				return nil, fmt.Errorf("unable to read values from key '%s' in %s '%s': %w", v.GetValuesKey(), v.Kind, namespacedName, err)
			}
			result = util.MergeMaps(result, values)
		default:
			// TODO(hidde): this is a bit of hack, as it mimics the way the option string is passed
			// 	to Helm from a CLI perspective. Given the parser is however not publicly accessible
			// 	while it contains all logic around parsing the target path, it is a fair trade-off.
			singleVal := v.TargetPath + "=" + string(valsData)
			if err := strvals.ParseInto(singleVal, result); err != nil {
				return nil, fmt.Errorf("unable to merge value from key '%s' in %s '%s' into target path '%s': %w", v.GetValuesKey(), v.Kind, namespacedName, v.TargetPath, err)
			}
		}
	}
	return util.MergeMaps(result, hr.GetValues()), nil
}

// loadHelmChart attempts to download the artifact from the provided source,
// loads it into a chart.Chart, and removes the downloaded artifact.
// It returns the loaded chart.Chart on success, or an error.
func (r *HelmReleaseReconciler) loadHelmChart(source *sourcev1.HelmChart) (*chart.Chart, error) {
	f, err := ioutil.TempFile("", fmt.Sprintf("%s-%s-*.tgz", source.GetNamespace(), source.GetName()))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	defer os.Remove(f.Name())

	res, err := http.Get(source.GetArtifact().URL)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artifact '%s' download failed (status code: %s)", source.GetArtifact().URL, res.Status)
	}

	if _, err = io.Copy(f, res.Body); err != nil {
		return nil, err
	}

	return loader.Load(f.Name())
}

// garbageCollect deletes the v1alpha1.HelmChart of the v2alpha1.HelmRelease,
// and uninstalls the Helm release if the resource has not been suspended.
func (r *HelmReleaseReconciler) garbageCollect(ctx context.Context, logger logr.Logger, hr v2.HelmRelease) error {
	if err := r.garbageCollectHelmChart(ctx, hr); err != nil {
		return err
	}
	if hr.Spec.Suspend {
		logger.Info("skipping garbage collection for suspended resource")
		return nil
	}
	return r.garbageCollectHelmRelease(logger, hr)
}

// garbageCollectHelmChart deletes the v1alpha1.HelmChart of the
// v2alpha1.HelmRelease.
func (r *HelmReleaseReconciler) garbageCollectHelmChart(ctx context.Context, hr v2.HelmRelease) error {
	if hr.Status.HelmChart == "" {
		return nil
	}
	var hc sourcev1.HelmChart
	chartNS, chartName := hr.Status.GetHelmChart()
	err := r.Client.Get(ctx, types.NamespacedName{Namespace: chartNS, Name: chartName}, &hc)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		err = fmt.Errorf("failed to garbage collect HelmChart '%s': %w", hr.Status.HelmChart, err)
		return err
	}
	err = r.Client.Delete(ctx, &hc)
	if err != nil {
		err = fmt.Errorf("failed to garbage collect HelmChart '%s': %w", hr.Status.HelmChart, err)
	}
	return err
}

// garbageCollectHelmRelease uninstalls the deployed Helm release of
// the given v2alpha1.HelmRelease.
func (r *HelmReleaseReconciler) garbageCollectHelmRelease(logger logr.Logger, hr v2.HelmRelease) error {
	run, err := runner.NewRunner(r.Config, hr.Namespace, logger)
	if err != nil {
		return err
	}
	_, err = run.Config.Releases.Deployed(hr.GetReleaseName())
	if err != nil {
		if errors.Is(err, driver.ErrNoDeployedReleases) {
			return nil
		}
		err = fmt.Errorf("failed to garbage collect resource: %w", err)
		return err
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
		v2.SetHelmReleaseCondition(hr, condition, corev1.ConditionFalse, failedReason, msg)
		r.event(*hr, revision, recorder.EventSeverityError, msg)
		return &ConditionError{Reason: failedReason, Err: errors.New(msg)}
	} else {
		msg := fmt.Sprintf("Helm %s succeeded", action)
		v2.SetHelmReleaseCondition(hr, condition, corev1.ConditionTrue, succeededReason, msg)
		r.event(*hr, revision, recorder.EventSeverityInfo, msg)
		return nil
	}
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

func helmChartFromTemplate(hr v2.HelmRelease) *sourcev1.HelmChart {
	template := hr.Spec.Chart
	return &sourcev1.HelmChart{
		ObjectMeta: v1.ObjectMeta{
			Name:      hr.GetHelmChartName(),
			Namespace: hr.Spec.Chart.GetNamespace(hr.Namespace),
		},
		Spec: sourcev1.HelmChartSpec{
			Chart:   template.Spec.Chart,
			Version: template.Spec.Version,
			SourceRef: sourcev1.LocalHelmChartSourceReference{
				Name: template.Spec.SourceRef.Name,
				Kind: template.Spec.SourceRef.Kind,
			},
			Interval:   template.GetInterval(hr.Spec.Interval),
			ValuesFile: template.Spec.ValuesFile,
		},
	}
}

func helmChartRequiresUpdate(hr v2.HelmRelease, chart sourcev1.HelmChart) bool {
	template := hr.Spec.Chart
	switch {
	case template.Spec.Chart != chart.Spec.Chart:
		return true
	case template.Spec.Version != chart.Spec.Version:
		return true
	case template.Spec.SourceRef.Name != chart.Spec.SourceRef.Name:
		return true
	case template.Spec.SourceRef.Kind != chart.Spec.SourceRef.Kind:
		return true
	case template.GetInterval(hr.Spec.Interval) != chart.Spec.Interval:
		return true
	default:
		return false
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
