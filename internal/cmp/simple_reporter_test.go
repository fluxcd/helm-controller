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
	"testing"

	"github.com/google/go-cmp/cmp"
	. "github.com/onsi/gomega"
)

func TestSimpleReporter_Report(t *testing.T) {
	type simple struct {
		A string
		B string
	}

	tests := []struct {
		name string
		a    simple
		b    simple
		want string
	}{
		{name: "change", a: simple{A: "a", B: "b"}, b: simple{A: "b", B: "a"}, want: `A:
-a
+b

B:
-b
+a`},
		{name: "addition", a: simple{}, b: simple{A: "a"}, want: `A:
+a`},
		{name: "removal", a: simple{A: "a", B: "b"}, b: simple{A: "a"}, want: `B:
-b`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			r := SimpleReporter{}
			_ = cmp.Diff(tt.a, tt.b, cmp.Reporter(&r))
			g.Expect(r.String()).To(Equal(tt.want))
		})
	}
}

func TestSimpleReporter_String(t *testing.T) {
	tests := []struct {
		name  string
		diffs []string
		want  string
	}{
		{name: "trims space", diffs: []string{" at start", "in\nbetween", "at end\n"}, want: `at start
in
between
at end`},
		{name: "joins with newline", diffs: []string{"a", "b", "c"}, want: `a
b
c`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &SimpleReporter{
				diffs: tt.diffs,
			}
			if got := r.String(); got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}
