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
	"github.com/fluxcd/pkg/runtime/events"
	"github.com/go-logr/logr"
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

func (r *HelmReleaseReconciler) reconcileRelease(ctx context.Context, log logr.Logger, run *runner.Runner, hr *v2.HelmRelease) (ctrl.Result, error) {
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
	// record the fact that we observed this but do not return an error.
	// Our watcher should observe the chart becoming ready, and queue
	// a new reconciliation.
	if hc.GetArtifact() == nil {
		msg := fmt.Sprintf("HelmChart '%s/%s' has no artifact", hc.GetNamespace(), hc.GetName())
		v2.HelmReleaseNotReady(hr, meta.DependencyNotReadyReason, msg)
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	}

	// Compose the values. As race conditions may happen due to
	// unordered applies (due to e.g. a HelmRelease with a reference
	// to a Secret is applied before the Secret itself), we expect
	// most errors to be transient and return them.
	values, err := r.composeValues(ctx, log, hr)
	if err != nil {
		v2.HelmReleaseNotReady(hr, v2.InitFailedReason, err.Error())
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, err.Error())
		return ctrl.Result{}, err
	}

	// Observe the last release. If this fails, we likely encountered a
	// transient error and should return it to requeue a reconciliation.
	rls, err := run.ObserveLastRelease(hr)
	if err != nil {
		msg := "failed to determine last deployed Helm release revision"
		v2.HelmReleaseNotReady(hr, v2.GetLastReleaseFailedReason, msg)
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, msg)
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

	// Previous release attempt resulted in a locked pending state,
	// attempt to remediate.
	releaseRevision := util.ReleaseRevision(rls)
	if rls != nil && hr.Status.LastReleaseRevision == releaseRevision && rls.Info.Status.IsPending() {
		msg := fmt.Sprintf("previous release did not finish (%s)", rls.Info.Status)
		v2.HelmReleaseNotReady(hr, v2.RemediatedCondition, msg)
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, msg)
		remediation.IncrementFailureCount(hr)
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	}

	valuesChecksum := util.ValuesChecksum(values)

	// We must have gathered all information to decide if we actually
	// need to move forward with the release at this point.
	switch {
	// Determine if there are any state changes to things we depend on
	// (chart revision, values checksum), or if the revision of the
	// release does not match what we have run ourselves.
	case v2.StateChanged(hr, hc.GetArtifact().Revision, releaseRevision, valuesChecksum):
		v2.HelmReleaseProgressing(hr)
		r.patchStatus(ctx, hr)
	// The state has not changed and the release is not in a failed state.
	case rls != nil && remediation.GetFailureCount(*hr) == 0:
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
	// This must always be done right before making the actual release,
	// as depending on the chart this can be a (memory) expensive
	// operation.
	loadedChart, err := r.loadHelmChart(hc)
	if err != nil {
		v2.HelmReleaseNotReady(hr, v2.ArtifactFailedReason, err.Error())
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, err.Error())
		return ctrl.Result{}, err
	}

	// Register our new release attempt and make the release.
	v2.HelmReleaseAttempt(hr, hc.GetArtifact().Revision, valuesChecksum)
	err = makeRelease(*hr, loadedChart, values)

	// Always record the revision when a new release has been made.
	hr.Status.LastReleaseRevision = util.ReleaseRevision(run.GetLastPersistedRelease())

	if err != nil {
		// Record the release failure and increment the failure count.
		meta.SetResourceCondition(hr, v2.ReleasedCondition, metav1.ConditionFalse, failureReason, err.Error())
		v2.HelmReleaseNotReady(hr, failureReason, err.Error())
		remediation.IncrementFailureCount(hr)
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, err.Error())

		// We should still requeue on the configured interval so that
		// both the Helm storage and ValuesFrom references are observed
		// on an interval.
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	}

	meta.SetResourceCondition(hr, v2.ReleasedCondition, metav1.ConditionTrue, successReason,
		run.GetLastPersistedRelease().Info.Description)
	r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityInfo, run.GetLastPersistedRelease().Info.Description)

	return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
}

func (r *HelmReleaseReconciler) reconcileTest(ctx context.Context, log logr.Logger, run *runner.Runner, hr *v2.HelmRelease) (ctrl.Result, error) {
	// If this release was already marked as successful,
	// we have nothing to do.
	if hr.Status.LastReleaseRevision == hr.Status.LastSuccessfulReleaseRevision {
		return ctrl.Result{}, nil
	}

	// Confirm the last release in storage equals to the release
	// we made ourselves.
	if revision := util.ReleaseRevision(run.GetLastPersistedRelease()); revision != hr.Status.LastReleaseRevision {
		err := fmt.Errorf("unexpected revision change from %q to %q", hr.Status.LastReleaseRevision, revision)
		v2.HelmReleaseNotReady(hr, meta.ReconciliationFailedReason, err.Error())
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, err.Error())
		return ctrl.Result{}, err
	}

	// If the release is not in a deployed state, we should not run the
	// tests.
	if run.GetLastPersistedRelease() == nil || run.GetLastPersistedRelease().Info.Status != release.StatusDeployed {
		return ctrl.Result{}, nil
	}

	// If tests are not enabled for this resource, we have nothing to do.
	if !hr.Spec.GetTest().Enable {
		hr.Status.LastSuccessfulReleaseRevision = util.ReleaseRevision(run.GetLastPersistedRelease())
		apimeta.RemoveStatusCondition(hr.GetStatusConditions(), v2.TestSuccessCondition)
		v2.HelmReleaseReady(hr, run.GetLastPersistedRelease().Info.Description)
		return ctrl.Result{}, nil
	}

	// Test suite was already run for this release.
	if util.HasRunTestSuite(run.GetLastPersistedRelease()) {
		return ctrl.Result{}, nil
	}

	// If the release does not have a test suite, we are successful.
	if !util.HasTestSuite(run.GetLastPersistedRelease()) {
		msg := "No test suite"
		meta.SetResourceCondition(hr, v2.TestSuccessCondition, metav1.ConditionTrue, v2.TestSucceededReason, msg)
		v2.HelmReleaseReady(hr, msg)
		hr.Status.LastSuccessfulReleaseRevision = util.ReleaseRevision(run.GetLastPersistedRelease())
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityInfo, msg)
		return ctrl.Result{}, nil
	}

	// Remove any previous test condition and run the tests.
	apimeta.RemoveStatusCondition(hr.GetStatusConditions(), v2.TestSuccessCondition)
	err := run.Test(*hr)

	// We can not target the revision to test, and it is possible that
	// the Helm storage state changed while we were doing other things.
	// Confirm the tests run against our last recorded release revision.
	if revision := util.ReleaseRevision(run.GetLastPersistedRelease()); revision != hr.Status.LastReleaseRevision {
		err := fmt.Errorf("unexpected revision change from %q to %q during test run", hr.Status, revision)
		v2.HelmReleaseNotReady(hr, meta.ReconciliationFailedReason, err.Error())
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, err.Error())
		return ctrl.Result{}, err
	}

	// Calculate the test results.
	successful, failed := util.CalculateTestSuiteResult(run.GetLastPersistedRelease())

	if err != nil {
		msg := fmt.Sprintf("Helm test suite failed (success: %v, failed: %v)", successful, failed)
		meta.SetResourceCondition(hr, v2.TestSuccessCondition, metav1.ConditionFalse, v2.TestFailedReason, msg)
		r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, msg)

		// Determine the remediation strategy that applies, and if we
		// need to increment the failure count.
		remediation := hr.Spec.GetInstall().GetRemediation()
		if hr.Status.LastSuccessfulReleaseRevision > 0 {
			remediation = hr.Spec.GetUpgrade().GetRemediation()
		}

		// Return if me must ignore the test results for the readiness of
		// the resource.
		if remediation.MustIgnoreTestFailures(hr.Spec.GetTest().IgnoreFailures) {
			hr.Status.LastSuccessfulReleaseRevision = util.ReleaseRevision(run.GetLastPersistedRelease())
			v2.HelmReleaseReady(hr, run.GetLastPersistedRelease().Info.Description)
			return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
		}

		remediation.IncrementFailureCount(hr)
		v2.HelmReleaseNotReady(hr, v2.TestFailedReason, msg)
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	}

	// Record the test success and mark the release as ready.
	msg := fmt.Sprintf("Helm test suite succeeded (success: %v)", successful)
	meta.SetResourceCondition(hr, v2.TestSuccessCondition, metav1.ConditionTrue, v2.TestSucceededReason, msg)
	v2.HelmReleaseReady(hr, msg)
	hr.Status.LastSuccessfulReleaseRevision = util.ReleaseRevision(run.GetLastPersistedRelease())
	r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityInfo, msg)

	return ctrl.Result{}, nil
}

func (r *HelmReleaseReconciler) reconcileRemediation(ctx context.Context, log logr.Logger, run *runner.Runner, hr *v2.HelmRelease) (ctrl.Result, error) {
	// Last release was successful, nothing to remediate.
	if hr.Status.LastReleaseRevision == hr.Status.LastSuccessfulReleaseRevision {
		return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
	}

	// Determine the remediation strategy that applies.
	remediation := hr.Spec.GetInstall().GetRemediation()
	if run.GetLastPersistedRelease() != nil && remediation.GetFailureCount(*hr) == 0 {
		remediation = hr.Spec.GetUpgrade().GetRemediation()
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
		hr.Status.LastReleaseRevision = util.ReleaseRevision(run.GetLastPersistedRelease())
		if err != nil {
			// Record observed upgrade remediation failure
			meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionFalse, v2.RollbackFailedReason, err.Error())
			v2.HelmReleaseNotReady(hr, v2.RollbackFailedReason, err.Error())
			r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, err.Error())
			return ctrl.Result{Requeue: true}, nil
		}

		// Record observed rollback remediation success.
		meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionTrue, v2.RollbackSucceededReason,
			run.GetLastPersistedRelease().Info.Description)
		hr.Status.LastSuccessfulReleaseRevision = hr.Status.LastReleaseRevision
	case v2.UninstallRemediationStrategy:
		if err := run.Uninstall(hr); err != nil {
			// Record observed uninstall failure.
			meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionFalse, v2.UninstallFailedReason, err.Error())
			v2.HelmReleaseNotReady(hr, v2.UninstallFailedReason, err.Error())
			r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityError, err.Error())
			return ctrl.Result{Requeue: true}, nil
		}

		// Record observed uninstall remediation success.
		meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionTrue, v2.UninstallSucceededReason,
			run.GetLastPersistedRelease().Info.Description)
		hr.Status.LastReleaseRevision = util.ReleaseRevision(run.GetLastPersistedRelease())
		hr.Status.LastSuccessfulReleaseRevision = hr.Status.LastReleaseRevision

		// If we did not keep our Helm release history, we are now
		// back to square one.
		if !hr.Spec.GetUninstall().KeepHistory {
			hr.Status.LastReleaseRevision = 0
			hr.Status.LastSuccessfulReleaseRevision = 0
		}
	}

	// Requeue instantly to retry.
	r.event(hr, hr.Status.LastAttemptedRevision, events.EventSeverityInfo, run.GetLastPersistedRelease().Info.Description)
	return ctrl.Result{Requeue: true}, nil
}

// composeValues attempts to resolve all v2beta1.ValuesReference resources
// and merges them as defined. Referenced resources are only retrieved once
// to ensure a single version is taken into account during the merge.
func (r *HelmReleaseReconciler) composeValues(ctx context.Context, log logr.Logger, hr *v2.HelmRelease) (chartutil.Values, error) {
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
							log.Info("could not find optional %s '%s'", v.Kind, namespacedName)
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
					log.Info("could not find optional %s '%s'", v.Kind, namespacedName)
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
							log.Info("could not find optional %s '%s'", v.Kind, namespacedName)
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
					log.Info("could not find optional %s '%s'", v.Kind, namespacedName)
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
