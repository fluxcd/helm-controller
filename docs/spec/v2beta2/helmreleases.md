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
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmRepository
metadata:
  name: podinfo
  namespace: default
spec:
  interval: 5m
  url: https://stefanprodan.github.io/podinfo
---
apiVersion: helm.toolkit.fluxcd.io/v2beta2
kind: HelmRelease
metadata:
  name: podinfo
  namespace: default
spec:
  interval: 10m
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

`.spec.chartRef` is an optional field used to refer to an [OCIRepository resource](https://fluxcd.io/flux/components/source/ocirepositories/) or a [HelmChart resource](https://fluxcd.io/flux/components/source/helmcharts/)
from which to fetch the Helm chart. The chart is fetched by the controller with the
information provided by `.status.artifact` of the referenced resource.

For a referenced resource of `kind OCIRepository`, the chart version of the last
release attempt is reported in `.status.lastAttemptedRevision`. The version is in
the format `<version>+<digest[0:12]>`. The digest of the OCI artifact is appended
to the version to ensure that a change in the artifact content triggers a new release.
The controller will automatically perform a Helm upgrade when the `OCIRepository`
detects a new digest in the OCI artifact stored in registry, even if the version
inside `Chart.yaml` is unchanged.

**Warning:** One of `.spec.chart` or `.spec.chartRef` must be set, but not both.
When switching from `.spec.chart` to `.spec.chartRef`, the controller will perform
an Helm upgrade and will garbage collect the old HelmChart object.

**Note:** On multi-tenant clusters, platform admins can disable cross-namespace
references with the `--no-cross-namespace-refs=true` controller flag. When this flag is
set, the HelmRelease can only refer to OCIRepositories in the same namespace as the
HelmRelease object.

#### OCIRepository reference example

```yaml
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: OCIRepository
metadata:
  name: podinfo
  namespace: default
spec:
  interval: 30s
  url: oci://ghcr.io/stefanprodan/charts/podinfo
  ref:
    tag: 6.6.0
---
apiVersion: helm.toolkit.fluxcd.io/v2beta2
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
apiVersion: source.toolkit.fluxcd.io/v1beta2
kind: HelmChart
metadata:
  name: podinfo
  namespace: default
spec:
  interval: 5m0s
  chart: podinfo
  reconcileStrategy: ChartVersion
  sourceRef:
    kind: HelmRepository
    name: podinfo
  version: '5.*'
---
apiVersion: helm.toolkit.fluxcd.io/v2beta2
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
apiVersion: helm.toolkit.fluxcd.io/v2beta2
kind: HelmRelease
metadata:
  name: backend
  namespace: default
spec:
  # ...omitted for brevity   
---
apiVersion: helm.toolkit.fluxcd.io/v2beta2
kind: HelmRelease
metadata:
  name: frontend
  namespace: default
spec:
  # ...omitted for brevity
  dependsOn:
    - name: backend
```

**Note:** This does not account for upgrade ordering. Kubernetes only allows
applying one resource (HelmRelease in this case) at a time, so there is no
way for the controller to know when a dependency HelmRelease may be updated.
Also, circular dependencies between HelmRelease resources must be avoided,
otherwise the interdependent HelmRelease resources will never be reconciled.

### Values

The values for the Helm release can be specified in two ways:

- [Values references](#values-references)
- [Inline values](#inline-values)

Changes to the combined values will trigger a new Helm release.

#### Values references

`.spec.valuesFrom` is an optional list to refer to ConfigMap and Secret
resources from which to take values. The values are merged in the order given,
with the later values overwriting earlier, and then [inline values](#inline-values)
overwriting those.

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
- `.disableWait` (Optional): Disables waiting for resources to be ready after
  the installation of the chart. Defaults to `false`.
- `.disableWaitForJobs` (Optional): Disables waiting for any Jobs to complete
  after the installation of the chart. Defaults to `false`.

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
- `.disableWait` (Optional): Disables waiting for resources to be ready after
  upgrading the release. Defaults to `false`.
- `.disableWaitForJobs` (Optional): Disables waiting for any Jobs to complete
  after upgrading the release. Defaults to `false`.
- `.force` (Optional): Forces resource updates through a replacement strategy.
  Defaults to `false`.
- `.preserveValues` (Optional): Instructs Helm to re-use the values from the
  last release while merging in overrides from [values](#values). Setting
  this flag makes the HelmRelease non-declarative. Defaults to `false`.

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
  `false`.

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

### KubeConfig reference

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
of helm-controller.

When both `.spec.kubeConfig` and a [Service Account reference](#service-account-reference)
are specified, the controller will impersonate the Service Account on the
target cluster.

The Helm storage is stored on the remote cluster in a namespace that equals to
the namespace of the HelmRelease, or the [configured storage namespace](#storage-namespace).
The release itself is made in a namespace that equals to the namespace of the
HelmRelease, or the [configured target namespace](#target-namespace). The
namespaces are expected to exist, with the exception that the target namespace
can be created on demand by Helm when namespace creation is [configured during
install](#install-configuration).

Other references to Kubernetes resources in the HelmRelease, like [values
references](#values-references), are expected to exist on the reconciling
cluster.

### Interval

`.spec.interval` is a required field that specifies the interval at which the
HelmRelease is reconciled, i.e. the controller ensures the current Helm release
matches the desired state.

After successfully reconciling the object, the controller requeues it for
inspection at the specified interval. The value must be in a [Go recognized
duration string format](https://pkg.go.dev/time#ParseDuration), e.g. `10m0s`
to reconcile the object every ten minutes.

If the `.metadata.generation` of a resource changes (due to e.g. a change to
the spec) or the HelmChart revision changes (which generates a Kubernetes
Event), this is handled instantly outside the interval window.

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
apiVersion: helm.toolkit.fluxcd.io/v2beta2
kind: HelmRelease
metadata:
  name: my-operator
  namespace: default
spec:
  interval: 10m
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
apiVersion: helm.toolkit.fluxcd.io/v2beta2
kind: HelmRelease
metadata:
 name: podinfo
 namespace: webapp
spec:
 serviceAccountName: webapp-reconciler
 interval: 5m
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

### Remote clusters / Cluster-API

Using a [`.spec.kubeConfig` reference](#kubeconfig-reference), it is possible
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
apiVersion: helm.toolkit.fluxcd.io/v2beta2
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
apiVersion: helm.toolkit.fluxcd.io/v2beta2
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
apiVersion: helm.toolkit.fluxcd.io/v2beta2
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

## HelmRelease Status

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
apiVersion: helm.toolkit.fluxcd.io/v2beta2
kind: HelmRelease
metadata:
  name: <release-name>
status:
  history:
    - chartName: podinfo
      chartVersion: 6.5.3
      configDigest: sha256:803f06d4673b07668ff270301ca54ca5829da3133c1219f47bd9f52a60b22f9f
      digest: sha256:3036cf7c06fd35b8ccb15c426fed9ce8a059a0a4befab1a47170b6e962c4d784
      firstDeployed: '2023-12-06T20:38:47Z'
      lastDeployed: '2023-12-06T20:52:06Z'
      name: podinfo
      namespace: podinfo
      status: deployed
      testHooks:
        podinfo-grpc-test-qulpw:
          lastCompleted: '2023-12-06T20:52:09Z'
          lastStarted: '2023-12-06T20:52:07Z'
          phase: Succeeded
        podinfo-jwt-test-xe0ch:
          lastCompleted: '2023-12-06T20:52:12Z'
          lastStarted: '2023-12-06T20:52:09Z'
          phase: Succeeded
        podinfo-service-test-eh6x2:
          lastCompleted: '2023-12-06T20:52:14Z'
          lastStarted: '2023-12-06T20:52:12Z'
          phase: Succeeded
      version: 3
    - chartName: podinfo
      chartVersion: 6.5.3
      configDigest: sha256:e15c415d62760896bd8bec192a44c5716dc224db9e0fc609b9ac14718f8f9e56
      digest: sha256:858b157a63889b25379e287e24a9b38beb09a8ae21f31ae2cf7ad53d70744375
      firstDeployed: '2023-12-06T20:38:47Z'
      lastDeployed: '2023-12-06T20:39:02Z'
      name: podinfo
      namespace: podinfo
      status: superseded
      testHooks:
        podinfo-grpc-test-aiuee:
          lastCompleted: '2023-12-06T20:39:04Z'
          lastStarted: '2023-12-06T20:39:02Z'
          phase: Succeeded
        podinfo-jwt-test-dme3b:
          lastCompleted: '2023-12-06T20:39:07Z'
          lastStarted: '2023-12-06T20:39:04Z'
          phase: Succeeded
        podinfo-service-test-fgvte:
          lastCompleted: '2023-12-06T20:39:09Z'
          lastStarted: '2023-12-06T20:39:07Z'
          phase: Succeeded
      version: 2
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

### Last Attempted Release Action

The helm-controller reports the last Helm release action it attempted to
perform in the `.status.lastAttemptedReleaseAction` field. The possible values
are `install` and `upgrade`.

This field is used by the controller to determine the active remediation
strategy for the HelmRelease.

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
