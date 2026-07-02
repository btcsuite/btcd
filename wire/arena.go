// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"errors"
	"sync"
)

// This file implements a small bump allocator ("script arena") used to stage
// variable-length scripts and witness items while a transaction is being
// decoded from the wire.  Once a transaction is fully decoded, all of its
// scripts are copied into a single exactly-sized allocation and the arena
// memory becomes dead, so the arena is strictly transient: no arena memory
// ever escapes a decode call.  That property is what makes reuse safe without
// any reference counting.
//
// The arena replaces the previous fixed 4 MiB script slab.  The slab approach
// suffered from memory amplification: decoding even a tiny transaction pinned
// a full 4 MiB buffer for the duration of the decode, a pool miss allocated
// and zeroed 4 MiB, and the channel-based free list could retain hundreds of
// megabytes indefinitely since channels are invisible to the garbage
// collector's pressure heuristics.
//
// The arena instead draws fixed-size chunks from a ladder of size classes
// backed by sync.Pool and bump-allocates scripts out of them.  A decode only
// touches memory proportional to the scripts it actually contains: typical
// transactions are served entirely by a single 16 KiB chunk while full blocks
// walk up to the larger classes.  Because the chunk pools are sync.Pools,
// idle chunks are released back to the runtime across garbage collection
// cycles instead of being pinned forever.

const (
	// scriptArenaMaxAlloc is the maximum total number of bytes that may be
	// bump-allocated from a script arena between rewinds.  Since the arena
	// is rewound for each transaction, this bounds the staged script data
	// of a single transaction.  It intentionally matches the size of the
	// script slab it replaced in order to preserve the existing decode
	// limits: no valid transaction can contain more script data than the
	// maximum block payload, so anything beyond this is malformed.
	scriptArenaMaxAlloc = 1 << 22

	// txScriptChunkClass is the chunk size class used as the starting
	// point when decoding standalone transactions.  See
	// scriptChunkClasses.
	txScriptChunkClass = 0

	// blockScriptChunkClass is the chunk size class used as the starting
	// point when decoding full blocks.  See scriptChunkClasses.
	blockScriptChunkClass = 2
)

// scriptChunkClasses defines the ladder of chunk sizes that back script
// arenas.  The classes were chosen so that the overwhelmingly common cases
// are served by the first chunk borrowed:
//
//   - 16 KiB covers virtually all standalone transactions seen on the
//     network, which typically carry well under 1 KiB of script data.
//   - 128 KiB covers large non-standard transactions (the standardness
//     limit for a transaction is 100 KB).
//   - 1 MiB covers the script data of typical full blocks.
//   - 4 MiB covers the worst case: total script data in a message is
//     bounded by scriptArenaMaxAlloc, and a single witness item is bounded
//     by maxWitnessItemSize, so the largest class can always satisfy any
//     single allocation.
var scriptChunkClasses = [...]int{
	16 << 10,
	128 << 10,
	1 << 20,
	1 << 22,
}

// scriptChunkPools holds one sync.Pool per chunk size class.  Unlike the
// previous channel-based free list, sync.Pool cooperates with the garbage
// collector, so chunk memory held by the pools is reclaimed when it goes
// unused instead of being retained indefinitely.
var scriptChunkPools = func() [len(scriptChunkClasses)]*sync.Pool {
	var pools [len(scriptChunkClasses)]*sync.Pool
	for i := range scriptChunkClasses {
		size := scriptChunkClasses[i]
		pools[i] = &sync.Pool{
			New: func() interface{} {
				b := make([]byte, size)
				return &b
			},
		}
	}
	return pools
}()

// putScriptChunk returns a chunk to the pool for its size class.  Chunks are
// only ever created by the class pools, so their lengths always match a class
// size exactly.
func putScriptChunk(chunk *[]byte) {
	for i, size := range scriptChunkClasses {
		if len(*chunk) == size {
			scriptChunkPools[i].Put(chunk)
			return
		}
	}
}

// errScriptArenaFull is returned by alloc when satisfying the request would
// push the total bytes allocated since the last rewind beyond
// scriptArenaMaxAlloc.
var errScriptArenaFull = errors.New("script arena capacity exceeded")

// scriptArena is a bump allocator for transient script buffers used during
// transaction decoding.  Allocations are carved out of fixed-size chunks
// drawn from the tiered chunk pools, and the entire arena is freed at once
// via release (or recycled via rewind), so individual allocations carry no
// bookkeeping at all.
//
// The zero value is not usable; arenas must be obtained via
// borrowScriptArena and returned via release.  Arenas are not safe for
// concurrent use, mirroring the decode paths they serve.
type scriptArena struct {
	// chunks holds every chunk currently borrowed from the class pools,
	// in the order they were acquired.  The last entry backs cur.
	chunks []*[]byte

	// cur is the chunk allocations are currently served from and off is
	// the bump offset within it.
	cur []byte
	off int

	// used tracks the total bytes handed out since the last rewind.  It
	// is capped at scriptArenaMaxAlloc to preserve the per-transaction
	// decode limit previously enforced by the fixed slab size.
	used int

	// class is the size class index the next chunk will be drawn from
	// (at minimum) when the current chunk is exhausted.
	class int
}

// scriptArenaPool recycles the arena structs themselves so that decoding a
// message does not allocate any bookkeeping state in the steady state.
var scriptArenaPool = sync.Pool{
	New: func() interface{} {
		return new(scriptArena)
	},
}

// borrowScriptArena returns an arena that will serve its first allocation
// from a chunk of the given starting size class.  Chunks are borrowed
// lazily, so an arena that never allocates never touches the chunk pools.
// The returned arena must be handed back via release.
func borrowScriptArena(startClass int) *scriptArena {
	a := scriptArenaPool.Get().(*scriptArena)
	a.class = startClass
	return a
}

// remaining returns the number of bytes that may still be allocated before
// the arena reaches its total capacity.
func (a *scriptArena) remaining() int {
	return scriptArenaMaxAlloc - a.used
}

// alloc returns a slice of exactly n bytes carved from the arena.  The
// returned slice has its capacity clamped to its length so that appends by
// callers can never bleed into neighboring allocations.  The contents are
// not zeroed; callers are expected to fully overwrite the buffer (the decode
// path fills every allocation with io.ReadFull).
//
// alloc fails with errScriptArenaFull once the total bytes handed out since
// the last rewind would exceed scriptArenaMaxAlloc.
func (a *scriptArena) alloc(n int) ([]byte, error) {
	if n > a.remaining() {
		return nil, errScriptArenaFull
	}
	if n > len(a.cur)-a.off {
		a.grow(n)
	}

	end := a.off + n
	s := a.cur[a.off:end:end]
	a.off = end
	a.used += n

	return s, nil
}

// grow borrows a new chunk large enough to hold n bytes and makes it the
// current chunk.  The new chunk comes from the next size class in the ladder
// so repeated growth converges to the largest class in a constant number of
// steps.  Any request is satisfiable because callers bound n by the arena
// capacity, which equals the largest class size.
func (a *scriptArena) grow(n int) {
	class := a.class
	if class >= len(scriptChunkClasses) {
		class = len(scriptChunkClasses) - 1
	}
	for class < len(scriptChunkClasses)-1 && scriptChunkClasses[class] < n {
		class++
	}

	chunk := scriptChunkPools[class].Get().(*[]byte)
	a.chunks = append(a.chunks, chunk)
	a.cur = *chunk
	a.off = 0
	a.class = class + 1
}

// rewind resets the arena so previously allocated memory is reused from the
// start, invalidating every slice handed out since the last rewind.  It is
// called between transactions while decoding a block: each transaction copies
// its scripts into a single exactly-sized allocation before decoding of the
// next one begins, so the staged data is already dead by the time rewind
// runs.
//
// To keep the reused footprint minimal, only the largest chunk is retained;
// smaller chunks acquired while the arena was warming up are returned to
// their pools.
func (a *scriptArena) rewind() {
	a.used = 0
	a.off = 0

	if len(a.chunks) == 0 {
		return
	}

	// Find the largest chunk, return the rest, and continue serving
	// allocations from it.
	largest := 0
	for i := 1; i < len(a.chunks); i++ {
		if len(*a.chunks[i]) > len(*a.chunks[largest]) {
			largest = i
		}
	}
	keep := a.chunks[largest]
	for i, chunk := range a.chunks {
		if i != largest {
			putScriptChunk(chunk)
		}
		a.chunks[i] = nil
	}

	a.chunks = append(a.chunks[:0], keep)
	a.cur = *keep

	// The next grow should pick up where the retained chunk's class
	// leaves off.
	for i, size := range scriptChunkClasses {
		if size == len(*keep) {
			a.class = i + 1
			break
		}
	}
}

// release returns every chunk to its class pool and recycles the arena
// struct itself.  All slices previously returned by alloc are invalidated.
func (a *scriptArena) release() {
	for i, chunk := range a.chunks {
		putScriptChunk(chunk)
		a.chunks[i] = nil
	}
	a.chunks = a.chunks[:0]
	a.cur = nil
	a.off = 0
	a.used = 0
	a.class = 0

	scriptArenaPool.Put(a)
}
