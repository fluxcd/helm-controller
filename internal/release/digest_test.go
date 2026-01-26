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

	"github.com/opencontainers/go-digest"
)

func TestDigest(t *testing.T) {
	tests := []struct {
		name string
		algo digest.Algorithm
		rel  Observation
		exp  digest.Digest
	}{
		{
			name: "SHA256",
			algo: digest.SHA256,
			rel: Observation{
				Name: "foo",
			},
			exp: "sha256:f1498f27a16a09cd7e1ee610d924df065c03d30035638babc95dd9a8d412ce4d",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := Digest(tt.algo, tt.rel)
			g.Expect(got).To(Equal(tt.exp))
		})
	}
}
