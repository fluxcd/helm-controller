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
	helmkube "helm.sh/helm/v4/pkg/kube"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

// actionThatWaits is implemented by HelmRelease action specs that
// support wait strategies.
type actionThatWaits interface {
	GetDisableWait() bool
}

// getWaitStrategy returns the wait strategy for the given action spec.
func getWaitStrategy(strategy v2.WaitStrategyName, spec actionThatWaits) helmkube.WaitStrategy {
	switch {
	case spec.GetDisableWait():
		return helmkube.HookOnlyStrategy
	case strategy != "":
		return helmkube.WaitStrategy(strategy)
	case UseHelm3Defaults:
		return helmkube.LegacyStrategy
	default:
		return helmkube.StatusWatcherStrategy
	}
}
