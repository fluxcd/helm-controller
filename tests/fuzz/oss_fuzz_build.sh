#!/usr/bin/env bash

# Copyright 2022 The Flux authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

set -euxo pipefail

GOPATH="${GOPATH:-/root/go}"
GO_SRC="${GOPATH}/src"
PROJECT_PATH="github.com/fluxcd/helm-controller"

cd "${GO_SRC}"

# Move fuzzer to their respective directories. 
# This removes dependency noises from the modules' go.mod and go.sum files.
cp "${PROJECT_PATH}/tests/fuzz/fuzz_controllers.go" "${PROJECT_PATH}/controllers/"


# compile fuzz tests for the runtime module
pushd "${PROJECT_PATH}"

go mod tidy
compile_go_fuzzer "${PROJECT_PATH}/controllers/" FuzzHelmreleaseComposeValues fuzz_helmrelease_composevalues
compile_go_fuzzer "${PROJECT_PATH}/controllers/" FuzzHelmreleaseReconcile fuzz_helmrelease_reconcile

popd
