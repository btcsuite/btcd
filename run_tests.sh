#!/usr/bin/env bash

# usage:
# ./run_tests.sh                         # local, go 1.11
# GOVERSION=1.10 ./run_tests.sh          # local, go 1.10 (vgo)
# ./run_tests.sh docker                  # docker, go 1.11
# GOVERSION=1.10 ./run_tests.sh docker   # docker, go 1.10 (vgo)
# ./run_tests.sh podman                  # podman, go 1.11
# GOVERSION=1.10 ./run_tests.sh podman   # podman, go 1.10 (vgo)

set -ex

# The script does automatic checking on a Go package and its sub-packages,
# including:
# 1. gofmt         (http://golang.org/cmd/gofmt/)
# 2. gosimple      (https://github.com/dominikh/go-simple)
# 3. unconvert     (https://github.com/mdempsky/unconvert)
# 4. ineffassign   (https://github.com/gordonklaus/ineffassign)
# 5. race detector (http://blog.golang.org/race-detector)

# gometalinter (github.com/alecthomas/gometalinter) is used to run each each
# static checker.

# To run on docker on windows, symlink /mnt/c to /c and then execute the script
# from the repo path under /c.  See:
# https://github.com/Microsoft/BashOnWindows/issues/1854
# for more details.

# Default GOVERSION
[[ ! "$GOVERSION" ]] && GOVERSION=1.11
REPO=dcrd

testrepo () {
  GO=go
  if [[ $GOVERSION == 1.10 ]]; then
    GO=vgo
  fi

  $GO version

  # binary needed for RPC tests
  env CC=gcc $GO build
  cp "$REPO" "$GOPATH/bin/"

  # run tests on all modules
  ROOTPATH=$($GO list -m -f {{.Dir}} 2>/dev/null)
  ROOTPATHPATTERN=$(echo $ROOTPATH | sed 's/\\/\\\\/g' | sed 's/\//\\\//g')
  MODPATHS=$($GO list -m -f {{.Dir}} all 2>/dev/null | grep "^$ROOTPATHPATTERN"\
    | sed -e "s/^$ROOTPATHPATTERN//" -e 's/^\\\|\///')
  MODPATHS=". $MODPATHS"
  for module in $MODPATHS; do
    echo "==> ${module}"
    (cd $module && env GORACE='halt_on_error=1' CC=gcc $GO test -short -race \
	  -tags rpctest ./...)
  done

  # check linters
  if [[ $GOVERSION != 1.10 ]]; then
    # linters do not work with modules yet
    go mod vendor
    unset GO111MODULE

    gometalinter --vendor --disable-all --deadline=10m \
      --enable=gofmt \
      --enable=gosimple \
      --enable=unconvert \
      --enable=ineffassign \
      ./...
    if [ $? != 0 ]; then
      echo 'gometalinter has some complaints'
      exit 1
    fi
  fi

  echo "------------------------------------------"
  echo "Tests completed successfully!"
}

DOCKER=
[[ "$1" == "docker" || "$1" == "podman" ]] && DOCKER=$1
if [ ! "$DOCKER" ]; then
    testrepo
    exit
fi

# use Travis cache with docker
DOCKER_IMAGE_TAG=decred-golang-builder-$GOVERSION
mkdir -p ~/.cache
if [ -f ~/.cache/$DOCKER_IMAGE_TAG.tar ]; then
  # load via cache
  $DOCKER load -i ~/.cache/$DOCKER_IMAGE_TAG.tar
else
  # pull and save image to cache
  $DOCKER pull decred/$DOCKER_IMAGE_TAG
  $DOCKER save decred/$DOCKER_IMAGE_TAG > ~/.cache/$DOCKER_IMAGE_TAG.tar
fi

$DOCKER run --rm -it -v $(pwd):/src:Z decred/$DOCKER_IMAGE_TAG /bin/bash -c "\
  rsync -ra --filter=':- .gitignore'  \
  /src/ /go/src/github.com/decred/$REPO/ && \
  cd github.com/decred/$REPO/ && \
  env GOVERSION=$GOVERSION GO111MODULE=on bash run_tests.sh"
