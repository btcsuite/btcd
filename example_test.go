// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain_test

import (
	"fmt"
	"github.com/conformal/btcchain"
	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcdb/memdb"
	"github.com/conformal/btcnet"
	"github.com/conformal/btcutil"
)

// This example demonstrates how to create a new chain instance and use
// ProcessBlock to attempt to attempt add a block to the chain.  As the package
// overview documentation describes, this includes all of the Bitcoin consensus
// rules.  This example intentionally attempts to insert a duplicate genesis
// block to illustrate how an invalid block is handled.
func ExampleBlockChain_ProcessBlock() {
	// Create a new database to store the accepted blocks into.  Typically
	// this would be opening an existing database and would not use memdb
	// which is a memory-only database backend, but we create a new db
	// here so this is a complete working example.
	db, err := btcdb.CreateDB("memdb")
	if err != nil {
		fmt.Printf("Failed to create database: %v\n", err)
		return
	}
	defer db.Close()

	// Insert the main network genesis block.  This is part of the initial
	// database setup.  Like above, this typically would not be needed when
	// opening an existing database.
	genesisBlock := btcutil.NewBlock(btcnet.MainNetParams.GenesisBlock)
	_, err = db.InsertBlock(genesisBlock)
	if err != nil {
		fmt.Printf("Failed to insert genesis block: %v\n", err)
		return
	}

	// Create a new BlockChain instance using the underlying database for
	// the main bitcoin network and ignore notifications.
	chain := btcchain.New(db, &btcnet.MainNetParams, nil)

	// Process a block.  For this example, we are going to intentionally
	// cause an error by trying to process the genesis block which already
	// exists.
	isOrphan, err := chain.ProcessBlock(genesisBlock, btcchain.BFNone)
	if err != nil {
		fmt.Printf("Failed to process block: %v\n", err)
		return
	}
	fmt.Println("Block accepted.  Is it an orphan?: %v", isOrphan)

	// Output:
	// Failed to process block: already have block 000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f
}
