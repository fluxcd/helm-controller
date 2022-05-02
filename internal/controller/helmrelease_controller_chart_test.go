/*
Copyright 2020 The Flux authors

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

package controller

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/hashicorp/go-retryablehttp"
	. "github.com/onsi/gomega"
	"github.com/opencontainers/go-digest"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
)

func TestHelmReleaseReconciler_getHelmChart(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	g.Expect(v2.AddToScheme(scheme)).To(Succeed())
	g.Expect(sourcev1b2.AddToScheme(scheme)).To(Succeed())

	chart := &sourcev1b2.HelmChart{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "some-namespace",
			Name:      "some-chart-name",
		},
	}

	tests := []struct {
		name            string
		rel             *v2.HelmRelease
		chart           *sourcev1b2.HelmChart
		expectChart     bool
		wantErr         bool
		disallowCrossNS bool
	}{
		{
			name: "retrieves HelmChart object from Status",
			rel: &v2.HelmRelease{
				Status: v2.HelmReleaseStatus{
					HelmChart: "some-namespace/some-chart-name",
				},
			},
			chart:       chart,
			expectChart: true,
		},
		{
			name: "no HelmChart found",
			rel: &v2.HelmRelease{
				Status: v2.HelmReleaseStatus{
					HelmChart: "some-namespace/some-chart-name",
				},
			},
			chart:       nil,
			expectChart: false,
			wantErr:     true,
		},
		{
			name: "no HelmChart in Status",
			rel: &v2.HelmRelease{
				Status: v2.HelmReleaseStatus{
					HelmChart: "",
				},
			},
			chart:       chart,
			expectChart: false,
			wantErr:     true,
		},
		{
			name: "ACL disallows cross namespace",
			rel: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: "default",
				},
				Status: v2.HelmReleaseStatus{
					HelmChart: "some-namespace/some-chart-name",
				},
			},
			chart:           chart,
			expectChart:     false,
			wantErr:         true,
			disallowCrossNS: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder()
			builder.WithScheme(scheme)
			if tt.chart != nil {
				builder.WithObjects(tt.chart)
			}

			r := &HelmReleaseReconciler{
				Client:              builder.Build(),
				EventRecorder:       record.NewFakeRecorder(32),
				NoCrossNamespaceRef: tt.disallowCrossNS,
			}

			got, err := r.getHelmChart(context.TODO(), tt.rel)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(got).To(BeNil())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			expect := g.Expect(got.ObjectMeta)
			if tt.expectChart {
				expect.To(BeEquivalentTo(tt.chart.ObjectMeta))
			} else {
				expect.To(BeNil())
			}
		})
	}
}

func TestHelmReleaseReconciler_loadHelmChart(t *testing.T) {
	g := NewWithT(t)

	b, err := os.ReadFile("testdata/chart-0.1.0.tgz")
	g.Expect(err).ToNot(HaveOccurred())
	g.Expect(b).ToNot(BeNil())
	dig := digest.SHA256.FromBytes(b)

	const chartPath = "/chart.tgz"
	server := httptest.NewServer(http.HandlerFunc(func(res http.ResponseWriter, req *http.Request) {
		if req.URL.Path == chartPath {
			res.WriteHeader(http.StatusOK)
			_, _ = res.Write(b)
			return
		}
		res.WriteHeader(http.StatusInternalServerError)
		return
	}))
	t.Cleanup(server.Close)

	chartURL := server.URL + chartPath

	client := retryablehttp.NewClient()
	client.Logger = nil
	client.RetryMax = 2

	t.Run("loads HelmChart from Artifact URL", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmReleaseReconciler{
			Client:        fake.NewClientBuilder().Build(),
			EventRecorder: record.NewFakeRecorder(32),
			httpClient:    client,
		}
		got, err := r.loadHelmChart(&sourcev1b2.HelmChart{
			Status: sourcev1b2.HelmChartStatus{
				Artifact: &sourcev1.Artifact{
					URL:    chartURL,
					Digest: dig.String(),
				},
			},
		})
		g.Expect(err).ToNot(HaveOccurred())
		g.Expect(got).ToNot(BeNil())
		g.Expect(got.Name()).To(Equal("chart"))
		g.Expect(got.Metadata.Version).To(Equal("0.1.0"))
	})

	t.Run("error on Artifact digest mismatch", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmReleaseReconciler{
			Client:        fake.NewClientBuilder().Build(),
			EventRecorder: record.NewFakeRecorder(32),
			httpClient:    client,
		}
		got, err := r.loadHelmChart(&sourcev1b2.HelmChart{
			Status: sourcev1b2.HelmChartStatus{
				Artifact: &sourcev1.Artifact{
					URL:    chartURL,
					Digest: "",
				},
			},
		})
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(BeNil())
	})

	t.Run("error on server error", func(t *testing.T) {
		g := NewWithT(t)

		r := &HelmReleaseReconciler{
			Client:        fake.NewClientBuilder().Build(),
			EventRecorder: record.NewFakeRecorder(32),
			httpClient:    client,
		}
		got, err := r.loadHelmChart(&sourcev1b2.HelmChart{
			Status: sourcev1b2.HelmChartStatus{
				Artifact: &sourcev1.Artifact{
					URL:    server.URL + "/invalid.tgz",
					Digest: "",
				},
			},
		})
		g.Expect(err).To(HaveOccurred())
		g.Expect(got).To(BeNil())
	})

	t.Run("EnvArtifactHostOverwrite overwrites Artifact hostname", func(t *testing.T) {
		g := NewWithT(t)

		t.Setenv(EnvArtifactHostOverwrite, strings.TrimPrefix(server.URL, "http://"))
		r := &HelmReleaseReconciler{
			Client:        fake.NewClientBuilder().Build(),
			EventRecorder: record.NewFakeRecorder(32),
			httpClient:    client,
		}
		got, err := r.loadHelmChart(&sourcev1b2.HelmChart{
			Status: sourcev1b2.HelmChartStatus{
				Artifact: &sourcev1.Artifact{
					URL:    "http://example.com" + chartPath,
					Digest: dig.String(),
				},
			},
		})
		g.Expect(err).To(Not(HaveOccurred()))
		g.Expect(got).ToNot(BeNil())
	})
}

func Test_copyAndVerifyArtifact(t *testing.T) {
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
			name:   "digest match",
			digest: "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			in:     bytes.NewReader([]byte("foo")),
			out:    io.Discard,
		},
		{
			name:    "digest mismatch",
			digest:  "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			in:      bytes.NewReader([]byte("bar")),
			out:     io.Discard,
			wantErr: true,
		},
		{
			name:    "copy failure (closed file)",
			digest:  "sha256:2c26b46b68ffc68ff99b453c1d30413413422d706483bfa0f98a5e886266e7ae",
			in:      bytes.NewReader([]byte("foo")),
			out:     closedF,
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			err := copyAndVerifyArtifact(&sourcev1.Artifact{Digest: tt.digest}, tt.in, tt.out)
			g.Expect(err != nil).To(Equal(tt.wantErr), err)
		})
	}
}

func Test_replaceHostname(t *testing.T) {
	tests := []struct {
		name     string
		URL      string
		hostname string
		want     string
		wantErr  bool
	}{
		{"hostname overwrite", "https://example.com/file.txt", "overwrite.com", "https://overwrite.com/file.txt", false},
		{"hostname overwrite with port", "https://example.com:8080/file.txt", "overwrite.com:6666", "https://overwrite.com:6666/file.txt", false},
		{"invalid url", ":malformed./com", "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got, err := replaceHostname(tt.URL, tt.hostname)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(got).To(BeEmpty())
				return
			}
			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}
