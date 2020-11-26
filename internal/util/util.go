/*
Copyright 2020 The Flux authors

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

package util

import (
	"crypto/sha1"
	"fmt"

	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	ctrl "sigs.k8s.io/controller-runtime"
)

// MergeMaps merges map b into given map a and returns the result.
// It allows overwrites of map values with flat values, and vice versa.
// This is copied from https://github.com/helm/helm/blob/v3.3.0/pkg/cli/values/options.go#L88,
// as the public chartutil.CoalesceTables function does not allow
// overwriting maps with flat values.
func MergeMaps(a, b map[string]interface{}) map[string]interface{} {
	out := make(map[string]interface{}, len(a))
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		if v, ok := v.(map[string]interface{}); ok {
			if bv, ok := out[k]; ok {
				if bv, ok := bv.(map[string]interface{}); ok {
					out[k] = MergeMaps(bv, v)
					continue
				}
			}
		}
		out[k] = v
	}
	return out
}

// ValuesChecksum calculates and returns the SHA1 checksum for the
// given chartutil.Values.
func ValuesChecksum(values chartutil.Values) string {
	var s string
	if len(values) != 0 {
		s, _ = values.YAML()
	}
	return fmt.Sprintf("%x", sha1.Sum([]byte(s)))
}

// ReleaseRevision returns the revision of the given release.Release.
func ReleaseRevision(rel *release.Release) int {
	if rel == nil {
		return 0
	}
	return rel.Version
}

// LowestNonZeroResult compares two reconciliation results and returns
// the one with lowest requeue time.
func LowestNonZeroResult(i, j ctrl.Result) ctrl.Result {
	switch {
	case i.IsZero():
		return j
	case j.IsZero():
		return i
	case i.Requeue:
		return i
	case j.Requeue:
		return j
	case i.RequeueAfter < j.RequeueAfter:
		return i
	default:
		return j
	}
}

func CalculateTestSuiteResult(rls *release.Release) ([]string, []string) {
	result := make(map[release.HookPhase][]string)
	executions := executionsByHookEvent(rls)
	tests, ok := executions[release.HookTest]
	if !ok {
		return nil, nil
	}
	for _, h := range tests {
		names := result[h.LastRun.Phase]
		result[h.LastRun.Phase] = append(names, h.Name)
	}
	return result[release.HookPhaseSucceeded], result[release.HookPhaseFailed]
}

func HasRunTestSuite(rls *release.Release) bool {
	executions := executionsByHookEvent(rls)
	tests, ok := executions[release.HookTest]
	if !ok {
		return false
	}
	for _, h := range tests {
		if !h.LastRun.StartedAt.IsZero() {
			return true
		}
	}
	return false
}

func HasTestSuite(rls *release.Release) bool {
	executions := executionsByHookEvent(rls)
	tests, ok := executions[release.HookTest]
	if !ok {
		return false
	}
	return len(tests) > 0
}

func executionsByHookEvent(rls *release.Release) map[release.HookEvent][]*release.Hook {
	result := make(map[release.HookEvent][]*release.Hook)
	for _, h := range rls.Hooks {
		for _, e := range h.Events {
			executions, ok := result[e]
			if !ok {
				executions = []*release.Hook{}
			}
			result[e] = append(executions, h)
		}
	}
	return result
}
