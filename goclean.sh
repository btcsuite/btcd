#!/bin/bash
# The script does automatic checking on a Go package and its sub-packages, including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 2. golint        (https://github.com/golang/lint)
# 3. go vet        (http://golang.org/cmd/vet)
# 4. gosimple      (https://github.com/dominikh/go-simple)
# 5. unconvert     (https://github.com/mdempsky/unconvert)
#
# gometalinter (github.com/alecthomas/gometalinter) is used to run each static
# checker.

set -ex

# compare two version numbers against each other
# for example: 1.9.5 > 1.10
function version_gt() {
    test "$(printf '%s\n' "$@" | sort -V | head -n 1)" != "$1";
}

# Make sure glide is installed and $GOPATH/bin is in your path.
# $ go get -u github.com/Masterminds/glide
# $ glide install
if [ ! -x "$(type -p glide)" ]; then
  exit 1
fi

# Make sure gometalinter is installed and $GOPATH/bin is in your path.
# $ go get -v github.com/alecthomas/gometalinter"
# $ gometalinter --install"
if [ ! -x "$(type -p gometalinter.v2)" ]; then
  exit 1
fi

linter_targets=$(glide novendor)

# Automatic checks
test -z "$(gometalinter.v2 -j 4 --disable-all \
--enable=gofmt \
--enable=golint \
--enable=vet \
--enable=gosimple \
--enable=unconvert \
--deadline=10m $linter_targets 2>&1 | grep -v 'ALL_CAPS\|OP_' 2>&1 | tee /dev/stderr)"

# with golang 1.10 we can now run test coverage against multiple packages
# https://golang.org/doc/go1.10#test
if version_gt $TRAVIS_GO_VERSION "1.10"; then
    go test -tags rpctest -covermode=count -coverprofile profile.cov $linter_targets
    go tool cover -func profile.cov
    goveralls -coverprofile=profile.cov -service=travis-ci
else
    go test -tags rpctest $linter_targets
fi
