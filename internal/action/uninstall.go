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

	helmaction "github.com/matheuscscp/helm/pkg/action"
	helmrelease "github.com/matheuscscp/helm/pkg/release"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

// UninstallOption can be used to modify Helm's action.Uninstall after the
// instructions from the v2.HelmRelease have been applied. This is for
// example useful to enable the dry-run setting as a CLI.
type UninstallOption func(cfg *helmaction.Uninstall)

// Uninstall runs the Helm uninstall action with the provided config, using the
// v2.HelmReleaseSpec of the given object to determine the target release
// and uninstall configuration.
//
// It does not determine if there is a desire to perform the action, this is
// expected to be done by the caller. In addition, it does not take note of the
// action result. The caller is expected to listen to this using a
// storage.ObserveFunc, which provides superior access to Helm storage writes.
func Uninstall(_ context.Context, config *helmaction.Configuration, obj *v2.HelmRelease, releaseName string, opts ...UninstallOption) (*helmrelease.UninstallReleaseResponse, error) {
	uninstall := newUninstall(config, obj, opts)
	return uninstall.Run(releaseName)
}

func newUninstall(config *helmaction.Configuration, obj *v2.HelmRelease, opts []UninstallOption) *helmaction.Uninstall {
	uninstall := helmaction.NewUninstall(config)

	uninstall.Timeout = obj.GetUninstall().GetTimeout(obj.GetTimeout()).Duration
	uninstall.DisableHooks = obj.GetUninstall().DisableHooks
	uninstall.KeepHistory = obj.GetUninstall().KeepHistory
	uninstall.Wait = !obj.GetUninstall().DisableWait
	uninstall.DeletionPropagation = obj.GetUninstall().GetDeletionPropagation()

	for _, opt := range opts {
		opt(uninstall)
	}

	return uninstall
}
