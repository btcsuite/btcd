// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"fmt"
	"math/rand"
	"testing"

	"github.com/btcsuite/btcd/chainhash/v2"
)

// BenchmarkMsgCFCheckptDecode benchmarks decoding of MsgCFCheckpt messages
// to measure the performance improvements from optimized memory allocation.
func BenchmarkMsgCFCheckptDecode(b *testing.B) {
	pver := ProtocolVersion

	// Test with varying number of headers: 1k, 10k, 100k.
	headerCounts := []int{1000, 10000, 100000}

	for _, numHeaders := range headerCounts {
		b.Run(fmt.Sprintf("headers_%d", numHeaders), func(b *testing.B) {
			var buf bytes.Buffer
			msg := NewMsgCFCheckpt(
				GCSFilterRegular, &chainhash.Hash{}, numHeaders,
			)

			rng := rand.New(rand.NewSource(12345))
			for i := 0; i < numHeaders; i++ {
				hash := chainhash.Hash{}
				rng.Read(hash[:])
				msg.AddCFHeader(&hash)
			}

			err := msg.BtcEncode(&buf, pver, BaseEncoding)
			if err != nil {
				b.Fatal(err)
			}

			encodedMsg := buf.Bytes()

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				r := bytes.NewReader(encodedMsg)

				var msg MsgCFCheckpt
				err := msg.BtcDecode(r, pver, BaseEncoding)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkMsgCFCheckptEncode benchmarks encoding of MsgCFCheckpt messages.
func BenchmarkMsgCFCheckptEncode(b *testing.B) {
	pver := ProtocolVersion

	// Test with varying number of headers: 1k, 10k, 100k.
	headerCounts := []int{1000, 10000, 100000}

	for _, numHeaders := range headerCounts {
		b.Run(fmt.Sprintf("headers_%d", numHeaders), func(b *testing.B) {
			msg := NewMsgCFCheckpt(
				GCSFilterRegular, &chainhash.Hash{}, numHeaders,
			)

			rng := rand.New(rand.NewSource(12345))
			for i := 0; i < numHeaders; i++ {
				hash := chainhash.Hash{}
				rng.Read(hash[:])
				msg.AddCFHeader(&hash)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				var buf bytes.Buffer
				err := msg.BtcEncode(&buf, pver, BaseEncoding)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkMsgCFCheckptDecodeEmpty benchmarks decoding empty checkpoint
// messages to ensure edge cases are handled efficiently.
func BenchmarkMsgCFCheckptDecodeEmpty(b *testing.B) {
	pver := ProtocolVersion

	var buf bytes.Buffer
	msg := NewMsgCFCheckpt(GCSFilterRegular, &chainhash.Hash{}, 0)
	if err := msg.BtcEncode(&buf, pver, BaseEncoding); err != nil {
		b.Fatal(err)
	}
	encodedMsg := buf.Bytes()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := bytes.NewReader(encodedMsg)
		var msg MsgCFCheckpt
		if err := msg.BtcDecode(r, pver, BaseEncoding); err != nil {
			b.Fatal(err)
		}
	}
}

