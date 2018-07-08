// Copyright (c) 2015-2016 The Decred developers
// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chainhash

import (
	"github.com/dchest/blake256"
)

// HashFunc calculates the hash of the supplied bytes.
// TODO(jcv) Should modify blake256 so it has the same interface as blake2
// and sha256 so these function can look more like btcsuite.  Then should
// try to get it to the upstream blake256 repo
func HashFunc(b []byte) [blake256.Size]byte {
	var outB [blake256.Size]byte
	copy(outB[:], HashB(b))

	return outB
}

// HashB calculates hash(b) and returns the resulting bytes.
func HashB(b []byte) []byte {
	a := blake256.New()
	a.Write(b)
	out := a.Sum(nil)
	return out
}

// HashH calculates hash(b) and returns the resulting bytes as a Hash.
func HashH(b []byte) Hash {
	return Hash(HashFunc(b))
}

// HashBlockSize is the block size of the hash algorithm in bytes.
const HashBlockSize = blake256.BlockSize
