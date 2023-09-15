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

package testutil

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	sourcev1 "github.com/fluxcd/source-controller/api/v1"
	"github.com/opencontainers/go-digest"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/rand"
)

// SaveChart saves the given chart to the given directory, and returns the
// path to the saved chart. The chart is saved with a random suffix to avoid
// name collisions.
func SaveChart(c *chart.Chart, outDir string) (string, error) {
	tmpDir, err := os.MkdirTemp("", "chart-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	tmpChart, err := chartutil.Save(c, tmpDir)
	if err != nil {
		return "", err
	}

	var (
		tmpChartFileName = filepath.Base(tmpChart)
		tmpChartExt      = filepath.Ext(tmpChartFileName)
		newChartFileName = strings.TrimSuffix(tmpChartFileName, tmpChartExt) + "-" + rand.String(5) + tmpChartExt
		targetPath       = filepath.Join(outDir, newChartFileName)
	)

	if err = os.Rename(tmpChart, targetPath); err != nil {
		return "", err
	}
	return targetPath, nil
}

// SaveChartAsArtifact saves the given chart to the given directory, and
// returns an artifact with the chart's metadata. The chart is saved with a
// random suffix to avoid name collisions.
func SaveChartAsArtifact(c *chart.Chart, algo digest.Algorithm, baseURL, outDir string) (*sourcev1.Artifact, error) {
	abs, err := SaveChart(c, outDir)
	if err != nil {
		return nil, err
	}

	f, err := os.Open(abs)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	bc := &byteCountReader{Reader: f}
	dig, err := algo.FromReader(bc)
	if err != nil {
		return nil, err
	}

	rel, err := filepath.Rel(outDir, abs)
	if err != nil {
		return nil, err
	}
	fileURL := strings.TrimSuffix(baseURL, "/") + "/" + rel

	return &sourcev1.Artifact{
		Path:           abs,
		URL:            fileURL,
		Revision:       c.Metadata.Version,
		Digest:         dig.String(),
		LastUpdateTime: v1.Now(),
		Size:           &bc.Count,
	}, nil
}

type byteCountReader struct {
	Reader io.Reader
	Count  int64
}

func (b *byteCountReader) Read(p []byte) (n int, err error) {
	n, err = b.Reader.Read(p)
	b.Count += int64(n)
	return n, err
}
