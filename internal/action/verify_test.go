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
	"testing"

	. "github.com/onsi/gomega"
	helmaction "helm.sh/helm/v3/pkg/action"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	helmrelease "helm.sh/helm/v3/pkg/release"
	helmstorage "helm.sh/helm/v3/pkg/storage"
	"helm.sh/helm/v3/pkg/storage/driver"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
	"github.com/fluxcd/helm-controller/internal/testutil"
)

func TestReleaseTargetChanged(t *testing.T) {
	const (
		defaultNamespace        = "default-ns"
		defaultName             = "default-name"
		defaultChartName        = "default-chart"
		defaultReleaseName      = "default-release"
		defaultTargetNamespace  = "default-target-ns"
		defaultStorageNamespace = "default-storage-ns"
	)

	tests := []struct {
		name      string
		chartName string
		spec      v2.HelmReleaseSpec
		status    v2.HelmReleaseStatus
		want      bool
	}{
		{
			name:      "no change",
			chartName: defaultChartName,
			spec:      v2.HelmReleaseSpec{},
			status: v2.HelmReleaseStatus{
				Current: &v2.HelmReleaseInfo{
					Name:      defaultName,
					Namespace: defaultNamespace,
					ChartName: defaultChartName,
				},
				StorageNamespace: defaultNamespace,
			},
			want: false,
		},
		{
			name:      "no storage namespace",
			chartName: defaultChartName,
			spec: v2.HelmReleaseSpec{
				ReleaseName: defaultReleaseName,
			},
			status: v2.HelmReleaseStatus{
				Current: &v2.HelmReleaseInfo{
					Name:      defaultReleaseName,
					Namespace: defaultNamespace,
					ChartName: defaultChartName,
				},
			},
			want: false,
		},
		{
			name: "no current",
			spec: v2.HelmReleaseSpec{},
			status: v2.HelmReleaseStatus{
				StorageNamespace: defaultNamespace,
				Current:          nil,
			},
			want: false,
		},
		{
			name:      "different storage namespace",
			chartName: defaultChartName,
			spec: v2.HelmReleaseSpec{
				StorageNamespace: defaultStorageNamespace,
			},
			status: v2.HelmReleaseStatus{
				Current: &v2.HelmReleaseInfo{
					Name:      defaultName,
					Namespace: defaultNamespace,
					ChartName: defaultChartName,
				},
				StorageNamespace: defaultNamespace,
			},
			want: true,
		},
		{
			name:      "different release namespace",
			chartName: defaultChartName,
			spec: v2.HelmReleaseSpec{
				TargetNamespace: defaultTargetNamespace,
			},
			status: v2.HelmReleaseStatus{
				Current: &v2.HelmReleaseInfo{
					Name:      defaultName,
					Namespace: defaultNamespace,
					ChartName: defaultChartName,
				},
				StorageNamespace: defaultNamespace,
			},
			want: true,
		},
		{
			name:      "different release name",
			chartName: defaultChartName,
			spec: v2.HelmReleaseSpec{
				ReleaseName: defaultReleaseName,
			},
			status: v2.HelmReleaseStatus{
				Current: &v2.HelmReleaseInfo{
					Name:      defaultName,
					Namespace: defaultNamespace,
					ChartName: defaultChartName,
				},
				StorageNamespace: defaultNamespace,
			},
			want: true,
		},
		{
			name:      "different chart name",
			chartName: "other-chart",
			spec:      v2.HelmReleaseSpec{},
			status: v2.HelmReleaseStatus{
				Current: &v2.HelmReleaseInfo{
					Name:      defaultName,
					Namespace: defaultNamespace,
					ChartName: defaultChartName,
				},
				StorageNamespace: defaultNamespace,
			},
			want: true,
		},
		{
			name:      "matching shortened release name",
			chartName: defaultChartName,
			spec: v2.HelmReleaseSpec{
				TargetNamespace: "target-namespace-exceeding-max-characters",
			},
			status: v2.HelmReleaseStatus{
				Current: &v2.HelmReleaseInfo{
					Name:      "target-namespace-exceeding-max-character-eceb26601388",
					Namespace: "target-namespace-exceeding-max-characters",
					ChartName: defaultChartName,
				},
				StorageNamespace: defaultNamespace,
			},
			want: false,
		},
		{
			name:      "different shortened release name",
			chartName: defaultChartName,
			spec: v2.HelmReleaseSpec{
				TargetNamespace: "target-namespace-exceeding-max-characters",
			},
			status: v2.HelmReleaseStatus{
				Current: &v2.HelmReleaseInfo{
					Name:      defaultName,
					Namespace: "target-namespace-exceeding-max-characters",
					ChartName: defaultChartName,
				},
				StorageNamespace: defaultNamespace,
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := ReleaseTargetChanged(&v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: defaultNamespace,
					Name:      defaultName,
				},
				Spec:   tt.spec,
				Status: tt.status,
			}, tt.chartName)
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestIsInstalled(t *testing.T) {
	var mockError = errors.New("query mock error")

	tests := []struct {
		name        string
		releaseName string
		releases    []*helmrelease.Release
		queryError  error
		want        bool
		wantErr     error
	}{
		{
			name:        "installed",
			releaseName: "release",
			releases: []*helmrelease.Release{
				testutil.BuildRelease(&helmrelease.MockReleaseOptions{
					Name:      "release",
					Version:   1,
					Status:    helmrelease.StatusDeployed,
					Namespace: "default",
				}),
			},
			want: true,
		},
		{
			name:        "not installed",
			releaseName: "release",
			want:        false,
		},
		{
			name:       "release list error",
			queryError: mockError,
			wantErr:    mockError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := helmstorage.Init(driver.NewMemory())
			for _, v := range tt.releases {
				g.Expect(s.Create(v)).To(Succeed())
			}

			s.Driver = &storage.Failing{
				Driver:   s.Driver,
				QueryErr: tt.queryError,
			}

			got, err := IsInstalled(&helmaction.Configuration{Releases: s}, tt.releaseName)

			if tt.wantErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(Equal(tt.wantErr))
				g.Expect(got).To(BeFalse())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(got).To(Equal(tt.want))
		})
	}
}

func TestVerifyReleaseInfo(t *testing.T) {
	mock := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
		Name:      "release",
		Version:   1,
		Status:    helmrelease.StatusDeployed,
		Namespace: "default",
	})
	otherMock := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
		Name:      "release",
		Version:   1,
		Status:    helmrelease.StatusSuperseded,
		Namespace: "default",
	})
	mockInfo := release.ObservedToInfo(release.ObserveRelease(mock))
	mockGetErr := errors.New("mock get error")

	tests := []struct {
		name     string
		info     *v2.HelmReleaseInfo
		release  *helmrelease.Release
		getError error
		want     *helmrelease.Release
		wantErr  error
	}{
		{
			name:    "valid release",
			info:    mockInfo,
			release: mock,
			want:    mock,
		},
		{
			name:    "invalid release",
			info:    mockInfo,
			release: otherMock,
			wantErr: ErrReleaseNotObserved,
		},
		{
			name:    "release not found",
			info:    mockInfo,
			release: nil,
			wantErr: ErrReleaseDisappeared,
		},
		{
			name:    "no release info",
			info:    nil,
			release: nil,
			wantErr: ErrReleaseNotFound,
		},
		{
			name:     "driver get error",
			info:     mockInfo,
			getError: mockGetErr,
			wantErr:  mockGetErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := helmstorage.Init(driver.NewMemory())
			if tt.release != nil {
				g.Expect(s.Create(tt.release)).To(Succeed())
			}

			s.Driver = &storage.Failing{
				Driver: s.Driver,
				GetErr: tt.getError,
			}

			rls, err := VerifyReleaseInfo(&helmaction.Configuration{Releases: s}, tt.info)
			if tt.wantErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(Equal(tt.wantErr))
				g.Expect(rls).To(BeNil())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(rls).To(Equal(tt.want))
		})
	}
}

func TestVerifyLastStorageItem(t *testing.T) {
	mockOne := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
		Name:      "release",
		Version:   1,
		Status:    helmrelease.StatusSuperseded,
		Namespace: "default",
	})
	mockTwo := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
		Name:      "release",
		Version:   2,
		Status:    helmrelease.StatusDeployed,
		Namespace: "default",
	})
	mockInfo := release.ObservedToInfo(release.ObserveRelease(mockTwo))
	mockQueryErr := errors.New("mock query error")

	tests := []struct {
		name       string
		info       *v2.HelmReleaseInfo
		releases   []*helmrelease.Release
		queryError error
		want       *helmrelease.Release
		wantErr    error
	}{
		{
			name:     "valid last release",
			info:     mockInfo,
			releases: []*helmrelease.Release{mockOne, mockTwo},
			want:     mockTwo,
		},
		{
			name:     "invalid last release",
			info:     mockInfo,
			releases: []*helmrelease.Release{mockOne},
			wantErr:  ErrReleaseNotObserved,
		},
		{
			name:     "no last release",
			info:     mockInfo,
			releases: []*helmrelease.Release{},
			wantErr:  ErrReleaseDisappeared,
		},
		{
			name:     "no release info",
			info:     nil,
			releases: nil,
			wantErr:  ErrReleaseNotFound,
		},
		{
			name:       "driver query error",
			info:       mockInfo,
			queryError: mockQueryErr,
			wantErr:    mockQueryErr,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			s := helmstorage.Init(driver.NewMemory())
			for _, v := range tt.releases {
				g.Expect(s.Create(v)).To(Succeed())
			}

			s.Driver = &storage.Failing{
				Driver:   s.Driver,
				QueryErr: tt.queryError,
			}

			rls, err := VerifyLastStorageItem(&helmaction.Configuration{Releases: s}, tt.info)
			if tt.wantErr != nil {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err).To(Equal(tt.wantErr))
				g.Expect(rls).To(BeNil())
				return
			}

			g.Expect(err).ToNot(HaveOccurred())
			g.Expect(rls).To(Equal(tt.want))
		})
	}
}

func TestVerifyReleaseObject(t *testing.T) {
	mockRls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
		Name:      "release",
		Version:   1,
		Status:    helmrelease.StatusSuperseded,
		Namespace: "default",
	})
	mockInfo := release.ObservedToInfo(release.ObserveRelease(mockRls))
	mockInfoIllegal := mockInfo.DeepCopy()
	mockInfoIllegal.Digest = "illegal"

	tests := []struct {
		name    string
		info    *v2.HelmReleaseInfo
		rls     *helmrelease.Release
		wantErr error
	}{
		{
			name: "valid digest",
			info: mockInfo,
			rls:  mockRls,
		},
		{
			name:    "illegal digest",
			info:    mockInfoIllegal,
			wantErr: ErrReleaseDigest,
		},
		{
			name: "invalid digest",
			info: mockInfo,
			rls: testutil.BuildRelease(&helmrelease.MockReleaseOptions{
				Name:      "release",
				Version:   1,
				Status:    helmrelease.StatusDeployed,
				Namespace: "default",
			}),
			wantErr: ErrReleaseNotObserved,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := VerifyReleaseObject(tt.info, tt.rls)

			if tt.wantErr != nil {
				g.Expect(got).To(HaveOccurred())
				g.Expect(got).To(Equal(tt.wantErr))
				return
			}

			g.Expect(got).NotTo(HaveOccurred())
		})
	}
}

func TestVerifyRelease(t *testing.T) {
	mockRls := testutil.BuildRelease(&helmrelease.MockReleaseOptions{
		Name:      "release",
		Version:   1,
		Status:    helmrelease.StatusSuperseded,
		Namespace: "default",
	})
	mockInfo := release.ObservedToInfo(release.ObserveRelease(mockRls))

	tests := []struct {
		name    string
		rls     *helmrelease.Release
		info    *v2.HelmReleaseInfo
		chrt    *helmchart.Metadata
		vals    chartutil.Values
		wantErr error
	}{
		{
			name: "equal",
			rls:  mockRls,
			info: mockInfo,
			chrt: mockRls.Chart.Metadata,
			vals: mockRls.Config,
		},
		{
			name:    "no release",
			rls:     nil,
			info:    mockInfo,
			chrt:    mockRls.Chart.Metadata,
			vals:    mockRls.Config,
			wantErr: ErrReleaseNotFound,
		},
		{
			name:    "no release info",
			rls:     mockRls,
			info:    nil,
			chrt:    mockRls.Chart.Metadata,
			vals:    mockRls.Config,
			wantErr: ErrConfigDigest,
		},
		{
			name: "chart meta diff",
			rls:  mockRls,
			info: mockInfo,
			chrt: &helmchart.Metadata{
				Name:    "some-other-chart",
				Version: "1.0.0",
			},
			vals:    mockRls.Config,
			wantErr: ErrChartChanged,
		},
		{
			name: "chart values diff",
			rls:  mockRls,
			info: mockInfo,
			chrt: mockRls.Chart.Metadata,
			vals: chartutil.Values{
				"some": "other",
			},
			wantErr: ErrConfigDigest,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			got := VerifyRelease(tt.rls, tt.info, tt.chrt, tt.vals)

			if tt.wantErr != nil {
				g.Expect(got).To(HaveOccurred())
				g.Expect(got).To(Equal(tt.wantErr))
				return
			}

			g.Expect(got).ToNot(HaveOccurred())
		})
	}
}
