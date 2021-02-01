package runner

import (
	"bytes"

	"helm.sh/helm/v3/pkg/postrender"
)

// combinedPostRenderer, a collection of Helm PostRenders which are
// invoked in the order of insertion.
type combinedPostRenderer struct {
	renderers []postrender.PostRenderer
}

func newCombinedPostRenderer() combinedPostRenderer {
	return combinedPostRenderer{
		renderers: make([]postrender.PostRenderer, 0),
	}
}

func (c *combinedPostRenderer) addRenderer(renderer postrender.PostRenderer) {
	c.renderers = append(c.renderers, renderer)
}

func (c *combinedPostRenderer) Run(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error) {
	var result *bytes.Buffer = renderedManifests
	for _, renderer := range c.renderers {
		result, err = renderer.Run(result)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
