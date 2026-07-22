// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import "testing"

// TestNormalizeVerString ensures valid semantic version separators are
// preserved when normalizing pre-release and build metadata strings.
func TestNormalizeVerString(t *testing.T) {
	t.Parallel()

	const version = "beta.rc1"
	if got := normalizeVerString(version); got != version {
		t.Fatalf("unexpected normalized version: got %q, want %q", got,
			version)
	}
}
