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

package runner

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"

	v2 "github.com/fluxcd/helm-controller/api/v2alpha1"
)

// Runner represents a Helm action runner capable of performing Helm
// operations for a v2alpha1.HelmRelease.
type Runner struct {
	config *action.Configuration
}

// NewRunner constructs a new Runner configured to run Helm actions with the
// given rest.Config, and the storage namespace configured to the provided
// namespace.
func NewRunner(clusterCfg *rest.Config, namespace string, logger logr.Logger) (*Runner, error) {
	cfg := new(action.Configuration)
	if err := cfg.Init(&genericclioptions.ConfigFlags{
		APIServer:   &clusterCfg.Host,
		CAFile:      &clusterCfg.CAFile,
		BearerToken: &clusterCfg.BearerToken,
	}, namespace, "secret", debugLogger(logger)); err != nil {
		return nil, err
	}
	return &Runner{config: cfg}, nil
}

// Install runs an Helm install action for the given v2alpha1.HelmRelease.
func (r *Runner) Install(hr v2.HelmRelease, chart *chart.Chart, values chartutil.Values) (*release.Release, error) {
	install := action.NewInstall(r.config)
	install.ReleaseName = hr.GetReleaseName()
	install.Namespace = hr.GetReleaseNamespace()
	install.Timeout = hr.Spec.GetInstall().GetTimeout(hr.GetTimeout()).Duration
	install.Wait = !hr.Spec.GetInstall().DisableWait
	install.DisableHooks = hr.Spec.GetInstall().DisableHooks
	install.DisableOpenAPIValidation = hr.Spec.GetInstall().DisableOpenAPIValidation
	install.Replace = hr.Spec.GetInstall().Replace
	install.SkipCRDs = hr.Spec.GetInstall().SkipCRDs

	return install.Run(chart, values.AsMap())
}

// Upgrade runs an Helm upgrade action for the given v2alpha1.HelmRelease.
func (r *Runner) Upgrade(hr v2.HelmRelease, chart *chart.Chart, values chartutil.Values) (*release.Release, error) {
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

	return upgrade.Run(hr.GetReleaseName(), chart, values.AsMap())
}

// Test runs an Helm test action for the given v2alpha1.HelmRelease.
func (r *Runner) Test(hr v2.HelmRelease) (*release.Release, error) {
	test := action.NewReleaseTesting(r.config)
	test.Namespace = hr.GetReleaseNamespace()
	test.Timeout = hr.Spec.GetTest().GetTimeout(hr.GetTimeout()).Duration

	return test.Run(hr.GetReleaseName())
}

// Rollback runs an Helm rollback action for the given v2alpha1.HelmRelease.
func (r *Runner) Rollback(hr v2.HelmRelease) error {
	rollback := action.NewRollback(r.config)
	rollback.Timeout = hr.Spec.GetRollback().GetTimeout(hr.GetTimeout()).Duration
	rollback.Wait = !hr.Spec.GetRollback().DisableWait
	rollback.DisableHooks = hr.Spec.GetRollback().DisableHooks
	rollback.Force = hr.Spec.GetRollback().Force
	rollback.Recreate = hr.Spec.GetRollback().Recreate
	rollback.CleanupOnFail = hr.Spec.GetRollback().CleanupOnFail

	return rollback.Run(hr.GetReleaseName())
}

// Uninstall runs an Helm uninstall action for the given v2alpha1.HelmRelease.
func (r *Runner) Uninstall(hr v2.HelmRelease) error {
	uninstall := action.NewUninstall(r.config)
	uninstall.Timeout = hr.Spec.GetUninstall().GetTimeout(hr.GetTimeout()).Duration
	uninstall.DisableHooks = hr.Spec.GetUninstall().DisableHooks
	uninstall.KeepHistory = hr.Spec.GetUninstall().KeepHistory

	_, err := uninstall.Run(hr.GetReleaseName())
	return err
}

// ObserveLastRelease observes the last revision, if there is one,
// for the actual Helm release associated with the given v2alpha1.HelmRelease.
func (r *Runner) ObserveLastRelease(hr v2.HelmRelease) (*release.Release, error) {
	rel, err := r.config.Releases.Last(hr.GetReleaseName())
	if err != nil && errors.Is(err, driver.ErrReleaseNotFound) {
		err = nil
	}
	return rel, err
}

func debugLogger(logger logr.Logger) func(format string, v ...interface{}) {
	return func(format string, v ...interface{}) {
		logger.V(1).Info(fmt.Sprintf(format, v...))
	}
}
