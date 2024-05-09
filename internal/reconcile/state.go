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
	"context"
	"errors"
	"fmt"

	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	"github.com/fluxcd/pkg/ssa/jsondiff"
	"helm.sh/helm/v3/pkg/kube"
	helmrelease "helm.sh/helm/v3/pkg/release"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/digest"
	interrors "github.com/fluxcd/helm-controller/internal/errors"
	"github.com/fluxcd/helm-controller/internal/postrender"
)

// ReleaseStatus represents the status of a Helm release as determined by
// comparing the Helm storage with the v2.HelmRelease object.
type ReleaseStatus string

// String returns the string representation of the release status.
func (s ReleaseStatus) String() string {
	return string(s)
}

const (
	// ReleaseStatusUnknown indicates that the status of the release could not
	// be determined.
	ReleaseStatusUnknown ReleaseStatus = "Unknown"
	// ReleaseStatusAbsent indicates that the release is not present in the
	// Helm storage.
	ReleaseStatusAbsent ReleaseStatus = "Absent"
	// ReleaseStatusUnmanaged indicates that the release is present in the Helm
	// storage, but is not managed by the v2.HelmRelease object.
	ReleaseStatusUnmanaged ReleaseStatus = "Unmanaged"
	// ReleaseStatusOutOfSync indicates that the release is present in the Helm
	// storage, but is not in sync with the v2.HelmRelease object.
	ReleaseStatusOutOfSync ReleaseStatus = "OutOfSync"
	// ReleaseStatusDrifted indicates that the release is present in the Helm
	// storage, but the cluster state has drifted from the manifest in the
	// storage.
	ReleaseStatusDrifted ReleaseStatus = "Drifted"
	// ReleaseStatusLocked indicates that the release is present in the Helm
	// storage, but is locked.
	ReleaseStatusLocked ReleaseStatus = "Locked"
	// ReleaseStatusUntested indicates that the release is present in the Helm
	// storage, but has not been tested.
	ReleaseStatusUntested ReleaseStatus = "Untested"
	// ReleaseStatusInSync indicates that the release is present in the Helm
	// storage, and is in sync with the v2.HelmRelease object.
	ReleaseStatusInSync ReleaseStatus = "InSync"
	// ReleaseStatusFailed indicates that the release is present in the Helm
	// storage, but has failed.
	ReleaseStatusFailed ReleaseStatus = "Failed"
)

// ReleaseState represents the state of a Helm release as determined by
// comparing the Helm storage with the v2.HelmRelease object.
type ReleaseState struct {
	// Status is the status of the release.
	Status ReleaseStatus
	// Reason for the Status.
	Reason string
	// Diff contains any differences between the Helm storage manifest and the
	// cluster state when Status equals ReleaseStatusDrifted.
	Diff jsondiff.DiffSet
}

// DetermineReleaseState determines the state of the Helm release as compared
// to the v2.HelmRelease object. It returns a ReleaseState that indicates
// the status of the release, and an error if the state could not be determined.
func DetermineReleaseState(ctx context.Context, cfg *action.ConfigFactory, req *Request) (ReleaseState, error) {
	rls, err := action.LastRelease(cfg.Build(nil), req.Object.GetReleaseName())
	if err != nil {
		if errors.Is(err, action.ErrReleaseNotFound) {
			return ReleaseState{Status: ReleaseStatusAbsent, Reason: "no release in storage for object"}, nil
		}
		return ReleaseState{Status: ReleaseStatusUnknown}, fmt.Errorf("failed to retrieve last release from storage: %w", err)
	}

	// If the release is in a pending state, it must be unlocked before any
	// further action can be taken.
	if rls.Info.Status.IsPending() {
		return ReleaseState{Status: ReleaseStatusLocked, Reason: fmt.Sprintf("release with status '%s'", rls.Info.Status)}, err
	}

	// Confirm we have a release object to compare against.
	if req.Object.Status.History.Len() == 0 {
		if rls.Info.Status == helmrelease.StatusUninstalled {
			return ReleaseState{Status: ReleaseStatusAbsent, Reason: "found uninstalled release in storage"}, nil
		}
		return ReleaseState{Status: ReleaseStatusUnmanaged, Reason: "found existing release in storage"}, err
	}

	// Verify the release object against the state we observed during our
	// last reconciliation.
	cur := req.Object.Status.History.Latest()
	if err := action.VerifyReleaseObject(cur, rls); err != nil {
		if interrors.IsOneOf(err, action.ErrReleaseDigest, action.ErrReleaseNotObserved) {
			// The release object has been mutated in such a way that we are
			// unable to determine the state of the release.
			// Effectively, this means that the object no longer manages the
			// release, and we should e.g. perform an upgrade to bring
			// the release back in-sync and under management.
			return ReleaseState{Status: ReleaseStatusUnmanaged, Reason: err.Error()}, nil
		}
		return ReleaseState{Status: ReleaseStatusUnknown}, fmt.Errorf("failed to verify release object: %w", err)
	}

	// Further determine the state of the release based on the Helm release
	// status, which can now be considered reliable.
	switch rls.Info.Status {
	case helmrelease.StatusFailed:
		return ReleaseState{Status: ReleaseStatusFailed}, nil
	case helmrelease.StatusUninstalled:
		return ReleaseState{Status: ReleaseStatusAbsent, Reason: "found uninstalled release in storage"}, nil
	case helmrelease.StatusDeployed:
		// Verify the release is in sync with the desired configuration.
		if err = action.VerifyRelease(rls, cur, req.Chart.Metadata, req.Values); err != nil {
			switch err {
			case action.ErrChartChanged, action.ErrConfigDigest:
				return ReleaseState{Status: ReleaseStatusOutOfSync, Reason: err.Error()}, nil
			default:
				return ReleaseState{Status: ReleaseStatusUnknown}, err
			}
		}

		// Verify if postrender digest has changed if config has not been
		// processed. For the processed or partially processed generation, the
		// updated observation will only be reflected at the end of a successful
		// reconciliation.  Comparing here would result the reconciliation to
		// get stuck in this check due to a mismatch forever.  The value can't
		// change without a new generation. Hence, compare the observed digest
		// for new generations only.
		ready := conditions.Get(req.Object, meta.ReadyCondition)
		if ready != nil && ready.ObservedGeneration != req.Object.Generation {
			var postrenderersDigest string
			if req.Object.Spec.PostRenderers != nil {
				postrenderersDigest = postrender.Digest(digest.Canonical, req.Object.Spec.PostRenderers).String()
			}
			if postrenderersDigest != req.Object.Status.ObservedPostRenderersDigest {
				return ReleaseState{Status: ReleaseStatusOutOfSync, Reason: "postrenderers digest has changed"}, nil
			}
		}

		// For the further determination of test results, we look at the
		// observed state of the object. As tests can be run manually by
		// users running e.g. `helm test`.
		if testSpec := req.Object.GetTest(); testSpec.Enable {
			// Confirm the release has been tested if enabled.
			if !cur.HasBeenTested() {
				return ReleaseState{Status: ReleaseStatusUntested}, nil
			}

			// Act on any observed test failure.
			remediation := req.Object.GetActiveRemediation()
			if remediation != nil && !remediation.MustIgnoreTestFailures(testSpec.IgnoreFailures) && cur.HasTestInPhase(helmrelease.HookPhaseFailed.String()) {
				return ReleaseState{Status: ReleaseStatusFailed, Reason: "release has test in failed phase"}, nil
			}
		}

		// Confirm the cluster state matches the desired config.
		if diffOpts := req.Object.GetDriftDetection(); diffOpts.MustDetectChanges() {
			diffSet, err := action.Diff(ctx, cfg.Build(nil), rls, kube.ManagedFieldsManager, req.Object.GetDriftDetection().Ignore...)
			hasChanges := diffSet.HasChanges()
			if err != nil {
				if !hasChanges {
					return ReleaseState{Status: ReleaseStatusUnknown}, fmt.Errorf("unable to determine cluster state: %w", err)
				}
				ctrl.LoggerFrom(ctx).Error(err, "diff of release against cluster state completed with error")
			}
			if hasChanges {
				return ReleaseState{Status: ReleaseStatusDrifted, Diff: diffSet}, nil
			}
		}

		return ReleaseState{Status: ReleaseStatusInSync}, nil
	default:
		return ReleaseState{Status: ReleaseStatusUnknown}, fmt.Errorf("unable to determine state for release with status '%s'", rls.Info.Status)
	}
}
