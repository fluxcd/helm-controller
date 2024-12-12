ARG GO_VERSION=1.23
ARG XX_VERSION=1.6.1

FROM --platform=$BUILDPLATFORM tonistiigi/xx:${XX_VERSION} AS xx

# Docker buildkit multi-arch build requires golang alpine
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder

# Copy the build utilities.
COPY --from=xx / /

ARG TARGETPLATFORM

WORKDIR /workspace

# copy api submodule
COPY api/ api/

# copy modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# cache modules
RUN go mod download

# copy source code
COPY main.go main.go
COPY internal/ internal/

# build without specifing the arch
ENV CGO_ENABLED=0
RUN xx-go build -trimpath -a -o helm-controller main.go

FROM alpine:3.21

RUN apk add --no-cache ca-certificates \
    && update-ca-certificates

COPY --from=builder /workspace/helm-controller /usr/local/bin/

USER 65534:65534

ENTRYPOINT [ "helm-controller" ]
