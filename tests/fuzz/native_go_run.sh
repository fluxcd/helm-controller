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

# This script iterates through all go fuzzing targets, running each one
# through the period of time established by FUZZ_TIME.

FUZZ_TIME=${FUZZ_TIME:-"5s"}

test_files=$(grep -r --include='**_test.go' --files-with-matches 'func Fuzz' .)

for file in ${test_files}
do
	targets=$(grep -oP 'func \K(Fuzz\w*)' "${file}")
	for target_name in ${targets}
	do
		echo "Running ${file}.${target_name} for ${FUZZ_TIME}."
		file_dir=$(dirname "${file}")

		go test -fuzz="${target_name}" -fuzztime "${FUZZ_TIME}" "${file_dir}"
	done
done
