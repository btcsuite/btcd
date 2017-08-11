// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"testing"
	"time"

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

	notifications := make(chan blockchain.Notification)
	chain.Subscribe(func(notification *blockchain.Notification) {
		go func() {
			notifications <- *notification
		}()
	})

	_, _, err = chain.ProcessBlock(blocks[1], blockchain.BFNone)
	if err != nil {
		t.Fatalf("ProcessBlock fail on block 1: %v\n", err)
	}

	select {
	case notification := <-notifications:
		if notification.Type != blockchain.NTBlockAccepted {
			t.Errorf("Expected NTBlockAccepted notification, got %v", notification.Type)
		}
	case <-time.After(time.Second):
		t.Error("Expected blockchain notification callback to fire")
	}
}
