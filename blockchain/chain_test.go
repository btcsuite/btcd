// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
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
	"github.com/decred/dcrutil"
)

// TestBlockchainFunction tests the various blockchain API to ensure proper
// functionality.
func TestBlockchainFunctions(t *testing.T) {
	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("validateunittests",
		simNetParams)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// The genesis block should fail to connect since it's already inserted.
	genesisBlock := simNetParams.GenesisBlock
	err = chain.CheckConnectBlock(dcrutil.NewBlock(genesisBlock))
	if err == nil {
		t.Errorf("CheckConnectBlock: Did not receive expected error")
	}

	// Load up the rest of the blocks up to HEAD~1.
	filename := filepath.Join("testdata/", "blocks0to168.bz2")
	fi, err := os.Open(filename)
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file.
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data.
	bcDecoder := gob.NewDecoder(bcBuf)
	blockChain := make(map[int64][]byte)

	// Decode the blockchain into the map.
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	// Insert blocks 1 to 168 and perform various tests.
	timeSource := blockchain.NewMedianTime()
	for i := 1; i <= 168; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Errorf("NewBlockFromBytes error: %v", err.Error())
		}
		bl.SetHeight(int64(i))

		_, _, err = chain.ProcessBlock(bl, timeSource, blockchain.BFNone)
		if err != nil {
			t.Fatalf("ProcessBlock error at height %v: %v", i, err.Error())
		}
	}

	val, err := chain.TicketPoolValue()
	if err != nil {
		t.Errorf("Failed to get ticket pool value: %v", err)
	}
	expectedVal := dcrutil.Amount(3495091704)
	if val != expectedVal {
		t.Errorf("Failed to get correct result for ticket pool value; "+
			"want %v, got %v", expectedVal, val)
	}

	a, _ := dcrutil.DecodeNetworkAddress("SsbKpMkPnadDcZFFZqRPY8nvdFagrktKuzB")
	hs, err := chain.TicketsWithAddress(a)
	if err != nil {
		t.Errorf("Failed to do TicketsWithAddress: %v", err)
	}
	expectedLen := 223
	if len(hs) != expectedLen {
		t.Errorf("Failed to get correct number of tickets for "+
			"TicketsWithAddress; want %v, got %v", expectedLen, len(hs))
	}

	totalSubsidy := chain.TotalSubsidy()
	expectedSubsidy := int64(35783267326630)
	if expectedSubsidy != totalSubsidy {
		t.Errorf("Failed to get correct total subsidy for "+
			"TotalSubsidy; want %v, got %v", expectedSubsidy,
			totalSubsidy)
	}
}
