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

	helmchart "helm.sh/helm/v3/pkg/chart"
	helmchartutil "helm.sh/helm/v3/pkg/chartutil"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

const (
	// ReconcilerTypeRelease is an ActionReconciler which produces a new
	// Helm release.
	ReconcilerTypeRelease ReconcilerType = "release"
	// ReconcilerTypeRemediate is an ActionReconciler which remediates a
	// failed Helm release.
	ReconcilerTypeRemediate ReconcilerType = "remediate"
	// ReconcilerTypeTest is an ActionReconciler which tests a Helm release.
	ReconcilerTypeTest ReconcilerType = "test"
	// ReconcilerTypeUnlock is an ActionReconciler which unlocks a Helm
	// release in a stale pending state. It differs from ReconcilerTypeRemediate
	// in that it does not produce a new Helm release.
	ReconcilerTypeUnlock ReconcilerType = "unlock"
	// ReconcilerTypeDriftCorrection is an ActionReconciler which corrects
	// Helm releases which have drifted from the cluster state.
	ReconcilerTypeDriftCorrection ReconcilerType = "drift correction"
)

// ReconcilerType is a string which identifies the type of ActionReconciler.
// It can be used to e.g. limiting the number of action (types) to be performed
// in a single reconciliation.
type ReconcilerType string

// ReconcilerTypeSet is a set of ReconcilerType.
type ReconcilerTypeSet []ReconcilerType

// Contains returns true if the set contains the given type.
func (s ReconcilerTypeSet) Contains(t ReconcilerType) bool {
	for _, r := range s {
		if r == t {
			return true
		}
	}
	return false
}

// Count returns the number of elements matching the given type.
func (s ReconcilerTypeSet) Count(t ReconcilerType) int {
	count := 0
	for _, r := range s {
		if r == t {
			count++
		}
	}
	return count
}

// Request is a request to be performed by an ActionReconciler. The reconciler
// writes the result of the request to the Object's status.
type Request struct {
	// Object is the Helm release to be reconciled, and describes the desired
	// state to the ActionReconciler.
	Object *v2.HelmRelease
	// Chart is the Helm chart to be installed or upgraded.
	Chart *helmchart.Chart
	// Values is the Helm chart values to be used for the installation or
	// upgrade.
	Values helmchartutil.Values
}

// ActionReconciler is an interface which defines the methods that a reconciler
// of a Helm action must implement.
type ActionReconciler interface {
	// Reconcile performs the reconcile action for the given Request. The
	// reconciler should write the result of the request to the Object's status.
	// An error is returned if the reconcile action cannot be performed and did
	// not result in a modification of the Helm storage. The caller should then
	// either retry, or abort the operation.
	Reconcile(ctx context.Context, req *Request) error
	// Name returns the name of the ActionReconciler. Typically, this equals
	// the name of the Helm action it performs.
	Name() string
	// Type returns the ReconcilerType of the ActionReconciler.
	Type() ReconcilerType
}
