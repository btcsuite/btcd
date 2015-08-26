// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake_test

import (
	"bytes"
	"compress/bzip2"
	"encoding/gob"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

// cloneTicketDB makes a deep copy of a ticket DB by
// serializing it to a gob and then deserializing it
// into an empty container.
func cloneTicketDB(tmdb *stake.TicketDB) (stake.TicketMaps, error) {
	mapsPointer := tmdb.DumpMapsPointer()
	mapsBytes, err := mapsPointer.GobEncode()
	if err != nil {
		return stake.TicketMaps{},
			fmt.Errorf("clone db error: could not serialize ticketMaps")
	}

	var mapsCopy stake.TicketMaps
	if err := mapsCopy.GobDecode(mapsBytes); err != nil {
		return stake.TicketMaps{},
			fmt.Errorf("clone db error: could not deserialize " +
				"ticketMaps")
	}

	return mapsCopy, nil
}

// hashInSlice returns whether a hash exists in a slice or not.
func hashInSlice(h *chainhash.Hash, list []*chainhash.Hash) bool {
	for _, hash := range list {
		if h.IsEqual(hash) {
			return true
		}
	}

	return false
}

func TestTicketDB(t *testing.T) {
	// Declare some useful variables
	testBCHeight := int64(168)

	// Set up a blockchain
	chain, teardownFunc, err := chainSetup("ticketdbunittests",
		simNetParams)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	filename := filepath.Join("..", "/../blockchain/testdata", "blocks0to168.bz2")
	fi, err := os.Open(filename)
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder := gob.NewDecoder(bcBuf)
	testBlockchain := make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&testBlockchain); err != nil {
		t.Errorf("error decoding test blockchain")
	}

	timeSource := blockchain.NewMedianTime()
	var CopyOfMapsAtBlock50, CopyOfMapsAtBlock168 stake.TicketMaps
	var ticketsToSpendIn167 []chainhash.Hash
	var sortedTickets167 []*stake.TicketData

	for i := int64(0); i <= testBCHeight; i++ {
		if i == 0 {
			continue
		}
		block, err := dcrutil.NewBlockFromBytes(testBlockchain[i])
		if err != nil {
			t.Fatalf("block deserialization error on block %v", i)
		}
		block.SetHeight(i)
		_, _, err = chain.ProcessBlock(block, timeSource, blockchain.BFNone)
		if err != nil {
			t.Fatalf("failed to process block %v: %v", i, err)
		}

		if i == 50 {
			// Create snapshot of tmdb at block 50
			CopyOfMapsAtBlock50, err = cloneTicketDB(chain.TMDB())
			if err != nil {
				t.Errorf("db cloning at block 50 failure! %v", err)
			}
		}

		// Test to make sure that ticket selection is working correctly.
		if i == 167 {
			// Sort the entire list of tickets lexicographically by sorting
			// each bucket and then appending it to the list. Then store it
			// to use in the next block.
			totalTickets := 0
			sortedSlice := make([]*stake.TicketData, 0)
			for i := 0; i < stake.BucketsSize; i++ {
				tix, err := chain.TMDB().DumpLiveTickets(uint8(i))
				if err != nil {
					t.Errorf("error dumping live tickets")
				}
				mapLen := len(tix)
				totalTickets += mapLen
				tempTdSlice := stake.NewTicketDataSlice(mapLen)
				itr := 0 // Iterator
				for _, td := range tix {
					tempTdSlice[itr] = td
					itr++
				}
				sort.Sort(tempTdSlice)
				sortedSlice = append(sortedSlice, tempTdSlice...)
			}
			sortedTickets167 = sortedSlice
		}

		if i == 168 {
			parentBlock, err := dcrutil.NewBlockFromBytes(testBlockchain[i-1])
			if err != nil {
				t.Errorf("block deserialization error on block %v", i-1)
			}
			pbhB, err := parentBlock.MsgBlock().Header.Bytes()
			if err != nil {
				t.Errorf("block header serialization error")
			}
			prng := stake.NewHash256PRNG(pbhB)
			ts, err := stake.FindTicketIdxs(int64(len(sortedTickets167)),
				int(simNetParams.TicketsPerBlock), prng)
			if err != nil {
				t.Errorf("failure on FindTicketIdxs")
			}
			for _, idx := range ts {
				ticketsToSpendIn167 =
					append(ticketsToSpendIn167, sortedTickets167[idx].SStxHash)
			}

			// Make sure that the tickets that were supposed to be spent or
			// missed were.
			spentTix, err := chain.TMDB().DumpSpentTickets(i)
			if err != nil {
				t.Errorf("DumpSpentTickets failure")
			}
			for _, h := range ticketsToSpendIn167 {
				if _, ok := spentTix[h]; !ok {
					t.Errorf("missing ticket %v that should have been missed "+
						"or spent in block %v", h, i)
				}
			}

			// Create snapshot of tmdb at block 168
			CopyOfMapsAtBlock168, err = cloneTicketDB(chain.TMDB())
			if err != nil {
				t.Errorf("db cloning at block 168 failure! %v", err)
			}
		}
	}

	// Remove five blocks from HEAD~1
	_, _, _, err = chain.TMDB().RemoveBlockToHeight(50)
	if err != nil {
		t.Errorf("error: %v", err)
	}

	// Test if the roll back was symmetric to the earlier snapshot
	if !reflect.DeepEqual(chain.TMDB().DumpMapsPointer(), CopyOfMapsAtBlock50) {
		t.Errorf("The td did not restore to a previous block height correctly!")
	}

	// Test rescanning a ticket db
	err = chain.TMDB().RescanTicketDB()
	if err != nil {
		t.Errorf("rescanticketdb err: %v", err.Error())
	}

	// Remove all blocks and rescan too
	_, _, _, err =
		chain.TMDB().RemoveBlockToHeight(simNetParams.StakeEnabledHeight)
	if err != nil {
		t.Errorf("error: %v", err)
	}
	err = chain.TMDB().RescanTicketDB()
	if err != nil {
		t.Errorf("rescanticketdb err: %v", err.Error())
	}

	// Test if the db file storage was symmetric to the earlier snapshot
	if !reflect.DeepEqual(chain.TMDB().DumpMapsPointer(), CopyOfMapsAtBlock168) {
		t.Errorf("The td did not rescan to HEAD correctly!")
	}

	err = os.Mkdir("testdata/", os.FileMode(0700))
	if err != nil {
		t.Error(err)
	}

	// Store the ticket db to disk
	err = chain.TMDB().Store("testdata/", "testtmdb")
	if err != nil {
		t.Errorf("error: %v", err)
	}

	var tmdb2 stake.TicketDB
	err = tmdb2.LoadTicketDBs("testdata/", "testtmdb", simNetParams, chain.DB())
	if err != nil {
		t.Errorf("error: %v", err)
	}

	// Test if the db file storage was symmetric to previously rescanned one
	if !reflect.DeepEqual(chain.TMDB().DumpMapsPointer(), tmdb2.DumpMapsPointer()) {
		t.Errorf("The td did not rescan to a previous block height correctly!")
	}

	tmdb2.Close()

	// Test dumping missing tickets from block 152
	missedIn152, _ := chainhash.NewHashFromStr(
		"84f7f866b0af1cc278cb8e0b2b76024a07542512c76487c83628c14c650de4fa")

	chain.TMDB().RemoveBlockToHeight(152)

	missedTix, err := chain.TMDB().DumpMissedTickets()
	if err != nil {
		t.Errorf("err dumping missed tix: %v", err.Error())
	}

	if _, exists := missedTix[*missedIn152]; !exists {
		t.Errorf("couldn't finding missed tx 1 %v in tmdb @ block 152!",
			missedIn152)
	}

	chain.TMDB().RescanTicketDB()

	// Make sure that the revoked map contains the revoked tx
	revokedSlice := []*chainhash.Hash{missedIn152}

	revokedTix, err := chain.TMDB().DumpRevokedTickets()
	if err != nil {
		t.Errorf("err dumping missed tix: %v", err.Error())
	}

	if len(revokedTix) != 1 {
		t.Errorf("revoked ticket map is wrong len, got %v, want %v",
			len(revokedTix), 1)
	}

	_, wasMissedIn152 := revokedTix[*revokedSlice[0]]
	ticketsRevoked := wasMissedIn152
	if !ticketsRevoked {
		t.Errorf("revoked ticket map did not include tickets missed in " +
			"block 152 and later revoked")
	}

	os.RemoveAll("ticketdb_test")
	os.Remove("./ticketdb_test.ver")
	os.Remove("testdata/testtmdb")
	os.Remove("testdata")
}

// --------------------------------------------------------------------------------
// TESTING VARIABLES BEGIN HERE

// simNetPowLimit is the highest proof of work value a Decred block
// can have for the simulation test network.  It is the value 2^255 - 1.
var simNetPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne)

// SimNetParams defines the network parameters for the simulation test Decred
// network.  This network is similar to the normal test network except it is
// intended for private use within a group of individuals doing simulation
// testing.  The functionality is intended to differ in that the only nodes
// which are specifically specified are used to create the network rather than
// following normal discovery rules.  This is important as otherwise it would
// just turn into another public testnet.
var simNetParams = &chaincfg.Params{
	Name:        "simnet",
	Net:         wire.SimNet,
	DefaultPort: "18555",

	// Chain parameters
	GenesisBlock:             &simNetGenesisBlock,
	GenesisHash:              &simNetGenesisHash,
	CurrentBlockVersion:      0,
	PowLimit:                 simNetPowLimit,
	PowLimitBits:             0x207fffff,
	ResetMinDifficulty:       false,
	GenerateSupported:        true,
	MaximumBlockSize:         1000000,
	TimePerBlock:             time.Second * 1,
	WorkDiffAlpha:            1,
	WorkDiffWindowSize:       8,
	WorkDiffWindows:          4,
	TargetTimespan:           time.Second * 1 * 8, // TimePerBlock * WindowSize
	RetargetAdjustmentFactor: 4,

	// Subsidy parameters.
	BaseSubsidy:           50000000000,
	MulSubsidy:            100,
	DivSubsidy:            101,
	ReductionInterval:     128,
	WorkRewardProportion:  6,
	StakeRewardProportion: 3,
	BlockTaxProportion:    1,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: nil,

	// Mempool parameters
	RelayNonStdTxs: true,

	// Address encoding magics
	PubKeyAddrID:     [2]byte{0x27, 0x6f}, // starts with Sk
	PubKeyHashAddrID: [2]byte{0x0e, 0x91}, // starts with Ss
	PKHEdwardsAddrID: [2]byte{0x0e, 0x71}, // starts with Se
	PKHSchnorrAddrID: [2]byte{0x0e, 0x53}, // starts with SS
	ScriptHashAddrID: [2]byte{0x0e, 0x6c}, // starts with Sc
	PrivateKeyID:     [2]byte{0x23, 0x07}, // starts with Ps

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x20, 0xb9, 0x03}, // starts with sprv
	HDPublicKeyID:  [4]byte{0x04, 0x20, 0xbd, 0x3d}, // starts with spub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 115, // ASCII for s

	// Decred PoS parameters
	MinimumStakeDiff:      20000,
	TicketPoolSize:        64,
	TicketsPerBlock:       5,
	TicketMaturity:        16,
	TicketExpiry:          256, // 4*TicketPoolSize
	CoinbaseMaturity:      16,
	SStxChangeMaturity:    1,
	TicketPoolSizeWeight:  4,
	StakeDiffAlpha:        1,
	StakeDiffWindowSize:   8,
	StakeDiffWindows:      8,
	MaxFreshStakePerBlock: 40,            // 8*TicketsPerBlock
	StakeEnabledHeight:    16 + 16,       // CoinbaseMaturity + TicketMaturity
	StakeValidationHeight: 16 + (64 * 2), // CoinbaseMaturity + TicketPoolSize*2
	StakeBaseSigScript:    []byte{0xDE, 0xAD, 0xBE, 0xEF},

	// Decred organization related parameters
	//
	// "Dev org" address is a 3-of-3 P2SH going to wallet:
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// aardvark adroitness aardvark adroitness
	// briefcase
	// (seed 0x00000000000000000000000000000000000000000000000000000000000000)
	//
	// This same wallet owns the three ledger outputs for simnet.
	//
	// P2SH details for simnet dev org is below.
	//
	// address: Scc4ZC844nzuZCXsCFXUBXTLks2mD6psWom
	// redeemScript: 532103e8c60c7336744c8dcc7b85c27789950fc52aa4e48f895ebbfb
	// ac383ab893fc4c2103ff9afc246e0921e37d12e17d8296ca06a8f92a07fbe7857ed1d4
	// f0f5d94e988f21033ed09c7fa8b83ed53e6f2c57c5fa99ed2230c0d38edf53c0340d0f
	// c2e79c725a53ae
	//   (3-of-3 multisig)
	// Pubkeys used:
	//   SkQmxbeuEFDByPoTj41TtXat8tWySVuYUQpd4fuNNyUx51tF1csSs
	//   SkQn8ervNvAUEX5Ua3Lwjc6BAuTXRznDoDzsyxgjYqX58znY7w9e4
	//   SkQkfkHZeBbMW8129tZ3KspEh1XBFC1btbkgzs6cjSyPbrgxzsKqk
	//
	OrganizationAddress: "ScuQxvveKGfpG1ypt6u27F99Anf7EW3cqhq",
	BlockOneLedger:      BlockOneLedgerSimNet,
}

// BlockOneLedgerSimNet is the block one output ledger for the simulation
// network. See below under "Decred organization related parameters" for
// information on how to spend these outputs.
var BlockOneLedgerSimNet = []*chaincfg.TokenPayout{
	{Address: "Sshw6S86G2bV6W32cbc7EhtFy8f93rU6pae", Amount: 100000 * 1e8},
	{Address: "SsjXRK6Xz6CFuBt6PugBvrkdAa4xGbcZ18w", Amount: 100000 * 1e8},
	{Address: "SsfXiYkYkCoo31CuVQw428N6wWKus2ZEw5X", Amount: 100000 * 1e8},
}

var bigOne = new(big.Int).SetInt64(1)

// simNetGenesisHash is the hash of the first block in the block chain for the
// simulation test network.
var simNetGenesisHash = simNetGenesisBlock.BlockSha()

// simNetGenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the simulation test network.  It is the same as the merkle root for
// the main network.
var simNetGenesisMerkleRoot = genesisMerkleRoot

// genesisCoinbaseTx legacy is the coinbase transaction for the genesis blocks for
// the regression test network and test network.
var genesisCoinbaseTxLegacy = wire.MsgTx{
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

// genesisMerkleRoot is the hash of the first transaction in the genesis block
// for the main network.
var genesisMerkleRoot = genesisCoinbaseTxLegacy.TxSha()

var regTestGenesisCoinbaseTx = wire.MsgTx{
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
			Value:   0x00000000,
			Version: 0x0000,
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
		VoteBits:    uint16(0x0000),
		FinalState:  [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Voters:      uint16(0x0000),
		FreshStake:  uint8(0x00),
		Revocations: uint8(0x00),
		Timestamp:   time.Unix(1401292357, 0), // 2009-01-08 20:54:25 -0600 CST
		PoolSize:    uint32(0),
		Bits:        0x207fffff, // 545259519
		SBits:       int64(0x0000000000000000),
		Nonce:       0x00000000,
		Height:      uint32(0),
	},
	Transactions:  []*wire.MsgTx{&regTestGenesisCoinbaseTx},
	STransactions: []*wire.MsgTx{},
}
