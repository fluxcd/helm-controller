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
	_ "crypto/sha256"
	_ "crypto/sha512"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/opencontainers/go-digest"
	_ "github.com/opencontainers/go-digest/blake3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"k8s.io/apimachinery/pkg/types"

	"github.com/fluxcd/pkg/runtime/acl"
	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	sourcev1b2 "github.com/fluxcd/source-controller/api/v1beta2"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
)

const (
	// EnvArtifactHostOverwrite can be used to overwrite the hostname.
	// The main purpose is while running controllers locally with e.g. mocked
	// storage data during development.
	EnvArtifactHostOverwrite = "ARTIFACT_HOST_OVERWRITE"
)

// getHelmChart retrieves the v1beta2.HelmChart for the given
// v2beta1.HelmRelease using the name that is advertised in the status
// object. It returns the v1beta2.HelmChart, or an error.
func (r *HelmReleaseReconciler) getHelmChart(ctx context.Context, hr *v2.HelmRelease) (*sourcev1b2.HelmChart, error) {
	namespace, name := hr.Status.GetHelmChart()
	chartName := types.NamespacedName{Namespace: namespace, Name: name}
	if r.NoCrossNamespaceRef && chartName.Namespace != hr.Namespace {
		return nil, acl.AccessDeniedError(fmt.Sprintf("can't access '%s/%s', cross-namespace references have been blocked",
			hr.Spec.Chart.Spec.SourceRef.Kind, types.NamespacedName{
				Namespace: hr.Spec.Chart.Spec.SourceRef.Namespace,
				Name:      hr.Spec.Chart.Spec.SourceRef.Name,
			}))
	}
	hc := sourcev1b2.HelmChart{}
	if err := r.Client.Get(ctx, chartName, &hc); err != nil {
		return nil, err
	}
	return &hc, nil
}

// loadHelmChart attempts to download the advertised v1beta2.Artifact from the
// provided v1beta2.HelmChart. The digest of the Artifact is confirmed to
// equal to the digest of the retrieved bytes before loading the chart.
// It returns the loaded chart.Chart, or an error.
func (r *HelmReleaseReconciler) loadHelmChart(source *sourcev1b2.HelmChart) (*chart.Chart, error) {
	artifactURL := source.GetArtifact().URL
	if hostname := os.Getenv(EnvArtifactHostOverwrite); hostname != "" {
		if replacedArtifactURL, err := replaceHostname(artifactURL, hostname); err == nil {
			artifactURL = replacedArtifactURL
		}
	}

	req, err := retryablehttp.NewRequest(http.MethodGet, artifactURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create a new request for artifact '%s': %w", source.GetArtifact().URL, err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil || resp != nil && resp.StatusCode != http.StatusOK {
		if resp != nil {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("artifact '%s' download failed (status code: %s)", source.GetArtifact().URL, resp.Status)
		}
		return nil, fmt.Errorf("artifact '%s' download failed: %w", source.GetArtifact().URL, err)
	}

	var c bytes.Buffer
	if err := copyAndVerifyArtifact(source.GetArtifact(), resp.Body, &c); err != nil {
		return nil, fmt.Errorf("artifact '%s' download failed: %w", source.GetArtifact().URL, err)
	}

	if err := resp.Body.Close(); err != nil {
		return nil, fmt.Errorf("artifact '%s' download failed: %w", source.GetArtifact().URL, err)
	}

	return loader.LoadArchive(&c)
}

// copyAndVerifyArtifact copies from reader into writer while confirming the
// digest of the copied data matches the digest from the provided Artifact.
// If this does not match, it returns an error.
func copyAndVerifyArtifact(artifact *sourcev1.Artifact, reader io.Reader, writer io.Writer) error {
	dig, err := digest.Parse(artifact.Digest)
	if err != nil {
		return fmt.Errorf("failed to verify artifact: %w", err)
	}

	// Verify the downloaded artifact against the advertised digest.
	verifier := dig.Verifier()
	mw := io.MultiWriter(verifier, writer)
	if _, err := io.Copy(mw, reader); err != nil {
		return err
	}

	if !verifier.Verified() {
		return fmt.Errorf("failed to verify artifact: computed digest doesn't match advertised '%s'", dig)
	}
	return nil
}

// replaceHostname parses the given URL and replaces the Host in the parsed
// result with the provided hostname. It returns the string result, or an
// error.
func replaceHostname(URL, hostname string) (string, error) {
	parsedURL, err := url.Parse(URL)
	if err != nil {
		return "", err
	}
	parsedURL.Host = hostname
	return parsedURL.String(), nil
}
