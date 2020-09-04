FROM golang:1.14 as builder

ARG TARGETPLATFORM

ENV CGO_ENABLED=0 \
  GO111MODULE=on

WORKDIR /workspace

# copy modules manifests
COPY go.mod go.sum ./

# cache modules
RUN go mod download

# copy all content
COPY . .

# build
RUN export GOOS=$(echo ${TARGETPLATFORM} | cut -d / -f1) && \
  export GOARCH=$(echo ${TARGETPLATFORM} | cut -d / -f2) && \
  export GOARM=$(echo ${TARGETPLATFORM} | cut -d / -f3 | cut -c2-) && \
  go build -a -o helm-controller main.go

FROM alpine:3.12

RUN apk add --no-cache ca-certificates tini

COPY --from=builder /workspace/helm-controller /usr/local/bin/

RUN addgroup -S controller && adduser -S -g controller controller

USER controller

ENTRYPOINT [ "/sbin/tini", "--", "helm-controller" ]
