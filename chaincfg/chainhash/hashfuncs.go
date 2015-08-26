// Copyright (c) 2015-2016 The Decred Developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chainhash

import (
	"github.com/decred/blake256"
)

// HashFunc calculates the hash of the supplied bytes.
// TODO(jcv) Should modify blake256 so it has the same interface as blake2
// and fastsha256 so these function can look more like btcsuite.  Then should
// try to get it to the upstream blake256 repo
func HashFunc(data []byte) [blake256.Size]byte {
	var outB [blake256.Size]byte
	a := blake256.New()
	a.Write(data)
	out := a.Sum(nil)
	for i, el := range out {
		outB[i] = el
	}

	return outB
}

// HashFuncB calculates hash(b) and returns the resulting bytes.
func HashFuncB(b []byte) []byte {
	a := blake256.New()
	a.Write(b)
	out := a.Sum(nil)
	return out
}

// HashFuncH calculates hash(b) and returns the resulting bytes as a Hash.
func HashFuncH(b []byte) Hash {
	var outB [blake256.Size]byte
	a := blake256.New()
	a.Write(b)
	out := a.Sum(nil)
	for i, el := range out {
		outB[i] = el
	}

	return Hash(outB)
}

// HashBlockSize is the block size of the hash algorithm in bytes.
const HashBlockSize = blake256.BlockSize
