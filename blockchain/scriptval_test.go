// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/txscript"
)

// TestCheckBlockScripts ensures that validating the all of the scripts in a
// known-good block doesn't return an error.
func TestCheckBlockScripts(t *testing.T) {
	runtime.GOMAXPROCS(runtime.NumCPU())

	testBlockNum := 277647
	blockDataFile := fmt.Sprintf("%d.dat.bz2", testBlockNum)
	blocks, err := loadBlocks(blockDataFile)
	if err != nil {
		t.Errorf("Error loading file: %v\n", err)
		return
	}
	if len(blocks) > 1 {
		t.Errorf("The test block file must only have one block in it")
	}

	txStoreDataFile := fmt.Sprintf("%d.txstore.bz2", testBlockNum)
	txStore, err := loadTxStore(txStoreDataFile)
	if err != nil {
		t.Errorf("Error loading txstore: %v\n", err)
		return
	}

	scriptFlags := txscript.ScriptBip16
	err = blockchain.TstCheckBlockScripts(blocks[0], txStore, scriptFlags, nil)
	if err != nil {
		t.Errorf("Transaction script validation failed: %v\n",
			err)
		return
	}
}
