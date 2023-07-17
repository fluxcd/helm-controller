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

func TestDigestValues(t *testing.T) {
	tests := []struct {
		name   string
		algo   digest.Algorithm
		values chartutil.Values
		want   digest.Digest
	}{
		{
			name:   "empty",
			algo:   digest.SHA256,
			values: chartutil.Values{},
			want:   "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name:   "nil",
			algo:   digest.SHA256,
			values: nil,
			want:   "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		},
		{
			name: "value map",
			algo: digest.SHA256,
			values: chartutil.Values{
				"replicas": 3,
				"image": map[string]interface{}{
					"tag":        "latest",
					"repository": "nginx",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"protocol": "TCP",
						"port":     8080,
					},
					map[string]interface{}{
						"port":     9090,
						"protocol": "UDP",
					},
				},
			},
			want: "sha256:fcdc2b0de1581a3633ada4afee3f918f6eaa5b5ab38c3fef03d5b48d3f85d9f6",
		},
		{
			name: "value map in different order",
			algo: digest.SHA256,
			values: chartutil.Values{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "latest",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"port":     8080,
						"protocol": "TCP",
					},
					map[string]interface{}{
						"port":     9090,
						"protocol": "UDP",
					},
				},
				"replicas": 3,
			},
			want: "sha256:fcdc2b0de1581a3633ada4afee3f918f6eaa5b5ab38c3fef03d5b48d3f85d9f6",
		},
		{
			// Explicit test for something that does not work with sigs.k8s.io/yaml.
			// See: https://go.dev/play/p/KRyfK9ZobZx
			name: "values map with numeric keys",
			algo: digest.SHA256,
			values: chartutil.Values{
				"replicas": 3,
				"test": map[string]interface{}{
					"632bd80235a05f4192aefade": "value1",
					"632bd80ddf416cf32fd50679": "value2",
					"632bd817c559818a52307da2": "value3",
					"632bd82398e71231a98004b6": "value4",
				},
			},
			want: "sha256:8a980fcbeadd6f05818f07e8aec14070c22250ca3d96af1fcd5f93b3e85b4d70",
		},
		{
			name: "values map with numeric keys in different order",
			algo: digest.SHA256,
			values: chartutil.Values{
				"test": map[string]interface{}{
					"632bd82398e71231a98004b6": "value4",
					"632bd817c559818a52307da2": "value3",
					"632bd80ddf416cf32fd50679": "value2",
					"632bd80235a05f4192aefade": "value1",
				},
				"replicas": 3,
			},
			want: "sha256:8a980fcbeadd6f05818f07e8aec14070c22250ca3d96af1fcd5f93b3e85b4d70",
		},
		{
			name: "using different algorithm",
			algo: digest.SHA512,
			values: chartutil.Values{
				"foo": "bar",
				"baz": map[string]interface{}{
					"cool": "stuff",
				},
			},
			want: "sha512:b5f9cd4855ca3b08afd602557f373069b1732ce2e6d52341481b0d38f1938452e9d7759ab177c66699962b592f20ceded03eea3cd405d8670578c47842e2c550",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DigestValues(tt.algo, tt.values); got != tt.want {
				t.Errorf("DigestValues() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestVerifyValues(t *testing.T) {
	tests := []struct {
		name   string
		digest digest.Digest
		values chartutil.Values
		want   bool
	}{
		{
			name:   "empty values",
			digest: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			values: chartutil.Values{},
			want:   true,
		},
		{
			name:   "nil values",
			digest: "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
			values: nil,
			want:   true,
		},
		{
			name:   "empty digest",
			digest: "",
			want:   false,
		},
		{
			name:   "invalid digest",
			digest: "sha512:invalid",
			values: nil,
			want:   false,
		},
		{
			name:   "matching values",
			digest: "sha256:fcdc2b0de1581a3633ada4afee3f918f6eaa5b5ab38c3fef03d5b48d3f85d9f6",
			values: chartutil.Values{
				"image": map[string]interface{}{
					"repository": "nginx",
					"tag":        "latest",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"port":     8080,
						"protocol": "TCP",
					},
					map[string]interface{}{
						"port":     9090,
						"protocol": "UDP",
					},
				},
				"replicas": 3,
			},
			want: true,
		},
		{
			name:   "matching values in different order",
			digest: "sha256:fcdc2b0de1581a3633ada4afee3f918f6eaa5b5ab38c3fef03d5b48d3f85d9f6",
			values: chartutil.Values{
				"replicas": 3,
				"image": map[string]interface{}{
					"tag":        "latest",
					"repository": "nginx",
				},
				"ports": []interface{}{
					map[string]interface{}{
						"protocol": "TCP",
						"port":     8080,
					},
					map[string]interface{}{
						"port":     9090,
						"protocol": "UDP",
					},
				},
			},
			want: true,
		},
		{
			name:   "matching values with numeric keys",
			digest: "sha256:8a980fcbeadd6f05818f07e8aec14070c22250ca3d96af1fcd5f93b3e85b4d70",
			values: chartutil.Values{
				"replicas": 3,
				"test": map[string]interface{}{
					"632bd80235a05f4192aefade": "value1",
					"632bd80ddf416cf32fd50679": "value2",
					"632bd817c559818a52307da2": "value3",
					"632bd82398e71231a98004b6": "value4",
				},
			},
			want: true,
		},
		{
			name:   "mismatching values",
			digest: "sha256:3f3641788a2d4abda3534eaa90c90b54916e4c6e3a5b2e1b24758b7bfa701ecd",
			values: chartutil.Values{
				"foo": "bar",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifyValues(tt.digest, tt.values); got != tt.want {
				t.Errorf("VerifyValues() = %v, want %v", got, tt.want)
			}
		})
	}
}
