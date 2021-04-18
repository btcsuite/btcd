// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/hex"
	"errors"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
)

// fetchBlockCmd defines the configuration options for the fetchblock command.
type fetchBlockCmd struct{}

var (
	// fetchBlockCfg defines the configuration options for the command.
	fetchBlockCfg = fetchBlockCmd{}
)

// Execute is the main entry point for the command.  It's invoked by the parser.
func (cmd *fetchBlockCmd) Execute(args []string) error {
	// Setup the global config options and ensure they are valid.
	if err := setupGlobalConfig(); err != nil {
		return err
	}

	if len(args) < 1 {
		return errors.New("required block hash parameter not specified")
	}
	blockHash, err := chainhash.NewHashFromStr(args[0])
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
		log.Infof("Fetching block %s", blockHash)
		startTime := time.Now()
		blockBytes, err := tx.FetchBlock(blockHash)
		if err != nil {
			return err
		}
		log.Infof("Loaded block in %v", time.Since(startTime))
		log.Infof("Block Hex: %s", hex.EncodeToString(blockBytes))
		return nil
	})
}

// Usage overrides the usage display for the command.
func (cmd *fetchBlockCmd) Usage() string {
	return "<block-hash>"
}
