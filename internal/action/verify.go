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

package action

import (
	"errors"

	"github.com/opencontainers/go-digest"
	helmaction "helm.sh/helm/v3/pkg/action"
	helmchart "helm.sh/helm/v3/pkg/chart"
	helmchartutil "helm.sh/helm/v3/pkg/chartutil"
	helmrelease "helm.sh/helm/v3/pkg/release"

	helmdriver "helm.sh/helm/v3/pkg/storage/driver"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/chartutil"
	"github.com/fluxcd/helm-controller/internal/release"
)

var (
	ErrReleaseDisappeared = errors.New("observed release disappeared from storage")
	ErrReleaseNotFound    = errors.New("no release found")
	ErrReleaseNotObserved = errors.New("release not observed to be made by reconciler")
	ErrReleaseDigest      = errors.New("release digest verification error")
	ErrChartChanged       = errors.New("release chart changed")
	ErrConfigDigest       = errors.New("release config digest verification error")
)

// VerifyStorage verifies that the last release in the Helm storage matches the
// Current state of the given HelmRelease. It returns the release, or an error
// of type ErrReleaseDisappeared, ErrReleaseNotFound, ErrReleaseNotObserved, or
// ErrReleaseDigest.
func VerifyStorage(config *helmaction.Configuration, obj *v2.HelmRelease) (*helmrelease.Release, error) {
	curRel := obj.Status.Current
	rls, err := config.Releases.Last(obj.GetReleaseName())
	if err != nil {
		if errors.Is(err, helmdriver.ErrReleaseNotFound) {
			if curRel != nil && curRel.Name == obj.GetReleaseName() && curRel.Namespace == obj.GetReleaseNamespace() {
				return nil, ErrReleaseDisappeared
			}
			return nil, ErrReleaseNotFound
		}
		return nil, err
	}
	if curRel == nil {
		return rls, ErrReleaseNotObserved
	}

	relDig, err := digest.Parse(obj.Status.Current.Digest)
	if err != nil {
		return rls, ErrReleaseDigest
	}
	verifier := relDig.Verifier()

	obs := release.ObserveRelease(rls)
	if err := obs.Encode(verifier); err != nil {
		// We are expected to be able to encode valid JSON, error out without a
		// typed error assuming malfunction to signal to e.g. retry.
		return nil, err
	}
	if !verifier.Verified() {
		return nil, ErrReleaseNotObserved
	}
	return rls, nil
}

// VerifyRelease verifies that the data of the given release matches the given
// chart metadata, and the provided values match the Current.ConfigDigest.
// It returns either an error of type ErrReleaseNotFound, ErrChartChanged or
// ErrConfigDigest, or nil.
func VerifyRelease(rls *helmrelease.Release, obj *v2.HelmRelease, chrt *helmchart.Metadata, vals helmchartutil.Values) error {
	if rls == nil {
		return ErrReleaseNotFound
	}

	if chrt != nil {
		if _, eq := chartutil.DiffMeta(*rls.Chart.Metadata, *chrt); !eq {
			return ErrChartChanged
		}
	}

	if !chartutil.VerifyValues(digest.Digest(obj.Status.Current.ConfigDigest), vals) {
		return ErrConfigDigest
	}
	return nil
}
