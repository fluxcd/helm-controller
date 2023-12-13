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
	"strings"
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
	const notFoundPath = "/not-found.tgz"
	server := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path == chartPath {
			res.WriteHeader(http.StatusOK)
			_, _ = res.Write(b)
			return
		}
		if req.URL.Path == notFoundPath {
			res.WriteHeader(http.StatusNotFound)
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

	t.Run("overwrites hostname", func(t *testing.T) {
		g := NewWithT(t)

		t.Setenv(envSourceControllerLocalhost, strings.TrimPrefix(server.URL, "http://"))
		wrongHostnameURL := "http://invalid.com" + chartPath

		got, err := SecureLoadChartFromURL(client, wrongHostnameURL, digest.String())
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

	t.Run("file not found error on 404", func(t *testing.T) {
		g := NewWithT(t)

		got, err := SecureLoadChartFromURL(client, server.URL+notFoundPath, digest.String())
		g.Expect(errors.Is(err, ErrFileNotFound)).To(BeTrue())
		g.Expect(got).To(BeNil())
	})

	t.Run("error on HTTP request failure", func(t *testing.T) {
		g := NewWithT(t)

		got, err := SecureLoadChartFromURL(client, server.URL+"/invalid.tgz", digest.String())
		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, ErrFileNotFound)).To(BeFalse())
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

func Test_overwriteHostname(t *testing.T) {
	tests := []struct {
		name     string
		URL      string
		hostname string
		want     string
		wantErr  bool
	}{
		{
			name:     "overwrite hostname",
			URL:      "http://example.com",
			hostname: "localhost",
			want:     "http://localhost",
		},
		{
			name:     "overwrite hostname with port",
			URL:      "http://example.com",
			hostname: "localhost:9090",
			want:     "http://localhost:9090",
		},
		{
			name:     "no hostname",
			URL:      "http://example.com",
			hostname: "",
			want:     "http://example.com",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := overwriteHostname(tt.URL, tt.hostname)
			if (err != nil) != tt.wantErr {
				t.Errorf("overwriteHostname() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("overwriteHostname() got = %v, want %v", got, tt.want)
			}
		})
	}
}
