// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"opensource.conformal.com/go/go-flags"
	"os"
	"path/filepath"
)

const (
	minCandidates        = 1
	maxCandidates        = 20
	defaultNumCandidates = 5
)

var (
	defaultDataDir = filepath.Join(btcdHomeDir(), "data")
)

// config defines the configuration options for findcheckpoint.
//
// See loadConfig for details on the configuration load process.
type config struct {
	DataDir       string `short:"b" long:"datadir" description:"Location of the btcd data directory"`
	NumCandidates int    `short:"n" long:"numcandidates" description:"Max num of checkpoint candidates to show {1-20}"`
	UseGoOutput   bool   `short:"g" long:"gooutput" description:"Display the candidates using Go syntax that is ready to insert into the btcchain checkpoint list"`
}

// btcdHomeDir returns an OS appropriate home directory for btcd.
func btcdHomeDir() string {
	// Search for Windows APPDATA first.  This won't exist on POSIX OSes.
	appData := os.Getenv("APPDATA")
	if appData != "" {
		return filepath.Join(appData, "btcd")
	}

	// Fall back to standard HOME directory that works for most POSIX OSes.
	home := os.Getenv("HOME")
	if home != "" {
		return filepath.Join(home, ".btcd")
	}

	// In the worst case, use the current directory.
	return "."
}

// loadConfig initializes and parses the config using command line options.
//
// The configuration proceeds as follows:
// 	1) Start with a default config with sane settings
// 	2) Pre-parse the command line to check for an alternative config file
// 	3) Load configuration file overwriting defaults with any specified options
// 	4) Parse CLI options and overwrite/add any specified options
//
// The above results in btcd functioning properly without any config settings
// while still allowing the user to override settings with config files and
// command line options.  Command line options always take precedence.
func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		DataDir:       defaultDataDir,
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

	// Validate the number of candidates.
	if cfg.NumCandidates < minCandidates || cfg.NumCandidates > maxCandidates {
		str := "%s: The specified number of candidates is out of " +
			"range -- parsed [%v]"
		err = fmt.Errorf(str, "loadConfig", cfg.NumCandidates)
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
	//cfg.DataDir = cleanAndExpandPath(cfg.DataDir)
	cfg.DataDir = filepath.Join(cfg.DataDir, "mainnet")

	return &cfg, remainingArgs, nil
}
