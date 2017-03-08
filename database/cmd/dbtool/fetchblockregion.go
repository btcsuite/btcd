// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/hex"
	"errors"
	"strconv"
	"time"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
)

// blockRegionCmd defines the configuration options for the fetchblockregion
// command.
type blockRegionCmd struct{}

var (
	// blockRegionCfg defines the configuration options for the command.
	blockRegionCfg = blockRegionCmd{}
)

// Execute is the main entry point for the command.  It's invoked by the parser.
func (cmd *blockRegionCmd) Execute(args []string) error {
	// Setup the global config options and ensure they are valid.
	if err := setupGlobalConfig(); err != nil {
		return err
	}

	// Ensure expected arguments.
	if len(args) < 1 {
		return errors.New("required block hash parameter not specified")
	}
	if len(args) < 2 {
		return errors.New("required start offset parameter not " +
			"specified")
	}
	if len(args) < 3 {
		return errors.New("required region length parameter not " +
			"specified")
	}

	// Parse arguments.
	blockHash, err := chainhash.NewHashFromStr(args[0])
	if err != nil {
		return err
	}
	startOffset, err := strconv.ParseUint(args[1], 10, 32)
	if err != nil {
		return err
	}
	regionLen, err := strconv.ParseUint(args[2], 10, 32)
	if err != nil {
		return err
	}

	// Load the block database.
	db, err := loadBlockDB()
	if err != nil {
		return err
	}
	defer db.Close()

	return db.View(func(tx database.Tx) error {
		log.Infof("Fetching block region %s<%d:%d>", blockHash,
			startOffset, startOffset+regionLen-1)
		region := database.BlockRegion{
			Hash:   blockHash,
			Offset: uint32(startOffset),
			Len:    uint32(regionLen),
		}
		startTime := time.Now()
		regionBytes, err := tx.FetchBlockRegion(&region)
		if err != nil {
			return err
		}
		log.Infof("Loaded block region in %v", time.Since(startTime))
		log.Infof("Region Hex: %s", hex.EncodeToString(regionBytes))
		return nil
	})
}

// Usage overrides the usage display for the command.
func (cmd *blockRegionCmd) Usage() string {
	return "<block-hash> <start-offset> <length-of-region>"
}
