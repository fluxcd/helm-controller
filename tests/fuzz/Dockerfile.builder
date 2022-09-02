FROM gcr.io/oss-fuzz-base/base-builder-go

COPY ./ $GOPATH/src/github.com/fluxcd/helm-controller/
COPY ./tests/fuzz/oss_fuzz_build.sh $SRC/build.sh

# Temporarily overrides compile_native_go_fuzzer.
# Pending upstream merge: https://github.com/google/oss-fuzz/pull/8285
COPY tests/fuzz/compile_native_go_fuzzer.sh /usr/local/bin/compile_native_go_fuzzer
RUN go install golang.org/x/tools/cmd/goimports@latest

WORKDIR $SRC
