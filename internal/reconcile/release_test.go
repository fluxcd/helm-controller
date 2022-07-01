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
	"testing"

	. "github.com/onsi/gomega"
	helmrelease "helm.sh/helm/v3/pkg/release"

	helmv2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/release"
)

const (
	mockReleaseName      = "mock-release"
	mockReleaseNamespace = "mock-ns"
)

func Test_observeRelease(t *testing.T) {
	const (
		otherReleaseName      = "other"
		otherReleaseNamespace = "other-ns"
	)

	t.Run("release", func(t *testing.T) {
		g := NewWithT(t)

		obj := &helmv2.HelmRelease{}
		mock := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusPendingInstall,
		})
		expect := release.ObservedToInfo(release.ObserveRelease(mock))

		observeRelease(obj)(mock)

		g.Expect(obj.Status.Previous).To(BeNil())
		g.Expect(obj.Status.Current).ToNot(BeNil())
		g.Expect(obj.Status.Current).To(Equal(expect))
	})

	t.Run("release with current", func(t *testing.T) {
		g := NewWithT(t)

		current := &helmv2.HelmReleaseInfo{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
		}
		obj := &helmv2.HelmRelease{
			Status: helmv2.HelmReleaseStatus{
				Current: current,
			},
		}
		mock := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      current.Name,
			Namespace: current.Namespace,
			Version:   current.Version + 1,
			Status:    helmrelease.StatusPendingInstall,
		})
		expect := release.ObservedToInfo(release.ObserveRelease(mock))

		observeRelease(obj)(mock)
		g.Expect(obj.Status.Previous).ToNot(BeNil())
		g.Expect(obj.Status.Previous).To(Equal(current))
		g.Expect(obj.Status.Current).ToNot(BeNil())
		g.Expect(obj.Status.Current).To(Equal(expect))
	})

	t.Run("release with current with different name", func(t *testing.T) {
		g := NewWithT(t)

		current := &helmv2.HelmReleaseInfo{
			Name:      otherReleaseName,
			Namespace: otherReleaseNamespace,
			Version:   3,
		}
		obj := &helmv2.HelmRelease{
			Status: helmv2.HelmReleaseStatus{
				Current: current,
			},
		}
		mock := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusPendingInstall,
		})
		expect := release.ObservedToInfo(release.ObserveRelease(mock))

		observeRelease(obj)(mock)
		g.Expect(obj.Status.Previous).To(BeNil())
		g.Expect(obj.Status.Current).ToNot(BeNil())
		g.Expect(obj.Status.Current).To(Equal(expect))
	})

	t.Run("release with update to previous", func(t *testing.T) {
		g := NewWithT(t)

		previous := &helmv2.HelmReleaseInfo{
			Name:      mockReleaseName,
			Namespace: mockReleaseNamespace,
			Version:   1,
			Status:    helmrelease.StatusDeployed.String(),
		}
		current := &helmv2.HelmReleaseInfo{
			Name:      previous.Name,
			Namespace: previous.Namespace,
			Version:   previous.Version + 1,
			Status:    helmrelease.StatusPendingInstall.String(),
		}
		obj := &helmv2.HelmRelease{
			Status: helmv2.HelmReleaseStatus{
				Current:  current,
				Previous: previous,
			},
		}
		mock := helmrelease.Mock(&helmrelease.MockReleaseOptions{
			Name:      previous.Name,
			Namespace: previous.Namespace,
			Version:   previous.Version,
			Status:    helmrelease.StatusSuperseded,
		})
		expect := release.ObservedToInfo(release.ObserveRelease(mock))

		observeRelease(obj)(mock)
		g.Expect(obj.Status.Previous).ToNot(BeNil())
		g.Expect(obj.Status.Previous).To(Equal(expect))
		g.Expect(obj.Status.Current).To(Equal(current))
	})
}
