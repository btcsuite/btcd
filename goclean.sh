#!/bin/bash
# The script does automatic checking on a Go package and its sub-packages, including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 2. golint        (https://github.com/golang/lint)
# 3. go vet        (http://golang.org/cmd/vet)

set -ex

# Automatic checks
test -z "$(gometalinter -j 8 --disable-all --enable=gofmt --enable=golint --enable=vet --vendor --deadline=10m ./... 2>&1 | egrep -v 'testdata/' | tee /dev/stderr)"

env GORACE="halt_on_error=1" go test -race $(glide nv)
