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

package release

import (
	"encoding/json"

	"github.com/opencontainers/go-digest"
)

// Digest calculates the digest of the given ObservedRelease by JSON encoding
// it into a hash.Hash of the given digest.Algorithm. The algorithm is expected
// to have been confirmed to be available by the caller, not doing this may
// result in panics.
func Digest(algo digest.Algorithm, rel ObservedRelease) digest.Digest {
	digester := algo.Digester()
	enc := json.NewEncoder(digester.Hash())
	if err := enc.Encode(rel); err != nil {
		return ""
	}
	return digester.Digest()
}
