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

package testutil

import (
	"fmt"

	helmrelease "helm.sh/helm/v3/pkg/release"
)

// ReleaseOptions is a helper to build a Helm release mock.
type ReleaseOptions struct {
	*helmrelease.Release
}

// ReleaseOption is a function that can be used to modify a release.
type ReleaseOption func(*ReleaseOptions)

// BuildRelease builds a release with release.Mock using the given options,
// and applies any provided options to the release before returning it.
func BuildRelease(mockOpts *helmrelease.MockReleaseOptions, opts ...ReleaseOption) *helmrelease.Release {
	mock := helmrelease.Mock(mockOpts)
	r := &ReleaseOptions{Release: mock}

	for _, opt := range opts {
		opt(r)
	}

	return r.Release
}

// ReleaseWithConfig sets the config on the release.
func ReleaseWithConfig(config map[string]interface{}) ReleaseOption {
	return func(options *ReleaseOptions) {
		options.Config = config
	}
}

// ReleaseWithLabels sets the labels on the release.
func ReleaseWithLabels(labels map[string]string) ReleaseOption {
	return func(options *ReleaseOptions) {
		options.Release.Labels = labels
	}
}

// ReleaseWithFailingHook appends a failing hook to the release.
func ReleaseWithFailingHook() ReleaseOption {
	return func(options *ReleaseOptions) {
		options.Release.Hooks = append(options.Release.Hooks, &helmrelease.Hook{
			Name:     "failing-hook",
			Kind:     "Pod",
			Manifest: fmt.Sprintf(manifestWithFailingTestHookTmpl, options.Release.Namespace),
			Events: []helmrelease.HookEvent{
				helmrelease.HookPostInstall,
				helmrelease.HookPostUpgrade,
				helmrelease.HookPostRollback,
				helmrelease.HookPostDelete,
			},
		})
	}
}

// ReleaseWithHookExecution appends a hook with a last run with the given
// execution phase on the release.
func ReleaseWithHookExecution(name string, events []helmrelease.HookEvent, phase helmrelease.HookPhase) ReleaseOption {
	return func(options *ReleaseOptions) {
		options.Release.Hooks = append(options.Release.Hooks, &helmrelease.Hook{
			Name:   name,
			Events: events,
			LastRun: helmrelease.HookExecution{
				StartedAt:   MustParseHelmTime("2006-01-02T15:10:05Z"),
				CompletedAt: MustParseHelmTime("2006-01-02T15:10:07Z"),
				Phase:       phase,
			},
		})
	}
}

// ReleaseWithTestHook appends a test hook to the release.
func ReleaseWithTestHook() ReleaseOption {
	return func(options *ReleaseOptions) {
		options.Release.Hooks = append(options.Release.Hooks, &helmrelease.Hook{
			Name:     "test-hook",
			Kind:     "ConfigMap",
			Manifest: fmt.Sprintf(manifestWithTestHookTmpl, options.Release.Namespace),
			Events: []helmrelease.HookEvent{
				helmrelease.HookTest,
			},
		})
	}
}

// ReleaseWithFailingTestHook appends a failing test hook to the release.
func ReleaseWithFailingTestHook() ReleaseOption {
	return func(options *ReleaseOptions) {
		options.Release.Hooks = append(options.Release.Hooks, &helmrelease.Hook{
			Name:     "failing-test-hook",
			Kind:     "Pod",
			Manifest: fmt.Sprintf(manifestWithFailingTestHookTmpl, options.Release.Namespace),
			Events: []helmrelease.HookEvent{
				helmrelease.HookTest,
			},
		})
	}
}

// ReleaseWithHooks sets the hooks on the release.
func ReleaseWithHooks(hooks []*helmrelease.Hook) ReleaseOption {
	return func(options *ReleaseOptions) {
		options.Release.Hooks = append(options.Release.Hooks, hooks...)
	}
}
