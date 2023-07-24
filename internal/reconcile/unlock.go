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
	helmdriver "helm.sh/helm/v3/pkg/storage/driver"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"

	"github.com/fluxcd/pkg/runtime/conditions"

	v2 "github.com/fluxcd/helm-controller/api/v2beta2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
)

// Unlock is an ActionReconciler which attempts to unlock the Status.Current
// of a Request.Object in the Helm storage if stuck in a pending state, by
// setting the status to release.StatusFailed and persisting it.
//
// This write to the Helm storage is observed, and updates the Status.Current
// field if the persisted object targets the same release version.
//
// Any pending state marks the v2beta2.HelmRelease object with
// ReleasedCondition=False, even if persisting the object to the Helm storage
// fails.
//
// If the Request.Object does not have a Status.Current, an ErrNoCurrent error
// is returned.
//
// At the end of the reconciliation, the Status.Conditions are summarized and
// propagated to the Ready condition on the Request.Object.
//
// The caller is assumed to have verified the integrity of Request.Object using
// e.g. action.VerifyReleaseInfo before calling Reconcile.
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
	var (
		cur = req.Object.GetCurrent().DeepCopy()
	)

	defer summarize(req)

	// We can only unlock a release if we have a current.
	if cur == nil {
		return fmt.Errorf("%w: required for unlock", ErrNoCurrent)
	}

	// Build action configuration to gain access to Helm storage.
	cfg := r.configFactory.Build(nil, observeUnlock(req.Object))

	// Retrieve last release object.
	rls, err := cfg.Releases.Get(cur.Name, cur.Version)
	if err != nil {
		// Ignore not found error. Assume caller will decide what to do
		// when it re-assess state to determine the next action.
		if errors.Is(err, helmdriver.ErrReleaseNotFound) {
			return nil
		}
		// Return any other error to retry.
		return err
	}

	// Ensure the release is in a pending state.
	if status := rls.Info.Status; status.IsPending() {
		// Update pending status to failed and persist.
		rls.SetStatus(helmrelease.StatusFailed, fmt.Sprintf("Release unlocked from stale '%s' state", status.String()))
		if err = cfg.Releases.Update(rls); err != nil {
			r.failure(req, status, err)
			return err
		}
		r.success(req, status)
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
	fmtUnlockFailure = "Unlock of release %s with chart %s in %s state failed: %s"
	// fmtUnlockSuccess is the message format for a successful unlock.
	fmtUnlockSuccess = "Unlocked release %s with chart %s in %s state"
)

// failure records the failure of an unlock action in the status of the given
// Request.Object by marking ReleasedCondition=False and increasing the failure
// counter. In addition, it emits a warning event for the Request.Object.
func (r *Unlock) failure(req *Request, status helmrelease.Status, err error) {
	// Compose failure message.
	cur := req.Object.GetCurrent()
	msg := fmt.Sprintf(fmtUnlockFailure, cur.FullReleaseName(), cur.VersionedChartName(), status.String(), strings.TrimSpace(err.Error()))

	// Mark unlock failure on object.
	req.Object.Status.Failures++
	conditions.MarkFalse(req.Object, v2.ReleasedCondition, "PendingRelease", msg)

	// Record warning event.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(cur.ChartVersion, cur.ConfigDigest),
		corev1.EventTypeWarning,
		"PendingRelease",
		msg,
	)
}

// success records the success of an unlock action in the status of the given
// Request.Object by marking ReleasedCondition=False and emitting an event.
func (r *Unlock) success(req *Request, status helmrelease.Status) {
	// Compose success message.
	cur := req.Object.GetCurrent()
	msg := fmt.Sprintf(fmtUnlockSuccess, cur.FullReleaseName(), cur.VersionedChartName(), status.String())

	// Mark unlock success on object.
	conditions.MarkFalse(req.Object, v2.ReleasedCondition, "PendingRelease", msg)

	// Record event.
	r.eventRecorder.AnnotatedEventf(
		req.Object,
		eventMeta(cur.ChartVersion, cur.ConfigDigest),
		corev1.EventTypeNormal,
		"PendingRelease",
		msg,
	)
}

// observeUnlock returns a storage.ObserveFunc that can be used to observe and
// record the result of an unlock action in the status of the given release.
// It updates the Status.Current field of the release if it equals the target
// of the unlock action.
func observeUnlock(obj *v2.HelmRelease) storage.ObserveFunc {
	return func(rls *helmrelease.Release) {
		if cur := obj.GetCurrent(); cur != nil {
			obs := release.ObserveRelease(rls)
			if obs.Targets(cur.Name, cur.Namespace, cur.Version) {
				obj.Status.Current = release.ObservedToInfo(obs)
			}
		}
	}
}
