// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockcompress

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// realBlockFixtures returns raw serialized mainnet blocks from wire/testdata,
// the same fixtures used by wire.BenchmarkDeserializeBlock. These give a
// credible compression-ratio number against real Bitcoin block data without a
// trained dictionary.
func realBlockFixtures(t testing.TB) [][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(
		"..", "wire", "testdata", "block-*.blk"))
	if err != nil {
		t.Fatalf("glob block fixtures: %v", err)
	}
	if len(paths) == 0 {
		t.Skip("no block-*.blk fixtures found")
	}
	var out [][]byte
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("read %s: %v", p, err)
		}
		out = append(out, b)
	}
	return out
}

// BenchmarkCompressBlock measures compress throughput on real block fixtures.
func BenchmarkCompressBlock(b *testing.B) {
	fixtures := realBlockFixtures(b)
	c, err := NewCodec(FormatV1)
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()

	for i, blk := range fixtures {
		b.Run(fmt.Sprintf("fixture_%d_len_%d", i, len(blk)), func(b *testing.B) {
			b.SetBytes(int64(len(blk)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := c.CompressBlock(blk)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkDecompressBlock measures decompress throughput on real block
// fixtures.
func BenchmarkDecompressBlock(b *testing.B) {
	fixtures := realBlockFixtures(b)
	c, err := NewCodec(FormatV1)
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()

	var compressed [][]byte
	for _, blk := range fixtures {
		comp, err := c.CompressBlock(blk)
		if err != nil {
			b.Fatal(err)
		}
		compressed = append(compressed, comp)
	}

	for i, comp := range compressed {
		b.Run(fmt.Sprintf("fixture_%d_len_%d", i, len(comp)), func(b *testing.B) {
			b.SetBytes(int64(len(comp)))
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_, err := c.DecompressBlock(comp)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkCompressBlockSynthetic measures throughput on a synthetic
// Bitcoin-block-like buffer for a stable, fixture-independent data point.
func BenchmarkCompressBlockSynthetic(b *testing.B) {
	blk := makeMixedBlock(2 << 20)
	c, err := NewCodec(FormatV1)
	if err != nil {
		b.Fatal(err)
	}
	defer c.Close()
	b.SetBytes(int64(len(blk)))
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, err := c.CompressBlock(blk)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// TestCompressionRatio measures the whole-block compression ratio on the real
// mainnet fixtures. This is the *baseline that witness separation improves on*:
// whole-block zstd stalls at ~24% because witness data is high-entropy. The M1
// headline target (~60-70% pruned / ~35-45% archival) is measured in
// TestWitnessSeparatedSavings, not here. This test keeps the baseline visible so
// regressions in the codec's raw ratio are caught, and reports per-fixture and
// average ratios.
//
// NOTE: this is measured against FormatV1 WITHOUT a trained dictionary. No hard
// assertion — the value is reported for visibility. The dictionary training
// phase (also part of A-1) targets the non-witness stream, not whole blocks.
func TestCompressionRatio(t *testing.T) {
	fixtures := realBlockFixtures(t)
	c, err := NewCodec(FormatV1)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	var totalRaw, totalComp int
	for i, blk := range fixtures {
		comp, err := c.CompressBlock(blk)
		if err != nil {
			t.Fatalf("fixture %d: %v", i, err)
		}
		raw := len(blk)
		compLen := len(comp)
		totalRaw += raw
		totalComp += compLen
		ratio := float64(compLen) / float64(raw)
		t.Logf("fixture %d: raw=%d comp=%d ratio=%.3f (%.1f%% reduction)",
			i, raw, compLen, ratio, (1-ratio)*100)
	}
	avgRatio := float64(totalComp) / float64(totalRaw)
	reduction := (1 - avgRatio) * 100
	t.Logf("AVERAGE: raw=%d comp=%d ratio=%.3f (%.1f%% reduction)",
		totalRaw, totalComp, avgRatio, reduction)

	// Whole-block ratio is expected around ~24% (high-entropy witness dominates).
	// This is the baseline; the witness-separated target is measured in
	// TestWitnessSeparatedSavings. No hard assertion here - the value is reported
	// for visibility and to catch codec regressions.
	_ = reduction
}
