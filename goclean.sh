#!/bin/bash
# The script does automatic checking on a Go package and its sub-packages, including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 3. go vet        (http://golang.org/cmd/vet)
# 4. gosimple      (https://github.com/dominikh/go-simple)
# 5. unconvert     (https://github.com/mdempsky/unconvert)
#

set -ex

# Check if we have gcc, if not then use clang, else exit. More compilers can be
# added if needed.
if command -v gcc &> /dev/null
then

    export CC=gcc

elif command -v clang &> /dev/null
then

    export CC=clang

else

    echo "neither gcc nor clang are in the PATH (they may not be installed),
    btcd will not compile due to cgo requiring a C compiler, exiting..."
    exit

fi

if ! command -v golangci-lint &> /dev/null
then
    echo "golangci-lint is not in the PATH, it will not run, exiting..."
    exit
fi

go test -tags="rpctest" ./...

# Automatic checks
golangci-lint run --deadline=10m --disable-all \
--enable=gofmt \
--enable=vet \
--enable=gosimple \
--enable=unconvert
