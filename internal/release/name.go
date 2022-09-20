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
	"crypto/sha256"
	"fmt"
)

// ShortenName returns a short release name in the format of
// '<shortened releaseName>-<hash>' for the given name
// if it exceeds 53 characters in length.
//
// The shortening is done by hashing the given release name with
// SHA256 and taking the first 12 characters of the resulting hash.
// The hash is then appended to the release name shortened to 40
// characters divided by a hyphen separator.
//
// For example: 'some-front-appended-namespace-release-wi-1234567890ab'
// where '1234567890ab' are the first 12 characters of the SHA hash.
func ShortenName(name string) string {
	if len(name) <= 53 {
		return name
	}

	const maxLength = 53
	const shortHashLength = 12

	sum := fmt.Sprintf("%x", sha256.Sum256([]byte(name)))
	shortName := name[:maxLength-(shortHashLength+1)] + "-"
	return shortName + sum[:shortHashLength]
}
