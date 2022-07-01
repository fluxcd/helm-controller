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

package chartutil

import (
	"github.com/opencontainers/go-digest"
	"helm.sh/helm/v3/pkg/chartutil"
)

// DigestValues calculates the digest of the values using the provided algorithm.
// The caller is responsible for ensuring that the algorithm is supported.
func DigestValues(algo digest.Algorithm, values chartutil.Values) digest.Digest {
	digester := algo.Digester()
	if err := values.Encode(digester.Hash()); err != nil {
		return ""
	}
	return digester.Digest()
}
