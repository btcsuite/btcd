// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcdb/ldb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/go-flags"
	"os"
	"path/filepath"
)

const (
	defaultDbType   = "leveldb"
	defaultDataFile = "bootstrap.dat"
	defaultProgress = 10000
)

var (
	btcdHomeDir    = btcutil.AppDataDir("btcd", false)
	defaultDataDir = filepath.Join(btcdHomeDir, "data")
	knownDbTypes   = btcdb.SupportedDBs()
	activeNetwork  = btcwire.MainNet
)

// config defines the configuration options for findcheckpoint.
//
// See loadConfig for details on the configuration load process.
type config struct {
	DataDir  string `short:"b" long:"datadir" description:"Location of the btcd data directory"`
	DbType   string `long:"dbtype" description:"Database backend to use for the Block Chain"`
	TestNet3 bool   `long:"testnet" description:"Use the test network"`
	InFile   string `short:"i" long:"infile" description:"File containing the block(s)"`
	Progress int    `short:"p" long:"progress" description:"Show a progress message every time this number of blocks is processed -- Use 0 to disable progress announcements"`
}

// filesExists reports whether the named file or directory exists.
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

// netName returns a human-readable name for the passed bitcoin network.
func netName(btcnet btcwire.BitcoinNet) string {
	net := "mainnet"
	if btcnet == btcwire.TestNet3 {
		net = "testnet"
	}
	return net
}

// loadConfig initializes and parses the config using command line options.
func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		DataDir:  defaultDataDir,
		DbType:   defaultDbType,
		InFile:   defaultDataFile,
		Progress: defaultProgress,
	}

	// Parse command line options.
	parser := flags.NewParser(&cfg, flags.Default)
	remainingArgs, err := parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			parser.WriteHelp(os.Stderr)
		}
		return nil, nil, err
	}

	// Choose the active network based on the flags.
	if cfg.TestNet3 {
		activeNetwork = btcwire.TestNet3
	}

	// Validate database type.
	if !validDbType(cfg.DbType) {
		str := "%s: The specified database type [%v] is invalid -- " +
			"supported types %v"
		err := fmt.Errorf(str, "loadConfig", cfg.DbType, knownDbTypes)
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// Append the network type to the data directory so it is "namespaced"
	// per network.  In addition to the block database, there are other
	// pieces of data that are saved to disk such as address manager state.
	// All data is specific to a network, so namespacing the data directory
	// means each individual piece of serialized data does not have to
	// worry about changing names per network and such.
	cfg.DataDir = filepath.Join(cfg.DataDir, netName(activeNetwork))

	// Ensure the specified block file exists.
	if !fileExists(cfg.InFile) {
		str := "%s: The specified block file [%v] does not exist"
		err := fmt.Errorf(str, "loadConfig", cfg.InFile)
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	return &cfg, remainingArgs, nil
}
