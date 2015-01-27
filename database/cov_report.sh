#!/bin/sh

# This script uses go tool cover to generate a test coverage report.
go test -coverprofile=cov.out && go tool cover -func=cov.out && rm -f cov.out
echo "============================================================"
(cd ldb && go test -coverprofile=cov.out && go tool cover -func=cov.out && \
  rm -f cov.out)
echo "============================================================"
(cd memdb && go test -coverprofile=cov.out && go tool cover -func=cov.out && \
  rm -f cov.out)
