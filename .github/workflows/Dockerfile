# GitHub action dockerfile
# Requires docker experimental features as buildx and BuildKit so not suitable for developers regular use.
# https://docs.docker.com/develop/develop-images/build_enhancements/#to-enable-buildkit-builds

###########################
# Build binaries stage
###########################
FROM --platform=$BUILDPLATFORM golang:1.17.8-alpine3.15 AS build
ADD . /app
WORKDIR /app
# Arguments required to build binaries targetting the correct OS and CPU architectures
ARG TARGETOS TARGETARCH
# Actually building the binaries
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go install -v . ./cmd/...

###########################
# Build docker image stage
###########################
FROM alpine:3.15
COPY --from=build /go/bin /bin
# 8333  Mainnet Bitcoin peer-to-peer port
# 8334  Mainet RPC port
EXPOSE 8333 8334
ENTRYPOINT ["btcd"]
