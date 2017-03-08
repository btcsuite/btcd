// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

import (
	"hash"

	"golang.org/x/crypto/ripemd160"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// Calculate the hash of hasher over buf.
func calcHash(buf []byte, hasher hash.Hash) []byte {
	hasher.Write(buf)
	return hasher.Sum(nil)
}

// Hash160 calculates the hash ripemd160(hash256(b)).
func Hash160(buf []byte) []byte {
	return calcHash(chainhash.HashB(buf), ripemd160.New())
}
