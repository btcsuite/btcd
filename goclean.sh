#!/bin/bash
# The script does automatic checking on a Go package and its sub-packages, including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 2. golint        (https://github.com/golang/lint)
# 3. go vet        (http://golang.org/cmd/vet)
# 4. gosimple      (https://github.com/dominikh/go-simple)
# 5. unconvert     (https://github.com/mdempsky/unconvert)
# 6. race detector (http://blog.golang.org/race-detector)
#
# gometalinter (github.com/alecthomas/gometalinter) is used to run each static
# checker.

set -ex

# Make sure glide is installed and $GOPATH/bin is in your path.
# $ go get -u github.com/Masterminds/glide
# $ glide install
if [ ! -x "$(type -p glide)" ]; then
  exit 1
fi

# Make sure gometalinter is installed and $GOPATH/bin is in your path.
# $ go get -v github.com/alecthomas/gometalinter"
# $ gometalinter --install"
if [ ! -x "$(type -p gometalinter)" ]; then
  exit 1
fi

linter_targets=$(glide novendor)

# Automatic checks
test -z "$(gometalinter -j 4 --disable-all \
--enable=gofmt \
--enable=golint \
--enable=vet \
--enable=gosimple \
--enable=unconvert \
--deadline=10m $linter_targets 2>&1 | grep -v 'ALL_CAPS\|OP_' 2>&1 | tee /dev/stderr)"
env GORACE="halt_on_error=1" go test -race -tags rpctest $linter_targets
