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

package postrender

import (
	"encoding/json"

	"github.com/opencontainers/go-digest"
	helmpostrender "helm.sh/helm/v3/pkg/postrender"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

// BuildPostRenderers creates the post-renderer instances from a HelmRelease
// and combines them into a single Combined post renderer.
func BuildPostRenderers(rel *v2.HelmRelease) helmpostrender.PostRenderer {
	if rel == nil {
		return nil
	}
	renderers := make([]helmpostrender.PostRenderer, 0)
	for _, r := range rel.Spec.PostRenderers {
		if r.Kustomize != nil {
			renderers = append(renderers, &Kustomize{
				Patches: r.Kustomize.Patches,
				Images:  r.Kustomize.Images,
			})
		}
	}
	renderers = append(renderers, NewOriginLabels(v2.GroupVersion.Group, rel.Namespace, rel.Name))
	if len(renderers) == 0 {
		return nil
	}
	return NewCombined(renderers...)
}

func Digest(algo digest.Algorithm, postrenders []v2.PostRenderer) digest.Digest {
	digester := algo.Digester()
	enc := json.NewEncoder(digester.Hash())
	if err := enc.Encode(postrenders); err != nil {
		return ""
	}
	return digester.Digest()
}
