# Development

> **Note:** Please take a look at <https://fluxcd.io/contributing/flux/>
> to find out about how to contribute to Flux and how to interact with the
> Flux Development team.

## Installing required dependencies

There are a number of dependencies required to be able to run the controller and its test suite locally:

- [Install Go](https://golang.org/doc/install)
- [Install Kustomize](https://kubernetes-sigs.github.io/kustomize/installation/)
- [Install Docker](https://docs.docker.com/engine/install/)
- (Optional) [Install Kubebuilder](https://book.kubebuilder.io/quick-start.html#installation)

In addition to the above, the following dependencies are also used by some of the `make` targets:

- `controller-gen` (v0.19.0)
- `gen-crd-api-reference-docs` (v0.3.0)
- `setup-envtest` (latest)

If any of the above dependencies are not present on your system, the first invocation of a `make` target that requires them will install them.

## How to run the test suite

Prerequisites:
* Go >= 1.25

You can run the test suite by simply doing

```bash
make test
```

## How to run the controller locally

Install the controller's CRDs on your test cluster:

```sh
make install
```

Note that `helm-controller` depends on [source-controller](https://github.com/fluxcd/source-controller) to acquire the Helm charts from Helm repositories. If `source-controller` is not running on your test cluster, you need to tell `helm-controller` where to find it. 

Port forward to source-controller artifacts server:

```sh
kubectl -n flux-system port-forward svc/source-controller 8080:80
```

Export the local address as `SOURCE_CONTROLLER_LOCALHOST`:

```sh
export SOURCE_CONTROLLER_LOCALHOST=localhost:8080
```

Alternatively, if your test cluster is already running `source-controller` and `helm-controller`, you need to scale down the in-cluster `helm-controller`:

```
kubectl -n flux-system scale deployment/helm-controller --replicas=0
```

Run the controller locally:

```sh
make run
```

## How to install the controller

### Building the container image

Set the name of the container image to be created from the source code. This will be used when building, pushing and referring to the image on YAML files:

```sh
export IMG=registry-path/helm-controller:latest
```

Build the container image, tagging it as `$(IMG)`:

```sh
make docker-build
```

Push the image into the repository:

```sh
make docker-push
```

**Note**: `make docker-build` will build an image for the `amd64` architecture.


### Deploying into a cluster

Deploy `helm-controller` into the cluster that is configured in the local kubeconfig file (i.e. `~/.kube/config`):

```sh
make deploy
```

## Debugging the controller locally

When reproducing an issue or stepping through reconciliation logic, the
following knobs make local runs cheaper and the resulting logs easier to
read.

### Limit the watched namespace

The controller watches every namespace by default. To narrow it to a single
namespace, set the `RUNTIME_NAMESPACE` environment variable before invoking
`make run`:

```sh
RUNTIME_NAMESPACE=flux-system make run
```

### Reduce reconcile concurrency

Each `HelmRelease` reconcile is processed concurrently (default
`--concurrent=4`). When debugging it is almost always easier to follow a
serial trace; pass `--concurrent=1` so reconciles run one at a time:

```sh
go run ./main.go --concurrent=1
```

### Suspend unrelated objects

If the controller is sharing a cluster with other Flux objects, suspend
anything not relevant to the test you're running so their reconciles don't
interleave with yours:

```sh
flux suspend helmrelease <name>
```

Resume with `flux resume helmrelease <name>` when you're done.
