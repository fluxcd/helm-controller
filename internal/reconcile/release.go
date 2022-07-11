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

package reconcile

import (
	"errors"

	helmrelease "helm.sh/helm/v3/pkg/release"

	helmv2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
)

var (
	// ErrNoCurrent is returned when the HelmRelease has no current release
	// but this is required by the ActionReconciler.
	ErrNoCurrent = errors.New("no current release")
	// ErrNoPrevious is returned when the HelmRelease has no previous release
	// but this is required by the ActionReconciler.
	ErrNoPrevious = errors.New("no previous release")
	// ErrReleaseMismatch is returned when the resulting release after running
	// an action does not match the expected current and/or previous release.
	// This can happen for actions where targeting a release by version is not
	// possible, for example while running tests.
	ErrReleaseMismatch = errors.New("release mismatch")
)

// observeRelease returns a storage.ObserveFunc which updates the Status.Current
// and Status.Previous fields of the HelmRelease object. It can be used to
// record Helm install and upgrade actions as - and while - they are written to
// the Helm storage.
func observeRelease(obj *helmv2.HelmRelease) storage.ObserveFunc {
	return func(rls *helmrelease.Release) {
		cur := obj.Status.Current.DeepCopy()
		obs := release.ObserveRelease(rls)
		if cur != nil && obs.Targets(cur.Name, cur.Namespace, 0) && cur.Version < obs.Version {
			// Add current to previous when we observe the first write of a
			// newer release.
			obj.Status.Previous = obj.Status.Current
		}
		if cur == nil || !obs.Targets(cur.Name, cur.Namespace, 0) || obs.Version >= cur.Version {
			// Overwrite current with newer release, or update it.
			obj.Status.Current = release.ObservedToInfo(obs)
		}
		if prev := obj.Status.Previous; prev != nil && obs.Targets(prev.Name, prev.Namespace, prev.Version) {
			// Write latest state of previous (e.g. status updates) to status.
			obj.Status.Previous = release.ObservedToInfo(obs)
		}
	}
}
