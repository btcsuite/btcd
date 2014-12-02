// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/conformal/btclog"
	"github.com/mably/btcchain"
	"github.com/mably/btcdb"
	_ "github.com/mably/btcdb/ldb"
	"github.com/mably/ppcd/limits"
)

const (
	// blockDbNamePrefix is the prefix for the btcd block database.
	blockDbNamePrefix = "blocks"
)

var (
	cfg *config
	log btclog.Logger
)

// loadBlockDB opens the block database and returns a handle to it.
func loadBlockDB() (btcdb.Db, error) {
	// The database name is based on the database type.
	dbName := blockDbNamePrefix + "_" + cfg.DbType
	if cfg.DbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(cfg.DataDir, dbName)

	log.Infof("Loading block database from '%s'", dbPath)
	db, err := btcdb.OpenDB(cfg.DbType, dbPath)
	if err != nil {
		// Return the error if it's not because the database doesn't
		// exist.
		if err != btcdb.ErrDbDoesNotExist {
			return nil, err
		}

		// Create the db if it does not exist.
		err = os.MkdirAll(cfg.DataDir, 0700)
		if err != nil {
			return nil, err
		}
		db, err = btcdb.CreateDB(cfg.DbType, dbPath)
		if err != nil {
			return nil, err
		}
	}

	// Get the latest block height from the database.
	_, height, err := db.NewestSha()
	if err != nil {
		db.Close()
		return nil, err
	}

	log.Infof("Block database loaded with block height %d", height)
	return db, nil
}

// realMain is the real main function for the utility.  It is necessary to work
// around the fact that deferred functions do not run when os.Exit() is called.
func realMain() error {
	// Load configuration and parse command line.
	tcfg, _, err := loadConfig()
	if err != nil {
		return err
	}
	cfg = tcfg

	// Setup logging.
	backendLogger := btclog.NewDefaultBackendLogger()
	defer backendLogger.Flush()
	log = btclog.NewSubsystemLogger(backendLogger, "")
	btcdb.UseLogger(btclog.NewSubsystemLogger(backendLogger, "BCDB: "))
	btcchain.UseLogger(btclog.NewSubsystemLogger(backendLogger, "CHAN: "))

	// Load the block database.
	db, err := loadBlockDB()
	if err != nil {
		log.Errorf("Failed to load database: %v", err)
		return err
	}
	defer db.Close()

	fi, err := os.Open(cfg.InFile)
	if err != nil {
		log.Errorf("Failed to open file %v: %v", cfg.InFile, err)
		return err
	}
	defer fi.Close()

	// Create a block importer for the database and input file and start it.
	// The done channel returned from start will contain an error if
	// anything went wrong.
	importer := newBlockImporter(db, fi)

	// Perform the import asynchronously.  This allows blocks to be
	// processed and read in parallel.  The results channel returned from
	// Import contains the statistics about the import including an error
	// if something went wrong.
	log.Info("Starting import")
	resultsChan := importer.Import()
	results := <-resultsChan
	if results.err != nil {
		log.Errorf("%v", results.err)
		return results.err
	}

	log.Infof("Processed a total of %d blocks (%d imported, %d already "+
		"known)", results.blocksProcessed, results.blocksImported,
		results.blocksProcessed-results.blocksImported)
	return nil
}

func main() {
	// Use all processor cores and up some limits.
	runtime.GOMAXPROCS(runtime.NumCPU())
	if err := limits.SetLimits(); err != nil {
		os.Exit(1)
	}

	// Work around defer not working after os.Exit()
	if err := realMain(); err != nil {
		os.Exit(1)
	}
}
