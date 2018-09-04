// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package version provides a single location to house the version information
// for dcrd and other utilities provided in the same repository.
package version

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	// semanticAlphabet defines the allowed characters for the pre-release
	// portion of a semantic version string.
	semanticAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-"

	// semanticBuildAlphabet defines the allowed characters for the build
	// portion of a semantic version string.
	semanticBuildAlphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-."
)

// These constants define the application version and follow the semantic
// versioning 2.0.0 spec (http://semver.org/).
const (
	Major uint = 1
	Minor uint = 4
	Patch uint = 0
)

var (
	// PreRelease is defined as a variable so it can be overridden during the
	// build process with:
	// '-ldflags "-X github.com/decred/dcrd/internal/version.PreRelease=foo"'
	// if needed.  It MUST only contain characters from semanticAlphabet per
	// the semantic versioning spec.
	PreRelease = "pre"

	// BuildMetadata is defined as a variable so it can be overridden during the
	// build process with:
	// '-ldflags "-X github.com/decred/dcrd/internal/version.BuildMetadata=foo"'
	// if needed.  It MUST only contain characters from semanticBuildAlphabet
	// per the semantic versioning spec.
	BuildMetadata = "dev"
)

// String returns the application version as a properly formed string per the
// semantic versioning 2.0.0 spec (http://semver.org/).
func String() string {
	// Start with the major, minor, and patch versions.
	version := fmt.Sprintf("%d.%d.%d", Major, Minor, Patch)

	// Append pre-release version if there is one.  The hyphen called for
	// by the semantic versioning spec is automatically appended and should
	// not be contained in the pre-release string.  The pre-release version
	// is not appended if it contains invalid characters.
	preRelease := NormalizePreRelString(PreRelease)
	if preRelease != "" {
		version = fmt.Sprintf("%s-%s", version, preRelease)
	}

	// Append build metadata if there is any.  The plus called for
	// by the semantic versioning spec is automatically appended and should
	// not be contained in the build metadata string.  The build metadata
	// string is not appended if it contains invalid characters.
	build := NormalizeBuildString(BuildMetadata)
	if build != "" {
		version = fmt.Sprintf("%s+%s", version, build)
	}

	return version
}

// normalizeSemString returns the passed string stripped of all characters
// which are not valid according to the provided semantic versioning alphabet.
func normalizeSemString(str, alphabet string) string {
	var result bytes.Buffer
	for _, r := range str {
		if strings.ContainsRune(alphabet, r) {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// NormalizePreRelString returns the passed string stripped of all characters
// which are not valid according to the semantic versioning guidelines for
// pre-release strings.  In particular they MUST only contain characters in
// semanticAlphabet.
func NormalizePreRelString(str string) string {
	return normalizeSemString(str, semanticAlphabet)
}

// NormalizeBuildString returns the passed string stripped of all characters
// which are not valid according to the semantic versioning guidelines for build
// metadata strings.  In particular they MUST only contain characters in
// semanticBuildAlphabet.
func NormalizeBuildString(str string) string {
	return normalizeSemString(str, semanticBuildAlphabet)
}
