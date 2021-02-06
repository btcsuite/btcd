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

ARG ARCH=amd64

FROM golang:1.14-alpine3.12 AS build-container

ARG ARCH
ENV GO111MODULE=on

ADD . /app
WORKDIR /app
RUN set -ex \
  && if [ "${ARCH}" = "amd64" ]; then export GOARCH=amd64; fi \
  && if [ "${ARCH}" = "arm32v7" ]; then export GOARCH=arm; fi \
  && if [ "${ARCH}" = "arm64v8" ]; then export GOARCH=arm64; fi \
  && echo "Compiling for $GOARCH" \
  && go install -v . ./cmd/...

FROM $ARCH/alpine:3.12

COPY --from=build-container /go/bin /bin

VOLUME ["/root/.btcd"]

EXPOSE 8333 8334

ENTRYPOINT ["btcd"]
