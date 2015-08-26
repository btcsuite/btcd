// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Contains useful functions for lottery winner and ticket number determination.

package stake

import (
	"encoding/binary"
	"fmt"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// Hash256PRNG is a determinstic pseudorandom number generator that uses a
// 256-bit secure hashing function to generate random uint32s starting from
// an initial seed.
type Hash256PRNG struct {
	seed      []byte         // The seed used to initialize
	hashIdx   int            // Position in the cached hash
	idx       uint64         // Position in the hash iterator
	seedState chainhash.Hash // Hash iterator root hash
	lastHash  chainhash.Hash // Cached last hash used
}

// NewHash256PRNG creates a pointer to a newly created hash256PRNG.
func NewHash256PRNG(seed []byte) *Hash256PRNG {
	// idx and lastHash are automatically initialized
	// as 0. We initialize the seed by appending a constant
	// to it and hashing to give 32 bytes. This ensures
	// that regardless of the input, the PRNG is always
	// doing a short number of rounds because it only
	// has to hash < 64 byte messages. The constant is
	// derived from the hexadecimal representation of
	// pi.
	cst := []byte{0x24, 0x3F, 0x6A, 0x88,
		0x85, 0xA3, 0x08, 0xD3}
	hp := new(Hash256PRNG)
	hp.seed = chainhash.HashFuncB(append(seed, cst...))
	initLH, err := chainhash.NewHash(hp.seed)
	if err != nil {
		return nil
	}
	hp.seedState = *initLH
	hp.lastHash = *initLH
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

	return chainhash.HashFuncH(finalState)
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
		hp.lastHash = chainhash.HashFuncH(append(hp.seed, idxB...))
		hp.idx++
		hp.hashIdx = 0
	}

	// 'roll over' the PRNG by re-hashing the seed when
	// we overflow idx.
	if hp.idx > 0xFFFFFFFF {
		hp.seedState = chainhash.HashFuncH(hp.seedState[:])
		hp.lastHash = hp.seedState
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

// FindTicketIdxs finds n many unique index numbers for a list length size.
func FindTicketIdxs(size int64, n int, prng *Hash256PRNG) ([]int, error) {
	if size < int64(n) {
		return nil, fmt.Errorf("list size too small")
	}

	if size > 0xFFFFFFFF {
		return nil, fmt.Errorf("list size too big")
	}
	sz := uint32(size)

	var list []int
	listLen := 0
	for listLen < n {
		r := int(prng.uniformRandom(sz))
		if !intInSlice(r, list) {
			list = append(list, r)
			listLen++
		}
	}

	return list, nil
}
