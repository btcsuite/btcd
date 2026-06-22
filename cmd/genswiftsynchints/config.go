package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/jessevdk/go-flags"
)

var (
	btcdHomeDir     = btcutil.AppDataDir("btcd", false)
	defaultDataDir  = filepath.Join(btcdHomeDir, "data")
	activeNetParams = &chaincfg.MainNetParams
)

// config defines the configuration options for genswiftsynchints.
//
// See loadConfig for details on the configuration load process.
type config struct {
	DataDir        string `short:"b" long:"datadir" description:"Location of the btcd data directory"`
	GenBlockHash   string `long:"genblockhash" description:"The block hash of the block where the hintsfile should end at. The txos from block with the following hash are included. A height below a previous run regenerates the bitmap and hintsfile for that lower height and reuses the existing outpoint mapping."`
	CPUProfile     string `long:"cpuprofile" description:"Write CPU profile to the specified file"`
	MemProfile     string `long:"memprofile" description:"Write memory profile to the specified file"`
	DeleteHints    bool   `long:"deletehints" description:"Delete the generated hintsfile, then exit"`
	DeleteMapping  bool   `long:"deletemapping" description:"Delete the outpoint mapping database and its metadata, then exit"`
	RegressionTest bool   `long:"regtest" description:"Use the regression test network"`
	SimNet         bool   `long:"simnet" description:"Use the simulation test network"`
	SigNet         bool   `long:"signet" description:"Use the signet test network"`
	TestNet3       bool   `long:"testnet" description:"Use the test network (version 3)"`
	TestNet4       bool   `long:"testnet4" description:"Use the test network (version 4)"`
}

// netName returns the name used when referring to a bitcoin network.  btcd
// places blocks for testnet version 3 in the data and log directory "testnet",
// which does not match the "testnet3" Name field of the chaincfg parameters.
// This function maps wire.TestNet3 back to "testnet" so the directory name
// stays "testnet" for testnet3.
func netName(chainParams *chaincfg.Params) string {
	switch chainParams.Net {
	case wire.TestNet3:
		return "testnet"
	default:
		return chainParams.Name
	}
}

// loadConfig initializes and parses the config using command line options.
func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		DataDir: defaultDataDir,
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
	if cfg.TestNet3 {
		numNets++
		activeNetParams = &chaincfg.TestNet3Params
	}
	if cfg.TestNet4 {
		numNets++
		activeNetParams = &chaincfg.TestNet4Params
	}
	if cfg.RegressionTest {
		numNets++
		activeNetParams = &chaincfg.RegressionNetParams
	}
	if cfg.SimNet {
		numNets++
		activeNetParams = &chaincfg.SimNetParams
	}
	if cfg.SigNet {
		numNets++
		activeNetParams = &chaincfg.SigNetParams
	}
	if numNets > 1 {
		str := "%s: The testnet, regtest, signet and simnet params can't be " +
			"used together -- choose one of the three"
		err := fmt.Errorf(str, funcName)
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return nil, nil, err
	}

	// Append the network type to the data directory so it is namespaced
	// per network.  In addition to the block database, there are other
	// pieces of data that are saved to disk such as address manager state.
	// All data is specific to a network, so namespacing the data directory
	// means each individual piece of serialized data does not have to
	// worry about changing names per network and such.
	cfg.DataDir = filepath.Join(cfg.DataDir, netName(activeNetParams))

	return &cfg, remainingArgs, nil
}
