# Changelog

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
