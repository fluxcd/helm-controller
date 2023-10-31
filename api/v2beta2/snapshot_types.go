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

package v2beta2

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Snapshot captures a point-in-time copy of the status information for a Helm release,
// as managed by the controller.
type Snapshot struct {
	// APIVersion is the API version of the Snapshot.
	// Provisional: when the calculation method of the Digest field is changed,
	// this field will be used to distinguish between the old and new methods.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
	// Digest is the checksum of the release object in storage.
	// It has the format of `<algo>:<checksum>`.
	// +required
	Digest string `json:"digest"`
	// Name is the name of the release.
	// +required
	Name string `json:"name"`
	// Namespace is the namespace the release is deployed to.
	// +required
	Namespace string `json:"namespace"`
	// Version is the version of the release object in storage.
	// +required
	Version int `json:"version"`
	// Status is the current state of the release.
	// +required
	Status string `json:"status"`
	// ChartName is the chart name of the release object in storage.
	// +required
	ChartName string `json:"chartName"`
	// ChartVersion is the chart version of the release object in
	// storage.
	// +required
	ChartVersion string `json:"chartVersion"`
	// ConfigDigest is the checksum of the config (better known as
	// "values") of the release object in storage.
	// It has the format of `<algo>:<checksum>`.
	// +required
	ConfigDigest string `json:"configDigest"`
	// FirstDeployed is when the release was first deployed.
	// +required
	FirstDeployed metav1.Time `json:"firstDeployed"`
	// LastDeployed is when the release was last deployed.
	// +required
	LastDeployed metav1.Time `json:"lastDeployed"`
	// Deleted is when the release was deleted.
	// +optional
	Deleted metav1.Time `json:"deleted,omitempty"`
	// TestHooks is the list of test hooks for the release as observed to be
	// run by the controller.
	// +optional
	TestHooks *map[string]*TestHookStatus `json:"testHooks,omitempty"`
}

// FullReleaseName returns the full name of the release in the format
// of '<namespace>/<name>.<version>
func (in *Snapshot) FullReleaseName() string {
	return fmt.Sprintf("%s/%s.v%d", in.Namespace, in.Name, in.Version)
}

// VersionedChartName returns the full name of the chart in the format of
// '<name>@<version>'.
func (in *Snapshot) VersionedChartName() string {
	return fmt.Sprintf("%s@%s", in.ChartName, in.ChartVersion)
}

// HasBeenTested returns true if TestHooks is not nil. This includes an empty
// map, which indicates the chart has no tests.
func (in *Snapshot) HasBeenTested() bool {
	return in != nil && in.TestHooks != nil
}

// GetTestHooks returns the TestHooks for the release if not nil.
func (in *Snapshot) GetTestHooks() map[string]*TestHookStatus {
	if in == nil {
		return nil
	}
	return *in.TestHooks
}

// HasTestInPhase returns true if any of the TestHooks is in the given phase.
func (in *Snapshot) HasTestInPhase(phase string) bool {
	if in != nil {
		for _, h := range in.GetTestHooks() {
			if h.Phase == phase {
				return true
			}
		}
	}
	return false
}

// SetTestHooks sets the TestHooks for the release.
func (in *Snapshot) SetTestHooks(hooks map[string]*TestHookStatus) {
	in.TestHooks = &hooks
}

// TestHookStatus holds the status information for a test hook as observed
// to be run by the controller.
type TestHookStatus struct {
	// LastStarted is the time the test hook was last started.
	// +optional
	LastStarted metav1.Time `json:"lastStarted,omitempty"`
	// LastCompleted is the time the test hook last completed.
	// +optional
	LastCompleted metav1.Time `json:"lastCompleted,omitempty"`
	// Phase the test hook was observed to be in.
	// +optional
	Phase string `json:"phase,omitempty"`
}
