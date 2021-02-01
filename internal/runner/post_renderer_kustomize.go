package runner

import (
	"bytes"
	"encoding/json"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
	"sigs.k8s.io/kustomize/api/filesys"
	"sigs.k8s.io/kustomize/api/konfig"
	"sigs.k8s.io/kustomize/api/krusty"
	kustypes "sigs.k8s.io/kustomize/api/types"
)

type postRendererKustomize struct {
	spec *v2.Kustomize
}

func newPostRendererKustomize(spec *v2.Kustomize) *postRendererKustomize {
	return &postRendererKustomize{
		spec: spec,
	}
}

func writeToFile(fs filesys.FileSystem, path string, content []byte) error {
	helmOutput, err := fs.Create(path)
	if err != nil {
		return err
	}
	helmOutput.Write(content)
	if err := helmOutput.Close(); err != nil {
		return err
	}
	return nil
}

func writeFile(fs filesys.FileSystem, path string, content *bytes.Buffer) error {
	helmOutput, err := fs.Create(path)
	if err != nil {
		return err
	}
	content.WriteTo(helmOutput)
	if err := helmOutput.Close(); err != nil {
		return err
	}
	return nil
}

func adaptImages(images []v2.Image) (output []kustypes.Image) {
	for _, image := range images {
		output = append(output, kustypes.Image{
			Name:    image.Name,
			NewName: image.NewName,
			NewTag:  image.NewTag,
			Digest:  image.Digest,
		})
	}
	return
}

func adaptSelector(selector *v2.Selector) (output *kustypes.Selector) {
	if selector != nil {
		output = &kustypes.Selector{}
		output.Gvk.Group = selector.Gvk.Group
		output.Gvk.Kind = selector.Gvk.Kind
		output.Gvk.Version = selector.Gvk.Version
		output.Name = selector.Name
		output.Namespace = selector.Namespace
		output.LabelSelector = selector.LabelSelector
		output.AnnotationSelector = selector.AnnotationSelector
	}
	return
}

func (k *postRendererKustomize) Run(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error) {
	buildOptions := &krusty.Options{
		UseKyaml:               false,
		DoLegacyResourceSort:   true,
		LoadRestrictions:       kustypes.LoadRestrictionsNone,
		AddManagedbyLabel:      false,
		DoPrune:                false,
		PluginConfig:           konfig.DisabledPluginConfig(),
		AllowResourceIdChanges: false,
	}
	fs := filesys.MakeFsInMemory()
	cfg := kustypes.Kustomization{}
	cfg.APIVersion = "kustomize.config.k8s.io/v1beta1"
	cfg.Kind = "Kustomization"
	cfg.Images = adaptImages(k.spec.Images)
	// add rendered Helm output as input resource to the Kustomization.
	const input = "helm-output.yaml"
	cfg.Resources = append(cfg.Resources, input)
	if err := writeFile(fs, input, renderedManifests); err != nil {
		return nil, err
	}
	// add strategic merge patches
	for _, m := range k.spec.PatchesStrategicMerge {
		patch, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		cfg.PatchesStrategicMerge = append(cfg.PatchesStrategicMerge, kustypes.PatchStrategicMerge(patch))
	}
	// add JSON patches
	for _, m := range k.spec.PatchesJSON6902 {
		patch, err := json.Marshal(m.Patch)
		if err != nil {
			return nil, err
		}
		cfg.PatchesJson6902 = append(cfg.PatchesJson6902, kustypes.Patch{
			Patch:  string(patch),
			Target: adaptSelector(&m.Target),
		})
	}
	kustomization, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := writeToFile(fs, "kustomization.yaml", kustomization); err != nil {
		return nil, err
	}
	kustomizer := krusty.MakeKustomizer(fs, buildOptions)
	resMap, err := kustomizer.Run(".")
	if err != nil {
		return nil, err
	}
	yaml, err := resMap.AsYaml()
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(yaml), nil
}
