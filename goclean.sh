#!/bin/bash
# The script does automatic checking on a Go package and its sub-packages, including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 2. go vet        (http://golang.org/cmd/vet)
# 3. gosimple      (https://github.com/dominikh/go-simple)
# 4. unconvert     (https://github.com/mdempsky/unconvert)
# 5. race detector (http://blog.golang.org/race-detector)
# 6. test coverage (http://blog.golang.org/cover)

# gometalinter (github.com/alecthomas/gometalinter) is used to run each each
# static checker.
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
test -z "$(gometalinter --disable-all \
--enable=gofmt \
--enable=vet \
--enable=gosimple \
--enable=unconvert \
--deadline=4m $linter_targets 2>&1 | tee /dev/stderr)"

env GORACE="halt_on_error=1" go test -short -race $linter_targets
