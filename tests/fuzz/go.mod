module github.com/fluxcd/helm-controller/tests/fuzz

// This module is used only to avoid polluting the main module
// with fuzz dependencies.

go 1.18

// Overwrite with local replace to ensure tests run with current state.
replace (
	github.com/fluxcd/helm-controller/api => ../../api
	github.com/fluxcd/helm-controller => ../../
)
