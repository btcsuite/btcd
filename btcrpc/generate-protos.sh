#!/bin/sh

# Generate the protos.
protoc -I/usr/local/include -I. \
       -I$GOPATH/src \
       --go_out=plugins=grpc:. \
       btcrpc.proto

