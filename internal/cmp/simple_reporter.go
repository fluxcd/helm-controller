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

package cmp

import (
	"fmt"
	"strings"

	"github.com/google/go-cmp/cmp"
)

// SimpleReporter is a simple reporter that only records differences detected
// during comparison.
type SimpleReporter struct {
	path  cmp.Path
	diffs []string
}

// Report writes a diff entry if rs is not equal. In the format of:
//  Path:
//  -old
//  +new
//
//  Path.Nested:
//  -removed
//
//  Path.Other:
//  +added
func (r *SimpleReporter) Report(rs cmp.Result) {
	if !rs.Equal() {
		vx, vy := r.path.Last().Values()
		var sb strings.Builder
		sb.WriteString(r.path.String() + ":\n")
		if !vx.IsZero() {
			sb.WriteString(fmt.Sprintf("-%+v\n", vx))
		}
		if !vy.IsZero() {
			sb.WriteString(fmt.Sprintf("+%+v\n", vy))
		}
		r.diffs = append(r.diffs, sb.String())
	}
}

// String returns the diff entries joined together with newline, trimmed from
// spaces.
func (r *SimpleReporter) String() string {
	return strings.TrimSpace(strings.Join(r.diffs, "\n"))
}

func (r *SimpleReporter) PushStep(ps cmp.PathStep) {
	r.path = append(r.path, ps)
}

func (r *SimpleReporter) PopStep() {
	r.path = r.path[:len(r.path)-1]
}
