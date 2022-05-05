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

package loader

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	. "github.com/onsi/gomega"
	digestlib "github.com/opencontainers/go-digest"
)

func TestSecureLoadChartFromURL(t *testing.T) {
	g := NewWithT(t)

	b, err := os.ReadFile("testdata/chart-0.1.0.tgz")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(b).ToNot(BeNil())
	digest := digestlib.SHA256.FromBytes(b)

	const chartPath = "/chart.tgz"
	server := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path == chartPath {
			res.WriteHeader(http.StatusOK)
			_, _ = res.Write(b)
			return
		}
		res.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(func() {
		server.Close()
	})

	chartURL := server.URL + chartPath

	client := retryablehttp.NewClient()
	client.Logger = nil
	client.RetryMax = 2

	t.Run("loads Helm chart from URL", func(t *testing.T) {
		g := NewWithT(t)

		got, err := SecureLoadChartFromURL(client, chartURL, digest.String())
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Name()).To(Equal("chart"))
		g.Expect(got.Metadata.Version).To(Equal("0.1.0"))
	})

	t.Run("error on chart data digest mismatch", func(t *testing.T) {
		g := NewWithT(t)

		got, err := SecureLoadChartFromURL(client, chartURL, digestlib.SHA256.FromString("invalid").String())
		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, ErrIntegrity)).To(BeTrue())
		g.Expect(got).To(BeNil())
	})

	t.Run("error on server error", func(t *testing.T) {
		g := NewWithT(t)

		got, err := SecureLoadChartFromURL(client, server.URL+"/invalid.tgz", digest.String())
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(BeNil())
	})
}

func Test_copyAndVerify(t *testing.T) {
	g := NewWithT(t)

	tmpDir := t.TempDir()
	closedF, err := os.CreateTemp(tmpDir, "closed.txt")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(closedF.Close()).ToNot(HaveOccurred())

	tests := []struct {
		name    string
		digest  string
		in      io.Reader
		out     io.Writer
		wantErr bool
	}{
		{
			name:   "digest match (SHA256)",
			digest: digestlib.SHA256.FromString("foo").String(),
			in:     bytes.NewReader([]byte("foo")),
			out:    bytes.NewBuffer(nil),
		},
		{
			name:   "digest match (SHA384)",
			digest: digestlib.SHA384.FromString("foo").String(),
			in:     bytes.NewReader([]byte("foo")),
			out:    bytes.NewBuffer(nil),
		},
		{
			name:   "digest match (SHA512)",
			digest: digestlib.SHA512.FromString("foo").String(),
			in:     bytes.NewReader([]byte("foo")),
			out:    bytes.NewBuffer(nil),
		},
		{
			name:   "digest match (BLAKE3)",
			digest: digestlib.BLAKE3.FromString("foo").String(),
			in:     bytes.NewReader([]byte("foo")),
			out:    bytes.NewBuffer(nil),
		},
		{
			name:    "digest mismatch",
			digest:  digestlib.SHA256.FromString("foo").String(),
			in:      bytes.NewReader([]byte("bar")),
			out:     io.Discard,
			wantErr: true,
		},
		{
			name:    "copy failure (closed file)",
			digest:  digestlib.SHA256.FromString("foo").String(),
			in:      bytes.NewReader([]byte("foo")),
			out:     closedF,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := copyAndVerify(tt.digest, tt.in, tt.out)
			g.Expect(err != nil).To(Equal(tt.wantErr), err)
		})
	}
}
