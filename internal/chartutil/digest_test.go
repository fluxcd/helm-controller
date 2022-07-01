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

package chartutil

import (
	"testing"

	"github.com/opencontainers/go-digest"
	"helm.sh/helm/v3/pkg/chartutil"
)

const testDigestAlgo = digest.SHA256

func TestDigestValues(t *testing.T) {
	tests := []struct {
		name   string
		values chartutil.Values
		want   digest.Digest
	}{
		{
			name:   "empty",
			values: chartutil.Values{},
			want:   "sha256:ca3d163bab055381827226140568f3bef7eaac187cebd76878e0b63e9e442356",
		},
		{
			name: "value map",
			values: chartutil.Values{
				"foo": "bar",
				"baz": map[string]string{
					"cool": "stuff",
				},
			},
			want: "sha256:3f3641788a2d4abda3534eaa90c90b54916e4c6e3a5b2e1b24758b7bfa701ecd",
		},
		{
			name: "value map in different order",
			values: chartutil.Values{
				"baz": map[string]string{
					"cool": "stuff",
				},
				"foo": "bar",
			},
			want: "sha256:3f3641788a2d4abda3534eaa90c90b54916e4c6e3a5b2e1b24758b7bfa701ecd",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DigestValues(testDigestAlgo, tt.values); got != tt.want {
				t.Errorf("DigestValues() = %v, want %v", got, tt.want)
			}
		})
	}
}
