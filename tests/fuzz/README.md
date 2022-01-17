# fuzz testing

Flux is part of Google's [oss fuzz] program which provides continuous fuzzing for 
open source projects. 

The long running fuzzing execution is configured in the [oss-fuzz repository].
Shorter executions are done on a per-PR basis, configured as a [github workflow].

For fuzzers to be called, they must be compiled within [oss_fuzz_build.sh](./oss_fuzz_build.sh).

### Testing locally

Build fuzzers:

```bash
make fuzz-build
```
All fuzzers will be built into `./build/fuzz/out`. 

Smoke test fuzzers:

```bash
make fuzz-smoketest
```

The smoke test runs each fuzzer once to ensure they are fully functional.

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


[oss fuzz]: https://github.com/google/oss-fuzz
[oss-fuzz repository]: https://github.com/google/oss-fuzz/tree/master/projects/fluxcd
[github workflow]: .github/workflows/cifuzz.yaml
