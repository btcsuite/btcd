#!/bin/bash

# Checks that the repo follows the Go version policy in README.md: CI, the
# Docker images and releases are built with the newest Go release, and every
# go.mod requires the previous release, both at the latest point release of
# their branch.

set -e

cd "$(git rev-parse --show-toplevel)"

# go.dev lists the latest point release of each supported Go branch.
RELEASES=$(curl -fsSL 'https://go.dev/dl/?mode=json' |
    jq -r '.[] | select(.stable) | .version' | sed 's/^go//' | sort -rV)
NEWEST=$(echo "$RELEASES" | sed -n 1p)
PREVIOUS=$(echo "$RELEASES" | sed -n 2p)
if [[ -z $NEWEST || -z $PREVIOUS ]]; then
    echo "Could not read the supported Go releases from go.dev." >&2
    exit 1
fi

STATUS=0
fail() {
    echo "$*" >&2
    STATUS=1
}

# golang_image_version prints the Go version of the golang image with the given
# digest, from the GOLANG_VERSION variable of its linux/amd64 image.
golang_image_version() {
    local registry=https://registry-1.docker.io/v2/library/golang
    local accept="application/vnd.oci.image.index.v1+json,\
application/vnd.docker.distribution.manifest.list.v2+json,\
application/vnd.oci.image.manifest.v1+json,\
application/vnd.docker.distribution.manifest.v2+json"
    local token manifest amd64 config

    token=$(curl -fsSL "https://auth.docker.io/token?\
service=registry.docker.io&scope=repository:library/golang:pull" |
        jq -r .token)
    manifest=$(curl -fsSL -H "Authorization: Bearer $token" \
        -H "Accept: $accept" "$registry/manifests/$1")

    # A multi-platform image lists the manifest of each platform.
    amd64=$(jq -r '.manifests[]? | select(.platform.os == "linux" and
        .platform.architecture == "amd64") | .digest' <<< "$manifest")
    if [[ -n $amd64 ]]; then
        manifest=$(curl -fsSL -H "Authorization: Bearer $token" \
            -H "Accept: $accept" "$registry/manifests/$amd64")
    fi

    config=$(jq -r .config.digest <<< "$manifest")
    curl -fsSL -H "Authorization: Bearer $token" "$registry/blobs/$config" |
        jq -r '.config.Env[] | select(startswith("GOLANG_VERSION=")) |
            ltrimstr("GOLANG_VERSION=")'
}

for mod in $(git ls-files '*go.mod'); do
    got=$(sed -n 's/^go //p' "$mod")
    if [[ $got != "$PREVIOUS" ]]; then
        fail "$mod: requires go $got, want go $PREVIOUS"
    fi
done

got=$(sed -n 's/^ *GO_VERSION: *//p' .github/workflows/main.yml)
if [[ $got != "$NEWEST" ]]; then
    fail ".github/workflows/main.yml: GO_VERSION is $got, want $NEWEST"
fi

# Check the golang image each Dockerfile's FROM line actually uses, rather than
# any text that mentions one: the root Dockerfile pins a bare digest.
for dockerfile in Dockerfile .github/workflows/Dockerfile; do
    images=$(sed -nE 's/^FROM (--platform=[^ ]+ )?(golang[^ ]*).*/\2/p' \
        "$dockerfile")
    if [[ -z $images ]]; then
        fail "$dockerfile: no golang build stage found"
    fi

    for image in $images; do
        # A version tag, as in golang:1.27.1-alpine3.24, has to match.
        if [[ $image =~ ^golang:([^@]+) ]]; then
            tag=${BASH_REMATCH[1]}
            if [[ $tag != "$NEWEST" && $tag != "$NEWEST"-* ]]; then
                fail "$dockerfile: built with golang:$tag, want $NEWEST"
            fi
        fi

        # So does the Go version of the image a digest pins, since the
        # digest is what Docker uses and a tag or a comment next to it can
        # be updated without it.
        if [[ $image =~ @(sha256:[0-9a-f]{64})$ ]]; then
            got=$(golang_image_version "${BASH_REMATCH[1]}")
            if [[ $got != "$NEWEST" ]]; then
                fail "$dockerfile: the pinned golang image has Go" \
                    "$got, want $NEWEST"
            fi
        elif [[ ! $image =~ ^golang: ]]; then
            fail "$dockerfile: $image pins no Go version"
        fi
    done
done

if [[ $STATUS -eq 0 ]]; then
    echo "Go versions follow the policy: built with $NEWEST," \
        "go.mod requires $PREVIOUS."
fi
exit $STATUS
