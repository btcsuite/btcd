#!/bin/bash
# The script does automatic checking on a Go package and its sub-packages, including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 2. golint        (https://github.com/golang/lint)
# 3. go vet        (http://golang.org/cmd/vet)
# 4. gosimple      (https://github.com/dominikh/go-simple)
# 5. unconvert     (https://github.com/mdempsky/unconvert)
# 6. race detector (http://blog.golang.org/race-detector)
# 7. test coverage (http://blog.golang.org/cover)
#
# gometalinter (github.com/alecthomas/gometalinter) is used to run each static
# checker.

set -ex

# Automatic checks
test -z "$(gometalinter --disable-all \
--enable=gofmt \
--enable=golint \
--enable=vet \
--enable=gosimple \
--enable=unconvert \
--deadline=4m $(glide novendor) | grep -v 'ALL_CAPS\|OP_' 2>&1 | tee /dev/stderr)"
env GORACE="halt_on_error=1" go test -v -race -tags rpctest $(glide novendor)

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
