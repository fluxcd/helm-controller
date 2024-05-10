# Changelog

## 1.0.1

**Release date:** 2024-05-10

This prerelease fixes a backwards compatibility issue that could occur when trying
to move from the `v2beta1` to `v2` API while specifing `.spec.chartRef`.

Fixes:
- Fix: Allow upgrading from v2beta1 to v2 (GA)
  [#982](https://github.com/fluxcd/helm-controller/pull/982)
- Fix: Make HelmChartTemplate a pointer in .spec.chart
  [#980](https://github.com/fluxcd/helm-controller/pull/980)

## 1.0.0

**Release date:** 2024-05-08

This is the general availability release of helm-controller. From now on, this controller
follows the [Flux release cadence and support pledge](https://fluxcd.io/flux/releases/).

This release promotes the `HelmRelease` API from `v2beta2` to `v2` (GA), and
comes with new features, improvements and bug fixes.

In addition, the controller has been updated to Kubernetes v1.30.0,
Helm v3.14.4, and various other dependencies to their latest version
to patch upstream CVEs.

### Highlights

The `helm.toolkit.fluxcd.io/v2` API comes with a new field
[`.spec.chartRef`](https://github.com/fluxcd/helm-controller/blob/release-v1.0.0-rc.1/docs/spec/v2/helmreleases.md#chart-reference)
that adds support for referencing `OCIRepository` and `HelmChart` objects in a `HelmRelease`.
When using `.spec.chartRef` instead of `.spec.chart`, the controller allows the reuse
of a Helm chart version across multiple `HelmRelease` resources.

The notification mechanism has been improved to provide more detailed metadata
in the notification payload. The controller now annotates the Kubernetes events with
the `appVersion` and `version` of the Helm chart, and the `oci digest` of the
chart artifact when available.

### Helm OCI support

Starting with this version, the recommended way of referencing Helm charts stored
in container registries is through [OCIRepository](https://fluxcd.io/flux/components/source/ocirepositories/).

The `OCIRepository` provides more flexibility in managing Helm charts,
as it allows targeting a Helm chart version by `tag`, `semver` or OCI `digest`.
It also provides a way to
[filter semver tags](https://github.com/fluxcd/source-controller/blob/release/v1.3.x/docs/spec/v1beta2/ocirepositories.md#semverfilter-example),
allowing targeting a specific version range e.g. pre-releases only, patch versions, etc.

Using `OCIRepository` objects instead of `HelmRepository` and `HelmChart` objects
improves the controller's performance and simplifies the debugging process.
If a chart version gets overwritten in the container registry, the controller
will detect the change in the upstream OCI digest and reconcile the `HelmRelease`
resources accordingly.
[Promoting](https://fluxcd.io/flux/use-cases/gh-actions-helm-promotion/)
a Helm chart version to production can be done by pinning the `OCIRepository`
to an immutable digest, ensuring that the chart version is not changed unintentionally.

Helm OCI example:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: OCIRepository
metadata:
  name: podinfo
  namespace: default
spec:
  interval: 10m
  layerSelector:
    mediaType: "application/vnd.cncf.helm.chart.content.v1.tar+gzip"
    operation: copy
  url: oci://ghcr.io/stefanprodan/charts/podinfo
  ref:
    semver: "*"
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: podinfo
  namespace: default
spec:
  interval: 10m
  chartRef:
    kind: OCIRepository
    name: podinfo
```

#### API changes

The `helm.toolkit.fluxcd.io` CRD contains the following versions:
- v2 (storage version)
- v2beta2 (deprecated)
- v2beta1 (deprecated)

New optional fields have been added to the `HelmRelease` API:

- `.spec.chartRef` allows referencing chart artifacts from `OCIRepository` and `HelmChart` objects.
- `.spec.chart.spec.ignoreMissingValuesFiles` allows ignoring missing values files instead of failing to reconcile.

Deprecated fields have been removed from the `HelmRelease` API:

- `.spec.chart.spec.valuesFile` replaced by `.spec.chart.spec.valuesFiles`
- `.spec.postRenderers.kustomize.patchesJson6902` replaced by `.spec.postRenderers.kustomize.patches`
- `.spec.postRenderers.kustomize.patchesStrategicMerge` replaced by `.spec.postRenderers.kustomize.patches`
- `.status.lastAppliedRevision` replaced by `.status.history.chartVersion`

#### Upgrade procedure

1. Before upgrading the controller, ensure that the `HelmRelease` v2beta2 manifests stored in Git
   are not using the deprecated fields. Search for `valuesFile` and replace it with `valuesFiles`,
   replace `patchesJson6902` and `patchesStrategicMerge` with `patches`. 
   Commit and push the changes to the Git repository, then wait for Flux to reconcile the changes.
2. Upgrade the controller and CRDs to v1.0.0 on the cluster using Flux v2.3 release.
   Note that helm-controller v1.0.0 requires source-controller v1.3.0.
3. Update the `apiVersion` field of the `HelmRelease` resources to `helm.toolkit.fluxcd.io/v2`,
   commit and push the changes to the Git repository.

Bumping the API version in manifests can be done gradually.
It is advised to not delay this procedure as the beta versions will be removed after 6 months.

### Full changelog

Improvements:
- Add the chart app version to status and events metadata
  [#968](https://github.com/fluxcd/helm-controller/pull/968)
- Promote HelmRelease API to v2 (GA)
  [#963](https://github.com/fluxcd/helm-controller/pull/963)
- Add `.spec.ignoreMissingValuesFiles` to HelmChartTemplate API
  [#942](https://github.com/fluxcd/helm-controller/pull/942)
- Update HelmChart API to v1 (GA)
  [#962](https://github.com/fluxcd/helm-controller/pull/962)
- Update dependencies to Kubernetes 1.30.0
  [#944](https://github.com/fluxcd/helm-controller/pull/944)
- Add support for HelmChart to `.spec.chartRef`
  [#945](https://github.com/fluxcd/helm-controller/pull/945)
- Add support for OCIRepository to `.spec.chartRef`
  [#905](https://github.com/fluxcd/helm-controller/pull/905)
- Update dependencies to Kustomize v5.4.0
  [#932](https://github.com/fluxcd/helm-controller/pull/932)
- Add notation verification provider to API
  [#930](https://github.com/fluxcd/helm-controller/pull/930)
- Update controller to Helm v3.14.3 and Kubernetes v1.29.0
  [#879](https://github.com/fluxcd/helm-controller/pull/879)
- Update controller-gen to v0.14.0
  [#910](https://github.com/fluxcd/helm-controller/pull/910)

Fixes:
- Track changes in `.spec.postRenderers`
  [#965](https://github.com/fluxcd/helm-controller/pull/965)
- Update Ready condition during drift correction
  [#885](https://github.com/fluxcd/helm-controller/pull/885)
- Fix patching on drift detection
  [#935](https://github.com/fluxcd/helm-controller/pull/935)
- Use corev1 event type for sending events
  [#908](https://github.com/fluxcd/helm-controller/pull/908)
- Reintroduce missing events for helmChart reconciliation failures
  [#907](https://github.com/fluxcd/helm-controller/pull/907)
- Remove `genclient:Namespaced` tag
  [#901](https://github.com/fluxcd/helm-controller/pull/901)

## 0.37.4

**Release date:** 2024-02-05

This prerelease comes with improvements in the HelmRelease status reporting.
After recovering from a reconciliation failure, sometimes the status may show
stale conditions which could be misleading. This has been fixed by ensuring that
the stale failure conditions get updated after failure recovery.

Improvements:
- Remove stale Ready=False conditions value to show more accurate status
  [#884](https://github.com/fluxcd/helm-controller/pull/884)
- Dependency update
  [#886](https://github.com/fluxcd/helm-controller/pull/886)

## 0.37.3

**Release date:** 2024-02-01

This prerelease comes with an update to the Kubernetes dependencies to
v1.28.6 and various other dependencies have been updated to their latest version
to patch upstream CVEs.

In addition, the controller is now built with Go 1.21.

Improvements:
- ci: Enable dependabot gomod updates
  [#874](https://github.com/fluxcd/helm-controller/pull/874)
- Update Go to 1.21
  [#872](https://github.com/fluxcd/helm-controller/pull/872)
- Various dependency updates
  [#882](https://github.com/fluxcd/helm-controller/pull/882)
  [#877](https://github.com/fluxcd/helm-controller/pull/877)
  [#876](https://github.com/fluxcd/helm-controller/pull/876)
  [#871](https://github.com/fluxcd/helm-controller/pull/871)
  [#867](https://github.com/fluxcd/helm-controller/pull/867)
  [#865](https://github.com/fluxcd/helm-controller/pull/865)
  [#862](https://github.com/fluxcd/helm-controller/pull/862)
  [#860](https://github.com/fluxcd/helm-controller/pull/860)

## 0.37.2

This prerelease fixes a bug that resulted in the controller not being able to
properly watch HelmRelease resources with specific labels.

Fixes:
- Properly configure namespace selector
  [#858](https://github.com/fluxcd/helm-controller/pull/858)

Improvements:
- build(deps): bump golang.org/x/crypto from 0.16.0 to 0.17.0
  [#856](https://github.com/fluxcd/helm-controller/pull/856)

## 0.37.1

This prerelease fixes a backwards compatibility issue that could occur when
trying to move from the `v2beta1` to `v2beta2` API while enabling drift
detection.

In addition, logging has been improved to provide faster feedback on any
HTTP errors encountered while fetching HelmChart artifacts, and the controller
will now set the `Stalled` condition as soon as it detects to be out of retries
without having to wait for the next reconciliation.

Lastly, Helm has been updated to v3.13.3.

Fixes:
- loader: allow overwrite of URL hostname again
  [#844](https://github.com/fluxcd/helm-controller/pull/844)
- api: ensure backwards compatibility v2beta1
  [#851](https://github.com/fluxcd/helm-controller/pull/851)

Improvements:
- loader: log HTTP errors to provide faster feedback
  [#845](https://github.com/fluxcd/helm-controller/pull/845)
- Update runtime to v0.43.3
  [#846](https://github.com/fluxcd/helm-controller/pull/846)
- Early stall condition detection after remediation
  [#848](https://github.com/fluxcd/helm-controller/pull/848)
- Update Helm to v3.13.3
  [#849](https://github.com/fluxcd/helm-controller/pull/849)

## 0.37.0

**Release date:** 2023-12-12

This prerelease promotes the `HelmRelease` API from `v2beta1` to `v2beta2`.
The promotion of the API is accompanied by a number of new features and bug
fixes. Refer to the highlights section below for more information.

In addition to the API promotion, this prerelease updates the controller
dependencies to their latest versions. Making the controller compatible with
Kubernetes v1.28.x, while updating the Helm library to v3.13.2, and the builtin
version of Kustomize used for post-rendering to v5.3.0.

Lastly, the base controller image has been updated to Alpine v3.19.

### Highlights

#### API changes

The upgrade is backwards compatible, and the controller will continue to
reconcile `HelmRelease` resources of the `v2beta1` API without requiring any
changes. However, making use of the new features requires upgrading the API
version.

- Drift detection and correction is now enabled on a per-release basis using
  the `.spec.driftDetection.mode` field. Refer to the [drift detection section](https://github.com/fluxcd/helm-controller/blob/v0.37.0/docs/spec/v2beta2/helmreleases.md#drift-detection)
  in the `v2beta2` specification for more information.
- Ignoring specific fields during drift detection and correction is now
  supported using the `.spec.driftDetection.ignore` field. Refer to the
  [ignore rules section](https://github.com/fluxcd/helm-controller/blob/v0.37.0/docs/spec/v2beta2/helmreleases.md#ignore-rules)
  in the `v2beta2` specification to learn more.
- Helm tests can now be selectively run using the `.spec.test.filters` field.
  Refer to the [test filters section](https://github.com/fluxcd/helm-controller/blob/v0.37.0/docs/spec/v2beta2/helmreleases.md#filtering-tests)
  in the `v2beta2` specification for more details.
- The controller now offers proper integration with [`kstatus`](https://github.com/kubernetes-sigs/cli-utils/blob/master/pkg/kstatus/README.md)
  and sets `Reconciling` and `Stalled` conditions. See the [Conditions section](https://github.com/fluxcd/helm-controller/blob/v0.37.0/docs/spec/v2beta2/helmreleases.md#conditions)
  in the `v2beta2` specification to read more about the conditions.
- The `.spec.maxHistory` default value has been lowered from `10` to `5` to
  increase the controller's performance.
- A history of metadata from Helm releases up to the previous successful release
  is now available in the `.status.history` field. This includes any Helm test
  results when enabled.
- The `.patchesStrategicMerge` and `.patchesJson6902` Kustomize post-rendering
  fields have been deprecated in favor of `.patches`.
- A `status.lastAttemptedConfigDigest` field has been introduced to track the
  last attempted configuration digest using a hash of the composed values.
- A `.status.lastAttemptedReleaseAction` field has been introduced to accurately
  determine the active remediation strategy.
- The `.status.lastHandledForceAt` and `.status.lastHandledResetAt` fields have
  been introduced to track the last time a force upgrade or reset was handled.
  This to accomadate newly introduced annotations to force upgrades and resets.
- The `.status.lastAppliedRevision` and `.status.lastReleaseRevision` fields
  have been deprecated in favor of `.status.history`.
- The `.status.lastAttemptedValuesChecksum` has been deprecated in favor of
  `.status.lastAttemptedConfigDigest`.

Although the `v2beta1` API is still supported, it is recommended to upgrade to
the `v2beta2` API as soon as possible. The `v2beta1` API will be removed after
6 months.

To upgrade to the `v2beta2` API, update the `apiVersion` field of your
`HelmRelease` resources to `helm.toolkit.fluxcd.io/v2beta2` after updating the
controller and Custom Resource Definitions.

#### Other notable improvements

- The reconciliation model of the controller has been improved to be able to
  better determine the state a Helm release is in. An example of this is that
  enabling Helm tests will not require a Helm upgrade to be run, but instead
  will run immediately if the release is in a `deployed` state already.
- The controller will detect Helm releases in a `pending-install`, `pending-upgrade`
  or `pending-rollback` state, and wil forcefully unlock the release (to a
  `failed` state) to allow the controller to reattempt the release.
- When drift correction is enabled, the controller will now attempt to correct
  drift it detects by creating and patching Kubernetes resources instead of
  running a Helm upgrade.
- The controller emits more detailed Kubernetes Events after running a Helm
  action. In addition, the controller will now emit a Kubernetes Event when
  a Helm release is uninstalled.
- The controller provides richer Condition messages before and after running a
  Helm action.
- Changes to a HelmRelease `.spec` which require a Helm uninstall for the
  changes to be successfully applied are now detected. For example, a change in
  `.spec.targetNamespace` or `.spec.releaseName`.
- When the release name exceeds the maximum length of 53 characters, the
  controller will now truncate the release name to 40 characters and append a
  short SHA256 hash of the release name prefixed with a `-` to ensure the
  release name is unique.
- New annotations have been introduced to force a Helm upgrade or to reset the
  number of retries for a release. Refer to the [forcing a release](https://github.com/fluxcd/helm-controller/blob/v0.37.0/docs/spec/v2beta2/helmreleases.md#forcing-a-release)
  and [resetting remediation retries](https://github.com/fluxcd/helm-controller/blob/v0.37.0/docs/spec/v2beta2/helmreleases.md#resetting-remediation-retries)
  sections in the `v2beta2` specification for more information.
- The digest algorithm used to calculate the digest of the composed values and
  hash of the release object can now be configured using the `--snapshot-digest-algo`
  controller flag. The default value is `sha256`.
- When the `HelmChart` resource for a `HelmRelease` is not `Ready`, the
  Conditions of the `HelmRelease` will now contain more detailed information
  about the reason.

To get a full overview of all changes, and see examples of the new features.
Please refer to the [v2beta2 specification](https://github.com/fluxcd/helm-controller/blob/v0.37.0/docs/spec/v2beta2/helmreleases.md).

### Full changelog

Improvements:
- Update dependencies
  [#791](https://github.com/fluxcd/helm-controller/pull/791)
  [#792](https://github.com/fluxcd/helm-controller/pull/792)
  [#799](https://github.com/fluxcd/helm-controller/pull/799)
  [#812](https://github.com/fluxcd/helm-controller/pull/812)
- Update source-controller dependency to v1.2.1
  [#793](https://github.com/fluxcd/helm-controller/pull/793)
  [#835](https://github.com/fluxcd/helm-controller/pull/835)
- Rework `HelmRelease` reconciliation logic
  [#738](https://github.com/fluxcd/helm-controller/pull/738)
  [#816](https://github.com/fluxcd/helm-controller/pull/816)
  [#825](https://github.com/fluxcd/helm-controller/pull/825)
  [#829](https://github.com/fluxcd/helm-controller/pull/829)
  [#830](https://github.com/fluxcd/helm-controller/pull/830)
  [#833](https://github.com/fluxcd/helm-controller/pull/833)
  [#836](https://github.com/fluxcd/helm-controller/pull/836)
- Update Kubernetes 1.28.x, Helm v3.13.2 and Kustomize v5.3.0
  [#817](https://github.com/fluxcd/helm-controller/pull/817)
  [#839](https://github.com/fluxcd/helm-controller/pull/839)
- Allow configuration of drift detection on `HelmRelease`
  [#815](https://github.com/fluxcd/helm-controller/pull/815)
- Allow configuration of snapshot digest algorithm
  [#818](https://github.com/fluxcd/helm-controller/pull/818)
- Remove obsolete code and tidy things
  [#819](https://github.com/fluxcd/helm-controller/pull/819)
- Add deprecation warning to v2beta1 API
  [#821](https://github.com/fluxcd/helm-controller/pull/821)
- Correct cluster drift using patches
  [#822](https://github.com/fluxcd/helm-controller/pull/822)
- Introduce `forceAt` and `resetAt` annotations
  [#823](https://github.com/fluxcd/helm-controller/pull/823)
- doc/spec: document `v2beta2` API
  [#828](https://github.com/fluxcd/helm-controller/pull/828)
- api: deprecate stategic merge and JSON 6902 patches
  [#832](https://github.com/fluxcd/helm-controller/pull/832)
- controller: enrich "HelmChart not ready" messages
  [#834](https://github.com/fluxcd/helm-controller/pull/834)
- build: update Alpine to 3.19
  [#838](https://github.com/fluxcd/helm-controller/pull/838)

## 0.36.2

**Release date:** 2023-10-11

This prerelease contains an improvement to retry the reconciliation of a
`HelmRelease` as soon as the chart is available in storage, instead of waiting
for the next reconciliation interval. Which is particularly useful when the
source-controller has just been upgraded.

In addition, it fixes a bug in which the controller would not properly label
Custom Resource Definitions.

Fixes:
- runner: ensure CRDs are properly labeled
  [#781](https://github.com/fluxcd/helm-controller/pull/781)
- fix: retry failed releases when charts are available in storage
  [#785](https://github.com/fluxcd/helm-controller/pull/785)

Improvements:
- Address typo in documentation
  [#777](https://github.com/fluxcd/helm-controller/pull/777)
- Update CI dependencies
  [#783](https://github.com/fluxcd/helm-controller/pull/783)
  [#786](https://github.com/fluxcd/helm-controller/pull/786)
- Address miscellaneous issues throughout code base
  [#788](https://github.com/fluxcd/helm-controller/pull/788)

## 0.36.1

**Release date:** 2023-09-18

This prerelease addresses a regression in which the captured Helm logs used in
a failure event would not include Helm's Kubernetes client logs, making it more
difficult to reason about e.g. timeout errors.

In addition, it contains a fix for the default service account used for the
(experimental) differ, and dependency updates of several dependencies.

Fixes:
- runner: address regression in captured Helm logs
  [#767](https://github.com/fluxcd/helm-controller/pull/767)
- Check source for nil artifact before loading chart
  [#768](https://github.com/fluxcd/helm-controller/pull/768)
- controller: use `DefaultServiceAccount` in differ
  [#774](https://github.com/fluxcd/helm-controller/pull/774)

Improvements:
- build(deps): bump the ci group dependencies
  [#761](https://github.com/fluxcd/helm-controller/pull/761)
  [#762](https://github.com/fluxcd/helm-controller/pull/762)
  [#766](https://github.com/fluxcd/helm-controller/pull/766)
  [#773](https://github.com/fluxcd/helm-controller/pull/773)
- build(deps): bump github.com/cyphar/filepath-securejoin from 0.2.3 to 0.2.4
  [#764](https://github.com/fluxcd/helm-controller/pull/764)
- Update source-controller to v1.1.1
  [#775](https://github.com/fluxcd/helm-controller/pull/775)

## 0.36.0

**Release date:** 2023-08-23

This prerelease introduces a `--interval-jitter-percentage` flag to the
controller to distribute the load more evenly when multiple objects are set up
with the same interval. The default of this flag is set to `5`, which means
that the interval will be jittered by a +/- 5% random value (e.g. if the
interval is 10 minutes, the actual reconciliation interval will be between 9.5
and 10.5 minutes).

In addition, the controller now stops exporting an object's metrics as soon as
it has been deleted.

Lastly, dependencies have been updated, such as an update of Helm to `v3.12.3`
and Kubernetes related dependencies to `v0.27.4`.

Improvements:
- Update dependencies
  [#748](https://github.com/fluxcd/helm-controller/pull/748)
- controller: jitter requeue interval
  [#751](https://github.com/fluxcd/helm-controller/pull/751)
- Delete stale metrics on delete
  [#753](https://github.com/fluxcd/helm-controller/pull/753)
- Update Helm to v3.12.3
  [#754](https://github.com/fluxcd/helm-controller/pull/754)
- Update Source API to v1.1.0
  [#758](https://github.com/fluxcd/helm-controller/pull/758)

Fixes:
- chore: fix typo reconciliation
  [#736](https://github.com/fluxcd/helm-controller/pull/736)

## 0.35.0

**Release date:** 2023-07-04

This prerelease adds support for configuring the deletion propagation policy to
use when a Helm uninstall is performed using `.spec.uninstall.deletionPropagation`,
which was [added as a feature to Helm in `v3.12.0`](https://github.com/helm/helm/releases/tag/v3.12.0).
Supported values are `background`, `foreground` and `orphan` (defaults to
`background`). See the [Helm documentation](https://helm.sh/docs/chart_best_practices/deleting/#deletion-propagation)

In addition, it offers support for Kubernetes `v1.27.3` and includes updates to
the controller's dependencies, such as an upgrade of Helm to `v3.12.1`.

Starting with this version, the build, release and provenance portions of the
Flux project supply chain [provisionally meet SLSA Build Level 3](https://fluxcd.io/flux/security/slsa-assessment/).

Improvements:
- Set deletion propagation for helm uninstall
  [#698](https://github.com/fluxcd/helm-controller/pull/698)
- Align `go.mod` version with Kubernetes (Go 1.20)
  [#715](https://github.com/fluxcd/helm-controller/pull/715)
- Update Go dependencies
  [#726](https://github.com/fluxcd/helm-controller/pull/726)
- Update source-controller to v1.0.0
  [#729](https://github.com/fluxcd/helm-controller/pull/729)

## 0.34.2

**Release date:** 2023-06-22

This prerelease fixes a regression bug for long-running reconciliations introduced
in v0.34.0.

In addition, the controller release workflow was enhanced with SLSA level 3 generators.
Starting with this version, provenance attestations are generated for
the controller release assets and for the multi-arch container images.

Fixes:
- Fix HelmRelease reconciliation loop
  [#703](https://github.com/fluxcd/helm-controller/pull/703)

Improvements:
- Add SLSA3 generators to release workflow
  [#705](https://github.com/fluxcd/helm-controller/pull/705)

## 0.34.1

**Release date:** 2023-06-01

This prerelease comes with a bug fix for the event metadata revision, which
was not included when a token was already present.

In addition, the source-controller dependency has been updated to v1.0.0-rc.5.

Fixes:
- Include revision and token in event metadata
  [#695](https://github.com/fluxcd/helm-controller/pull/695)

Improvements:
- Update source-controller to v1.0.0-rc.5
  [#696](https://github.com/fluxcd/helm-controller/pull/696)

## 0.34.0

**Release date:** 2023-05-29

This prerelease comes with support for Helm 3.12.0 and Kustomize v5.0.3.

⚠️ Note that Kustomize v5 contains breaking changes, please consult their
[changelog](https://github.com/kubernetes-sigs/kustomize/releases/tag/kustomize%2Fv5.0.0)
for more details.

In addition, the controller dependencies have been updated to
Kubernetes v1.27.2 and controller-runtime v0.15.0.

Lastly, the logic to forward events to notification-controller has been modified
to use `.status.lastAttemptedValuesChecksum` as an event metadata token to
prevent incorrect rate limiting.

Improvements:
- Update Kubernetes, controller-runtime and Helm
  [#690](https://github.com/fluxcd/helm-controller/pull/690)
- Remove the tini supervisor, and other nits
  [#691](https://github.com/fluxcd/helm-controller/pull/691)
- Use last attempted values checksum as event metadata token
  [#692](https://github.com/fluxcd/helm-controller/pull/692)
- Update source-controller to v1.0.0-rc.4
  [#693](https://github.com/fluxcd/helm-controller/pull/693)

## 0.33.0

**Release date:** 2023-05-12

This prerelease comes with a change to the calculation of the release values
checksum. Previously, the checksum was calculated based on the values as
provided by the user, which could lead to an upgrade when the values changed
order, but not content. This has been changed to calculate the checksum based
on the values after stable sorting them by key. This means that the checksum
will only change when the values actually change.

In addition, the dependencies have been updated including a mitigation for
CVE-2023-2253, and the controller base image has been updated to Alpine 3.18.

Improvements:
- Stable sort release values by key
  [#684](https://github.com/fluxcd/helm-controller/pull/684)
- Update Alpine to 3.18
  [#685](https://github.com/fluxcd/helm-controller/pull/685)
- build(deps): bump github.com/docker/distribution from 2.8.1+incompatible to 2.8.2+incompatible
  [#686](https://github.com/fluxcd/helm-controller/pull/686)
- Update dependencies
  [#687](https://github.com/fluxcd/helm-controller/pull/687)

## 0.32.2

**Release date:** 2023-04-13

This prerelease comes with a bug fix for a nil pointer deref that could occur
when drift detection was enabled.

In addition, it updates Helm to `v3.11.3`, which includes two fixes for Go
routine leaks.

Fixes:
- Fix nil pointer deref during diff attempt
  [#672](https://github.com/fluxcd/helm-controller/pull/672)
- Update Helm to v3.11.3
  [#673](https://github.com/fluxcd/helm-controller/pull/673)

## 0.32.1

**Release date:** 2023-04-03

This prerelease comes with a bug fix in HelmRelease related to the
`.spec.chart.metadata` field which wasn't truly optional, leading to empty value
assignment even when it wasn't set.

Fixes:
- Fix chart metadata by making it truly optional
  [#665](https://github.com/fluxcd/helm-controller/pull/665)

## 0.32.0

**Release date:** 2023-03-31

This prerelease comes with a number of new features and improvements, and
solves a long-standing issue with the patching of the HelmRelease status.

### Highlights

#### Management of HelmChart labels and annotations

The HelmRelease now supports the definition of a set of labels and annotations
using `.spec.chart.metadata.labels` and `.spec.chart.metadata.annotations`,
which will be applied to the HelmChart created by the controller.

#### Sharding

The controller can now be configured with `--watch-label-selector`, after
which only HelmRelease objects with this label will be reconciled by the
controller.

This allows for horizontal scaling, where the controller can be deployed
multiple times with a unique label selector which is used as the sharding key.

Note that if you want to ensure a HelmChart gets created for a specific
source-controller instance, you have to provide the labels for this controller
in `.spec.chart.metadata.labels` of the HelmRelease.

In addition, the source referenced (i.e. HelmRepository) in the HelmChart must
be available to this same controller instance.

#### Opt-out of persistent Kubernetes client

The HelmRelease now supports opting out of the persistent Kubernetes client
introduced in `v0.31.0` by defining `.spec.persistentClient: false` (default
`true`).

This can be useful when a HelmRelease is used to manage a Helm chart that
itself manages Custom Resource Definitions outside the Helm chart's CRD
lifecycle (like OPA Gatekeeper), as the persistent client will not observe
the creation of these resources.

Disabling this increases memory consumption, and should only be used when
necessary.

#### Verification of Artifact Digest

The controller will now verify the Digest of the Artifact as advertised by
the HelmChart, introduced in source-controller `v0.35.0`.

Due to this, the controller will now require source-controller `v0.35.0` or
higher (and ships with `v1.0.0-rc.1` by default, which includes v1 of the
`Artifact` API).

### Full changelog

Improvements:
- Manage labels and annotations for a HelmChart
  [#631](https://github.com/fluxcd/helm-controller/pull/631)
- Verify Digest of Artifact
  [#651](https://github.com/fluxcd/helm-controller/pull/651)
- Move `controllers` to `internal/controllers`
  [#653](https://github.com/fluxcd/helm-controller/pull/653)
- Update dependencies
  [#654](https://github.com/fluxcd/helm-controller/pull/654)
- Add reconciler sharding capability based on label selector
  [#658](https://github.com/fluxcd/helm-controller/pull/658)
- Add `PersistentClient` flag to allow control over Kubernetes client behavior
  [#659](https://github.com/fluxcd/helm-controller/pull/659)
- Update source-controller to v1.0.0-rc.1
  [#661](https://github.com/fluxcd/helm-controller/pull/661)
- config/*: update API versions and file names
  [#662](https://github.com/fluxcd/helm-controller/pull/662)

Fixes:
- Update status patch logic
  [#660](https://github.com/fluxcd/helm-controller/pull/660)

## 0.31.2

**Release date:** 2023-03-20

This prerelease extends the drift detection feature introduced in `v0.31.0`
with support for disabling the correction of drift using the `CorrectDrift`
feature gate.

When disabled while `DetectDrift` is enabled (using `--feature-gates=DetectDrift=true,CorrectDrift=false`),
the controller will only emit events and log messages when drift is detected,
but will not attempt to correct it. This allows to transition to drift
detection and correction in a controlled manner.

In addition, the controller dependencies have been updated to their latest
versions.

Fixes:
- Allow opt-out of drift correction
  [#647](https://github.com/fluxcd/helm-controller/pull/647)

Improvements:
- Update dependencies
  [#649](https://github.com/fluxcd/helm-controller/pull/649)

## 0.31.1

**Release date:** 2023-03-10

This prerelease extends the OOM watch feature introduced in `v0.31.0` with
support for automatic detection of cgroup v1 paths, and flags to configure
alternative paths using `--oom-watch-max-memory-path` and
`--oom-watch-current-memory-path`.

Fixes:
- oomwatch: auto-detect well known cgroup paths
  [#641](https://github.com/fluxcd/helm-controller/pull/641)

## 0.31.0

**Release date:** 2023-03-08

This prerelease comes with a number of new features and improvements after a
long period of non-substantial changes.

### Highlights

#### Experimental drift detection

The controller now supports experimental drift detection, which can be enabled
by configuring the Deployment with `--feature-gates=DetectDrift=true`. This
feature is still in its early stages, and lacks certain UX features. Diff
output is currently available in the controller logs when the `--log-level=debug`
flag is set.

The feature itself makes use of the same approach as kustomize-controller to
detect drift using a dry-run Server Side Apply of the rendered manifests of a
release. When drift is detected, the controller will emit an event and trigger
a Helm upgrade.

When a specific object from a release causes spurious upgrades, it can be
excluded by annotating or labeling the object with
`helm.toolkit.fluxcd.io/driftDetection: disabled`. Refer to the [drift detection
documentation](https://github.com/fluxcd/helm-controller/blob/v0.31.0/docs/spec/v2beta1/helmreleases.md#excluding-resources-from-drift-detection)
for more information.

#### Cancellation of actions on controller shutdown

When a `SIGTERM` signal is received by the controller, it will now propagate
this to any running Helm action, which will mark the release as `failed`. This
should prevent the controller from getting stuck in a `pending` state when
receiving a `SIGTERM` signal.

#### Detection of near OOM

The controller can now be configured to detect when it is nearing an OOM kill.
This is enabled by configuring the Deployment with
`--feature-gates=OOMWatch=true`.

When enabled, the controller will monitor its memory usage as reported by
cgroups, and when it is nearing OOM, attempt to gracefully shutdown. Releases
that are currently being upgraded will be cancelled (resulting in a `failed`
release as opposed to a `pending` deadlock), and no new releases will be
started.

This is best combined with a thoughtful configuration of remediation strategies
on the `HelmRelease` resources, to ensure that the controller can recover from
the failed release.

To control the threshold at which the controller will attempt to shut down, use
the `--oom-watch-memory-threshold` (default `95`) and `--oom-watch-interval`
(default `500ms`) flags.

In a future release, we will add support for unlocking releases that are in a
pending state as a different approach to handling OOM situations. But this is
waiting for architectural changes to happen first.

#### Kubernetes client improvements

We have made a number of improvements to the Kubernetes client used by the
controller for Helm actions, which should reduce the memory usage of the
controller and the number of API requests it makes when creating or replacing
Custom Resource Definitions.

#### Miscellaneous

`klog` is now configured to log using the same logger as the rest  of the
controller (providing a consistent log format).

In addition, the controller is now built with Go 1.20, and the dependencies
have been updated.

### Full changelog

Improvements:
- Enable experimental drift detection
  [#617](https://github.com/fluxcd/helm-controller/pull/617)
- helm: propagate context to install and upgrade
  [#620](https://github.com/fluxcd/helm-controller/pull/620)
- Check if Service Account exists before uninstalling release
  [#623](https://github.com/fluxcd/helm-controller/pull/623)
- runner: configure Helm action cfg log levels
  [#625](https://github.com/fluxcd/helm-controller/pull/625)
- Update dependencies
  [#626](https://github.com/fluxcd/helm-controller/pull/626)
  [#627](https://github.com/fluxcd/helm-controller/pull/627)
  [#635](https://github.com/fluxcd/helm-controller/pull/635)
- Add OOM watcher to allow graceful shutdown
  [#628](https://github.com/fluxcd/helm-controller/pull/628)
- kube: unify clients into single RESTClientGetter
  [#630](https://github.com/fluxcd/helm-controller/pull/630)
- Use `logger.SetLogger` to also configure `klog`
  [#633](https://github.com/fluxcd/helm-controller/pull/633)

## 0.30.0

**Release date:** 2023-02-17

This prerelease adds support for parsing the
[RFC-0005](https://github.com/fluxcd/flux2/tree/main/rfcs/0005-artifact-revision-and-digest)
revision format produced by source-controller `>=v0.35.0`.

In addition, the controller dependencies have been updated to their latest
versions, including a security patch of Helm to `v3.11.1`.

Improvements:
- Support RFC-0005 revision format
  [#606](https://github.com/fluxcd/helm-controller/pull/606)
- Update dependencies
  [#610](https://github.com/fluxcd/helm-controller/pull/610)
- Update source-controller to v0.35.1
  [#612](https://github.com/fluxcd/helm-controller/pull/612)

## 0.29.0

**Release date:** 2023-02-01

This prerelease comes with an update of Kubernetes dependencies to v1.26, Helm
to v3.11.0, and a general update of other dependencies to their latest versions.

Starting with this release, Custom Resource Definitions installed by the
controller as part of a [`Create` or `CreateReplace`
policy](https://github.com/fluxcd/helm-controller/blob/main/docs/spec/v2beta1/helmreleases.md#crds)
are now labeled with `helm.toolkit.fluxcd.io/name` and
`helm.toolkit.fluxcd.io/namespace` to allow tracking the `HelmRelease` origin
of the resource. Note that these labels are only added to new and/or changed
resources, existing resources will not be updated.

Improvements:
- build: Enable SBOM and SLSA Provenance
  [#594](https://github.com/fluxcd/helm-controller/pull/594)
- Update dependencies
  [#595](https://github.com/fluxcd/helm-controller/pull/595)
- Patch CRDs with origin labels
  [#596](https://github.com/fluxcd/helm-controller/pull/596)
- Update source-controller to v0.34.0
  [#597](https://github.com/fluxcd/helm-controller/pull/597)

## 0.28.1

**Release date:** 2022-12-22

This prerelease sets the default value for the `--graceful-shutdown-timeout` to
match the default value we set in the Deployment for
`terminationGracePeriodSeconds` which is 600s. This should fix handling graceful
shutdown correctly.

Fixes:
- Align `graceful-shutdown-timeout` with `terminationGracePeriodSeconds` 
  [#584](https://github.com/fluxcd/helm-controller/pull/584)

## 0.28.0

**Release date:** 2022-12-20

This prerelease disables the caching of Secret and ConfigMap resources to
improve memory usage. To opt-out from this behaviour, start the controller
with: `--feature-gates=CacheSecretsAndConfigMaps=true`.

In addition, a new flag `--graceful-shutdown-timeout` has been added to
control the duration of the graceful shutdown period. The default value is
`-1` (disabled), to help prevent releases from being stuck due to the
controller being terminated before the Helm action has completed.

Helm has been updated to v3.10.3, which includes security fixes.

Fixes:
- Assign the value of `DisableOpenApiValidation` during upgrade
  [#564](https://github.com/fluxcd/helm-controller/pull/564)
- build: Fix cifuzz and improve fuzz tests' reliability
  [#565](https://github.com/fluxcd/helm-controller/pull/565)
- Minor typo in doc
  [#566](https://github.com/fluxcd/helm-controller/pull/566)
- fuzz: Use build script from upstream and fix fuzzers
  [#578](https://github.com/fluxcd/helm-controller/pull/578)

Improvements:
- Disable caching of Secrets and ConfigMaps
  [#513](https://github.com/fluxcd/helm-controller/513)
- Allow overriding ctrl manager graceful shutdown timeout
  [#570](https://github.com/fluxcd/helm-controller/pull/570)
  [#582](https://github.com/fluxcd/helm-controller/pull/582)
- helm: Update SDK to v3.10.3
  [#577](https://github.com/fluxcd/helm-controller/pull/577)
- Update source-controller and dependencies
  [#581](https://github.com/fluxcd/helm-controller/pull/581)

## 0.27.0

**Release date:** 2022-11-22

This prerelease comes with re-added support for `h` in the HelmRelease
`spec.timeout` field, so that users can use hours to set reconciliation
timeouts.

Improvements:
- Allow 'h' in HelmRelease timeout field
  [#559](https://github.com/fluxcd/helm-controller/pull/559)
- Use Flux Event API v1beta1
  [#557](https://github.com/fluxcd/helm-controller/pull/557)
- Update dependencies
  [#561](https://github.com/fluxcd/helm-controller/pull/561)


## 0.26.0

**Release date:** 2022-10-21

This prerelease comes with support for Cosign verification of Helm OCI charts.
The signatures verification can be configured by setting `HelmRelease.spec.chart.spec.verify` with
`provider` as `cosign` and a `secretRef` to a secret containing the public key.
Cosign keyless verification is also supported, please see the
[HelmChart API documentation](https://github.com/fluxcd/source-controller/blob/api/v0.31.0/docs/spec/v1beta2/helmcharts.md#verification)
for more details.

In addition, the controller dependencies have been updated
to Kubernetes v1.25.3 and Helm v3.10.1.

Improvements:
- Enable Cosign verification of Helm charts stored as OCI artifacts in container registries
  [#545](https://github.com/fluxcd/helm-controller/pull/545)
- API: allow configuration of h unit for timeouts
  [#549](https://github.com/fluxcd/helm-controller/pull/549)
- Update dependencies
  [#550](https://github.com/fluxcd/helm-controller/pull/550)

## 0.25.0

**Release date:** 2022-09-29

This prerelease comes with strict validation rules for API fields which define a
(time) duration. Effectively, this means values without a time unit (e.g. `ms`,
`s`, `m`, `h`) will now be rejected by the API server. To stimulate sane
configurations, the units `ns`, `us` and `µs` can no longer be configured, nor
can `h` be set for fields defining a timeout value.

In addition, the controller dependencies have been updated
to Kubernetes controller-runtime v0.13.

:warning: **Breaking changes:**
- `.spec.interval` new validation pattern is `"^([0-9]+(\\.[0-9]+)?(ms|s|m|h))+$"`
- `.spec.timeout` new validation pattern is `"^([0-9]+(\\.[0-9]+)?(ms|s|m))+$"`

Improvements:
- api: add custom validation for v1.Duration types
  [#533](https://github.com/fluxcd/helm-controller/pull/533)
- Build with Go 1.19
  [#537](https://github.com/fluxcd/helm-controller/pull/537)
- Update dependencies
  [#538](https://github.com/fluxcd/helm-controller/pull/538)

## 0.24.0

**Release date:** 2022-09-12

This prerelease comes with improvements to fuzzing.
In addition, the controller dependencies have been updated
to Kubernetes controller-runtime v0.12.

:warning: **Breaking change:** The controller logs have been aligned
with the Kubernetes structured logging. For more details on the new logging
structure please see: [fluxcd/flux2#3051](https://github.com/fluxcd/flux2/issues/3051).

Improvements:
- Align controller logs to Kubernetes structured logging
  [#528](https://github.com/fluxcd/helm-controller/pull/528)
- fuzz: Fix upstream build and optimise execution
  [#529](https://github.com/fluxcd/helm-controller/pull/529)

## 0.23.1

**Release date:** 2022-08-29

This prerelease comes with updates to the controller dependencies
including Kubernetes v1.25.0, Helm v3.9.4 and Kustomize v4.5.7.

Improvements:
- Update Kubernetes packages to v1.25.0
  [#524](https://github.com/fluxcd/helm-controller/pull/524)

## 0.23.0

**Release date:** 2022-08-19

This prerelease comes with panic recovery, to protect the controller
from crashing when reconciliations lead to a crash.

The API now enforces validation to `Spec.ValuesFrom` subfields:
`TargetPath` and `ValuesKey`.

The new image contains updates to patch alpine CVEs.

Improvements:
- Enable RecoverPanic option on reconciler 
  [#516](https://github.com/fluxcd/helm-controller/pull/516)
- Add validation to TargetPath and ValuesKey 
  [#520](https://github.com/fluxcd/helm-controller/pull/520)
- Update dependencies 
  [#521](https://github.com/fluxcd/helm-controller/pull/521)

## 0.22.2

**Release date:** 2022-07-13

This prerelease updates dependencies to patch upstream CVEs.

Improvements:
- Fix github.com/emicklei/go-restful (CVE-2022-1996)
  [#507](https://github.com/fluxcd/helm-controller/pull/507)
- Update dependencies
  [#501](https://github.com/fluxcd/helm-controller/pull/501)
- Update gopkg.in/yaml.v3 to v3.0.1
  [#502](https://github.com/fluxcd/helm-controller/pull/502)
- build: Upgrade to Go 1.18
  [#505](https://github.com/fluxcd/helm-controller/pull/505)

## 0.22.1

**Release date:** 2022-06-07

This prerelease fixes a regression bug introduced in [#480](https://github.com/fluxcd/helm-controller/pull/480)
which would cause the impersonation config to pick the incorrect
`TargetNamespace` for the account name if it was set.

In addition, Kubernetes dependencies have been updated to `v0.24.1`, and
`github.com/containerd/containerd` was updated to `v1.6.6` to mitigate
CVE-2022-31030.

Fixes:
- kube: configure proper account impersonation NS
  [#492](https://github.com/fluxcd/helm-controller/pull/492)
  [#494](https://github.com/fluxcd/helm-controller/pull/494)

Improvements:
- Update dependencies
  [#493](https://github.com/fluxcd/helm-controller/pull/493)

## 0.22.0

**Release date:** 2022-06-01

This prerelease fixes an issue where the token used for Helm operations would
go stale if it was provided using a Bound Service Account Token Volume.

Starting with this version, the controller conforms to the Kubernetes
[API Priority and Fairness](https://kubernetes.io/docs/concepts/cluster-administration/flow-control/).
The controller detects if the server-side throttling is enabled and uses the
advertised rate limits. When server-side throttling is enabled, the controller
ignores the `--kube-api-qps` and `--kube-api-burst` flags.

Fixes:
- kube: load KubeConfig (token) from FS on every reconcile
  [#480](https://github.com/fluxcd/helm-controller/pull/480)
- Updating API group name to helm.toolkit.fluxcd.io in docs
  [#484](https://github.com/fluxcd/helm-controller/pull/484)

Improvements:
- Update dependencies
  [#482](https://github.com/fluxcd/helm-controller/pull/482)
- Update source-controller to v0.25.0
  [#490](https://github.com/fluxcd/helm-controller/pull/490)

## 0.21.0

**Release date:** 2022-05-03

This prerelease introduces support for defining a KubeConfig Secret data key in
the `.spec.kubeConfig.SecretRef.key` (default: `value` or `value.yaml`).

In addition, dependencies have been updated.

Improvements:
- Support defining a KubeConfig Secret data key
  [#461](https://github.com/fluxcd/helm-controller/pull/461)
  [#472](https://github.com/fluxcd/helm-controller/pull/472)
  [#474](https://github.com/fluxcd/helm-controller/pull/474)
- Update dependencies
  [#470](https://github.com/fluxcd/helm-controller/pull/470)
  [#473](https://github.com/fluxcd/helm-controller/pull/473)

## 0.20.1

**Release date:** 2022-04-20

This prerelease equals to [`v0.20.0`](#0200), but is tagged at the right
revision.

## 0.20.0

**Release date:** 2022-04-19

This prerelease adds support for configuring the exponential back-off retry
using newly introduced flags: `--min-retry-delay` (default: `750ms`) and
`--max-retry-delay` (default: `15min`). Previously the defaults were set to
`5ms` and `1000s`.

In addition, all dependencies have been updated to their latest versions,
including an update of Helm to `v3.8.2`.

Improvements:
- Add flags for exponential back-off retry
  [#465](https://github.com/fluxcd/helm-controller/pull/465)
- Update Helm to v3.8.2
  [#466](https://github.com/fluxcd/helm-controller/pull/466)
- Update dependencies
  [#467](https://github.com/fluxcd/helm-controller/pull/467)

## 0.19.0

**Release date:** 2022-04-05

This prerelease adds some breaking changes around the use and handling of kubeconfigs 
files for remote reconciliations. It updates documentation and progress other 
housekeeping tasks.

**Breaking changes**:

- Use of file-based KubeConfig options are now permanently disabled (e.g. 
`TLSClientConfig.CAFile`, `TLSClientConfig.KeyFile`, `TLSClientConfig.CertFile`
and `BearerTokenFile`). The drive behind the change was to discourage
insecure practices of mounting Kubernetes tokens inside the controller's container file system.
- Use of `TLSClientConfig.Insecure` in KubeConfig file is disabled by default,
but can enabled at controller level with the flag `--insecure-kubeconfig-tls`.
- Use of `ExecProvider` in KubeConfig file is now disabled by default,
but can enabled at controller level with the flag `--insecure-kubeconfig-exec`.

Improvements:
- Update KubeConfig documentation
  [#457](https://github.com/fluxcd/helm-controller/pull/457)
- Update docs links to toolkit.fluxcd.io
  [#456](https://github.com/fluxcd/helm-controller/pull/456)
- Add kubeconfig flags
  [#455](https://github.com/fluxcd/helm-controller/pull/455)
- Align version of dependencies when Fuzzing
  [#452](https://github.com/fluxcd/helm-controller/pull/452)

## 0.18.2

**Release date:** 2022-03-25

This prerelease updates the source-controller and Kustomize dependencies to
their latest versions.

Improvements:
- Update Kustomize to v4.5.3
  [#449](https://github.com/fluxcd/helm-controller/pull/449)
- Update source-controller API to v0.22.3
  [#450](https://github.com/fluxcd/helm-controller/pull/450)

## 0.18.1

**Release date:** 2022-03-23

This prerelease ensures the API objects fully adhere to newly introduced
interfaces, allowing them to work in combination with e.g. the
[`conditions`](https://pkg.go.dev/github.com/fluxcd/pkg/runtime@v0.13.2/conditions)
package.

In addition, it ensures (Kubernetes) Event annotations are prefixed with the
FQDN of the Helm API Group. For example, `revision` is now
`helm.toolkit.fluxcd.io/revision`.

This to facilitate improvements to the notification-controller, where
annotations prefixed with the FQDN of the Group of the Involved Object will be
transformed into "fields".

Improvements:
- Implement `meta.ObjectWithConditions` interfaces
  [#444](https://github.com/fluxcd/helm-controller/pull/444)
- Update source-controller API to v0.22.1
  [#445](https://github.com/fluxcd/helm-controller/pull/445)

Fixes:
- Prefix revision annotation with API Group FQDN
  [#447](https://github.com/fluxcd/helm-controller/pull/447)

## 0.18.0

**Release date:** 2022-03-21

This prerelease adds support to the Helm post renderer for Kustomize patches
capable of targeting objects based on kind, label and annotation selectors
using `.spec.postRenderers[].kustomize.patches`.

In addition, various dependencies where updated to their latest versions, and
the code base was refactored to align with the `fluxcd/pkg/runtime` v0.13
release.

The source-controller dependency was updated to version `v0.22` which 
introduces API `v1beta2` and deprecates `v1beta1`.

Improvements:
- Update `pkg/runtime` and `apis/meta`
  [#421](https://github.com/fluxcd/helm-controller/pull/421)
- api: Move Status in CRD printcolumn to the end
  [#425](https://github.com/fluxcd/helm-controller/pull/425)
- Support targeted Patches in the PostRenderer specification
  [#432](https://github.com/fluxcd/helm-controller/pull/432)
- Update dependencies
  [#440](https://github.com/fluxcd/helm-controller/pull/440)
  [#441](https://github.com/fluxcd/helm-controller/pull/441)

## 0.17.2

**Release date:** 2022-03-15

This prerelease comes with an update for `github.com/containerd/containerd` to
`v1.5.10` to please static security analysers and fix any warnings for
CVE-2022-23648.

In addition, it updates Helm from a forked and patched `v3.8.0`, to the
official `v3.8.1` release, and updates minor dependencies.

The Deployment manifest contains a patch to set the
`.spec.securityContext.fsGroup`, which may be required for some EKS setups
as reported in https://github.com/fluxcd/flux2/issues/2537.

Improvements:
- Update Helm to v3.8.1
  [#434](https://github.com/fluxcd/helm-controller/pull/434)
- add fsgroup for securityContext
  [#435](https://github.com/fluxcd/helm-controller/pull/435)
- Update containerd to v1.5.10 and tidy go.mod
  [#436](https://github.com/fluxcd/helm-controller/pull/436)

## 0.17.1

**Release date:** 2022-02-22

This prerelease ensures the QPS and Burst configuration is properly propagated
to the Kubernetes client used by Helm actions, and updates multiple
dependencies to pull in CVE fixes.

Improvements:
- Update dependencies
  [#420](https://github.com/fluxcd/helm-controller/pull/420)
- Set QPS and Burst when impersonating service account
  [#422](https://github.com/fluxcd/helm-controller/pull/422)

## 0.17.0

**Release date:** 2022-02-16

This prerelease introduces a breaking change to the Helm uninstall behavior, as
the `--wait` flag is now enabled by default. Resulting in Helm to wait for
resources to be deleted while uninstalling a release. Disabling this behavior
is possible by declaring `spec.uninstall.disableWait: true` in a `HelmRelease`.

Improvements:
- Add uninstall disableWait flag
  [#416](https://github.com/fluxcd/helm-controller/pull/416)

## 0.16.0

**Release date:** 2022-02-01

This prerelease comes with security improvements for multi-tenant clusters:
- Platform admins can enforce impersonation across the cluster using the `--default-service-account` flag.
  When the flag is set, all `HelmReleases`, which don't have `spec.serviceAccountName` specified,
  use the service account name provided by `--default-service-account=<SA Name>` in the namespace of the object.
- Platform admins can disable cross-namespace references with the `--no-cross-namespace-refs=true` flag.
  When this flag is set, `HelmReleases` can only refer to sources (`HelmRepositories`, `GitRepositories` and `Buckets`)
  in the same namespace as the `HelmRelease` object, preventing tenants from accessing another tenant's repositories.

In addition, the controller comes with a temporary fork of Helm v3.8.0 with a patch applied from
[helm/pull/10486](https://github.com/helm/helm/pull/10486) to solve a memory leak.

The controller container images are signed with
[Cosign and GitHub OIDC](https://github.com/sigstore/cosign/blob/22007e56aee419ae361c9f021869a30e9ae7be03/KEYLESS.md),
and a Software Bill of Materials in [SPDX format](https://spdx.dev) has been published on the release page.

Starting with this version, the controller deployment conforms to the
Kubernetes [restricted pod security standard](https://kubernetes.io/docs/concepts/security/pod-security-standards/#restricted):
- all Linux capabilities were dropped
- the root filesystem was set to read-only
- the seccomp profile was set to the runtime default
- run as non-root was enabled
- the user and group ID was set to 65534

**Breaking changes**:
- The use of new seccomp API requires Kubernetes 1.19.
- The controller container is now executed under 65534:65534 (userid:groupid).
  This change may break deployments that hard-coded the user ID of 'controller' in their PodSecurityPolicy.
- When both `spec.kubeConfig` and `spec.ServiceAccountName` are specified, the controller will impersonate
  the service account on the target cluster, previously the controller ignored the service account.

Features:
- Allow setting a default service account for impersonation
  [#406](https://github.com/fluxcd/helm-controller/pull/406)
- Allow disabling cross-namespace references
  [#408](https://github.com/fluxcd/helm-controller/pull/408)

Improvements:
- Update Helm to patched 3.8.0
  [#409](https://github.com/fluxcd/helm-controller/pull/409)
- Publish SBOM and sign release artifacts
  [#401](https://github.com/fluxcd/helm-controller/pull/401)
- Drop capabilities, set userid and enable seccomp
  [#385](https://github.com/fluxcd/helm-controller/pull/385)
- Update development documentation
  [#397](https://github.com/fluxcd/helm-controller/pull/397)
- Refactor Fuzz implementation
  [#396](https://github.com/fluxcd/helm-controller/pull/396)

Fixes:
- Use patch instead of update when adding finalizers
  [#395](https://github.com/fluxcd/helm-controller/pull/395)
- Fix the missing protocol for the first port in manager config
  [#405](https://github.com/fluxcd/helm-controller/pull/405)
- Use go-install-tool for gen-crd-api-reference-docs
  [#392](https://github.com/fluxcd/helm-controller/pull/392)
- Use go install instead of go get in Makefile
  [#391](https://github.com/fluxcd/helm-controller/pull/391)

## 0.15.0

**Release date:** 2022-01-10

This prerelease comes with an update to the Kubernetes and controller-runtime dependencies
to align them with the Kubernetes 1.23 release, including an update of Helm to `v3.7.2`.

In addition, the controller is now built with Go 1.17 and Alpine 3.15.

Improvements:
- Update Go to v1.17
  [#348](https://github.com/fluxcd/helm-controller/pull/348)
- Update Helm to v3.7.2
  [#380](https://github.com/fluxcd/helm-controller/pull/380)

Fixes:
- Fix inconsistent code-style raised at security audit
  [#386](https://github.com/fluxcd/helm-controller/pull/386)

## 0.14.1

**Release date:** 2021-12-09

This prerelease updates the dependency on the source-controller to `v0.19.2`,
which includes the fixes from source-controller `v0.19.1`, and changes the
length of the SHA hex added to the SemVer metadata of a `HelmChart`. Refer to
the source-controller [changelog](https://github.com/fluxcd/source-controller/blob/main/CHANGELOG.md#0191)
for more information.

:warning: There have been additional user reports about charts complaining
about a `+` character in the label:

```
metadata.labels: Invalid value: "1.2.3+a4303ff0f6fb560ea032f9981c6bd7c7f146d083.1": a valid label must be an empty string or consist of alphanumeric characters, '-', '_' or '.', and must start and end with an alphanumeric character (e.g. 'MyValue', or 'my_value', or '12345', regex used for validation is '(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])?')
```

Given the [Helm chart best practices mention to replace this character with a
`_`](https://helm.sh/docs/chart_best_practices/conventions/#version-numbers),
we encourage you to patch this in your (upstream) chart.
Pseudo example using [template functions](https://helm.sh/docs/chart_template_guide/function_list/):

```yaml
{{- replace "+" "_" .Chart.Version | trunc 63 }}
```

In addition, the dependency on `github.com/opencontainers/runc` is updated to
`v1.0.3` to please static security analysers and fix any warnings for
CVE-2021-43784.

Improvements:
- Update kustomize packages to Kustomize v4.4.1
  [#374](https://github.com/fluxcd/helm-controller/pull/374)
- Update dependencies (fix CVE-2021-43784)
  [#376](https://github.com/fluxcd/helm-controller/pull/376)
- Update source-controller to v0.19.2
  [#377](https://github.com/fluxcd/helm-controller/pull/377)

Fixes:
- docs/spec: Fix reconcile annotation key in example
  [#371](https://github.com/fluxcd/helm-controller/pull/371)

## 0.14.0

**Release date:** 2021-11-23

This prerelease updates the dependency on the source-controller to `v0.19.0`, which
includes **breaking behavioral changes**, including one beneficial to users making
use of `ValuesFiles` references. Refer to the [changelog](https://github.com/fluxcd/source-controller/blob/v0.19.0/CHANGELOG.md#0190)
for more information.

In addition, it updates Alpine to `v3.14`, and several dependencies to their latest
version. Solving an issue with `rest_client_request_latency_seconds_.*` high
cardinality metrics.

Improvements:
- Update Alpine to v3.14
  [#360](https://github.com/fluxcd/helm-controller/pull/360)
- Update various dependencies to mitigate CVE warnings
  [#361](https://github.com/fluxcd/helm-controller/pull/361)
- Update opencontainers/image-spec to v1.0.2
  [#362](https://github.com/fluxcd/helm-controller/pull/362)
- Update controller-runtime to v0.10.2
  [#363](https://github.com/fluxcd/helm-controller/pull/363)
- Update source-controller to v0.19.0
  [#364](https://github.com/fluxcd/helm-controller/pull/364)

## 0.13.0

**Release date:** 2021-11-12

This prerelease comes with artifact integrity verification.
During the acquisition of an artifact, helm-controller computes its checksum using SHA-2
and verifies that it matches the checksum advertised in the `Status` of the Source.

Improvements:
* Verify artifacts integrity
  [#358](https://github.com/fluxcd/helm-controller/pull/358)

## 0.12.2

**Release date:** 2021-11-11

This prerelease downgrades Helm to `v3.6.3` due to high memory usage issues
inherited from upstream dependency changes. For technical details about the
issue, see [this comment](https://github.com/fluxcd/helm-controller/issues/345#issuecomment-959104091).

As Helm `v3.7.0` did not introduce any new features from the perspective of
the controller, we consider this to be a patch which reverts the unwanted
behavior introduced in `v0.12.0`.

Fixes:
* Set the managed fields owner to helm-controller
  [#346](https://github.com/fluxcd/helm-controller/pull/346)
* Downgrade Helm to v3.6.3 due to OOM issues
  [#352](https://github.com/fluxcd/helm-controller/pull/352)
* Replace containerd with version that patches CVEs
  [#356](https://github.com/fluxcd/helm-controller/pull/356)

## 0.12.1

**Release date:** 2021-10-14

This prerelease updates Helm to `v3.7.1`, and ensures `ReconcileStrategy`
changes are applied to the underlying `HelmChart` of a `HelmRelease`.

Improvements:
* Update Helm to v3.7.1
  [#336](https://github.com/fluxcd/helm-controller/pull/336)
* Nit: update tests to use non-deprecated ValuesFiles
  [#334](https://github.com/fluxcd/helm-controller/pull/334)

Fixes:
* Update the release if ReconcileStrategy changes
  [#333](https://github.com/fluxcd/helm-controller/pull/333)

## 0.12.0

**Release date:** 2021-10-08

This prerelease updates Helm to `v3.7.0`, this Helm version should include
improvements to the locking mechanism of releases, which should result in
a reduction of deadlocked releases that have been reported in the past.

In addition, it is now possible to define a `ReconcileStrategy` in the
`HelmChartTemplateSpec`. By setting the value of this field to `Revision`,
a new artifact will be made available for charts from `Bucket` and
`GitRepository` sources whenever a new revision is available.
The default value of the field is `ChartVersion`, which looks for version
changes in the `Chart.yaml` file.

Improvements:
* Update fluxcd/source-controller to v0.16.0
  [#329](https://github.com/fluxcd/helm-controller/pull/329)
* Introduce ReconcileStrategy in HelmChartTemplateSpec
  [#329](https://github.com/fluxcd/helm-controller/pull/329)
* Update sigs.k8s.io/kustomize/api to v0.10.0
  [#330](https://github.com/fluxcd/helm-controller/pull/330)
* Update Helm to v3.7.0
  [#330](https://github.com/fluxcd/helm-controller/pull/330)

Fixes:
* Fix indentation for PostRenderers example
  [#314](https://github.com/fluxcd/helm-controller/pull/314)

## 0.11.2

**Release date:** 2021-08-05

This prerelease comes with support for SOPS encrypted kubeconfig loaded from the
value of the `value.yaml` key in the object, and ensures quoted values are treated
as strings when a `targetPath` is set for a `valuesFrom` item.

To enhance the experience of consumers observing the `HelmRelease` object using
`kstatus`, a default of `-1` is now configured for the `observedGeneration` to
ensure it does not report a false positive in the time the controller has not
marked the resource with a `Ready` condition yet.

In addition, it updates Helm to `v3.6.3` and aligns the Kubernetes dependencies
with `v1.21.3`.

Improvements:
* Set default observedGeneration to -1 on HelmReleases
  [#294](https://github.com/fluxcd/helm-controller/pull/294)
* Treat quoted values as string when targetPath is set
  [#298](https://github.com/fluxcd/helm-controller/pull/298)
* Make the kubeconfig secrets compatible with SOPS
  [#305](https://github.com/fluxcd/helm-controller/pull/306)
* Update dependencies
  [#307](https://github.com/fluxcd/helm-controller/pull/306)

Fixes:
* Remove old util ObjectKey
  [#305](https://github.com/fluxcd/helm-controller/pull/305)

## 0.11.1

**Release date:** 2021-06-18

This prerelease updates Helm to `v3.6.1`, this is a security update which has
no impact as transport is handled by the source-controller. For more details
please see the [source-controller `v0.15.1`
changelog](https://github.com/fluxcd/source-controller/blob/v0.15.1/CHANGELOG.md).

Improvements:
* Update Helm and source-controller
  [#277](https://github.com/fluxcd/helm-controller/pull/277)
* Panic on non-nil AddToScheme errors in main init
  [#278](https://github.com/fluxcd/helm-controller/pull/278)

## 0.11.0

**Release date:** 2021-06-09

This prerelease comes with an update to the Kubernetes and controller-runtime
dependencies to align them with the Kubernetes 1.21 release, including an update
of Helm to `v3.6.0`.

It introduces breaking changes to the Helm behavior as the `--wait-for-jobs`
flag that was introduced in Helm `v3.5.0` is now enabled by default. Disabling
this behavior is possible by declaring `spec.<install|upgrade|rollback>.disableWaitForJobs: true`
in a `HelmRelease`.

Improvements:
* Add support for Helm `--wait-for-jobs` flag
  [#271](https://github.com/fluxcd/helm-controller/pull/271)
* Update dependencies
  [#273](https://github.com/fluxcd/helm-controller/pull/273)
* Add nightly builds workflow and allow RC releases
  [#274](https://github.com/fluxcd/helm-controller/pull/274)

Fixes:
* Fix HelmChartTemplateSpec Doc missing valuesFiles info
  [#266](https://github.com/fluxcd/helm-controller/pull/266)

## 0.10.1

**Release date:** 2021-05-10

This prerelease fixes a bug where an `skipCRDs is set to false and
crds is set to Skip` error would be thrown if the deprecated `skipCRDs`
field was omitted by giving the CRD policy field precedence over the
deprecated one.

Improvements:
* Update source-controller dependencies to v0.12.2
  [#262](https://github.com/fluxcd/helm-controller/pull/262)

Fixes:
* Give CRD policy precedence over skipCRDs field
  [#261](https://github.com/fluxcd/helm-controller/pull/261)

## 0.10.0

**Release date:** 2021-04-21

This prerelease introduces support for defining a `CRDsPolicy` in the
`HelmReleaseSpec`, `Install` and `Upgrade` objects, while deprecating
the `SkipCRDs` fields.

Supported policies:

* `Skip`: Do neither install nor replace (update) any CRDs.
* `Create`: New CRDs are created, existing CRDs are neither updated nor
  deleted.
* `CreateReplace`: New CRDs are created, existing CRDs are updated
  (replaced) but not deleted.
  
In case `CreateReplace` is used as an `Upgrade` policy, Custom Resource
Definitions are applied by the controller before a Helm upgrade is
performed. On rollbacks, the Custom Resource Definitions are left
untouched and **not** rolled back.

The `ValuesFile` field in the `HelmChart` template has been deprecated
in favour of the new `ValuesFiles` field.

Features:
* Initial support for CRDs (upgrade) policies
  [#250](https://github.com/fluxcd/helm-controller/pull/250)
  [#254](https://github.com/fluxcd/helm-controller/pull/254)
* Add `ValuesFiles` to `HelmChart` spec
  [#252](https://github.com/fluxcd/helm-controller/pull/252)

Improvements:
* Update Helm to v3.5.4
  [#253](https://github.com/fluxcd/helm-controller/pull/253)
* Update dependencies
  [#251](https://github.com/fluxcd/helm-controller/pull/251)
  [#253](https://github.com/fluxcd/helm-controller/pull/253)

Fixes:
* docs: minor `createNamespace` placement fix
  [#246](https://github.com/fluxcd/helm-controller/pull/246)

## 0.9.0

**Release date:** 2021-03-26

This prerelease comes with a breaking change to the leader election ID
from `5b6ca942.fluxcd.io` to `helm-controller-leader-election`
to be more descriptive. This change should not have an impact on most
installations, as the default replica count is `1`. If you are running
a setup with multiple replicas, it is however advised to scale down
before  upgrading.

To ease debugging wait timeout errors, the last 5 deduplicated log lines
from Helm are now recorded in the status conditions and events of the
`HelmRelease`.

To track the origin of resources that are created by a Helm operation
performed by the controllers, they are now labeled with
`helm.toolkit.fluxcd.io/name` and `helm.toolkit.fluxcd.io/namespace`
using a builtin post render.

The suspended status of resources is now recorded to a
`gotk_suspend_status` Prometheus gauge metric.

Improvements:
* Capture and expose debug (log) information on release failure
  [#219](https://github.com/fluxcd/helm-controller/pull/219)
* Record suspension metrics
  [#236](https://github.com/fluxcd/helm-controller/pull/236)
* Label release resources with HelmRelease origin
  [#238](https://github.com/fluxcd/helm-controller/pull/238)
* Set leader election deadline to 30s
  [#239](https://github.com/fluxcd/helm-controller/pull/239)
* Update source-controller API to v0.10.0
  [#240](https://github.com/fluxcd/helm-controller/pull/240)

## 0.8.2

**Release date:** 2021-03-15

This prerelease comes with patch updates to Helm and controller-runtime
dependencies.

Improvements:
* Update dependencies
  [#232](https://github.com/fluxcd/helm-controller/pull/232)

## 0.8.1

**Release date:** 2021-03-05

This prerelease comes with improvements to the notification system.
The controller retries with exponential backoff when fetching artifacts,
preventing spamming events when source-controller becomes
unavailable for a short period of time.

Improvements:
* Retry with exponential backoff when fetching artifacts
  [#216](https://github.com/fluxcd/helm-controller/pull/216)

Fixes:
* fix: log messages contain '%s'
  [#229](https://github.com/fluxcd/helm-controller/pull/229)

## 0.8.0

**Release date:** 2021-02-24

This is the eight MINOR prerelease.

Due to changes in Helm [v3.5.2](https://github.com/helm/helm/releases/tag/v3.5.2),
charts not versioned using **strict semver** are no longer compatible with
source-controller (and the embedded `HelmChart` template in the `HelmRelease`).
When using charts from Git, make sure that the `version`
field is set in `Chart.yaml`.

Improvements:
* Allow the controller to be run locally
  [#216](https://github.com/fluxcd/helm-controller/pull/216)
* Add a release deployment event when reconciling a release
  [#217](https://github.com/fluxcd/helm-controller/pull/217)
* Use `MergeMaps` from pkg/runtime v0.8.2
  [#220](https://github.com/fluxcd/helm-controller/pull/220)
* Refactor release workflow
  [#223](https://github.com/fluxcd/helm-controller/pull/223)
* Update dependencies
  [#225](https://github.com/fluxcd/helm-controller/pull/225)
* Use source-controller manifest from GitHub release
  [#226](https://github.com/fluxcd/helm-controller/pull/226)

## 0.7.0

**Release date:** 2021-02-12

This is the seventh MINOR prerelease.

Support has been added for Kustomize based post renderer, making it possible
to define images, strategic merge and JSON 6902 patches within the
`HelmRelease`.

`pprof` endpoints have been enabled on the metrics server, making it easier to
collect runtime information to for example debug performance issues.

Features:
* Support for Kustomize based PostRenderer
  [#202](https://github.com/fluxcd/helm-controller/pull/202)
  [#205](https://github.com/fluxcd/helm-controller/pull/205)
  [#206](https://github.com/fluxcd/helm-controller/pull/206)

Improvements:
* Update dependencies
  [#207](https://github.com/fluxcd/helm-controller/pull/207)
* Enable pprof endpoints on metrics server
  [#209](https://github.com/fluxcd/helm-controller/pull/209)
* Update Alpine to v3.13
  [#210](https://github.com/fluxcd/helm-controller/pull/210)

## 0.6.1

**Release date:** 2021-01-25

This prerelease adds support for configuring the namespace of the
Helm storage by defining a `StorageNamespace` in the `HelmRelease`
resource (defaults to the namespace of the resource).

## 0.6.0

**Release date:** 2021-01-22

This is the sixth MINOR prerelease.

Two new argument flags are introduced to support configuring the QPS
(`--kube-api-qps`) and burst (`--kube-api-burst`) while communicating
with the Kubernetes API server.

The `LocalObjectReference` from the Kubernetes core has been replaced
with our own, making `Name` a required field. The impact of this should
be limited to direct API consumers only, as the field was already
required by controller logic.

## 0.5.2

**Release date:** 2021-01-18

This prerelease comes with updates to Kubernetes and Helm dependencies.
The Kubernetes packages were updated to `v1.20.2` and Helm to `v3.5.0`.

## 0.5.1

**Release date:** 2021-01-14

This prerelease fixes a regression bug introduced in `v0.5.0` that caused
reconciliation request annotations to be ignored in certain scenarios.

## 0.5.0

**Release date:** 2021-01-12

This is the fifth MINOR prerelease, upgrading the `controller-runtime`
dependencies to `v0.7.0`.

The container image for ARMv7 and ARM64 that used to be published
separately as `helm-controller:*-arm64` has been merged with the AMD64
image.

## 0.4.4

**Release date:** 2020-12-16

This prerelease increases the `terminationGracePeriodSeconds` of the
controller `Deployment` from `10` to `600`, to allow release processes
that make use of the default timeout (`5m0s`) to finish, and upgrades
the source-controller API dependency to `v0.5.5`.

## 0.4.3

**Release date:** 2020-12-10

This prerelease upgrades various dependencies.

* Kubernetes dependency upgrades to `v1.19.4`
* Helm upgrade to `v3.4.2`

## 0.4.2

**Release date:** 2020-12-04

This prerelease fixes a bug in the merging of values.

## 0.4.1

**Release date:** 2020-11-30

This prerelease introduces support for Helm's namespace creation
feature by defining `CreateNamespace` in the `Install` configuration
of the `HelmRelease`. Take note that deleting the `HelmRelease` does
not remove the created namespace, and managing namespaces outside of
the `HelmRelease` is advised.

In addition, it includes a fix for a bug that caused the finalizer to
never be removed if a release no longer existed in the Helm storage.

## 0.4.0

**Release date:** 2020-11-26

This the fourth MINOR prerelease. It adds support for impersonating a
Service Account during Helm actions by defining a `ServiceAccountName`
in the `HelmRelease`, and includes various bug fixes.

## 0.3.0

**Release date:** 2020-11-20

This is the third MINOR prerelease. It introduces a breaking change to
the API package; the status condition type has changed to the type
introduced in Kubernetes API machinery `v1.19.0`.

## 0.2.2

**Release date:** 2020-11-18

This prerelease comes with a bugfix for chart divergence detections.

## 0.2.1

**Release date:** 2020-11-17

This prerelease comes with improvements to status reporting, and a
bugfix for the (temporary) dead lock that would occur on some
transient values composition and chart loading errors.

## 0.2.0

**Release date:** 2020-10-29

This is the second MINOR prerelease, it comes with a breaking change:

* The histogram metric `gotk_reconcile_duration` was renamed to `gotk_reconcile_duration_seconds`

Other notable changes:

* Added support for cross-cluster Helm releases by defining a `KubeConfig`
  reference in the `HelmReleaseSpec`.
* The annotation `fluxcd.io/reconcileAt` was renamed to `reconcile.fluxcd.io/requestedAt`,
  the former will be removed in a next release but is backwards
  compatible for now.

## 0.1.3

**Release date:** 2020-10-16

This prereleases fixes two bugs:

- `HelmRelease` resources with a `spec.valuesFrom` reference making use
  of a `targetPath` defined as the first item will now compose without
  failing.
- The chart reconciliation and readiness logic has been rewritten to
  better work with no-op chart updates and guarantee readiness state
  observation accuracy. This prevents it from `HelmRelease`s getting
  stuck on a "HelmChart is not ready" state.

## 0.1.2

**Release date:** 2020-10-13

This prerelease comes with Prometheus instrumentation for the controller's resources.

For each kind, the controller exposes a gauge metric to track the `Ready` condition status,
and a histogram with the reconciliation duration in seconds:

* `gotk_reconcile_condition{kind, name, namespace, status, type="Ready"}`
* `gotk_reconcile_duration{kind, name, namespace}`

## 0.1.1

**Release date:** 2020-10-02

This prerelease fixes a regression bug introduced in `v0.1.0`
resulting in the `spec.targetNamespace` not being taken into
account.

## 0.1.0

**Release date:** 2020-09-30

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

## 0.0.10

**Release date:** 2020-09-23

This prerelease adds support for Helm charts from `Bucket` sources,
support for optional `ValuesFrom` references, and a Helm upgrade from
`3.3.3` to `3.3.4`.

## 0.0.9

**Release date:** 2020-09-22

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

## 0.0.8

**Release date:** 2020-09-11

This prerelease adds support for defining a `ValuesFile` in the
`HelmChartTemplateSpec` to overwrite the default chart values with another
values file, as supported by `>=0.0.15` of the source-controller, and a
`--watch-all-namespaces` flag (defaults to `true`) to provide the option
to only watch the runtime namespace of the controller for resources.

## 0.0.7

**Release date:** 2020-09-04

This prerelease comes with documentation fixes.
Container images for linux/amd64 and linux/arm64 are published to GHCR.

## 0.0.6

**Release date:** 2020-09-02

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

## 0.0.5

**Release date:** 2020-08-26

This prerelease adds support for conditional remediation on failed Helm
actions, and includes several (breaking) changes to the API:

* The `maxRetries` value should now be set on the respective
  `install.remediation.retries` and `upgrade.remediation.retries` fields.
* The `rollback.enable` field has been removed in favour of
  `upgrade.remediateLastFailure`.
* Failing Helm tests will now result in a `False` `Ready` condition by
  default, ignoring test failures can be re-enabled by configuring
  `test.ignoreFailures` to `true`.

## 0.0.4

**Release date:** 2020-08-20

This prerelease adds support for merging a flat single value from
a `ValueReference` at the defined `TargetPath`, and fixes a bug in
the merging of values where overwrites of a map with a flat single
value was not allowed.

## 0.0.3

**Release date:** 2020-08-18

This prerelease upgrades the `github.com/fluxcd/pkg/*` dependencies to
dedicated versioned modules, and makes the `api` package available as
a dedicated versioned module.

## 0.0.2

**Release date:** 2020-08-12

In this prerelease the Helm package was upgraded to [v3.3.0](https://github.com/helm/helm/releases/tag/v3.3.0).

## 0.0.1

**Release date:** 2020-07-31

This prerelease comes with a breaking change, the CRDs group has been
renamed to `helm.toolkit.fluxcd.io`. The dependency on `source-controller`
has been updated to `v0.0.7` to be able to work with `source.toolkit.fluxcd.io`
resources.

## 0.0.1-beta.4

**Release date:** 2020-07-22

This beta release fixes a bug affecting helm release status reevaluation.

## 0.0.1-beta.3

**Release date:** 2020-07-21

This beta release fixes a bug affecting helm charts reconciliation.

## 0.0.1-beta.2

**Release date:** 2020-07-21

This beta release comes with various bug fixes and minor improvements.

## 0.0.1-beta.1

**Release date:** 2020-07-20

This beta release drops support for Kubernetes <1.16.
The CRDs have been updated to `apiextensions.k8s.io/v1`.

## 0.0.1-alpha.2

**Release date:** 2020-07-16

This alpha release comes with improvements to alerts delivering,
logging, and fixes a bug in the lookup of HelmReleases when a
HelmChart revision had changed.

## 0.0.1-alpha.1

**Release date:** 2020-07-13

This is the first alpha release of helm-controller.
The controller is an implementation of the
[helm.fluxcd.io/v2alpha1](https://github.com/fluxcd/helm-controller/tree/v0.0.1-alpha.1/docs/spec/v2alpha1) API.
