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
	// Chart defines the Helm chart name, version and repository.
	// +required
	Chart HelmChartTemplate `json:"chart"`

	// Interval at which to reconcile the Helm release.
	// +required
	Interval metav1.Duration `json:"interval"`

	// Suspend tells the reconciler to suspend reconciliation for this HelmRelease,
	// it does not apply to already started reconciliations. Defaults to false.
	// +optional
	Suspend bool `json:"suspend,omitempty"`

	// ReleaseName used for the Helm release. Defaults to a composition of
	// '[TargetNamespace-]Name'.
	// +optional
	ReleaseName string `json:"releaseName,omitempty"`

	// TargetNamespace to target when performing operations for the HelmRelease.
	// Defaults to the namespace of the HelmRelease.
	// +optional
	TargetNamespace string `json:"targetNamespace,omitempty"`

	// DependsOn may contain a list of HelmReleases that must be ready before this
	// HelmRelease can be reconciled.
	// +optional
	DependsOn []string `json:"dependsOn,omitempty"`

	// Timeout is the time to wait for any individual Kubernetes operation (like Jobs
	// for hooks) during the performance of a Helm action. Defaults to '5m0s'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// MaxHistory is the number of revisions saved by Helm for this release.
	// Use '0' for an unlimited number of revisions; defaults to '10'.
	// +optional
	MaxHistory *int `json:"maxHistory,omitempty"`

	// Install holds the configuration for Helm install actions for this release.
	// +optional
	Install Install `json:"install,omitempty"`

	// Upgrade holds the configuration for Helm upgrade actions for this release.
	// +optional
	Upgrade Upgrade `json:"upgrade,omitempty"`

	// Test holds the configuration for Helm test actions for this release.
	// +optional
	Test Test `json:"test,omitempty"`

	// Rollback holds the configuration for Helm rollback actions for this release.
	// +optional
	Rollback Rollback `json:"rollback,omitempty"`

	// Uninstall holds the configuration for Helm uninstall actions for this release.
	// +optional
	Uninstall Uninstall `json:"uninstall,omitempty"`

	// ValuesFrom holds references to resources containing Helm values, and information
	// about how they should be merged.
	ValuesFrom []ValuesReference `json:"valuesFrom,omitempty"

	// Values holds the values for this Helm release.
	// +optional
	Values apiextensionsv1.JSON `json:"values,omitempty"`
}

// HelmChartTemplate defines the template from which the controller
// will generate a HelmChart object in the same namespace as the HelmRepository.
type HelmChartTemplate struct {
	// Name of the Helm chart, as made available by the referenced Helm repository.
	// +required
	Name string `json:"name"`

	// Version semver expression, defaults to latest when omitted.
	// +optional
	Version string `json:"version,omitempty"`

	// The name and namespace of the source HelmRepository the chart is available at.
	// +required
	SourceRef CrossNamespaceObjectReference `json:"sourceRef"`

	// Interval at which to check the Helm repository for chart updates.
	// Defaults to 'HelmReleaseSpec.Interval'.
	// +optional
	Interval *metav1.Duration `json:"interval,omitempty"`
}

// Install holds the configuration for Helm install actions.
type Install struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like Jobs
	// for hooks) during the performance of a Helm install action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

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

// Upgrade holds the configuration for Helm upgrade actions.
type Upgrade struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like Jobs
	// for hooks) during the performance of a Helm upgrade action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// MaxRetries is the number of retries that should be attempted on failures before
	// bailing. Defaults to '0', a negative integer equals to unlimited retries.
	// +optional
	MaxRetries int `json:"maxRetries,omitempty"`

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

// Test holds the configuration for Helm test actions.
type Test struct {
	// Enable enables Helm test actions for this release after an
	// Helm install or upgrade action has been performed.
	// +optional
	Enable bool `json:"enable,omitempty"`

	// Timeout is the time to wait for any individual Kubernetes operation
	// during the performance of a Helm test action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// Rollback holds the configuration for Helm rollback actions.
type Rollback struct {
	// Enable enables Helm rollback actions for this release after an
	// Helm install or upgrade action failure.
	// +optional
	Enable bool `json:"enable,omitempty"`

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

// Uninstall holds the configuration for Helm uninstall actions.
type Uninstall struct {
	// Timeout is the time to wait for any individual Kubernetes operation (like Jobs
	// for hooks) during the performance of a Helm uninstall action. Defaults to
	// 'HelmReleaseSpec.Timeout'.
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`

	// DisableHooks prevents hooks from running during the Helm rollback action.
	// +optional
	DisableHooks bool `json:"disableHooks,omitempty"`

	// KeepHistory tells Helm to remove all associated resources and mark the release
	// as deleted, but retain the release history.
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
	// +required
	Name string `json:"name"`

	// Namespace of the referent.
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
	// +required
	Name string `json:"name"`
	
	// ValuesKey is the key in the referent the values can be found at.
	// Defaults to 'values.yaml'.
	// +optional
	ValuesKey string `json:"valuesKey,omitempty"`
}
```

Status condition types:

```go
const (
	// ReadyCondition represents the fact that the HelmRelease has been successfully reconciled.
	ReadyCondition string = "Ready"

	// InstalledCondition represents the fact that the HelmRelease has been successfully installed.
	InstalledCondition string = "Installed"

	// UpgradedCondition represents the fact that the HelmRelease has been successfully upgraded.
	UpgradedCondition string = "Upgraded"

	// TestedCondition represents the fact that the HelmRelease has been successfully tested.
	TestedCondition string = "Tested"

	// RolledBackCondition represents the fact that the HelmRelease has been successfully rolled back.
	RolledBackCondition string = "RolledBack"

	// UninstalledCondition represents the fact that the HelmRelease has been successfully uninstalled.
	UninstalledCondition string = "Uninstalled"
)
```

Status condition reasons:

```go
const (
	// ReconciliationSucceededReason represents the fact that the reconciliation of the release has succeeded.
	ReconciliationSucceededReason string = "ReconciliationSucceeded"

	// ReconciliationFailedReason represents the fact that the reconciliation of the release has failed.
	ReconciliationFailedReason string = "ReconciliationFailed"

	// InstallSucceededReason represents the fact that the Helm install for the release succeeded.
	InstallSucceededReason string = "InstallSucceeded"

	// InstallFailedReason represents the fact that the Helm install for the release failed.
	InstallFailedReason string = "InstallFailed"

	// UpgradeSucceededReason represents the fact that the Helm upgrade for the release succeed.
	UpgradeSucceededReason string = "UpgradeSucceeded"

	// UpgradeFailedReason represents the fact that the Helm upgrade for the release failed.
	UpgradeFailedReason string = "UpgradeFailed"

	// TestFailedReason represents the fact that the Helm test for the release failed.
	TestSucceededReason string = "TestSucceeded"

	// TestFailedReason represents the fact that the Helm test for the release failed.
	TestFailedReason string = "TestFailed"

	// RollbackSucceededReason represents the fact that the Helm rollback for the release succeeded.
	RollbackSucceededReason string = "RollbackSucceeded"

	// RollbackFailedReason represents the fact that the Helm test for the release failed.
	RollbackFailedReason string = "RollbackFailed"

	// UninstallSucceededReason represents the fact that the Helm uninstall for the release succeeded.
	UninstallSucceededReason string = "UninstallSucceeded"

	// UninstallFailedReason represents the fact that the Helm uninstall for the release failed.
	UninstallFailedReason string = "UninstallFailed"

	// ArtifactFailedReason represents the fact that the artifact download for the release failed.
	ArtifactFailedReason string = "ArtifactFailed"

	// InitFailedReason represents the fact that the initialization of the Helm configuration failed.
	InitFailedReason string = "InitFailed"

	// ProgressingReason represents the fact that the reconciliation for the resource is underway.
	ProgressingReason string = "Progressing"

	// DependencyNotReadyReason represents the fact that the one of the dependencies is not ready.
	DependencyNotReadyReason string = "DependencyNotReady"

	// SuspendedReason represents the fact that the reconciliation of the HelmRelease is suspended.
	SuspendedReason string = "Suspended"
)
```

## Source reference

The `HelmRelease` `spec.chart.sourceRef` is a reference to an object managed by
[source-controller](https://github.com/fluxcd/source-controller). When the source
[revision](https://github.com/fluxcd/source-controller/blob/master/docs/spec/v1alpha1/common.md#source-status) 
changes, it generates a Kubernetes event that triggers a new release.

Supported source types:

* [HelmRepository](https://github.com/fluxcd/source-controller/blob/master/docs/spec/v1alpha1/helmrepositories.md)

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
    name: podinfo
    version: '^4.0.0'
    sourceRef:
      kind: HelmRepository
      name: podinfo
    interval: 1m
  test:
    enable: true
  rollback:
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
    name: podinfo
    version: '^4.0.0'
    sourceRef:
      kind: HelmRepository
      name: podinfo
    interval: 1m
  dependsOn:
    - backend
  test:
    enable: true
  rollback:
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

From time to time an Helm upgrade made by the helm-controller may fail, automatically recovering
from this via a Helm rollback action is possible by enabling rollbacks for the `HelmRelease`.

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2alpha1
kind: HelmRelease
metadata:
  name: podinfo
spec:
  interval: 5m
  chart:
    name: podinfo
    version: '^4.0.0'
    sourceRef:
      kind: HelmRepository
      name: podinfo
    interval: 1m
  rollback:
    enable: true
  values:
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
```

At present, rollbacks are only supported for failed upgrades. Rollback support for other failed
actions (i.e. tests) is in the scope of the controller but awaits a proper design.

## Enabling Helm test actions

To make the controller run the Helm tests available for your chart after a successful Helm install
or upgrade, `spec.test.enable` should be set to `true`.

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2alpha1
kind: HelmRelease
metadata:
  name: podinfo
spec:
  interval: 5m
  chart:
    name: podinfo
    version: '^4.0.0'
    sourceRef:
      kind: HelmRepository
      name: podinfo
    interval: 1m
  test:
    enable: true
  values:
    resources:
      requests:
        cpu: 100m
        memory: 64Mi
```

At present, failed tests do not mark the `HelmRelease` as not `Ready`. Making this configurable is
in the scope of the controller but awaits a proper design, as well as running them on a schedule or
for other actions than a successful Helm install or upgrade.

## Status

When the controller completes a reconciliation, it reports the result in the status sub-resource.

A successful reconciliation sets the `Ready` condition to `true`:

```yaml
status:
  conditions:
  - lastTransitionTime: "2020-07-13T13:13:40Z"
    message: Helm installation succeeded
    reason: InstallSucceeded
    status: "True"
    type: Install
  - lastTransitionTime: "2020-07-13T13:13:42Z"
    message: release reconciliation succeeded
    reason: ReconciliationSucceeded
    status: "True"
    type: Ready
  lastAppliedRevision: 4.0.6
  lastAttemptedRevision: 4.0.6
  lastReleaseRevision: 1
  observedGeneration: 2
```

You can wait for the helm-controller to complete a reconciliation with:

```sh
kubectl wait helmrelease/podinfo --for=condition=ready
```

A failed reconciliation sets the `Ready` condition to `false`:

```yaml
status:
  conditions:
  - lastTransitionTime: "2020-07-13T13:17:28Z"
    message: 'error validating "": error validating data: ValidationError(Deployment.spec.replicas):
      invalid type for io.k8s.api.apps.v1.DeploymentSpec.replicas: got "string",
      expected "integer"'
    reason: UpgradeFailed
    status: "False"
    type: Upgrade
  - lastTransitionTime: "2020-07-13T13:17:28Z"
    message: release reconciliation failed
    reason: ReconciliationFailed
    status: "False"
    type: Ready
  failures: 1
  lastAppliedRevision: 4.0.6
  lastAttemptedRevision: 4.0.6
  lastReleaseRevision: 1
  observedGeneration: 3
```
