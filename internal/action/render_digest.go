/*
Copyright 2026 The Flux authors

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

// PROOF OF CONCEPT: this file exists to size the change proposed in
// https://github.com/fluxcd/helm-controller/issues/1583. It is not meant to
// be merged as-is.
package action

import (
	"context"

	helmaction "helm.sh/helm/v4/pkg/action"
	helmchartutil "helm.sh/helm/v4/pkg/chart/common"
	helmchart "helm.sh/helm/v4/pkg/chart/v2"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/digest"
)

// RenderTemplateDigest performs a server-side dry-run Helm upgrade to obtain
// a fresh render of the chart, including live evaluation of the Helm
// `lookup` function against the current cluster state, and returns the
// digest of the resulting release manifest.
//
// Only Release.Manifest is digested, hook manifests (Release.Hooks) are
// intentionally excluded, matching what Diff already compares the cluster
// state against.
func RenderTemplateDigest(ctx context.Context, config *helmaction.Configuration, obj *v2.HelmRelease,
	chrt *helmchart.Chart, vals helmchartutil.Values) (string, error) {
	rls, err := Upgrade(ctx, config, obj, chrt, vals, func(upgrade *helmaction.Upgrade) {
		upgrade.DryRunStrategy = helmaction.DryRunServer
	})
	if err != nil {
		return "", err
	}
	return digest.Canonical.FromString(rls.Manifest).String(), nil
}
