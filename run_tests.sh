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

TESTDIRS=$(go list ./... | grep -v '/vendor/')
TESTCMD="test -z \"\$(gometalinter --vendor --disable-all \
  --enable=gofmt \
  --enable=vet \
  --enable=unconvert \
  --deadline=10m ./... 2>&1 | tee /dev/stderr)\"&& \
  env GORACE='halt_on_error=1' go test -short -race \
  -tags rpctest \
  \${TESTDIRS}"

if [ $GOVERSION == "local" ]; then
    go get -v github.com/alecthomas/gometalinter; gometalinter --install
    eval $TESTCMD
    exit
fi

DOCKER_IMAGE_TAG=decred-golang-builder-$GOVERSION

docker pull decred/$DOCKER_IMAGE_TAG
if [ $? != 0 ]; then
	echo 'docker pull failed'
	exit 1
fi

TMPFILE=$(mktemp)
docker run --rm -it -v $(pwd):/src decred/$DOCKER_IMAGE_TAG /bin/bash -c "\
  rsync -ra --filter=':- .gitignore'  \
  /src/ /go/src/github.com/decred/$REPO/ && \
  cd github.com/decred/$REPO/ && \
  cp Gopkg.lock $TMPFILE && \
  dep ensure && \
  diff Gopkg.lock $TMPFILE >/dev/null || (echo 'lockfile must be updated with dep ensure'; exit 1) && \
  go install . ./cmd/... && \
  $TESTCMD
"

if [ $? != 0 ]; then
	echo 'docker run failed'
	exit 1
fi

echo "------------------------------------------"
echo "Tests complete."
