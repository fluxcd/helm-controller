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
	"strings"

	helmrelease "helm.sh/helm/v3/pkg/release"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
)

// Unlock is an ActionReconciler which attempts to unlock the latest release
// for a Request.Object in the Helm storage if stuck in a pending state, by
// setting the status to release.StatusFailed and persisting it.
//
// This write to the Helm storage is observed, and updates the Status.History
// field if the persisted object targets the same release version.
//
// Any pending state marks the v2.HelmRelease object with
// ReleasedCondition=False, even if persisting the object to the Helm storage
// fails.
//
// At the end of the reconciliation, the Status.Conditions are summarized and
// propagated to the Ready condition on the Request.Object.
type Unlock struct {
	configFactory *action.ConfigFactory
	eventRecorder record.EventRecorder
}

// NewUnlock returns a new Unlock reconciler configured with the provided
// values.
func NewUnlock(cfg *action.ConfigFactory, recorder record.EventRecorder) *Unlock {
	return &Unlock{configFactory: cfg, eventRecorder: recorder}
}

func (r *Unlock) Reconcile(_ context.Context, req *Request) error {
	defer summarize(req)

	// Build action configuration to gain access to Helm storage.
	cfg := r.configFactory.Build(nil, observeUnlock(req.Object))

	// Retrieve last release object.
	rls, err := action.LastRelease(cfg, req.Object.GetReleaseName())
	if err != nil {
		// Ignore not found error. Assume caller will decide what to do
		// when it re-assess state to determine the next action.
		if errors.Is(err, action.ErrReleaseNotFound) {
			return nil
		}
		// Return any other error to retry.
		return err
	}

	// Ensure the release is in a pending state.
	cur := processCurrentSnaphot(req.Object, rls)
	if status := rls.Info.Status; status.IsPending() {
		// Update pending status to failed and persist.
		rls.SetStatus(helmrelease.StatusFailed, fmt.Sprintf("Release unlocked from stale '%s' state", status.String()))
		if err = cfg.Releases.Update(rls); err != nil {
			r.failure(req, cur, status, err)
			return err
		}
		r.success(req, cur, status)
	}
	return nil
}

func (r *Unlock) Name() string {
	return "unlock"
}

func (r *Unlock) Type() ReconcilerType {
	return ReconcilerTypeUnlock
}

const (
	// fmtUnlockFailure is the message format for an unlock failure.
	fmtUnlockFailure = "Unlock of Helm release %s with chart %s in %s state failed: %s"
	// fmtUnlockSuccess is the message format for a successful unlock.
	fmtUnlockSuccess = "Unlocked Helm release %s with chart %s in %s state"
)

// failure records the failure of an unlock action in the status of the given
// Request.Object by marking ReleasedCondition=False and increasing the failure
// counter. In addition, it emits a warning event for the Request.Object.
func (r *Unlock) failure(req *Request, cur *v2.Snapshot, status helmrelease.Status, err error) {
	// Compose failure message.
	msg := fmt.Sprintf(fmtUnlockFailure, cur.FullReleaseName(), cur.VersionedChartName(), status.String(), strings.TrimSpace(err.Error()))

	// Mark unlock failure on object.
	req.Object.Status.Failures++
	conditions.MarkFalse(req.Object, v2.ReleasedCondition, "PendingRelease", "%s", msg)

	// Record warning event.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(cur.ChartVersion, cur.ConfigDigest, addAppVersion(cur.AppVersion), addOCIDigest(cur.OCIDigest)),
		corev1.EventTypeWarning,
		"PendingRelease",
		msg,
	)
}

// success records the success of an unlock action in the status of the given
// Request.Object by marking ReleasedCondition=False and emitting an event.
func (r *Unlock) success(req *Request, cur *v2.Snapshot, status helmrelease.Status) {
	// Compose success message.
	msg := fmt.Sprintf(fmtUnlockSuccess, cur.FullReleaseName(), cur.VersionedChartName(), status.String())

	// Mark unlock success on object.
	conditions.MarkFalse(req.Object, v2.ReleasedCondition, "PendingRelease", "%s", msg)

	// Record event.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(cur.ChartVersion, cur.ConfigDigest, addAppVersion(cur.AppVersion), addOCIDigest(cur.OCIDigest)),
		corev1.EventTypeNormal,
		"PendingRelease",
		msg,
	)
}

// observeUnlock returns a storage.ObserveFunc to track unlocking actions on
// a HelmRelease.
// It updates the snapshot of a release when an unlock action is observed for
// that release.
func observeUnlock(obj *v2.HelmRelease) storage.ObserveFunc {
	return func(rls *helmrelease.Release) {
		for i := range obj.Status.History {
			snap := obj.Status.History[i]
			if snap.Targets(rls.Name, rls.Namespace, rls.Version) {
				obj.Status.History[i] = release.ObservedToSnapshot(releaseToObservation(rls, snap))
				return
			}
		}
	}
}

// processCurrentSnaphot processes the current snapshot based on a Helm release.
// It also looks for the OCIDigest in the corresponding v2.HelmRelease history and
// updates the current snapshot with the OCIDigest if found.
func processCurrentSnaphot(obj *v2.HelmRelease, rls *helmrelease.Release) *v2.Snapshot {
	cur := release.ObservedToSnapshot(release.ObserveRelease(rls))
	for i := range obj.Status.History {
		snap := obj.Status.History[i]
		if snap.Targets(rls.Name, rls.Namespace, rls.Version) {
			cur.OCIDigest = snap.OCIDigest
		}
	}
	return cur
}
