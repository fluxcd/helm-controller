FROM golang:1.13 as builder

WORKDIR /workspace

# copy modules manifests
COPY go.mod go.mod
COPY go.sum go.sum

# cache modules
RUN go mod download

# copy source code
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/

# build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o helm-controller main.go

FROM alpine:3.12

RUN apk add --no-cache ca-certificates tini

COPY --from=builder /workspace/helm-controller /usr/local/bin/

RUN addgroup -S controller && adduser -S -g controller controller

USER controller

ENTRYPOINT [ "/sbin/tini", "--", "helm-controller" ]
