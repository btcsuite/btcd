// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockcompress

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/wire/v2"
)

// loadRealBlock deserializes a real mainnet .blk fixture into a MsgBlock.
func loadRealBlock(tb testing.TB, path string) *wire.MsgBlock {
	tb.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		tb.Fatalf("read %s: %v", path, err)
	}
	var blk wire.MsgBlock
	if err := blk.Deserialize(bytes.NewReader(raw)); err != nil {
		tb.Fatalf("deserialize %s: %v", path, err)
	}
	return &blk
}

// realBlockPaths returns the wire/testdata block fixtures.
func realBlockPaths(tb testing.TB) []string {
	tb.Helper()
	paths, err := filepath.Glob(filepath.Join("..", "wire", "testdata", "block-*.blk"))
	if err != nil {
		tb.Fatalf("glob: %v", err)
	}
	if len(paths) == 0 {
		tb.Skip("no block fixtures")
	}
	return paths
}

// TestWitnessSeparatedSavings is the go/no-go measurement for the combined
// design: compress the non-witness stream and optionally prune the witness
// stream. It uses btcd's own wire SerializeNoWitness to split blocks, then
// reports three savings figures per fixture and blended:
//
//  1. compress-only:        zstd on the whole block (witness included)
//  2. prune-witness-only:   drop witness, keep stripped block uncompressed
//  3. combined:             drop witness + zstd on the stripped block
//
// The combined figure is the real disk-space headline for the witness-separated
// storage design (see docs/ROADMAP.md, M1).
func TestWitnessSeparatedSavings(t *testing.T) {
	paths := realBlockPaths(t)
	c, err := NewCodec(FormatV1)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	var totRaw, totCompWhole, totStripped, totStrippedComp int

	for i, p := range paths {
		blk := loadRealBlock(t, p)

		// Whole block (with witness), as on disk today.
		var whole bytes.Buffer
		if err := blk.Serialize(&whole); err != nil {
			t.Fatalf("serialize: %v", err)
		}
		raw := whole.Len()

		// Stripped block (no witness), via btcd's existing wire API.
		var stripped bytes.Buffer
		if err := blk.SerializeNoWitness(&stripped); err != nil {
			t.Fatalf("serialize nowitness: %v", err)
		}
		str := stripped.Len()

		// Compressed whole block.
		compWhole, err := c.CompressBlock(whole.Bytes())
		if err != nil {
			t.Fatalf("compress whole: %v", err)
		}

		// Compressed stripped block (the non-witness stream).
		compStripped, err := c.CompressBlock(stripped.Bytes())
		if err != nil {
			t.Fatalf("compress stripped: %v", err)
		}

		witnessBytes := raw - str
		witnessPct := 100.0 * float64(witnessBytes) / float64(raw)
		compOnly := 100.0 * (1 - float64(len(compWhole))/float64(raw))
		pruneOnly := 100.0 * (1 - float64(str)/float64(raw))
		combined := 100.0 * (1 - float64(len(compStripped))/float64(raw))

		t.Logf("fixture %d: raw=%d stripped=%d witness=%d (%.1f%%)",
			i, raw, str, witnessBytes, witnessPct)
		t.Logf("  compress-only:        %.1f%% reduction (-> %d bytes)",
			compOnly, len(compWhole))
		t.Logf("  prune-witness-only:   %.1f%% reduction (-> %d bytes)",
			pruneOnly, str)
		t.Logf("  combined (prune+zstd): %.1f%% reduction (-> %d bytes)",
			combined, len(compStripped))

		totRaw += raw
		totCompWhole += len(compWhole)
		totStripped += str
		totStrippedComp += len(compStripped)
	}

	t.Logf("---- BLENDED across %d fixtures ----", len(paths))
	t.Logf("raw=%d", totRaw)
	t.Logf("compress-only:        %.1f%% reduction",
		100.0*(1-float64(totCompWhole)/float64(totRaw)))
	t.Logf("prune-witness-only:   %.1f%% reduction",
		100.0*(1-float64(totStripped)/float64(totRaw)))
	t.Logf("combined (prune+zstd): %.1f%% reduction",
		100.0*(1-float64(totStrippedComp)/float64(totRaw)))
}
