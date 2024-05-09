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
	"sort"

	eventv1 "github.com/fluxcd/pkg/apis/event/v1beta1"
	"github.com/fluxcd/pkg/apis/meta"
	"github.com/fluxcd/pkg/runtime/conditions"
	helmrelease "helm.sh/helm/v3/pkg/release"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v2 "github.com/fluxcd/helm-controller/api/v2"
	"github.com/fluxcd/helm-controller/internal/action"
	"github.com/fluxcd/helm-controller/internal/release"
	"github.com/fluxcd/helm-controller/internal/storage"
)

var (
	// ErrNoLatest is returned when the HelmRelease has no latest release
	// but this is required by the ActionReconciler.
	ErrNoLatest = errors.New("no latest release")
	// ErrReleaseMismatch is returned when the resulting release after running
	// an action does not match the expected latest and/or previous release.
	// This can happen for actions where targeting a release by version is not
	// possible, for example while running tests.
	ErrReleaseMismatch = errors.New("release mismatch")
)

// mutateObservedRelease is a function that mutates the Observation with the
// given HelmRelease object.
type mutateObservedRelease func(*v2.HelmRelease, release.Observation) release.Observation

// observedReleases is a map of Helm releases as observed to be written to the
// Helm storage. The key is the version of the release.
type observedReleases map[int]release.Observation

// sortedVersions returns the versions of the observed releases in descending
// order.
func (r observedReleases) sortedVersions() (versions []int) {
	for ver := range r {
		versions = append(versions, ver)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(versions)))
	return
}

// recordOnObject records the observed releases on the HelmRelease object.
func (r observedReleases) recordOnObject(obj *v2.HelmRelease, mutators ...mutateObservedRelease) {
	switch len(r) {
	case 0:
		return
	case 1:
		var obs release.Observation
		for _, o := range r {
			obs = o
		}
		for _, mut := range mutators {
			obs = mut(obj, obs)
		}
		obj.Status.History = append(v2.Snapshots{release.ObservedToSnapshot(obs)}, obj.Status.History...)
	default:
		versions := r.sortedVersions()
		obs := r[versions[0]]
		for _, mut := range mutators {
			obs = mut(obj, obs)
		}
		obj.Status.History = append(v2.Snapshots{release.ObservedToSnapshot(obs)}, obj.Status.History...)

		for _, ver := range versions[1:] {
			for i := range obj.Status.History {
				snap := obj.Status.History[i]
				if snap.Targets(r[ver].Name, r[ver].Namespace, r[ver].Version) {
					obs := r[ver]
					obs.OCIDigest = snap.OCIDigest
					newSnap := release.ObservedToSnapshot(obs)
					newSnap.SetTestHooks(snap.GetTestHooks())
					obj.Status.History[i] = newSnap
					return
				}
			}
		}
	}
}

func mutateOCIDigest(obj *v2.HelmRelease, obs release.Observation) release.Observation {
	obs.OCIDigest = obj.Status.LastAttemptedRevisionDigest
	return obs
}

func releaseToObservation(rls *helmrelease.Release, snapshot *v2.Snapshot) release.Observation {
	obs := release.ObserveRelease(rls)
	obs.OCIDigest = snapshot.OCIDigest
	return obs
}

// observeRelease returns a storage.ObserveFunc that stores the observed
// releases in the given observedReleases map.
// It can be used for Helm actions that modify multiple releases in the
// Helm storage, such as install and upgrade.
func observeRelease(observed observedReleases) storage.ObserveFunc {
	return func(rls *helmrelease.Release) {
		obs := release.ObserveRelease(rls)
		observed[obs.Version] = obs
	}
}

// summarize composes a Ready condition out of the Remediated, TestSuccess and
// Released conditions of the given Request.Object, and sets it on the object.
//
// The composition is made by sorting them by highest generation and priority
// of the summary conditions, taking the first result.
//
// Not taking the generation of the object itself into account ensures that if
// the change in generation of the resource does not result in a release, the
// Ready condition is still reflected for the current generation based on a
// release made for the previous generation.
//
// It takes the current specification of the object into account, and deals
// with the conditional handling of TestSuccess. Deleting the condition when
// tests are not enabled, and excluding it when failures must be ignored.
//
// If Ready=True, any Stalled condition is removed.
//
// The ObservedPostRenderersDigest is updated if the post-renderers exist.
func summarize(req *Request) {
	var sumConds = []string{v2.RemediatedCondition, v2.ReleasedCondition}
	if req.Object.GetTest().Enable && !req.Object.GetTest().IgnoreFailures {
		sumConds = []string{v2.RemediatedCondition, v2.TestSuccessCondition, v2.ReleasedCondition}
	}

	// Remove any stale TestSuccess condition as soon as tests are disabled.
	if !req.Object.GetTest().Enable {
		conditions.Delete(req.Object, v2.TestSuccessCondition)
	}

	conds := req.Object.Status.Conditions
	if len(conds) == 0 {
		// Nothing to summarize if there are no conditions.
		return
	}

	sort.SliceStable(conds, func(i, j int) bool {
		iPos, ok := inStringSlice(sumConds, conds[i].Type)
		if !ok {
			return false
		}

		jPos, ok := inStringSlice(sumConds, conds[j].Type)
		if !ok {
			return true
		}

		return (conds[i].ObservedGeneration >= conds[j].ObservedGeneration) && (iPos < jPos)
	})

	status := conds[0].Status

	// Any remediated state is considered an error.
	if conds[0].Type == v2.RemediatedCondition {
		status = metav1.ConditionFalse
	}

	if status == metav1.ConditionTrue {
		conditions.Delete(req.Object, meta.StalledCondition)
	}

	conditions.Set(req.Object, &metav1.Condition{
		Type:               meta.ReadyCondition,
		Status:             status,
		Reason:             conds[0].Reason,
		Message:            conds[0].Message,
		ObservedGeneration: req.Object.Generation,
	})
}

// eventMessageWithLog returns an event message composed out of the given
// message and any log messages by appending them to the message.
func eventMessageWithLog(msg string, log *action.LogBuffer) string {
	if log != nil && log.Len() > 0 {
		msg = msg + "\n\nLast Helm logs:\n\n" + log.String()
	}
	return msg
}

// addMeta is a function that adds metadata to an event map.
type addMeta func(map[string]string)

const (
	// metaOCIDigestKey is the key for the chart OCI artifact digest.
	metaOCIDigestKey = "oci-digest"

	// metaAppVersionKey is the key for the app version found in chart metadata.
	metaAppVersionKey = "app-version"
)

// eventMeta returns the event (annotation) metadata based on the given
// parameters.
func eventMeta(revision, token string, metas ...addMeta) map[string]string {
	var metadata map[string]string
	if revision != "" || token != "" {
		metadata = make(map[string]string)
		if revision != "" {
			metadata[eventMetaGroupKey(eventv1.MetaRevisionKey)] = revision
		}
		if token != "" {
			metadata[eventMetaGroupKey(eventv1.MetaTokenKey)] = token
		}
	}

	for _, add := range metas {
		add(metadata)
	}

	return metadata
}

func addOCIDigest(digest string) addMeta {
	return func(m map[string]string) {
		if digest != "" {
			if m == nil {
				m = make(map[string]string)
			}
			m[eventMetaGroupKey(metaOCIDigestKey)] = digest
		}
	}
}

func addAppVersion(appVersion string) addMeta {
	return func(m map[string]string) {
		if appVersion != "" {
			if m == nil {
				m = make(map[string]string)
			}
			m[eventMetaGroupKey(metaAppVersionKey)] = appVersion
		}
	}
}

// eventMetaGroupKey returns the event (annotation) metadata key prefixed with
// the group.
func eventMetaGroupKey(key string) string {
	return v2.GroupVersion.Group + "/" + key
}
