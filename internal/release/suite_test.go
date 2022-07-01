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
	"fmt"
	"log"
	"os"
	"testing"

	"helm.sh/helm/v3/pkg/release"
)

var (
	// smallRelease is 125K while encoded.
	smallRelease *release.Release
	// midRelease is 17K while encoded, but heavier in metadata than smallRelease.
	midRelease *release.Release
	// biggerRelease is 862K while encoded.
	biggerRelease *release.Release
)

func TestMain(m *testing.M) {
	var err error
	if smallRelease, err = decodeReleaseFromFile("testdata/istio-base-1"); err != nil {
		log.Fatal(err)
	}
	if midRelease, err = decodeReleaseFromFile("testdata/podinfo-helm-1"); err != nil {
		log.Fatal(err)
	}
	if biggerRelease, err = decodeReleaseFromFile("testdata/prom-stack-1"); err != nil {
		log.Fatal(err)
	}
	r := m.Run()
	os.Exit(r)
}

func decodeReleaseFromFile(path string) (*release.Release, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to load encoded release data: %w", err)
	}
	rel, err := decodeRelease(string(b))
	if err != nil {
		return nil, fmt.Errorf("failed to decode release data: %w", err)
	}
	return rel, nil
}
