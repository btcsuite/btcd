#!/usr/bin/env bash
set -ex

# The script does automatic checking on a Go package and its sub-packages,
# including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 2. go vet        (http://golang.org/cmd/vet)
# 3. unconvert     (https://github.com/mdempsky/unconvert)
# 4. race detector (http://blog.golang.org/race-detector)

# gometalinter (github.com/alecthomas/gometalinter) is used to run each each
# static checker.

# To run on docker on windows, symlink /mnt/c to /c and then execute the script
# from the repo path under /c.  See:
# https://github.com/Microsoft/BashOnWindows/issues/1854
# for more details.

#Default GOVERSION
GOVERSION=${1:-1.9}
REPO=dcrd

TESTCMD="test -z \"\$(gometalinter --disable-all \
  --enable=gofmt \
  --enable=vet \
  --enable=unconvert \
  --vendor \
  --deadline=10m . 2>&1 | tee /dev/stderr)\"&& \
  env GORACE='halt_on_error=1' go test -short -race \
  -tags rpctest \
  \$(glide novendor)"

if [ $GOVERSION == "local" ]; then
    go get -v github.com/alecthomas/gometalinter; gometalinter --install
    eval $TESTCMD
    exit
fi

DOCKER_IMAGE_TAG=decred-golang-builder-$GOVERSION

docker pull decred/$DOCKER_IMAGE_TAG

docker run --rm -it -v $(pwd):/src decred/$DOCKER_IMAGE_TAG /bin/bash -c "\
  rsync -ra --filter=':- .gitignore'  \
  /src/ /go/src/github.com/decred/$REPO/ && \
  cd github.com/decred/$REPO/ && \
  glide install && \
  go install \$(glide novendor) && \
  $TESTCMD
"

echo "------------------------------------------"
echo "Tests complete."
