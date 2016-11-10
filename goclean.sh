#!/bin/bash
# The script does automatic checking on a Go package and its sub-packages, including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 2. goimports     (https://github.com/bradfitz/goimports)
# 3. golint        (https://github.com/golang/lint)
# 4. go vet        (http://golang.org/cmd/vet)
# 5. test coverage (http://blog.golang.org/cover)

# gometaling (github.com/alecthomas/gometalinter) is used to run each each
# static checker.

set -e

# Automatic checks
test -z "$(gometalinter --disable-all \
--enable=gofmt \
--enable=golint \
--enable=vet \
--enable=goimports \
--deadline=20s $(glide novendor) | grep -v 'ALL_CAPS\|OP_' 2>&1 | tee /dev/stderr)"
