# helm.toolkit.fluxcd.io/v2beta1

This is the v2beta1 API specification for declaratively managing Helm chart releases with
Kubernetes manifests.

## Specification

- [`HelmRelease` CRD](helmreleases.md)
    + [Specification](helmreleases.md#specification)
        * [Reference types](helmreleases.md#reference-types)
        * [Status specification](helmreleases.md#status-specification)
    + [Helm release placement](helmreleases.md#helm-release-placement)
    + [Helm chart template](helmreleases.md#helm-chart-template)
    + [Values overrides](helmreleases.md#values-overrides)
    + [Reconciliation](helmreleases.md#reconciliation)
        * [Disabling resource waiting](helmreleases.md#disabling-resource-waiting)
        * [`HelmRelease` dependencies](helmreleases.md#helmrelease-dependencies)
        * [Configuring Helm test actions](helmreleases.md#configuring-helm-test-actions)
        * [Configuring failure remediation](helmreleases.md#configuring-failure-remediation)
    + [Post Renders](helmreleases.md#post-renderers)
    + [Status](helmreleases.md#status)

## Implementation

* [helm-controller](https://github.com/fluxcd/helm-controller/)
