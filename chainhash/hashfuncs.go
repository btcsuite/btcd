// Copyright (c) 2015 The Decred developers
// Copyright (c) 2016-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chainhash

import (
	"crypto/sha256"
	"io"
)

// HashB calculates hash(b) and returns the resulting bytes.
func HashB(b []byte) []byte {
	hash := sha256.Sum256(b)
	return hash[:]
}

// HashH calculates hash(b) and returns the resulting bytes as a Hash.
func HashH(b []byte) Hash {
	return Hash(sha256.Sum256(b))
}

// DoubleHashB calculates hash(hash(b)) and returns the resulting bytes.
func DoubleHashB(b []byte) []byte {
	first := sha256.Sum256(b)
	second := sha256.Sum256(first[:])
	return second[:]
}

// DoubleHashH calculates hash(hash(b)) and returns the resulting bytes as a
// Hash.
func DoubleHashH(b []byte) Hash {
	first := sha256.Sum256(b)
	return Hash(sha256.Sum256(first[:]))
}

// DoubleHashRaw calculates hash(hash(w)) where w is the resulting bytes from
// the given serialize function and returns the resulting bytes as a Hash.
func DoubleHashRaw(serialize func(w io.Writer) error) Hash {
	// Encode the transaction into the hash.  Ignore the error returns
	// since the only way the encode could fail is being out of memory
	// or due to nil pointers, both of which would cause a run-time panic.
	h := sha256.New()
	_ = serialize(h)

	// This buf is here because Sum() will append the result to the passed
	// in byte slice.  Pre-allocating here saves an allocation on the second
	// hash as we can reuse it.  This allocation also does not escape to the
	// heap, saving an allocation.
	buf := make([]byte, 0, HashSize)
	first := h.Sum(buf)
	h.Reset()
	h.Write(first)
	res := h.Sum(buf)
	return *(*Hash)(res)
}
