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
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

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
	const slowPath = "/slow.tgz"
	handler := http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case chartPath:
			res.WriteHeader(http.StatusOK)
			_, _ = res.Write(b)
		case notFoundPath:
			res.WriteHeader(http.StatusNotFound)
		case slowPath:
			time.Sleep(200 * time.Millisecond)
			res.WriteHeader(http.StatusOK)
			_, _ = res.Write(b)
		default:
			res.WriteHeader(http.StatusInternalServerError)
		}
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	chartURL := server.URL + chartPath

	t.Run("loads Helm chart from URL", func(t *testing.T) {
		g := NewWithT(t)

		got, err := SecureLoadChartFromURL(context.Background(), chartURL, digest.String(), 2, 5*time.Second, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Name()).To(Equal("chart"))
		g.Expect(got.Metadata.Version).To(Equal("0.1.0"))
	})

	t.Run("loads Helm chart from URL over TLS", func(t *testing.T) {
		g := NewWithT(t)

		tlsServer := httptest.NewTLSServer(handler)
		t.Cleanup(tlsServer.Close)

		tlsConfig := tlsServer.Client().Transport.(*http.Transport).TLSClientConfig

		// The fetcher must upgrade the scheme from http to https.
		httpURL := "http://" + strings.TrimPrefix(tlsServer.URL, "https://") + chartPath
		got, err := SecureLoadChartFromURL(context.Background(), httpURL, digest.String(), 2, 5*time.Second, tlsConfig)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Name()).To(Equal("chart"))
		g.Expect(got.Metadata.Version).To(Equal("0.1.0"))
	})

	t.Run("overwrites hostname", func(t *testing.T) {
		g := NewWithT(t)

		t.Setenv(envSourceControllerLocalhost, strings.TrimPrefix(server.URL, "http://"))
		wrongHostnameURL := "http://invalid.com" + chartPath

		got, err := SecureLoadChartFromURL(context.Background(), wrongHostnameURL, digest.String(), 2, 5*time.Second, nil)
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Name()).To(Equal("chart"))
		g.Expect(got.Metadata.Version).To(Equal("0.1.0"))
	})

	t.Run("error on chart data digest mismatch", func(t *testing.T) {
		g := NewWithT(t)

		got, err := SecureLoadChartFromURL(context.Background(), chartURL, digestlib.SHA256.FromString("invalid").String(), 2, 5*time.Second, nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(BeNil())
	})

	t.Run("file not found error on 404", func(t *testing.T) {
		g := NewWithT(t)

		got, err := SecureLoadChartFromURL(context.Background(), server.URL+notFoundPath, digest.String(), 2, 5*time.Second, nil)
		g.Expect(errors.Is(err, ErrFileNotFound)).To(BeTrue())
		g.Expect(got).To(BeNil())
	})

	t.Run("error on HTTP request failure", func(t *testing.T) {
		g := NewWithT(t)

		got, err := SecureLoadChartFromURL(context.Background(), server.URL+"/invalid.tgz", digest.String(), 2, 5*time.Second, nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(errors.Is(err, ErrFileNotFound)).To(BeFalse())
		g.Expect(got).To(BeNil())
	})

	t.Run("timeout", func(t *testing.T) {
		g := NewWithT(t)

		got, err := SecureLoadChartFromURL(context.Background(), server.URL+slowPath, digest.String(), 0, 10*time.Millisecond, nil)
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(BeNil())
	})
}
