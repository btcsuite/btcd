#!/bin/bash

VERSION_PROTOC="libprotoc 3.5.1"

# Make sure the correct version of protoc is installed.
LOCAL_PROTOC="$(protoc --version)"
if [ "$LOCAL_PROTOC" != "$VERSION_PROTOC" ]
then
	echo "Wrong protoc version. Got $LOCAL_PROTOC; need $VERSION_PROTOC."
	exit 1
fi

#TODO add version check for 
# - protoc-gen-go
# - protoc-gen-grpc-gateway

# Generate the protos.
protoc -I/usr/local/include -I. \
       -I$GOPATH/src \
       --go_out=plugins=grpc:. \
       btcrpc.proto

