#!/bin/bash
# The script does automatic checking on a Go package and its sub-packages, including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 3. go vet        (http://golang.org/cmd/vet)
# 4. gosimple      (https://github.com/dominikh/go-simple)
# 5. unconvert     (https://github.com/mdempsky/unconvert)
#

set -ex

go test -tags="rpctest" ./...

# Automatic checks
golangci-lint run --deadline=10m --disable-all \
--enable=gofmt \
--enable=vet \
--enable=gosimple \
--enable=unconvert
