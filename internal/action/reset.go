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
	"github.com/opencontainers/go-digest"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	intchartutil "github.com/fluxcd/helm-controller/internal/chartutil"
)

const (
	differentGenerationReason = "generation differs from last attempt"
	differentRevisionReason   = "chart version differs from last attempt"
	differentValuesReason     = "values differ from last attempt"
	resetRequestedReason      = "reset requested through annotation"
)

// MustResetFailures returns a reason and true if the HelmRelease's status
// indicates that the HelmRelease failure counters must be reset.
// This is the case if the data used to make the last (failed) attempt has
// changed in a way that indicates that a new attempt should be made.
// For example, a change in generation, chart version, or values.
// If no change is detected, an empty string is returned along with false.
func MustResetFailures(obj *v2.HelmRelease, chart *chart.Metadata, values chartutil.Values) (string, bool) {
	// Always check if a reset is requested.
	// This is done first, so that the HelmReleaseStatus.LastHandledResetAt
	// field is updated even if the reset request is not handled due to other
	// diverging data.
	resetRequested := v2.ShouldHandleResetRequest(obj)

	switch {
	case obj.Status.LastAttemptedGeneration != obj.Generation:
		return differentGenerationReason, true
	case obj.Status.GetLastAttemptedRevision() != chart.Version:
		return differentRevisionReason, true
	case obj.Status.LastAttemptedConfigDigest != "" || obj.Status.LastAttemptedValuesChecksum != "":
		d := obj.Status.LastAttemptedConfigDigest
		if d == "" {
			// TODO: remove this when the deprecated field is removed.
			d = "sha1:" + obj.Status.LastAttemptedValuesChecksum
		}
		if ok := intchartutil.VerifyValues(digest.Digest(d), values); !ok {
			return differentValuesReason, true
		}
	}

	if resetRequested {
		return resetRequestedReason, true
	}

	return "", false
}
