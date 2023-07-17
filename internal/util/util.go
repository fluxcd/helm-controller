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

package util

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	intyaml "github.com/fluxcd/helm-controller/internal/yaml"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
)

// ValuesChecksum calculates and returns the SHA1 checksum for the
// given chartutil.Values.
func ValuesChecksum(values chartutil.Values) string {
	var s string
	if len(values) != 0 {
		s, _ = values.YAML()
	}
	return fmt.Sprintf("%x", sha1.Sum([]byte(s)))
}

// OrderedValuesChecksum sort the chartutil.Values then calculates
// and returns the SHA1 checksum for the sorted values.
func OrderedValuesChecksum(values chartutil.Values) string {
	var buf bytes.Buffer
	if len(values) != 0 {
		_ = intyaml.Encode(&buf, values, intyaml.SortMapSlice)
	}
	return fmt.Sprintf("%x", sha1.Sum(buf.Bytes()))
}

// ReleaseRevision returns the revision of the given release.Release.
func ReleaseRevision(rel *release.Release) int {
	if rel == nil {
		return 0
	}
	return rel.Version
}
