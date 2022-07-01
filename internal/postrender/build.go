package postrender

import (
	helmpostrender "helm.sh/helm/v3/pkg/postrender"

	helmv2 "github.com/fluxcd/helm-controller/api/v2beta2"
)

// BuildPostRenderers creates the post renderer instances from a HelmRelease
// and combines them into a single Combined post renderer.
func BuildPostRenderers(rel *helmv2.HelmRelease) (helmpostrender.PostRenderer, error) {
	if rel == nil {
		return nil, nil
	}
	renderers := make([]helmpostrender.PostRenderer, 0)
	for _, r := range rel.Spec.PostRenderers {
		if r.Kustomize != nil {
			renderers = append(renderers, &Kustomize{
				Patches:               r.Kustomize.Patches,
				PatchesStrategicMerge: r.Kustomize.PatchesStrategicMerge,
				PatchesJSON6902:       r.Kustomize.PatchesJSON6902,
				Images:                r.Kustomize.Images,
			})
		}
	}
	renderers = append(renderers, NewOriginLabels(helmv2.GroupVersion.Group, rel.Namespace, rel.Name))
	if len(renderers) == 0 {
		return nil, nil
	}
	return NewCombined(renderers...), nil
}
