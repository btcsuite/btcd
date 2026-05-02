// Copyright (c) 2024 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	_ "embed"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// SwiftSyncData contains the precomputed bitmap for fast UTXO bootstrapping.
// The bitmap indicates which transaction outputs have been spent up to a
// certain block height. During initial sync, instead of replaying all
// transactions and tracking spends, btcd can use this bitmap to directly
// construct the UTXO set by only adding outputs where the corresponding bit
// is 0 (unspent).
type SwiftSyncData struct {
	// Height is the block height up to which the bitmap covers.
	// Blocks 1 through Height (inclusive) can be processed using swift sync.
	Height int32

	// Hash is the block hash at Height, used for verification.
	Hash *chainhash.Hash

	// Bitmap contains packed bits where bit index i corresponds to the i-th
	// transaction output (starting from block 1, in block order, transaction
	// order, then output order). A bit value of 0 means the output is unspent,
	// and 1 means spent.
	Bitmap []byte
}

// signetSwiftSyncBitmapFile contains the raw bitmap file for signet.
// The file format is:
//   - 8 bytes: total allocated bits (little endian uint64) - this is the header
//   - remaining bytes: the bitmap data
//
//go:embed data/signet_swiftsync.bin
var signetSwiftSyncBitmapFile []byte

// signetSwiftSyncHash is the block hash at the swift sync height for signet.
// Height: 285205
// Hash: 00000004b1cc694c48295fc56c2d78b88abdd648ee60a21548feba259e6dedf1
var signetSwiftSyncHash = chainhash.Hash{
	0xf1, 0xed, 0x6d, 0x9e, 0x25, 0xba, 0xfe, 0x48,
	0x15, 0xa2, 0x60, 0xee, 0x48, 0xd6, 0xbd, 0x8a,
	0xb8, 0x78, 0x2d, 0x6c, 0xc5, 0x5f, 0x29, 0x48,
	0x4c, 0x69, 0xcc, 0xb1, 0x04, 0x00, 0x00, 0x00,
}

const (
	// swiftSyncBitmapHeaderSize is the size of the header in the bitmap file.
	// The header contains the total number of allocated bits as a uint64.
	swiftSyncBitmapHeaderSize = 8

	// signetSwiftSyncHeight is the block height covered by the signet bitmap.
	signetSwiftSyncHeight = 285205
)

// sigNetSwiftSync contains the swift sync data for signet.
// It is initialized from the embedded bitmap file.
var sigNetSwiftSync *SwiftSyncData

func init() {
	// Skip the 8-byte header to get the actual bitmap data.
	if len(signetSwiftSyncBitmapFile) > swiftSyncBitmapHeaderSize {
		sigNetSwiftSync = &SwiftSyncData{
			Height: signetSwiftSyncHeight,
			Hash:   &signetSwiftSyncHash,
			Bitmap: signetSwiftSyncBitmapFile[swiftSyncBitmapHeaderSize:],
		}
		// Update SigNetParams to point to the initialized data.
		// This is needed because SigNetParams is initialized before
		// this init() runs.
		SigNetParams.SwiftSync = sigNetSwiftSync
	}
}
