/*
Copyright 2026 The Flux authors

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

package v2

const (
	// InventoryBuildFailedReason represents the fact that the inventory
	// build failed.
	InventoryBuildFailedReason string = "InventoryBuildFailed"

	// NamespaceCheckSkippedReason represents the fact that namespace
	// scope check was skipped due to RESTMapper error.
	NamespaceCheckSkippedReason string = "NamespaceCheckSkipped"
)

// ResourceInventory contains a list of Kubernetes resource object references
// that have been applied by a HelmRelease.
type ResourceInventory struct {
	// Entries of Kubernetes resource object references.
	Entries []ResourceRef `json:"entries"`
}

// ResourceRef contains the information necessary to locate a resource within a cluster.
type ResourceRef struct {
	// ID is the string representation of the Kubernetes resource object's metadata,
	// in the format '<namespace>_<name>_<group>_<kind>'.
	ID string `json:"id"`

	// Version is the API version of the Kubernetes resource object's kind.
	Version string `json:"v"`
}
