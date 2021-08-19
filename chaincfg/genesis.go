// Copyright (c) 2014-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"time"

	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/wire"
)

// genesisCoinbaseTx is the coinbase transaction for the genesis blocks for
// the main network, regression test network, and test network (version 3).
var genesisCoinbaseTx = wire.MsgTx{
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			SignatureScript: []byte{
				0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, 0x17,
				0x69, 0x6e, 0x73, 0x65, 0x72, 0x74, 0x20, 0x74,
				0x69, 0x6d, 0x65, 0x73, 0x74, 0x61, 0x6d, 0x70,
				0x20, 0x73, 0x74, 0x72, 0x69, 0x6e, 0x67,
			},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value: 0x8e1bc9bf040000, // 400000000 * COIN
			PkScript: []byte{
				0x76, 0xa9, 0x14, 0x34, 0x59, 0x91, 0xdb, 0xf5,
				0x7b, 0xfb, 0x01, 0x4b, 0x87, 0x00, 0x6a, 0xcd,
				0xfa, 0xfb, 0xfc, 0x5f, 0xe8, 0x29, 0x2f, 0x88,
				0xac,
			},
		},
	},
	LockTime: 0,
}

// genesisHash is the hash of the first block in the block chain for the main
// network (genesis block).
var genesisHash = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0x63, 0xf4, 0x34, 0x6a, 0x4d, 0xb3, 0x4f, 0xdf,
	0xce, 0x29, 0xa7, 0x0f, 0x5e, 0x8d, 0x11, 0xf0,
	0x65, 0xf6, 0xb9, 0x16, 0x02, 0xb7, 0x03, 0x6c,
	0x7f, 0x22, 0xf3, 0xa0, 0x3b, 0x28, 0x89, 0x9c,
})

// genesisMerkleRoot is the hash of the first transaction in the genesis block
// for the main network.
var genesisMerkleRoot = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0xcc, 0x59, 0xe5, 0x9f, 0xf9, 0x7a, 0xc0, 0x92,
	0xb5, 0x5e, 0x42, 0x3a, 0xa5, 0x49, 0x51, 0x51,
	0xed, 0x6f, 0xb8, 0x05, 0x70, 0xa5, 0xbb, 0x78,
	0xcd, 0x5b, 0xd1, 0xc3, 0x82, 0x1c, 0x21, 0xb8,
})

// genesisClaimTrie is the hash of the first transaction in the genesis block
// for the main network.
var genesisClaimTrie = chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
})

// genesisBlock defines the genesis block of the block chain which serves as the
// public transaction ledger for the main network.
var genesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},         // 0000000000000000000000000000000000000000000000000000000000000000
		MerkleRoot: genesisMerkleRoot,        // b8211c82c3d15bcd78bba57005b86fed515149a53a425eb592c07af99fe559cc
		ClaimTrie:  genesisClaimTrie,         // 0000000000000000000000000000000000000000000000000000000000000001
		Timestamp:  time.Unix(1446058291, 0), // 28 Oct 2015 18:51:31 +0000 UTC
		Bits:       0x1f00ffff,               // 486604799 [00000000ffff0000000000000000000000000000000000000000000000000000]
		Nonce:      0x00000507,               // 1287
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

// regTestGenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the regression test network.  It is the same as the merkle root for
// the main network.
var regTestGenesisMerkleRoot = genesisMerkleRoot

// regTestGenesisBlock defines the genesis block of the block chain which serves
// as the public transaction ledger for the regression test network.
var regTestGenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},         // 0000000000000000000000000000000000000000000000000000000000000000
		MerkleRoot: regTestGenesisMerkleRoot, // b8211c82c3d15bcd78bba57005b86fed515149a53a425eb592c07af99fe559cc
		ClaimTrie:  genesisClaimTrie,         // 0000000000000000000000000000000000000000000000000000000000000001
		Timestamp:  time.Unix(1446058291, 0), // 28 Oct 2015 18:51:31 +0000 UTC
		Bits:       0x207fffff,               // 545259519 [7fffff0000000000000000000000000000000000000000000000000000000000]
		Nonce:      1,
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

// regTestGenesisHash is the hash of the first block in the block chain for the
// regression test network (genesis block).
var regTestGenesisHash = regTestGenesisBlock.BlockHash()

// testNet3GenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the test network (version 3).  It is the same as the merkle root
// for the main network.
var testNet3GenesisMerkleRoot = genesisMerkleRoot

// testNet3GenesisBlock defines the genesis block of the block chain which
// serves as the public transaction ledger for the test network (version 3).
var testNet3GenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},          // 0000000000000000000000000000000000000000000000000000000000000000
		MerkleRoot: testNet3GenesisMerkleRoot, // b8211c82c3d15bcd78bba57005b86fed515149a53a425eb592c07af99fe559cc
		ClaimTrie:  genesisClaimTrie,          // 0000000000000000000000000000000000000000000000000000000000000001
		Timestamp:  time.Unix(1446058291, 0),  // 28 Oct 2015 18:51:31 +0000 UTC
		Bits:       0x1f00ffff,                // 486604799 [00000000ffff0000000000000000000000000000000000000000000000000000]
		Nonce:      0x00000507,                // 1287
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

// testNet3GenesisHash is the hash of the first block in the block chain for the
// test network (version 3).
var testNet3GenesisHash = testNet3GenesisBlock.BlockHash()

// simNetGenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the simulation test network.  It is the same as the merkle root for
// the main network.
var simNetGenesisMerkleRoot = genesisMerkleRoot

// simNetGenesisBlock defines the genesis block of the block chain which serves
// as the public transaction ledger for the simulation test network.
var simNetGenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},         // 0000000000000000000000000000000000000000000000000000000000000000
		MerkleRoot: simNetGenesisMerkleRoot,  // 4a5e1e4baab89f3a32518a88c31bc87f618f76673e2cc77ab2127b7afdeda33b
		Timestamp:  time.Unix(1401292357, 0), // 2014-05-28 15:52:37 +0000 UTC
		Bits:       0x207fffff,               // 545259519 [7fffff0000000000000000000000000000000000000000000000000000000000]
		Nonce:      2,
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

// simNetGenesisHash is the hash of the first block in the block chain for the
// simulation test network.
var simNetGenesisHash = simNetGenesisBlock.BlockHash()

// sigNetGenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the signet test network. It is the same as the merkle root for
// the main network.
var sigNetGenesisMerkleRoot = genesisMerkleRoot

// sigNetGenesisBlock defines the genesis block of the block chain which serves
// as the public transaction ledger for the signet test network.
var sigNetGenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:    1,
		PrevBlock:  chainhash.Hash{},         // 0000000000000000000000000000000000000000000000000000000000000000
		MerkleRoot: sigNetGenesisMerkleRoot,  // 4a5e1e4baab89f3a32518a88c31bc87f618f76673e2cc77ab2127b7afdeda33b
		Timestamp:  time.Unix(1598918400, 0), // 2020-09-01 00:00:00 +0000 UTC
		Bits:       0x1e0377ae,               // 503543726 [00000377ae000000000000000000000000000000000000000000000000000000]
		Nonce:      52613770,
	},
	Transactions: []*wire.MsgTx{&genesisCoinbaseTx},
}

// sigNetGenesisHash is the hash of the first block in the block chain for the
// signet test network.
var sigNetGenesisHash = sigNetGenesisBlock.BlockHash()
