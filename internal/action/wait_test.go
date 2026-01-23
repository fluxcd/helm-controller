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
	"testing"

	. "github.com/onsi/gomega"
	helmkube "helm.sh/helm/v4/pkg/kube"

	v2 "github.com/fluxcd/helm-controller/api/v2"
)

type mockActionThatWaits struct {
	disableWait bool
}

func (m *mockActionThatWaits) GetDisableWait() bool {
	return m.disableWait
}

func TestGetWaitStrategy(t *testing.T) {
	for _, tt := range []struct {
		name             string
		useHelm3Defaults bool
		strategy         v2.WaitStrategyName
		actionSpec       actionThatWaits
		expectedWait     helmkube.WaitStrategy
	}{
		{
			name:             "wait disabled",
			useHelm3Defaults: false,
			actionSpec:       &mockActionThatWaits{disableWait: true},
			expectedWait:     helmkube.HookOnlyStrategy,
		},
		{
			name:             "wait disabled with UseHelm3Defaults",
			useHelm3Defaults: true,
			actionSpec:       &mockActionThatWaits{disableWait: true},
			expectedWait:     helmkube.HookOnlyStrategy,
		},
		{
			name:             "wait enabled with UseHelm3Defaults",
			useHelm3Defaults: true,
			actionSpec:       &mockActionThatWaits{disableWait: false},
			expectedWait:     helmkube.LegacyStrategy,
		},
		{
			name:             "wait enabled with Helm4 defaults",
			useHelm3Defaults: false,
			actionSpec:       &mockActionThatWaits{disableWait: false},
			expectedWait:     helmkube.StatusWatcherStrategy,
		},
		{
			name:             "user specified watcher strategy",
			useHelm3Defaults: true, // default would be legacy
			strategy:         v2.WaitStrategyWatcher,
			actionSpec:       &mockActionThatWaits{disableWait: false},
			expectedWait:     helmkube.StatusWatcherStrategy,
		},
		{
			name:             "user specified legacy strategy",
			useHelm3Defaults: false, // default would be watcher
			strategy:         v2.WaitStrategyLegacy,
			actionSpec:       &mockActionThatWaits{disableWait: false},
			expectedWait:     helmkube.LegacyStrategy,
		},
		{
			name:             "wait disabled takes precedence over user specified strategy",
			useHelm3Defaults: false,
			strategy:         v2.WaitStrategyWatcher,
			actionSpec:       &mockActionThatWaits{disableWait: true},
			expectedWait:     helmkube.HookOnlyStrategy,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Save and restore UseHelm3Defaults
			oldUseHelm3Defaults := UseHelm3Defaults
			t.Cleanup(func() { UseHelm3Defaults = oldUseHelm3Defaults })
			UseHelm3Defaults = tt.useHelm3Defaults

			waitStrategy := getWaitStrategy(tt.strategy, tt.actionSpec)
			g.Expect(waitStrategy).To(Equal(tt.expectedWait))
		})
	}
}
