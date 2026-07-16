// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockcompress

// versionConfig pins, per format version, the dictionary bytes and encoder
// level used for both compression and decompression. This is the single point
// that guarantees determinism: two nodes on the same format version read the
// same versionConfig and therefore produce identical compressed output and
// identical decompressed output.
type versionConfig struct {
	dict  []byte
	level EncoderLevel
}

// registeredVersions is the registry of all format versions a running node can
// read or write. Every historical format version must remain registered here
// forever (with its original dictionary) so that a node can always decode old
// files. Removing a version breaks the cross-version determinism invariant.
//
// Dictionaries are embedded via //go:embed in dictionary_assets.go once
// trained. Until a trained dictionary is available, FormatV1 ships without a
// dictionary; general-purpose zstd yields ~30% on the non-witness stream
// (stripped blocks), and a trained dictionary is expected to push higher (see
// docs/ROADMAP.md, M1). The whole-block ratio (~24%) is not the target —
// witness separation is what delivers the headline reduction.
var registeredVersions = map[FormatVersion]versionConfig{
	FormatV1: {
		dict:  nil, // populated by dictionary_assets.go when a dictionary is trained
		level: LevelDefault,
	},
}
