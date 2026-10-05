// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"fmt"
	"math/rand"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestScriptArenaAlloc ensures basic allocation invariants: exact lengths,
// capacity clamped to length, and no overlap between consecutive
// allocations.
func TestScriptArenaAlloc(t *testing.T) {
	ar := borrowScriptArena()
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
		require.Equal(
			t, len(s), bytes.Count(s, []byte{byte(marker)}),
		)
	}
}

// TestScriptArenaGrowth ensures the arena walks up the chunk size class
// ladder as demand grows and that a single allocation larger than the next
// class skips ahead to a class that fits it.
func TestScriptArenaGrowth(t *testing.T) {
	ar := borrowScriptArena()
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

// TestScriptArenaAllocNegativeSize ensures a negative allocation size is
// rejected with errScriptArenaFull.  The decode path never passes one, but
// a caller that did would otherwise panic on slice bounds since the
// capacity check only compares against the remaining budget.
func TestScriptArenaAllocNegativeSize(t *testing.T) {
	ar := borrowScriptArena()
	defer ar.release()

	for _, size := range []int{-1, -58, -(8 << 20)} {
		_, err := ar.alloc(size)
		require.ErrorIs(t, err, errScriptArenaFull, "size %d", size)
	}

	// Rejected allocations must not consume any of the budget.
	require.Equal(t, scriptArenaMaxAlloc, ar.remaining())
}

// TestScriptArenaCapacity ensures the arena enforces the per-transaction
// staging limit that the fixed slab previously provided.
func TestScriptArenaCapacity(t *testing.T) {
	ar := borrowScriptArena()
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
	ar := borrowScriptArena()
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

	// The allocation must be served from the retained chunk itself, not
	// from a copy that merely holds equal bytes.
	require.Same(t, &largest[0], &s[0])
}

// TestBlockArenaTransactionReuse ensures block staging depends on the largest
// transaction rather than the aggregate scripts across the block.
func TestBlockArenaTransactionReuse(t *testing.T) {
	for _, locations := range []bool{false, true} {
		name := "deserialize"
		if locations {
			name = "transaction_locations"
		}

		t.Run(name, func(t *testing.T) {
			original := scriptChunkPools
			t.Cleanup(func() {
				scriptChunkPools = original
			})

			var allocated [len(scriptChunkClasses)]int
			for i, size := range scriptChunkClasses {
				scriptChunkPools[i] = &chunkClassPool{
					fixed: make(
						chan *[]byte, scriptChunkFixedCaps[i],
					),
					pool: sync.Pool{New: func() interface{} {
						allocated[i]++
						chunk := make([]byte, size)
						return &chunk
					}},
				}
			}

			block := MsgBlock{Header: blockOne.Header}
			for i := 0; i < 1000; i++ {
				block.AddTransaction(blockOne.Transactions[0])
			}

			var encoded bytes.Buffer
			require.NoError(t, block.Serialize(&encoded))

			var decoded MsgBlock
			if locations {
				locs, err := decoded.DeserializeTxLoc(
					bytes.NewBuffer(encoded.Bytes()),
				)
				require.NoError(t, err)
				require.Len(t, locs, len(block.Transactions))
			} else {
				require.NoError(t, decoded.Deserialize(
					bytes.NewReader(encoded.Bytes()),
				))
			}

			var roundTrip bytes.Buffer
			require.NoError(t, decoded.Serialize(&roundTrip))
			require.Equal(t, encoded.Bytes(), roundTrip.Bytes())

			// The whole block must be staged by a single smallest-class
			// chunk: one borrow serves every transaction.
			for i, count := range allocated {
				want := 0
				if i == 0 {
					want = 1
				}
				require.Equal(t, want, count, "class %d", i)
			}
		})
	}
}

// TestScriptArenaDecodeGrowth decodes a transaction whose script data
// overflows the starting chunk class for standalone transactions, ensuring
// the decode path grows the arena transparently.
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
					errs <- fmt.Errorf("cross-arena " +
						"aliasing: decoded script " +
						"mismatch")
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
		ar := borrowScriptArena()

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
				require.Equal(
					t, len(s),
					bytes.Count(s, []byte{byte(marker)}),
				)
			}

			ar.rewind()
			require.LessOrEqual(t, len(ar.chunks), 1)
		}

		ar.release()
	}
}
