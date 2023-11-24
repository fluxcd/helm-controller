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

package digest

import (
	"errors"
	"testing"

	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
)

func TestAlgorithmForName(t *testing.T) {
	tests := []struct {
		name    string
		want    digest.Algorithm
		wantErr error
	}{
		{
			name: "sha256",
			want: digest.SHA256,
		},
		{
			name: "sha384",
			want: digest.SHA384,
		},
		{
			name: "sha512",
			want: digest.SHA512,
		},
		{
			name: "blake3",
			want: digest.BLAKE3,
		},
		{
			name: "sha1",
			want: SHA1,
		},
		{
			name:    "not-available",
			wantErr: digest.ErrDigestUnsupported,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			got, err := AlgorithmForName(tt.name)
			if tt.wantErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(errors.Is(err, tt.wantErr)).To(BeTrue())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
