# GitHub action dockerfile
# Requires docker experimental features as buildx and BuildKit so not suitable for developers regular use.
# https://docs.docker.com/develop/develop-images/build_enhancements/#to-enable-buildkit-builds

###########################
# Build binaries stage
###########################
FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine3.24@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS build
ADD . /app
WORKDIR /app
# Arguments required to build binaries targetting the correct OS and CPU architectures
ARG TARGETOS TARGETARCH
# Build into a flat directory. When cross-compiling, go install would put the
# binaries into a per-platform subdirectory such as /go/bin/linux_arm64.
RUN GOOS=$TARGETOS GOARCH=$TARGETARCH go build -v -o /out/ . ./cmd/...

###########################
# Build docker image stage
###########################
FROM alpine:3.24
COPY --from=build /out/ /bin/
# 8333  Mainnet Bitcoin peer-to-peer port
# 8334  Mainet RPC port
EXPOSE 8333 8334
ENTRYPOINT ["btcd"]
