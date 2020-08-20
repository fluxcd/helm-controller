# Changelog

All notable changes to this project are documented in this file.

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
