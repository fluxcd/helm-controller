FROM gcr.io/oss-fuzz-base/base-builder-go

RUN wget https://go.dev/dl/go1.23.0.linux-amd64.tar.gz \
    && mkdir temp-go \
    && rm -rf /root/.go/* \
    && tar -C temp-go/ -xzf go1.23.0.linux-amd64.tar.gz \
    && mv temp-go/go/* /root/.go/

ENV SRC=$GOPATH/src/github.com/fluxcd/helm-controller
ENV FLUX_CI=true

COPY ./ $SRC
RUN wget https://raw.githubusercontent.com/google/oss-fuzz/master/projects/fluxcd/build.sh -O $SRC/build.sh

WORKDIR $SRC
