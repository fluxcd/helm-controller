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
	"reflect"
	"testing"

	goyaml "gopkg.in/yaml.v2"
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

func TestSortMapSlice(t *testing.T) {
	tests := []struct {
		name     string
		input    goyaml.MapSlice
		expected goyaml.MapSlice
	}{
		{
			name: "Simple case",
			input: goyaml.MapSlice{
				{Key: "b", Value: 2},
				{Key: "a", Value: 1},
			},
			expected: goyaml.MapSlice{
				{Key: "a", Value: 1},
				{Key: "b", Value: 2},
			},
		},
		{
			name: "Nested MapSlice",
			input: goyaml.MapSlice{
				{Key: "b", Value: 2},
				{Key: "a", Value: 1},
				{Key: "c", Value: goyaml.MapSlice{
					{Key: "d", Value: 4},
					{Key: "e", Value: 5},
				}},
			},
			expected: goyaml.MapSlice{
				{Key: "a", Value: 1},
				{Key: "b", Value: 2},
				{Key: "c", Value: goyaml.MapSlice{
					{Key: "d", Value: 4},
					{Key: "e", Value: 5},
				}},
			},
		},
		{
			name:     "Empty MapSlice",
			input:    goyaml.MapSlice{},
			expected: goyaml.MapSlice{},
		},
		{
			name: "Single element",
			input: goyaml.MapSlice{
				{Key: "a", Value: 1},
			},
			expected: goyaml.MapSlice{
				{Key: "a", Value: 1},
			},
		},
		{
			name: "Already sorted",
			input: goyaml.MapSlice{
				{Key: "a", Value: 1},
				{Key: "b", Value: 2},
				{Key: "c", Value: 3},
			},
			expected: goyaml.MapSlice{
				{Key: "a", Value: 1},
				{Key: "b", Value: 2},
				{Key: "c", Value: 3},
			},
		},

		{
			name: "Complex Case",
			input: goyaml.MapSlice{
				{Key: "b", Value: 2},
				{Key: "a", Value: map[interface{}]interface{}{
					"d": []interface{}{4, 5},
					"c": 3,
				}},
				{Key: "c", Value: goyaml.MapSlice{
					{Key: "f", Value: 6},
					{Key: "e", Value: goyaml.MapSlice{
						{Key: "h", Value: 8},
						{Key: "g", Value: 7},
					}},
				}},
			},
			expected: goyaml.MapSlice{
				{Key: "a", Value: map[interface{}]interface{}{
					"c": 3,
					"d": []interface{}{4, 5},
				}},
				{Key: "b", Value: 2},
				{Key: "c", Value: goyaml.MapSlice{
					{Key: "e", Value: goyaml.MapSlice{
						{Key: "g", Value: 7},
						{Key: "h", Value: 8},
					}},
					{Key: "f", Value: 6},
				}},
			},
		},
		{
			name: "Map slice in slice",
			input: goyaml.MapSlice{
				{Key: "b", Value: 2},
				{Key: "a", Value: []interface{}{
					map[interface{}]interface{}{
						"d": 4,
						"c": 3,
					},
					1,
				}},
			},
			expected: goyaml.MapSlice{
				{Key: "a", Value: []interface{}{
					map[interface{}]interface{}{
						"c": 3,
						"d": 4,
					},
					1,
				}},
				{Key: "b", Value: 2},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			SortMapSlice(test.input)
			if !reflect.DeepEqual(test.input, test.expected) {
				t.Errorf("Expected %v, got %v", test.expected, test.input)
			}
		})
	}
}
