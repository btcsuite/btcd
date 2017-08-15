// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
)

// Test that notification callbacks are fired on events.
func TestNotifications(t *testing.T) {
	blocks, err := loadBlocks("blk_0_to_4.dat.bz2")
	if err != nil {
		t.Fatalf("Error loading file: %v\n", err)
	}

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err :=
		chainSetup("notifications", &chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("Failed to setup chain instance: %v", err)
	}
	defer teardownFunc()

	notificationCount := 0

	callback := func(notification *blockchain.Notification) {
		if notification.Type == blockchain.NTBlockAccepted {
			notificationCount++
		}
	}

	// Register callback 3 times then assert it is called three times
	chain.Subscribe(callback)
	chain.Subscribe(callback)
	chain.Subscribe(callback)

	_, _, err = chain.ProcessBlock(blocks[1], blockchain.BFNone)
	if err != nil {
		t.Fatalf("ProcessBlock fail on block 1: %v\n", err)
	}

	if notificationCount != 3 {
		t.Fatalf("Expected notification callback to be executed 3 times, found %d",
			notificationCount)
	}
}
