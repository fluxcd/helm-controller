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

package release

import (
	"testing"

	. "github.com/onsi/gomega"
	helmrelease "helm.sh/helm/v3/pkg/release"

	"github.com/fluxcd/helm-controller/internal/testutil"
)

func TestGetTestHooks(t *testing.T) {
	g := NewWithT(t)

	hooks := []*helmrelease.Hook{
		{
			Name: "pre-install",
			Events: []helmrelease.HookEvent{
				helmrelease.HookPreInstall,
			},
		},
		{
			Name: "test",
			Events: []helmrelease.HookEvent{
				helmrelease.HookTest,
			},
		},
		{
			Name: "post-install",
			Events: []helmrelease.HookEvent{
				helmrelease.HookPostInstall,
			},
		},
		{
			Name: "combined-test-hook",
			Events: []helmrelease.HookEvent{
				helmrelease.HookPostRollback,
				helmrelease.HookTest,
			},
		},
	}

	g.Expect(GetTestHooks(&helmrelease.Release{
		Hooks: hooks,
	})).To(testutil.Equal(map[string]*helmrelease.Hook{
		hooks[1].Name: hooks[1],
		hooks[3].Name: hooks[3],
	}))
}

func TestIsHookForEvent(t *testing.T) {
	g := NewWithT(t)

	hook := &helmrelease.Hook{
		Events: []helmrelease.HookEvent{
			helmrelease.HookPreInstall,
			helmrelease.HookPostInstall,
		},
	}
	g.Expect(IsHookForEvent(hook, helmrelease.HookPreInstall)).To(BeTrue())
	g.Expect(IsHookForEvent(hook, helmrelease.HookPostInstall)).To(BeTrue())
	g.Expect(IsHookForEvent(hook, helmrelease.HookTest)).To(BeFalse())
}
