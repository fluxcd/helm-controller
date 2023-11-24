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
	"bytes"
	"testing"

	goyaml "gopkg.in/yaml.v2"
	"sigs.k8s.io/yaml"
)

func TestSortMapSlice(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]interface{}
		want  map[string]interface{}
	}{
		{
			name:  "empty map",
			input: map[string]interface{}{},
			want:  map[string]interface{}{},
		},
		{
			name: "flat map",
			input: map[string]interface{}{
				"b": "value-b",
				"a": "value-a",
				"c": "value-c",
			},
			want: map[string]interface{}{
				"a": "value-a",
				"b": "value-b",
				"c": "value-c",
			},
		},
		{
			name: "nested map",
			input: map[string]interface{}{
				"b": "value-b",
				"a": "value-a",
				"c": map[string]interface{}{
					"z": "value-z",
					"y": "value-y",
				},
			},
			want: map[string]interface{}{
				"a": "value-a",
				"b": "value-b",
				"c": map[string]interface{}{
					"y": "value-y",
					"z": "value-z",
				},
			},
		},
		{
			name: "map with slices",
			input: map[string]interface{}{
				"b": []interface{}{"apple", "banana", "cherry"},
				"a": []interface{}{"orange", "grape"},
				"c": []interface{}{"strawberry"},
			},
			want: map[string]interface{}{
				"a": []interface{}{"orange", "grape"},
				"b": []interface{}{"apple", "banana", "cherry"},
				"c": []interface{}{"strawberry"},
			},
		},
		{
			name: "map with mixed data types",
			input: map[string]interface{}{
				"b": 50,
				"a": "value-a",
				"c": []interface{}{"strawberry", "banana"},
				"d": map[string]interface{}{
					"x": true,
					"y": 123,
				},
			},
			want: map[string]interface{}{
				"a": "value-a",
				"b": 50,
				"c": []interface{}{"strawberry", "banana"},
				"d": map[string]interface{}{
					"x": true,
					"y": 123,
				},
			},
		},
		{
			name: "map with complex structure",
			input: map[string]interface{}{
				"a": map[string]interface{}{
					"c": "value-c",
					"b": "value-b",
					"a": "value-a",
				},
				"b": "value-b",
				"c": map[string]interface{}{
					"z": map[string]interface{}{
						"a": "value-a",
						"b": "value-b",
						"c": "value-c",
					},
					"y": "value-y",
				},
				"d": map[string]interface{}{
					"q": "value-q",
					"p": "value-p",
					"r": "value-r",
				},
				"e": []interface{}{"strawberry", "banana"},
			},
			want: map[string]interface{}{
				"a": map[string]interface{}{
					"a": "value-a",
					"b": "value-b",
					"c": "value-c",
				},
				"b": "value-b",
				"c": map[string]interface{}{
					"y": "value-y",
					"z": map[string]interface{}{
						"a": "value-a",
						"b": "value-b",
						"c": "value-c",
					},
				},
				"d": map[string]interface{}{
					"p": "value-p",
					"q": "value-q",
					"r": "value-r",
				},
				"e": []interface{}{"strawberry", "banana"},
			},
		},
		{
			name: "map with empty slices and maps",
			input: map[string]interface{}{
				"b": []interface{}{},
				"a": map[string]interface{}{},
			},
			want: map[string]interface{}{
				"a": map[string]interface{}{},
				"b": []interface{}{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := yaml.JSONObjectToYAMLObject(tt.input)
			SortMapSlice(input)

			expect, err := goyaml.Marshal(input)
			if err != nil {
				t.Fatalf("error marshalling output: %v", err)
			}
			actual, err := goyaml.Marshal(tt.want)
			if err != nil {
				t.Fatalf("error marshalling want: %v", err)
			}

			if !bytes.Equal(expect, actual) {
				t.Errorf("SortMapSlice() = %s, want %s", expect, actual)
			}
		})
	}
}
