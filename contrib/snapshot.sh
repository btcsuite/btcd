#!/bin/sh

read -r -d '' help << EOM
snapshot.sh - helper script for generating snapshot from lbcd's app dir.

The default output name "lbcd_snapshot_<height>_<lbcd_ver>_<date>.tar.zst"

To extract the snapshot (data/mainter/):

    zstd -d lbcd_snapshot_<height>_<lbcd_ver>_<date>.tar.zst | tar xf - -C <appdir>

Default appdir of lbcd on different OSes:

        Darwin)  "\${HOME}/Library/Application Support/Lbcd"
        Linux)   "\${HOME}/.lbcd"
        Windows) "%%LOCALAPPDATA%%/lbcd"

Options:

    -h Display this message.
    -d Specify APPDIR to copy the snapshot from.

    -o Specify the output filename of snapshot.
    -b Specify the best block height of the snapshot. (ignored if -o is specified)
    -l Specify git tag of the running lbcd. (ignored if -o is specified)
    -t Specify the date when the snapshot is generated. (ignored if -o is specified)
EOM

while getopts o:d:b:l:t:h flag
do
    case "${flag}" in
	h) printf "${help}\n\n"; exit 0;;
        d) appdir=${OPTARG};;

        o) snapshot=${OPTARG};;
        b) height=${OPTARG};;
        l) lbcd_ver=${OPTARG};;
        t) date=${OPTARG};;
    esac
done

if [ -z "$appdir" ]; then
    case $(uname) in
        Darwin)  appdir="${HOME}/Library/Application Support/Lbcd" ;;
        Linux)   appdir="${HOME}/.lbcd" ;;
        Windows) appdir="%LOCALAPPDATA%/lbcd" ;;
    esac
fi


if [ -z ${snapshot} ]; then
    git_repo=$(git rev-parse --show-toplevel)
    [ -z "${height}" ] && height=$(go run ${git_repo}/claimtrie/cmd block best --showhash=false)
    [ -z "${lbcd_ver}" ] && lbcd_ver=$(git describe --tags)
    [ -z "${date}" ] && date=$(date  +"%Y-%m-%d")
    snapshot="lbcd_snapshot_${height}_${lbcd_ver}_${date}.tar.zst"
fi


echo "Generating $snapshot ..."

tar c -C "${appdir}" data/mainnet | zstd -9 --no-progress -o "${snapshot}"

