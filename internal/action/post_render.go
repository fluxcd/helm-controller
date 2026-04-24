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

package action

import (
	"helm.sh/helm/v4/pkg/action"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

// toHelmPostRenderStrategy converts the API PostRenderStrategy to the Helm SDK value.
// If the strategy is not set, it defaults to PostRenderStrategyCombined (Helm 4 default),
// or PostRenderStrategyNoHooks when UseHelm3Defaults is enabled.
func toHelmPostRenderStrategy(strategy v2.PostRenderStrategy) action.PostRenderStrategy {
	if strategy == "" {
		if UseHelm3Defaults {
			return action.PostRenderStrategyNoHooks
		}
		return action.PostRenderStrategyCombined
	}
	return action.PostRenderStrategy(strategy)
}
