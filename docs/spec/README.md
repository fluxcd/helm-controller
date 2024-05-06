# Helm Controller

The Helm Controller is a [Kubernetes operator](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/),
allowing one to declaratively manage Helm chart releases with Kubernetes manifests.

## Motivation

The main goal is to provide an automated operator that can perform Helm actions (e.g.
install, upgrade, uninstall, rollback, test) and continuously reconcile the state of Helm releases.

When provisioning a new cluster, one may wish to install Helm releases in a specific order, for
example because one relies on a service mesh admission controller managed by a `HelmRelease` and
the proxy injector must be functional before deploying applications into the mesh, or when
[Custom Resource Definitions are managed in a separate chart](https://helm.sh/docs/chart_best_practices/custom_resource_definitions/#method-2-separate-charts)
and this chart needs to be installed first.

When dealing with an incident, one may wish to suspend the reconciliation of some Helm releases,
without having to stop the reconciler and affect the whole cluster.

When operating a cluster, different teams may wish to receive notifications about the status of
their Helm releases. For example, the on-call team would receive alerts about all failures in
the prod namespace, while the frontend team may wish to be alerted when a new version of the
frontend chart was released, no matter the namespace.

## Design

The reconciliation process can be defined with a Kubernetes custom resource called `HelmRelease`
that describes the Helm release, a `HelmChart` template, and the configuration for a set of Helm
actions that should be (conditionally) executed. Based on this the reconciler:

- reconciles a `HelmChart` based on the provided template
- confirms the chart artifact is available
- checks if all depends-on conditions are meet
- fetches the available chart artifact
- performs a Helm install or upgrade action if needed
- performs a Helm test action if enabled
- performs a reconciliation strategy (rollback, uninstall) and retries as configured if any Helm action failed
- performs in cluster drift detection and correction if enabled

The controller that runs these Helm actions relies on [source-controller](https://github.com/fluxcd/source-controller)
for providing the Helm charts from Helm repositories or any other source that source-controller
could support in the future.

Reconciliation of the `HelmRelease` runs on-a-schedule and can be triggered manually by a
cluster admin, or automatically by a source event such as a Helm chart revision change.

When a custom resource is deleted from the cluster, the controller's garbage collector removes
the associated `HelmChart` and uninstalls the Helm release. Deleting a suspended resource does not
trigger a Helm uninstall.

Alerting can be configured with a Kubernetes custom resource that specifies a webhook address, and a
group of `HelmRelease` resources to be monitored using the [notification-controller](https://github.com/fluxcd/notification-controller).

The API design of the controller can be found at [helm.toolkit.fluxcd.io/v2](./v2/helmreleases.md).

## Backward compatibility

| Feature                                                             | Helm Controller          | Helm Operator      |
| ------------------------------------------------------------------- | ------------------------ | ------------------ |
| Helm install, upgrade, test, rollback, uninstall                    | :heavy_check_mark:       | :heavy_check_mark: |
| Extensive control over configuration of individual actions          | :heavy_check_mark:       | :x:                |
| Helm charts from Helm repositories                                  | :heavy_check_mark:       | :heavy_check_mark: |
| Helm charts from Git repositories                                   | :heavy_check_mark:       | :heavy_check_mark: |
| Helm uninstall on resource removal                                  | :heavy_check_mark:       | :heavy_check_mark: |
| Conditional run of actions (i.e. rolling back after test failure)   | :heavy_check_mark:       | :x:                |
| Helm plugins                                                        | :x:                      | :heavy_check_mark: |
| Container image updates                                             | :x:                      | :heavy_check_mark: |
| Automated chart updates based on semver ranges                      | :heavy_check_mark:       | :x:                |
