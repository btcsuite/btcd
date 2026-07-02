// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"compress/bzip2"
	"io"
	"os"
	"testing"
)

// smallTxBytes returns the serialization of a typical small transaction,
// mirroring the payload used by BenchmarkDeserializeTxSmall.
func smallTxBytes() []byte {
	return []byte{
		0x01, 0x00, 0x00, 0x00, // Version
		0x01, // Varint for number of input transactions
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Previous output hash
		0xff, 0xff, 0xff, 0xff, // Previous output index
		0x07,                                     // Varint for length of signature script
		0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, // Signature script
		0xff, 0xff, 0xff, 0xff, // Sequence
		0x01,                                           // Varint for number of output transactions
		0x00, 0xf2, 0x05, 0x2a, 0x01, 0x00, 0x00, 0x00, // Transaction amount
		0x43, // Varint for length of pk script
		0x41, // OP_DATA_65
		0x04, 0x96, 0xb5, 0x38, 0xe8, 0x53, 0x51, 0x9c,
		0x72, 0x6a, 0x2c, 0x91, 0xe6, 0x1e, 0xc1, 0x16,
		0x00, 0xae, 0x13, 0x90, 0x81, 0x3a, 0x62, 0x7c,
		0x66, 0xfb, 0x8b, 0xe7, 0x94, 0x7b, 0xe6, 0x3c,
		0x52, 0xda, 0x75, 0x89, 0x37, 0x95, 0x15, 0xd4,
		0xe0, 0xa6, 0x04, 0xf8, 0x14, 0x17, 0x81, 0xe6,
		0x22, 0x94, 0x72, 0x11, 0x66, 0xbf, 0x62, 0x1e,
		0x73, 0xa8, 0x2c, 0xbf, 0x23, 0x42, 0xc8, 0x58,
		0xee,                   // 65-byte signature
		0xac,                   // OP_CHECKSIG
		0x00, 0x00, 0x00, 0x00, // Lock time
	}
}

// BenchmarkDeserializeTxSmallParallel measures small transaction decoding
// with all CPUs decoding concurrently, which models many peers relaying
// mempool transactions at once.  This stresses contention on the shared
// script staging pool as well as the transient memory borrowed per decode.
func BenchmarkDeserializeTxSmallParallel(b *testing.B) {
	buf := smallTxBytes()

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		r := bytes.NewReader(buf)
		var tx MsgTx
		for pb.Next() {
			r.Seek(0, 0)
			if err := tx.Deserialize(r); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkDeserializeTxLargeParallel measures concurrent decoding of a
// very large transaction, exercising the staging buffer growth path from
// every CPU at once.
func BenchmarkDeserializeTxLargeParallel(b *testing.B) {
	fi, err := os.Open("testdata/megatx.bin.bz2")
	if err != nil {
		b.Fatalf("Failed to read transaction data: %v", err)
	}
	defer fi.Close()
	buf, err := io.ReadAll(bzip2.NewReader(fi))
	if err != nil {
		b.Fatalf("Failed to read transaction data: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		r := bytes.NewReader(buf)
		var tx MsgTx
		for pb.Next() {
			r.Seek(0, 0)
			if err := tx.Deserialize(r); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// BenchmarkDeserializeBlockParallel measures concurrent decoding of a full
// mainnet block from every CPU at once, which models parallel block
// download during initial block sync.
func BenchmarkDeserializeBlockParallel(b *testing.B) {
	buf, err := os.ReadFile(
		"testdata/block-00000000000000000021868c2cefc52a480d173c849412fe81c4e5ab806f94ab.blk",
	)
	if err != nil {
		b.Fatalf("Failed to read block data: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		r := bytes.NewReader(buf)
		var block MsgBlock
		for pb.Next() {
			r.Seek(0, 0)
			if err := block.Deserialize(r); err != nil {
				b.Fatal(err)
			}
		}
	})
}
