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
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

var (
	// ErrIntegrity signals a chart loader failed to verify the integrity of
	// a chart, for example due to a checksum mismatch.
	ErrIntegrity = errors.New("integrity failure")
)

// SecureLoadChartFromURL attempts to download a Helm chart from the given URL
// using the provided client. The SHA256 sum of the retrieved data is confirmed
// to equal the given checksum before loading the chart. It returns the loaded
// chart.Chart, or an error.
func SecureLoadChartFromURL(client *retryablehttp.Client, URL, checksum string) (*chart.Chart, error) {
	req, err := retryablehttp.NewRequest(http.MethodGet, URL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil || resp != nil && resp.StatusCode != http.StatusOK {
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("failed to load chart from '%s': %s", URL, resp.Status)
	}

	var c bytes.Buffer
	if err := copyAndSHA256Check(checksum, resp.Body, &c); err != nil {
		return nil, err
	}

	if err := resp.Body.Close(); err != nil {
		return nil, err
	}
	return loader.LoadArchive(&c)
}

// copyAndSHA256Check copies from reader into writer while confirming the
// SHA256 checksum of the copied data matches the provided checksum. If
// this does not match, it returns an error.
func copyAndSHA256Check(checksum string, reader io.Reader, writer io.Writer) error {
	hasher := sha256.New()
	mw := io.MultiWriter(hasher, writer)
	if _, err := io.Copy(mw, reader); err != nil {
		return fmt.Errorf("failed to verify artifact: %w", err)
	}
	if calc := fmt.Sprintf("%x", hasher.Sum(nil)); checksum != calc {
		return fmt.Errorf("%w: computed checksum '%s' doesn't match '%s'", ErrIntegrity, calc, checksum)
	}
	return nil
}
