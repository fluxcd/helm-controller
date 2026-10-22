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
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	chart "helm.sh/helm/v4/pkg/chart/v2"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	ctrl "sigs.k8s.io/controller-runtime"

	fluxfetch "github.com/fluxcd/pkg/http/fetch"
)

const (
	// envSourceControllerLocalhost is the name of the environment variable
	// used to override the hostname of the source-controller from which
	// the chart is downloaded.
	envSourceControllerLocalhost = "SOURCE_CONTROLLER_LOCALHOST"

	// envSourceWatcherLocalhost is the name of the environment variable
	// used to override the hostname of the source-watcher from which
	// the chart is downloaded.
	envSourceWatcherLocalhost = "SOURCE_WATCHER_LOCALHOST"
)

var (
	// ErrFileNotFound is an error type used to signal 404 HTTP status code responses.
	ErrFileNotFound = errors.New("file not found")
)

// SecureLoadChartFromURL attempts to download a Helm chart from the given URL,
// verifies it against the given digest and loads it. The error may be of type
// ErrFileNotFound if the artifact does not exist.
func SecureLoadChartFromURL(ctx context.Context, URL, digest string, retries int, timeout time.Duration, tlsConfig *tls.Config) (*chart.Chart, error) {
	sourceLocalhost := os.Getenv(envSourceControllerLocalhost)
	if strings.Contains(URL, "//source-watcher") {
		sourceLocalhost = os.Getenv(envSourceWatcherLocalhost)
	}

	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	fetcher := fluxfetch.New(
		fluxfetch.WithLogger(ctrl.LoggerFrom(ctx)),
		fluxfetch.WithRetries(retries),
		fluxfetch.WithHostnameOverwrite(sourceLocalhost),
		fluxfetch.WithTLSConfig(tlsConfig),
	)

	buf, err := fetcher.FetchBytes(ctx, URL, digest)
	if err != nil {
		if errors.Is(err, fluxfetch.ErrFileNotFound) {
			return nil, fmt.Errorf("failed to download chart from '%s': %w", URL, ErrFileNotFound)
		}
		return nil, err
	}

	return loader.LoadArchive(&buf)
}
