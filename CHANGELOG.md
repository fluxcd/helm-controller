# Changelog

All notable changes to this project are documented in this file.

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
[helm.fluxcd.io/v2alpha1](https://github.com/fluxcd/helm-controller/tree/master/docs/spec/v2alpha1) API.
