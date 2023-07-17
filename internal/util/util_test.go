/*
Copyright 2020 The Flux authors

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

package util

import (
	"testing"

	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
)

func TestValuesChecksum(t *testing.T) {
	tests := []struct {
		name   string
		values chartutil.Values
		want   string
	}{
		{
			name:   "empty",
			values: chartutil.Values{},
			want:   "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		},
		{
			name: "value map",
			values: chartutil.Values{
				"foo": "bar",
				"baz": map[string]string{
					"cool": "stuff",
				},
			},
			want: "7d487b668ca37fe68c42adfc06fa4d0e74443afd",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValuesChecksum(tt.values); got != tt.want {
				t.Errorf("ValuesChecksum() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrderedValuesChecksum(t *testing.T) {
	tests := []struct {
		name   string
		values chartutil.Values
		want   string
	}{
		{
			name:   "empty",
			values: chartutil.Values{},
			want:   "da39a3ee5e6b4b0d3255bfef95601890afd80709",
		},
		{
			name: "value map",
			values: chartutil.Values{
				"foo": "bar",
				"baz": map[string]string{
					"fruit": "apple",
					"cool":  "stuff",
				},
			},
			want: "dfd6589332e4d2da5df7bcbf5885f406f08b58ee",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := OrderedValuesChecksum(tt.values); got != tt.want {
				t.Errorf("OrderedValuesChecksum() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReleaseRevision(t *testing.T) {
	var rel *release.Release
	if rev := ReleaseRevision(rel); rev != 0 {
		t.Fatalf("ReleaseRevision() = %v, want %v", rev, 0)
	}
	rel = &release.Release{Version: 1}
	if rev := ReleaseRevision(rel); rev != 1 {
		t.Fatalf("ReleaseRevision() = %v, want %v", rev, 1)
	}
}
