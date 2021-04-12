/*
Copyright 2021 The Flux authors

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
	"sync"

	"github.com/go-logr/logr"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/postrender"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	v2 "github.com/fluxcd/helm-controller/pkg/apis/helmrelease/v2beta1"
)

type ActionError struct {
	Err          error
	CapturedLogs string
}

func (e ActionError) Error() string {
	return e.Err.Error()
}

func (e ActionError) Unwrap() error {
	return e.Err
}

// Runner represents a Helm action runner capable of performing Helm
// operations for a v2beta1.HelmRelease.
type Runner struct {
	mu        sync.Mutex
	config    *action.Configuration
	logBuffer *LogBuffer
}

// NewRunner constructs a new Runner configured to run Helm actions with the
// given genericclioptions.RESTClientGetter, and the release and storage
// namespace configured to the provided values.
func NewRunner(getter genericclioptions.RESTClientGetter, storageNamespace string, logger logr.Logger) (*Runner, error) {
	runner := &Runner{
		logBuffer: NewLogBuffer(NewDebugLog(logger).Log, 0),
	}
	runner.config = new(action.Configuration)
	if err := runner.config.Init(getter, storageNamespace, "secret", runner.logBuffer.Log); err != nil {
		return nil, err
	}
	return runner, nil
}

// Create post renderer instances from HelmRelease and combine them into
// a single combined post renderer.
func postRenderers(hr v2.HelmRelease) (postrender.PostRenderer, error) {
	var combinedRenderer = newCombinedPostRenderer()
	for _, r := range hr.Spec.PostRenderers {
		if r.Kustomize != nil {
			combinedRenderer.addRenderer(newPostRendererKustomize(r.Kustomize))
		}
	}
	combinedRenderer.addRenderer(newPostRendererOriginLabels(&hr))
	if len(combinedRenderer.renderers) == 0 {
		return nil, nil
	}
	return &combinedRenderer, nil
}

// Install runs an Helm install action for the given v2beta1.HelmRelease.
func (r *Runner) Install(hr v2.HelmRelease, chart *chart.Chart, values chartutil.Values) (*release.Release, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.logBuffer.Reset()

	install := action.NewInstall(r.config)
	install.ReleaseName = hr.GetReleaseName()
	install.Namespace = hr.GetReleaseNamespace()
	install.Timeout = hr.Spec.GetInstall().GetTimeout(hr.GetTimeout()).Duration
	install.Wait = !hr.Spec.GetInstall().DisableWait
	install.DisableHooks = hr.Spec.GetInstall().DisableHooks
	install.DisableOpenAPIValidation = hr.Spec.GetInstall().DisableOpenAPIValidation
	install.Replace = hr.Spec.GetInstall().Replace
	install.SkipCRDs = hr.Spec.GetInstall().SkipCRDs
	install.Devel = true
	renderer, err := postRenderers(hr)
	if err != nil {
		return nil, err
	}
	install.PostRenderer = renderer
	if hr.Spec.TargetNamespace != "" {
		install.CreateNamespace = hr.Spec.GetInstall().CreateNamespace
	}

	rel, err := install.Run(chart, values.AsMap())
	return rel, wrapActionErr(r.logBuffer, err)
}

// Upgrade runs an Helm upgrade action for the given v2beta1.HelmRelease.
func (r *Runner) Upgrade(hr v2.HelmRelease, chart *chart.Chart, values chartutil.Values) (*release.Release, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.logBuffer.Reset()

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
	upgrade.Devel = true
	renderer, err := postRenderers(hr)
	if err != nil {
		return nil, err
	}
	upgrade.PostRenderer = renderer

	rel, err := upgrade.Run(hr.GetReleaseName(), chart, values.AsMap())
	return rel, wrapActionErr(r.logBuffer, err)
}

// Test runs an Helm test action for the given v2beta1.HelmRelease.
func (r *Runner) Test(hr v2.HelmRelease) (*release.Release, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.logBuffer.Reset()

	test := action.NewReleaseTesting(r.config)
	test.Namespace = hr.GetReleaseNamespace()
	test.Timeout = hr.Spec.GetTest().GetTimeout(hr.GetTimeout()).Duration

	rel, err := test.Run(hr.GetReleaseName())
	return rel, wrapActionErr(r.logBuffer, err)
}

// Rollback runs an Helm rollback action for the given v2beta1.HelmRelease.
func (r *Runner) Rollback(hr v2.HelmRelease) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.logBuffer.Reset()

	rollback := action.NewRollback(r.config)
	rollback.Timeout = hr.Spec.GetRollback().GetTimeout(hr.GetTimeout()).Duration
	rollback.Wait = !hr.Spec.GetRollback().DisableWait
	rollback.DisableHooks = hr.Spec.GetRollback().DisableHooks
	rollback.Force = hr.Spec.GetRollback().Force
	rollback.Recreate = hr.Spec.GetRollback().Recreate
	rollback.CleanupOnFail = hr.Spec.GetRollback().CleanupOnFail

	err := rollback.Run(hr.GetReleaseName())
	return wrapActionErr(r.logBuffer, err)
}

// Uninstall runs an Helm uninstall action for the given v2beta1.HelmRelease.
func (r *Runner) Uninstall(hr v2.HelmRelease) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	defer r.logBuffer.Reset()

	uninstall := action.NewUninstall(r.config)
	uninstall.Timeout = hr.Spec.GetUninstall().GetTimeout(hr.GetTimeout()).Duration
	uninstall.DisableHooks = hr.Spec.GetUninstall().DisableHooks
	uninstall.KeepHistory = hr.Spec.GetUninstall().KeepHistory

	_, err := uninstall.Run(hr.GetReleaseName())
	return wrapActionErr(r.logBuffer, err)
}

// ObserveLastRelease observes the last revision, if there is one,
// for the actual Helm release associated with the given v2beta1.HelmRelease.
func (r *Runner) ObserveLastRelease(hr v2.HelmRelease) (*release.Release, error) {
	rel, err := r.config.Releases.Last(hr.GetReleaseName())
	if err != nil && errors.Is(err, driver.ErrReleaseNotFound) {
		err = nil
	}
	return rel, err
}

func wrapActionErr(log *LogBuffer, err error) error {
	if err == nil {
		return err
	}
	err = &ActionError{
		Err:          err,
		CapturedLogs: log.String(),
	}
	return err
}
