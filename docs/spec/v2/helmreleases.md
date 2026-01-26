# Helm Releases

<!-- menuweight:10 -->

The `HelmRelease` API allows for controller-driven reconciliation of Helm
releases via Helm actions such as install, upgrade, test, uninstall, and
rollback. In addition to this, it detects and corrects cluster state drift
from the desired release state.

## Example

The following is an example of a HelmRelease which installs the
[podinfo Helm chart](https://github.com/stefanprodan/podinfo/tree/master/charts/podinfo).

```yaml
---
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmRepository
metadata:
  name: podinfo
  namespace: default
spec:
  interval: 15m
  url: https://stefanprodan.github.io/podinfo
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: podinfo
  namespace: default
spec:
  interval: 15m
  timeout: 5m
  chart:
    spec:
      chart: podinfo
      version: '6.5.*'
      sourceRef:
        kind: HelmRepository
        name: podinfo
      interval: 5m
  releaseName: podinfo
  install:
    remediation:
      retries: 3
  upgrade:
    remediation:
      retries: 3
  test:
    enable: true
  driftDetection:
    mode: enabled
    ignore:
    - paths: ["/spec/replicas"]
      target:
        kind: Deployment
  values:
    replicaCount: 2
```

In the above example:

- A [HelmRepository](https://fluxcd.io/flux/components/source/helmrepositories/)
  named `podinfo` is created, pointing to the Helm repository from which the 
  podinfo chart can be installed.
- A HelmRelease named `podinfo` is created, that will create a [HelmChart](https://fluxcd.io/flux/components/source/helmcharts/) object
  from [the `.spec.chart`](#chart-template) and watch it for Artifact changes.
- The controller will fetch the chart from the HelmChart's Artifact and use it
  together with the `.spec.releaseName` and `.spec.values` to confirm if the
  Helm release exists and is up-to-date.
- If the Helm release does not exist, is not up-to-date, or has not observed to
  be made by the controller based on [the HelmRelease's history](#history), then
  the controller will install or upgrade the release. If this fails, it is
  allowed to retry the operation a number of times while requeueing between
  attempts, as defined by the respective [remediation configurations](#configuring-failure-handling).
- If the [Helm tests](#test-configuration) for the release have not been run
  before for this release, the HelmRelease will run them.
- When the Helm release in storage is up-to-date, the controller will check if
  the release in the cluster has drifted from the desired state, as defined by
  the [drift detection configuration](#drift-detection). If it has, the
  controller will [correct the drift](#drift-correction) by re-applying the
  desired state.
- The controller will repeat the above steps at the interval defined by
  `.spec.interval`, or when the configuration changes in a way that affects the
  desired state of the Helm release (e.g. a new chart version or values).

You can run this example by saving the manifest into `podinfo.yaml`.

1. Apply the resource on the cluster:

   ```sh
   kubectl apply -f podinfo.yaml
   ```

2. Run `kubectl get helmrelease` to see the HelmRelease:

   ```console
   NAME      AGE   READY   STATUS
   podinfo   15s   True    Helm test succeeded for release default/podinfo.v1 with chart podinfo@6.5.3: 3 test hooks completed successfully
   ```

3. Run `kubectl describe helmrelease podinfo` to see the [Conditions](#conditions)
   and [History](#history) in the HelmRelease's Status:

   ```console
   ...
   Status:
     Conditions:
       Last Transition Time:  2023-12-04T14:17:47Z
       Message:               Helm test succeeded for release default/podinfo.v1 with chart podinfo@6.5.3: 3 test hooks completed successfully
       Observed Generation:   1
       Reason:                TestSucceeded
       Status:                True
       Type:                  Ready
       Last Transition Time:  2023-12-04T14:17:39Z
       Message:               Helm install succeeded for release default/podinfo.v1 with chart podinfo@6.5.3
       Observed Generation:   1
       Reason:                InstallSucceeded
       Status:                True
       Type:                  Released
       Last Transition Time:  2023-12-04T14:17:47Z
       Message:               Helm test succeeded for release default/podinfo.v1 with chart podinfo@6.5.3: 3 test hooks completed successfully
       Observed Generation:   1
       Reason:                TestSucceeded
       Status:                True
       Type:                  TestSuccess
     Helm Chart:              default/default-podinfo
     History:
       Chart Name:      podinfo
       Chart Version:   6.5.3
       Config Digest:   sha256:e15c415d62760896bd8bec192a44c5716dc224db9e0fc609b9ac14718f8f9e56
       Digest:          sha256:e59aeb8b854f42e44756c2ef552a073051f1fc4f90e68aacbae7f824139580bc
       First Deployed:  2023-12-04T14:17:35Z
       Last Deployed:   2023-12-04T14:17:35Z
       Name:            podinfo
       Namespace:       default
       Status:          deployed
       Test Hooks:
         Podinfo - Grpc - Test - Scyhk:
           Last Completed:  2023-12-04T14:17:42Z
           Last Started:    2023-12-04T14:17:39Z
           Phase:           Succeeded
         Podinfo - Jwt - Test - Scddu:
           Last Completed:  2023-12-04T14:17:45Z
           Last Started:    2023-12-04T14:17:42Z
           Phase:           Succeeded
         Podinfo - Service - Test - Uibss:
           Last Completed:           2023-12-04T14:17:47Z
           Last Started:             2023-12-04T14:17:45Z
           Phase:                    Succeeded
       Version:                      1
     Last Applied Revision:          6.5.3
     Last Attempted Config Digest:   sha256:e15c415d62760896bd8bec192a44c5716dc224db9e0fc609b9ac14718f8f9e56
     Last Attempted Generation:      1
     Last Attempted Release Action:  install
     Last Attempted Revision:        6.5.3
     Observed Generation:            1
     Storage Namespace:              default
   Events:
     Type    Reason            Age   From             Message
     ----    ------            ----  ----             -------
     Normal  HelmChartCreated  23s   helm-controller  Created HelmChart/default/default-podinfo with SourceRef 'HelmRepository/default/podinfo'
     Normal  HelmChartInSync   22s   helm-controller  HelmChart/default/default-podinfo with SourceRef 'HelmRepository/default/podinfo' is in-sync
     Normal  InstallSucceeded  18s   helm-controller  Helm install succeeded for release default/podinfo.v1 with chart podinfo@6.5.3
     Normal  TestSucceeded     10s   helm-controller  Helm test succeeded for release default/podinfo.v1 with chart podinfo@6.5.3: 3 test hooks completed successfully
   ```

## Writing a HelmRelease spec

As with all other Kubernetes config, a HelmRelease needs `apiVersion`,
`kind`, and `metadata` fields. The name of a HelmRelease object must be a
valid [DNS subdomain name](https://kubernetes.io/docs/concepts/overview/working-with-objects/names#dns-subdomain-names).

A HelmRelease also needs a
[`.spec` section](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#spec-and-status).

### Chart template

`.spec.chart` is an optional field used by the helm-controller as a template to
create a new [HelmChart resource](https://fluxcd.io/flux/components/source/helmcharts/).

The spec for the HelmChart is provided via `.spec.chart.spec`, refer to
[writing a HelmChart spec](https://fluxcd.io/flux/components/source/helmcharts/#writing-a-helmchart-spec)
for in-depth information.

Annotations and labels can be added by configuring the respective
`.spec.chart.metadata` fields.

The HelmChart is created in the same namespace as the `.sourceRef`, with a name
matching the HelmRelease's `<.metadata.namespace>-<.metadata.name>`, and will
be reported in `.status.helmChart`.

The chart version of the last release attempt is reported in
`.status.lastAttemptedRevision`. The controller will automatically perform a
Helm release when the HelmChart produces a new chart (version).

**Warning:** Changing the `.spec.chart` to a Helm chart with a different name
(as specified in the chart's `Chart.yaml`) will cause the controller to
uninstall any previous release before installing the new one.

**Note:** On multi-tenant clusters, platform admins can disable cross-namespace
references with the `--no-cross-namespace-refs=true` flag. When this flag is
set, the HelmRelease can only refer to Sources in the same namespace as the
HelmRelease object.

### Chart reference

`.spec.chartRef` is an optional field used to refer to the Source object which has an
Artifact containing the Helm chart. It has two required fields:

- `kind`: The Kind of the referred Source object. Supported Source types:
  + [OCIRepository](https://fluxcd.io/flux/components/source/ocirepositories/)
  + [HelmChart](https://fluxcd.io/flux/components/source/helmcharts/)
  + [ExternalArtifact](https://fluxcd.io/flux/components/source/externalartifacts/) (requires `--feature-gates=ExternalArtifact=true` flag)
- `name`: The Name of the referred Source object.

For a referenced resource of kind `OCIRepository`, the chart version of the last
release attempt is reported in `.status.lastAttemptedRevision`. The version is in
the format `<version>+<digest[0:12]>`. The digest of the OCI artifact is appended
to the version to ensure that a change in the artifact content triggers a new release.
The controller will automatically perform a Helm upgrade when the `OCIRepository`
detects a new digest in the OCI artifact stored in registry, even if the version
inside `Chart.yaml` is unchanged.

**Note:** Disabling the appending of the digest to the chart version can be done
with the `--feature-gates=DisableChartDigestTracking=true` controller flag.

**Warning:** One of `.spec.chart` or `.spec.chartRef` must be set, but not both.
When switching from `.spec.chart` to `.spec.chartRef`, the controller will perform
an Helm upgrade and will garbage collect the old HelmChart object.

**Note:** On multi-tenant clusters, platform admins can disable cross-namespace
references with the `--no-cross-namespace-refs=true` controller flag. When this flag is
set, the HelmRelease can only refer to OCIRepositories in the same namespace as the
HelmRelease object.

#### OCIRepository reference example

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
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
    semver: ">= 6.0.0"
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
    namespace: default
  values:
    replicaCount: 2
```

#### HelmChart reference example

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: HelmChart
metadata:
  name: podinfo
  namespace: default
spec:
  interval: 10m
  chart: podinfo
  sourceRef:
    kind: HelmRepository
    name: podinfo
  version: "6.x"
  valuesFiles:
    - values-prod.yaml
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: podinfo
  namespace: default
spec:
  interval: 10m
  chartRef:
    kind: HelmChart
    name: podinfo
    namespace: default
  values:
    replicaCount: 2
```

### Release name

`.spec.releaseName` is an optional field used to specify the name of the Helm
release. It defaults to a composition of `[<target namespace>-]<name>`.

**Warning:** Changing the release name of a HelmRelease which has already been
installed will not rename the release. Instead, the existing release will be
uninstalled before installing a new release with the new name.

**Note:** When the composition exceeds the maximum length of 53 characters, the
name is shortened by hashing the release name with SHA-256. The resulting name
is then composed of the first 40 characters of the release name, followed by a
dash (`-`), followed by the first 12 characters of the hash. For example,
`a-very-lengthy-target-namespace-with-a-nice-object-name` becomes
`a-very-lengthy-target-namespace-with-a-nic-97af5d7f41f3`.

### Target namespace

`.spec.targetNamespace` is an optional field used to specify the namespace to
which the Helm release is made. It defaults to the namespace of the
HelmRelease.

**Warning:** Changing the target namespace of a HelmRelease which has already
been installed will not move the release to the new namespace. Instead, the
existing release will be uninstalled before installing a new release in the new
target namespace.

### Storage namespace

`.spec.storageNamespace` is an optional field used to specify the namespace
in which Helm stores release information. It defaults to the namespace of the
HelmRelease.

**Warning:** Changing the storage namespace of a HelmRelease which has already
been installed will not move the release to the new namespace. Instead, the
existing release will be uninstalled before installing a new release in the new
storage namespace.

**Note:** When making use of the Helm CLI and attempting to make use of
`helm get` commands to inspect a release, the `-n` flag should target the
storage namespace of the HelmRelease.

### Service Account reference

`.spec.serviceAccountName` is an optional field used to specify the
Service Account to be impersonated while reconciling the HelmRelease.
For more information, refer to [Role-based access control](#role-based-access-control).

### Persistent client

`.spec.persistentClient` is an optional field to instruct the controller to use
a persistent Kubernetes client for this release. If specified, the client will
be reused for the duration of the reconciliation, instead of being created and
destroyed for each (step of a) Helm action. If not set, it defaults to `true.`

**Note:** This method generally boosts performance but could potentially cause
complications with specific Helm charts. For instance, charts creating Custom
Resource Definitions outside Helm's CRD lifecycle hooks during installation
might face issues where these resources are not recognized as available,
especially by post-install hooks.

### Max history

`.spec.maxHistory` is an optional field to configure the number of release
revisions saved by Helm. If not set, it defaults to `5`.

**Note:** Although setting this to `0` for an unlimited number of revisions is
permissible, it is advised against due to performance reasons.

### Dependencies

`.spec.dependsOn` is an optional list to refer to other HelmRelease objects
which the HelmRelease depends on. If specified, the HelmRelease is only allowed
to proceed after the referred HelmReleases are ready, i.e. have the `Ready`
condition marked as `True`.

This is helpful when there is a need to make sure other resources exist before
the workloads defined in a HelmRelease are released. For example, before
installing objects of a certain Custom Resource kind, the Custom Resource
Defintions and the related controller must exist in the cluster.

```yaml
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: backend
  namespace: default
spec:
  # ...omitted for brevity   
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: frontend
  namespace: default
spec:
  # ...omitted for brevity
  dependsOn:
    - name: backend
```

**Note:** Circular dependencies between HelmRelease resources must be avoided,
otherwise the interdependent HelmRelease resources will never be reconciled.

#### Dependency Ready Expression

`.spec.dependsOn[].readyExpr` is an optional field that can be used to define a CEL expression
to determine the readiness of a HelmRelease dependency.

This is helpful for when custom logic is needed to determine if a dependency is ready.
For example, when performing a lockstep upgrade, the `readyExpr` can be used to
verify that a dependency has a matching version in values before proceeding with the
reconciliation of the dependent HelmRelease.

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: backend
  namespace: default
spec:
  # ...omitted for brevity
  values: 
    app:
      version: v1.2.3
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: frontend
  namespace: default
spec:
  # ...omitted for brevity
  values:
    app:
      version: v1.2.3
  dependsOn:
    - name: backend
      readyExpr: >
        dep.spec.values.app.version == self.spec.values.app.version &&
        dep.status.conditions.filter(e, e.type == 'Ready').all(e, e.status == 'True') &&
        dep.metadata.generation == dep.status.observedGeneration
```

The CEL expression contains the following variables:

- `dep`: The dependency HelmRelease object being evaluated.
- `self`: The HelmRelease object being reconciled.

**Note:** When `readyExpr` is specified, the built-in readiness check is replaced by the logic
defined in the CEL expression. You can configure the controller to run both the CEL expression
evaluation and the built-in readiness check, with the `AdditiveCELDependencyCheck`
[feature gate](https://fluxcd.io/flux/components/helm/options/#feature-gates).

### Values

The values for the Helm release can be specified in two ways:

- [Values references](#values-references)
- [Inline values](#inline-values)

Changes to the combined values will trigger a new Helm release.

#### Values references

`.spec.valuesFrom` is an optional list to refer to ConfigMap and Secret
resources from which to take values. The values are merged in the order given,
with the later values overwriting earlier, and then [inline values](#inline-values)
overwriting those. When `targetPath` is set, it will overwrite everything before,
including inline values.

An item on the list offers the following subkeys:

- `kind`: Kind of the values referent, supported values are `ConfigMap` and
  `Secret`.
- `name`: The `.metadata.name` of the values referent, in the same namespace as
  the HelmRelease.
- `valuesKey` (Optional): The `.data` key where the values.yaml or a specific
  value can be found. Defaults to `values.yaml` when omitted.
- `targetPath` (Optional): The YAML dot notation path at which the value should
  be merged. When set, the valuesKey is expected to be a single flat value.
  Defaults to empty when omitted, which results in the values getting merged at
  the root.
- `optional` (Optional): Whether this values reference is optional. When
  `true`, a not found error for the values reference is ignored, but any
  `valuesKey`, `targetPath` or transient error will still result in a
  reconciliation failure. Defaults to `false` when omitted.

```yaml
spec:
  valuesFrom:
    - kind: ConfigMap
      name: prod-env-values
      valuesKey: values-prod.yaml
    - kind: Secret
      name: prod-tls-values
      valuesKey: crt
      targetPath: tls.crt
      optional: true
```

**Note:** The `targetPath` supports the same formatting as you would supply as
an argument to the `helm` binary using `--set [path]=[value]`. In addition to
this, the referred value can contain the same value formats (e.g. `{a,b,c}` for
a list). You can read more about the available formats and limitations in the
[Helm documentation](https://helm.sh/docs/intro/using_helm/#the-format-and-limitations-of---set).

For JSON strings, the [limitations are the same as while using `helm`](https://github.com/helm/helm/issues/5618)
and require you to escape the full JSON string (including `=`, `[`, `,`, `.`).

To make a HelmRelease react immediately to changes in the referenced Secret
or ConfigMap see [this](#reacting-immediately-to-configuration-dependencies)
section.

#### Inline values

`.spec.values` is an optional field to inline values within a HelmRelease. When
[values references](#values-references) are defined, inline values are merged
with the values from these references, overwriting any existing ones.

```yaml
spec:
  values:
    replicaCount: 2
```

### Install configuration

`.spec.install` is an optional field to specify the configuration for the
controller to use when running a [Helm install action](https://helm.sh/docs/helm/helm_install/).

The field offers the following subfields:

- `.timeout` (Optional): The time to wait for any individual Kubernetes
  operation (like Jobs for hooks) during the installation of the chart.
  Defaults to the [global timeout value](#timeout).
- `.crds` (Optional): The Custom Resource Definition install policy to use.
  Valid values are `Skip`, `Create` and `CreateReplace`. Default is `Create`,
  which will create Custom Resource Definitions when they do not exist. Refer
  to [Custom Resource Definition lifecycle](#controlling-the-lifecycle-of-custom-resource-definitions)
  for more information.
- `.replace` (Optional): Instructs Helm to re-use the [release name](#release-name),
  but only if that name is a deleted release which remains in the history.
  Defaults to `false`.
- `.createNamespace` (Optional): Instructs Helm to create the [target namespace](#target-namespace)
  if it does not exist. On uninstall, the created namespace will not be garbage
  collected. Defaults to `false`.
- `.disableHooks` (Optional): Prevents [chart hooks](https://helm.sh/docs/topics/charts_hooks/)
  from running during the installation of the chart. Defaults to `false`.
- `.disableOpenAPIValidation` (Optional): Prevents Helm from validating the
  rendered templates against the Kubernetes OpenAPI Schema. Defaults to `false`.
- `.disableSchemaValidation` (Optional): Prevents Helm from validating the
  values against the JSON Schema. Defaults to `false`.
- `.disableTakeOwnership` (Optional): Disables taking ownership of existing resources
  during the Helm install action. Defaults to `false`.
- `.disableWait` (Optional): Disables waiting for resources to be ready after
  the installation of the chart. Defaults to `false`.
- `.disableWaitForJobs` (Optional): Disables waiting for any Jobs to complete
  after the installation of the chart. Defaults to `false`.
- `.serverSideApply` (Optional): Enables Server-Side Apply for resources during
  the installation. When `true`, the controller uses Kubernetes Server-Side
  Apply which provides better conflict detection and field ownership tracking.
  Defaults to `true` (or `false` when the `UseHelm3Defaults` feature gate is
  enabled).

#### Install strategy

`.spec.install.strategy` is an optional field to specify the strategy
to use when running a Helm install action.

The field offers the following subfields:

- `.name` (Required): The name of the install strategy to use. One of
  `RemediateOnFailure` or `RetryOnFailure`.
  If the `.spec.install.strategy` field is not specified, the HelmRelease
  reconciliation behaves as if `.spec.install.strategy.name` was set to
  `RemediateOnFailure`.
- `.retryInterval` (Optional): The time to wait between retries of failed
  releases when the install strategy is set to `RetryOnFailure`. Defaults
  to `5m`. Cannot be used with `RemediateOnFailure`.

The default `RemediateOnFailure` strategy applies the rules defined by the
`.spec.install.remediation` field to the install action, i.e. the same
behavior of the controller prior to the introduction of the `RetryOnFailure`
strategy.

The `RetryOnFailure` strategy will retry a failed install with an upgrade
after the interval defined by the `.spec.install.strategy.retryInterval`
field.

#### Install remediation

`.spec.install.remediation` is an optional field to configure the remediation
strategy to use when the installation of a Helm chart fails.

The field offers the following subfields:

- `.retries` (Optional): The number of retries that should be attempted on
  failures before bailing. Remediation, using an [uninstall](#uninstall-configuration),
  is performed between each attempt. Defaults to `0`, a negative integer equals
  to an infinite number of retries.
- `.ignoreTestFailures` (Optional): Instructs the controller to not remediate
  when a [Helm test](#test-configuration) failure occurs. Defaults to
  `.spec.test.ignoreFailures`.
- `.remediateLastFailure` (Optional): Instructs the controller to remediate the
  last failure when no retries remain. Defaults to `false`.

### Upgrade configuration

`.spec.upgrade` is an optional field to specify the configuration for the
controller to use when running a [Helm upgrade action](https://helm.sh/docs/helm/helm_upgrade/).

The field offers the following subfields:

- `.timeout` (Optional): The time to wait for any individual Kubernetes
  operation (like Jobs for hooks) during the upgrade of the release.
  Defaults to the [global timeout value](#timeout).
- `.crds` (Optional): The Custom Resource Definition upgrade policy to use.
  Valid values are `Skip`, `Create` and `CreateReplace`. Default is `Skip`.
  Refer to [Custom Resource Definition lifecycle](#controlling-the-lifecycle-of-custom-resource-definitions)
  for more information.
- `.cleanupOnFail` (Optional): Allows deletion of new resources created during
  the upgrade of the release when it fails. Defaults to `false`.
- `.disableHooks` (Optional): Prevents [chart hooks](https://helm.sh/docs/topics/charts_hooks/)
  from running during the upgrade of the release. Defaults to `false`.
- `.disableOpenAPIValidation` (Optional): Prevents Helm from validating the
  rendered templates against the Kubernetes OpenAPI Schema. Defaults to `false`.
- `.disableSchemaValidation` (Optional): Prevents Helm from validating the
  values against the JSON Schema. Defaults to `false`.
- `.disableTakeOwnership` (Optional): Disables taking ownership of existing resources
  during the Helm upgrade action. Defaults to `false`.
- `.disableWait` (Optional): Disables waiting for resources to be ready after
  upgrading the release. Defaults to `false`.
- `.disableWaitForJobs` (Optional): Disables waiting for any Jobs to complete
  after upgrading the release. Defaults to `false`.
- `.force` (Optional): Forces resource updates through a replacement strategy.
  Defaults to `false`.
- `.preserveValues` (Optional): Instructs Helm to re-use the values from the
  last release while merging in overrides from [values](#values). Setting
  this flag makes the HelmRelease non-declarative. Defaults to `false`.
- `.serverSideApply` (Optional): Controls Server-Side Apply for resources during
  the upgrade. Can be `enabled`, `disabled`, or `auto`. When `auto`, the apply
  method will be based on the release's previous usage. Defaults to `auto`.

#### Upgrade strategy

`.spec.upgrade.strategy` is an optional field to specify the strategy
to use when running a Helm upgrade action.

The field offers the following subfields:

- `.name` (Required): The name of the upgrade strategy to use. One of
  `RemediateOnFailure` or `RetryOnFailure`. If the `.spec.upgrade.strategy`
  field is not specified, the HelmRelease reconciliation behaves as if
  `.spec.upgrade.strategy.name` was set to `RemediateOnFailure`.
- `.retryInterval` (Optional): The time to wait between retries of failed
  releases when the upgrade strategy is set to `RetryOnFailure`. Defaults
  to `5m`. Cannot be used with `RemediateOnFailure`.

The default `RemediateOnFailure` strategy applies the rules defined by the
`.spec.upgrade.remediation` field to the upgrade action, i.e. the same
behavior of the controller prior to the introduction of the `RetryOnFailure`
strategy.

The `RetryOnFailure` strategy will retry failed upgrades in a regular
interval defined by the `.spec.upgrade.strategy.retryInterval` field,
without applying any remediation.

#### Upgrade remediation

`.spec.upgrade.remediation` is an optional field to configure the remediation
strategy to use when the upgrade of a Helm release fails.

The field offers the following subfields:

- `.retries` (Optional): The number of retries that should be attempted on
  failures before bailing. Remediation, using the `.strategy`, is performed
  between each attempt. Defaults to `0`, a negative integer equals to an
  infinite number of retries.
- `.strategy` (Optional): The remediation strategy to use when a Helm upgrade
  fails. Valid values are `rollback` and `uninstall`. Defaults to `rollback`.
- `.ignoreTestFailures` (Optional): Instructs the controller to not remediate
  when a [Helm test](#test-configuration) failure occurs. Defaults to
  `.spec.test.ignoreFailures`.
- `.remediateLastFailure` (Optional): Instructs the controller to remediate the
  last failure when no retries remain. Defaults to `false` unless `.retries` is
  greater than `0`.

### Test configuration

`.spec.test` is an optional field to specify the configuration values for the
[Helm test action](https://helm.sh/docs/helm/helm_test/).

To make the controller run the [Helm tests available for the chart](https://helm.sh/docs/topics/chart_tests/)
after a successful Helm install or upgrade, `.spec.test.enable` can be set to
`true`. When enabled, the test results will be available in the
[`.status.history`](#history) field and emitted as a Kubernetes Event.

By default, when tests are enabled, failures in tests are considered release
failures, and thus are subject to the triggering Helm action's remediation
configuration. However, test failures can be ignored by setting
`.spec.test.ignoreFailures` to `true`. In this case, no remediation action
will be taken, and the test failure will not affect the `Ready` status
condition. This can be overridden per Helm action by setting the respective
[install](#install-configuration) or [upgrade](#upgrade-configuration)
configuration option.

```yaml
spec:
  test:
    enable: true
    ignoreFailures: true
```

#### Filtering tests

`.spec.test.filters` is an optional list to include or exclude specific tests
from being run.

```yaml
spec:
  test:
    enable: true
    filters:
      - name: my-release-test-connection
        exclude: false
      - name: my-release-test-migration
        exclude: true
```

### Rollback configuration

`.spec.rollback` is an optional field to specify the configuration values for
a [Helm rollback action](https://helm.sh/docs/helm/helm_rollback/). This
configuration applies when the [upgrade remediation strategy](#upgrade-remediation)
is set to `rollback`.

The field offers the following subfields:

- `.timeout` (Optional): The time to wait for any individual Kubernetes
  operation (like Jobs for hooks) during the rollback of the release.
  Defaults to the [global timeout value](#timeout).
- `.cleanupOnFail` (Optional): Allows deletion of new resources created during
  the rollback of the release when it fails. Defaults to `false`.
- `.disableHooks` (Optional): Prevents [chart hooks](https://helm.sh/docs/topics/charts_hooks/)
  from running during the rollback of the release. Defaults to `false`.
- `.disableWait` (Optional): Disables waiting for resources to be ready after
  rolling back the release. Defaults to `false`.
- `.disableWaitForJobs` (Optional): Disables waiting for any Jobs to complete
  after rolling back the release. Defaults to `false`.
- `.force` (Optional): Forces resource updates through a replacement strategy.
  Defaults to `false`.
- `.recreate` (Optional): Performs Pod restarts if applicable. Defaults to
  `false`. **Warning**: As of Flux v2.8, this option is deprecated and no
  longer has any effect. It will be removed in a future release. The
  helm-controller will print a warning if this option is used. Please
  see the [Helm 4 issue](https://github.com/fluxcd/helm-controller/issues/1300#issuecomment-3740272924)
  for more details.
- `.serverSideApply` (Optional): Controls Server-Side Apply for resources during
  the rollback. Can be `enabled`, `disabled`, or `auto`. When `auto`, the apply
  method will be based on the release's previous usage. Defaults to `auto`.

### Uninstall configuration

`.spec.uninstall` is an optional field to specify the configuration values for
a [Helm uninstall action](https://helm.sh/docs/helm/helm_uninstall/). This
configuration applies to the [install remediation](#install-remediation), and
when the [upgrade remediation strategy](#upgrade-remediation) is set to
`uninstall`.

The field offers the following subfields:

- `.timeout` (Optional): The time to wait for any individual Kubernetes
  operation (like Jobs for hooks) during the uninstalltion of the release.
  Defaults to the [global timeout value](#timeout).
- `.deletionPropagation` (Optional): The [deletion propagation policy](https://kubernetes.io/docs/tasks/administer-cluster/use-cascading-deletion/)
  when a Helm uninstall is performed. Valid values are `background`,
  `foreground` and `orphan`. Defaults to `background`.
- `.disableHooks` (Optional): Prevents [chart hooks](https://helm.sh/docs/topics/charts_hooks/)
  from running during the uninstallation of the release. Defaults to `false`.
- `.disableWait` (Optional): Disables waiting for resources to be deleted after
  uninstalling the release. Defaults to `false`.
- `.keepHistory` (Optional): Instructs Helm to remove all associated resources
  and mark the release as deleted, but to retain the release history. Defaults
  to `false`.

### Drift detection

`.spec.driftDetection` is an optional field to enable the detection (and
correction) of cluster-state drift compared to the manifest from the Helm
storage.

When `.spec.driftDetection.mode` is set to `warn` or `enabled`, and the
desired state of the HelmRelease is in-sync with the Helm release object in
the storage, the controller will compare the manifest from the Helm storage
with the current state of the cluster using a
[server-side dry-run apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/).

If this comparison detects a drift (either due to a resource being created
or modified during the dry-run), the controller will emit a Kubernetes Event
with a short summary of the detected changes. In addition, a more extensive
[JSON Patch](https://datatracker.ietf.org/doc/html/rfc6902) summary is logged
to the controller logs (with `--log-level=debug`).

#### Drift correction

Furthermore, when `.spec.driftDetection.mode` is set to `enabled`, the
controller will attempt to correct the drift by creating and patching the
resources based on the server-side dry-run apply result.

At the end of the correction attempt, it will emit a Kubernetes Event with a
summary of the changes it made and any failures it encountered. In case of a
failure, it will continue to detect and correct drift until the desired state
has been reached, or a new Helm action is triggered (due to e.g. a change to
the spec).

#### Ignore rules

`.spec.driftDetection.ignore` is an optional field to provide
[JSON Pointers](https://datatracker.ietf.org/doc/html/rfc6901) to ignore while
detecting and correcting drift. This can for example be useful when Horizontal
Pod Autoscaling is enabled for a Deployment, or when a Helm chart has hooks
which mutate a resource.

```yaml
spec:
  driftDetection:
    mode: enabled
    ignore:
      - paths: ["/spec/replicas"]
```

**Note:** It is possible to achieve a likewise behavior as using
[ignore annotations](#ignore-annotation) by configuring a JSON Pointer
targeting a whole document (`""`).

To ignore `.paths` in a specific target resource, a `.target` selector can be
applied to the ignored paths.

```yaml
spec:
  driftDetection:
    ignore:
     - paths: ["/spec/replicas"]
       target:
         kind: Deployment
```

The following `.target` selectors are available, defining multiple fields
causes the selector to be more specific:

- `group` (Optional): Matches the `.apiVersion` group of resources while
  offering support for regular expressions. For example, `apps`,
  `helm.toolkit.fluxcd.io` or `.*.toolkit.fluxcd.io`.
- `version` (Optional): Matches the `.apiVersion` version of resources while
  offering support for regular expressions. For example, `v1`, `v2beta2` or
  `v2beta[\d]`.
- `kind` (Optional): Matches the `.kind` of resources while offering support
  for regular expressions. For example, `Deployment`, `HelmRelelease` or
  `(HelmRelease|HelmChart)`.
- `name` (Optional): Matches the `.metadata.name` of resources while offering
  support for regular expressions. For example, `podinfo` or `podinfo.*`.
- `namespace` (Optional): Matches the `.metadata.namespace` of resources while
  offering support for regular expressions. For example, `my-release-ns` or
  `.*-system`.
- `annotationSelector` (Optional): Matches the `.metadata.annotations` of
  resources using a [label selector expression](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors).
  For example, `environment = production` or `environment notin (staging)`.
- `labelSelector` (Optional): Matches the `.metadata.labels` of resources
  using a [label selector expression](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#label-selectors).
  For example, `environment = production` or `environment notin (staging)`.

#### Ignore annotation

To exclude certain resources from the comparison, they can be labeled or
annotated with `helm.toolkit.fluxcd.io/driftDetection: disabled`. Using
[post-renderers](#post-renderers), this can be applied to any resource
rendered by Helm.

```yaml
spec:
  postRenderers:
    - kustomize:
        patches:
          - target:
              version: v1
              kind: Deployment
              name: my-app
            patch: |
              - op: add
                path: /metadata/annotations/helm.toolkit.fluxcd.io~1driftDetection
                value: disabled
```

**Note:** In many cases, it may be better (and easier) to configure an [ignore
rule](#ignore-rules) to ignore (a portion of) a resource.

### Common metadata

`.spec.commonMetadata` is an optional field used to specify any metadata that
should be applied to all the Helm Chart's resources via kustomize post renderer. It has two optional fields:

- `labels`: A map used for setting [labels](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/)
  on an object. Any existing label will be overridden if it matches with a key in
  this map.
- `annotations`: A map used for setting [annotations](https://kubernetes.io/docs/concepts/overview/working-with-objects/annotations/)
  on an object. Any existing annotation will be overridden if it matches with a key
  in this map.

### Post renderers

`.spec.postRenderers` is an optional list to provide [post rendering](https://helm.sh/docs/topics/advanced/#post-rendering)
capabilities using the following built-in Kustomize directives:

- [patches](https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/patches/) (`kustomize.patches`)
- [images](https://kubectl.docs.kubernetes.io/references/kustomize/kustomization/images/) (`kustomize.images`)

Post renderers are applied in the order given, and persisted by Helm to the
manifest for the release in the storage.

**Note:** [Helm has a limitation at present](https://github.com/helm/helm/issues/7891),
which prevents post renderers from being applied to chart hooks.

```yaml
spec:
  postRenderers:
    - kustomize:
        patches:
          - target:
              version: v1
              kind: Deployment
              name: metrics-server
            patch: |
              - op: add
                path: /metadata/labels/environment
                value: production
        images:
          - name: docker.io/bitnami/metrics-server
            newName: docker.io/bitnami/metrics-server
            newTag: 0.4.1-debian-10-r54
```

### Wait strategy

`.spec.waitStrategy` is an optional field to configure how the controller waits
for resources to become ready after Helm actions.

The field offers the following subfields:

- `.name` (Required): The strategy for waiting for resources to be ready.
  One of `watcher` or `legacy`. The `watcher` strategy uses kstatus to watch resource
  statuses, while the `legacy` strategy uses Helm v3's waiting logic. Defaults to
  `watcher`, or to `legacy` when the `UseHelm3Defaults` feature gate is enabled.

```yaml
spec:
  waitStrategy:
    name: watcher
```

### Health check expressions

`.spec.healthCheckExprs` can be used to define custom logic for performing health
checks on custom resources using [Common Expression Language (CEL)](https://cel.dev/).

The expressions are evaluated only when the Helm action taking place has wait
enabled (i.e. `.spec.<action>.disableWait` is `false`) and the `watcher`
wait strategy is used (i.e. `.spec.waitStrategy.name` is `watcher`).

The `.spec.healthCheckExprs` field accepts a list of objects with the following fields:

- `apiVersion`: The API version of the custom resource. Required.
- `kind`: The kind of the custom resource. Required.
- `current`: A required CEL expression that returns `true` if the resource is ready.
- `inProgress`: An optional CEL expression that returns `true` if the resource
  is still being reconciled.
- `failed`: An optional CEL expression that returns `true` if the resource
  failed to reconcile.

The controller will evaluate the expressions in the following order:

1. `inProgress` if specified
2. `failed` if specified
3. `current`

The first expression that evaluates to `true` will determine the health
status of the custom resource.

For example, to define a set of health check expressions for the `SealedSecret`
custom resource:

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: sealed-secrets
spec:
  interval: 10m
  chartRef:
    kind: OCIRepository
    name: sealed-secrets-chart
  values:
    replicaCount: 2
  healthCheckExprs:
    - apiVersion: bitnami.com/v1alpha1
      kind: SealedSecret
      failed: status.conditions.filter(e, e.type == 'Synced').all(e, e.status == 'False')
      current: status.conditions.filter(e, e.type == 'Synced').all(e, e.status == 'True')
```

A common error is writing expressions that reference fields that do not
exist in the custom resource. This will cause the controller to wait
for the resource to be ready until the timeout is reached. To avoid this,
make sure your CEL expressions are correct. The
[CEL Playground](https://playcel.undistro.io/) is a useful resource for
this task. The input passed to each expression is the custom resource
object itself. You can check for field existence with the
[`has(...)` CEL macro](https://github.com/google/cel-spec/blob/master/doc/langdef.md#macros),
just be aware that `has(status)` errors if `status` does not (yet) exist
on the top level of the resource you are using.

It's worth checking if [the library](/flux/cheatsheets/cel-healthchecks/)
has expressions for the custom resources you are using.

### KubeConfig (Remote clusters)

With the `.spec.kubeConfig` field a HelmRelease
can apply and manage resources on a remote cluster.

Two authentication alternatives are available:

- `.spec.kubeConfig.secretRef`: Secret-based authentication using a
  static kubeconfig stored in a Kubernetes Secret in the same namespace
  as the HelmRelease.
- `.spec.kubeConfig.configMapRef` (Recommended): Secret-less authentication
  building a kubeconfig dynamically with parameters stored in a Kubernetes
  ConfigMap in the same namespace as the HelmRelease via workload identity.

To make a HelmRelease react immediately to changes in the referenced Secret
or ConfigMap see [this](#reacting-immediately-to-configuration-dependencies)
section.

When both `.spec.kubeConfig` and
[`.spec.serviceAccountName`](#service-account-reference) are specified,
the controller will impersonate the ServiceAccount on the target cluster,
i.e. a ServiceAccount with name `.spec.serviceAccountName` must exist in
the target cluster inside a namespace with the same name as the namespace
of the HelmRelease. For example, if the HelmRelease is in the namespace
`apps` of the cluster where Flux is running, then the ServiceAccount
must be in the `apps` namespace of the target remote cluster, and have the
name `.spec.serviceAccountName`. In other words, the namespace of the
HelmRelease must exist both in the cluster where Flux is running
and in the target remote cluster where Flux will apply resources.

The Helm storage is stored on the remote cluster in a namespace that equals to
the namespace of the HelmRelease, or the [configured storage namespace](#storage-namespace).
The release itself is made in a namespace that equals to the namespace of the
HelmRelease, or the [configured target namespace](#target-namespace). The
namespaces are expected to exist, with the exception that the target namespace
can be created on demand by Helm when namespace creation is [configured during
install](#install-configuration).

Other references to Kubernetes resources in the HelmRelease, like
[values references](#values-references), are expected to exist on
the cluster where Flux is running.

#### Secret-based authentication

`.spec.kubeConfig.secretRef.name` is an optional field to specify the name of
a Secret containing a KubeConfig. If specified, the Helm operations will be
targeted at the default cluster specified in this KubeConfig instead of using
the in-cluster Service Account.

The Secret defined in the `.secretRef` must exist in the same namespace as the
HelmRelease. On every reconciliation, the KubeConfig bytes will be loaded from
the `.secretRef.key` (default: `value` or `value.yaml`) of the Secret's data,
and the Secret can thus be regularly updated if cluster access tokens have to
rotate due to expiration.

```yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: prod-kubeconfig
type: Opaque
stringData:
  value.yaml: |
    apiVersion: v1
    kind: Config
    # ...omitted for brevity
```

**Note:** The KubeConfig should be self-contained and not rely on binaries, the
environment, or credential files from the helm-controller Pod. This matches the
constraints of KubeConfigs from current Cluster API providers. KubeConfigs with
`cmd-path` in them likely won't work without a custom, per-provider installation
of helm-controller. For more information, see
[remote clusters/Cluster-API](#remote-cluster-api-clusters).

#### Secret-less authentication

The field `.spec.kubeConfig.configMapRef.name` can be used to specify the
name of a ConfigMap in the same namespace as the HelmRelease containing
parameters for secret-less authentication via workload identity. The
supported keys inside the `.data` field of the ConfigMap are:

- `.data.provider`: The provider to use. One of `aws`, `azure`, `gcp`,
  or `generic`. Required. The `aws` provider is used for connecting to
  remote EKS clusters, `azure` for AKS, `gcp` for GKE, and `generic`
  for Kubernetes OIDC authentication between clusters. For the
  `generic` provider, the remote cluster must be configured to trust
  the OIDC issuer of the cluster where Flux is running.
- `.data.cluster`: The fully qualified resource name of the Kubernetes
  cluster in the cloud provider API. Not used by the `generic`
  provider. Required when one of `.data.address` or `.data["ca.crt"]` is
  not set, or if the provider is `aws` (required for defining a region).
- `.data.address`: The address of the Kubernetes API server. Required
  for `generic`. For the other providers, if not specified, the
  first address in the cluster resource will be used, and if
  specified, it must match one of the addresses in the cluster
  resource.
  If `audiences` is not set, will be used as the audience for the
  `generic` provider.
- `.data["ca.crt"]`: The optional PEM-encoded CA certificate for the
  Kubernetes API server. If not set, the controller will use the
  CA certificate from the cluster resource.
- `.data.audiences`: The optional audiences as a list of
  line-break-separated strings for the Kubernetes ServiceAccount token.
  Defaults to the address for the `generic` provider, or to specific
  values for the other providers depending on the provider.
- `.data.serviceAccountName`: The optional name of the Kubernetes
  ServiceAccount in the same namespace that should be used
  for authentication. If not specified, the controller
  ServiceAccount will be used. Not confuse with the ServiceAccount
  used for impersonation, which is specified with
  [`.spec.serviceAccountName`](#service-account-reference) directly
  in the HelmRelease spec and must exist in the target remote cluster.

The `.data.cluster` field, when specified, must have the following formats:

- `aws`: `arn:<partition>:eks:<region>:<account-id>:cluster/<cluster-name>`
- `azure`: `/subscriptions/<subscription-id>/resourceGroups/<resource-group>/providers/Microsoft.ContainerService/managedClusters/<cluster-name>`
- `gcp`: `projects/<project-id>/locations/<location>/clusters/<cluster-name>`

For complete guides on workload identity and setting up permissions for
this feature, see the following docs:

- [EKS](/flux/integrations/aws/#for-amazon-elastic-kubernetes-service)
- [AKS](/flux/integrations/azure/#for-azure-kubernetes-service)
- [GKE](/flux/integrations/gcp/#for-google-kubernetes-engine)
- [Generic](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#configuring-the-api-server)

Example for an EKS cluster:

```yaml
apiVersion: helm.toolkit.fluxcd.io/v1
kind: HelmRelease
metadata:
  name: backend
  namespace: apps
spec:
  ... # other fields omitted for brevity
  kubeConfig:
    configMapRef:
      name: kubeconfig
  serviceAccountName: apps-sa # optional. must exist in the target cluster. user for impersonation
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: kubeconfig
  namespace: apps
data:
  provider: aws
  cluster: arn:aws:eks:eu-central-1:123456789012:cluster/my-cluster
  serviceAccountName: apps-iam-role # optional. maps to an AWS IAM Role. used for authentication
```

### Interval

`.spec.interval` is a required field that specifies the interval at which the
HelmRelease is reconciled, i.e. the controller ensures the current Helm release
matches the desired state.

After successfully reconciling the object, the controller requeues it for
inspection at the specified interval. The value must be in a [Go recognized
duration string format](https://pkg.go.dev/time#ParseDuration), e.g. `15m0s`
to reconcile the object every fifteen minutes.

If the `.metadata.generation` of a resource changes (due to e.g. a change to
the spec) or the HelmChart revision changes (which generates a Kubernetes
Event), or a ConfigMap/Secret referenced in `valuesFrom` changes,
this is handled instantly outside the interval window.

**Note:** The controller can be configured to apply a jitter to the interval in
order to distribute the load more evenly when multiple HelmRelease objects are
set up with the same interval. For more information, please refer to the 
[helm-controller configuration options](https://fluxcd.io/flux/components/helm/options/).

### Timeout

`.spec.timeout` is an optional field to specify a timeout for a Helm action like
install, upgrade or rollback. The value must be in a
[Go recognized duration string format](https://pkg.go.dev/time#ParseDuration),
e.g. `5m30s` for a timeout of five minutes and thirty seconds. The default
value is `5m0s`.

### Suspend

`.spec.suspend` is an optional field to suspend the reconciliation of a
HelmRelease. When set to `true`, the controller will stop reconciling the
HelmRelease, and changes to the resource or the Helm chart will not result in
a new Helm release. When the field is set to `false` or removed, it will
resume.

## Working with HelmReleases

### Recommended settings

When deploying applications to production environments, it is recommended
to use OCI-based Helm charts with OCIRepository as `chartRef`, and
to configure the following fields, while adjusting them to your desires for
responsiveness:

```yaml
apiVersion: source.toolkit.fluxcd.io/v1
kind: OCIRepository
metadata:
  name: webapp-chart
  namespace: apps
spec:
  interval: 5m # check for new versions every 5 minutes and trigger an upgrade
  url: oci://ghcr.io/org/charts/webapp
  secretRef:
    name: registry-auth # Image pull secret with read-only access
  layerSelector: # select the Helm chart layer
    mediaType: "application/vnd.cncf.helm.chart.content.v1.tar+gzip"
    operation: copy
  ref:
    semver: "*" # track the latest stable version
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: webapp
  namespace: apps
spec:
  releaseName: webapp
  chartRef:
    kind: OCIRepository
    name: webapp-chart
  interval: 30m # run drift detection every 30 minutes
  driftDetection:
    mode: enabled # undo kubectl edits and other unintended changes
  install:
    strategy:
      name: RetryOnFailure # retry failed installations instead of uninstalling
      retryInterval: 5m # retry failed installations every five minutes
  upgrade:
    crds: CreateReplace # update CRDs when upgrading
    strategy:
      name: RetryOnFailure # retry failed upgrades instead of rollback
      retryInterval: 5m # retry failed upgrades every five minutes
  # All ConfigMaps and Secrets referenced in valuesFrom should
  # be labelled with `reconcile.fluxcd.io/watch: Enabled`
  valuesFrom:
    - kind: ConfigMap
      name: webapp-values
    - kind: Secret
      name: webapp-secret-values
```

Note that the `RetryOnFailure` strategy is suitable for statefulsets
and other workloads that cannot tolerate rollbacks and have a high rollout duration
susceptible to health check timeouts and transient capacity errors.

For stateless workloads and applications that can tolerate rollbacks, the
`RemediateOnFailure` strategy may be more suitable, as it will ensure that
the last known good state is restored in case of a failure.

### Configuring failure handling

From time to time, a Helm installation, upgrade, or accompanying [Helm test](#test-configuration)
may fail. When this happens, by default no action is taken, and the release is
left in a failed state. However, several automatic failure remediation options
can be set via [`.spec.install.remediation`](#install-remediation) and
[`.spec.upgrade.remediation`](#upgrade-remediation).

By configuring the `.retries` field for the respective action, the controller
will first remediate the failure by performing a Helm rollback or uninstall, and
then reattempt the action. It will repeat this process until the `.retries`
are exhausted, or the action succeeds.

Once the `.retries` are exhausted, the controller will stop attempting to
remediate the failure, and the Helm release will be left in a failed state.
To ensure the Helm release is brought back to the last known good state or
uninstalled, `.remediateLastFailure` can be set to `true`.
For Helm upgrades, this defaults to `true` if at least one retry is configured.

When a new release configuration or Helm chart is detected, the controller will
reset the failure counters and attempt to install or upgrade the release again.

**Note:** In addition to the automatic failure remediation options, the
controller can be instructed to [force a Helm release](#forcing-a-release) or
to [retry a failed Helm release](#resetting-remediation-retries)

### Controlling the lifecycle of Custom Resource Definitions

Helm does support [the installation of Custom Resource Definitions](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-1-let-helm-do-it-for-you)
(CRDs) as part of a chart. However, it has no native support for
[upgrading CRDs](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#some-caveats-and-explanations):

> There is no support at this time for upgrading or deleting CRDs using Helm.
> This was an explicit decision after much community discussion due to the
> danger for unintentional data loss. Furthermore, there is currently no
> community consensus around how to handle CRDs and their lifecycle. As this
> evolves, Helm will add support for those use cases.

If you write your own Helm charts, you can work around this limitation by
putting your CRDs into the templates instead of the `crds/` directory, or by
factoring them out into a separate Helm chart as suggested by the [official Helm
documentation](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-2-separate-charts).

However, if you use a third-party Helm chart that installs CRDs, not being able
to upgrade the CRDs via HelmRelease objects might become a cumbersome limitation
within your GitOps workflow. Therefore, Flux allows you to opt in to upgrading
CRDs by setting the `.crds` policy in the [`.spec.install`](#install-configuration)
and [`.spec.upgrade`](#upgrade-configuration) configurations.

The following policy values are supported:

- `Skip`: Skip the installation or upgrade of CRDs. This is the default value
  for `.spec.upgrade.crds`.
- `Create`: Create CRDs if they do not exist, but do not upgrade or delete them.
  This is the default value for `.spec.install.crds`.
- `CreateReplace`: Create new CRDs, update (replace) existing ones, but **do
  not** delete CRDs which no longer exist in the current Helm chart.

For example, if you want to update CRDs when installing and upgrading a Helm
chart, you can set the `.spec.install.crds` and `.spec.upgrade.crds` policies to
`CreateReplace`:

```yaml
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: my-operator
  namespace: default
spec:
  interval: 15m
  chart:
    spec:
      chart: my-operator
      version: "1.0.1"
      sourceRef:
        kind: HelmRepository
        name: my-operator-repo
      interval: 5m
  install:
    crds: CreateReplace
  upgrade:
    crds: CreateReplace
```

### Role-based access control

By default, a HelmRelease runs under the cluster admin account and can create,
modify, and delete cluster level objects (ClusterRoles, ClusterRoleBindings,
CustomResourceDefinitions, etc.) and namespaced objects (Deployments, Ingresses,
etc.)

For certain HelmReleases, a cluster administrator may wish to restrict the
permissions of the HelmRelease to a specific namespace or to a specific set of
namespaced objects. To restrict a HelmRelease, one can assign a Service Account
under which the reconciliation is performed using
[`.spec.serviceAccountName`](#service-account-reference).

Assuming you want to restrict a group of HelmReleases to a single namespace,
you can create a Service Account with a RoleBinding that grants access only to
that namespace.

For example, the following Service Account and RoleBinding restricts the
HelmRelease to the `webapp` namespace:

```yaml
---
apiVersion: v1
kind: Namespace
metadata:
  name: webapp
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: webapp-reconciler
  namespace: webapp
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: webapp-reconciler
  namespace: webapp
rules:
  - apiGroups: ["*"]
    resources: ["*"]
    verbs: ["*"]
---
 apiVersion: rbac.authorization.k8s.io/v1
 kind: RoleBinding
 metadata:
   name: webapp-reconciler
   namespace: webapp
 roleRef:
   apiGroup: rbac.authorization.k8s.io
   kind: Role
   name: webapp-reconciler
 subjects:
   - kind: ServiceAccount
     name: webapp-reconciler
     namespace: webapp
```

**Note:** The above resources are not created by the helm-controller, but should
be created by a cluster administrator and preferably be managed by a
[Kustomization](https://fluxcd.io/flux/components/kustomize/kustomizations/).

The Service Account can then be referenced in the HelmRelease:

```yaml
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
 name: podinfo
 namespace: webapp
spec:
 serviceAccountName: webapp-reconciler
 interval: 15m
 chart:
   spec:
     chart: podinfo
     sourceRef:
       kind: HelmRepository
       name: podinfo
```

When the controller reconciles the `podinfo` HelmRelease, it will impersonate
the `webapp-reconciler` Service Account. If the chart contains cluster level
objects like CustomResourceDefinitions, the reconciliation will fail since the
account it runs under has no permissions to alter objects outside the
`webapp` namespace.

#### Enforcing impersonation

On multi-tenant clusters, platform admins can enforce impersonation with the
`--default-service-account` flag.

When the flag is set, HelmReleases which do not have a `.spec.serviceAccountName`
specified will use the Service Account name provided by
`--default-service-account=<name>` in the namespace of the HelmRelease object.

For further best practices on securing helm-controller, see our
[best practices guide](https://fluxcd.io/flux/security/best-practices).

### Remote Cluster API clusters

Using a [`.spec.kubeConfig` reference](#kubeconfig-remote-clusters), it is possible
to manage the full lifecycle of Helm releases on remote clusters.
This composes well with Cluster-API bootstrap providers such as CAPBK (kubeadm),
CAPA (AWS), and others.

To reconcile a HelmRelease to a CAPI controlled cluster, put the HelmRelease in
the same namespace as your Cluster object, and set the
`.spec.kubeConfig.secretRef.name` to `<cluster-name>-kubeconfig`:

```yaml
---
apiVersion: cluster.x-k8s.io/v1alpha3
kind: Cluster
metadata:
  name: stage # the kubeconfig Secret will contain the Cluster name
  namespace: capi-stage
spec:
  clusterNetwork:
    pods:
      cidrBlocks:
        - 10.100.0.0/16
    serviceDomain: stage-cluster.local
    services:
      cidrBlocks:
        - 10.200.0.0/12
  controlPlaneRef:
    apiVersion: controlplane.cluster.x-k8s.io/v1alpha3
    kind: KubeadmControlPlane
    name: stage-control-plane
    namespace: capi-stage
  infrastructureRef:
    apiVersion: infrastructure.cluster.x-k8s.io/v1alpha3
    kind: DockerCluster
    name: stage
    namespace: capi-stage
---
# ... unrelated Cluster API objects omitted for brevity ...
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: kube-prometheus-stack
  namespace: capi-stage
spec:
  kubeConfig:
    secretRef:
      name: stage-kubeconfig # Cluster API creates this for the matching Cluster
  chart:
    spec:
      chart: prometheus
      version: ">=4.0.0 <5.0.0"
      sourceRef:
        kind: HelmRepository
        name: prometheus-community
  install:
    remediation:
      retries: -1
```

The Cluster and HelmRelease can be created at the same time as long as the
[install remediation configuration](#install-remediation) is set to a
forgiving number of `.retries`. The HelmRelease will then eventually succeed
in installing the Helm chart once the cluster is available.

If you want to target clusters created by other means than Cluster-API, you can
create a Service Account with the necessary permissions on the target cluster,
generate a KubeConfig for that account, and then create a Secret on the cluster
where helm-controller is running. For example:

```shell
kubectl -n default create secret generic prod-kubeconfig \
    --from-file=value.yaml=./kubeconfig
```

### Triggering a reconcile

To manually tell the helm-controller to reconcile a HelmRelease outside the
[specified interval window](#interval), it can be annotated with
`reconcile.fluxcd.io/requestedAt: <arbitrary value>`.

Annotating the resource queues the HelmRelease for reconciliation if the
`<arbitrary-value>` differs from the last value the controller acted on, as
reported in `.status.lastHandledReconcileAt`.

Using `kubectl`:

```sh
kubectl annotate --field-manager=flux-client-side-apply --overwrite helmrelease/<helmrelease-name> reconcile.fluxcd.io/requestedAt="$(date +%s)"
```

Using `flux`:

```sh
flux reconcile helmrelease <helmrelease-name>
```

### Forcing a release

To instruct the helm-controller to forcefully perform a Helm install or
upgrade without making changes to the spec, it can be annotated with
`reconcile.fluxcd.io/forceAt: <arbitrary value>` while simultaneously
[triggering a reconcile](#triggering-a-reconcile) with the same value.

Annotating the resource forces a one-off Helm install or upgrade if the
`<arbitrary-value>` differs from the last value the controller acted on, as
reported in `.status.lastHandledForceAt` and `.status.lastHandledReconcileAt`.

Using `kubectl`:

```sh
TOKEN="$(date +%s)"; \
kubectl annotate --field-manager=flux-client-side-apply --overwrite helmrelease/<helmrelease-name> \
"reconcile.fluxcd.io/requestedAt=$TOKEN" \
"reconcile.fluxcd.io/forceAt=$TOKEN"
```

Using `flux`:

```sh
flux reconcile helmrelease <helmrelease-name> --force
```

### Resetting remediation retries

To instruct the helm-controller to reset the number of retries while
attempting to perform a Helm release, it can be annotated with
`reconcile.fluxcd.io/resetAt: <arbitrary value>` while simultaneously
[triggering a reconcile](#triggering-a-reconcile) with the same value.

Annotating the resource resets the failure counts on the object if the
`<arbitrary-value>` differs from the last value the controller acted on, as
reported in `.status.lastHandledResetAt` and `.status.lastHandledReconcileAt`.
This effectively allows it to continue to attempt to perform a Helm release
based on the [install](#install-remediation) or [upgrade](#upgrade-remediation)
remediation configuration.

Using `kubectl`:

```sh
TOKEN="$(date +%s)"; \
kubectl annotate --field-manager=flux-client-side-apply --overwrite helmrelease/<helmrelease-name> \
"reconcile.fluxcd.io/requestedAt=$TOKEN" \
"reconcile.fluxcd.io/resetAt=$TOKEN"
```

Using `flux`:

```sh
flux reconcile helmrelease <helmrelease-name> --reset
```

### Handling failed uninstall

At times, a Helm uninstall may fail due to the resource deletion taking a long
time, resources getting stuck in deleting phase due to some resource delete
policy in the cluster or some failing delete hooks. Depending on the scenario,
this can be handled in a few different ways.

For resources that take long to delete but are certain to get deleted without
any intervention, failed uninstall will be retried until they succeeds. The
HelmRelease object will remain in a failed state until the uninstall succeeds.
Once uninstall is successful, the HelmRelease object will get deleted.

If resources get stuck at deletion due to some dependency on some other
resource or policy, the controller will keep retrying to delete the resources.
The HelmRelease object will remain in a failed state. Once the cause of resource
deletion issue is resolved by intervention, HelmRelease uninstallation will
succeed and the HelmRelease object will get deleted. In case the cause of the
deletion issue can't be resolved, the HelmRelease can be force deleted by
manually deleting the [Helm storage
secret](https://helm.sh/docs/topics/advanced/#storage-backends) from the
respective release namespace. When the controller retries uninstall and cannot
find the release, it assumes that the release has been deleted, Helm uninstall
succeeds and the HelmRelease object gets deleted. This leaves behind all the
release resources. They have to be manually deleted.

If a chart with pre-delete hooks fail, the controller will re-run the hooks
until they succeed and unblock the uninstallation. The Helm uninstall error
will be present in the status of HelmRelease. This can be used to identify which
hook is failing. If the hook failure persists, to run uninstall without the
hooks, equivalent of running `helm uninstall --no-hooks`, update the HelmRelease
to set `.spec.uninstall.disableHooks` to `true`.

```yaml
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
...
spec:
  ...
  uninstall:
    disableHooks: true
```

In the next reconciliation, the controller will run Helm uninstall without the
hooks. On success, the HelmRelease will get deleted. Otherwise, check the status
of the HelmRelease for other failure that may be blocking the uninstall.

In case of charts with post-delete hooks, since the hook runs after the deletion
of the resources and the Helm storage, the hook failure will result in an
initial uninstall failure. In the subsequent reconciliation to retry uninstall,
since the Helm storage for the release got deleted, uninstall will succeed and
the HelmRelease object will get deleted.

Any leftover pre or post-delete hook resources have to be manually deleted.

### Waiting for `Ready`

When a change is applied, it is possible to wait for the HelmRelease to reach a
`Ready` state using `kubectl`:

```sh
kubectl wait helmrelease/<helmrelease-name> --for=condition=ready --timeout=5m
```

### Suspending and resuming

When you find yourself in a situation where you temporarily want to pause the
reconciliation of a HelmRelease, you can suspend it using the
[`.spec.suspend` field](#suspend).

#### Suspend a HelmRelease

In your YAML declaration:

```yaml
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: <helmrelease-name>
spec:
  suspend: true
```

Using `kubectl`:

```sh
kubectl patch helmrelease <helmrelease-name> --field-manager=flux-client-side-apply -p '{\"spec\": {\"suspend\" : true }}'
```

Using `flux`:

```sh
flux suspend helmrelease <helmrelease-name>
```

##### Resume a HelmRelease

In your YAML declaration, comment out (or remove) the field:

```yaml
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: <helmrelease-name>
spec:
  # suspend: true
```

**Note:** Setting the field value to `false` has the same effect as removing
it, but does not allow for "hot patching" using e.g. `kubectl` while practicing
GitOps; as the manually applied patch would be overwritten by the declared
state in Git.

Using `kubectl`:

```sh
kubectl patch helmrelease <helmrelease-name> --field-manager=flux-client-side-apply -p '{\"spec\" : {\"suspend\" : false }}'
```

Using `flux`:

```sh
flux resume helmrelease <helmrelease-name>
```

### Debugging a HelmRelease

There are several ways to gather information about a HelmRelease for debugging
purposes.

#### Describe the HelmRelease

Describing a HelmRelease using `kubectl describe helmrelease <release-name>`
displays the latest recorded information for the resource in the Status and
Events sections:

```console
...
Status:
  Conditions:
    Last Transition Time:  2023-12-06T18:23:21Z
    Message:               Failed to install after 1 attempt(s)
    Observed Generation:   1
    Reason:                RetriesExceeded
    Status:                True
    Type:                  Stalled
    Last Transition Time:  2023-12-06T18:23:21Z
    Message:               Helm test failed for release podinfo/podinfo.v1 with chart podinfo@6.5.3: 1 error occurred:
                           * pod podinfo-fault-test-a0tew failed
    Observed Generation:   1
    Reason:                TestFailed
    Status:                False
    Type:                  Ready
    Last Transition Time:  2023-12-06T18:23:16Z
    Message:               Helm install succeeded for release podinfo/podinfo.v1 with chart podinfo@6.5.3
    Observed Generation:   1
    Reason:                InstallSucceeded
    Status:                True
    Type:                  Released
    Last Transition Time:  2023-12-06T18:23:21Z
    Message:               Helm test failed for release podinfo/podinfo.v1 with chart podinfo@6.5.3: 1 error occurred:
                           * pod podinfo-fault-test-a0tew failed
    Observed Generation:   1
    Reason:                TestFailed
    Status:                False
    Type:                  TestSuccess
...
  History:
    Chart Name:      podinfo
    Chart Version:   6.5.3
    Config Digest:   sha256:2598fd0e8c65bae746c6686a61c2b2709f47ba8ed5c36450ae1c30aea9c88e9f
    Digest:          sha256:24f31c6f2f3da97b217a794b5fb9234818296c971ff9f849144bf07438976e4d
    First Deployed:  2023-12-06T18:23:12Z
    Last Deployed:   2023-12-06T18:23:12Z
    Name:            podinfo
    Namespace:       default
    Status:          deployed
    Test Hooks:
      podinfo-fault-test-a0tew:
        Last Completed:  2023-12-06T18:23:21Z
        Last Started:    2023-12-06T18:23:16Z
        Phase:           Failed
      podinfo-grpc-test-rzg5v:
      podinfo-jwt-test-7k1hv:
      Podinfo - Service - Test - Bgoeg:
    Version:                      1
...
Events:
  Type     Reason            Age   From             Message
  ----     ------            ----  ----             -------
  Normal   HelmChartCreated  88s   helm-controller  Created HelmChart/podinfo/podinfo-podinfo with SourceRef 'HelmRepository/podinfo/podinfo'
  Normal   HelmChartInSync   88s   helm-controller  HelmChart/podinfo/podinfo-podinfo with SourceRef 'HelmRepository/podinfo/podinfo' is in-sync
  Normal   InstallSucceeded  83s   helm-controller  Helm install succeeded for release podinfo/podinfo.v1 with chart podinfo@6.5.3
  Warning  TestFailed        78s   helm-controller  Helm test failed for release podinfo/podinfo.v1 with chart podinfo@6.5.3: 1 error occurred:
           * pod podinfo-fault-test-a0tew failed
```

#### Trace emitted Events

To view events for specific HelmRelease(s), `kubectl events` can be used in
combination with `--for` to list the Events for specific objects. For example,
running

```shell
kubectl events --for HelmRelease/<release-name>
```

lists

```shell
LAST SEEN   TYPE      REASON             OBJECT                MESSAGE
88s         Normal    HelmChartCreated   HelmRelease/podinfo   Created HelmChart/podinfo/podinfo-podinfo with SourceRef 'HelmRepository/podinfo/podinfo'
88s         Normal    HelmChartInSync    HelmRelease/podinfo   HelmChart/podinfo/podinfo-podinfo with SourceRef 'HelmRepository/podinfo/podinfo' is in-sync
83s         Normal    InstallSucceeded   HelmRelease/podinfo   Helm install succeeded for release podinfo/podinfo.v1 with chart podinfo@6.5.3
78s         Warning   TestFailed         HelmRelease/podinfo   Helm test failed for release podinfo/podinfo.v1 with chart podinfo@6.5.3: 1 error occurred:
                                                               * pod podinfo-fault-test-a0tew failed
```

Besides being reported in Events, the controller may also log reconciliation
errors. The Flux CLI offers commands for filtering the logs for a specific
HelmRelease, e.g. `flux logs --level=error --kind=HelmRelease --name=<release-name>.`

#### Rendering the final Values locally

When using multiple [values references](#values-references) in a
HelmRelease, it can be useful to inspect the final values computed from the various sources.
This can be done by pointing the Flux CLI to the in-cluster HelmRelease object:

```shell
flux debug hr <release-name> -n <namespace> --show-values
```

The command will output the final values by merging the in-line values from the HelmRelease
with the values from the referenced ConfigMaps and/or Secrets.

**Note:** The debug command will print sensitive information if Kubernetes Secrets
are referenced in the HelmRelease `.spec.valuesFrom` field, so exercise caution
when using this command.

### Reacting immediately to configuration dependencies

To trigger a Helm release upgrade when changes occur in referenced
Secrets or ConfigMaps, you can set the following label on the
Secret or ConfigMap:

```yaml
metadata:
  labels:
    reconcile.fluxcd.io/watch: Enabled
```

An alternative to labeling every Secret or ConfigMap is
setting the `--watch-configs-label-selector=owner!=helm`
[flag](https://fluxcd.io/flux/components/helm/options/#flags)
in helm-controller, which allows watching all Secrets and
ConfigMaps except for Helm storage Secrets.

**Note**: An upgrade will be triggered for an event on a referenced
Secret/ConfigMap even if it's marked as optional in the `.spec.valuesFrom`
field, including deletion events.

## HelmRelease Status

### Events

The controller emits Kubernetes Events to report the result of each Helm action
performed for a HelmRelease. These events can be used to monitor the progress
of the HelmRelease and can be forwarded to external systems using 
[notification-controller alerts](https://fluxcd.io/flux/monitoring/alerts/).

The controller annotates the events with the Helm chart version, app version,
and with the chart OCI digest if available.

#### Event example

```yaml
apiVersion: v1
kind: Event
metadata:
  annotations:
    helm.toolkit.fluxcd.io/app-version: 6.6.1
    helm.toolkit.fluxcd.io/revision: 6.6.1+0cc9a8446c95
    helm.toolkit.fluxcd.io/oci-digest: sha256:0cc9a8446c95009ef382f5eade883a67c257f77d50f84e78ecef2aac9428d1e5
  creationTimestamp: "2024-05-07T05:02:34Z"
  name: podinfo.17cd1c4e15d474bb
  namespace: default
firstTimestamp: "2024-05-07T05:02:34Z"
involvedObject:
  apiVersion: helm.toolkit.fluxcd.io/v2
  kind: HelmRelease
  name: podinfo
  namespace: default
lastTimestamp: "2024-05-07T05:02:34Z"
message: 'Helm test succeeded for release podinfo/podinfo.v2 with chart podinfo@6.6.1+0cc9a8446c95:
  3 test hooks completed successfully'
reason: TestSucceeded
source:
  component: helm-controller
type: Normal
```

### History

The HelmRelease shows the history of Helm releases it has performed up to the
previous successful release as a list in the `.status.history` of the resource.
The history is ordered by the time of the release, with the most recent release
first.

When [Helm tests](#test-configuration) are enabled, the history will also
include the status of the tests which were run for each release.

#### History example

```yaml
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: <release-name>
status:
  history:
    - appVersion: 6.6.1
      chartName: podinfo
      chartVersion: 6.6.1+0cc9a8446c95
      configDigest: sha256:e15c415d62760896bd8bec192a44c5716dc224db9e0fc609b9ac14718f8f9e56
      digest: sha256:e59349a6d8cf01d625de9fe73efd94b5e2a8cc8453d1b893ec367cfa2105bae9
      firstDeployed: "2024-05-07T04:54:21Z"
      lastDeployed: "2024-05-07T04:54:55Z"
      name: podinfo
      namespace: podinfo
      ociDigest: sha256:0cc9a8446c95009ef382f5eade883a67c257f77d50f84e78ecef2aac9428d1e5
      status: deployed
      testHooks:
        podinfo-grpc-test-goyey:
          lastCompleted: "2024-05-07T04:55:11Z"
          lastStarted: "2024-05-07T04:55:09Z"
          phase: Succeeded
      version: 2
    - appVersion: 6.6.0
      chartName: podinfo
      chartVersion: 6.6.0+cdd538a0167e
      configDigest: sha256:e15c415d62760896bd8bec192a44c5716dc224db9e0fc609b9ac14718f8f9e56
      digest: sha256:9be0d34ced6b890a72026749bc0f1f9e3c1a89673e17921bbcc0f27774f31c3a
      firstDeployed: "2024-05-07T04:54:21Z"
      lastDeployed: "2024-05-07T04:54:21Z"
      name: podinfo
      namespace: podinfo
      ociDigest: sha256:cdd538a0167e4b51152b71a477e51eb6737553510ce8797dbcc537e1342311bb
      status: superseded
      testHooks:
        podinfo-grpc-test-q0ucx:
          lastCompleted: "2024-05-07T04:54:25Z"
          lastStarted: "2024-05-07T04:54:23Z"
          phase: Succeeded
      version: 1
```

### Inventory

The HelmRelease reports the list of Kubernetes resource objects that have been
applied by the Helm release in `.status.inventory`. This can be used to
identify which objects are managed by the HelmRelease. The inventory records
are in the format `<namespace>_<name>_<group>_<kind>`.

The inventory includes all resources from the rendered manifests, as well as
CRDs from the chart's `crds/` directory. Helm hooks are not included in the
inventory, as they are not considered part of the release by Helm.

#### Inventory example

```yaml
---
apiVersion: helm.toolkit.fluxcd.io/v2
kind: HelmRelease
metadata:
  name: <release-name>
status:
  inventory:
    entries:
      - id: default_podinfo__Service
        v: v1
      - id: default_podinfo_apps_Deployment
        v: v1
      - id: default_podinfo_autoscaling_HorizontalPodAutoscaler
        v: v2
```

### Conditions

A HelmRelease enters various states during its lifecycle, reflected as
[Kubernetes Conditions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties).
It can be [reconciling](#reconciling-helmrelease) when it is being processed by
the controller, it can be [ready](#ready-helmrelease) when the Helm release is
installed and up-to-date, or it can [fail](#failed-helmrelease) during
reconciliation.

The HelmRelease API is compatible with the [kstatus specification](https://github.com/kubernetes-sigs/cli-utils/tree/master/pkg/kstatus),
and reports `Reconciling` and `Stalled` conditions where applicable to provide
better (timeout) support to solutions polling the HelmRelease to become `Ready`.

#### Reconciling HelmRelease

The helm-controller marks the HelmRepository as _reconciling_ when it is working
on re-assessing the Helm release state, or working on a Helm action such as
installing or upgrading the release.

This can be due to one of the following reasons (without this being an
exhaustive list):

- The desired state of the HelmRelease has changed, and the controller is
  working on installing or upgrading the Helm release.
- The generation of the HelmRelease is newer than the [Observed
  Generation](#observed-generation).
- The HelmRelease has been installed or upgraded, but the [Helm
  test](#test-configuration) is still running.
- The HelmRelease is installed or upgraded, but the controller is working on
  [detecting](#drift-detection) or [correcting](#drift-correction) drift.

When the HelmRelease is "reconciling", the `Ready` Condition status becomes
`Unknown` when the controller is working on a Helm install or upgrade, and the
controller adds a Condition with the following attributes to the HelmRelease's
`.status.conditions`:

- `type: Reconciling`
- `status: "True"`
- `reason: Progressing` | `reason: ProgressingWithRetry`

The Condition `message` is updated during the course of the reconciliation to
report the Helm action being performed at any particular moment.

The Condition has a ["negative polarity"](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties),
and is only present on the HelmRelease while the status is `"True"`.

#### Ready HelmRelease

The helm-controller marks the HelmRelease as _ready_ when it has the following
characteristics:

- The Helm release is installed and up-to-date. This means that the Helm
  release has been installed or upgraded, the release's chart has the same
  version as the [Helm chart referenced by the HelmRelease](#chart-template),
  and the [values](#values) used to install or upgrade the release have not
  changed.
- The Helm release has passed any [Helm tests](#test-configuration) that are
  enabled.
- The HelmRelease is not being [reconciled](#reconciling-helmrelease).

When the HelmRelease is "ready", the controller sets a Condition with the
following attributes in the HelmRelease's `.status.conditions`:

- `type: Ready`
- `status: "True"`
- `reason: InstallSucceeded` | `reason: UpgradeSucceeded` | `reason: TestSucceeded`

This `Ready` Condition will retain a status value of `"True"` until the
HelmRelease is marked as reconciling, or e.g. an [error occurs](#failed-helmrelease)
due to a failed Helm action.

When a Helm install or upgrade has completed, the controller sets a Condition
with the following attributes in the HelmRelease's `.status.conditions`:

- `type: Released`
- `status: "True"`
- `reason: InstallSucceeded` | `reason: UpgradeSucceeded`

The `Released` Condition will retain a status value of `"True"` until the
next Helm install or upgrade has completed.

When [Helm tests are enabled](#test-configuration) and completed successfully,
the controller sets a Condition with the following attributes in the
HelmRelease's `.status.conditions`:

- `type: TestSuccess`
- `status: "True"`
- `reason: TestSucceeded`

The `TestSuccess` Condition will retain a status value of `"True"` until the
next Helm install or upgrade occurs, or the Helm tests are disabled.

#### Failed HelmRelease

The helm-controller may get stuck trying to determine state or produce a Helm
release without completing. This can occur due to some of the following factors:

- The HelmChart does not have an Artifact, or is not ready.
- The HelmRelease's dependencies are not ready.
- The composition of [values references](#values-references) and [inline values](#inline-values)
  failed due to a misconfiguration.
- The Helm action (install, upgrade, rollback, uninstall) failed.
- The Helm action succeeded, but the [Helm test](#test-configuration) failed.

When the failure is due to an error during a Helm install or upgrade, a
Condition with the following attributes is added:

- `type: Released`
- `status: "False"`
- `reason: InstallFailed` | `reason: UpgradeFailed`

In case the failure is due to an error during a Helm test, a Condition with the
following attributes is added:

- `type: TestSuccess`
- `status: "False"`
- `reason: TestFailed`

This `TestSuccess` Condition will only count as a failure when the Helm test
results have [not been ignored](#configuring-failure-handling).

When the failure has resulted in a rollback or uninstall, a Condition with the
following attributes is added:

- `type: Remediated`
- `status: "True"`
- `reason: RollbackSucceeded` | `reason: UninstallSucceeded` | `reason: RollbackFailed` | `reason: UninstallFailed`

This `Remediated` Condition will retain a status value of `"True"` until the
next Helm install or upgrade has completed.

When the HelmRelease is "failing", the controller sets a Condition with the
following attributes in the HelmRelease's `.status.conditions`:

- `type: Ready`
- `status: "False"`
- `reason: InstallFailed` | `reason: UpgradeFailed` | `reason: TestFailed` | `reason: RollbackSucceeded` | `reason: UninstallSucceeded` | `reason: RollbackFailed` | `reason: UninstallFailed` | `reason: <arbitrary error>`

Note that a HelmRelease can be [reconciling](#reconciling-helmrelease) while
failing at the same time. For example, due to a new release attempt after
remediating a failed Helm action. When a reconciliation fails, the `Reconciling`
Condition reason would be `ProgressingWithRetry`. When the reconciliation is
performed again after the failure, the reason is updated to `Progressing`.

### Storage Namespace

The helm-controller reports the active storage namespace in the
`.status.storageNamespace` field.

When the [`.spec.storageNamespace`](#storage-namespace) is changed, the
controller will use the namespace from the Status to perform a Helm uninstall
for the release in the old storage namespace, before performing a Helm install
using the new storage namespace.

### Failure Counters

The helm-controller reports the number of failures it encountered for a
HelmRelease in the `.status.failures`, `.status.installFailures` and
`.status.upgradeFailures` fields.

The `.status.failures` field is a general counter for all failures, while the
`.status.installFailures` and `.status.upgradeFailures` fields are counters
which are specific to the Helm install and upgrade actions respectively.
The latter two counters are used to determine if the controller is allowed to
retry an action when [install](#install-remediation) or [upgrade](#upgrade-remediation)
remediation is enabled.

The counters are reset when a new configuration is applied to the HelmRelease,
the [values](#values) change, or when a new Helm chart version is discovered.
In addition, they can be [reset using an annotation](#resetting-remediation-retries).

### Observed Generation

The helm-controller reports an observed generation in the HelmRelease's
`.status.observedGeneration`. The observed generation is the latest
`.metadata.generation` which resulted in either a [ready state](#ready-helmrelease),
or stalled due to error it can not recover from without human intervention.

### Observed Post Renderers Digest

The helm-controller reports the digest for the [post renderers](#post-renderers)
it last rendered the Helm chart with in the for a successful Helm install or
upgrade in the `.status.observedPostRenderersDigest` field.

This field is used by the controller to determine if a deployed Helm release
is in sync with the HelmRelease `spec.postRenderers` configuration and whether
it should trigger a Helm upgrade.

### Last Attempted Config Digest

The helm-controller reports the digest for the [values](#values) it last
attempted to perform a Helm install or upgrade with in the
`.status.lastAttemptedConfigDigest` field.

The digest is used to determine if the controller should reset the
[failure counters](#failure-counters) due to a change in the values.

### Last Attempted Revision

The helm-controller reports the revision of the Helm chart it last attempted
to perform a Helm install or upgrade with in the
`.status.lastAttemptedRevision` field.

The revision is used by the controller to determine if it should reset the
[failure counters](#failure-counters) due to a change in the chart version.

### Last Attempted Revision Digest

The helm-controller reports the OCI artifact digest of the Helm chart it last attempted
to perform a Helm install or upgrade with in the
`.status.lastAttemptedRevisionDigest` field.

This field is present in status only when `.spec.chartRef.type` is set to `OCIRepository`.

### Last Attempted Release Action

The helm-controller reports the last Helm release action it attempted to
perform in the `.status.lastAttemptedReleaseAction` field. The possible values
are `install` and `upgrade`.

This field is used by the controller to determine the active remediation
strategy for the HelmRelease.

### Last Attempted Release Action Duration

The helm-controller reports the duration of the last Helm release action it
attempted to perform in the `.status.lastAttemptedReleaseActionDuration` field.

### Last Handled Reconcile At

The helm-controller reports the last `reconcile.fluxcd.io/requestedAt`
annotation value it acted on in the `.status.lastHandledReconcileAt` field.

For practical information about this field, see
[triggering a reconcile](#triggering-a-reconcile).

### Last Handled Force At

The helm-controller reports the last `reconcile.fluxcd.io/forceAt`
annotation value it acted on in the `.status.lastHandledForceAt` field.

For practical information about this field, see
[forcing a release](#forcing-a-release).

### Last Handled Reset At

The helm-controller reports the last `reconcile.fluxcd.io/resetAt`
annotation value it acted on in the `.status.lastHandledResetAt` field.

For practical information about this field, see
[resetting remediation retries](#resetting-remediation-retries).
