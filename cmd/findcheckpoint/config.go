// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ffldb"
	"github.com/decred/dcrd/dcrutil"
	flags "github.com/jessevdk/go-flags"
)

const (
	minCandidates        = 1
	maxCandidates        = 20
	defaultNumCandidates = 5
	defaultDbType        = "ffldb"
)

var (
	dcrdHomeDir     = dcrutil.AppDataDir("dcrd", false)
	defaultDataDir  = filepath.Join(dcrdHomeDir, "data")
	knownDbTypes    = database.SupportedDrivers()
	activeNetParams = &chaincfg.MainNetParams
)

// config defines the configuration options for findcheckpoint.
//
// See loadConfig for details on the configuration load process.
type config struct {
	DataDir       string `short:"b" long:"datadir" description:"Location of the dcrd data directory"`
	DbType        string `long:"dbtype" description:"Database backend to use for the Block Chain"`
	TestNet       bool   `long:"testnet" description:"Use the test network"`
	SimNet        bool   `long:"simnet" description:"Use the simulation test network"`
	NumCandidates int    `short:"n" long:"numcandidates" description:"Max num of checkpoint candidates to show {1-20}"`
	UseGoOutput   bool   `short:"g" long:"gooutput" description:"Display the candidates using Go syntax that is ready to insert into the dcrchain checkpoint list"`
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

// loadConfig initializes and parses the config using command line options.
func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		DataDir:       defaultDataDir,
		DbType:        defaultDbType,
		NumCandidates: defaultNumCandidates,
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

	// Multiple networks can't be selected simultaneously.
	funcName := "loadConfig"
	numNets := 0
	// Count number of network flags passed; assign active network params
	// while we're at it
	if cfg.TestNet {
		numNets++
		activeNetParams = &chaincfg.TestNet3Params
	}
	if cfg.SimNet {
		numNets++
		activeNetParams = &chaincfg.SimNetParams
	}
	if numNets > 1 {
		str := "%s: the testnet, regtest, and simnet params can't be " +
			"used together -- choose one of the three"
		err := fmt.Errorf(str, funcName)
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// Validate database type.
	if !validDbType(cfg.DbType) {
		str := "%s: the specified database type [%v] is invalid -- " +
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
	cfg.DataDir = filepath.Join(cfg.DataDir, activeNetParams.Name)

	// Validate the number of candidates.
	if cfg.NumCandidates < minCandidates || cfg.NumCandidates > maxCandidates {
		str := "%s: The specified number of candidates is out of " +
			"range -- parsed [%v]"
		err = fmt.Errorf(str, "loadConfig", cfg.NumCandidates)
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	return &cfg, remainingArgs, nil
}
