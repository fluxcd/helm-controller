# helm-controller

[![CII Best Practices](https://bestpractices.coreinfrastructure.org/projects/4784/badge)](https://bestpractices.coreinfrastructure.org/projects/4784)
[![e2e](https://github.com/fluxcd/helm-controller/workflows/e2e/badge.svg)](https://github.com/fluxcd/helm-controller/actions)
[![report](https://goreportcard.com/badge/github.com/fluxcd/helm-controller)](https://goreportcard.com/report/github.com/fluxcd/helm-controller)
[![license](https://img.shields.io/github/license/fluxcd/helm-controller.svg)](https://github.com/fluxcd/helm-controller/blob/main/LICENSE)
[![release](https://img.shields.io/github/release/fluxcd/helm-controller/all.svg)](https://github.com/fluxcd/helm-controller/releases)

The helm-controller is a Kubernetes operator, allowing one to declaratively
manage Helm chart releases. It is part of a composable [GitOps toolkit](https://fluxcd.io/flux/components)
and depends on [source-controller][] to acquire the Helm charts from Helm
repositories.

The desired state of a Helm release is described through a Kubernetes Custom
Resource named `HelmRelease`. Based on the creation, mutation or removal of a
`HelmRelease` resource in the cluster, Helm actions are performed by the
operator.

![overview](docs/diagrams/helm-controller-overview.png)

## Features

* Watches for `HelmRelease` objects and generates `HelmChart` objects
* Supports `HelmChart` artifacts produced from `HelmRepository`,
  `GitRepository` and `Bucket` sources
* Fetches artifacts produced by [source-controller][] from `HelmChart`
  and `OCIRepository` objects
* Watches `HelmChart` objects for revision changes (including semver
  ranges for charts from `HelmRepository` sources)
* Performs automated Helm actions, including Helm tests, rollbacks and
  uninstalls
* Offers extensive configuration options for automated remediation
  (rollback, uninstall, retry) on failed Helm install, upgrade or test
  actions
* Runs Helm install/upgrade in a specific order, taking into account the
  depends-on relationship defined in a set of `HelmRelease` objects
* Reports Helm release statuses (alerting provided by
  [notification-controller][])
* Built-in Kustomize compatible Helm post renderer, providing support
  for strategic merge, JSON 6902 and images patches
* Supports detecting and correcting in-cluster changes compared to the desired
  state of the Helm release

## Guides

* [Get started with GitOps Toolkit](https://fluxcd.io/flux/get-started/)
* [Manage Helm Releases](https://fluxcd.io/flux/guides/helmreleases/)
* [Setup Notifications](https://fluxcd.io/flux/guides/notifications/)

## Specifications

* [API](docs/spec/v2/README.md)
* [Controller](docs/spec/README.md)

[source-controller]: https://github.com/fluxcd/source-controller
[notification-controller]: https://github.com/fluxcd/notification-controller
