# Development

> **Note:** Please take a look at <https://fluxcd.io/docs/contributing/flux/>
> to find out about how to contribute to Flux and how to interact with the
> Flux Development team.

## How to run the test suite

Prerequisites:
* go >= 1.16
* kubebuilder >= 2.3
* kustomize >= 3.1

You can run the unit tests by simply doing

```bash
make test
```

## How to run the controller locally

Install flux on your test cluster:

```sh
flux install
```

Scale the in-cluster controller to zero:

```sh
kubectl -n flux-system scale deployment/helm-controller --replicas=0
```

Port forward to source-controller artifacts server:

```sh
kubectl -n flux-system port-forward svc/source-controller 8080:80
```

Export the local address as `SOURCE_CONTROLLER_LOCALHOST`:

```sh
export SOURCE_CONTROLLER_LOCALHOST=localhost:8080
```

Run the controller locally:

```sh
make run
```
