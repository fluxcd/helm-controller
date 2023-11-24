/*
Copyright 2023 The Flux authors

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

package yaml

import (
	"io"

	goyaml "gopkg.in/yaml.v2"
	"sigs.k8s.io/yaml"
)

// PreEncoder allows for pre-processing of the YAML data before encoding.
type PreEncoder func(goyaml.MapSlice)

// Encode encodes the given data to YAML format and writes it to the provided
// io.Write, without going through a byte representation (unlike
// sigs.k8s.io/yaml#Unmarshal).
//
// It optionally takes one or more PreEncoder functions that allow
// for pre-processing of the data before encoding, such as sorting the data.
//
// It returns an error if the data cannot be encoded.
func Encode(w io.Writer, data map[string]interface{}, pe ...PreEncoder) error {
	ms := yaml.JSONObjectToYAMLObject(data)
	for _, m := range pe {
		m(ms)
	}
	return goyaml.NewEncoder(w).Encode(ms)
}
