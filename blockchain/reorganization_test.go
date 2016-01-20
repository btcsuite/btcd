// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"bytes"
	"compress/bzip2"
	"encoding/gob"
	"os"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrutil"
)

// TestReorganization loads a set of test blocks which force a chain
// reorganization to test the block chain handling code.
func TestReorganization(t *testing.T) {
	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("reorgunittest",
		simNetParams)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	err = chain.GenerateInitialIndex()
	if err != nil {
		t.Errorf("GenerateInitialIndex: %v", err)
	}

	// The genesis block should fail to connect since it's already
	// inserted.
	genesisBlock := simNetParams.GenesisBlock
	err = chain.CheckConnectBlock(dcrutil.NewBlock(genesisBlock))
	if err == nil {
		t.Errorf("CheckConnectBlock: Did not receive expected error")
	}

	// Load up the rest of the blocks up to HEAD.
	filename := filepath.Join("testdata/", "reorgto179.bz2")
	fi, err := os.Open(filename)
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder := gob.NewDecoder(bcBuf)
	blockChain := make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	// Load up the short chain
	timeSource := blockchain.NewMedianTime()
	finalIdx1 := 179
	for i := 1; i < finalIdx1+1; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Errorf("NewBlockFromBytes error: %v", err.Error())
		}
		bl.SetHeight(int64(i))

		_, _, err = chain.ProcessBlock(bl, timeSource, blockchain.BFNone)
		if err != nil {
			t.Errorf("ProcessBlock error: %v", err.Error())
		}
	}

	// Load the long chain and begin loading blocks from that too,
	// forcing a reorganization
	// Load up the rest of the blocks up to HEAD.
	filename = filepath.Join("testdata/", "reorgto180.bz2")
	fi, err = os.Open(filename)
	bcStream = bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf = new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder = gob.NewDecoder(bcBuf)
	blockChain = make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	forkPoint := 131
	finalIdx2 := 180
	for i := forkPoint; i < finalIdx2+1; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Errorf("NewBlockFromBytes error: %v", err.Error())
		}
		bl.SetHeight(int64(i))

		_, _, err = chain.ProcessBlock(bl, timeSource, blockchain.BFNone)
		if err != nil {
			t.Errorf("ProcessBlock error: %v", err.Error())
		}
	}

	// Ensure our blockchain is at the correct best tip
	topBlock, _ := chain.GetTopBlock()
	tipHash := topBlock.Sha()
	expected, _ := chainhash.NewHashFromStr("5ab969d0afd8295b6cd1506f2a310d" +
		"259322015c8bd5633f283a163ce0e50594")
	if *tipHash != *expected {
		t.Errorf("Failed to correctly reorg; expected tip %v, got tip %v",
			expected, tipHash)
	}
	have, err := chain.HaveBlock(expected)
	if !have {
		t.Errorf("missing tip block after reorganization test")
	}
	if err != nil {
		t.Errorf("unexpected error testing for presence of new tip block "+
			"after reorg test: %v", err)
	}
	return
}
