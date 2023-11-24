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

package release

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestShortName(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name     string
		expected string
	}{
		{
			name:     "release-name",
			expected: "release-name",
		},
		{
			name:     "release-name-with-very-long-name-which-is-longer-than-53-characters",
			expected: "release-name-with-very-long-name-which-i-788ca0d0d7b0",
		},
		{
			name:     "another-release-name-with-very-long-name-which-is-longer-than-53-characters",
			expected: "another-release-name-with-very-long-name-7e72150d5a36",
		},
		{
			name:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		got := ShortenName(tt.name)
		g.Expect(got).To(Equal(tt.expected), got)
		g.Expect(got).To(Satisfy(func(s string) bool { return len(s) <= 53 }))
	}
}
