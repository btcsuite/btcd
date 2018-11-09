// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package harness

import (
	"encoding/binary"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// hdSeed is the BIP 32 seed used by the InMemoryWallet to initialize it's
// HD root key. This value is hard coded in order to ensure
// deterministic behavior across test runs.
var hdSeed = [chainhash.HashSize]byte{
	0x79, 0xa6, 0x1a, 0xdb, 0xc6, 0xe5, 0xa2, 0xe1,
	0x39, 0xd2, 0x71, 0x3a, 0x54, 0x6e, 0xc7, 0xc8,
	0x75, 0x63, 0x2e, 0x75, 0xf1, 0xdf, 0x9c, 0x3f,
	0xa6, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// NewTestSeed generates new test wallet seed using predefined array hdSeed
// and a custom salt number
// The wallet's final HD seed is: [hdSeed || salt]. This method
// ensures that each harness instance uses a deterministic root seed
// based on its salt.
func NewTestSeed(salt uint32) [chainhash.HashSize + 4]byte {
	seed := [chainhash.HashSize + 4]byte{}
	copy(seed[:], hdSeed[:])
	binary.BigEndian.PutUint32(seed[:chainhash.HashSize], salt)
	return seed
}
