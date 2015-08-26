// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	//	"fmt"
	"math/big"
	"testing"
	//	"time"

	"github.com/decred/dcrd/blockchain"
	//	"github.com/decred/dcrd/blockchain/stake"
	//	"github.com/decred/dcrd/chaincfg"
	//	"github.com/decred/dcrd/database"
	//"github.com/decred/dcrutil"
)

func TestBigToCompact(t *testing.T) {
	tests := []struct {
		in  int64
		out uint32
	}{
		{0, 0},
		{-1, 25231360},
	}

	for x, test := range tests {
		n := big.NewInt(test.in)
		r := blockchain.BigToCompact(n)
		if r != test.out {
			t.Errorf("TestBigToCompact test #%d failed: got %d want %d\n",
				x, r, test.out)
			return
		}
	}
}

func TestCompactToBig(t *testing.T) {
	tests := []struct {
		in  uint32
		out int64
	}{
		{10000000, 0},
	}

	for x, test := range tests {
		n := blockchain.CompactToBig(test.in)
		want := big.NewInt(test.out)
		if n.Cmp(want) != 0 {
			t.Errorf("TestCompactToBig test #%d failed: got %d want %d\n",
				x, n.Int64(), want.Int64())
			return
		}
	}
}

func TestCalcWork(t *testing.T) {
	tests := []struct {
		in  uint32
		out int64
	}{
		{10000000, 0},
	}

	for x, test := range tests {
		bits := uint32(test.in)

		r := blockchain.CalcWork(bits)
		if r.Int64() != test.out {
			t.Errorf("TestCalcWork test #%d failed: got %v want %d\n",
				x, r.Int64(), test.out)
			return
		}
	}
}

// TODO Make more elaborate tests for difficulty. The difficulty algorithms
// have already been tested to death in simnet/testnet/mainnet simulations,
// but we should really have a unit test for them that includes tests for
// edge cases.
func TestDiff(t *testing.T) {
	/*
		db, err := database.Create("memdb")
		if err != nil {
			t.Errorf("error creating db: %v", err)
		}

		// Setup a teardown function for cleaning up.  This function is
		// returned to the caller to be invoked when it is done testing.
		teardown := func() {
			db.Close()
		}
		defer teardown()

		// var tmdb *stake.TicketDB

		// Create the main chain instance.
		chain, err := blockchain.New(&blockchain.Config{
			DB:          db,
			ChainParams: &chaincfg.MainNetParams,
		})

		//timeSource := blockchain.NewMedianTime()

		// Grab some blocks

		// Build fake blockchain

		// Calc new difficulty

		ts := time.Now()

		d, err := chain.CalcNextRequiredDifficulty(ts)
		if err != nil {
			t.Errorf("Failed to get difficulty: %v\n", err)
			return
		}
		if d != 486604799 { // This is hardcoded in genesis block but not exported anywhere.
			t.Error("Failed to get initial difficulty.")
		}

		sd, err := chain.CalcNextRequiredStakeDifficulty()
		if err != nil {
			t.Errorf("Failed to get stake difficulty: %v\n", err)
			return
		}
		if sd != chaincfg.MainNetParams.MinimumStakeDiff {
			t.Error("Incorrect initial stake difficulty.")
		}

		// Compare

		// Repeat for a few more
	*/
}
