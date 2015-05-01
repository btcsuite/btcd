// Copyright (c) 2013 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ldb"
	"github.com/btcsuite/btcd/wire"
)

const blockDbNamePrefix = "blocks"

var (
	cfg *config
)

// loadBlockDB opens the block database and returns a handle to it.
func loadBlockDB() (database.Db, error) {
	// The database name is based on the database type.
	dbType := cfg.DbType
	dbName := blockDbNamePrefix + "_" + dbType
	if dbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(cfg.DataDir, dbName)
	fmt.Printf("Loading block database from '%s'\n", dbPath)
	db, err := database.OpenDB(dbType, dbPath)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// findCandidates searches the chain backwards for checkpoint candidates and
// returns a slice of found candidates, if any.  It also stops searching for
// candidates at the last checkpoint that is already hard coded into btcchain
// since there is no point in finding candidates before already existing
// checkpoints.
func findCandidates(db database.Db, latestHash *wire.ShaHash) ([]*chaincfg.Checkpoint, error) {
	// Start with the latest block of the main chain.
	block, err := db.FetchBlockBySha(latestHash)
	if err != nil {
		return nil, err
	}

	// Setup chain and get the latest checkpoint.  Ignore notifications
	// since they aren't needed for this util.
	chain := blockchain.New(db, activeNetParams, nil)
	latestCheckpoint := chain.LatestCheckpoint()
	if latestCheckpoint == nil {
		return nil, fmt.Errorf("unable to retrieve latest checkpoint")
	}

	// The latest known block must be at least the last known checkpoint
	// plus required checkpoint confirmations.
	checkpointConfirmations := int64(blockchain.CheckpointConfirmations)
	requiredHeight := latestCheckpoint.Height + checkpointConfirmations
	if block.Height() < requiredHeight {
		return nil, fmt.Errorf("the block database is only at height "+
			"%d which is less than the latest checkpoint height "+
			"of %d plus required confirmations of %d",
			block.Height(), latestCheckpoint.Height,
			checkpointConfirmations)
	}

	// Indeterminate progress setup.
	numBlocksToTest := block.Height() - requiredHeight
	progressInterval := (numBlocksToTest / 100) + 1 // min 1
	fmt.Print("Searching for candidates")
	defer fmt.Println()

	// Loop backwards through the chain to find checkpoint candidates.
	candidates := make([]*chaincfg.Checkpoint, 0, cfg.NumCandidates)
	numTested := int64(0)
	for len(candidates) < cfg.NumCandidates && block.Height() > requiredHeight {
		// Display progress.
		if numTested%progressInterval == 0 {
			fmt.Print(".")
		}

		// Determine if this block is a checkpoint candidate.
		isCandidate, err := chain.IsCheckpointCandidate(block)
		if err != nil {
			return nil, err
		}

		// All checks passed, so this node seems like a reasonable
		// checkpoint candidate.
		if isCandidate {
			checkpoint := chaincfg.Checkpoint{
				Height: block.Height(),
				Hash:   block.Sha(),
			}
			candidates = append(candidates, &checkpoint)
		}

		prevHash := &block.MsgBlock().Header.PrevBlock
		block, err = db.FetchBlockBySha(prevHash)
		if err != nil {
			return nil, err
		}
		numTested++
	}
	return candidates, nil
}

// showCandidate display a checkpoint candidate using and output format
// determined by the configuration parameters.  The Go syntax output
// uses the format the btcchain code expects for checkpoints added to the list.
func showCandidate(candidateNum int, checkpoint *chaincfg.Checkpoint) {
	if cfg.UseGoOutput {
		fmt.Printf("Candidate %d -- {%d, newShaHashFromStr(\"%v\")},\n",
			candidateNum, checkpoint.Height, checkpoint.Hash)
		return
	}

	fmt.Printf("Candidate %d -- Height: %d, Hash: %v\n", candidateNum,
		checkpoint.Height, checkpoint.Hash)

}

func main() {
	// Load configuration and parse command line.
	tcfg, _, err := loadConfig()
	if err != nil {
		return
	}
	cfg = tcfg

	// Load the block database.
	db, err := loadBlockDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load database: %v\n", err)
		return
	}
	defer db.Close()

	// Get the latest block hash and height from the database and report
	// status.
	latestHash, height, err := db.NewestSha()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return
	}
	fmt.Printf("Block database loaded with block height %d\n", height)

	// Find checkpoint candidates.
	candidates, err := findCandidates(db, latestHash)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Unable to identify candidates: %v", err)
		return
	}

	// No candidates.
	if len(candidates) == 0 {
		fmt.Println("No candidates found.")
		return
	}

	// Show the candidates.
	for i, checkpoint := range candidates {
		showCandidate(i+1, checkpoint)
	}
}
