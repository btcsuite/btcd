// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"fmt"
	"math/big"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/memdb"
	"github.com/decred/dcrutil"
)

// This example demonstrates how to create a new chain instance and use
// ProcessBlock to attempt to attempt add a block to the chain.  As the package
// overview documentation describes, this includes all of the Decred consensus
// rules.  This example intentionally attempts to insert a duplicate genesis
// block to illustrate how an invalid block is handled.
func ExampleBlockChain_ProcessBlock() {
	// Create a new database to store the accepted blocks into.  Typically
	// this would be opening an existing database and would not use memdb
	// which is a memory-only database backend, but we create a new db
	// here so this is a complete working example.
	db, err := database.CreateDB("memdb")
	if err != nil {
		fmt.Printf("Failed to create database: %v\n", err)
		return
	}
	defer db.Close()

	var tmdb *stake.TicketDB
	// Insert the main network genesis block.  This is part of the initial
	// database setup.  Like above, this typically would not be needed when
	// opening an existing database.
	genesisBlock := dcrutil.NewBlock(chaincfg.MainNetParams.GenesisBlock)
	_, err = db.InsertBlock(genesisBlock)
	if err != nil {
		fmt.Printf("Failed to insert genesis block: %v\n", err)
		return
	}

	// Create a new BlockChain instance without an initialized signature
	// verification cache, using the underlying database for the main
	// bitcoin network and ignore notifications.
	chain := blockchain.New(db, tmdb, &chaincfg.MainNetParams, nil, nil)

	// Create a new median time source that is required by the upcoming
	// call to ProcessBlock.  Ordinarily this would also add time values
	// obtained from other peers on the network so the local time is
	// adjusted to be in agreement with other peers.
	timeSource := blockchain.NewMedianTime()

	// Process a block.  For this example, we are going to intentionally
	// cause an error by trying to process the genesis block which already
	// exists.
	isOrphan, _, err := chain.ProcessBlock(genesisBlock, timeSource, blockchain.BFNone)
	if err != nil {
		fmt.Printf("Failed to process block: %v\n", err)
		return
	}
	fmt.Printf("Block accepted. Is it an orphan?: %v", isOrphan)

	// This output is dependent on the genesis block, and needs to be
	// updated if the mainnet genesis block is updated.
	// Output:
	// Failed to process block: already have block 267a53b5ee86c24a48ec37aee4f4e7c0c4004892b7259e695e9f5b321f1ab9d2
}

// This example demonstrates how to convert the compact "bits" in a block header
// which represent the target difficulty to a big integer and display it using
// the typical hex notation.
func ExampleCompactToBig() {
	// Convert the bits from block 300000 in the main Decred block chain.
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
