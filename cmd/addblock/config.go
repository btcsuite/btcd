// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
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
	"github.com/decred/dcrd/wire"
	flags "github.com/jessevdk/go-flags"
)

const (
	defaultDbType   = "ffldb"
	defaultDataFile = "bootstrap.dat"
	defaultProgress = 10
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
	DataDir           string `short:"b" long:"datadir" description:"Location of the dcrd data directory"`
	DbType            string `long:"dbtype" description:"Database backend to use for the Block Chain"`
	TestNet           bool   `long:"testnet" description:"Use the test network"`
	SimNet            bool   `long:"simnet" description:"Use the simulation test network"`
	InFile            string `short:"i" long:"infile" description:"File containing the block(s)"`
	NoExistsAddrIndex bool   `long:"noexistsaddrindex" description:"Do not build a full index of which addresses were ever seen on the blockchain"`
	TxIndex           bool   `long:"txindex" description:"Build a full hash-based transaction index which makes all transactions available via the getrawtransaction RPC"`
	AddrIndex         bool   `long:"addrindex" description:"Build a full address-based transaction index which makes the searchrawtransactions RPC available"`
	Progress          int    `short:"p" long:"progress" description:"Show a progress message each time this number of seconds have passed -- Use 0 to disable progress announcements"`
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

// netName returns the name used when referring to a bitcoin network.  At the
// time of writing, dcrd currently places blocks for testnet version 2 in the
// data and log directory "testnet2", which does not match the Name field of the
// chaincfg parameters.  This function can be used to override this directory name
// as "testnet2" when the passed active network matches wire.TestNet2.
//
// A proper upgrade to move the data and log directories for this network to
// "testnet2" is planned for the future, at which point this function can be
// removed and the network parameter's name used instead.
func netName(chainParams *chaincfg.Params) string {
	switch chainParams.Net {
	case wire.TestNet2:
		return "testnet2"
	default:
		return chainParams.Name
	}
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

	// Multiple networks can't be selected simultaneously.
	funcName := "loadConfig"
	numNets := 0
	// Count number of network flags passed; assign active network params
	// while we're at it
	if cfg.TestNet {
		numNets++
		activeNetParams = &chaincfg.TestNet2Params
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
	cfg.DataDir = filepath.Join(cfg.DataDir, netName(activeNetParams))

	// Ensure the specified block file exists.
	if !fileExists(cfg.InFile) {
		str := "%s: the specified block file [%v] does not exist"
		err := fmt.Errorf(str, "loadConfig", cfg.InFile)
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	return &cfg, remainingArgs, nil
}
