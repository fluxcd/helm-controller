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

package controllers

import (
	"context"
	"fmt"

	"github.com/fluxcd/pkg/apis/meta"
	"helm.sh/helm/v3/pkg/chartutil"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/strvals"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"

	v2 "github.com/fluxcd/helm-controller/api/v2beta1"
	"github.com/fluxcd/helm-controller/internal/runner"
	"github.com/fluxcd/helm-controller/internal/util"
)

func (r *HelmReleaseReconciler) reconcileRelease(ctx context.Context, run *runner.Runner, hr *v2.HelmRelease) (ctrl.Result, error) {
	defer r.patchStatus(ctx, hr)
	defer r.recordReadiness(*hr)

	// Given we are in control over creating the chart, we should
	// return an error as we either need to reconcile again to recreate
	// it, or reattempt because we encountered a transient error.
	hc, err := r.getHelmChart(ctx, hr)
	if err != nil {
		err = fmt.Errorf("failed to get HelmChart for resource: %w", err)
		v2.HelmReleaseNotReady(hr, v2.GetLastReleaseFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// Check if the chart has an artifact. If there is no artifact,
	// record the fact that we observed this but do not return an error
	// as our watcher should observe any artifact changes and queue a
	// reconciliation.
	if hc.GetArtifact() == nil {
		msg := fmt.Sprintf("HelmChart '%s/%s' has no artifact", hc.GetNamespace(), hc.GetName())
		v2.HelmReleaseNotReady(hr, meta.DependencyNotReadyReason, msg)
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	}

	// Compose the values. Given race conditions may happen due to
	// unordered applies (e.g. a HelmRelease with a reference to
	// a Secret is applied before the Secret itself), we expect
	// most errors to be transient and return them.
	values, err := r.composeValues(ctx, *hr)
	if err != nil {
		v2.HelmReleaseNotReady(hr, v2.InitFailedReason, err.Error())
		return ctrl.Result{}, err
	}
	// Calculate the checksum for the values.
	valuesChecksum := util.ValuesChecksum(values)

	// Observe the last release. If this fails, we likely encountered a
	// transient error and should return it to requeue a reconciliation.
	rls, err := run.ObserveLastRelease(*hr)
	if err != nil {
		msg := "failed to determine last deployed Helm release revision"
		v2.HelmReleaseNotReady(hr, v2.GetLastReleaseFailedReason, msg)
		return ctrl.Result{}, err
	}

	// Determine the remediation strategy that applies.
	remediation := hr.Spec.GetInstall().GetRemediation()
	if hr.Status.LastSuccessfulReleaseRevision > 0 {
		remediation = hr.Spec.GetUpgrade().GetRemediation()
	}

	releaseRevision := util.ReleaseRevision(rls)
	switch {
	// Previous release attempt resulted in a locked pending state,
	// attempt to remediate.
	case rls != nil && hr.Status.LastReleaseRevision == releaseRevision && rls.Info.Status.IsPending():
		v2.HelmReleaseNotReady(hr, v2.RemediatedCondition,
			fmt.Sprintf("previous release did not finish (%s)", rls.Info.Status))
		remediation.IncrementFailureCount(hr)
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	// Determine if there are any state changes to things we depend on
	// (chart revision, values checksum), or if the revision of the
	// release does not match what we have run ourselves.
	case v2.StateChanged(*hr, hc.GetArtifact().Revision, releaseRevision, valuesChecksum):
		v2.HelmReleaseProgressing(hr)
		r.patchStatus(ctx, hr)
	// The state has not changed and the release is not in a failed state.
	case remediation.GetFailureCount(*hr) == 0:
		v2.HelmReleaseReady(hr, rls.Info.Description)
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	// We have exhausted our retries.
	case remediation.RetriesExhausted(*hr):
		v2.HelmReleaseNotReady(hr, meta.ReconciliationFailedReason, "exhausted release retries")
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	// Our previous reconciliation attempt failed, skip release to retry.
	case hr.Status.LastSuccessfulReleaseRevision > 0 && hr.Status.LastReleaseRevision != hr.Status.LastSuccessfulReleaseRevision:
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	}

	// Attempt to download and load the chart. As we already checked
	// if an artifact is advertised in the chart object, any error
	// we encounter is expected to be transient.
	loadedChart, err := r.loadHelmChart(hc)
	if err != nil {
		v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// Determine the release strategy based on the existence of a
	// release in the Helm storage.
	makeRelease, remediation := run.Install, hr.Spec.GetInstall().GetRemediation()
	successReason, failureReason := v2.InstallSucceededReason, v2.InstallFailedReason
	if rls != nil {
		makeRelease, remediation = run.Upgrade, hr.Spec.GetUpgrade().GetRemediation()
		successReason, failureReason = v2.UpgradeSucceededReason, v2.UpgradeFailedReason
	}

	// Register our new release attempt.
	v2.HelmReleaseAttempt(hr, hc.GetArtifact().Revision, valuesChecksum)

	// Make the actual release.
	err = makeRelease(*hr, loadedChart, values)

	// Always record the revision when a new release has been made.
	hr.Status.LastReleaseRevision = util.ReleaseRevision(run.GetLastObservedRelease())

	if err != nil {
		// Record the release failure and increment the failure count.
		meta.SetResourceCondition(hr, v2.ReleasedCondition, metav1.ConditionFalse, failureReason, err.Error())
		v2.HelmReleaseNotReady(hr, failureReason, err.Error())
		remediation.IncrementFailureCount(hr)

		// We should still requeue on the configured interval so that
		// both the Helm storage and ValuesFrom references are observed
		// on an interval.
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	}

	// Record the release success.
	meta.SetResourceCondition(hr, v2.ReleasedCondition, metav1.ConditionTrue, successReason,
		run.GetLastObservedRelease().Info.Description)

	return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
}

func (r *HelmReleaseReconciler) reconcileTest(ctx context.Context, run *runner.Runner, hr *v2.HelmRelease) (ctrl.Result, error) {
	defer r.patchStatus(ctx, hr)
	defer r.recordReadiness(*hr)

	// If this release was already marked as successful,
	// we have nothing to do.
	if hr.Status.LastReleaseRevision == hr.Status.LastSuccessfulReleaseRevision {
		return ctrl.Result{}, nil
	}

	// Confirm the last release in storage equals to the release
	// we ourselves made.
	if revision := util.ReleaseRevision(run.GetLastObservedRelease()); revision != hr.Status.LastReleaseRevision {
		err := fmt.Errorf("unexpected revision change from %q to %q", hr.Status.LastReleaseRevision, revision)
		v2.HelmReleaseNotReady(hr, meta.ReconciliationFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// If the release is not in a deployed state, we should not run the
	// tests.
	if run.GetLastObservedRelease() == nil || run.GetLastObservedRelease().Info.Status != release.StatusDeployed {
		return ctrl.Result{}, nil
	}

	// If tests are not enabled for this resource, we have nothing to do.
	if !hr.Spec.GetTest().Enable {
		hr.Status.LastSuccessfulReleaseRevision = util.ReleaseRevision(run.GetLastObservedRelease())
		apimeta.RemoveStatusCondition(hr.GetStatusConditions(), v2.TestSuccessCondition)
		v2.HelmReleaseReady(hr, run.GetLastObservedRelease().Info.Description)
		return ctrl.Result{}, nil
	}

	// Test suite was already run for this release.
	if hasRunTestSuite(run.GetLastObservedRelease()) {
		return ctrl.Result{}, nil
	}

	// If the release does not have a test suite, we are successful.
	if !hasTestSuite(run.GetLastObservedRelease()) {
		msg := "No test suite"
		meta.SetResourceCondition(hr, v2.TestSuccessCondition, metav1.ConditionTrue, v2.TestSucceededReason, msg)
		v2.HelmReleaseReady(hr, msg)
		hr.Status.LastSuccessfulReleaseRevision = util.ReleaseRevision(run.GetLastObservedRelease())
		return ctrl.Result{}, nil
	}

	// Remove any previous test condition and run the tests.
	apimeta.RemoveStatusCondition(hr.GetStatusConditions(), v2.TestSuccessCondition)
	err := run.Test(*hr)

	// Given we can not target the revision to test, we need to reconfirm
	// that we actually run the tests for a release we made ourselves.
	if revision := util.ReleaseRevision(run.GetLastObservedRelease()); revision != hr.Status.LastReleaseRevision {
		err := fmt.Errorf("unexpected revision change from %q to %q during test run", hr.Status, revision)
		v2.HelmReleaseNotReady(hr, meta.ReconciliationFailedReason, err.Error())
		return ctrl.Result{}, err
	}

	// Calculate the test results.
	successful, failed := calculateHelmTestSuiteResult(run.GetLastObservedRelease())

	// Only increment the failure counter if this test suite run
	// resulted in a failure.
	if err != nil {
		msg := fmt.Sprintf("Helm test suite failed (success: %v, failed: %v)", successful, failed)
		meta.SetResourceCondition(hr, v2.TestSuccessCondition, metav1.ConditionFalse, v2.TestFailedReason, msg)

		// Determine the remediation strategy that applies, and if we
		// need to increment the failure count.
		remediation := hr.Spec.GetInstall().GetRemediation()
		if hr.Status.LastSuccessfulReleaseRevision > 0 {
			remediation = hr.Spec.GetUpgrade().GetRemediation()
		}

		// We should ignore the test results for the readiness of
		// the resource.
		if remediation.MustIgnoreTestFailures(hr.Spec.GetTest().IgnoreFailures) {
			hr.Status.LastSuccessfulReleaseRevision = util.ReleaseRevision(run.GetLastObservedRelease())
			v2.HelmReleaseReady(hr, run.GetLastObservedRelease().Info.Description)
			return ctrl.Result{}, nil
		}

		remediation.IncrementFailureCount(hr)
		v2.HelmReleaseNotReady(hr, v2.TestFailedReason, msg)
		return ctrl.Result{}, nil
	}

	// Record the test success and mark the release as ready.
	msg := fmt.Sprintf("Helm test suite succeeded (success: %v)", successful)
	meta.SetResourceCondition(hr, v2.TestSuccessCondition, metav1.ConditionTrue, v2.TestSucceededReason, msg)
	v2.HelmReleaseReady(hr, msg)
	hr.Status.LastSuccessfulReleaseRevision = util.ReleaseRevision(run.GetLastObservedRelease())

	return ctrl.Result{}, nil
}

func (r *HelmReleaseReconciler) reconcileRemediation(ctx context.Context, run *runner.Runner, hr *v2.HelmRelease) (ctrl.Result, error) {
	defer r.patchStatus(ctx, hr)
	defer r.recordReadiness(*hr)

	// Last release was successful, nothing to remediate.
	if hr.Status.LastReleaseRevision == hr.Status.LastSuccessfulReleaseRevision {
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	}

	// Determine the remediation strategy that applies.
	remediation := hr.Spec.GetUpgrade().GetRemediation()
	if remediation.GetFailureCount(*hr) == 0 {
		remediation = hr.Spec.GetInstall().GetRemediation()
	}

	// Return if there are no retries left, or if we should not remediate
	// the last failure.
	if remediation.RetriesExhausted(*hr) && !remediation.MustRemediateLastFailure() {
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	}

	// Perform the remediation action for the configured strategy.
	switch remediation.GetStrategy() {
	case v2.RollbackRemediationStrategy:
		err := run.Rollback(*hr)
		hr.Status.LastReleaseRevision = util.ReleaseRevision(run.GetLastObservedRelease())
		if err != nil {
			meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionFalse, v2.RollbackFailedReason, err.Error())
			v2.HelmReleaseNotReady(hr, v2.RollbackFailedReason, err.Error())
			return ctrl.Result{Requeue: true}, nil
		}
		meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionTrue, v2.RollbackSucceededReason, "Rollback complete")
		hr.Status.LastSuccessfulReleaseRevision = hr.Status.LastReleaseRevision
	case v2.UninstallRemediationStrategy:
		if err := run.Uninstall(*hr); err != nil {
			meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionFalse, v2.UninstallFailedReason, err.Error())
			v2.HelmReleaseNotReady(hr, v2.UninstallFailedReason, err.Error())
			return ctrl.Result{Requeue: true}, nil
		}

		meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionTrue, v2.UninstallSucceededReason, "Uninstallation complete")
		hr.Status.LastReleaseRevision = util.ReleaseRevision(run.GetLastObservedRelease())
		if !hr.Spec.GetUninstall().KeepHistory {
			hr.Status.LastReleaseRevision = 0
			hr.Status.LastSuccessfulReleaseRevision = 0
		}
	}

	// Requeue instantly to retry.
	return ctrl.Result{Requeue: true}, nil
}

// composeValues attempts to resolve all v2beta1.ValuesReference resources
// and merges them as defined. Referenced resources are only retrieved once
// to ensure a single version is taken into account during the merge.
func (r *HelmReleaseReconciler) composeValues(ctx context.Context, hr v2.HelmRelease) (chartutil.Values, error) {
	result := chartutil.Values{}

	configMaps := make(map[string]*corev1.ConfigMap)
	secrets := make(map[string]*corev1.Secret)

	for _, v := range hr.Spec.ValuesFrom {
		namespacedName := types.NamespacedName{Namespace: hr.Namespace, Name: v.Name}
		var valuesData []byte

		switch v.Kind {
		case "ConfigMap":
			resource, ok := configMaps[namespacedName.String()]
			if !ok {
				// The resource may not exist, but we want to act on a single version
				// of the resource in case the values reference is marked as optional.
				configMaps[namespacedName.String()] = nil

				resource = &corev1.ConfigMap{}
				if err := r.Get(ctx, namespacedName, resource); err != nil {
					if apierrors.IsNotFound(err) {
						if v.Optional {
							r.Log.Info(fmt.Sprintf("could not find optional %s '%s'", v.Kind, namespacedName))
							continue
						}
						return nil, fmt.Errorf("could not find %s '%s'", v.Kind, namespacedName)
					}
					return nil, err
				}
				configMaps[namespacedName.String()] = resource
			}
			if resource == nil {
				if v.Optional {
					r.Log.Info(fmt.Sprintf("could not find optional %s '%s'", v.Kind, namespacedName))
					continue
				}
				return nil, fmt.Errorf("could not find %s '%s'", v.Kind, namespacedName)
			}
			if data, ok := resource.Data[v.GetValuesKey()]; !ok {
				return nil, fmt.Errorf("missing key '%s' in %s '%s'", v.GetValuesKey(), v.Kind, namespacedName)
			} else {
				valuesData = []byte(data)
			}
		case "Secret":
			resource, ok := secrets[namespacedName.String()]
			if !ok {
				// The resource may not exist, but we want to act on a single version
				// of the resource in case the values reference is marked as optional.
				secrets[namespacedName.String()] = nil

				resource = &corev1.Secret{}
				if err := r.Get(ctx, namespacedName, resource); err != nil {
					if apierrors.IsNotFound(err) {
						if v.Optional {
							r.Log.Info(fmt.Sprintf("could not find optional %s '%s'", v.Kind, namespacedName))
							continue
						}
						return nil, fmt.Errorf("could not find %s '%s'", v.Kind, namespacedName)
					}
					return nil, err
				}
				secrets[namespacedName.String()] = resource
			}
			if resource == nil {
				if v.Optional {
					r.Log.Info(fmt.Sprintf("could not find optional %s '%s'", v.Kind, namespacedName))
					continue
				}
				return nil, fmt.Errorf("could not find %s '%s'", v.Kind, namespacedName)
			}
			if data, ok := resource.Data[v.GetValuesKey()]; !ok {
				return nil, fmt.Errorf("missing key '%s' in %s '%s'", v.GetValuesKey(), v.Kind, namespacedName)
			} else {
				valuesData = data
			}
		default:
			return nil, fmt.Errorf("unsupported ValuesReference kind '%s'", v.Kind)
		}
		switch v.TargetPath {
		case "":
			values, err := chartutil.ReadValues(valuesData)
			if err != nil {
				return nil, fmt.Errorf("unable to read values from key '%s' in %s '%s': %w", v.GetValuesKey(), v.Kind, namespacedName, err)
			}
			result = util.MergeMaps(result, values)
		default:
			// TODO(hidde): this is a bit of hack, as it mimics the way the option string is passed
			// 	to Helm from a CLI perspective. Given the parser is however not publicly accessible
			// 	while it contains all logic around parsing the target path, it is a fair trade-off.
			singleValue := v.TargetPath + "=" + string(valuesData)
			if err := strvals.ParseInto(singleValue, result); err != nil {
				return nil, fmt.Errorf("unable to merge value from key '%s' in %s '%s' into target path '%s': %w", v.GetValuesKey(), v.Kind, namespacedName, v.TargetPath, err)
			}
		}
	}
	return util.MergeMaps(result, hr.GetValues()), nil
}

func calculateHelmTestSuiteResult(rls *release.Release) ([]string, []string) {
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

func hasRunTestSuite(rls *release.Release) bool {
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

func hasTestSuite(rls *release.Release) bool {
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
