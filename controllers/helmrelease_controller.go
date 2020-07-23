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
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	kuberecorder "k8s.io/client-go/tools/record"
	"k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/fluxcd/pkg/lockedfile"
	"github.com/fluxcd/pkg/recorder"
	sourcev1 "github.com/fluxcd/source-controller/api/v1alpha1"

	v2 "github.com/fluxcd/helm-controller/api/v2alpha1"
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

// +kubebuilder:rbac:groups=helm.fluxcd.io,resources=helmreleases,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=helm.fluxcd.io,resources=helmreleases/status,verbs=get;update;patch

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
			if err := r.gc(ctx, log, hr); err != nil {
				r.event(hr, hr.Status.LastAttemptedRevision, recorder.EventSeverityError, fmt.Sprintf("garbage collection for deleted resource failed: %s", err.Error()))
				// Return the error so we retry the failed garbage collection
				return ctrl.Result{}, err
			}

			// Remove our finalizer from the list and update it
			hr.ObjectMeta.Finalizers = removeString(hr.ObjectMeta.Finalizers, v2.HelmReleaseFinalizer)
			if err := r.Update(ctx, &hr); err != nil {
				return ctrl.Result{}, err
			}
		}
		// Stop reconciliation as the object is being deleted
		return ctrl.Result{}, nil
	}

	if hr.Spec.Suspend {
		msg := "HelmRelease is suspended, skipping reconciliation"
		hr = v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.SuspendedReason, msg)
		if err := r.Status().Update(ctx, &hr); err != nil {
			log.Error(err, "unable to update status")
			return ctrl.Result{Requeue: true}, err
		}
		log.Info(msg)
		return ctrl.Result{}, nil
	}

	hr = v2.HelmReleaseProgressing(hr)
	if err := r.Status().Update(ctx, &hr); err != nil {
		log.Error(err, "unable to update status")
		return ctrl.Result{Requeue: true}, err
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
		hr = v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.ArtifactFailedReason, msg)
		if err := r.Status().Update(ctx, &hr); err != nil {
			log.Error(err, "unable to update status")
			return ctrl.Result{Requeue: true}, err
		}
		return ctrl.Result{}, reconcileErr
	}

	// Check chart artifact readiness
	if hc.GetArtifact() == nil {
		msg := "HelmChart is not ready"
		hr = v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.ArtifactFailedReason, msg)
		r.event(hr, hr.Status.LastAttemptedRevision, recorder.EventSeverityInfo, msg)
		log.Info(msg)
		if err := r.Status().Update(ctx, &hr); err != nil {
			log.Error(err, "unable to update status")
			return ctrl.Result{Requeue: true}, err
		}
		return ctrl.Result{}, nil
	}

	// Check dependencies
	if len(hr.Spec.DependsOn) > 0 {
		if err := r.checkDependencies(hr); err != nil {
			msg := fmt.Sprintf("dependencies do not meet ready condition (%s), retrying in %s", err.Error(), r.requeueDependency.String())
			r.event(hr, hc.GetArtifact().Revision, recorder.EventSeverityInfo, msg)
			log.Info(msg)

			hr = v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.DependencyNotReadyReason, err.Error())
			if err := r.Status().Update(ctx, &hr); err != nil {
				log.Error(err, "unable to update status")
				return ctrl.Result{Requeue: true}, err
			}
			// Exponential backoff would cause execution to be prolonged too much,
			// instead we requeue on a fixed interval.
			return ctrl.Result{RequeueAfter: r.requeueDependency}, nil
		}
		log.Info("all dependencies are ready, proceeding with release")
	}

	// Compose values
	values, err := r.composeValues(ctx, hr)
	if err != nil {
		hr = v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.InitFailedReason, err.Error())
		r.event(hr, hr.Status.LastAttemptedRevision, recorder.EventSeverityError, err.Error())
		if err := r.Status().Update(ctx, &hr); err != nil {
			log.Error(err, "unable to update status")
			return ctrl.Result{Requeue: true}, err
		}
		return ctrl.Result{}, nil
	}

	reconciledHr, reconcileErr := r.release(log, *hr.DeepCopy(), hc, values)
	if reconcileErr != nil {
		r.event(hr, hc.GetArtifact().Revision, recorder.EventSeverityError, fmt.Sprintf("reconciliation failed: %s", reconcileErr.Error()))
	}

	if err := r.Status().Update(ctx, &reconciledHr); err != nil {
		log.Error(err, "unable to update status after reconciliation")
		return ctrl.Result{Requeue: true}, err
	}

	// Log reconciliation duration
	log.Info(fmt.Sprintf("reconcilation finished in %s, next run in %s",
		time.Now().Sub(start).String(),
		hr.Spec.Interval.Duration.String(),
	))

	return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, reconcileErr
}

type HelmReleaseReconcilerOptions struct {
	MaxConcurrentReconciles   int
	DependencyRequeueInterval time.Duration
}

func (r *HelmReleaseReconciler) SetupWithManager(mgr ctrl.Manager, opts HelmReleaseReconcilerOptions) error {
	r.requeueDependency = opts.DependencyRequeueInterval
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2.HelmRelease{}).
		WithEventFilter(HelmReleaseReconcileAtPredicate{}).
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
		prevChartNS, prevChartName := hr.Status.GetHelmChart()
		var prevHelmChart sourcev1.HelmChart
		err := r.Client.Get(ctx, types.NamespacedName{Namespace: prevChartNS, Name: prevChartName}, &prevHelmChart)
		switch {
		case apierrors.IsNotFound(err):
			// noop
		case err != nil:
			return nil, false, err
		default:
			if err := r.Client.Delete(ctx, &prevHelmChart); err != nil {
				err = fmt.Errorf("failed to garbage collect HelmChart: %w", err)
				return nil, false, err
			}
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

func (r *HelmReleaseReconciler) release(log logr.Logger, hr v2.HelmRelease, source sourcev1.Source, values chartutil.Values) (v2.HelmRelease, error) {
	// Acquire lock
	unlock, err := lock(fmt.Sprintf("%s-%s", hr.GetName(), hr.GetNamespace()))
	if err != nil {
		err = fmt.Errorf("lockfile error: %w", err)
		return v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, sourcev1.StorageOperationFailedReason, err.Error()), err
	}
	defer unlock()

	// Create temp working dir
	tmpDir, err := ioutil.TempDir("", hr.GetReleaseName())
	if err != nil {
		err = fmt.Errorf("tmp dir error: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download artifact
	artifactPath, err := download(source.GetArtifact().URL, tmpDir)
	if err != nil {
		return v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.ArtifactFailedReason, "artifact acquisition failed"), err
	}

	// Load chart
	loadedChart, err := loader.Load(artifactPath)
	if err != nil {
		return v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.ArtifactFailedReason, "failed to load chart"), err
	}

	// Initialize config
	cfg, err := newActionCfg(log, r.Config, hr)
	if err != nil {
		return v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.InitFailedReason, "failed to initialize Helm action configuration"), err
	}

	// Get the current release
	rel, err := cfg.Releases.Deployed(hr.GetReleaseName())
	if err != nil && !errors.Is(err, driver.ErrNoDeployedReleases) {
		return v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.InitFailedReason, "failed to determine if release exists"), err
	}

	// Install or upgrade the release
	success := true
	if errors.Is(err, driver.ErrNoDeployedReleases) {
		rel, err = install(cfg, loadedChart, hr, values)
		r.handleHelmActionResult(hr, source, err, "install", v2.InstalledCondition, v2.InstallSucceededReason, v2.InstallFailedReason)
		success = err == nil
	} else if v2.ShouldUpgrade(hr, source.GetArtifact().Revision, rel.Version) {
		rel, err = upgrade(cfg, loadedChart, hr, values)
		r.handleHelmActionResult(hr, source, err, "upgrade", v2.UpgradedCondition, v2.UpgradeSucceededReason, v2.UpgradeFailedReason)
		success = err == nil
	}

	// Run tests
	if v2.ShouldTest(hr) {
		rel, err = test(cfg, hr)
		r.handleHelmActionResult(hr, source, err, "test", v2.TestedCondition, v2.TestSucceededReason, v2.TestFailedReason)
	}

	// Run rollback
	if rel != nil && v2.ShouldRollback(hr, rel.Version) {
		success = false
		err = rollback(cfg, hr)
		r.handleHelmActionResult(hr, source, err, "rollback", v2.RolledBackCondition, v2.RollbackSucceededReason, v2.RollbackFailedReason)
	}

	// Determine release number after action runs
	var releaseRevision int
	if curRel, err := cfg.Releases.Deployed(hr.GetReleaseName()); err == nil {
		releaseRevision = curRel.Version
	}

	// Run uninstall
	if v2.ShouldUninstall(hr, releaseRevision) {
		success = false
		err = uninstall(cfg, hr)
		if err == nil {
			releaseRevision = 0
		}
		r.handleHelmActionResult(hr, source, err, "uninstall", v2.UninstalledCondition, v2.UninstallSucceededReason, v2.UninstallFailedReason)
	}

	if !success {
		return v2.HelmReleaseNotReady(hr, source.GetArtifact().Revision, releaseRevision, v2.ReconciliationFailedReason, "release reconciliation failed"), err
	}
	return v2.HelmReleaseReady(hr, source.GetArtifact().Revision, releaseRevision, v2.ReconciliationSucceededReason, "release reconciliation succeeded"), nil
}

func (r *HelmReleaseReconciler) checkDependencies(hr v2.HelmRelease) error {
	for _, dep := range hr.Spec.DependsOn {
		depName := types.NamespacedName{
			Namespace: hr.GetNamespace(),
			Name:      dep,
		}
		var depHr v2.HelmRelease
		err := r.Get(context.Background(), depName, &depHr)
		if err != nil {
			return fmt.Errorf("unable to get '%s' dependency: %w", depName, err)
		}

		if len(depHr.Status.Conditions) == 0 || depHr.Generation != depHr.Status.ObservedGeneration {
			return fmt.Errorf("dependency '%s' is not ready", depName)
		}

		for _, condition := range depHr.Status.Conditions {
			if condition.Type == v2.ReadyCondition && condition.Status != corev1.ConditionTrue {
				return fmt.Errorf("dependency '%s' is not ready", depName)
			}
		}
	}
	return nil
}

func (r *HelmReleaseReconciler) gc(ctx context.Context, log logr.Logger, hr v2.HelmRelease) error {
	// Garbage collect the HelmChart
	if hr.Status.HelmChart != "" {
		var hc sourcev1.HelmChart
		chartNS, chartName := hr.Status.GetHelmChart()
		err := r.Client.Get(ctx, types.NamespacedName{Namespace: chartNS, Name: chartName}, &hc)
		switch {
		case apierrors.IsNotFound(err):
			// noop
		case err == nil:
			if err = r.Client.Delete(ctx, &hc); err != nil {
				return err
			}
		default:
			return err
		}
	}

	// Uninstall the Helm release
	var uninstallErr error
	if !hr.Spec.Suspend {
		cfg, err := newActionCfg(log, r.Config, hr)
		if err != nil {
			return err
		}
		_, err = cfg.Releases.Deployed(hr.GetReleaseName())
		switch {
		case errors.Is(err, driver.ErrNoDeployedReleases):
			// noop
		case err == nil:
			uninstallErr = uninstall(cfg, hr)
		default:
			return err
		}
	}

	switch uninstallErr {
	case nil:
		r.event(hr, hr.Status.LastAttemptedRevision, recorder.EventSeverityInfo, "Helm uninstall for deleted resource succeeded")
		return nil
	default:
		return uninstallErr
	}
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
		values, err := chartutil.ReadValues(valsData)
		if err != nil {
			return nil, fmt.Errorf("unable to read values from key '%s' in %s '%s': %w", v.GetValuesKey(), v.Kind, namespacedName, err)
		}
		result = chartutil.CoalesceTables(result, values)
	}
	return chartutil.CoalesceTables(result, hr.GetValues()), nil
}

func (r *HelmReleaseReconciler) handleHelmActionResult(hr v2.HelmRelease, source sourcev1.Source, err error, action string, condition string, succeededReason string, failedReason string) {
	if err != nil {
		v2.SetHelmReleaseCondition(&hr, condition, corev1.ConditionFalse, failedReason, err.Error())
		r.event(hr, source.GetArtifact().Revision, recorder.EventSeverityError, fmt.Sprintf("Helm %s failed: %s", action, err.Error()))
	} else {
		msg := fmt.Sprintf("Helm %s succeeded", action)
		v2.SetHelmReleaseCondition(&hr, condition, corev1.ConditionTrue, succeededReason, msg)
		r.event(hr, source.GetArtifact().Revision, recorder.EventSeverityInfo, msg)
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
			Name:    template.Name,
			Version: template.Version,
			HelmRepositoryRef: corev1.LocalObjectReference{
				Name: template.SourceRef.Name,
			},
			Interval: template.GetInterval(hr.Spec.Interval),
		},
	}
}

func helmChartRequiresUpdate(hr v2.HelmRelease, chart sourcev1.HelmChart) bool {
	template := hr.Spec.Chart
	switch {
	case template.Name != chart.Spec.Name:
		return true
	case template.Version != chart.Spec.Version:
		return true
	case template.SourceRef.Name != chart.Spec.HelmRepositoryRef.Name:
		return true
	case template.GetInterval(hr.Spec.Interval) != chart.Spec.Interval:
		return true
	default:
		return false
	}
}

func install(cfg *action.Configuration, chart *chart.Chart, hr v2.HelmRelease, values chartutil.Values) (*release.Release, error) {
	install := action.NewInstall(cfg)
	install.ReleaseName = hr.GetReleaseName()
	install.Namespace = hr.GetReleaseNamespace()
	install.Timeout = hr.Spec.Install.GetTimeout(hr.GetTimeout()).Duration
	install.Wait = !hr.Spec.Install.DisableWait
	install.DisableHooks = hr.Spec.Install.DisableHooks
	install.DisableOpenAPIValidation = hr.Spec.Install.DisableOpenAPIValidation
	install.Replace = hr.Spec.Install.Replace
	install.SkipCRDs = hr.Spec.Install.SkipCRDs

	return install.Run(chart, values.AsMap())
}

func upgrade(cfg *action.Configuration, chart *chart.Chart, hr v2.HelmRelease, values chartutil.Values) (*release.Release, error) {
	upgrade := action.NewUpgrade(cfg)
	upgrade.Namespace = hr.GetReleaseNamespace()
	upgrade.ResetValues = !hr.Spec.Upgrade.PreserveValues
	upgrade.ReuseValues = hr.Spec.Upgrade.PreserveValues
	upgrade.MaxHistory = hr.GetMaxHistory()
	upgrade.Timeout = hr.Spec.Upgrade.GetTimeout(hr.GetTimeout()).Duration
	upgrade.Wait = !hr.Spec.Upgrade.DisableWait
	upgrade.DisableHooks = hr.Spec.Upgrade.DisableHooks
	upgrade.Force = hr.Spec.Upgrade.Force
	upgrade.CleanupOnFail = hr.Spec.Upgrade.CleanupOnFail

	return upgrade.Run(hr.GetReleaseName(), chart, values.AsMap())
}

func test(cfg *action.Configuration, hr v2.HelmRelease) (*release.Release, error) {
	test := action.NewReleaseTesting(cfg)
	test.Namespace = hr.GetReleaseNamespace()
	test.Timeout = hr.Spec.Test.GetTimeout(hr.GetTimeout()).Duration

	return test.Run(hr.GetReleaseName())
}

func rollback(cfg *action.Configuration, hr v2.HelmRelease) error {
	rollback := action.NewRollback(cfg)
	rollback.Timeout = hr.Spec.Rollback.GetTimeout(hr.GetTimeout()).Duration
	rollback.Wait = !hr.Spec.Rollback.DisableWait
	rollback.DisableHooks = hr.Spec.Rollback.DisableHooks
	rollback.Force = hr.Spec.Rollback.Force
	rollback.Recreate = hr.Spec.Rollback.Recreate
	rollback.CleanupOnFail = hr.Spec.Rollback.CleanupOnFail

	return rollback.Run(hr.GetReleaseName())
}

func uninstall(cfg *action.Configuration, hr v2.HelmRelease) error {
	uninstall := action.NewUninstall(cfg)
	uninstall.Timeout = hr.Spec.Uninstall.GetTimeout(hr.GetTimeout()).Duration
	uninstall.DisableHooks = hr.Spec.Uninstall.DisableHooks
	uninstall.KeepHistory = hr.Spec.Uninstall.KeepHistory

	_, err := uninstall.Run(hr.GetReleaseName())
	return err
}

func lock(name string) (unlock func(), err error) {
	lockFile := path.Join(os.TempDir(), name+".lock")
	mutex := lockedfile.MutexAt(lockFile)
	return mutex.Lock()
}

func download(url, tmpDir string) (string, error) {
	fp := filepath.Join(tmpDir, "artifact.tar.gz")
	out, err := os.Create(fp)
	if err != nil {
		return "", err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fp, fmt.Errorf("artifact '%s' download failed (status code: %s)", url, resp.Status)
	}

	if _, err = io.Copy(out, resp.Body); err != nil {
		return "", err
	}

	return fp, nil
}

func newActionCfg(log logr.Logger, clusterCfg *rest.Config, hr v2.HelmRelease) (*action.Configuration, error) {
	cfg := new(action.Configuration)
	ns := hr.GetReleaseNamespace()
	err := cfg.Init(&genericclioptions.ConfigFlags{
		Namespace:   &ns,
		APIServer:   &clusterCfg.Host,
		CAFile:      &clusterCfg.CAFile,
		BearerToken: &clusterCfg.BearerToken,
	}, hr.Namespace, "secret", actionLogger(log))
	return cfg, err
}

func actionLogger(logger logr.Logger) func(format string, v ...interface{}) {
	return func(format string, v ...interface{}) {
		logger.Info(fmt.Sprintf(format, v...))
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
