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
	_ "crypto/sha256"
	_ "crypto/sha512"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"

	"github.com/hashicorp/go-retryablehttp"
	digestlib "github.com/opencontainers/go-digest"
	_ "github.com/opencontainers/go-digest/blake3"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

const (
	// envSourceControllerLocalhost is the name of the environment variable
	// used to override the hostname of the source-controller from which
	// the chart is usually downloaded.
	envSourceControllerLocalhost = "SOURCE_CONTROLLER_LOCALHOST"
)

var (
	// ErrFileNotFound is an error type used to signal 404 HTTP status code responses.
	ErrFileNotFound = errors.New("file not found")
	// ErrIntegrity signals a chart loader failed to verify the integrity of
	// a chart, for example due to a digest mismatch.
	ErrIntegrity = errors.New("integrity failure")
)

// SecureLoadChartFromURL attempts to download a Helm chart from the given URL
// using the provided client. The retrieved data is verified against the given
// digest before loading the chart. It returns the loaded chart.Chart, or an
// error. The error may be of type ErrIntegrity if the integrity check fails.
func SecureLoadChartFromURL(client *retryablehttp.Client, URL, digest string) (*chart.Chart, error) {
	URL, err := overwriteHostname(URL, os.Getenv(envSourceControllerLocalhost))
	if err != nil {
		return nil, err
	}

	req, err := retryablehttp.NewRequest(http.MethodGet, URL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil || resp != nil && resp.StatusCode != http.StatusOK {
		if err != nil {
			return nil, err
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("failed to download chart from '%s': %w", URL, ErrFileNotFound)
		}
		return nil, fmt.Errorf("failed to download chart from '%s' (status: %s)", URL, resp.Status)
	}

	var c bytes.Buffer
	if err := copyAndVerify(digest, resp.Body, &c); err != nil {
		_ = resp.Body.Close()
		return nil, err
	}

	if err := resp.Body.Close(); err != nil {
		return nil, err
	}
	return loader.LoadArchive(&c)
}

// copyAndVerify copies the contents of reader to writer, and verifies the
// integrity of the data using the given digest. It returns an error if the
// integrity check fails.
func copyAndVerify(digest string, reader io.Reader, writer io.Writer) error {
	dig, err := digestlib.Parse(digest)
	if err != nil {
		return fmt.Errorf("failed to parse digest '%s': %w", digest, err)
	}

	verifier := dig.Verifier()
	mw := io.MultiWriter(verifier, writer)
	if _, err := io.Copy(mw, reader); err != nil {
		return fmt.Errorf("failed to copy and verify chart artifact: %w", err)
	}

	if !verifier.Verified() {
		return fmt.Errorf("%w: computed digest doesn't match '%s'", ErrIntegrity, dig)
	}
	return nil
}

// overwriteHostname overwrites the hostname of the given URL with the given
// hostname. If the hostname is empty, the URL is returned unmodified.
func overwriteHostname(URL, hostname string) (string, error) {
	if hostname == "" {
		return URL, nil
	}

	u, err := url.Parse(URL)
	if err != nil {
		return "", fmt.Errorf("failed to parse URL to overwrite hostname: %w", err)
	}
	u.Host = hostname
	return u.String(), nil
}
