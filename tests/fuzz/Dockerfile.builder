FROM gcr.io/oss-fuzz-base/base-builder-go

COPY ./ $GOPATH/src/github.com/fluxcd/helm-controller/
COPY ./tests/fuzz/oss_fuzz_build.sh $SRC/build.sh

WORKDIR $SRC
