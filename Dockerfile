# Docker buildkit multi-arch build requires golang alpine
FROM golang:1.16-alpine as builder

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
COPY controllers/ controllers/
COPY internal/ internal/

# build without specifing the arch
RUN CGO_ENABLED=0 go build -a -o helm-controller main.go

FROM alpine:3.13

# link repo to the GitHub Container Registry image
LABEL org.opencontainers.image.source="https://github.com/fluxcd/helm-controller"

RUN apk add --no-cache ca-certificates tini

COPY --from=builder /workspace/helm-controller /usr/local/bin/

RUN addgroup -S controller && adduser -S controller -G controller

USER controller

ENTRYPOINT [ "/sbin/tini", "--", "helm-controller" ]
