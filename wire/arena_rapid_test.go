// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

// poisonScriptArena scribbles over the exact chunks used by a decode. A
// decoded transaction that still aliases its staging arena will be corrupted.
func poisonScriptArena(ar *scriptArena) {
	for _, chunk := range ar.chunks {
		for i := range *chunk {
			(*chunk)[i] = 0xaa
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
			Value:    rapid.Int64Range(0, 1<<40).Draw(rt, "value"),
			PkScript: genScript(rt, "pkScript", scriptMax),
		})
	}

	tx.LockTime = uint32(rapid.IntRange(0, 1<<30).Draw(rt, "lockTime"))

	return tx
}

// TestScriptArenaPropertyTxRoundTrip checks two properties over random
// transactions: serialize/deserialize is the identity, and the decoded
// transaction owns all of its memory after its exact staging chunks are
// overwritten.
func TestScriptArenaPropertyTxRoundTrip(t *testing.T) {
	rapid.Check(t, func(rt *rapid.T) {
		tx := genMsgTx(rt)

		var wireBuf bytes.Buffer
		require.NoError(rt, tx.Serialize(&wireBuf))

		ar := borrowScriptArena(txScriptChunkClass)
		defer ar.release()

		buf := binarySerializer.Borrow()
		defer binarySerializer.Return(buf)

		var decoded MsgTx
		err := decoded.btcDecode(
			bytes.NewReader(wireBuf.Bytes()), 0, WitnessEncoding, buf, ar,
		)
		require.NoError(rt, err)

		var reserialized bytes.Buffer
		require.NoError(rt, decoded.Serialize(&reserialized))
		require.Equal(rt, wireBuf.Bytes(), reserialized.Bytes())

		// Overwrite the exact chunks used by this decode rather than
		// relying on a later pool lookup to return the same chunks.
		poisonScriptArena(ar)

		var afterPoison bytes.Buffer
		require.NoError(rt, decoded.Serialize(&afterPoison))
		require.Equal(rt, wireBuf.Bytes(), afterPoison.Bytes())
	})
}

// TestScriptArenaPropertyBlockRoundTrip checks round-trip stability for full
// blocks, including the rewinds performed while transactions share an arena.
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

		// Decode the serialized block again to exercise arena reuse, then
		// verify the first decoded block remains stable.
		var again MsgBlock
		err = again.Deserialize(bytes.NewReader(reserialized.Bytes()))
		require.NoError(rt, err)

		var afterReuse bytes.Buffer
		require.NoError(rt, decoded.Serialize(&afterReuse))
		require.Equal(rt, wireBuf.Bytes(), afterReuse.Bytes())
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
// ReadTxOut has an exact-sized backing allocation owned by the output.
func TestReadTxOutOwnedScript(t *testing.T) {
	class := txScriptChunkClass
	size := scriptChunkClasses[class]
	testPool := &chunkClassPool{
		fixed: make(chan *[]byte, 1),
		pool: sync.Pool{
			New: func() interface{} {
				chunk := make([]byte, size)
				return &chunk
			},
		},
	}

	origPool := scriptChunkPools[class]
	scriptChunkPools[class] = testPool
	t.Cleanup(func() {
		scriptChunkPools[class] = origPool
	})

	orig := blockOne.Transactions[0].TxOut[0]
	var buf bytes.Buffer
	require.NoError(t, WriteTxOut(&buf, 0, 0, orig))

	var txOut TxOut
	require.NoError(t, ReadTxOut(bytes.NewReader(buf.Bytes()), 0, 0, &txOut))
	require.Equal(t, orig.PkScript, txOut.PkScript)
	require.Equal(t, len(txOut.PkScript), cap(txOut.PkScript))

	// ReadTxOut releases its staging arena before returning. Pull the exact
	// chunk back out of the isolated pool and overwrite it to ensure the
	// returned script does not alias staging memory.
	chunk := testPool.get()
	for i := range *chunk {
		(*chunk)[i] = 0xaa
	}
	require.Equal(t, orig.PkScript, txOut.PkScript)
}

// TestScriptArenaReleaseSafety exercises the misuse guards within a single
// ownership interval: double release is a no-op, and a released arena refuses
// to allocate even after a rewind.
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
