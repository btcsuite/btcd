// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import "testing"

// TestNormalizeVerString ensures valid semantic version separators are
// preserved, and that characters outside the semantic alphabet are still
// filtered out.
func TestNormalizeVerString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			// Dotted pre-release identifier, as used by the
			// release candidate tags.
			name: "dotted pre-release identifier",
			in:   "beta.rc1",
			want: "beta.rc1",
		},
		{
			// Dotted build metadata, which appBuild may be set to
			// via -ldflags at build time.
			name: "dotted build metadata",
			in:   "exp.sha.5114f85",
			want: "exp.sha.5114f85",
		},
		{
			// Guards against the filter being dropped entirely:
			// an identity function satisfies the cases above.
			name: "character outside the alphabet",
			in:   "beta!rc1",
			want: "betarc1",
		},
	}

	for _, test := range tests {
		got := normalizeVerString(test.in)
		if got != test.want {
			t.Fatalf("%s: unexpected normalized version: got %q, "+
				"want %q", test.name, got, test.want)
		}
	}
}
