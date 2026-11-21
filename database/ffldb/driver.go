// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ffldb

import (
	"fmt"

	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/btcsuite/btclog"
)

var log = btclog.Disabled

const (
	dbType = "ffldb"
)

// Options configures the ffldb driver. It can be passed as an optional third
// argument to database.Open and database.Create.
type Options struct {
	// BlockCacheSize is the LevelDB block cache capacity in bytes. Zero
	// selects LevelDB's default. It is independent of the UTXO cache.
	BlockCacheSize int
}

// openDriver validates the driver arguments and opens or creates the database.
func openDriver(funcName string, create bool, args ...interface{}) (database.DB, error) {
	if len(args) != 2 && len(args) != 3 {
		return nil, fmt.Errorf("invalid arguments to %s.%s -- expected "+
			"database path, block network and optional ffldb.Options",
			dbType, funcName)
	}

	dbPath, ok := args[0].(string)
	if !ok {
		return nil, fmt.Errorf("first argument to %s.%s is invalid -- "+
			"expected database path string", dbType, funcName)
	}

	network, ok := args[1].(wire.BitcoinNet)
	if !ok {
		return nil, fmt.Errorf("second argument to %s.%s is invalid -- "+
			"expected block network", dbType, funcName)
	}

	var opts Options
	if len(args) == 3 {
		opts, ok = args[2].(Options)
		if !ok {
			return nil, fmt.Errorf("third argument to %s.%s is "+
				"invalid -- expected ffldb.Options", dbType,
				funcName)
		}
		if opts.BlockCacheSize < 0 {
			return nil, fmt.Errorf("block cache size must be non-negative")
		}
	}

	return openDB(dbPath, network, create, opts)
}

// openDBDriver opens an existing database for use.
func openDBDriver(args ...interface{}) (database.DB, error) {
	return openDriver("Open", false, args...)
}

// createDBDriver creates and opens a new database.
func createDBDriver(args ...interface{}) (database.DB, error) {
	return openDriver("Create", true, args...)
}

// useLogger is the callback provided during driver registration that sets the
// current logger to the provided one.
func useLogger(logger btclog.Logger) {
	log = logger
}

func init() {
	// Register the driver.
	driver := database.Driver{
		DbType:    dbType,
		Create:    createDBDriver,
		Open:      openDBDriver,
		UseLogger: useLogger,
	}
	if err := database.RegisterDriver(driver); err != nil {
		panic(fmt.Sprintf("Failed to register database driver '%s': %v",
			dbType, err))
	}
}
