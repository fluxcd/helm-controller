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

func (r *HelmReleaseReconciler) reconcileRelease(ctx context.Context, run *runner.Runner, hr *v2.HelmRelease, rls *release.Release) (ctrl.Result, error) {
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
		return ctrl.Result{}, nil
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

	// Determine if there are any state changes to things we depend on
	// (chart revision, values checksum), or if the revision in the Helm
	// storage no longer matches what we ought to have run ourselves.
	// In both scenarios we should reset state and allow a release to be
	// made.
	if hr.Status.LastAttemptedRevision != hc.GetArtifact().Revision ||
		hr.Status.LastReleaseRevision != util.ReleaseRevision(rls) ||
		hr.Status.LastAttemptedValuesChecksum != valuesChecksum ||
		hr.Status.ObservedGeneration != hr.Generation {
		v2.HelmReleaseProgressing(hr)
		r.patchStatus(ctx, hr)
	}

	// If we already made a release and it was successful, we have
	// nothing to do.
	if released := apimeta.FindStatusCondition(hr.Status.Conditions, v2.ReleasedCondition); released != nil {
		if released.Status == metav1.ConditionTrue {
			// We may have encountered a transient error in a previous
			// run, ensure ready condition is reset.
			v2.HelmReleaseReady(hr, released.Message)
			return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
		}
	}

	// The remediation failed, and making a release on top of this
	// can give unpredicted results. Return as a safety precaution.
	remediated := apimeta.FindStatusCondition(hr.Status.Conditions, v2.RemediatedCondition)
	if remediated != nil && remediated.Status == metav1.ConditionFalse {
		if remediated.Status == metav1.ConditionFalse {
			v2.HelmReleaseNotReady(hr, meta.ReconciliationFailedReason, remediated.Message)
			return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
		}
	}

	// Determine the active remediation strategy configuration based on the
	// failure counts.
	var prevStrategy v2.DeploymentAction
	prevStrategy = hr.Spec.GetUpgrade()
	if prevStrategy.GetRemediation().GetFailureCount(*hr) == 0 {
		prevStrategy = hr.Spec.GetInstall()
	}

	// Determine if we have exhausted our install or upgrade retries.
	if prevStrategy.GetRemediation().RetriesExhausted(*hr) {
		v2.HelmReleaseNotReady(hr, meta.ReconciliationFailedReason, fmt.Sprintf("Exhausted %s retries", prevStrategy.GetDescription()))
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

	// Register our attempt and ensure the release is no longer marked
	// as remediated.
	v2.HelmReleaseAttempt(hr, hc.GetArtifact().Revision, valuesChecksum)
	apimeta.RemoveStatusCondition(hr.GetStatusConditions(), v2.RemediatedCondition)

	// Make the actual release.
	rls, err = makeRelease(*hr, loadedChart, values)

	// Record the new revision if we have one.
	if revision := util.ReleaseRevision(rls); revision > hr.Status.LastReleaseRevision {
		hr.Status.LastReleaseRevision = revision
	}

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

	// Record the release success and ensure the release is no longer
	// marked as tested.
	meta.SetResourceCondition(hr, v2.ReleasedCondition, metav1.ConditionTrue, successReason, rls.Info.Description)
	apimeta.RemoveStatusCondition(hr.GetStatusConditions(), v2.TestSuccessCondition)

	// If we do not need to perform any more actions, we can already
	// mark the release as ready.
	if !hr.Spec.GetTest().Enable || remediation.MustIgnoreTestFailures(hr.Spec.GetTest().IgnoreFailures) {
		v2.HelmReleaseReady(hr, rls.Info.Description)
	}

	return ctrl.Result{RequeueAfter: hr.Spec.Interval.Duration}, nil
}

func (r *HelmReleaseReconciler) reconcileTest(ctx context.Context, run *runner.Runner, hr *v2.HelmRelease, rls *release.Release) (ctrl.Result, error) {
	defer r.patchStatus(ctx, hr)
	defer r.recordReadiness(*hr)

	// If tests are not enabled for this resource, we have nothing to do.
	if !hr.Spec.GetTest().Enable {
		return ctrl.Result{}, nil
	}

	// If we already run the tests, we have nothing to do.
	if tested := apimeta.FindStatusCondition(hr.Status.Conditions, v2.TestSuccessCondition); tested != nil {
		return ctrl.Result{}, nil
	}

	// If the release did not succeed, we should not run the tests.
	released := apimeta.FindStatusCondition(hr.Status.Conditions, v2.ReleasedCondition)
	if released == nil || released.Status == metav1.ConditionFalse {
		return ctrl.Result{}, nil
	}

	// Run the tests.
	var err error
	_, err = run.Test(*hr)
	if err != nil {
		// Record the test failure.
		meta.SetResourceCondition(hr, v2.TestSuccessCondition, metav1.ConditionFalse, v2.TestFailedReason, err.Error())

		// Determine the remediation strategy that applies, and if we
		// need to increment the failure count.
		remediation := hr.Spec.GetInstall().GetRemediation()
		if released.Reason == v2.UpgradeSucceededReason {
			remediation = hr.Spec.GetUpgrade().GetRemediation()
		}
		if !remediation.MustIgnoreTestFailures(hr.Spec.GetTest().IgnoreFailures) {
			remediation.IncrementFailureCount(hr)
			meta.SetResourceCondition(hr, v2.ReleasedCondition, metav1.ConditionFalse, v2.TestFailedReason, err.Error())
			v2.HelmReleaseNotReady(hr, v2.TestFailedReason, err.Error())
		}

		return ctrl.Result{}, nil
	}

	// Record the test success and mark the release as ready.
	msg := "Tests succeeded"
	meta.SetResourceCondition(hr, v2.TestSuccessCondition, metav1.ConditionTrue, v2.TestSucceededReason, msg)
	v2.HelmReleaseReady(hr, msg)

	return ctrl.Result{}, nil
}

func (r *HelmReleaseReconciler) reconcileRemediation(ctx context.Context, run *runner.Runner, hr *v2.HelmRelease, rls *release.Release) (ctrl.Result, error) {
	defer r.patchStatus(ctx, hr)
	defer r.recordReadiness(*hr)

	// Compare the revision in our status to the revision of the given
	// release. If they still equal, we did not create a new release
	// during this reconciliation run and should not remediate.
	if revision := util.ReleaseRevision(rls); revision == hr.Status.LastReleaseRevision {
		return ctrl.Result{}, nil
	}

	// Determine the remediation strategy configuration based on the
	// failure counts.
	remediation := hr.Spec.GetUpgrade().GetRemediation()
	if remediation.GetFailureCount(*hr) == 0 {
		remediation = hr.Spec.GetInstall().GetRemediation()
	}

	// If there are no failures, we should not remediate.
	if remediation.GetFailureCount(*hr) == 0 {
		return ctrl.Result{}, nil
	}

	// Return if there are no retries left, or if we should not remediate
	// the last failure.
	if remediation.RetriesExhausted(*hr) && !remediation.MustRemediateLastFailure() {
		return ctrl.Result{}, nil
	}

	// Perform the remediation action for the configured strategy.
	switch remediation.GetStrategy() {
	case v2.RollbackRemediationStrategy:
		if err := run.Rollback(*hr); err != nil {
			meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionFalse, v2.RollbackFailedReason, err.Error())
			v2.HelmReleaseNotReady(hr, v2.RollbackFailedReason, err.Error())
			return ctrl.Result{}, nil
		}
		meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionTrue, v2.RollbackSucceededReason, "Rollback complete")
	case v2.UninstallRemediationStrategy:
		if err := run.Uninstall(*hr); err != nil {
			meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionFalse, v2.UninstallFailedReason, err.Error())
			v2.HelmReleaseNotReady(hr, v2.UninstallFailedReason, err.Error())
			return ctrl.Result{}, nil
		}
		meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionTrue, v2.UninstallSucceededReason, "Uninstallation complete")
	}

	// Determine the release revision after successfully performing
	// the remediation.
	var err error
	rls, err = run.ObserveLastRelease(*hr)
	if err != nil {
		meta.SetResourceCondition(hr, v2.RemediatedCondition, metav1.ConditionFalse, v2.GetLastReleaseFailedReason, err.Error())
		v2.HelmReleaseNotReady(hr, v2.GetLastReleaseFailedReason, err.Error())
		return ctrl.Result{}, nil
	}
	hr.Status.LastReleaseRevision = util.ReleaseRevision(rls)

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
