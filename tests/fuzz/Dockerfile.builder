FROM golang:1.18 AS go

FROM gcr.io/oss-fuzz-base/base-builder-go

# ensures golang 1.18 to enable go native fuzzing.
COPY --from=go /usr/local/go /usr/local/

COPY ./ $GOPATH/src/github.com/fluxcd/helm-controller/
COPY ./tests/fuzz/oss_fuzz_build.sh $SRC/build.sh

WORKDIR $SRC
