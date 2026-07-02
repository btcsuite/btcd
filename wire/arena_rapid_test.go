// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// poisonScriptChunkPools scribbles a poison pattern over chunks sitting in
// the class pools.  Any decoded message that still aliases arena memory
// (rather than owning an exact-size copy of its scripts) will have its
// contents corrupted by this, so re-verifying a message after poisoning
// proves that no arena memory escaped the decode.
func poisonScriptChunkPools() {
	for _, pool := range scriptChunkPools {
		// Pull a handful of chunks out of each pool, poison them, and
		// put them back.  This reaches the chunks released by recent
		// decodes on this P.
		var chunks []*[]byte
		for i := 0; i < 4; i++ {
			chunks = append(chunks, pool.get())
		}
		for _, chunk := range chunks {
			for i := range *chunk {
				(*chunk)[i] = 0xaa
			}
			pool.put(chunk)
		}
	}
}

// genScript returns a rapid generator for a non-nil script of up to maxLen
// bytes.  Zero-length scripts are always non-nil so that decoded messages,
// which never produce nil scripts, compare equal structurally.
func genScript(rt *rapid.T, label string, maxLen int) []byte {
	n := rapid.IntRange(0, maxLen).Draw(rt, label+"Len")
	s := make([]byte, n)
	for i := 0; i < n; i++ {
		s[i] = byte(rapid.IntRange(0, 255).Draw(rt, label+"Byte"))
	}
	return s
}

// genMsgTx generates a structurally valid random transaction.  Script and
// witness sizes are biased small like real traffic but occasionally exceed
// the 16 KiB starting chunk class so the arena growth path is exercised.
func genMsgTx(rt *rapid.T) *MsgTx {
	tx := NewMsgTx(int32(rapid.IntRange(0, 2).Draw(rt, "version")))

	// Most generated scripts are small, but roughly one in ten draws
	// from a range that overflows the first chunk class.
	scriptMax := 512
	if rapid.IntRange(0, 9).Draw(rt, "bigScripts") == 0 {
		scriptMax = 3 * scriptChunkClasses[0]
	}

	numIn := rapid.IntRange(1, 6).Draw(rt, "numIn")
	hasWitness := rapid.Bool().Draw(rt, "hasWitness")
	for i := 0; i < numIn; i++ {
		ti := &TxIn{
			PreviousOutPoint: OutPoint{
				Index: uint32(rapid.IntRange(0, 1<<30).Draw(
					rt, "prevIndex",
				)),
			},
			SignatureScript: genScript(rt, "sigScript", scriptMax),
			Sequence: uint32(
				rapid.IntRange(0, 1<<30).Draw(rt, "sequence"),
			),
		}
		for j := range ti.PreviousOutPoint.Hash {
			ti.PreviousOutPoint.Hash[j] = byte(
				rapid.IntRange(0, 255).Draw(rt, "prevHash"),
			)
		}

		// When the transaction is witnessy every input carries a
		// (possibly empty) non-nil witness stack, matching what the
		// decoder produces.
		if hasWitness {
			numItems := rapid.IntRange(0, 3).Draw(rt, "numWitness")
			ti.Witness = make(TxWitness, numItems)
			for j := 0; j < numItems; j++ {
				ti.Witness[j] = genScript(
					rt, "witnessItem", scriptMax,
				)
			}
		}

		tx.AddTxIn(ti)
	}

	// A witness marker with zero total witness items is rejected by the
	// decoder (and never produced by the encoder), so force at least one
	// item when the transaction claims to be witnessy.
	if hasWitness && !tx.HasWitness() {
		tx.TxIn[0].Witness = TxWitness{
			genScript(rt, "forcedWitness", scriptMax),
		}
	}

	numOut := rapid.IntRange(0, 6).Draw(rt, "numOut")
	for i := 0; i < numOut; i++ {
		tx.AddTxOut(&TxOut{
			Value: int64(
				rapid.IntRange(0, 1<<40).Draw(rt, "value"),
			),
			PkScript: genScript(rt, "pkScript", scriptMax),
		})
	}

	tx.LockTime = uint32(rapid.IntRange(0, 1<<30).Draw(rt, "lockTime"))

	return tx
}

// TestScriptArenaPropertyTxRoundTrip checks two properties over random
// transactions: serialize/deserialize is the identity, and the decoded
// transaction owns all of its memory (poisoning the arena chunk pools after
// the decode must not change it).
func TestScriptArenaPropertyTxRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tx := genMsgTx(rt)

		var wireBuf bytes.Buffer
		require.NoError(rt, tx.Serialize(&wireBuf))

		var decoded MsgTx
		err := decoded.Deserialize(bytes.NewReader(wireBuf.Bytes()))
		require.NoError(rt, err)

		var reserialized bytes.Buffer
		require.NoError(rt, decoded.Serialize(&reserialized))
		require.Equal(rt, wireBuf.Bytes(), reserialized.Bytes())

		// If the decoded transaction aliased arena memory, poisoning
		// the pools would corrupt its scripts and the second
		// serialization would differ.
		poisonScriptChunkPools()

		var afterPoison bytes.Buffer
		require.NoError(rt, decoded.Serialize(&afterPoison))
		require.Equal(rt, wireBuf.Bytes(), afterPoison.Bytes())
	})
}

// TestScriptArenaPropertyBlockRoundTrip checks the same round-trip and
// ownership properties for full blocks, which share a single arena across
// all of their transactions.
func TestScriptArenaPropertyBlockRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		block := &MsgBlock{Header: blockOne.Header}
		numTxs := rapid.IntRange(1, 8).Draw(rt, "numTxs")
		for i := 0; i < numTxs; i++ {
			block.AddTransaction(genMsgTx(rt))
		}

		var wireBuf bytes.Buffer
		require.NoError(rt, block.Serialize(&wireBuf))

		var decoded MsgBlock
		err := decoded.Deserialize(bytes.NewReader(wireBuf.Bytes()))
		require.NoError(rt, err)

		var reserialized bytes.Buffer
		require.NoError(rt, decoded.Serialize(&reserialized))
		require.Equal(rt, wireBuf.Bytes(), reserialized.Bytes())

		poisonScriptChunkPools()

		var afterPoison bytes.Buffer
		require.NoError(rt, decoded.Serialize(&afterPoison))
		require.Equal(rt, wireBuf.Bytes(), afterPoison.Bytes())
	})
}

// TestScriptArenaPropertyAllocator drives the raw allocator with random
// alloc/rewind sequences against a simple model, checking capacity
// accounting and that live allocations never alias one another.
func TestScriptArenaPropertyAllocator(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		startClass := rapid.IntRange(
			0, len(scriptChunkClasses)-1,
		).Draw(rt, "startClass")

		ar := borrowScriptArena(startClass)
		defer ar.release()

		type allocation struct {
			s      []byte
			marker byte
		}
		var live []allocation
		modelUsed := 0

		verifyLive := func() {
			for _, a := range live {
				count := bytes.Count(a.s, []byte{a.marker})
				require.Equal(rt, len(a.s), count)
			}
		}

		steps := rapid.IntRange(1, 60).Draw(rt, "steps")
		for i := 0; i < steps; i++ {
			if rapid.IntRange(0, 9).Draw(rt, "op") == 0 {
				verifyLive()
				ar.rewind()
				live = live[:0]
				modelUsed = 0
				require.LessOrEqual(rt, len(ar.chunks), 1)
				continue
			}

			n := rapid.IntRange(0, 300_000).Draw(rt, "n")
			s, err := ar.alloc(n)

			// The model predicts exactly when allocation fails.
			if n > scriptArenaMaxAlloc-modelUsed {
				require.ErrorIs(
					rt, err, errScriptArenaFull,
				)
				continue
			}
			require.NoError(rt, err)
			require.Len(rt, s, n)
			require.Equal(rt, n, cap(s))
			modelUsed += n
			require.Equal(
				rt, scriptArenaMaxAlloc-modelUsed,
				ar.remaining(),
			)

			marker := byte(len(live) % 251)
			for j := range s {
				s[j] = marker
			}
			live = append(live, allocation{s: s, marker: marker})
		}
		verifyLive()
	})
}

// TestReadTxOutOwnedScript ensures the script returned by the exported
// ReadTxOut owns its memory: the arena used to stage it is recycled when
// ReadTxOut returns, so a script still aliasing arena memory would be
// corrupted by the pool poisoning below.
func TestReadTxOutOwnedScript(t *testing.T) {
	orig := blockOne.Transactions[0].TxOut[0]
	var buf bytes.Buffer
	require.NoError(t, WriteTxOut(&buf, 0, 0, orig))

	var txOut TxOut
	require.NoError(t, ReadTxOut(bytes.NewReader(buf.Bytes()), 0, 0, &txOut))
	require.Equal(t, orig.PkScript, txOut.PkScript)

	poisonScriptChunkPools()

	require.Equal(t, orig.PkScript, txOut.PkScript)
}

// TestScriptArenaReleaseSafety exercises the misuse guards: double release
// is a no-op, and a released arena refuses to allocate even after a rewind.
func TestScriptArenaReleaseSafety(t *testing.T) {
	ar := borrowScriptArena(txScriptChunkClass)
	_, err := ar.alloc(128)
	require.NoError(t, err)

	ar.release()

	// Allocating through a stale reference must fail loudly.
	_, err = ar.alloc(1)
	require.ErrorIs(t, err, errScriptArenaFull)

	// A rewind must not resurrect a released arena.
	ar.rewind()
	_, err = ar.alloc(1)
	require.ErrorIs(t, err, errScriptArenaFull)

	// Double release must be a no-op rather than double-inserting the
	// arena into the pool.
	ar.release()

	// A fresh borrow resets the poisoned state.
	fresh := borrowScriptArena(txScriptChunkClass)
	defer fresh.release()
	s, err := fresh.alloc(32)
	require.NoError(t, err)
	require.Len(t, s, 32)
}
