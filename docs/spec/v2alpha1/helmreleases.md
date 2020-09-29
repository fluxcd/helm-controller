# Helm Releases

The `HelmRelease` API defines a resource for automated controller driven Helm actions.

## Specification

A **helmrelease** object defines the source of a Helm chart by referencing an object
managed by [source-controller](https://github.com/fluxcd/source-controller), the interval
reconciliation should happen at, and a set of options to control the settings of the
automated Helm actions that are being performed.

```go
// HelmReleaseSpec defines the desired state of HelmRelease.
type HelmReleaseSpec struct {
	// Chart defines the template of the v1alpha1.HelmChart that should be created
	// for this HelmRelease.
	// +required
	Chart HelmChartTemplate `json:"chart"`

	// Interval at which to reconcile the Helm release.
	// +required
	Interval metav1.Duration `json:"interval"`

	// Suspend tells the controller to suspend reconciliation for this HelmRelease,
	// it does not apply to already started reconciliations. Defaults to false.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// ReleaseName used for the Helm release. Defaults to the name of the HelmRelease.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=53
	// +kubebuilder:validation:Optional
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// ReleaseNamespace used for the Helm release. Defaults to the namespace
	// of the HelmRelease.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Optional
	// +optional
	ReleaseNamespace string `json:"ReleaseNamespace,omitempty"`

	// DependsOn may contain a list of HelmReleases that must be ready before this
	// HelmRelease can be reconciled.
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`

	// Timeout is the time to wait for any individual Kubernetes operation (like Jobs
	// for hooks) during the performance of a Helm action. Defaults to '5m0s'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// MaxHistory is the number of revisions saved by Helm for this HelmRelease.
	// Use '0' for an unlimited number of revisions; defaults to '10'.
	// +optional
	MaxHistory *int `json:"maxHistory,omitempty"`

	// Install holds the configuration for Helm install actions for this HelmRelease.
	// +optional
	Install *Install `json:"install,omitempty"`

	// Upgrade holds the configuration for Helm upgrade actions for this HelmRelease.
	// +optional
	Upgrade *Upgrade `json:"upgrade,omitempty"`

	// Test holds the configuration for Helm test actions for this HelmRelease.
	// +optional
	Test *Test `json:"test,omitempty"`

	// Rollback holds the configuration for Helm rollback actions for this HelmRelease.
	// +optional
	Rollback *Rollback `json:"rollback,omitempty"`

	// Uninstall holds the configuration for Helm uninstall actions for this HelmRelease.
	// +optional
	Uninstall *Uninstall `json:"uninstall,omitempty"`

	// ValuesFrom holds references to resources containing Helm values for this HelmRelease,
	// and information about how they should be merged.
	ValuesFrom []ValuesReference `json:"valuesFrom,omitempty"`

	// Values holds the values for this Helm release.
	// +optional
	Values *apiextensionsv1.JSON `json:"values,omitempty"`
}

// HelmChartTemplate defines the template from which the controller will generate a
// v1alpha1.HelmChart object in the same namespace as the referenced v1alpha1.Source.
type HelmChartTemplate struct {
	// Spec holds the template for the v1alpha1.HelmChartSpec for this HelmRelease.
	// +required
	Spec HelmChartTemplateSpec `json:"spec"`
}

// HelmChartTemplateSpec defines the template from which the controller will generate
// a v1alpha1.HelmChartSpec object.
type HelmChartTemplateSpec struct {
	// The name or path the Helm chart is available at in the SourceRef.
	// +required
	Chart string `json:"chart"`

	// Version semver expression, ignored for charts from GitRepository and
	// Bucket sources. Defaults to latest when omitted.
	// +optional
	Version string `json:"version,omitempty"`

	// The name and namespace of the v1alpha1.Source the chart is available at.
	// +required
	SourceRef CrossNamespaceObjectReference `json:"sourceRef"`

	// Interval at which to check the v1alpha1.Source for updates.
	// Defaults to 'HelmReleaseSpec.Interval'.
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`
}

// Install holds the configuration for Helm install actions performed for this HelmRelease.
type Install struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like Jobs
	// for hooks) during the performance of a Helm install action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Remediation holds the remediation configuration for when the
	// Helm install action for the HelmRelease fails. The default
	// is to not perform any action.
	// +optional
	Remediation *InstallRemediation `json:"remediation,omitempty"`

	// DisableWait disables the waiting for resources to be ready after a
	// Helm install has been performed.
	// +optional
	DisableWait bool `json:"disableWait,omitempty"`

	// DisableHooks prevents hooks from running during the Helm install action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// DisableOpenAPIValidation prevents the Helm install action from
	// validating rendered templates against the Kubernetes OpenAPI Schema.
	// +optional
	DisableOpenAPIValidation bool `json:"disableOpenAPIValidation,omitempty"`

	// Replace tells the Helm install action to re-use the 'ReleaseName', but
	// only if that name is a deleted release which remains in the history.
	// +optional
	Replace bool `json:"replace,omitempty"`

	// SkipCRDs tells the Helm install action to not install any CRDs. By default,
	// CRDs are installed if not already present.
	// +optional
	SkipCRDs bool `json:"skipCRDs,omitempty"`
}

// InstallRemediation holds the configuration for Helm install remediation.
type InstallRemediation struct {
	// Retries is the number of retries that should be attempted on failures before
	// bailing. Remediation, using an uninstall, is performed between each attempt.
	// Defaults to '0', a negative integer equals to unlimited retries.
	// +optional
	Retries int `json:"retries,omitempty"`

	// IgnoreTestFailures tells the controller to skip remediation when
	// the Helm tests are run after an install action but fail.
	// Defaults to 'Test.IgnoreFailures'.
	// +optional
	IgnoreTestFailures *bool `json:"ignoreTestFailures,omitempty"`

	// RemediateLastFailure tells the controller to remediate the last
	// failure, when no retries remain. Defaults to 'false'.
	// +optional
	RemediateLastFailure *bool `json:"remediateLastFailure,omitempty"`
}

// Upgrade holds the configuration for Helm upgrade actions for this HelmRelease.
type Upgrade struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like Jobs
	// for hooks) during the performance of a Helm upgrade action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// Remediation holds the remediation configuration for when the
	// Helm upgrade action for the HelmRelease fails. The default
	// is to not perform any action.
	// +optional
	Remediation *UpgradeRemediation `json:"remediation,omitempty"`

	// DisableWait disables the waiting for resources to be ready after a
	// Helm upgrade has been performed.
	// +optional
	DisableWait bool `json:"disableWait,omitempty"`

	// DisableHooks prevents hooks from running during the Helm upgrade action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// DisableOpenAPIValidation prevents the Helm upgrade action from
	// validating rendered templates against the Kubernetes OpenAPI Schema.
	// +optional
	DisableOpenAPIValidation bool `json:"disableOpenAPIValidation,omitempty"`

	// Force forces resource updates through a replacement strategy.
	// +optional
	Force bool `json:"force,omitempty"`

	// PreserveValues will make Helm reuse the last release's values and merge
	// in overrides from 'Values'. Setting this flag makes the HelmRelease
	// non-declarative.
	// +optional
	PreserveValues bool `json:"preserveValues,omitempty"`

	// CleanupOnFail allows deletion of new resources created during the Helm
	// upgrade action when it fails.
	// +optional
	CleanupOnFail bool `json:"cleanupOnFail,omitempty"`
}

// UpgradeRemediation holds the configuration for Helm upgrade remediation.
type UpgradeRemediation struct {
	// Retries is the number of retries that should be attempted on failures before
	// bailing. Remediation, using 'Strategy', is performed between each attempt.
	// Defaults to '0', a negative integer equals to unlimited retries.
	// +optional
	Retries int `json:"retries,omitempty"`

	// IgnoreTestFailures tells the controller to skip remediation when
	// the Helm tests are run after an upgrade action but fail.
	// Defaults to 'Test.IgnoreFailures'.
	// +optional
	IgnoreTestFailures *bool `json:"ignoreTestFailures,omitempty"`

	// RemediateLastFailure tells the controller to remediate the last
	// failure, when no retries remain. Defaults to 'false' unless 'Retries'
	// is greater than 0.
	// +optional
	RemediateLastFailure *bool `json:"remediateLastFailure,omitempty"`

	// Strategy to use for failure remediation.
	// Defaults to 'rollback'.
	// +kubebuilder:validation:Enum=rollback;uninstall
	// +optional
	Strategy *RemediationStrategy `json:"strategy,omitempty"`
}

// Test holds the configuration for Helm test actions for this HelmRelease.
type Test struct {
	// Enable enables Helm test actions for this HelmRelease after an
	// Helm install or upgrade action has been performed.
	// +optional
	Enable bool `json:"enable,omitempty"`

	// Timeout is the time to wait for any individual Kubernetes operation
	// during the performance of a Helm test action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// IgnoreFailures tells the controller to skip remediation when
	// the Helm tests are run but fail.
	// Can be overwritten for tests run after install or upgrade actions
	// in 'Install.IgnoreTestFailures' and 'Upgrade.IgnoreTestFailures'.
	// +optional
	IgnoreFailures bool `json:"ignoreFailures,omitempty"`
}

// Rollback holds the configuration for Helm rollback actions for this HelmRelease.
type Rollback struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like Jobs
	// for hooks) during the performance of a Helm rollback action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// DisableWait disables the waiting for resources to be ready after a
	// Helm rollback has been performed.
	// +optional
	DisableWait bool `json:"disableWait,omitempty"`

	// DisableHooks prevents hooks from running during the Helm rollback action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// Recreate performs pod restarts for the resource if applicable.
	// +optional
	Recreate bool `json:"recreate,omitempty"`

	// Force forces resource updates through a replacement strategy.
	// +optional
	Force bool `json:"force,omitempty"`

	// CleanupOnFail allows deletion of new resources created during the Helm
	// rollback action when it fails.
	// +optional
	CleanupOnFail bool `json:"cleanupOnFail,omitempty"`
}

// Uninstall holds the configuration for Helm uninstall actions for this HelmRelease.
type Uninstall struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like Jobs
	// for hooks) during the performance of a Helm uninstall action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// DisableHooks prevents hooks from running during the Helm rollback action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// KeepHistory tells Helm to remove all associated resources and mark the release as
	// deleted, but retain the release history.
	// +optional
	KeepHistory bool `json:"keepHistory,omitempty"`
}
```

Reference types:

```go
// CrossNamespaceObjectReference contains enough information to let you locate the
// typed referenced object at cluster level.
type CrossNamespaceObjectReference struct {
	// APIVersion of the referent.
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind of the referent.
	// +kubebuilder:validation:Enum=HelmRepository
	// +required
	Kind string `json:"kind,omitempty"`

	// Name of the referent.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// Namespace of the referent.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:Optional
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ValuesReference contains a reference to a resource containing Helm values,
// and optionally the key they can be found at.
type ValuesReference struct {
	// Kind of the values referent, valid values are ('Secret', 'ConfigMap').
	// +kubebuilder:validation:Enum=Secret;ConfigMap
	// +required
	Kind string `json:"kind"`

	// Name of the values referent. Should reside in the same namespace as the
	// referring resource.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=253
	// +required
	Name string `json:"name"`

	// ValuesKey is the data key where the values.yaml or a specific value can
	// be found at. Defaults to 'values.yaml'.
	// +optional
	ValuesKey string `json:"valuesKey,omitempty"`

	// TargetPath is the YAML dot notation path the value should be merged at.
	// When set, the ValuesKey is expected to be a single flat value.
	// Defaults to 'None', which results in the values getting merged at the root.
	// +optional
	TargetPath string `json:"targetPath,omitempty"`
}
```

Status condition types:

```go
const (
	// ReadyCondition represents the fact that the HelmRelease has been successfully reconciled.
	ReadyCondition string = "Ready"

	// ReleasedCondition represents the fact that the HelmRelease has been successfully released.
	ReleasedCondition string = "Released"

	// TestSuccessCondition represents the fact that the tests for the HelmRelease are succeeding.
	TestSuccessCondition string = "TestSuccess"

	// RemediatedCondition represents the fact that the HelmRelease has been successfully remediated.
	RemediatedCondition string = "Remediated"
)
```

Status condition reasons:

```go
const (
	// ReconciliationSucceededReason represents the fact that the reconciliation of the HelmRelease has succeeded.
	ReconciliationSucceededReason string = "ReconciliationSucceeded"

	// ReconciliationFailedReason represents the fact that the reconciliation of the HelmRelease has failed.
	ReconciliationFailedReason string = "ReconciliationFailed"

	// InstallSucceededReason represents the fact that the Helm install for the HelmRelease succeeded.
	InstallSucceededReason string = "InstallSucceeded"

	// InstallFailedReason represents the fact that the Helm install for the HelmRelease failed.
	InstallFailedReason string = "InstallFailed"

	// UpgradeSucceededReason represents the fact that the Helm upgrade for the HelmRelease succeeded.
	UpgradeSucceededReason string = "UpgradeSucceeded"

	// UpgradeFailedReason represents the fact that the Helm upgrade for the HelmRelease failed.
	UpgradeFailedReason string = "UpgradeFailed"

	// TestSucceededReason represents the fact that the Helm tests for the HelmRelease succeeded.
	TestSucceededReason string = "TestSucceeded"

	// TestFailedReason represents the fact that the Helm tests for the HelmRelease failed.
	TestFailedReason string = "TestsFailed"

	// RollbackSucceededReason represents the fact that the Helm rollback for the HelmRelease succeeded.
	RollbackSucceededReason string = "RollbackSucceeded"

	// RollbackFailedReason represents the fact that the Helm test for the HelmRelease failed.
	RollbackFailedReason string = "RollbackFailed"

	// UninstallSucceededReason represents the fact that the Helm uninstall for the HelmRelease succeeded.
	UninstallSucceededReason string = "UninstallSucceeded"

	// UninstallFailedReason represents the fact that the Helm uninstall for the HelmRelease failed.
	UninstallFailedReason string = "UninstallFailed"

	// ArtifactFailedReason represents the fact that the artifact download for the HelmRelease failed.
	ArtifactFailedReason string = "ArtifactFailed"

	// InitFailedReason represents the fact that the initialization of the Helm configuration failed.
	InitFailedReason string = "InitFailed"

	// GetLastReleaseFailedReason represents the fact that observing the last release failed.
	GetLastReleaseFailedReason string = "GetLastReleaseFailed"

	// ProgressingReason represents the fact that the reconciliation for the resource is underway.
	ProgressingReason string = "Progressing"

	// DependencyNotReadyReason represents the fact that the one of the dependencies is not ready.
	DependencyNotReadyReason string = "DependencyNotReady"

	// SuspendedReason represents the fact that the reconciliation of the HelmRelease is suspended.
	SuspendedReason string = "Suspended"
)
```

## Source reference

The `HelmRelease` `spec.chart.spec.sourceRef` is a reference to an object managed by
[source-controller](https://github.com/fluxcd/source-controller). When the source
[revision](https://github.com/fluxcd/source-controller/blob/master/docs/spec/v1alpha1/common.md#source-status) 
changes, it generates a Kubernetes event that triggers a new release.

Supported source types:

* [HelmRepository](https://github.com/fluxcd/source-controller/blob/master/docs/spec/v1alpha1/helmrepositories.md)
* [GitRepository](https://github.com/fluxcd/source-controller/blob/master/docs/spec/v1alpha1/gitrepositories.md)
* [Bucket](https://github.com/fluxcd/source-controller/blob/master/docs/spec/v1alpha1/buckets.md)

## Reconciliation

The `HelmRelease` `spec.interval` tells the reconciler at which interval to reconcile the release. The
interval time units are `s`, `m` and `h` e.g. `interval: 5m`, the minimum value should be over 60 seconds.

The reconcilation can be suspended by setting `spec.susped` to `true`.

The reconciler can be told to reconcile the `HelmRelease` outside of the specified interval
by annotating the object with:

```go
const (
	// ReconcileAtAnnotation is the annotation used for triggering a
	// reconciliation outside of the specified schedule.
	ReconcileAtAnnotation string = "fluxcd.io/reconcileAt"
)
```

On-demand execution example:

```bash
kubectl annotate --overwrite helmrelease/podinfo fluxcd.io/reconcileAt="$(date +%s)"
```

## `HelmRelease` dependencies

When applying a `HelmRelease`, you may need to make sure other releases exist before the release is
reconciled. For example, because your chart relies on the presence of a Custom Resource Definition
installed by another `HelmRelease`.

With `spec.dependsOn` you can specify that the execution of a `HelmRelease` follows another. When
you add `dependsOn` entries to a `HelmRelease`, that `HelmRelease` is reconciled only after all of
its dependencies are ready. The readiness state of a `HelmRelease` is determined by its last apply
status condition.

Assuming two `HelmRelease` resources:

- `backend` - contains the backend of the application
- `frontend` - contains the frontend of the application and relies on the backend

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2alpha1
kind: HelmRelease
metadata:
  name: backend
spec:
  interval: 5m
  chart:
    spec:
      chart: podinfo
      version: '>=4.0.0 <5.0.0'
      sourceRef:
        kind: HelmRepository
        name: podinfo
      interval: 1m
  upgrade:
    remediation:
      remediateLastFailure: true
  test:
    enable: true
  values:
    service:
      grpcService: backend
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
---
apiVersion: helm.toolkit.fluxcd.io/v2alpha1
kind: HelmRelease
metadata:
  name: frontend
spec:
  interval: 5m
  chart:
    spec:
      chart: podinfo
      version: '>=4.0.0 <5.0.0'
      sourceRef:
        kind: HelmRepository
        name: podinfo
      interval: 1m
  dependsOn:
    - backend
  upgrade:
    remediation:
      remediateLastFailure: true
  test:
    enable: true
  values:
    backend: http://backend-podinfo:9898/echo
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
```

> **Note** that circular dependencies between `HelmRelease` resources must be avoided, otherwise
> the interdependent `HelmRelease` resources will never be reconciled.

## Enabling Helm rollback actions

From time to time a Helm upgrade made by the helm-controller may fail, automatically recovering
from this via a Helm rollback action is possible by enabling rollbacks for the `HelmRelease`.

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2alpha1
kind: HelmRelease
metadata:
  name: podinfo
spec:
  interval: 5m
  chart:
    spec:
      chart: podinfo
      version: '>=4.0.0 <5.0.0'
      sourceRef:
        kind: HelmRepository
        name: podinfo
      interval: 1m
  upgrade:
    remediation:
      remediateLastFailure: true
  values:
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
```

## Configuring failure remediation

By default, when a Helm action (install/upgrade/test) fails, no remediation is taken
(uninstall/rollback/retries). However, remediation can be opted in to in several ways
using `spec.install.remediation` and `spec.upgrade.remediation`.

Each of these support `retries`, to configure the number of additional attempts after an initial
failure. A negative integer results in infinite retries. This implicitly opts-in to a remediation
action between each attempt. The remediation action for install failures is an uninstall. The
remediation action for upgrade failures is by default a rollback, however
`spec.upgrade.remediation.strategy` can be set to `uninstall`, in which case after the uninstall,
the `spec.install` configuration takes over.

One can also opt-in to remediation of the last failure (when no retries remain) by:

1. For installs, setting `spec.install.remediation.remediateLastFailure` to `true`.
2. For upgrades, setting `spec.upgrade.remediation.remediateLastFailure` to `true`, or configuring
   at least one retry.

   ```yaml
   apiVersion: helm.fluxcd.io/v2alpha1
   kind: HelmRelease
   metadata:
     name: podinfo
   spec:
     interval: 5m
     chart:
       spec:
         chart: podinfo
         version: '>=4.0.0 <5.0.0'
         sourceRef:
           kind: HelmRepository
           name: podinfo
         interval: 1m
     install:
       remediation:
         retries: 3
     upgrade:
       remediation:
         remediateLastFailure: false
     values:
       resources:
         requests:
           cpu: 100m
           memory: 64Mi
   ```

## Configuring Helm test actions

To make the controller run the Helm tests available for your chart after a successful Helm install
or upgrade, `spec.test.enable` should be set to `true`.

By default, when tests are enabled, failures in tests are considered release failures, and thus
are subject to the triggering Helm action's `remediation` configuration. However, test failures
can be ignored by setting `spec.test.ignoreFailures` to `true`. In this case, no remediation will
be taken, and the test failure will not affect the `Ready` status condition. This can be further
configured per Helm action by setting `spec.install.remediation.ignoreTestFailures` or
`spec.upgrade.remediation.ignoreTestFailures`, which default to `spec.test.ignoreFailures`.

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2alpha1
kind: HelmRelease
metadata:
  name: podinfo
spec:
  interval: 5m
  chart:
    spec:
      chart: podinfo
    version: '>=4.0.0 <5.0.0'
    sourceRef:
      kind: HelmRepository
      name: podinfo
    interval: 1m
  test:
    enable: true
    ignoreFailures: true
  values:
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
```

## Status

When the controller completes a reconciliation, it reports the result in the status sub-resource.

The following `status.conditions` are supported:

* `Ready` - status of the last reconciliation attempt
* `Released` - status of the last release attempt (install/upgrade/test) against the current state
* `TestSuccess` - status of the last test attempt against the current state
* `Remediated` - status of the last remediation attempt (uninstall/rollback) due to a failure of the
  last release attempt against the current state

You can wait for the helm-controller to complete a reconciliation with:

```sh
kubectl wait helmrelease/podinfo --for=condition=ready
```

### Examples

Install Success:

```yaml
status:
  conditions:
  - lastTransitionTime: "2020-07-13T13:13:40Z"
    message: Helm install succeeded
    reason: InstallSucceeded
    status: "True"
    type: Released
  - lastTransitionTime: "2020-07-13T13:13:40Z"
    message: Helm test succeeded
    reason: TestSucceeded
    status: "True"
    type: TestSuccess
  - lastTransitionTime: "2020-07-13T13:13:42Z"
    message: release reconciliation succeeded
    reason: ReconciliationSucceeded
    status: "True"
    type: Ready
  lastAppliedRevision: 4.0.6
  lastAttemptedRevision: 4.0.6
  lastObservedTime: "2020-07-13T13:18:42Z"
  lastReleaseRevision: 1
  observedGeneration: 2
```

Upgrade Failure:

```yaml
status:
  conditions:
  - lastTransitionTime: "2020-07-13T13:17:28Z"
    message: 'error validating "": error validating data: ValidationError(Deployment.spec.replicas):
      invalid type for io.k8s.api.apps.v1.DeploymentSpec.replicas: got "string",
      expected "integer"'
    reason: UpgradeFailed
    status: "False"
    type: Released
  - lastTransitionTime: "2020-07-13T13:17:28Z"
    message: 'error validating "": error validating data: ValidationError(Deployment.spec.replicas):
      invalid type for io.k8s.api.apps.v1.DeploymentSpec.replicas: got "string",
      expected "integer"'
    reason: UpgradeFailed
    status: "False"
    type: Ready
  failures: 1
  lastAppliedRevision: 4.0.6
  lastAttemptedRevision: 4.0.6
  lastObservedTime: "2020-07-13T18:17:28Z"
  lastReleaseRevision: 1
  observedGeneration: 3
```

Ignored Test Failure:

```yaml
status:
  conditions:
  - lastTransitionTime: "2020-07-13T13:13:40Z"
    message: Helm install succeeded
    reason: InstallSucceeded
    status: "True"
    type: Released
  - lastTransitionTime: "2020-07-13T13:13:40Z"
    message: Helm test failed
    reason: TestsFailed
    status: "False"
    type: TestSuccess
  - lastTransitionTime: "2020-07-13T13:13:42Z"
    message: release reconciliation succeeded
    reason: ReconciliationSucceeded
    status: "True"
    type: Ready
  lastAppliedRevision: 4.0.6
  lastAttemptedRevision: 4.0.6
  lastObservedTime: "2020-07-13T13:18:42Z"
  lastReleaseRevision: 1
  observedGeneration: 2
```
