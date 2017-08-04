// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
)

// MainNet ------------------------------------------------------------------------

// genesisCoinbaseTx is the coinbase transaction for the genesis blocks for
// the main network.
var genesisCoinbaseTx = wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			// Fully null.
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
				Tree:  0,
			},
			SignatureScript: []byte{
				0x00, 0x00,
			},
			Sequence:    0xffffffff,
			BlockHeight: wire.NullBlockHeight,
			BlockIndex:  wire.NullBlockIndex,
			ValueIn:     wire.NullValueIn,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Version: 0x0000,
			Value:   0x00000000,
			PkScript: []byte{
				0x80, 0x16, 0x79, 0xe9, 0x85, 0x61, 0xad, 0xa9,
				0x6c, 0xae, 0xc2, 0x94, 0x9a, 0x5d, 0x41, 0xc4,
				0xca, 0xb3, 0x85, 0x1e, 0xb7, 0x40, 0xd9, 0x51,
				0xc1, 0x0e, 0xcb, 0xcf, 0x26, 0x5c, 0x1f, 0xd9,
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// genesisMerkleRoot is the hash of the first transaction in the genesis block
// for the main network.
var genesisMerkleRoot = genesisCoinbaseTx.TxHashFull()

// genesisBlock defines the genesis block of the block chain which serves as the
// public transaction ledger for the main network.
//
// The genesis block for Decred mainnet, testnet, and simnet are not evaluated
// for proof of work. The only values that are ever used elsewhere in the
// blockchain from it are:
// (1) The genesis block hash is used as the PrevBlock in params.go.
// (2) The difficulty starts off at the value given by bits.
// (3) The stake difficulty starts off at the value given by SBits.
// (4) The timestamp, which guides when blocks can be built on top of it
//      and what the initial difficulty calculations come out to be.
//
// The genesis block is valid by definition and none of the fields within
// it are validated for correctness.
var genesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:      1,
		PrevBlock:    chainhash.Hash{},
		MerkleRoot:   genesisMerkleRoot,
		StakeRoot:    chainhash.Hash{},
		Timestamp:    time.Unix(1454954400, 0), // Mon, 08 Feb 2016 18:00:00 GMT
		Bits:         0x1b01ffff,               // Difficulty 32767
		SBits:        2 * 1e8,                  // 2 Coin
		Nonce:        0x00000000,
		StakeVersion: 0,
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

// genesisHash is the hash of the first block in the block chain for the main
// network (genesis block).
var genesisHash = genesisBlock.BlockHash()

// genesisCoinbaseTxLegacy is the coinbase transaction for the genesis block for
// the test network.
var genesisCoinbaseTxLegacy = wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			SignatureScript: []byte{
				0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, 0x45, /* |.......E| */
				0x54, 0x68, 0x65, 0x20, 0x54, 0x69, 0x6d, 0x65, /* |The Time| */
				0x73, 0x20, 0x30, 0x33, 0x2f, 0x4a, 0x61, 0x6e, /* |s 03/Jan| */
				0x2f, 0x32, 0x30, 0x30, 0x39, 0x20, 0x43, 0x68, /* |/2009 Ch| */
				0x61, 0x6e, 0x63, 0x65, 0x6c, 0x6c, 0x6f, 0x72, /* |ancellor| */
				0x20, 0x6f, 0x6e, 0x20, 0x62, 0x72, 0x69, 0x6e, /* | on brin| */
				0x6b, 0x20, 0x6f, 0x66, 0x20, 0x73, 0x65, 0x63, /* |k of sec|*/
				0x6f, 0x6e, 0x64, 0x20, 0x62, 0x61, 0x69, 0x6c, /* |ond bail| */
				0x6f, 0x75, 0x74, 0x20, 0x66, 0x6f, 0x72, 0x20, /* |out for |*/
				0x62, 0x61, 0x6e, 0x6b, 0x73, /* |banks| */
			},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value: 0x00000000,
			PkScript: []byte{
				0x41, 0x04, 0x67, 0x8a, 0xfd, 0xb0, 0xfe, 0x55, /* |A.g....U| */
				0x48, 0x27, 0x19, 0x67, 0xf1, 0xa6, 0x71, 0x30, /* |H'.g..q0| */
				0xb7, 0x10, 0x5c, 0xd6, 0xa8, 0x28, 0xe0, 0x39, /* |..\..(.9| */
				0x09, 0xa6, 0x79, 0x62, 0xe0, 0xea, 0x1f, 0x61, /* |..yb...a| */
				0xde, 0xb6, 0x49, 0xf6, 0xbc, 0x3f, 0x4c, 0xef, /* |..I..?L.| */
				0x38, 0xc4, 0xf3, 0x55, 0x04, 0xe5, 0x1e, 0xc1, /* |8..U....| */
				0x12, 0xde, 0x5c, 0x38, 0x4d, 0xf7, 0xba, 0x0b, /* |..\8M...| */
				0x8d, 0x57, 0x8a, 0x4c, 0x70, 0x2b, 0x6b, 0xf1, /* |.W.Lp+k.| */
				0x1d, 0x5f, 0xac, /* |._.| */
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// TestNet2 ------------------------------------------------------------------------

// testNetGenesisMerkleRoot is the hash of the first transaction in the genesis block
// for the test network.
var testNet2GenesisMerkleRoot = genesisCoinbaseTxLegacy.TxHash()

// testNetGenesisBlock defines the genesis block of the block chain which
// serves as the public transaction ledger for the test network (version 3).
var testNet2GenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:      4,
		PrevBlock:    chainhash.Hash{},
		MerkleRoot:   testNet2GenesisMerkleRoot,
		Timestamp:    time.Unix(1489550400, 0), // 2017-03-15 TestNet10
		Bits:         0x1e00ffff,
		SBits:        20000000,
		Nonce:        0x18aea41a,
		StakeVersion: 0,
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTxLegacy},
}

// testNetGenesisHash is the hash of the first block in the block chain for the
// test network.
var testNet2GenesisHash = testNet2GenesisBlock.BlockHash()

// SimNet -------------------------------------------------------------------------

var regTestGenesisCoinbaseTx = wire.MsgTx{
	SerType: wire.TxSerializeFull,
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			SignatureScript: []byte{
				0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, 0x45, /* |.......E| */
				0x54, 0x68, 0x65, 0x20, 0x54, 0x69, 0x6d, 0x65, /* |The Time| */
				0x73, 0x20, 0x30, 0x33, 0x2f, 0x4a, 0x61, 0x6e, /* |s 03/Jan| */
				0x2f, 0x32, 0x30, 0x30, 0x39, 0x20, 0x43, 0x68, /* |/2009 Ch| */
				0x61, 0x6e, 0x63, 0x65, 0x6c, 0x6c, 0x6f, 0x72, /* |ancellor| */
				0x20, 0x6f, 0x6e, 0x20, 0x62, 0x72, 0x69, 0x6e, /* | on brin| */
				0x6b, 0x20, 0x6f, 0x66, 0x20, 0x73, 0x65, 0x63, /* |k of sec|*/
				0x6f, 0x6e, 0x64, 0x20, 0x62, 0x61, 0x69, 0x6c, /* |ond bail| */
				0x6f, 0x75, 0x74, 0x20, 0x66, 0x6f, 0x72, 0x20, /* |out for |*/
				0x62, 0x61, 0x6e, 0x6b, 0x73, /* |banks| */
			},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value: 0x00000000,
			PkScript: []byte{
				0x41, 0x04, 0x67, 0x8a, 0xfd, 0xb0, 0xfe, 0x55, /* |A.g....U| */
				0x48, 0x27, 0x19, 0x67, 0xf1, 0xa6, 0x71, 0x30, /* |H'.g..q0| */
				0xb7, 0x10, 0x5c, 0xd6, 0xa8, 0x28, 0xe0, 0x39, /* |..\..(.9| */
				0x09, 0xa6, 0x79, 0x62, 0xe0, 0xea, 0x1f, 0x61, /* |..yb...a| */
				0xde, 0xb6, 0x49, 0xf6, 0xbc, 0x3f, 0x4c, 0xef, /* |..I..?L.| */
				0x38, 0xc4, 0xf3, 0x55, 0x04, 0xe5, 0x1e, 0xc1, /* |8..U....| */
				0x12, 0xde, 0x5c, 0x38, 0x4d, 0xf7, 0xba, 0x0b, /* |..\8M...| */
				0x8d, 0x57, 0x8a, 0x4c, 0x70, 0x2b, 0x6b, 0xf1, /* |.W.Lp+k.| */
				0x1d, 0x5f, 0xac, /* |._.| */
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// simNetGenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the simulation test network.  It is the same as the merkle root for
// the main network.
var simNetGenesisMerkleRoot = genesisMerkleRoot

// simNetGenesisBlock defines the genesis block of the block chain which serves
// as the public transaction ledger for the simulation test network.
var simNetGenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version: 1,
		PrevBlock: chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}),
		MerkleRoot: simNetGenesisMerkleRoot,
		StakeRoot: chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}),
		VoteBits:     0,
		FinalState:   [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Voters:       0,
		FreshStake:   0,
		Revocations:  0,
		Timestamp:    time.Unix(1401292357, 0), // 2009-01-08 20:54:25 -0600 CST
		PoolSize:     0,
		Bits:         0x207fffff, // 545259519
		SBits:        0,
		Nonce:        0,
		StakeVersion: 0,
		Height:       0,
	},
	Transactions:  []*wire.MsgTx{&regTestGenesisCoinbaseTx},
	STransactions: []*wire.MsgTx{},
}

// simNetGenesisHash is the hash of the first block in the block chain for the
// simulation test network.
var simNetGenesisHash = simNetGenesisBlock.BlockHash()
