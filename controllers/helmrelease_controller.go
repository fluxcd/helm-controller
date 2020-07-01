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

	_ "helm.sh/helm/v3/pkg/action"

	v2 "github.com/fluxcd/helm-controller/api/v2alpha1"
	"github.com/fluxcd/helm-controller/internal/lockedfile"
)

// HelmReleaseReconciler reconciles a HelmRelease object
type HelmReleaseReconciler struct {
	client.Client
	Config *rest.Config
	Log    logr.Logger
	Scheme *runtime.Scheme
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
		hr = v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, msg)
		if err := r.Status().Update(ctx, &hr); err != nil {
			log.Error(err, "unable to update HelmRelease status")
			return ctrl.Result{Requeue: true}, err
		}
		log.Info(msg)
		return ctrl.Result{}, nil
	}

	reconciledHr, err := r.release(hr, source)
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
	MaxConcurrentReconciles int
}

func (r *HelmReleaseReconciler) SetupWithManager(mgr ctrl.Manager, opts HelmReleaseReconcilerOptions) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v2.HelmRelease{}).
		WithEventFilter(HelmReleaseReconcileAtPredicate{}).
		WithOptions(controller.Options{MaxConcurrentReconciles: opts.MaxConcurrentReconciles}).
		Complete(r)
}

func (r *HelmReleaseReconciler) release(hr v2.HelmRelease, source sourcev1.Source) (v2.HelmRelease, error) {
	// Acquire lock
	unlock, err := r.lock(fmt.Sprintf("%s-%s", hr.GetName(), hr.GetNamespace()))
	if err != nil {
		err = fmt.Errorf("tmp dir error: %w", err)
		return v2.HelmReleaseNotReady(hr, sourcev1.StorageOperationFailedReason, err.Error()), err
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
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, "artifact acquisition failed"), err
	}

	// Load chart
	loadedChart, err := loader.Load(artifactPath)
	if err != nil {
		return v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, "failed to load chart"), err
	}

	// Initialize config
	cfg, err := r.newActionCfg(hr)
	if err != nil {
		return v2.HelmReleaseNotReady(hr, v2.InitFailedReason, "failed to initialize Helm action configuration"), err
	}

	// Get the current release
	rel, err := cfg.Releases.Deployed(hr.Name)
	if err != nil && err != driver.ErrReleaseNotFound {
		return v2.HelmReleaseNotReady(hr, v2.InitFailedReason, "failed to determine if release exists"), err
	}

	// Install or upgrade the release
	if err == driver.ErrReleaseNotFound {
		if rel, err = r.install(cfg, loadedChart, hr); err != nil {
			v2.SetHelmReleaseCondition(&hr, v2.InstallCondition, corev1.ConditionFalse, v2.InstallFailedReason, err.Error())
			// TODO(hidde): conditional uninstall?
			return v2.HelmReleaseNotReady(hr, v2.ReconciliationFailedReason, "Helm install failed"), err
		}
		v2.SetHelmReleaseCondition(&hr, v2.InstallCondition, corev1.ConditionTrue, v2.InstallSucceededReason, "Helm installation succeeded")
	} else {
		if rel, err = r.upgrade(cfg, loadedChart, hr); err != nil {
			v2.SetHelmReleaseCondition(&hr, v2.UpgradeCondition, corev1.ConditionFalse, v2.UpgradeFailedReason, err.Error())
			return v2.HelmReleaseNotReady(hr, v2.ReconciliationFailedReason, "Helm upgrade failed"), err
		}
		v2.SetHelmReleaseCondition(&hr, v2.UpgradeCondition, corev1.ConditionTrue, v2.UpgradeSucceededReason, "Helm upgrade succeeded")
	}

	// TODO(hidde): check if test needs to be run
	// TODO(hidde): check if rollback needs to be performed

	return v2.HelmReleaseReady(hr, source.GetArtifact().Revision, rel.Version, v2.ReconciliationSucceededReason, "successfully reconciled HelmRelease"), nil
}

func (r *HelmReleaseReconciler) install(cfg *action.Configuration, chart *chart.Chart, hr v2.HelmRelease) (*release.Release, error) {
	install := action.NewInstall(cfg)
	install.ReleaseName = hr.Name
	install.Namespace = hr.Namespace

	return install.Run(chart, hr.Spec.Values.Data)
}

func (r *HelmReleaseReconciler) upgrade(cfg *action.Configuration, chart *chart.Chart, hr v2.HelmRelease) (*release.Release, error) {
	upgrade := action.NewUpgrade(cfg)
	upgrade.Namespace = hr.Namespace
	// TODO(hidde): make this configurable
	upgrade.ResetValues = true

	return upgrade.Run(hr.Name, chart, hr.Spec.Values.Data)
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

func (r *HelmReleaseReconciler) newActionCfg(hr v2.HelmRelease) (*action.Configuration, error) {
	cfg := new(action.Configuration)
	// TODO(hidde): write our own init
	err := cfg.Init(&genericclioptions.ConfigFlags{
		Namespace:   &hr.Namespace,
		APIServer:   &r.Config.Host,
		CAFile:      &r.Config.CAFile,
		BearerToken: &r.Config.BearerToken,
	}, hr.Namespace, "secret", r.Log.Info)
	return cfg, err
}
