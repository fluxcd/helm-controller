# helm.toolkit.fluxcd.io/v2alpha1

This is the v2alpha1 API specification for declaratively managing Helm chart releases with
Kubernetes manifests.

## Specification

- [`HelmRelease` CRD](helmreleases.md)
    + [Source reference](helmreleases.md#source-reference)
    + [Reconciliation](helmreleases.md#reconciliation)
    + [`HelmRelease` dependencies](helmreleases.md#helmrelease-dependencies)
    + [Configuring failure remediation](helmreleases.md#configuring-failure-remediation)
    + [Enabling Helm rollback actions](helmreleases.md#enabling-helm-rollback-actions)
    + [Enabling Helm test actions](helmreleases.md#configuring-helm-test-actions)
    + [Status](helmreleases.md#status)

## Implementation

* [helm-controller](https://github.com/fluxcd/helm-controller/)
