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

package runner

import (
	"errors"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/fluxcd/helm-controller/internal/storage"
)

// Runner is a Helm action runner capable of performing Helm operations
// for a v2beta1.HelmRelease.
type Runner struct {
	config   *action.Configuration
	observer *storage.Observer
}

// NewRunner constructs a new Runner configured to run Helm actions with the
// given genericclioptions.RESTClientGetter, and the release and storage
// namespace configured to the provided values.
func NewRunner(getter genericclioptions.RESTClientGetter, hr *v2.HelmRelease, log logr.Logger) (*Runner, error) {
	cfg := new(action.Configuration)
	if err := cfg.Init(getter, hr.GetNamespace(), strings.ToLower(driver.SecretsDriverName), debugLogger(log)); err != nil {
		return nil, err
	}
	return NewRunnerWithConfig(cfg, hr)
}

// NewRunnerWithConfig constructs a new Runner configured with the
// given action.Configuration.
func NewRunnerWithConfig(cfg *action.Configuration, hr *v2.HelmRelease) (*Runner, error) {
	last, err := cfg.Releases.Get(hr.GetReleaseName(), hr.Status.LastReleaseRevision)
	if err != nil && !errors.Is(err, driver.ErrReleaseNotFound) {
		return nil, err
	}
	observer := storage.NewObserver(cfg.Releases.Driver, last)
	cfg.Releases = helmstorage.Init(observer)
	return &Runner{config: cfg, observer: observer}, nil
}

// Install runs an Helm install action for the given v2beta1.HelmRelease.
func (r *Runner) Install(hr v2.HelmRelease, chart *chart.Chart, values chartutil.Values) error {
	install := action.NewInstall(r.config)
	install.ReleaseName = hr.GetReleaseName()
	install.Namespace = hr.GetReleaseNamespace()
	install.Timeout = hr.Spec.GetInstall().GetTimeout(hr.GetTimeout()).Duration
	install.Wait = !hr.Spec.GetInstall().DisableWait
	install.DisableHooks = hr.Spec.GetInstall().DisableHooks
	install.DisableOpenAPIValidation = hr.Spec.GetInstall().DisableOpenAPIValidation
	install.Replace = hr.Spec.GetInstall().Replace
	install.SkipCRDs = hr.Spec.GetInstall().SkipCRDs

	if hr.Spec.TargetNamespace != "" {
		install.CreateNamespace = hr.Spec.GetInstall().CreateNamespace
	}

	_, err := install.Run(chart, values.AsMap())
	return err
}

// Upgrade runs an Helm upgrade action for the given v2beta1.HelmRelease.
func (r *Runner) Upgrade(hr v2.HelmRelease, chart *chart.Chart, values chartutil.Values) error {
	upgrade := action.NewUpgrade(r.config)
	upgrade.Namespace = hr.GetReleaseNamespace()
	upgrade.ResetValues = !hr.Spec.GetUpgrade().PreserveValues
	upgrade.ReuseValues = hr.Spec.GetUpgrade().PreserveValues
	upgrade.MaxHistory = hr.GetMaxHistory()
	upgrade.Timeout = hr.Spec.GetUpgrade().GetTimeout(hr.GetTimeout()).Duration
	upgrade.Wait = !hr.Spec.GetUpgrade().DisableWait
	upgrade.DisableHooks = hr.Spec.GetUpgrade().DisableHooks
	upgrade.Force = hr.Spec.GetUpgrade().Force
	upgrade.CleanupOnFail = hr.Spec.GetUpgrade().CleanupOnFail

	_, err := upgrade.Run(hr.GetReleaseName(), chart, values.AsMap())
	return err
}

// Test runs an Helm test action for the given v2beta1.HelmRelease.
func (r *Runner) Test(hr v2.HelmRelease) error {
	test := action.NewReleaseTesting(r.config)
	test.Namespace = hr.GetReleaseNamespace()
	test.Timeout = hr.Spec.GetTest().GetTimeout(hr.GetTimeout()).Duration

	_, err := test.Run(hr.GetReleaseName())
	return err
}

// Rollback runs an Helm rollback action to the last successful release
// revision of the given v2beta1.HelmRelease.
func (r *Runner) Rollback(hr v2.HelmRelease) error {
	rollback := action.NewRollback(r.config)
	rollback.Timeout = hr.Spec.GetRollback().GetTimeout(hr.GetTimeout()).Duration
	rollback.Wait = !hr.Spec.GetRollback().DisableWait
	rollback.DisableHooks = hr.Spec.GetRollback().DisableHooks
	rollback.Force = hr.Spec.GetRollback().Force
	rollback.Recreate = hr.Spec.GetRollback().Recreate
	rollback.CleanupOnFail = hr.Spec.GetRollback().CleanupOnFail
	rollback.Version = hr.Status.LastSuccessfulReleaseRevision

	return rollback.Run(hr.GetReleaseName())
}

// Uninstall runs an Helm uninstall action for the given v2beta1.HelmRelease.
func (r *Runner) Uninstall(hr *v2.HelmRelease) error {
	uninstall := action.NewUninstall(r.config)
	uninstall.Timeout = hr.Spec.GetUninstall().GetTimeout(hr.GetTimeout()).Duration
	uninstall.DisableHooks = hr.Spec.GetUninstall().DisableHooks
	uninstall.KeepHistory = hr.Spec.GetUninstall().KeepHistory

	_, err := uninstall.Run(hr.GetReleaseName())
	return err
}

// ObserveLastRelease observes the last revision, if there is one,
// for the actual Helm release associated with the given v2beta1.HelmRelease.
func (r *Runner) ObserveLastRelease(hr *v2.HelmRelease) (*release.Release, error) {
	rel, err := r.config.Releases.Last(hr.GetReleaseName())
	if err != nil && errors.Is(err, driver.ErrReleaseNotFound) {
		err = nil
	}
	return rel, err
}

// GetLastPersistedRelease returns the release last persisted to the
// Helm storage by a Runner action.
func (r *Runner) GetLastPersistedRelease() *release.Release {
	return r.observer.GetLastObservedRelease()
}

func debugLogger(logger logr.Logger) func(format string, v ...interface{}) {
	return func(format string, v ...interface{}) {
		logger.V(1).Info(fmt.Sprintf(format, v...))
	}
}
