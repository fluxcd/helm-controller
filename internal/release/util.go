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
	helmrelease "helm.sh/helm/v3/pkg/release"
)

// GetTestHooks returns the list of test hooks for the given release, indexed
// by hook name.
func GetTestHooks(rls *helmrelease.Release) map[string]*helmrelease.Hook {
	th := make(map[string]*helmrelease.Hook)
	for _, h := range rls.Hooks {
		if IsHookForEvent(h, helmrelease.HookTest) {
			th[h.Name] = h
		}
	}
	return th
}

// IsHookForEvent returns if the given hook fires on the provided event.
func IsHookForEvent(hook *helmrelease.Hook, event helmrelease.HookEvent) bool {
	if hook != nil {
		for _, e := range hook.Events {
			if e == event {
				return true
			}
		}
	}
	return false
}
