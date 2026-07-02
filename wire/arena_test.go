// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"math/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestScriptArenaAlloc ensures basic allocation invariants: exact lengths,
// capacity clamped to length, and no overlap between consecutive
// allocations.
func TestScriptArenaAlloc(t *testing.T) {
	ar := borrowScriptArena(txScriptChunkClass)
	defer ar.release()

	sizes := []int{0, 1, 25, 512, 1000, 33}
	allocs := make([][]byte, 0, len(sizes))
	for _, size := range sizes {
		s, err := ar.alloc(size)
		require.NoError(t, err)
		require.Len(t, s, size)
		require.Equal(t, size, cap(s))

		// Fill the allocation with a marker so overlap with any
		// other allocation is detectable below.
		for i := range s {
			s[i] = byte(len(allocs))
		}
		allocs = append(allocs, s)
	}

	// Every allocation must still hold its own marker; a bump allocator
	// bug that handed out overlapping memory would have clobbered one of
	// the earlier allocations.
	for marker, s := range allocs {
		for _, b := range s {
			require.Equal(t, byte(marker), b)
		}
	}
}

// TestScriptArenaGrowth ensures the arena walks up the chunk size class
// ladder as demand grows and that a single allocation larger than the next
// class skips ahead to a class that fits it.
func TestScriptArenaGrowth(t *testing.T) {
	ar := borrowScriptArena(txScriptChunkClass)
	defer ar.release()

	// The first allocation draws the starting class.
	_, err := ar.alloc(100)
	require.NoError(t, err)
	require.Len(t, ar.chunks, 1)
	require.Equal(t, scriptChunkClasses[0], len(*ar.chunks[0]))

	// Exhausting the first chunk must borrow the next class up.
	_, err = ar.alloc(scriptChunkClasses[0])
	require.NoError(t, err)
	require.Len(t, ar.chunks, 2)
	require.Equal(t, scriptChunkClasses[1], len(*ar.chunks[1]))

	// An allocation bigger than the next class in the ladder must skip
	// ahead to one that fits it in a single chunk.
	_, err = ar.alloc(scriptChunkClasses[2] + 1)
	require.NoError(t, err)
	require.Len(t, ar.chunks, 3)
	require.Equal(
		t, scriptChunkClasses[len(scriptChunkClasses)-1],
		len(*ar.chunks[2]),
	)
}

// TestScriptArenaCapacity ensures the arena enforces the per-transaction
// staging limit that the fixed slab previously provided.
func TestScriptArenaCapacity(t *testing.T) {
	ar := borrowScriptArena(blockScriptChunkClass)
	defer ar.release()

	// Fill the arena right up to its capacity.
	_, err := ar.alloc(scriptArenaMaxAlloc - 1)
	require.NoError(t, err)
	require.Equal(t, 1, ar.remaining())

	_, err = ar.alloc(1)
	require.NoError(t, err)
	require.Equal(t, 0, ar.remaining())

	// The next allocation must fail, and a rewind must restore the full
	// capacity.
	_, err = ar.alloc(1)
	require.ErrorIs(t, err, errScriptArenaFull)

	ar.rewind()
	require.Equal(t, scriptArenaMaxAlloc, ar.remaining())

	_, err = ar.alloc(1)
	require.NoError(t, err)
}

// TestScriptArenaRewind ensures rewinding recycles the memory of the largest
// chunk in place and returns the smaller warm-up chunks to their pools.
func TestScriptArenaRewind(t *testing.T) {
	ar := borrowScriptArena(txScriptChunkClass)
	defer ar.release()

	// Grow through two classes.
	_, err := ar.alloc(scriptChunkClasses[0])
	require.NoError(t, err)
	_, err = ar.alloc(scriptChunkClasses[0])
	require.NoError(t, err)
	require.Len(t, ar.chunks, 2)
	largest := *ar.chunks[1]

	ar.rewind()

	// Only the largest chunk survives the rewind and subsequent
	// allocations are served from its start.
	require.Len(t, ar.chunks, 1)
	s, err := ar.alloc(8)
	require.NoError(t, err)
	require.Equal(t, &largest[0], &s[0])
}

// TestScriptArenaDecodeGrowth decodes a transaction whose script data
// overflows the starting chunk class for standalone transactions, ensuring
// the decode path grows the arena transparently rather than erroring like
// the old fixed-buffer bounds check would have.
func TestScriptArenaDecodeGrowth(t *testing.T) {
	// Build a transaction with a signature script comfortably larger
	// than the 16 KiB starting chunk.
	bigScript := make([]byte, 3*scriptChunkClasses[0])
	rng := rand.New(rand.NewSource(1337))
	rng.Read(bigScript)

	tx := NewMsgTx(1)
	tx.AddTxIn(&TxIn{
		PreviousOutPoint: OutPoint{Index: 0xffffffff},
		SignatureScript:  bigScript,
		Sequence:         0xffffffff,
	})
	tx.AddTxOut(NewTxOut(0, []byte{0x51}))

	var buf bytes.Buffer
	require.NoError(t, tx.Serialize(&buf))

	var decoded MsgTx
	require.NoError(t, decoded.Deserialize(bytes.NewReader(buf.Bytes())))
	require.Equal(t, bigScript, decoded.TxIn[0].SignatureScript)
}

// TestScriptArenaConcurrentDecode exercises the chunk pools from many
// goroutines at once to give the race detector a chance to catch unsound
// sharing between arenas.
func TestScriptArenaConcurrentDecode(t *testing.T) {
	// Serialize one small and one multi-input transaction as shared
	// decode inputs.
	var smallTxBuf, multiTxBuf bytes.Buffer
	require.NoError(t, blockOne.Transactions[0].Serialize(&smallTxBuf))
	require.NoError(t, multiTx.Serialize(&multiTxBuf))

	const numWorkers = 16
	const decodesPerWorker = 200

	var wg sync.WaitGroup
	errs := make(chan error, numWorkers)
	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < decodesPerWorker; i++ {
				var tx1, tx2 MsgTx
				err := tx1.Deserialize(
					bytes.NewReader(smallTxBuf.Bytes()),
				)
				if err != nil {
					errs <- err
					return
				}
				err = tx2.Deserialize(
					bytes.NewReader(multiTxBuf.Bytes()),
				)
				if err != nil {
					errs <- err
					return
				}

				// Verify the decoded scripts match the
				// originals so cross-arena aliasing would
				// surface as corruption.
				if !bytes.Equal(
					tx2.TxIn[0].SignatureScript,
					multiTx.TxIn[0].SignatureScript,
				) {
					errs <- errScriptArenaFull
					return
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
}

// TestScriptArenaRandomizedAllocations drives the arena with a deterministic
// random allocation/rewind pattern and checks that live allocations never
// alias each other.
func TestScriptArenaRandomizedAllocations(t *testing.T) {
	rng := rand.New(rand.NewSource(42))

	for round := 0; round < 50; round++ {
		startClass := rng.Intn(len(scriptChunkClasses))
		ar := borrowScriptArena(startClass)

		for rewinds := 0; rewinds < 4; rewinds++ {
			var live [][]byte
			for ar.remaining() > 0 && rng.Float64() > 0.05 {
				n := rng.Intn(64 << 10)
				if n > ar.remaining() {
					n = ar.remaining()
				}

				s, err := ar.alloc(n)
				require.NoError(t, err)
				require.Len(t, s, n)

				marker := byte(len(live))
				for i := range s {
					s[i] = marker
				}
				live = append(live, s)
			}

			// All live allocations must retain their markers.
			for marker, s := range live {
				for _, b := range s {
					require.Equal(t, byte(marker), b)
				}
			}

			ar.rewind()
			require.LessOrEqual(t, len(ar.chunks), 1)
		}

		ar.release()
	}
}
