// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"testing"
)

// FuzzTxDecode feeds arbitrary bytes to the transaction decoder, which
// stages script data in the decode arena.  Inputs that decode successfully
// must round-trip: re-serializing and re-decoding yields the same bytes,
// which would fail if any script aliased recycled arena memory or if the
// arena handed out overlapping allocations.
func FuzzTxDecode(f *testing.F) {
	// Seed with existing test vectors: the block one coinbase, the
	// multi input/output transaction, and a witness transaction.
	var seed bytes.Buffer
	if err := blockOne.Transactions[0].Serialize(&seed); err != nil {
		f.Fatal(err)
	}
	f.Add(seed.Bytes())

	seed.Reset()
	if err := multiTx.Serialize(&seed); err != nil {
		f.Fatal(err)
	}
	f.Add(seed.Bytes())

	f.Add(multiWitnessTxEncoded)

	f.Fuzz(func(t *testing.T, data []byte) {
		var tx MsgTx
		if err := tx.Deserialize(bytes.NewReader(data)); err != nil {
			return
		}

		var first bytes.Buffer
		if err := tx.Serialize(&first); err != nil {
			t.Fatalf("serialize after decode: %v", err)
		}

		var again MsgTx
		err := again.Deserialize(bytes.NewReader(first.Bytes()))
		if err != nil {
			t.Fatalf("re-decode of serialized tx: %v", err)
		}

		var second bytes.Buffer
		if err := again.Serialize(&second); err != nil {
			t.Fatalf("serialize after re-decode: %v", err)
		}
		if !bytes.Equal(first.Bytes(), second.Bytes()) {
			t.Fatal("tx serialization is not a fixed point")
		}
	})
}

// FuzzBlockDecode feeds arbitrary bytes to the block decoder, which shares
// a single decode arena across every transaction in the block, rewinding it
// between transactions.  Successful decodes must round-trip byte for byte.
func FuzzBlockDecode(f *testing.F) {
	var seed bytes.Buffer
	if err := blockOne.Serialize(&seed); err != nil {
		f.Fatal(err)
	}
	f.Add(seed.Bytes())

	f.Fuzz(func(t *testing.T, data []byte) {
		var block MsgBlock
		if err := block.Deserialize(bytes.NewReader(data)); err != nil {
			return
		}

		var first bytes.Buffer
		if err := block.Serialize(&first); err != nil {
			t.Fatalf("serialize after decode: %v", err)
		}

		var again MsgBlock
		err := again.Deserialize(bytes.NewReader(first.Bytes()))
		if err != nil {
			t.Fatalf("re-decode of serialized block: %v", err)
		}

		var second bytes.Buffer
		if err := again.Serialize(&second); err != nil {
			t.Fatalf("serialize after re-decode: %v", err)
		}
		if !bytes.Equal(first.Bytes(), second.Bytes()) {
			t.Fatal("block serialization is not a fixed point")
		}
	})
}
