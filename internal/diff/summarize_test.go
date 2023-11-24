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

package diff

import (
	"testing"

	extjsondiff "github.com/wI2L/jsondiff"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/fluxcd/pkg/ssa/jsondiff"
)

func TestSummarizeDiffSet(t *testing.T) {
	diffSet := jsondiff.DiffSet{
		&jsondiff.Diff{
			GroupVersionKind: schema.GroupVersionKind{
				Kind: "ConfigMap",
			},
			Namespace: "namespace-1",
			Name:      "config",
			Type:      jsondiff.DiffTypeNone,
		},
		&jsondiff.Diff{
			GroupVersionKind: schema.GroupVersionKind{
				Kind: "Secret",
			},
			Namespace: "namespace-x",
			Name:      "naughty",
			Type:      jsondiff.DiffTypeCreate,
		},
		&jsondiff.Diff{
			GroupVersionKind: schema.GroupVersionKind{
				Kind: "StatefulSet",
			},
			Namespace: "default",
			Name:      "hello-world",
			Type:      jsondiff.DiffTypeExclude,
		},
		&jsondiff.Diff{
			GroupVersionKind: schema.GroupVersionKind{
				Kind: "Deployment",
			},
			Namespace: "tenant-y",
			Name:      "touched-me",
			Type:      jsondiff.DiffTypeUpdate,
			Patch: extjsondiff.Patch{
				{Type: extjsondiff.OperationAdd},
				{Type: extjsondiff.OperationReplace},
				{Type: extjsondiff.OperationReplace},
				{Type: extjsondiff.OperationReplace},
				{Type: extjsondiff.OperationRemove},
				{Type: extjsondiff.OperationRemove},
			},
		},
	}

	tests := []struct {
		name    string
		include []jsondiff.DiffType
		want    string
	}{
		{
			name:    "default",
			include: nil,
			want: `Secret/namespace-x/naughty removed
StatefulSet/default/hello-world excluded
Deployment/tenant-y/touched-me changed (1 additions, 3 changes, 2 removals)`,
		},
		{
			name: "include unchanged",
			include: []jsondiff.DiffType{
				jsondiff.DiffTypeNone,
			},
			want: "ConfigMap/namespace-1/config unchanged",
		},
		{
			name: "include removed",
			include: []jsondiff.DiffType{
				jsondiff.DiffTypeCreate,
			},
			want: "Secret/namespace-x/naughty removed",
		},
		{
			name: "include excluded",
			include: []jsondiff.DiffType{
				jsondiff.DiffTypeExclude,
			},
			want: "StatefulSet/default/hello-world excluded",
		},
		{
			name: "include changed",
			include: []jsondiff.DiffType{
				jsondiff.DiffTypeUpdate,
			},
			want: "Deployment/tenant-y/touched-me changed (1 additions, 3 changes, 2 removals)",
		},
		{
			name: "include multiple types",
			include: []jsondiff.DiffType{
				jsondiff.DiffTypeNone,
				jsondiff.DiffTypeUpdate,
			},
			want: `ConfigMap/namespace-1/config unchanged
Deployment/tenant-y/touched-me changed (1 additions, 3 changes, 2 removals)`,
		},
		{
			name:    "empty set",
			include: []jsondiff.DiffType{},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SummarizeDiffSet(diffSet, tt.include...)
			if got != tt.want {
				t.Errorf("SummarizeDiffSet() =\n\n%v\n\nwant\n\n%v", got, tt.want)
			}
		})
	}
}

func TestSummarizeDiffSetBrief(t *testing.T) {
	diffSet := jsondiff.DiffSet{
		&jsondiff.Diff{Type: jsondiff.DiffTypeCreate},
		&jsondiff.Diff{Type: jsondiff.DiffTypeUpdate},
		&jsondiff.Diff{Type: jsondiff.DiffTypeExclude},
		&jsondiff.Diff{Type: jsondiff.DiffTypeNone},
		&jsondiff.Diff{Type: jsondiff.DiffTypeNone},
	}

	tests := []struct {
		name    string
		include []jsondiff.DiffType
		want    string
	}{
		{
			name:    "default include",
			include: nil,
			want:    "removed: 1, changed: 1, excluded: 1",
		},
		{
			name: "include create and update",
			include: []jsondiff.DiffType{
				jsondiff.DiffTypeCreate,
				jsondiff.DiffTypeUpdate,
			},
			want: "removed: 1, changed: 1",
		},
		{
			name: "include all types",
			include: []jsondiff.DiffType{
				jsondiff.DiffTypeCreate,
				jsondiff.DiffTypeUpdate,
				jsondiff.DiffTypeExclude,
				jsondiff.DiffTypeNone,
			},
			want: "removed: 1, changed: 1, excluded: 1, unchanged: 2",
		},
		{
			name:    "include none",
			include: []jsondiff.DiffType{},
			want:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SummarizeDiffSetBrief(diffSet, tt.include...)
			if got != tt.want {
				t.Errorf("SummarizeDiffSetBrief() = %v, want %v", got, tt.want)
			}
		})
	}
}
