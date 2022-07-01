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
	"testing"

	"github.com/opencontainers/go-digest"
	"helm.sh/helm/v3/pkg/release"

	intdigest "github.com/fluxcd/helm-controller/internal/digest"
)

func init() {
	intdigest.Canonical = digest.SHA256
}

func benchmarkNewObservedRelease(rel release.Release, b *testing.B) {
	b.ReportAllocs()
	for n := 0; n < b.N; n++ {
		ObservedToInfo(ObserveRelease(&rel))
	}
}

func BenchmarkNewObservedReleaseSmall(b *testing.B) {
	benchmarkNewObservedRelease(*smallRelease, b)
}

func BenchmarkNewObservedReleaseMid(b *testing.B) {
	benchmarkNewObservedRelease(*midRelease, b)
}

func BenchmarkNewObservedReleaseBigger(b *testing.B) {
	benchmarkNewObservedRelease(*biggerRelease, b)
}
