// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

var (
	btcdHomeDir     = btcutil.AppDataDir("btcd", false)
	knownDbTypes    = database.SupportedDrivers()
	activeNetParams = &chaincfg.MainNetParams

	// Default global config.
	cfg = &config{
		DataDir: filepath.Join(btcdHomeDir, "data"),
		DbType:  "ffldb",
	}
)

// config defines the global configuration options.
type config struct {
	DataDir        string `short:"b" long:"datadir" description:"Location of the btcd data directory"`
	DbType         string `long:"dbtype" description:"Database backend to use for the Block Chain"`
	TestNet3       bool   `long:"testnet" description:"Use the test network"`
	RegressionTest bool   `long:"regtest" description:"Use the regression test network"`
	SimNet         bool   `long:"simnet" description:"Use the simulation test network"`
}

// fileExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// validDbType returns whether or not dbType is a supported database type.
func validDbType(dbType string) bool {
	for _, knownType := range knownDbTypes {
		if dbType == knownType {
			return true
		}
	}

	return false
}

// netName returns the name used when referring to a bitcoin network.  At the
// time of writing, btcd currently places blocks for testnet version 3 in the
// data and log directory "testnet", which does not match the Name field of the
// chaincfg parameters.  This function can be used to override this directory name
// as "testnet" when the passed active network matches wire.TestNet3.
//
// A proper upgrade to move the data and log directories for this network to
// "testnet3" is planned for the future, at which point this function can be
// removed and the network parameter's name used instead.
func netName(chainParams *chaincfg.Params) string {
	switch chainParams.Net {
	case wire.TestNet3:
		return "testnet"
	default:
		return chainParams.Name
	}
}

// setupGlobalConfig examine the global configuration options for any conditions
// which are invalid as well as performs any addition setup necessary after the
// initial parse.
func setupGlobalConfig() error {
	// Multiple networks can't be selected simultaneously.
	// Count number of network flags passed; assign active network params
	// while we're at it
	numNets := 0
	if cfg.TestNet3 {
		numNets++
		activeNetParams = &chaincfg.TestNet3Params
	}
	if cfg.RegressionTest {
		numNets++
		activeNetParams = &chaincfg.RegressionNetParams
	}
	if cfg.SimNet {
		numNets++
		activeNetParams = &chaincfg.SimNetParams
	}
	if numNets > 1 {
		return errors.New("The testnet, regtest, and simnet params " +
			"can't be used together -- choose one of the three")
	}

	// Validate database type.
	if !validDbType(cfg.DbType) {
		str := "The specified database type [%v] is invalid -- " +
			"supported types %v"
		return fmt.Errorf(str, cfg.DbType, knownDbTypes)
	}

	// Append the network type to the data directory so it is "namespaced"
	// per network.  In addition to the block database, there are other
	// pieces of data that are saved to disk such as address manager state.
	// All data is specific to a network, so namespacing the data directory
	// means each individual piece of serialized data does not have to
	// worry about changing names per network and such.
	cfg.DataDir = filepath.Join(cfg.DataDir, netName(activeNetParams))

	return nil
}
