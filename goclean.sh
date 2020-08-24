#!/bin/bash
# The script does automatic checking on a Go package and its sub-packages, including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 3. go vet        (http://golang.org/cmd/vet)
# 4. gosimple      (https://github.com/dominikh/go-simple)
# 5. unconvert     (https://github.com/mdempsky/unconvert)
# 6. race detector (http://blog.golang.org/race-detector)
# 7. test coverage (http://blog.golang.org/cover)

set -ex

env GORACE="halt_on_error=1" go test -race -tags="rpctest" -covermode atomic -coverprofile=profile.cov ./...

# Automatic checks
golangci-lint run --deadline=10m --disable-all \
--enable=gofmt \
--enable=vet \
--enable=gosimple \
--enable=unconvert
