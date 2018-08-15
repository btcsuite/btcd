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
# 9246  Mainnet Bitcoin peer-to-peer port
# 9245  Mainet RPC port

ARG ARCH=amd64
# using the SHA256 instead of tags
# https://github.com/opencontainers/image-spec/blob/main/descriptor.md#digests
# https://cloud.google.com/architecture/using-container-images
# https://github.com/google/go-containerregistry/blob/main/cmd/crane/README.md
# ➜  ~ crane digest golang:1.16-alpine3.12
# sha256:db2475a1dbb2149508e5db31d7d77a75e6600d54be645f37681f03f2762169ba
FROM golang@sha256:db2475a1dbb2149508e5db31d7d77a75e6600d54be645f37681f03f2762169ba AS build-container

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
