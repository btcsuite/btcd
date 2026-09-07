// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"testing"
)

func benchmarkCountPayload(b *testing.B, prefix []byte, count uint64) []byte {
	b.Helper()

	var payload bytes.Buffer
	if _, err := payload.Write(prefix); err != nil {
		b.Fatalf("unable to write payload prefix: %v", err)
	}
	if err := WriteVarInt(&payload, ProtocolVersion, count); err != nil {
		b.Fatalf("unable to write element count: %v", err)
	}

	return payload.Bytes()
}

// BenchmarkTruncatedDecodeBudget measures allocation when a length or element
// count is present without the bytes needed to satisfy it.
func BenchmarkTruncatedDecodeBudget(b *testing.B) {
	b.Run("message_payload", func(b *testing.B) {
		header := makeHeader(
			MainNet, CmdBlock, MaxProtocolMessageLength, 0,
		)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _, _, err := ReadMessageN(
				bytes.NewReader(header), ProtocolVersion, MainNet,
			)
			if err == nil {
				b.Fatal("expected a truncated payload error")
			}
		}
	})

	b.Run("variable_bytes", func(b *testing.B) {
		payload := benchmarkCountPayload(
			b, nil, MaxProtocolMessageLength,
		)
		payload = append(payload, 0x01)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, err := ReadVarBytes(
				bytes.NewReader(payload), ProtocolVersion,
				MaxMessagePayload, "payload",
			)
			if err == nil {
				b.Fatal("expected a truncated byte payload error")
			}
		}
	})

	b.Run("inventory", func(b *testing.B) {
		payload := benchmarkCountPayload(b, nil, MaxInvPerMsg)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var msg MsgInv
			err := msg.BtcDecode(
				bytes.NewReader(payload), ProtocolVersion, BaseEncoding,
			)
			if err == nil {
				b.Fatal("expected a truncated inventory error")
			}
		}
	})

	b.Run("merkle_hashes", func(b *testing.B) {
		payload := benchmarkCountPayload(
			b, make([]byte, MaxBlockHeaderPayload+4), maxTxPerBlock,
		)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var msg MsgMerkleBlock
			err := msg.BtcDecode(
				bytes.NewReader(payload), ProtocolVersion, BaseEncoding,
			)
			if err == nil {
				b.Fatal("expected a truncated merkle block error")
			}
		}
	})

	b.Run("transaction_inputs", func(b *testing.B) {
		payload := benchmarkCountPayload(
			b, make([]byte, 4), maxTxInPerMessage,
		)

		// Prewarm the script arena so the comparison isolates the count-sized
		// transaction input allocation changed by this patch.
		ar := borrowScriptArena(txScriptChunkClass)
		ar.release()

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var msg MsgTx
			err := msg.BtcDecode(
				bytes.NewReader(payload), ProtocolVersion, WitnessEncoding,
			)
			if err == nil {
				b.Fatal("expected a truncated transaction input error")
			}
		}
	})
}
