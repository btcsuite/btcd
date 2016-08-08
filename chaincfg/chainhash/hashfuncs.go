// Copyright (c) 2015 The Decred developers
// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chainhash

import "github.com/btcsuite/fastsha256"

// HashB calculates hash(b) and returns the resulting bytes.
func HashB(b []byte) []byte {
	hash := fastsha256.Sum256(b)
	return hash[:]
}

// HashH calculates hash(b) and returns the resulting bytes as a Hash.
func HashH(b []byte) Hash {
	return Hash(fastsha256.Sum256(b))
}

// DoubleHashB calculates hash(hash(b)) and returns the resulting bytes.
func DoubleHashB(b []byte) []byte {
	first := fastsha256.Sum256(b)
	second := fastsha256.Sum256(first[:])
	return second[:]
}

// DoubleHashH calculates hash(hash(b)) and returns the resulting bytes as a
// Hash.
func DoubleHashH(b []byte) Hash {
	first := fastsha256.Sum256(b)
	return Hash(fastsha256.Sum256(first[:]))
}
