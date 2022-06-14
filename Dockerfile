# This Dockerfile builds btcd from source and creates a small (55 MB) docker container based on alpine linux.
#
# Clone this repository and run the following command to build and tag a fresh btcd amd64 container:
#
# docker build . -t yourregistry/btcd
#
# You can use the following command to buid an arm64v8 container:
#
# docker build . -t yourregistry/btcd --build-arg ARCH=arm64v8
#
# For more information how to use this docker image visit:
# https://github.com/btcsuite/btcd/tree/master/docs
#
# 8333  Mainnet Bitcoin peer-to-peer port
# 8334  Mainet RPC port

FROM golang:alpine AS build-container

ENV GO111MODULE=on

ADD . /app
WORKDIR /app
RUN go install -v . ./cmd/...

FROM alpine:latest

COPY --from=build-container /go/bin /bin

EXPOSE 8333 8334

ENTRYPOINT ["btcd"]
