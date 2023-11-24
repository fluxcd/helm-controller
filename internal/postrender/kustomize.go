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

package postrender

import (
	"bytes"
	"encoding/json"
	"sync"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/resmap"
	kustypes "sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/filesys"

	"github.com/fluxcd/pkg/apis/kustomize"
)

// Kustomize is a Helm post-render plugin that runs Kustomize.
type Kustomize struct {
	// Patches is a list of patches to apply to the rendered manifests.
	Patches []kustomize.Patch
	// PatchesStrategicMerge is a list of strategic merge patches to apply to
	// the rendered manifests.
	PatchesStrategicMerge []apiextensionsv1.JSON
	// PatchesJSON6902 is a list of JSON patches to apply to the rendered
	// manifests.
	PatchesJSON6902 []kustomize.JSON6902Patch
	// Images is a list of images to replace in the rendered manifests.
	Images []kustomize.Image
}

func (k *Kustomize) Run(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error) {
	fs := filesys.MakeFsInMemory()
	cfg := kustypes.Kustomization{}
	cfg.APIVersion = kustypes.KustomizationVersion
	cfg.Kind = kustypes.KustomizationKind
	cfg.Images = adaptImages(k.Images)

	// Add rendered Helm output as input resource to the Kustomization.
	const input = "helm-output.yaml"
	cfg.Resources = append(cfg.Resources, input)
	if err := writeFile(fs, input, renderedManifests); err != nil {
		return nil, err
	}

	// Add patches.
	for _, m := range k.Patches {
		cfg.Patches = append(cfg.Patches, kustypes.Patch{
			Patch:  m.Patch,
			Target: adaptSelector(m.Target),
		})
	}

	// Add strategic merge patches.
	for _, m := range k.PatchesStrategicMerge {
		cfg.PatchesStrategicMerge = append(cfg.PatchesStrategicMerge, kustypes.PatchStrategicMerge(m.Raw))
	}

	// Add JSON 6902 patches.
	for i, m := range k.PatchesJSON6902 {
		patch, err := json.Marshal(m.Patch)
		if err != nil {
			return nil, err
		}
		cfg.PatchesJson6902 = append(cfg.PatchesJson6902, kustypes.Patch{
			Patch:  string(patch),
			Target: adaptSelector(&k.PatchesJSON6902[i].Target),
		})
	}

	// Write kustomization config to file.
	kustomization, err := json.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if err := writeToFile(fs, "kustomization.yaml", kustomization); err != nil {
		return nil, err
	}
	resMap, err := buildKustomization(fs, ".")
	if err != nil {
		return nil, err
	}
	yaml, err := resMap.AsYaml()
	if err != nil {
		return nil, err
	}
	return bytes.NewBuffer(yaml), nil
}

func writeToFile(fs filesys.FileSystem, path string, content []byte) error {
	helmOutput, err := fs.Create(path)
	if err != nil {
		return err
	}
	if _, err = helmOutput.Write(content); err != nil {
		return err
	}
	if err = helmOutput.Close(); err != nil {
		return err
	}
	return nil
}

func writeFile(fs filesys.FileSystem, path string, content *bytes.Buffer) error {
	helmOutput, err := fs.Create(path)
	if err != nil {
		return err
	}
	if _, err = content.WriteTo(helmOutput); err != nil {
		return err
	}
	if err = helmOutput.Close(); err != nil {
		return err
	}
	return nil
}

func adaptImages(images []kustomize.Image) (output []kustypes.Image) {
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

func adaptSelector(selector *kustomize.Selector) (output *kustypes.Selector) {
	if selector != nil {
		output = &kustypes.Selector{}
		output.Gvk.Group = selector.Group
		output.Gvk.Kind = selector.Kind
		output.Gvk.Version = selector.Version
		output.Name = selector.Name
		output.Namespace = selector.Namespace
		output.LabelSelector = selector.LabelSelector
		output.AnnotationSelector = selector.AnnotationSelector
	}
	return
}

// TODO: remove mutex when kustomize fixes the concurrent map read/write panic
var kustomizeRenderMutex sync.Mutex

// buildKustomization wraps krusty.MakeKustomizer with the following settings:
// - load files from outside the kustomization.yaml root
// - disable plugins except for the builtin ones
func buildKustomization(fs filesys.FileSystem, dirPath string) (resmap.ResMap, error) {
	// Temporary workaround for concurrent map read and map write bug
	// https://github.com/kubernetes-sigs/kustomize/issues/3659
	kustomizeRenderMutex.Lock()
	defer kustomizeRenderMutex.Unlock()

	buildOptions := &krusty.Options{
		LoadRestrictions: kustypes.LoadRestrictionsNone,
		PluginConfig:     kustypes.DisabledPluginConfig(),
	}

	k := krusty.MakeKustomizer(buildOptions)
	return k.Run(fs, dirPath)
}
