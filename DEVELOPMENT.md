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

Use this section when reproducing an issue against a real cluster. Follow
[How to run the controller locally](#how-to-run-the-controller-locally) first
so CRDs are installed, the in-cluster Deployment is scaled to zero when needed,
and `SOURCE_CONTROLLER_LOCALHOST` is set if source-controller is only reachable
via port-forward.

### Readable, verbose logs

`make run` starts the process with the default log settings (`info` / `json`).
For interactive debugging, prefer console encoding and a higher verbosity:

```sh
go run ./main.go --log-level=debug --log-encoding=console
```

`--log-level=trace` is available when you need the most detailed controller-runtime
output. The full flag list lives in [`docs/README.md`](docs/README.md).

### Reduce noise from other HelmReleases

A shared test cluster may hold many `HelmRelease` objects. Suspend everything
you are not debugging so their reconciles do not interleave with yours:

```sh
flux suspend helmrelease --all --namespace <namespace>
# or a single object:
flux suspend helmrelease <name> --namespace <namespace>
```

Resume with `flux resume` when finished. Alternatively, label the object under
test and narrow the manager watch with `--watch-label-selector` (for example
`debugging=true`). Keep `--watch-all-namespaces=true` (the default) unless you
intentionally want to hide cross-namespace behaviour.

### Trigger a reconcile on demand

After changing cluster state or attaching a debugger, request an immediate
reconcile without waiting for the interval:

```sh
kubectl annotate --field-manager=flux-client-side-apply --overwrite \
  helmrelease/<name> -n <namespace> \
  reconcile.fluxcd.io/requestedAt="$(date +%s)"
```

### Profiles (pprof)

pprof handlers are registered on the metrics server (default
`--metrics-addr=:8080`). While the controller is running locally:

```sh
go tool pprof http://localhost:8080/debug/pprof/heap
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30
```

### Debugging with VS Code

Create `.vscode/launch.json` (gitignored) after installing CRDs and scaling down
the in-cluster Deployment as described above:

```json
{
  "version": "0.2.0",
  "configurations": [
    {
      "name": "Debug helm-controller",
      "type": "go",
      "request": "launch",
      "mode": "auto",
      "program": "${workspaceFolder}/main.go",
      "args": [
        "--log-level=debug",
        "--log-encoding=console"
      ],
      "env": {
        "SOURCE_CONTROLLER_LOCALHOST": "localhost:8080"
      }
    }
  ]
}
```

Start with **Run > Start Debugging**. Adjust `SOURCE_CONTROLLER_LOCALHOST` only
when you are port-forwarding source-controller as documented above.