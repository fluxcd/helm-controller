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
	"os"
	"testing"

	"sigs.k8s.io/yaml"
)

func TestEncode(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]interface{}
		preEncoders []PreEncoder
		want        []byte
	}{
		{
			name:  "empty map",
			input: map[string]interface{}{},
			want: []byte(`{}
`),
		},
		{
			name: "simple values",
			input: map[string]interface{}{
				"replicaCount": 3,
			},
			want: []byte(`replicaCount: 3
`),
		},
		{
			name: "with pre-encoder",
			input: map[string]interface{}{
				"replicaCount": 3,
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "latest",
				},
				"port": 8080,
			},
			preEncoders: []PreEncoder{SortMapSlice},
			want: []byte(`image:
  repository: nginx
  tag: latest
port: 8080
replicaCount: 3
`),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var actual bytes.Buffer
			err := Encode(&actual, tt.input, tt.preEncoders...)
			if err != nil {
				t.Fatalf("error encoding: %v", err)
			}

			if !bytes.Equal(actual.Bytes(), tt.want) {
				t.Errorf("Encode() = %v, want: %s", actual.String(), tt.want)
			}
		})
	}
}

func BenchmarkEncode(b *testing.B) {
	// Test against the values.yaml from the kube-prometheus-stack chart, which
	// is a fairly large file.
	v, err := os.ReadFile("testdata/values.yaml")
	if err != nil {
		b.Fatalf("error reading testdata: %v", err)
	}

	var data map[string]interface{}
	if err = yaml.Unmarshal(v, &data); err != nil {
		b.Fatalf("error unmarshalling testdata: %v", err)
	}

	b.Run("EncodeWithSort", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			Encode(bytes.NewBuffer(nil), data, SortMapSlice)
		}
	})

	b.Run("SigYAMLMarshal", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			yaml.Marshal(data)
		}
	})
}
