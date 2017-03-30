#!/bin/bash
# The script does automatic checking on a Go package and its sub-packages, including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 2. golint        (https://github.com/golang/lint)
# 3. go vet        (http://golang.org/cmd/vet)
# 4. goimports     (https://github.com/bradfitz/goimports)
# 5. unconvert     (https://github.com/mdempsky/unconvert)
# 6. test coverage (http://blog.golang.org/cover)

# gometalinter (github.com/alecthomas/gometalinter) is used to run each each
# static checker.

set -e

# Automatic checks
test -z "$(gometalinter -j4 --disable-all \
--enable=gofmt \
--enable=golint \
--enable=vet \
--enable=goimports \
--enable=unconvert \
--deadline=20s --vendor ./... | grep -v 'ALL_CAPS\|OP_' 2>&1 | tee /dev/stderr)"
