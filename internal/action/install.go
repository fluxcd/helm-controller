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
	"fmt"

	helmaction "helm.sh/helm/v4/pkg/action"
	helmchartutil "helm.sh/helm/v4/pkg/chart/common"
	helmchart "helm.sh/helm/v4/pkg/chart/v2"
	helmrelease "helm.sh/helm/v4/pkg/release/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/features"
	"github.com/fluxcd/helm-controller/internal/postrender"
	"github.com/fluxcd/helm-controller/internal/release"
)

// InstallOption can be used to modify Helm's action.Install after the instructions
// from the v2.HelmRelease have been applied. This is for example useful to
// enable the dry-run setting as a CLI.
type InstallOption func(action *helmaction.Install)

// Install runs the Helm install action with the provided config, using the
// v2.HelmReleaseSpec of the given object to determine the target release
// and rollback configuration.
//
// It performs the installation according to the spec, which includes installing
// the CRDs according to the defined policy.
//
// It does not determine if there is a desire to perform the action, this is
// expected to be done by the caller. In addition, it does not take note of the
// action result. The caller is expected to listen to this using a
// storage.ObserveFunc, which provides superior access to Helm storage writes.
func Install(ctx context.Context, config *helmaction.Configuration, obj *v2.HelmRelease,
	chrt *helmchart.Chart, vals helmchartutil.Values, opts ...InstallOption) (*helmrelease.Release, error) {
	install := newInstall(config, obj, opts)
	install.ForceConflicts = install.ServerSideApply // We always force conflicts on server-side apply.

	policy, err := crdPolicyOrDefault(obj.GetInstall().CRDs)
	if err != nil {
		return nil, err
	}
	if err := applyCRDs(config, policy, chrt, vals, install.ServerSideApply, setOriginVisitor(v2.GroupVersion.Group, obj.Namespace, obj.Name)); err != nil {
		return nil, fmt.Errorf("failed to apply CustomResourceDefinitions: %w", err)
	}

	rlsr, err := install.RunWithContext(ctx, chrt, vals.AsMap())
	if err != nil {
		return nil, err
	}
	return rlsr.(*helmrelease.Release), err
}

func newInstall(config *helmaction.Configuration, obj *v2.HelmRelease, opts []InstallOption) *helmaction.Install {
	install := helmaction.NewInstall(config)
	switch {
	case UseHelm3Defaults:
		install.ServerSideApply = false
	default:
		install.ServerSideApply = true
	}
	if ssa := obj.GetInstall().ServerSideApply; ssa != nil {
		install.ServerSideApply = *ssa
	}

	install.ReleaseName = release.ShortenName(obj.GetReleaseName())
	install.Namespace = obj.GetReleaseNamespace()
	install.Timeout = obj.GetInstall().GetTimeout(obj.GetTimeout()).Duration
	install.TakeOwnership = !obj.GetInstall().DisableTakeOwnership
	install.WaitStrategy = getWaitStrategy(obj.GetInstall())
	install.WaitForJobs = !obj.GetInstall().DisableWaitForJobs
	install.DisableHooks = obj.GetInstall().DisableHooks
	install.DisableOpenAPIValidation = obj.GetInstall().DisableOpenAPIValidation
	install.SkipSchemaValidation = obj.GetInstall().DisableSchemaValidation
	install.Replace = obj.GetInstall().Replace
	install.Devel = true
	install.SkipCRDs = true

	if obj.Spec.TargetNamespace != "" {
		install.CreateNamespace = obj.GetInstall().CreateNamespace
	}

	// If the user opted-in to allow DNS lookups, enable it.
	if allowDNS, _ := features.Enabled(features.AllowDNSLookups); allowDNS {
		install.EnableDNS = allowDNS
	}

	install.PostRenderer = postrender.BuildPostRenderers(obj)

	for _, opt := range opts {
		opt(install)
	}

	return install
}
