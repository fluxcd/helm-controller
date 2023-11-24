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

package digest

import (
	"crypto"
	_ "crypto/sha256"
	_ "crypto/sha512"

	"github.com/opencontainers/go-digest"
	_ "github.com/opencontainers/go-digest/blake3"
)

const (
	SHA1 digest.Algorithm = "sha1"
)

var (
	// Canonical is the primary digest algorithm used to calculate checksums
	// for e.g. Helm release objects and config values.
	Canonical = digest.SHA256
)

func init() {
	// Register SHA-1 algorithm for support of legacy values checksums.
	digest.RegisterAlgorithm(SHA1, crypto.SHA1)
}
