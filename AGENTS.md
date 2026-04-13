# AGENTS.md

Guidance for AI coding assistants working in `fluxcd/helm-controller`. Read this file before making changes.

## Contribution workflow for AI agents

These rules come from [`fluxcd/flux2/CONTRIBUTING.md`](https://github.com/fluxcd/flux2/blob/main/CONTRIBUTING.md) and apply to every Flux repository.

- **Do not add `Signed-off-by` or `Co-authored-by` trailers with your agent name.** Only a human can legally certify the DCO.
- **Disclose AI assistance** with an `Assisted-by` trailer naming your agent and model:
  ```sh
  git commit -s -m "Add support for X" --trailer "Assisted-by: <agent-name>/<model-id>"
  ```
  The `-s` flag adds the human's `Signed-off-by` from their git config — do not remove it.
- **Commit message format:** Subject in imperative mood ("Add feature X" instead of "Adding feature X"), capitalized, no trailing period, ≤50 characters. Body wrapped at 72 columns, explaining what and why. No `@mentions` or `#123` issue references in the commit — put those in the PR description.
- **Trim verbiage:** in PR descriptions, commit messages, and code comments. No marketing prose, no restating the diff, no emojis.
- **Rebase, don't merge:** Never merge `main` into the feature branch; rebase onto the latest `main` and push with `--force-with-lease`. Squash before merge when asked.
- **Pre-PR gate:** `make tidy fmt vet && make test` must pass and the working tree must be clean after codegen. Commit regenerated files in the same PR.
- **Flux is GA:** Backward compatibility is mandatory. Breaking changes to CRD fields, status, CLI flags, metrics, or observable behavior will be rejected. Design additive changes and keep older API versions round-tripping.
- **Copyright:** All new `.go` files must begin with the boilerplate from `hack/boilerplate.go.txt` (Apache 2.0). Update the year to the current year when copying.
- **Spec docs:** New features and API changes must be documented in `docs/spec/v2/helmreleases.md`. Update it in the same PR that introduces the change.
- **Tests:** New features, improvements and fixes must have test coverage. Add unit tests in `internal/controller/*_test.go` and other `internal/*` packages as appropriate. Follow the existing patterns for test organization, fixtures, and assertions. Run tests locally before pushing.

## Code quality

Before submitting code, review your changes for the following:

- **No secrets in logs or events.** Never surface auth tokens, passwords, kubeconfig data, or Helm release secret contents in error messages, conditions, events, or log lines.
- **No unchecked I/O.** Close HTTP response bodies, file handles, and chart archive readers in `defer` statements. Check and propagate errors from `io.Copy`, `os.Remove`, `os.Rename`.
- **No path traversal.** Validate and sanitize file paths extracted from chart archives. Never `filepath.Join` with untrusted components without validation.
- **No unbounded reads.** Use `io.LimitReader` when reading from network or archive sources. Respect existing size limits; do not introduce new reads without bounds.
- **No direct Helm calls from reconcile code.** All Helm SDK interactions go through `internal/action/ConfigFactory`, which wires the observing storage driver. Bypassing it breaks history tracking and remediation.
- **No hardcoded defaults for security settings.** TLS verification must remain enabled by default; impersonation and cross-namespace ref restrictions must be honored.
- **Helm secret ownership.** Helm release secrets (`sh.helm.release.v1.*`) are owned by Helm, not the controller. Do not add logic that creates, modifies, or deletes them outside of Helm operations.
- **Error handling.** Wrap errors with `%w` for chain inspection. Do not swallow errors silently. Return actionable error messages that help users diagnose the issue without leaking internal state.
- **Resource cleanup.** Ensure temporary files, directories, and fetched chart artifacts are cleaned up on all code paths (success and error). Use `defer` and `t.TempDir()` in tests.
- **Concurrency safety.** Do not introduce shared mutable state without synchronization. Reconcilers run concurrently; per-object work must be isolated.
- **No panics.** Never use `panic` in runtime code paths. Return errors and let the reconciler handle them gracefully.
- **Minimal surface.** Keep new exported APIs, flags, and environment variables to the minimum needed. Every export is a backward-compatibility commitment.

## Project overview

helm-controller is a core component of the [Flux GitOps Toolkit](https://fluxcd.io/flux/components/). It reconciles `HelmRelease` objects into real Helm releases on a target cluster. It depends on source-controller for chart delivery: either a `HelmChart` (generated from a `HelmRepository`, `GitRepository`, or `Bucket`) or an `OCIRepository` referenced through `spec.chartRef`. The controller drives the full Helm lifecycle (install, upgrade, test, rollback, uninstall) via the embedded Helm SDK, tracks its own release history in `HelmRelease.status`, and can detect and correct in-cluster drift against the manifest persisted in Helm storage.

## Repository layout

- `main.go` — controller entrypoint: flag parsing, feature gates, manager and `HelmReleaseReconciler` wiring. The only binary.
- `api/` — separate Go module (`github.com/fluxcd/helm-controller/api`) with the public CRD types. Consumed by downstream projects.
  - `api/v2` — current stable version (`helm.toolkit.fluxcd.io/v2`). Types: `HelmRelease`, `HelmChartTemplate`, `DriftDetection`, `Snapshots`, etc.
  - `api/v2beta2`, `api/v2beta1` — prior versions retained for conversion and backward compatibility. Do not add new fields there.
- `config/` — Kustomize manifests: `crd/bases` (generated CRDs), `manager`, `rbac`, `default`, `samples`, `testdata`. CRDs here are generated — never hand-edit.
- `hack/` — `boilerplate.go.txt` license header, `api-docs/` config for `gen-crd-api-reference-docs`.
- `docs/` — user-facing spec (`docs/spec/v2/helmreleases.md`), generated API reference (`docs/api/v2/helm.md`), and internal notes (`docs/internal/`).
- `internal/` — non-exported controller logic.
  - `controller/` — `HelmReleaseReconciler` (top-level `Reconcile`, indexers, manager setup, envtest suite tests).
  - `reconcile/` — the release state machine. `AtomicRelease` is the top-level `ActionReconciler` that loops: compute `ReleaseState` via `state.go`, pick the next action (`Install`, `Upgrade`, `Test`, `RollbackRemediation`, `UninstallRemediation`, `Unlock`, `CorrectClusterDrift`), run it, patch, and loop until done, stalled, or a retry is needed. `helmchart_template.go` reconciles the `HelmChart` source object from `spec.chart`.
  - `action/` — thin wrappers around the Helm SDK (`install.go`, `upgrade.go`, `rollback.go`, `uninstall.go`, `test.go`, `verify.go`, `diff.go`, `wait.go`) plus the `ConfigFactory` that builds Helm `action.Configuration` with the observing storage driver. Helm semantics live here.
  - `release/` — release name derivation, config/chart digest calculation, and storage-snapshot observation helpers.
  - `storage/` — `Observer` Helm storage driver: wraps the upstream driver so writes to Helm secrets/configmaps are captured into `status.History` regardless of whether the Helm action returned an error.
  - `postrender/` — built-in Kustomize post-renderer (strategic merge, JSON6902, image patches) plus `CommonRenderer` for `spec.commonMetadata`.
  - `diff/`, `cmp/`, `digest/` — drift detection helpers and digest algorithms (`sha256`, `sha384`, `sha512`, `blake3`) used for snapshotting state.
  - `loader/` — chart artifact fetch from source-controller (HTTP, retries).
  - `kube/`, `acl/`, `predicates/`, `inventory/`, `errors/`, `strings/`, `features/`, `oomwatch/`, `testutil/` — small utility packages.
  - `controller_test/` — shared end-to-end style fixtures.

## APIs and CRDs

- Group: `helm.toolkit.fluxcd.io`. Kind: `HelmRelease` (the only CRD owned by this controller).
- Served versions: `v2` (storage, current), `v2beta2`, `v2beta1`. Finalizer: `finalizers.fluxcd.io`. `HelmReleaseKind` is exported from `api/v2`.
- Types in `api/v2/helmrelease_types.go`. Condition type constants (`Released`, `TestSuccess`, `Remediated`, `Drifted`, plus `Ready`/`Reconciling`/`Stalled` from `fluxcd/pkg/apis/meta`) are in `api/v2/condition_types.go`. Release snapshots in `snapshot_types.go`.
- The CRD manifest at `config/crd/bases/helm.toolkit.fluxcd.io_helmreleases.yaml` is generated from kubebuilder markers. Edit the Go types and regenerate — never edit the YAML by hand.

## Build, test, lint

All workflows go through the root `Makefile`. Go version tracks `go.mod`. Tools (`controller-gen`, `gen-crd-api-reference-docs`, `setup-envtest`) auto-install into `build/gobin/`.

- `make tidy` — tidy both the root and `api/` modules.
- `make fmt` / `make vet` — run in both modules.
- `make generate` — `controller-gen` deepcopy stubs in `api/`.
- `make manifests` — regenerate CRDs and RBAC under `config/` from kubebuilder markers.
- `make api-docs` — regenerate `docs/api/v2/helm.md`.
- `make manager` — build `build/bin/manager`.
- `make test` — full suite. Depends on `tidy generate fmt vet manifests api-docs install-envtest download-crd-deps`; sets `KUBEBUILDER_ASSETS`, downloads the `source.toolkit.fluxcd.io` CRDs (`HelmChart`, `OCIRepository`, `ExternalArtifact`) pinned to the version in `go.mod`, runs `go test ./...` at the root, then `go test ./...` inside `api/`.
- `make install` / `make run` / `make deploy` / `make dev-deploy` — cluster workflows (see `DEVELOPMENT.md`). `make run` expects source-controller to be reachable (optionally via `SOURCE_CONTROLLER_LOCALHOST=localhost:8080`).
- `make docker-build` / `make docker-push` — container image.

## Codegen and generated files

Check `go.mod` and the `Makefile` for current dependency and tool versions. After changing API types or kubebuilder markers, regenerate and commit the results:

```sh
make generate manifests api-docs
```

Generated files (never hand-edit):

- `api/*/zz_generated.deepcopy.go`
- `config/crd/bases/*.yaml`
- `config/rbac/role.yaml`
- `docs/api/v2/helm.md`

Load-bearing `replace` in `go.mod` — do not remove:

- `helm.sh/helm/v4` → `github.com/fluxcd/helm/v4` (Flux fork with upstream bugfixes). Do not remove without coordinating with maintainers.

The `fluxcd/source-controller/api` version in `go.mod` drives `SOURCE_VER` and determines which source CRDs are downloaded for envtest.

Bump `fluxcd/pkg/*` modules as a set — version skew breaks `go.sum`. Run `make tidy` after any bump.

## Conventions

- Standard `gofmt`, explicit errors. All exported identifiers get doc comments, as do non-trivial unexported types and functions.
- **Controller structure.** `HelmReleaseReconciler.Reconcile` handles suspend, dependency checks, artifact acquisition, impersonation, then delegates the Helm lifecycle to `reconcile.NewAtomicRelease(...)`. Don't add Helm action logic to the top-level reconciler.
- **Release state machine.** The `AtomicRelease` loop is authoritative. Any new action type must implement `reconcile.ActionReconciler` (`Reconcile`, `Name`, `Type`) and be selected by `actionForState` based on a `ReleaseStatus` returned by `DetermineReleaseState`. New `ReconcilerType` constants go in `reconcile/reconcile.go`. Respect the `releaseStrategy` contract so a single reconcile attempt never runs the same action twice.
- **Conditions.** Writes go through `github.com/fluxcd/pkg/runtime/conditions`. Owned conditions are listed in `reconcile.OwnedConditions`. The controller writes `Released`, `TestSuccess`, `Remediated`, `Reconciling`, `Stalled`, `Ready`, and `Drifted`. `Ready` is a summary of the other owned conditions and must never be set directly outside `summarize`.
- **Release history.** `status.History` is a `Snapshots` list populated by the `storage.Observer` driver, which wraps the real Helm driver so every persisted revision is observed even if the Helm action ultimately errored. `MaxHistory` defaults to 5; truncation is handled by the observer/reconcile code, not by Helm's native history pruning.
- **Digests.** Config (values), chart, and post-renderer digests are computed in `internal/digest` and `internal/release` using the algorithm set by `--snapshot-digest-algo` (default `sha256`, `blake3` supported). Drift of `ObservedPostRenderersDigest` or the config digest forces an upgrade.
- **Drift detection.** Configured per-HelmRelease via `spec.driftDetection.mode` (`disabled`, `warn`, `enabled`). The old cluster-wide `DetectDrift` / `CorrectDrift` feature gates are deprecated — prefer the spec field. Correction uses SSA through `fluxcd/pkg/ssa`. `--override-manager` lets operators declare field managers whose changes should be overridden.
- **Atomic / wait semantics.** Helm's `--atomic` is *not* used directly; atomicity is implemented via the release state machine and remediation strategies. Waits use `kstatus` polling (`action/wait.go`), or Helm's built-in wait when `spec.waitStrategy` selects it. CEL `healthCheckExprs` run inside the poller strategy only.
- **Remediation & retries.** `spec.install.remediation` / `spec.upgrade.remediation` drive `RemediateOnFailure`; the `DefaultToRetryOnFailure` feature gate swaps the default to `RetryOnFailure`. Branch off `req.Object.GetActiveRemediation()` and `GetActiveRetry(defaultToRetryOnFailure)`.
- **OCI charts.** Consumed via `spec.chartRef` pointing at an `OCIRepository`. `DisableChartDigestTracking` disables the upgrade-on-digest-change behavior for identical chart versions.
- **Post-renderers.** Built-in `kustomize`, plus `commonMetadata` and user-defined `postRenderers`. Implementations in `internal/postrender`; new renderers plug into `BuildPostRenderers`.
- **ACL.** Cross-namespace source references are allowed by default but can be disabled with `--no-cross-namespace-refs`. The `internal/acl` package centralizes this.
- **Test seam.** `ConfigFactory` in `internal/action` is the test seam for swapping Helm configuration.

## Testing

- Unit and controller tests use `sigs.k8s.io/controller-runtime/envtest` via the Flux `pkg/runtime/testenv` wrapper. `make install-envtest` downloads control-plane binaries into `build/testbin/`; `make test` wires `KUBEBUILDER_ASSETS` automatically.
- `make download-crd-deps` fetches source-controller CRDs into `build/config/crd/bases/`. The controller suite loads both `build/...` and `config/crd/bases` as CRD sources.
- Integration tests live in `internal/controller/*_test.go` with `TestMain` in `suite_test.go` setting up `testEnv` and a `testserver.HTTPServer` that serves fake chart artifacts.
- Reconcile/action-level tests live under `internal/reconcile/` and `internal/action/`; they exercise `ActionReconciler` implementations against a fake Helm storage and a mocked `ConfigFactory`.
- Run a single test: `make test GO_TEST_ARGS='-run TestAtomicRelease_Reconcile'`.
- The `api/` module has its own small test suite; `make test` runs it after the main module.

## Gotchas and non-obvious rules

- The release state machine is the source of truth. A naive "just call Helm upgrade" change inside `HelmReleaseReconciler` will break remediation, history tracking, and drift correction. Extend `AtomicRelease` and `DetermineReleaseState` instead.
- The observing storage driver (`internal/storage.Observer`) is why partial Helm failures still show up in `status.History`.
- `status.History` is capped by `spec.maxHistory` (default 5). The controller, not Helm, owns truncation. Setting `maxHistory: 0` means unlimited.
- On uninstall the controller runs Helm uninstall; leftover `sh.helm.release.v1.*` secrets typically indicate a user-initiated partial cleanup and need manual intervention.
- `spec.storageNamespace` defaults to the HelmRelease's namespace but can diverge from `spec.targetNamespace`. Code that touches Helm storage must use `HelmRelease.GetStorageNamespace()`, not assume it equals the target.
- CRD-heavy charts (kube-prometheus-stack and similar) interact poorly with `persistentClient: true` when charts create CRDs outside Helm's CRD hooks. The field defaults to true; leave it that way unless you understand the trade-off.
- Drift detection compares the Helm storage manifest against live objects via SSA dry-run. Charts that mutate their own rendered manifests in-cluster (webhooks, operators) loop forever unless their field managers are added to `--override-manager` or their objects are excluded via `spec.driftDetection.ignore`.
- `AdoptLegacyReleases`, `DetectDrift`, and `CorrectDrift` feature gates are deprecated/ignored from v1.5.0. Do not add new code paths gated on them.
- `api/` is its own Go module. Changes to API types require `go mod tidy` in both `./` and `./api` (handled by `make tidy`). Downstream modules import `api/` directly, so API refactors are user-visible.
