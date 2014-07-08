// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcdb_test

import (
	"fmt"
	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcdb/memdb"
	"github.com/conformal/btcnet"
	"github.com/conformal/btcutil"
)

// This example demonstrates creating a new database and inserting the genesis
// block into it.
func ExampleCreateDB() {
	// Notice in these example imports that the memdb driver is loaded.
	// Ordinarily this would be whatever driver(s) your application
	// requires.
	// import (
	//	"github.com/conformal/btcdb"
	// 	_ "github.com/conformal/btcdb/memdb"
	// )

	// Create a database and schedule it to be closed on exit.  This example
	// uses a memory-only database to avoid needing to write anything to
	// the disk.  Typically, you would specify a persistent database driver
	// such as "leveldb" and give it a database name as the second
	// parameter.
	db, err := btcdb.CreateDB("memdb")
	if err != nil {
		fmt.Println(err)
		return
	}
	defer db.Close()

	// Insert the main network genesis block.
	genesis := btcutil.NewBlock(btcnet.MainNetParams.GenesisBlock)
	newHeight, err := db.InsertBlock(genesis)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("New height:", newHeight)

	// Output:
	// New height: 0
}
