// Copyright (c) 2014-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"fmt"
	"math/big"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
)

// This example demonstrates how to create a new chain instance and use
// ProcessBlock to attempt to add a block to the chain.  As the package
// overview documentation describes, this includes all of the Bitcoin consensus
// rules.  This example intentionally attempts to insert a duplicate genesis
// block to illustrate how an invalid block is handled.
func ExampleBlockChain_ProcessBlock() {
	// Create a new database to store the accepted blocks into.  Typically
	// this would be opening an existing database and would not be deleting
	// and creating a new database like this, but it is done here so this is
	// a complete working example and does not leave temporary files laying
	// around.
	dbPath := filepath.Join(os.TempDir(), "exampleprocessblock")
	_ = os.RemoveAll(dbPath)
	db, err := database.Create("ffldb", dbPath, chaincfg.MainNetParams.Net)
	if err != nil {
		fmt.Printf("Failed to create database: %v\n", err)
		return
	}
	defer os.RemoveAll(dbPath)
	defer db.Close()

	// Create a new BlockChain instance using the underlying database for
	// the main bitcoin network.  This example does not demonstrate some
	// of the other available configuration options such as specifying a
	// notification callback and signature cache.  Also, the caller would
	// ordinarily keep a reference to the median time source and add time
	// values obtained from other peers on the network so the local time is
	// adjusted to be in agreement with other peers.
	chain, err := blockchain.New(&blockchain.Config{
		DB:          db,
		ChainParams: &chaincfg.MainNetParams,
		TimeSource:  blockchain.NewMedianTime(),
	})
	if err != nil {
		fmt.Printf("Failed to create chain instance: %v\n", err)
		return
	}

	// Process a block.  For this example, we are going to intentionally
	// cause an error by trying to process the genesis block which already
	// exists.
	genesisBlock := btcutil.NewBlock(chaincfg.MainNetParams.GenesisBlock)
	isMainChain, isOrphan, err := chain.ProcessBlock(genesisBlock,
		blockchain.BFNone)
	if err != nil {
		fmt.Printf("Failed to process block: %v\n", err)
		return
	}
	fmt.Printf("Block accepted. Is it on the main chain?: %v", isMainChain)
	fmt.Printf("Block accepted. Is it an orphan?: %v", isOrphan)

	// Output:
	// Failed to process block: already have block 000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f
}

// This example demonstrates how to convert the compact "bits" in a block header
// which represent the target difficulty to a big integer and display it using
// the typical hex notation.
func ExampleCompactToBig() {
	// Convert the bits from block 300000 in the main block chain.
	bits := uint32(419465580)
	targetDifficulty := blockchain.CompactToBig(bits)

	// Display it in hex.
	fmt.Printf("%064x\n", targetDifficulty.Bytes())

	// Output:
	// 0000000000000000896c00000000000000000000000000000000000000000000
}

// This example demonstrates how to convert a target difficulty into the compact
// "bits" in a block header which represent that target difficulty .
func ExampleBigToCompact() {
	// Convert the target difficulty from block 300000 in the main block
	// chain to compact form.
	t := "0000000000000000896c00000000000000000000000000000000000000000000"
	targetDifficulty, success := new(big.Int).SetString(t, 16)
	if !success {
		fmt.Println("invalid target difficulty")
		return
	}
	bits := blockchain.BigToCompact(targetDifficulty)

	fmt.Println(bits)

	// Output:
	// 419465580
}
