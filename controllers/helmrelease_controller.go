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

	sourcev1 "github.com/fluxcd/source-controller/api/v1alpha1"
	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	"github.com/fluxcd/pkg/lockedfile"

	v2 "github.com/fluxcd/helm-controller/api/v2alpha1"
)

// HelmReleaseReconciler reconciles a HelmRelease object
type HelmReleaseReconciler struct {
	client.Client
	Config            *rest.Config
	Log               logr.Logger
	Scheme            *runtime.Scheme
	requeueDependency time.Duration
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

	log := r.Log.WithValues(strings.ToLower(hr.Kind), req.NamespacedName)

	hr = v2.HelmReleaseProgressing(hr)
	if err := r.Status().Update(ctx, &hr); err != nil {
		log.Error(err, "unable to update HelmRelease status")
		return ctrl.Result{Requeue: true}, err
	}

	// Get the artifact
	var source sourcev1.Source
	switch hr.Spec.SourceRef.Kind {
	case "HelmChart":
		var helmChart sourcev1.HelmChart
		chartName := types.NamespacedName{
			Namespace: hr.GetNamespace(),
			Name:      hr.Spec.SourceRef.Name,
		}
		err := r.Client.Get(ctx, chartName, &helmChart)
		if err != nil {
			log.Error(err, "HelmChart not found", "helmchart", chartName)
			return ctrl.Result{Requeue: true}, err
		}
		source = &helmChart
	default:
		err := fmt.Errorf("source '%s' kind '%s' not supported", hr.Spec.SourceRef.Name, hr.Spec.SourceRef.Kind)
		return ctrl.Result{}, err
	}

	// Check source readiness
	if source.GetArtifact() == nil {
		msg := "Source is not ready"
		hr = v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.ArtifactFailedReason, msg)
		if err := r.Status().Update(ctx, &hr); err != nil {
			log.Error(err, "unable to update HelmRelease status")
			return ctrl.Result{Requeue: true}, err
		}
		log.Info(msg)
		return ctrl.Result{}, nil
	}

	// Check dependencies
	if len(hr.Spec.DependsOn) > 0 {
		if err := r.checkDependencies(hr); err != nil {
			hr = v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.DependencyNotReadyReason, err.Error())
			if err := r.Status().Update(ctx, &hr); err != nil {
				log.Error(err, "unable to update HelmRelease status")
				return ctrl.Result{Requeue: true}, err
			}
			// Exponential backoff would cause execution to be prolonged too much,
			// instead we requeue on a fixed interval.
			msg := fmt.Sprintf("Dependencies do not meet ready condition (%s), retrying in %s", err.Error(), r.requeueDependency.String())
			log.Info(msg)
			return ctrl.Result{RequeueAfter: r.requeueDependency}, nil
		}
		log.Info("All dependencies area ready, proceeding with apply")
	}

	reconciledHr, err := r.release(log, hr, source)
	if err != nil {
		log.Error(err, "HelmRelease reconciliation failed", "revision", source.GetArtifact().Revision)
	}

	if err := r.Status().Update(ctx, &reconciledHr); err != nil {
		log.Error(err, "unable to update HelmRelease status after reconciliation")
		return ctrl.Result{Requeue: true}, err
	}

	// Log reconciliation duration
	log.Info(fmt.Sprintf("HelmRelease reconcilation finished in %s, next run in %s",
		time.Now().Sub(start).String(),
		hr.Spec.Interval.Duration.String(),
	))

	return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
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

func (r *HelmReleaseReconciler) release(log logr.Logger, hr v2.HelmRelease, source sourcev1.Source) (v2.HelmRelease, error) {
	// Acquire lock
	unlock, err := r.lock(fmt.Sprintf("%s-%s", hr.GetName(), hr.GetNamespace()))
	if err != nil {
		err = fmt.Errorf("tmp dir error: %w", err)
		return v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, sourcev1.StorageOperationFailedReason, err.Error()), err
	}
	defer unlock()

	// Create temp working dir
	tmpDir, err := ioutil.TempDir("", hr.Name)
	if err != nil {
		err = fmt.Errorf("tmp dir error: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// Download artifact
	artifactPath, err := r.download(source.GetArtifact().URL, tmpDir)
	if err != nil {
		return v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.ArtifactFailedReason, "artifact acquisition failed"), err
	}

	// Load chart
	loadedChart, err := loader.Load(artifactPath)
	if err != nil {
		return v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.ArtifactFailedReason, "failed to load chart"), err
	}

	// Initialize config
	cfg, err := r.newActionCfg(log, hr)
	if err != nil {
		return v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.InitFailedReason, "failed to initialize Helm action configuration"), err
	}

	// Get the current release
	rel, err := cfg.Releases.Deployed(hr.Name)
	if err != nil && !errors.Is(err, driver.ErrNoDeployedReleases) {
		return v2.HelmReleaseNotReady(hr, hr.Status.LastAttemptedRevision, hr.Status.LastReleaseRevision, v2.InitFailedReason, "failed to determine if release exists"), err
	}

	// Install or upgrade the release
	success := hr.Status.Failures == 0
	if errors.Is(err, driver.ErrNoDeployedReleases) {
		if rel, err = r.install(cfg, loadedChart, hr); err != nil {
			v2.SetHelmReleaseCondition(&hr, v2.InstallCondition, corev1.ConditionFalse, v2.InstallFailedReason, err.Error())
		} else {
			v2.SetHelmReleaseCondition(&hr, v2.InstallCondition, corev1.ConditionTrue, v2.InstallSucceededReason, "Helm installation succeeded")
		}
		success = err == nil
	} else if v2.ShouldUpgrade(hr, source.GetArtifact().Revision, rel.Version) {
		if rel, err = r.upgrade(cfg, loadedChart, hr); err != nil {
			v2.SetHelmReleaseCondition(&hr, v2.UpgradeCondition, corev1.ConditionFalse, v2.UpgradeFailedReason, err.Error())
		} else {
			v2.SetHelmReleaseCondition(&hr, v2.UpgradeCondition, corev1.ConditionTrue, v2.UpgradeSucceededReason, "Helm upgrade succeeded")
		}
		success = err == nil
	}

	// Run tests
	if v2.ShouldTest(hr) {
		if rel, err = r.test(cfg, hr); err != nil {
			v2.SetHelmReleaseCondition(&hr, v2.TestCondition, corev1.ConditionFalse, v2.TestFailedReason, err.Error())
		} else {
			v2.SetHelmReleaseCondition(&hr, v2.TestCondition, corev1.ConditionTrue, v2.TestSucceededReason, "Helm test succeeded")
		}
	}

	// Run rollback
	if rel != nil && v2.ShouldRollback(hr, rel.Version) {
		success = false
		if err = r.rollback(cfg, hr); err != nil {
			v2.SetHelmReleaseCondition(&hr, v2.RollbackCondition, corev1.ConditionFalse, v2.RollbackFailedReason, err.Error())
		} else {
			v2.SetHelmReleaseCondition(&hr, v2.RollbackCondition, corev1.ConditionTrue, v2.RollbackSucceededReason, "Helm rollback succeeded")
		}
	}

	// Determine release number after action runs
	var releaseRevision int
	if curRel, err := cfg.Releases.Deployed(hr.Name); err == nil {
		releaseRevision = curRel.Version
	}

	// Run uninstall
	if v2.ShouldUninstall(hr, releaseRevision) {
		if err = r.uninstall(cfg, hr); err != nil {
			v2.SetHelmReleaseCondition(&hr, v2.UninstallCondition, corev1.ConditionFalse, v2.UninstallFailedReason, err.Error())
		} else {
			v2.SetHelmReleaseCondition(&hr, v2.UninstallCondition, corev1.ConditionTrue, v2.UninstallSucceededReason, "Helm uninstall succeeded")
		}
	}

	if !success {
		return v2.HelmReleaseNotReady(hr, source.GetArtifact().Revision, releaseRevision, v2.ReconciliationFailedReason, "release reconciliation failed"), err
	}
	return v2.HelmReleaseReady(hr, source.GetArtifact().Revision, releaseRevision, v2.ReconciliationSucceededReason, "release reconciliation succeeded"), nil
}

func (r *HelmReleaseReconciler) install(cfg *action.Configuration, chart *chart.Chart, hr v2.HelmRelease) (*release.Release, error) {
	install := action.NewInstall(cfg)
	install.ReleaseName = hr.GetReleaseName()
	install.Namespace = hr.GetReleaseNamespace()
	install.Timeout = hr.Spec.Install.GetTimeout(hr.GetTimeout()).Duration
	install.Wait = !hr.Spec.Install.DisableWait
	install.DisableHooks = hr.Spec.Install.DisableHooks
	install.DisableOpenAPIValidation = hr.Spec.Install.DisableOpenAPIValidation
	install.Replace = hr.Spec.Install.Replace
	install.SkipCRDs = hr.Spec.Install.SkipCRDs

	return install.Run(chart, hr.GetValues())
}

func (r *HelmReleaseReconciler) upgrade(cfg *action.Configuration, chart *chart.Chart, hr v2.HelmRelease) (*release.Release, error) {
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

	return upgrade.Run(hr.Name, chart, hr.GetValues())
}

func (r *HelmReleaseReconciler) test(cfg *action.Configuration, hr v2.HelmRelease) (*release.Release, error) {
	test := action.NewReleaseTesting(cfg)
	test.Namespace = hr.GetReleaseNamespace()
	test.Timeout = hr.Spec.Test.GetTimeout(hr.GetTimeout()).Duration

	return test.Run(hr.GetReleaseName())
}

func (r *HelmReleaseReconciler) rollback(cfg *action.Configuration, hr v2.HelmRelease) error {
	rollback := action.NewRollback(cfg)
	rollback.Timeout = hr.Spec.Rollback.GetTimeout(hr.GetTimeout()).Duration
	rollback.Wait = !hr.Spec.Rollback.DisableWait
	rollback.DisableHooks = hr.Spec.Rollback.DisableHooks
	rollback.Force = hr.Spec.Rollback.Force
	rollback.Recreate = hr.Spec.Rollback.Recreate
	rollback.CleanupOnFail = hr.Spec.Rollback.CleanupOnFail

	return rollback.Run(hr.GetReleaseName())
}

func (r *HelmReleaseReconciler) uninstall(cfg *action.Configuration, hr v2.HelmRelease) error {
	uninstall := action.NewUninstall(cfg)
	uninstall.Timeout = hr.Spec.Uninstall.GetTimeout(hr.GetTimeout()).Duration
	uninstall.DisableHooks = hr.Spec.Uninstall.DisableHooks

	_, err := uninstall.Run(hr.GetReleaseName())
	return err
}

func (r *HelmReleaseReconciler) lock(name string) (unlock func(), err error) {
	lockFile := path.Join(os.TempDir(), name+".lock")
	mutex := lockedfile.MutexAt(lockFile)
	return mutex.Lock()
}

func (r *HelmReleaseReconciler) download(url, tmpDir string) (string, error) {
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

		if len(depHr.Status.Conditions) == 0 {
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

func (r *HelmReleaseReconciler) newActionCfg(log logr.Logger, hr v2.HelmRelease) (*action.Configuration, error) {
	cfg := new(action.Configuration)
	ns := hr.GetReleaseNamespace()
	err := cfg.Init(&genericclioptions.ConfigFlags{
		Namespace:   &ns,
		APIServer:   &r.Config.Host,
		CAFile:      &r.Config.CAFile,
		BearerToken: &r.Config.BearerToken,
	}, hr.Namespace, "secret", actionLogger(log))
	return cfg, err
}

func actionLogger(logger logr.Logger) func(format string, v ...interface{}) {
	return func(format string, v ...interface{}) {
		logger.Info(fmt.Sprintf(format, v...))
	}
}
