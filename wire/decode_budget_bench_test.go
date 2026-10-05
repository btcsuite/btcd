// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"os"
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

func benchmarkCompleteStreamingMessage(b *testing.B, msg Message) {
	b.Helper()

	var encoded bytes.Buffer
	_, err := WriteMessageWithEncodingN(
		&encoded, msg, ProtocolVersion, MainNet, WitnessEncoding,
	)
	if err != nil {
		b.Fatal(err)
	}
	payload := encoded.Bytes()

	b.SetBytes(int64(len(payload)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		reader := &readSizeRecorder{
			reader: bytes.NewReader(payload),
		}
		_, _, _, err := ReadMessageWithEncodingN(
			reader, ProtocolVersion, MainNet, WitnessEncoding,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkCompleteStreamingMessage measures complete framed messages read
// from a reader that does not expose its remaining length.
func BenchmarkCompleteStreamingMessage(b *testing.B) {
	b.Run("ping", func(b *testing.B) {
		benchmarkCompleteStreamingMessage(b, NewMsgPing(1))
	})

	blockBytes, err := os.ReadFile(
		"testdata/block-00000000000000000021868c2cefc52a480d173c849412fe81c4e5ab806f94ab.blk",
	)
	if err != nil {
		b.Fatal(err)
	}
	var block MsgBlock
	if err := block.Deserialize(bytes.NewReader(blockBytes)); err != nil {
		b.Fatal(err)
	}
	b.Run("block", func(b *testing.B) {
		benchmarkCompleteStreamingMessage(b, &block)
	})
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
		// The 4-byte transaction version is read by MsgTx.BtcDecode before
		// the input count varint.
		payload := benchmarkCountPayload(
			b, make([]byte, 4), maxTxInPerMessage,
		)

		// Prewarm the script arena so the borrow and release pair populates
		// the chunk pool, and the benchmark measures only the count-sized
		// transaction input allocation made while decoding rather than the
		// one-time chunk creation.
		ar := borrowScriptArena()
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

	b.Run("streaming_transaction_inputs", func(b *testing.B) {
		// The 4-byte transaction version is read by MsgTx.BtcDecode before
		// the input count varint.
		payload := benchmarkCountPayload(
			b, make([]byte, 4), maxTxInPerMessage,
		)

		ar := borrowScriptArena()
		ar.release()

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			reader := &readSizeRecorder{
				reader: bytes.NewReader(payload),
			}
			var msg MsgTx
			err := msg.BtcDecode(
				reader, ProtocolVersion, WitnessEncoding,
			)
			if err == nil {
				b.Fatal("expected a truncated transaction input error")
			}
		}
	})

	b.Run("streaming_filter_checkpoints", func(b *testing.B) {
		payload := benchmarkCountPayload(
			b, make([]byte, 33), maxCFHeadersLen,
		)

		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			reader := &readSizeRecorder{
				reader: bytes.NewReader(payload),
			}
			var msg MsgCFCheckpt
			err := msg.BtcDecode(
				reader, ProtocolVersion, BaseEncoding,
			)
			if err == nil {
				b.Fatal("expected a truncated checkpoint error")
			}
		}
	})
}
