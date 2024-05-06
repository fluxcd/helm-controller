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

// Package features sets the feature gates that
// helm-controller supports, and their default states.
package features

import feathelper "github.com/fluxcd/pkg/runtime/features"

const (
	// CacheSecretsAndConfigMaps configures the caching of Secrets and ConfigMaps
	// by the controller-runtime client.
	//
	// When enabled, it will cache both object types, resulting in increased memory
	// usage and cluster-wide RBAC permissions (list and watch).
	CacheSecretsAndConfigMaps = "CacheSecretsAndConfigMaps"

	// DetectDrift configures the detection of cluster state drift compared to
	// the desired state as described in the manifest of the Helm release
	// storage object.
	// Deprecated in v0.37.0, use the drift detection mode on the HelmRelease
	// object instead.
	DetectDrift = "DetectDrift"

	// CorrectDrift configures the correction of cluster state drift compared to
	// the desired state as described in the manifest of the Helm release. It
	// is only effective when DetectDrift is enabled.
	// Deprecated in v0.37.0, use the drift detection mode on the HelmRelease
	// object instead.
	CorrectDrift = "CorrectDrift"

	// AllowDNSLookups allows the controller to perform DNS lookups when rendering Helm
	// templates. This is disabled by default, as it can be a security risk.
	//
	// Ref: https://github.com/helm/helm/security/advisories/GHSA-pwcw-6f5g-gxf8
	AllowDNSLookups = "AllowDNSLookups"

	// OOMWatch enables the OOM watcher, which will gracefully shut down the controller
	// when the memory usage exceeds the configured limit. This is disabled by default.
	OOMWatch = "OOMWatch"

	// AdoptLegacyReleases enables the adoption of the historical Helm release
	// based on the status fields from a v2beta1 HelmRelease object.
	// This is enabled by default to support an upgrade path from v2beta1 to v2
	// without the need to upgrade the Helm release. But it can be disabled to
	// avoid potential abuse of the adoption mechanism.
	AdoptLegacyReleases = "AdoptLegacyReleases"
)

var features = map[string]bool{
	// CacheSecretsAndConfigMaps
	// opt-in from v0.28
	CacheSecretsAndConfigMaps: false,
	// DetectDrift
	// deprecated in v0.37.0
	DetectDrift: false,
	// CorrectDrift,
	// deprecated in v0.37.0
	CorrectDrift: false,
	// AllowDNSLookups
	// opt-in from v0.31
	AllowDNSLookups: false,
	// OOMWatch
	// opt-in from v0.31
	OOMWatch: false,
	// AdoptLegacyReleases
	// opt-out from v0.37
	AdoptLegacyReleases: true,
}

// FeatureGates contains a list of all supported feature gates and
// their default values.
func FeatureGates() map[string]bool {
	return features
}

// Enabled verifies whether the feature is enabled or not.
//
// This is only a wrapper around the Enabled func in
// pkg/runtime/features, so callers won't need to import
// both packages for checking whether a feature is enabled.
func Enabled(feature string) (bool, error) {
	return feathelper.Enabled(feature)
}

// Disable disables the specified feature. If the feature is not
// present, it's a no-op.
func Disable(feature string) {
	if _, ok := features[feature]; ok {
		features[feature] = false
	}
}
