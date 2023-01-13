package runner

import (
	"bytes"
	"io"
	"strings"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
	goyaml "gopkg.in/yaml.v3"
)

func newPostRendererYamlFlatten(release *v2.HelmRelease) *postRendererYamlFlatten {
	return &postRendererYamlFlatten{
		name:      release.ObjectMeta.Name,
		namespace: release.ObjectMeta.Namespace,
	}
}

type postRendererYamlFlatten struct {
	name      string
	namespace string
}

func (k *postRendererYamlFlatten) Run(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error) {
	decoder := goyaml.NewDecoder(renderedManifests)

	var res []string
	for {
		var value interface{}
		err := decoder.Decode(&value)
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		valueBytes, err := goyaml.Marshal(value)
		if err != nil {
			return nil, err
		}
		res = append(res, string(valueBytes))
	}

	concattedFlattenedYaml := strings.Join(res, "---\n")
	return bytes.NewBufferString(concattedFlattenedYaml), nil
}
