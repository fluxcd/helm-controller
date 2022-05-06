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

	"helm.sh/helm/v3/pkg/postrender"
)

// Combined is a collection of Helm PostRenders which are
// invoked in the order of insertion.
type Combined struct {
	renderers []postrender.PostRenderer
}

func NewCombined(renderer ...postrender.PostRenderer) *Combined {
	pr := make([]postrender.PostRenderer, 0)
	pr = append(pr, renderer...)
	return &Combined{
		renderers: pr,
	}
}

func (c *Combined) Run(renderedManifests *bytes.Buffer) (modifiedManifests *bytes.Buffer, err error) {
	var result = renderedManifests
	for _, renderer := range c.renderers {
		result, err = renderer.Run(result)
		if err != nil {
			return nil, err
		}
	}
	return result, nil
}
