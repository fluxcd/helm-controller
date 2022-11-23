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

# This file aims for:
# - Dynamically discover and build all fuzz tests within the repository.
# - Work for both local make fuzz-smoketest and the upstream oss-fuzz.

GOPATH="${GOPATH:-/root/go}"
GO_SRC="${GOPATH}/src"
PROJECT_PATH="github.com/fluxcd/helm-controller"

# install_deps installs all dependencies needed for upstream oss-fuzz.
# Unfortunately we can't pin versions here, as we want to always
# have the latest, so that we can reproduce errors occuring upstream.
install_deps(){
	if ! command -v go-118-fuzz-build &> /dev/null; then
		go install github.com/AdamKorcz/go-118-fuzz-build@latest
	fi
}

install_deps

cd "${GO_SRC}/${PROJECT_PATH}"

# Ensure any project-specific requirements are catered for ahead of
# the generic build process.
if [ -f "tests/fuzz/oss_fuzz_prebuild.sh" ]; then
	tests/fuzz/oss_fuzz_prebuild.sh
fi

modules=$(find . -mindepth 1 -maxdepth 4 -type f -name 'go.mod' | cut -c 3- | sed 's|/[^/]*$$||' | sort -u | sed 's;/go.mod;;g' | sed 's;go.mod;.;g')

for module in ${modules}; do

	cd "${GO_SRC}/${PROJECT_PATH}/${module}"

	# TODO: stop ignoring recorder_fuzzer_test.go. Temporary fix for fuzzing building issues.
	test_files=$(grep -r --include='**_test.go' --files-with-matches 'func Fuzz' . || echo "")
	if [ -z "${test_files}" ]; then
		continue
	fi

	go get github.com/AdamKorcz/go-118-fuzz-build/testing

	# Iterate through all Go Fuzz targets, compiling each into a fuzzer.
	for file in ${test_files}; do
		# If the subdir is a module, skip this file, as it will be handled
		# at the next iteration of the outer loop. 
		if [ -f "$(dirname "${file}")/go.mod" ]; then
			continue
		fi

		targets=$(grep -oP 'func \K(Fuzz\w*)' "${file}")
		for target_name in ${targets}; do
			# Transform module path into module name (e.g. git/libgit2 to git_libgit2).
			module_name=$(echo ${module} | tr / _)
			# Compose fuzzer name based on the lowercase version of the func names.
			# The module name is added after the fuzz prefix, for better discoverability.
			fuzzer_name=$(echo "${target_name}" | tr '[:upper:]' '[:lower:]' | sed "s;fuzz_;fuzz_${module_name}_;g")
			target_dir=$(dirname "${file}")

			echo "Building ${file}.${target_name} into ${fuzzer_name}"
			compile_native_go_fuzzer "${target_dir}" "${target_name}" "${fuzzer_name}"
		done
	done
done
