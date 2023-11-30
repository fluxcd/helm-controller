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
	"fmt"
	"strings"

	extjsondiff "github.com/wI2L/jsondiff"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/fluxcd/pkg/ssa/jsondiff"
)

// DefaultDiffTypes is the default set of jsondiff.DiffType types to include in
// summaries.
var DefaultDiffTypes = []jsondiff.DiffType{
	jsondiff.DiffTypeCreate,
	jsondiff.DiffTypeUpdate,
	jsondiff.DiffTypeExclude,
}

// SummarizeDiffSet returns a summary of the given DiffSet, including only
// the given jsondiff.DiffType types. If no types are given, the
// DefaultDiffTypes set is used.
//
// The summary is a string with one line per Diff, in the format:
// `Kind/namespace/name: <summary>`
//
// Where summary is one of:
//
//   - unchanged
//   - removed
//   - excluded
//   - changed (x added, y changed, z removed)
//
// For example:
//
//	Deployment/default/hello-world changed (1 added, 1 changed, 1 removed)
//	Deployment/default/hello-world2 removed
//	Deployment/default/hello-world3 excluded
//	Deployment/default/hello-world4 unchanged
func SummarizeDiffSet(set jsondiff.DiffSet, include ...jsondiff.DiffType) string {
	if include == nil {
		include = DefaultDiffTypes
	}

	var summary strings.Builder
	for _, diff := range set {
		if diff == nil || !typeInSlice(diff.Type, include) {
			continue
		}

		switch diff.Type {
		case jsondiff.DiffTypeNone:
			writeResourceName(diff.DesiredObject, &summary)
			summary.WriteString(" unchanged\n")
		case jsondiff.DiffTypeCreate:
			writeResourceName(diff.DesiredObject, &summary)
			summary.WriteString(" removed\n")
		case jsondiff.DiffTypeExclude:
			writeResourceName(diff.DesiredObject, &summary)
			summary.WriteString(" excluded\n")
		case jsondiff.DiffTypeUpdate:
			writeResourceName(diff.DesiredObject, &summary)
			added, changed, removed := summarizeUpdate(diff)
			summary.WriteString(fmt.Sprintf(" changed (%d additions, %d changes, %d removals)\n", added, changed, removed))
		}
	}
	return strings.TrimSpace(summary.String())
}

// SummarizeDiffSetBrief returns a brief summary of the given DiffSet.
//
// The summary is a string in the format:
//
//	removed: x, changed: y, excluded: z, unchanged: w
//
// For example:
//
//	removed: 1, changed: 3, excluded: 1, unchanged: 2
func SummarizeDiffSetBrief(set jsondiff.DiffSet, include ...jsondiff.DiffType) string {
	var removed, changed, excluded, unchanged int
	for _, diff := range set {
		switch diff.Type {
		case jsondiff.DiffTypeCreate:
			removed++
		case jsondiff.DiffTypeUpdate:
			changed++
		case jsondiff.DiffTypeExclude:
			excluded++
		case jsondiff.DiffTypeNone:
			unchanged++
		}
	}

	if include == nil {
		include = DefaultDiffTypes
	}

	var summary strings.Builder
	for _, t := range include {
		switch t {
		case jsondiff.DiffTypeCreate:
			summary.WriteString(fmt.Sprintf("removed: %d, ", removed))
		case jsondiff.DiffTypeUpdate:
			summary.WriteString(fmt.Sprintf("changed: %d, ", changed))
		case jsondiff.DiffTypeExclude:
			summary.WriteString(fmt.Sprintf("excluded: %d, ", excluded))
		case jsondiff.DiffTypeNone:
			summary.WriteString(fmt.Sprintf("unchanged: %d, ", unchanged))
		}
	}
	return strings.TrimSuffix(summary.String(), ", ")
}

// ResourceName returns the resource name in the format `kind/namespace/name`.
func ResourceName(obj client.Object) string {
	var summary strings.Builder
	writeResourceName(obj, &summary)
	return summary.String()
}

const resourceSeparator = "/"

// writeResourceName writes the resource name in the format
// `kind/namespace/name` to the given strings.Builder.
func writeResourceName(obj client.Object, summary *strings.Builder) {
	summary.WriteString(obj.GetObjectKind().GroupVersionKind().Kind)
	summary.WriteString(resourceSeparator)
	if ns := obj.GetNamespace(); ns != "" {
		summary.WriteString(ns)
		summary.WriteString(resourceSeparator)
	}
	summary.WriteString(obj.GetName())
}

// SummarizeUpdate returns the number of added, changed and removed fields
// in the given update patch.
func summarizeUpdate(diff *jsondiff.Diff) (added, changed, removed int) {
	for _, p := range diff.Patch {
		switch p.Type {
		case extjsondiff.OperationAdd:
			added++
		case extjsondiff.OperationReplace:
			changed++
		case extjsondiff.OperationRemove:
			removed++
		}
	}
	return
}

// typeInSlice returns true if the given jsondiff.DiffType is in the slice.
func typeInSlice(t jsondiff.DiffType, slice []jsondiff.DiffType) bool {
	for _, s := range slice {
		if t == s {
			return true
		}
	}
	return false
}
