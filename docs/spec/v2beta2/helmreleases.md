# Helm Releases

The `HelmRelease` API defines a way to declaratively manage the lifecycle of
Helm releases.

## Example

The following is an example of a HelmRelease.

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

- ...

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

`.spec.chart` is a required field used by the helm-controller as a template to
create a new [HelmChart resource](https://fluxcd.io/flux/components/source/helmcharts/).

The spec for the HelmChart is provided via `.spec.chart.spec`, refer to
[writing a HelmChart spec](https://fluxcd.io/flux/components/source/helmcharts/#writing-a-helmchart-spec)
for in-depth information.

Annotations and labels can be added by configuring the respective
`.spec.chart.metadata` fields.

The HelmChart is created in the same namespace as the `.sourceRef`, with a name
matching the HelmRelease's `<.metadata.namespace>-<.metadata.name>`, and will
be reported in `.status.helmChart`.

**Note:** On multi-tenant clusters, platform admins can disable cross-namespace
references with the `--no-cross-namespace-refs=true` flag. When this flag is
set, the HelmRelease can only refer to Sources in the same namespace as the
HelmRelease object.

### Release name

`.spec.releaseName` is an optional field used to specify the name of the Helm
release. It defaults to a composition of `[<target namespace>-]<name>`.

**Note:** When the composition exceeds the maximum length of 53 characters, the
name is shortened by hashing the release name with SHA-256. The resulting name
is then composed of the first 40 characters of the release name, followed by a
dash (`-`), followed by the 12 characters of the hash. For example,
`a-very-lengthy-target-namespace-with-a-nice-object-name` becomes
`a-very-lengthy-target-namespace-with-a-nic-97af5d7f41f3`.

### Target namespace

`.spec.targetNamespace` is an optional field used to specify the namespace to
which the Helm release is made. It defaults to the namespace of the
HelmRelease.

### Storage namespace

`.spec.storageNamespace` is an optional field used to specify the namespace
in which Helm stores release information. It defaults to the namespace of the
HelmRelease.

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
  Valid values are `Skip`, `Create` and `CreateReplace`. Default is `None`.
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
  this flag makes the HelmRelease non-declarative.

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
`true`.

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
 `.disableHooks` (Optional): Prevents [chart hooks](https://helm.sh/docs/topics/charts_hooks/)
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
- `.deletionPropagation` (Optional):
 `.disableHooks` (Optional): Prevents [chart hooks](https://helm.sh/docs/topics/charts_hooks/)
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
the storage. The controller will compare the manifest from the Helm storage
with the current state of the cluster using a
[server-side dry-run apply](https://kubernetes.io/docs/reference/using-api/server-side-apply/).

If this comparison detects a drift (either due to a resource being created
or modified during the dry-run), the controller will emit a Kubernetes Event
with a short summary of the detected changes. In addition, a more extensive
[JSON Patch](https://datatracker.ietf.org/doc/html/rfc6902) summary is logged
to the controller logs (with `--log-level=debug`).

#### Drift correction

When `.spec.driftDetection.mode` is set to `enabled`, the controller will
attempt to correct the drift by creating and patching the resources based on
the server-side dry-run result.

At the end of the correction attempt, it will emit a Kubernetes Event with a
summary of the changes it made and any failures it encountered. In case of a
failure, it will continue to detect and correct drift until the desired state
has been reached, or a new Helm action is triggered (due to e.g. a change to
the spec).

#### Ignore rules

`.spec.driftDetection.ignore` is an optional field to provide
[JSON Pointers](https://datatracker.ietf.org/doc/html/rfc6901) to ignore while
detecting and correcting drift. This can for example be easeful when Horizontal
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

### The reconciliation model

### Controlling the lifecycle of Custom Resource Definitions

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
upgrade, it can be annotated with `reconcile.fluxcd.io/forceAt: <arbitrary value>`
while simultaneously [triggering a reconcile](#triggering-a-reconcile) with the
same value.

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

#### Trace emitted Events

## HelmRelease Status

### Conditions

### History
