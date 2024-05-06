# helm.toolkit.fluxcd.io/v2

This is the v2 API specification for declaratively managing Helm chart
releases with Kubernetes manifests.

## Specification

- [HelmRelease CRD](helmreleases.md)
  + [Example](helmreleases.md#example)
  + [Writing a HelmRelease spec](helmreleases.md#writing-a-helmrelease-spec)
  + [Working with HelmReleases](helmreleases.md#working-with-helmreleases)
  + [HelmRelease Status](helmreleases.md#helmrelease-status)

## Implementation

* [helm-controller](https://github.com/fluxcd/helm-controller/)
