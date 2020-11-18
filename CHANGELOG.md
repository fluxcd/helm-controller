# Changelog

All notable changes to this project are documented in this file.

## 0.2.2 (2020-11-18)

This prerelease comes with a bugfix for chart divergence detections.

## 0.2.1 (2020-11-17)

This prerelease comes with improvements to status reporting, and a
bugfix for the (temporary) dead lock that would occur on some
transient values composition and chart loading errors.

## 0.2.0 (2020-10-29)

This is the second MINOR prerelease, it comes with a breaking change:

* The histogram metric `gotk_reconcile_duration` was renamed to `gotk_reconcile_duration_seconds`

Other notable changes:

* Added support for cross-cluster Helm releases by defining a `KubeConfig`
  reference in the `HelmReleaseSpec`.
* The annotation `fluxcd.io/reconcileAt` was renamed to `reconcile.fluxcd.io/requestedAt`,
  the former will be removed in a next release but is backwards
  compatible for now.

## 0.1.3 (2020-10-16)

This prereleases fixes two bugs:

- `HelmRelease` resources with a `spec.valuesFrom` reference making use
  of a `targetPath` defined as the first item will now compose without
  failing.
- The chart reconciliation and readiness logic has been rewritten to
  better work with no-op chart updates and guarantee readiness state
  observation accuracy. This prevents it from `HelmRelease`s getting
  stuck on a "HelmChart is not ready" state.

## 0.1.2 (2020-10-13)

This prerelease comes with Prometheus instrumentation for the controller's resources.

For each kind, the controller exposes a gauge metric to track the `Ready` condition status,
and a histogram with the reconciliation duration in seconds:

* `gotk_reconcile_condition{kind, name, namespace, status, type="Ready"}`
* `gotk_reconcile_duration{kind, name, namespace}`

## 0.1.1 (2020-10-02)

This prerelease fixes a regression bug introduced in `v0.1.0`
resulting in the `spec.targetNamespace` not being taken into
account.

## 0.1.0 (2020-09-30)

This is the first MINOR prerelease, it promotes the
`helm.toolkit.fluxcd.io` API to `v2beta1` and removes support for
`v2alpha1`.

Going forward, changes to the API will be accompanied by a conversion
mechanism. With this release the API becomes more stable, but while in
beta phase there are no guarantees about backwards compatibility
between beta releases.

A breaking change was introduced to the `Status` object, as the
`LastObservedTime` field has been removed in favour of the newly
introduced `LastHandledReconcileAt`. This field records the value
of the `fluxcd.io/reconcilateAt` annotation, which makes it possible
for e.g. the `gotk` CLI to observe if the controller has handled
the resource since the manual reconciliation request was made.

## 0.0.10 (2020-09-23)

This prerelease adds support for Helm charts from `Bucket` sources,
support for optional `ValuesFrom` references, and a Helm upgrade from
`3.3.3` to `3.3.4`.

## 0.0.9 (2020-09-22)

This prerelease adds support for `DependsOn` references to other namespaces
than the `HelmRelease` resource resides in, container images for ARMv7 and
ARMv8 published to `ghcr.io/fluxcd/helm-controller-arm64`, a Helm upgrade
from `3.3.1` to `3.3.3`, and a refactor of the `Status` object.

The latter introduces the following breaking changes to the `Status` object:

* The `Installed`, `Upgraded`, `RolledBack`, and `Uninstalled` conditions
  have been removed, since they did not represent current state, but rather
  actions taken, which are already recorded by events.
* The `ObservedStateReconciled` field has been removed, since it solved the
  problem of remembering past release successes, but not past release
  failures, after other subsequent failures such as dependency failures,
  Kubernetes API failures, etc.
* The `Tested` condition has been renamed to `TestSuccess`, for forward
  compatibility with interval based Helm tests.

While introducing the following new `Status` conditions:

* `Remediated` which records whether the release is currently in a
   remediated state. It is used to prevent release retries after remediation
   failures. We were previously not doing this for rollback failures.
* `Released` which records whether the current state has been successfully
   released. This is used to remember the last release attempt status,
   regardless of any subsequent other failures such as dependency failures,
   Kubernetes API failures, etc.

## 0.0.8 (2020-09-11)

This prerelease adds support for defining a `ValuesFile` in the
`HelmChartTemplateSpec` to overwrite the default chart values with another
values file, as supported by `>=0.0.15` of the source-controller, and a
`--watch-all-namespaces` flag (defaults to `true`) to provide the option
to only watch the runtime namespace of the controller for resources.

## 0.0.7 (2020-09-04)

This prerelease comes with documentation fixes.
Container images for linux/amd64 and linux/arm64 are published to GHCR.

## 0.0.6 (2020-09-02)

This prerelease adds support for Helm charts from `GitRepository` sources,
improvements for a more graceful failure recovery, and an upgrade of Helm
from `v3.0.0` to `v3.0.1`. It includes several (breaking) changes to the
API.

The `spec` of the `HelmRelease` has a multitude of breaking changes:

* `spec.chart` (which contained the template for the `HelmChart` template)
  has moved one level down to `spec.chart.spec`. This matches the pod
  template defined in the Kubernetes `Deployment` kind, and allows for
  adding e.g. a `spec.chart.metadata` field in a future iteration to be
  able to define annotations and/or labels.
* The `spec.chart.name` field has been renamed to `spec.chart.spec.chart`,
  and now accepts a chart name (for charts from `HelmRepository` 
  sources) or a path (for charts from `GitRepository` sources), to follow
  changes made to the `HelmChart` API.
* The `spec.chart.spec.sourceRef.kind` is now mandatory, and accepts both
  `HelmRepository` and `GitRepository` values.

The `status` object has two new fields to help end-users and automation
(tools) with observing state:

* `observedStateReconciled` represents whether the observed state of the
  has been successfully reconciled. This field is marked as `true` on a
  `Ready==True` condition, and only reset on `generation`, values, and/or
  chart changes.
* `lastObservedTime` reflects the last time at which the `HelmRelease` was
  observed. This can for example be used to observe if the `HelmRelease` is
  running on the configured `spec.interval` and/or reacting to `ReconcileAt`
  annotations.

## 0.0.5 (2020-08-26)

This prerelease adds support for conditional remediation on failed Helm
actions, and includes several (breaking) changes to the API:

* The `maxRetries` value should now be set on the respective
  `install.remediation.retries` and `upgrade.remediation.retries` fields.
* The `rollback.enable` field has been removed in favour of
  `upgrade.remediateLastFailure`.
* Failing Helm tests will now result in a `False` `Ready` condition by
  default, ignoring test failures can be re-enabled by configuring
  `test.ignoreFailures` to `true`.

## 0.0.4 (2020-08-20)

This prerelease adds support for merging a flat single value from
a `ValueReference` at the defined `TargetPath`, and fixes a bug in
the merging of values where overwrites of a map with a flat single
value was not allowed.

## 0.0.3 (2020-08-18)

This prerelease upgrades the `github.com/fluxcd/pkg/*` dependencies to
dedicated versioned modules, and makes the `api` package available as
a dedicated versioned module.

## 0.0.2 (2020-08-12)

In this prerelease the Helm package was upgraded to [v3.3.0](https://github.com/helm/helm/releases/tag/v3.3.0).

## 0.0.1 (2020-07-31)

This prerelease comes with a breaking change, the CRDs group has been
renamed to `helm.toolkit.fluxcd.io`. The dependency on `source-controller`
has been updated to `v0.0.7` to be able to work with `source.toolkit.fluxcd.io`
resources.

## 0.0.1-beta.4 (2020-07-22)

This beta release fixes a bug affecting helm release status reevaluation.

## 0.0.1-beta.3 (2020-07-21)

This beta release fixes a bug affecting helm charts reconciliation.

## 0.0.1-beta.2 (2020-07-21)

This beta release comes with various bug fixes and minor improvements.

## 0.0.1-beta.1 (2020-07-20)

This beta release drops support for Kubernetes <1.16.
The CRDs have been updated to `apiextensions.k8s.io/v1`.

## 0.0.1-alpha.2 (2020-07-16)

This alpha release comes with improvements to alerts delivering,
logging, and fixes a bug in the lookup of HelmReleases when a
HelmChart revision had changed.

## 0.0.1-alpha.1 (2020-07-13)

This is the first alpha release of helm-controller.
The controller is an implementation of the
[helm.fluxcd.io/v2alpha1](https://github.com/fluxcd/helm-controller/tree/v0.0.1-alpha.1/docs/spec/v2alpha1) API.
