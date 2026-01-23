/*
Copyright 2022 The Flux authors

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

package action

import (
	"context"
	"errors"
	"fmt"

	"github.com/fluxcd/cli-utils/pkg/kstatus/polling/engine"
	helmaction "helm.sh/helm/v4/pkg/action"
	helmchartutil "helm.sh/helm/v4/pkg/chart/common"
	helmchart "helm.sh/helm/v4/pkg/chart/v2"
	helmkube "helm.sh/helm/v4/pkg/kube"
	helmrelease "helm.sh/helm/v4/pkg/release/v1"
	helmdriver "helm.sh/helm/v4/pkg/storage/driver"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/features"
	"github.com/fluxcd/helm-controller/internal/postrender"
	"github.com/fluxcd/helm-controller/internal/release"
)

// UpgradeOption can be used to modify Helm's action.Upgrade after the instructions
// from the v2.HelmRelease have been applied. This is for example useful to
// enable the dry-run setting as a CLI.
type UpgradeOption func(upgrade *helmaction.Upgrade)

// WithUpgradeStatusReader sets the status reader used to evaluate
// health checks during upgrade wait.
func WithUpgradeStatusReader(reader engine.StatusReader) UpgradeOption {
	return func(upgrade *helmaction.Upgrade) {
		upgrade.WaitOptions = append(upgrade.WaitOptions, helmkube.WithKStatusReaders(reader))
	}
}

// Upgrade runs the Helm upgrade action with the provided config, using the
// v2.HelmReleaseSpec of the given object to determine the target release
// and upgrade configuration.
//
// It performs the upgrade according to the spec, which includes upgrading the
// CRDs according to the defined policy.
//
// It does not determine if there is a desire to perform the action, this is
// expected to be done by the caller. In addition, it does not take note of the
// action result. The caller is expected to listen to this using a
// storage.ObserveFunc, which provides superior access to Helm storage writes.
func Upgrade(ctx context.Context, config *helmaction.Configuration, obj *v2.HelmRelease, chrt *helmchart.Chart,
	vals helmchartutil.Values, opts ...UpgradeOption) (*helmrelease.Release, error) {
	upgrade := newUpgrade(config, obj, opts)

	// Resolve "auto" server-side apply setting.
	// We need to copy this code from Helm because we need to set ForceConflicts
	// based on the resolved value, since we always force conflicts on server-side apply
	// (Helm does not).
	releaseName := release.ShortenName(obj.GetReleaseName())
	serverSideApply := upgrade.ServerSideApply == "true"
	if upgrade.ServerSideApply == "auto" {
		lastRelease, err := config.Releases.Last(releaseName)
		if err != nil {
			if errors.Is(err, helmdriver.ErrReleaseNotFound) {
				return nil, helmdriver.NewErrNoDeployedReleases(releaseName)
			}
			return nil, err
		}
		lastReleaseTyped, ok := lastRelease.(*helmrelease.Release)
		if !ok {
			return nil, fmt.Errorf("only the Chart API v2 is supported")
		}
		serverSideApply = lastReleaseTyped.ApplyMethod == "ssa"
		upgrade.ServerSideApply = fmt.Sprint(serverSideApply)
	}
	upgrade.ForceConflicts = serverSideApply // We always force conflicts on server-side apply.

	policy, err := crdPolicyOrDefault(obj.GetUpgrade().CRDs)
	if err != nil {
		return nil, err
	}

	if err := applyCRDs(config, policy, chrt, vals, serverSideApply,
		upgrade.WaitStrategy, upgrade.WaitOptions,
		setOriginVisitor(v2.GroupVersion.Group, obj.Namespace, obj.Name)); err != nil {
		return nil, fmt.Errorf("failed to apply CustomResourceDefinitions: %w", err)
	}

	rlsr, err := upgrade.RunWithContext(ctx, releaseName, chrt, vals.AsMap())
	if err != nil {
		return nil, err
	}
	rlsrTyped, ok := rlsr.(*helmrelease.Release)
	if !ok {
		return nil, fmt.Errorf("only the Chart API v2 is supported")
	}
	return rlsrTyped, err
}

func newUpgrade(config *helmaction.Configuration, obj *v2.HelmRelease, opts []UpgradeOption) *helmaction.Upgrade {
	upgrade := helmaction.NewUpgrade(config)
	upgrade.ServerSideApply = "auto" // This must be the upgrade default regardless of UseHelm3Defaults.
	if ssa := obj.GetUpgrade().ServerSideApply; ssa != "" {
		upgrade.ServerSideApply = toHelmSSAValue(ssa)
	}

	upgrade.Namespace = obj.GetReleaseNamespace()
	upgrade.ResetValues = !obj.GetUpgrade().PreserveValues
	upgrade.ReuseValues = obj.GetUpgrade().PreserveValues
	upgrade.MaxHistory = obj.GetMaxHistory()
	upgrade.Timeout = obj.GetUpgrade().GetTimeout(obj.GetTimeout()).Duration
	upgrade.TakeOwnership = !obj.GetUpgrade().DisableTakeOwnership
	upgrade.WaitStrategy = getWaitStrategy(obj.GetWaitStrategy(), obj.GetUpgrade())
	upgrade.WaitForJobs = !obj.GetUpgrade().DisableWaitForJobs
	upgrade.DisableHooks = obj.GetUpgrade().DisableHooks
	upgrade.DisableOpenAPIValidation = obj.GetUpgrade().DisableOpenAPIValidation
	upgrade.SkipSchemaValidation = obj.GetUpgrade().DisableSchemaValidation
	upgrade.ForceReplace = obj.GetUpgrade().Force
	upgrade.CleanupOnFail = obj.GetUpgrade().CleanupOnFail
	upgrade.Devel = true

	// If the user opted-in to allow DNS lookups, enable it.
	if allowDNS, _ := features.Enabled(features.AllowDNSLookups); allowDNS {
		upgrade.EnableDNS = allowDNS
	}

	upgrade.PostRenderer = postrender.BuildPostRenderers(obj)

	for _, opt := range opts {
		opt(upgrade)
	}

	return upgrade
}
