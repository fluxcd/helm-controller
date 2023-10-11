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

package cmp

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/google/go-cmp/cmp"
)

// SimpleUnstructuredReporter is a simple reporter for Unstructured objects
// that only records differences detected during comparison, in a diff-like
// format.
type SimpleUnstructuredReporter struct {
	path  cmp.Path
	diffs []string
}

// Report writes a diff entry if rs is not equal. In the format of:
//
//	.spec.replicas
//	-3
//	+1
//
//	.spec.template.spec.containers.[0].command.[6]
//	---deleted=true
//
//	.spec.template.spec.containers.[0].env.[?->1]
//	+map[name:ADDED]
func (r *SimpleUnstructuredReporter) Report(rs cmp.Result) {
	if !rs.Equal() {
		vx, vy := r.path.Last().Values()
		isNonEmptyX := isValidAndNonEmpty(vx)
		isNonEmptyY := isValidAndNonEmpty(vy)

		if !isNonEmptyX && !isNonEmptyY {
			// Skip empty values.
			return
		}

		var sb strings.Builder
		writePathString(r.path, &sb)
		sb.WriteString("\n")
		if isNonEmptyX {
			sb.WriteString(fmt.Sprintf("-%v", vx))
			sb.WriteString("\n")
		}
		if isNonEmptyY {
			sb.WriteString(fmt.Sprintf("+%v", vy))
			sb.WriteString("\n")
		}
		r.diffs = append(r.diffs, sb.String())
	}
}

// String returns the diff entries joined together with newline, trimmed from
// spaces.
func (r *SimpleUnstructuredReporter) String() string {
	return strings.TrimSpace(strings.Join(r.diffs, "\n"))
}

func (r *SimpleUnstructuredReporter) PushStep(ps cmp.PathStep) {
	r.path = append(r.path, ps)
}

func (r *SimpleUnstructuredReporter) PopStep() {
	r.path = r.path[:len(r.path)-1]
}

// https://github.com/istio/istio/blob/2caf81c64a2213dde39174a0a36cae530dc52b69/operator/pkg/compare/compare.go#L79
func isValidAndNonEmpty(v reflect.Value) bool {
	if !v.IsValid() {
		return false
	}
	switch v.Kind() {
	case reflect.Interface:
		return isValidAndNonEmpty(v.Elem())
	case reflect.Array, reflect.Chan, reflect.Map, reflect.Slice, reflect.String:
		return v.Len() > 0
	}
	return true
}

func writePathString(path cmp.Path, sb *strings.Builder) {
	for _, st := range path {
		switch t := st.(type) {
		case cmp.MapIndex:
			sb.WriteString(fmt.Sprintf(".%v", t.Key()))
		case cmp.SliceIndex:
			sb.WriteString(fmt.Sprintf("%v", t.String()))
		}
	}
}
