// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import "testing"

// TestNormalizeVerString ensures valid semantic version separators are
// preserved when normalizing pre-release and build metadata strings.
func TestNormalizeVerString(t *testing.T) {
	t.Parallel()

	tests := []string{
		// Dotted pre-release identifier, as used by the release
		// candidate tags.
		"beta.rc1",

		// Dotted build metadata, which appBuild may be set to via
		// -ldflags at build time.
		"exp.sha.5114f85",
	}

	for _, version := range tests {
		if got := normalizeVerString(version); got != version {
			t.Fatalf("unexpected normalized version: got %q, "+
				"want %q", got, version)
		}
	}
}
