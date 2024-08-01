FROM alpine
WORKDIR /
COPY ./bin/manager /manager

ENTRYPOINT ["/manager"]
