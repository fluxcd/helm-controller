/*
Copyright 2023 The Flux authors

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
	"testing"

	. "github.com/onsi/gomega"
	"github.com/matheuscscp/helm/pkg/chart"
	"github.com/matheuscscp/helm/pkg/chartutil"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/fluxcd/pkg/apis/meta"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

func TestMustResetFailures(t *testing.T) {
	tests := []struct {
		name       string
		obj        *v2.HelmRelease
		chart      *chart.Metadata
		values     chartutil.Values
		want       bool
		wantReason string
	}{
		{
			name: "on generation change",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: v2.HelmReleaseStatus{
					LastAttemptedGeneration: 2,
				},
			},
			want:       true,
			wantReason: differentGenerationReason,
		},
		{
			name: "on revision change",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: v2.HelmReleaseStatus{
					LastAttemptedGeneration: 1,
					LastAttemptedRevision:   "1.0.0",
				},
			},
			chart: &chart.Metadata{
				Version: "1.1.0",
			},
			want:       true,
			wantReason: differentRevisionReason,
		},
		{
			name: "on config digest change",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: v2.HelmReleaseStatus{
					LastAttemptedGeneration:   1,
					LastAttemptedRevision:     "1.0.0",
					LastAttemptedConfigDigest: "sha256:9933f58f8bf459eb199d59ebc8a05683f3944e1242d9f5467d99aa2cf08a5370",
				},
			},
			chart: &chart.Metadata{
				Version: "1.0.0",
			},
			values: chartutil.Values{
				"foo": "bar",
			},
			want:       true,
			wantReason: differentValuesReason,
		},
		{
			name: "on (deprecated) values checksum change",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: v2.HelmReleaseStatus{
					LastAttemptedGeneration:     1,
					LastAttemptedRevision:       "1.0.0",
					LastAttemptedValuesChecksum: "a856118d270c0db44a9019d51e2bba4fc3e6bac7",
				},
			},
			chart: &chart.Metadata{
				Version: "1.0.0",
			},
			values: chartutil.Values{
				"foo": "bar",
			},
			want:       true,
			wantReason: differentValuesReason,
		},
		{
			name: "on reset request through annotation",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
					Annotations: map[string]string{
						meta.ReconcileRequestAnnotation: "a",
						v2.ResetRequestAnnotation:       "a",
					},
				},
				Status: v2.HelmReleaseStatus{
					LastAttemptedGeneration:   1,
					LastAttemptedRevision:     "1.0.0",
					LastAttemptedConfigDigest: "sha256:1dabc4e3cbbd6a0818bd460f3a6c9855bfe95d506c74726bc0f2edb0aecb1f4e",
				},
			},
			chart: &chart.Metadata{
				Version: "1.0.0",
			},
			values: chartutil.Values{
				"foo": "bar",
			},
			want:       true,
			wantReason: resetRequestedReason,
		},
		{
			name: "without change no reset",
			obj: &v2.HelmRelease{
				ObjectMeta: metav1.ObjectMeta{
					Generation: 1,
				},
				Status: v2.HelmReleaseStatus{
					LastAttemptedGeneration:   1,
					LastAttemptedRevision:     "1.0.0",
					LastAttemptedConfigDigest: "sha256:1dabc4e3cbbd6a0818bd460f3a6c9855bfe95d506c74726bc0f2edb0aecb1f4e",
				},
			},
			chart: &chart.Metadata{
				Version: "1.0.0",
			},
			values: chartutil.Values{
				"foo": "bar",
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			reason, got := MustResetFailures(tt.obj, tt.chart, tt.values)
			g.Expect(got).To(Equal(tt.want))
			g.Expect(reason).To(Equal(tt.wantReason))
		})
	}
}
