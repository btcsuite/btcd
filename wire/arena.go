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

	// maxScriptChunkSize is the size of the largest chunk class.  It
	// must be at least scriptArenaMaxAlloc so that any single allocation
	// passing the arena capacity check can be satisfied by one chunk;
	// the compile-time assertion below enforces the coupling.
	maxScriptChunkSize = 1 << 22

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
	maxScriptChunkSize,
}

// Compile-time assertion that the largest chunk class can satisfy any single
// allocation that passes the arena capacity check.  grow relies on this: if
// scriptArenaMaxAlloc were raised past maxScriptChunkSize, allocSlow could
// select a chunk smaller than the requested allocation and panic slicing it.
var _ [maxScriptChunkSize - scriptArenaMaxAlloc]byte

// scriptChunkFixedCaps is the number of chunks per size class that are
// retained in a small bounded free list rather than being subject to GC
// reclamation.  The large classes are the expensive ones to re-create (a
// pool miss zeroes the whole chunk), and only a handful are ever live at
// once since block decode concurrency is bounded, so pinning a few of them
// buys stable block decode performance for at most 16*1 MiB + 8*4 MiB =
// 48 MiB, and only once block traffic has actually warmed them.  The 1 MiB
// class gets the deepest list since every block decode starts there, while
// the 4 MiB class only serves the rare transaction that stages more than
// 1 MiB of script data.  The small classes cost well under a microsecond
// to re-create, so they are left entirely to the GC-cooperating sync.Pool.
var scriptChunkFixedCaps = [len(scriptChunkClasses)]int{0, 0, 16, 8}

// chunkClassPool hands out chunks of a single size class.  It layers a
// small bounded free list (fixed) in front of a sync.Pool: the fixed list
// gives deterministic reuse for the handful of chunks that stay hot, while
// the sync.Pool absorbs bursts beyond it and lets the garbage collector
// reclaim that overflow when it goes idle.  The previous design was a single
// channel free list holding up to 125 four-MiB slabs (~524 MB) that the GC
// could never reclaim.
type chunkClassPool struct {
	fixed chan *[]byte
	pool  sync.Pool
}

// get returns a chunk for this class, preferring the bounded free list.
func (p *chunkClassPool) get() *[]byte {
	select {
	case chunk := <-p.fixed:
		return chunk
	default:
	}
	return p.pool.Get().(*[]byte)
}

// put returns a chunk to this class, refilling the bounded free list first
// and spilling the rest into the sync.Pool.
func (p *chunkClassPool) put(chunk *[]byte) {
	select {
	case p.fixed <- chunk:
	default:
		p.pool.Put(chunk)
	}
}

// scriptChunkPools holds one pool per chunk size class.
var scriptChunkPools = func() [len(scriptChunkClasses)]*chunkClassPool {
	var pools [len(scriptChunkClasses)]*chunkClassPool
	for i := range scriptChunkClasses {
		size := scriptChunkClasses[i]
		pools[i] = &chunkClassPool{
			fixed: make(chan *[]byte, scriptChunkFixedCaps[i]),
			pool: sync.Pool{
				New: func() interface{} {
					b := make([]byte, size)
					return &b
				},
			},
		}
	}
	return pools
}()

// putScriptChunk returns a chunk to the pool for its size class.  Chunks are
// only ever created by the class pools, so their lengths always match a class
// size exactly; anything else indicates internal corruption.
func putScriptChunk(chunk *[]byte) {
	for i, size := range scriptChunkClasses {
		if len(*chunk) == size {
			scriptChunkPools[i].put(chunk)
			return
		}
	}

	panic("putScriptChunk: chunk size matches no class")
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

	// dead marks an arena that has been released.  A dead arena rejects
	// all allocations and ignores rewinds until it is borrowed again, so
	// a stale reference held past release fails loudly with a decode
	// error instead of silently carving memory out of chunks that
	// another decode may now own.
	dead bool
}

// scriptArenaPool recycles the arena structs themselves so that decoding a
// message does not allocate any bookkeeping state in the steady state.
var scriptArenaPool = sync.Pool{
	New: func() interface{} {
		return new(scriptArena)
	},
}

// borrowScriptArena returns an arena that will serve its first allocation
// from a chunk of the given starting size class.  The startClass must be a
// valid index into scriptChunkClasses; all callers pass one of the
// *ScriptChunkClass constants.  Chunks are borrowed lazily, so an arena
// that never allocates never touches the chunk pools.  The returned arena
// must be handed back via release.
//
// Forgetting to call release is safe in the memory sense: the chunks are
// ordinary garbage collected slices, so an abandoned arena is simply
// reclaimed by the GC and only the pooling benefit is lost.
func borrowScriptArena(startClass int) *scriptArena {
	a := scriptArenaPool.Get().(*scriptArena)
	a.class = startClass
	a.used = 0
	a.dead = false
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
		return a.allocSlow(n)
	}

	end := a.off + n
	s := a.cur[a.off:end:end]
	a.off = end
	a.used += n

	return s, nil
}

// allocSlow services an allocation that does not fit in the current chunk
// by growing first.  It is kept separate so the common in-chunk alloc path
// stays small; alloc itself still lands just past the compiler's inlining
// budget due to the call here, which is left as a future optimization.
func (a *scriptArena) allocSlow(n int) ([]byte, error) {
	a.grow(n)

	s := a.cur[:n:n]
	a.off = n
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

	chunk := scriptChunkPools[class].get()
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
	// A released arena stays dead: rewinding must not resurrect its
	// capacity, otherwise a stale reference could allocate out of chunks
	// that have already been handed to another decode.
	if a.dead {
		return
	}

	a.used = 0
	a.off = 0

	// The common case by far is a single chunk (a transaction that fit
	// its starting class), where cur already points at it and resetting
	// the offsets above is all that's needed.
	if len(a.chunks) <= 1 {
		return
	}

	a.rewindSlow()
}

// rewindSlow handles the multi-chunk case of rewind, kept out of line so
// the hot single-chunk rewind path stays small enough to inline.
func (a *scriptArena) rewindSlow() {
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
//
// release is idempotent: a second call on an already released arena is a
// no-op rather than double-inserting the arena into the pool, which would
// hand the same arena to two concurrent decodes.  A released arena also
// rejects any further alloc calls with errScriptArenaFull (used is poisoned
// past the capacity limit) so a stale reference held past release fails
// loudly instead of silently carving memory out of chunks that another
// decode may now own.
func (a *scriptArena) release() {
	if a.dead {
		return
	}

	for i, chunk := range a.chunks {
		putScriptChunk(chunk)
		a.chunks[i] = nil
	}
	a.chunks = a.chunks[:0]
	a.cur = nil
	a.off = 0
	a.class = 0

	// Poison the capacity so any alloc through a stale reference fails
	// with errScriptArenaFull rather than succeeding against recycled
	// memory.  One past the limit makes remaining negative, so even a
	// zero-length alloc is rejected.
	a.used = scriptArenaMaxAlloc + 1
	a.dead = true

	scriptArenaPool.Put(a)
}
