/*
Copyright 2025 The Flux authors

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
	v2 "github.com/fluxcd/helm-controller/api/v2"
)

// toHelmSSAValue converts the API ServerSideApplyMode to the Helm SDK value.
// The API uses "enabled"/"disabled"/"auto" to avoid YAML boolean auto-conversion,
// while the Helm SDK expects "true"/"false"/"auto".
func toHelmSSAValue(mode v2.ServerSideApplyMode) string {
	switch mode {
	case v2.ServerSideApplyEnabled:
		return "true"
	case v2.ServerSideApplyDisabled:
		return "false"
	default:
		return string(mode)
	}
}
