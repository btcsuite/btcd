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
test -z "$(for package in $(glide novendor); do golint $package; done | grep -v 'ALL_CAPS\|OP_\|NewFieldVal' | tee /dev/stderr)"
test -z "$(go vet $(glide novendor) 2>&1 | tee /dev/stderr)"
env GORACE="halt_on_error=1" go test -v -race $(glide novendor)

# Run test coverage on each subdirectories and merge the coverage profile.

set +x
echo "mode: count" > profile.cov

# Standard go tooling behavior is to ignore dirs with leading underscores.
for dir in $(find . -maxdepth 10 -not -path '.' -not -path './.git*' \
    -not -path '*/_*' -not -path './cmd*' -not -path './release*' \
    -not -path './vendor*' -type d)
do
if ls $dir/*.go &> /dev/null; then
  go test -covermode=count -coverprofile=$dir/profile.tmp $dir
  if [ -f $dir/profile.tmp ]; then
    cat $dir/profile.tmp | tail -n +2 >> profile.cov
    rm $dir/profile.tmp
  fi
fi
done

# To submit the test coverage result to coveralls.io,
# use goveralls (https://github.com/mattn/goveralls)
# goveralls -coverprofile=profile.cov -service=travis-ci
