#!/bin/bash
# The script does automatic checking on a Go package and its sub-packages, including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 2. golint        (https://github.com/golang/lint)
# 3. go vet        (http://golang.org/cmd/vet)
# 4. race detector (http://blog.golang.org/race-detector)
# 5. test coverage (http://blog.golang.org/cover)

set -ex

# Automatic checks
test -z "$(go fmt $(glide novendor) | tee /dev/stderr)"
test -z "$(go vet $(glide novendor) 2>&1 | tee /dev/stderr)"
env GORACE="halt_on_error=1" go test -short -race $(glide novendor)
