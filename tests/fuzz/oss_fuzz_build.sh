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
TMP_DIR=$(mktemp -d /tmp/oss_fuzz-XXXXXX)

cleanup(){
	rm -rf "${TMP_DIR}"
}
trap cleanup EXIT

install_deps(){
	if ! command -v go-118-fuzz-build &> /dev/null || ! command -v addimport &> /dev/null; then
		mkdir -p "${TMP_DIR}/go-118-fuzz-build"

		git clone https://github.com/AdamKorcz/go-118-fuzz-build "${TMP_DIR}/go-118-fuzz-build"
		cd "${TMP_DIR}/go-118-fuzz-build"
		go build -o "${GOPATH}/bin/go-118-fuzz-build"

		cd addimport
		go build -o "${GOPATH}/bin/addimport"
	fi

	if ! command -v goimports &> /dev/null; then
		go install golang.org/x/tools/cmd/goimports@latest
	fi
}

# Removes the content of test funcs which could cause the Fuzz
# tests to break.
remove_test_funcs(){
	filename=$1

	echo "removing co-located *testing.T"
	sed -i -e '/func Test.*testing.T) {$/ {:r;/\n}/!{N;br}; s/\n.*\n/\n/}' "${filename}"

	# After removing the body of the go testing funcs, consolidate the imports.
	goimports -w "${filename}"
}

install_deps

cd "${GO_SRC}/${PROJECT_PATH}"

go get github.com/AdamKorcz/go-118-fuzz-build/utils

# Iterate through all Go Fuzz targets, compiling each into a fuzzer.
test_files=$(grep -r --include='**_test.go' --files-with-matches 'func Fuzz' .)
for file in ${test_files}
do
	remove_test_funcs "${file}"

	targets=$(grep -oP 'func \K(Fuzz\w*)' "${file}")
	for target_name in ${targets}
	do
		fuzzer_name=$(echo "${target_name}" | tr '[:upper:]' '[:lower:]')
		target_dir=$(dirname "${file}")

		echo "Building ${file}.${target_name} into ${fuzzer_name}"
		compile_native_go_fuzzer "${target_dir}" "${target_name}" "${fuzzer_name}"
	done
done
