#!/bin/sh

# This script uses the standard Go test coverage tools to generate a test coverage report.

# Run tests with coverage enabled and generate coverage profile.
go test -cover -coverprofile=coverage.txt ./...

# Display function-level coverage statistics.
go tool cover -func=coverage.txt
