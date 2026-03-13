package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/wire"
	"github.com/jessevdk/go-flags"
)

const (
	defaultDbType = "ffldb"
)

var (
	btcdHomeDir     = btcutil.AppDataDir("btcd", false)
	defaultDataDir  = filepath.Join(btcdHomeDir, "data")
	knownDbTypes    = database.SupportedDrivers()
	activeNetParams = &chaincfg.MainNetParams
)

// config defines the configuration options for findcheckpoint.
//
// See loadConfig for details on the configuration load process.
type config struct {
	DataDir        string `short:"b" long:"datadir" description:"Location of the btcd data directory"`
	DbType         string `long:"dbtype" description:"Database backend to use for the Block Chain"`
	GenBlockHash   string `long:"genblockhash" description:"The block hash of the block where the bitmap should end at. The txos from block with the following hash are included."`
	CPUProfile     string `long:"cpuprofile" description:"Write CPU profile to the specified file"`
	MemProfile     string `long:"memprofile" description:"Write memory profile to the specified file"`
	DeleteBitmap   bool   `long:"deletebitmap" description:"Delete the bitmap file and its progress metadata, then exit"`
	DeleteMapping  bool   `long:"deletemapping" description:"Delete the outpoint mapping database and its metadata, then exit"`
	RegressionTest bool   `long:"regtest" description:"Use the regression test network"`
	SimNet         bool   `long:"simnet" description:"Use the simulation test network"`
	SigNet         bool   `long:"signet" description:"Use the signet test network"`
	TestNet3       bool   `long:"testnet" description:"Use the test network (version 3)"`
	TestNet4       bool   `long:"testnet4" description:"Use the test network (version 4)"`
}

// validDbType returns whether or not dbType is a supported database type.
func validDbType(dbType string) bool {
	return slices.Contains(knownDbTypes, dbType)
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

// loadConfig initializes and parses the config using command line options.
func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		DataDir: defaultDataDir,
		DbType:  defaultDbType,
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

	fmt.Println("knowndb", knownDbTypes)

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
	cfg.DataDir = filepath.Join(cfg.DataDir, netName(activeNetParams))

	return &cfg, remainingArgs, nil
}
