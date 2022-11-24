# fuzz testing

Flux is part of Google's [oss fuzz] program which provides continuous fuzzing for 
open source projects. 

The long running fuzzing execution is configured in the [oss-fuzz repository].
Shorter executions are done on a per-PR basis, configured as a [github workflow].

### Testing locally

Build fuzzers:

```bash
make fuzz-build
```
All fuzzers will be built into `./build/fuzz/out`. 

Smoke test fuzzers:

All the fuzzers will be built and executed once, to ensure they are fully functional.

```bash
make fuzz-smoketest
```

Run fuzzer locally:
```bash
./build/fuzz/out/fuzz_conditions_match
```

Run fuzzer inside a container:

```bash
	docker run --rm -ti \
		-v "$(pwd)/build/fuzz/out":/out \
		gcr.io/oss-fuzz/fluxcd \
		/out/fuzz_conditions_match
```

### Caveats of creating oss-fuzz compatible tests

#### Segregate fuzz tests

OSS-Fuzz does not properly support mixed `*_test.go` files, in which there is a combination
of fuzz and non-fuzz tests. To mitigate this problem, ensure your fuzz tests are not in the
same file as other Go tests. As a pattern, call your fuzz test files `*_fuzz_test.go`.

#### Build tags to avoid conflicts when running Go tests

Due to the issue above, code duplication will occur when creating fuzz tests that rely on
helper functions that are shared with other tests. To avoid build issues, add a conditional
build tag at the top of the `*_fuzz_test.go` file:
```go
//go:build gofuzz_libfuzzer
// +build gofuzz_libfuzzer
```

The build tag above is set at [go-118-fuzz-build].
At this point in time we can't pass on specific tags from [compile_native_go_fuzzer].

### Running oss-fuzz locally

The `make fuzz-smoketest` is meant to be an easy way to reproduce errors that may occur
upstream. If our checks ever run out of sync with upstream, the upstream tests can be
executed locally with:

```
git clone --depth 1 https://github.com/google/oss-fuzz
cd oss-fuzz
python infra/helper.py build_image fluxcd
python infra/helper.py build_fuzzers --sanitizer address --architecture x86_64 fluxcd
python infra/helper.py check_build --sanitizer address --architecture x86_64 fluxcd
```

For latest info on testing oss-fuzz locally, refer to the [upstream guide].

[oss fuzz]: https://github.com/google/oss-fuzz
[oss-fuzz repository]: https://github.com/google/oss-fuzz/tree/master/projects/fluxcd
[github workflow]: .github/workflows/cifuzz.yaml
[upstream guide]: https://google.github.io/oss-fuzz/getting-started/new-project-guide/#testing-locally
[go-118-fuzz-build]: https://github.com/AdamKorcz/go-118-fuzz-build/blob/b2031950a318d4f2dcf3ec3e128f904d5cf84623/main.go#L40
[compile_native_go_fuzzer]: https://github.com/google/oss-fuzz/blob/c2d827cb78529fdc757c9b0b4fea0f1238a54814/infra/base-images/base-builder/compile_native_go_fuzzer#L32