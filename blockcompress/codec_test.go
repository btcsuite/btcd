// Copyright (c) 2025 The btcd developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockcompress

import (
	"bytes"
	"errors"
	"testing"
)

// newTestCodec returns a FormatV1 codec and a cleanup func.
func newTestCodec(t *testing.T) (*Codec, func()) {
	t.Helper()
	c, err := NewCodec(FormatV1)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	return c, func() { c.Close() }
}

// TestRoundTrip verifies the core determinism/losslessness guarantee:
// decompress(compress(x)) == x for representative inputs.
func TestRoundTrip(t *testing.T) {
	c, cleanup := newTestCodec(t)
	defer cleanup()

	cases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte{}},
		{"single byte", []byte{0x00}},
		{"small repetitive", bytes.Repeat([]byte{0x76, 0xa9, 0x14}, 100)},
		{"p2pkh-like", makeP2PKHLike(10)},
		{"1KB random", randBytes(1 << 10)},
		{"64KB random", randBytes(64 << 10)},
		{"1MB random", randBytes(1 << 20)},
		{"4MB mixed", makeMixedBlock(4 << 20)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comp, err := c.CompressBlock(tc.data)
			if err != nil {
				t.Fatalf("CompressBlock: %v", err)
			}
			out, err := c.DecompressBlock(comp)
			if err != nil {
				t.Fatalf("DecompressBlock: %v", err)
			}
			if !bytes.Equal(out, tc.data) {
				t.Fatalf("round-trip mismatch: got len %d, want len %d",
					len(out), len(tc.data))
			}
		})
	}
}

// TestDeterminismAcrossInstances verifies that two codecs bound to the same
// format version produce identical compressed output for the same input. This
// is the on-disk byte-identity property: all nodes on the same version emit
// the same bytes.
func TestDeterminismAcrossInstances(t *testing.T) {
	c1, cleanup1 := newTestCodec(t)
	defer cleanup1()
	c2, cleanup2 := newTestCodec(t)
	defer cleanup2()

	inputs := [][]byte{
		bytes.Repeat([]byte{0x76, 0xa9, 0x14}, 1000),
		makeP2PKHLike(100),
		makeMixedBlock(1 << 20),
		randBytes(1 << 16),
	}

	for i, in := range inputs {
		a, err := c1.CompressBlock(in)
		if err != nil {
			t.Fatalf("c1.CompressBlock[%d]: %v", i, err)
		}
		b, err := c2.CompressBlock(in)
		if err != nil {
			t.Fatalf("c2.CompressBlock[%d]: %v", i, err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("input %d: two codecs produced different compressed "+
				"output (len a=%d, len b=%d) - determinism violated", i,
				len(a), len(b))
		}
	}
}

// TestDeterminismAcrossRuns verifies that repeated compression with the same
// codec produces identical output (no run-to-run nondeterminism from the
// encoder).
func TestDeterminismAcrossRuns(t *testing.T) {
	c, cleanup := newTestCodec(t)
	defer cleanup()

	in := makeMixedBlock(1 << 18)
	first, err := c.CompressBlock(in)
	if err != nil {
		t.Fatalf("CompressBlock: %v", err)
	}
	for i := 0; i < 5; i++ {
		out, err := c.CompressBlock(in)
		if err != nil {
			t.Fatalf("CompressBlock run %d: %v", i, err)
		}
		if !bytes.Equal(out, first) {
			t.Fatalf("run %d produced different output than first run", i)
		}
	}
}

// TestCorruptionDetection verifies that a corrupted compressed stream is
// rejected rather than silently producing wrong bytes. This is the integrity
// guarantee that backs the "deterministic results" concern.
func TestCorruptionDetection(t *testing.T) {
	c, cleanup := newTestCodec(t)
	defer cleanup()

	orig := makeMixedBlock(1 << 16)
	comp, err := c.CompressBlock(orig)
	if err != nil {
		t.Fatalf("CompressBlock: %v", err)
	}

	// Flip a byte in the middle of the compressed payload.
	comp[len(comp)/2] ^= 0xff

	_, err = c.DecompressBlock(comp)
	if err == nil {
		t.Fatalf("decompression of corrupted stream succeeded; expected error")
	}
}

// TestTruncatedStream verifies that a truncated compressed stream is rejected.
func TestTruncatedStream(t *testing.T) {
	c, cleanup := newTestCodec(t)
	defer cleanup()

	orig := makeMixedBlock(1 << 16)
	comp, err := c.CompressBlock(orig)
	if err != nil {
		t.Fatalf("CompressBlock: %v", err)
	}

	// Cut off the last third.
	_, err = c.DecompressBlock(comp[:len(comp)*2/3])
	if err == nil {
		t.Fatalf("decompression of truncated stream succeeded; expected error")
	}
}

// TestUnknownVersion verifies that constructing a codec for an unregistered
// version fails.
func TestUnknownVersion(t *testing.T) {
	_, err := NewCodec(FormatVersion(99))
	if err == nil {
		t.Fatal("NewCodec with unknown version succeeded; expected error")
	}
}

// TestVersionZeroRejected verifies that version 0 (reserved for uncompressed
// legacy files) is not a valid codec version.
func TestVersionZeroRejected(t *testing.T) {
	_, err := NewCodec(0)
	if err == nil {
		t.Fatal("NewCodec(0) succeeded; expected error")
	}
}

// TestNilCodec verifies nil receivers are handled safely.
func TestNilCodec(t *testing.T) {
	var c *Codec
	if _, err := c.CompressBlock([]byte{1}); err == nil {
		t.Fatal("nil codec CompressBlock succeeded; expected error")
	}
	if _, err := c.DecompressBlock([]byte{1}); err == nil {
		t.Fatal("nil codec DecompressBlock succeeded; expected error")
	}
}

// TestMaxEncodedSizeBound verifies the returned bound is never smaller than the
// input (compression worst case) and holds for the max Bitcoin block weight.
func TestMaxEncodedSizeBound(t *testing.T) {
	cases := []int{0, 1, 1 << 10, 1 << 20, 4 << 20}
	for _, n := range cases {
		bound := MaxEncodedSize(n)
		if n > 0 && bound < n {
			t.Errorf("MaxEncodedSize(%d) = %d < input", n, bound)
		}
	}
}

// TestVersionAccessor verifies the codec reports its bound version.
func TestVersionAccessor(t *testing.T) {
	c, cleanup := newTestCodec(t)
	defer cleanup()
	if c.Version() != FormatV1 {
		t.Fatalf("Version = %d, want %d", c.Version(), FormatV1)
	}
}

// TestCloseIdempotent verifies Close is safe to call multiple times.
func TestCloseIdempotent(t *testing.T) {
	c, err := NewCodec(FormatV1)
	if err != nil {
		t.Fatalf("NewCodec: %v", err)
	}
	c.Close()
	c.Close() // must not panic
}

// --- helpers ---

// makeP2PKHLike produces synthetic data with the structure of P2PKH script
// output scripts: a fixed wrapper around random 20-byte hashes. This is the
// kind of repetitive structure zstd exploits on real Bitcoin blocks.
func makeP2PKHLike(n int) []byte {
	var buf []byte
	for i := 0; i < n; i++ {
		// OP_DUP OP_HASH160 PUSH20 <20-byte hash> OP_EQUALVERIFY OP_CHECKSIG
		buf = append(buf, 0x76, 0xa9, 0x14)
		buf = append(buf, randN(20)...)
		buf = append(buf, 0x88, 0xac)
		// amount: 8-byte little-endian, clustered small values
		buf = append(buf, byte(i&0xff), byte((i>>8)&0x0f), 0, 0, 0, 0, 0, 0)
	}
	return buf
}

// makeMixedBlock produces a synthetic Bitcoin-block-like buffer: a header,
// many transactions with P2PKH/P2WPKH outputs and random inputs, mimicking the
// compressibility profile of a real block.
func makeMixedBlock(size int) []byte {
	buf := make([]byte, 0, size)
	// 80-byte header (mostly incompressible).
	buf = append(buf, randN(80)...)
	for len(buf) < size {
		// tx version (4) + varint input count.
		buf = append(buf, randN(4)...)
		buf = append(buf, 0x01) // 1 input
		// outpoint: 32-byte txid (some reuse for compressibility) + 4 vout.
		buf = append(buf, randN(32)...)
		buf = append(buf, 0x00, 0x00, 0x00, 0x00)
		// signature script: length-prefixed, repetitive.
		buf = append(buf, 0x47) // PUSH71
		buf = append(buf, randN(71)...)
		// sequence.
		buf = append(buf, 0xff, 0xff, 0xff, 0xff)
		// 2 outputs.
		buf = append(buf, 0x02)
		for o := 0; o < 2; o++ {
			// amount, clustered.
			buf = append(buf, byte(len(buf)&0xff), byte((len(buf)>>8)&0x0f),
				0, 0, 0, 0, 0, 0)
			if o&1 == 0 {
				// P2PKH: OP_DUP OP_HASH160 PUSH20 <hash> OP_EQUALVERIFY OP_CHECKSIG.
				buf = append(buf, 0x76, 0xa9, 0x14)
				buf = append(buf, randN(20)...)
				buf = append(buf, 0x88, 0xac)
			} else {
				// P2WPKH: OP_0 PUSH20 <hash>.
				buf = append(buf, 0x00, 0x14)
				buf = append(buf, randN(20)...)
			}
		}
		// locktime.
		buf = append(buf, 0x00, 0x00, 0x00, 0x00)
	}
	if len(buf) > size {
		buf = buf[:size]
	}
	return buf
}

// randBytes returns n pseudo-random bytes. Not crypto-secure; fine for tests.
func randBytes(n int) []byte {
	return randN(n)
}

// randN returns n deterministic-ish pseudo-random bytes from a local generator
// so tests are reproducible.
var randState uint64 = 0x12345678abcdef

func randN(n int) []byte {
	b := make([]byte, n)
	for i := 0; i < n; i++ {
		// xorshift64.
		randState ^= randState << 13
		randState ^= randState >> 7
		randState ^= randState << 17
		b[i] = byte(randState)
	}
	return b
}

// Ensure errors.Is is referenced (used by callers; kept imported to avoid
// unused-import churn if future tests add wrapped-error checks).
var _ = errors.Is
