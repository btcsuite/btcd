// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Contains useful functions for lottery winner and ticket number determination.

package stake

import (
	"encoding/binary"
	"fmt"

	"github.com/decred/dcrd/blockchain/stake/internal/tickettreap"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

var (
	// seedConst is a constant derived from the hex representation of pi. It
	// is used alonwith a caller-provided seed when initializing
	// the deterministic lottery prng.
	seedConst = [8]byte{0x24, 0x3F, 0x6A, 0x88, 0x85, 0xA3, 0x08, 0xD3}
)

// Hash256PRNG is a determinstic pseudorandom number generator that uses a
// 256-bit secure hashing function to generate random uint32s starting from
// an initial seed.
type Hash256PRNG struct {
	seed     chainhash.Hash // The seed used to initialize
	hashIdx  int            // Position in the cached hash
	idx      uint64         // Position in the hash iterator
	lastHash chainhash.Hash // Cached last hash used
}

// NewHash256PRNG creates a pointer to a newly created hash256PRNG.
func NewHash256PRNG(seed []byte) *Hash256PRNG {
	// idx and lastHash are automatically initialized
	// as 0.  We initialize the seed by appending a constant
	// to it and hashing to give 32 bytes. This ensures
	// that regardless of the input, the PRNG is always
	// doing a short number of rounds because it only
	// has to hash < 64 byte messages.  The constant is
	// derived from the hexadecimal representation of
	// pi.
	hp := new(Hash256PRNG)
	hp.seed = chainhash.HashH(append(seed, seedConst[:]...))
	hp.lastHash = hp.seed
	hp.idx = 0
	return hp
}

// StateHash returns a hash referencing the current state the deterministic PRNG.
func (hp *Hash256PRNG) StateHash() chainhash.Hash {
	fHash := hp.lastHash
	fIdx := hp.idx
	fHashIdx := hp.hashIdx

	finalState := make([]byte, len(fHash)+4+1)
	copy(finalState, fHash[:])
	binary.BigEndian.PutUint32(finalState[len(fHash):], uint32(fIdx))
	finalState[len(fHash)+4] = byte(fHashIdx)

	return chainhash.HashH(finalState)
}

// Hash256Rand returns a uint32 random number using the pseudorandom number
// generator and updates the state.
func (hp *Hash256PRNG) Hash256Rand() uint32 {
	r := binary.BigEndian.Uint32(hp.lastHash[hp.hashIdx*4 : hp.hashIdx*4+4])
	hp.hashIdx++

	// 'roll over' the hash index to use and store it.
	if hp.hashIdx > 7 {
		idxB := make([]byte, 4, 4)
		binary.BigEndian.PutUint32(idxB, uint32(hp.idx))
		hp.lastHash = chainhash.HashH(append(hp.seed[:], idxB...))
		hp.idx++
		hp.hashIdx = 0
	}

	// 'roll over' the PRNG by re-hashing the seed when
	// we overflow idx.
	if hp.idx > 0xFFFFFFFF {
		hp.seed = chainhash.HashH(hp.seed[:])
		hp.lastHash = hp.seed
		hp.idx = 0
	}

	return r
}

// uniformRandom returns a random in the range [0 ... upperBound) while avoiding
// modulo bias, thus giving a normal distribution within the specified range.
//
// Ported from
// https://github.com/conformal/clens/blob/master/src/arc4random_uniform.c
func (hp *Hash256PRNG) uniformRandom(upperBound uint32) uint32 {
	var r, min uint32
	if upperBound < 2 {
		return 0
	}

	if upperBound > 0x80000000 {
		min = 1 + ^upperBound
	} else {
		// (2**32 - (x * 2)) % x == 2**32 % x when x <= 2**31
		min = ((0xFFFFFFFF - (upperBound * 2)) + 1) % upperBound
	}

	for {
		r = hp.Hash256Rand()
		if r >= min {
			break
		}
	}

	return r % upperBound
}

// intInSlice returns true if an integer is in the passed slice, false otherwise.
func intInSlice(i int, sl []int) bool {
	for _, e := range sl {
		if i == e {
			return true
		}
	}

	return false
}

// findTicketIdxs finds n many unique index numbers for a list length size.
func findTicketIdxs(size int, n uint16, prng *Hash256PRNG) ([]int, error) {
	if size < int(n) {
		return nil, fmt.Errorf("list size too small: %v < %v",
			size, n)
	}

	max := int64(0xFFFFFFFF)
	if int64(size) > max {
		return nil, fmt.Errorf("list size too big: %v > %v",
			size, max)
	}
	sz := uint32(size)

	var list []int
	var listLen uint16
	for listLen < n {
		r := int(prng.uniformRandom(sz))
		if !intInSlice(r, list) {
			list = append(list, r)
			listLen++
		}
	}

	return list, nil
}

// FindTicketIdxs is the exported version of findTicketIdxs used for testing.
func FindTicketIdxs(size int, n uint16, prng *Hash256PRNG) ([]int, error) {
	return findTicketIdxs(size, n, prng)
}

// fetchWinners is a ticket database specific function which iterates over the
// entire treap and finds winners at selected indexes.  These are returned
// as a slice of pointers to keys, which can be recast as []*chainhash.Hash.
// Importantly, it maintains the list of winners in the same order as specified
// in the original idxs passed to the function.
func fetchWinners(idxs []int, t *tickettreap.Immutable) ([]*tickettreap.Key, error) {
	if idxs == nil {
		return nil, fmt.Errorf("empty idxs list")
	}
	if t == nil || t.Len() == 0 {
		return nil, fmt.Errorf("missing or empty treap")
	}

	winners := make([]*tickettreap.Key, len(idxs))
	for i, idx := range idxs {
		if idx < 0 || idx >= t.Len() {
			return nil, fmt.Errorf("idx %v out of bounds", idx)
		}

		if idx < t.Len() {
			k, _ := t.GetByIndex(idx)
			winners[i] = &k
		}
	}

	return winners, nil
}

// fetchExpired is a ticket database specific function which iterates over the
// entire treap and finds tickets that are equal or less than the given height.
// These are returned as a slice of pointers to keys, which can be recast as
// []*chainhash.Hash.
func fetchExpired(height uint32, t *tickettreap.Immutable) []*tickettreap.Key {
	var expired []*tickettreap.Key
	t.ForEach(func(k tickettreap.Key, v *tickettreap.Value) bool {
		if v.Height <= height {
			expired = append(expired, &k)
		}

		return true
	})

	return expired
}
