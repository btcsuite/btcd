// Copyright (c) 2013 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/btcsuite/btclog"
	flags "github.com/btcsuite/go-flags"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ldb"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

type config struct {
	DataDir   string `short:"b" long:"datadir" description:"Directory to store data"`
	DbType    string `long:"dbtype" description:"Database backend"`
	TestNet   bool   `long:"testnet" description:"Use the test network"`
	SimNet    bool   `long:"simnet" description:"Use the simulation test network"`
	ShaString string `short:"s" description:"Block SHA to process" required:"true"`
}

var (
	dcrdHomeDir     = dcrutil.AppDataDir("dcrd", false)
	defaultDataDir  = filepath.Join(dcrdHomeDir, "data")
	log             btclog.Logger
	activeNetParams = &chaincfg.MainNetParams
)

const (
	argSha = iota
	argHeight
)

// netName returns the name used when referring to a decred network.  At the
// time of writing, dcrd currently places blocks for testnet version 0 in the
// data and log directory "testnet", which does not match the Name field of the
// chaincfg parameters.  This function can be used to override this directory name
// as "testnet" when the passed active network matches wire.TestNet.
//
// A proper upgrade to move the data and log directories for this network to
// "testnet" is planned for the future, at which point this function can be
// removed and the network parameter's name used instead.
func netName(chainParams *chaincfg.Params) string {
	switch chainParams.Net {
	case wire.TestNet:
		return "testnet"
	default:
		return chainParams.Name
	}
}

func main() {
	cfg := config{
		DbType:  "leveldb",
		DataDir: defaultDataDir,
	}
	parser := flags.NewParser(&cfg, flags.Default)
	_, err := parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			parser.WriteHelp(os.Stderr)
		}
		return
	}

	backendLogger := btclog.NewDefaultBackendLogger()
	defer backendLogger.Flush()
	log = btclog.NewSubsystemLogger(backendLogger, "")
	database.UseLogger(log)

	// Multiple networks can't be selected simultaneously.
	funcName := "main"
	numNets := 0
	// Count number of network flags passed; assign active network params
	// while we're at it
	if cfg.TestNet {
		numNets++
		activeNetParams = &chaincfg.TestNetParams
	}
	if cfg.SimNet {
		numNets++
		activeNetParams = &chaincfg.SimNetParams
	}
	if numNets > 1 {
		str := "%s: The testnet, regtest, and simnet params can't be " +
			"used together -- choose one of the three"
		err := fmt.Errorf(str, funcName)
		fmt.Fprintln(os.Stderr, err)
		parser.WriteHelp(os.Stderr)
		return
	}
	cfg.DataDir = filepath.Join(cfg.DataDir, netName(activeNetParams))

	blockDbNamePrefix := "blocks"
	dbName := blockDbNamePrefix + "_" + cfg.DbType
	if cfg.DbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(cfg.DataDir, dbName)

	log.Infof("loading db")
	db, err := database.OpenDB(cfg.DbType, dbPath)
	if err != nil {
		log.Warnf("db open failed: %v", err)
		return
	}
	defer db.Close()
	log.Infof("db load complete")

	_, height, err := db.NewestSha()
	log.Infof("loaded block height %v", height)

	sha, err := getSha(db, cfg.ShaString)
	if err != nil {
		log.Infof("Invalid block hash %v", cfg.ShaString)
		return
	}

	err = db.DropAfterBlockBySha(&sha)
	if err != nil {
		log.Warnf("failed %v", err)
	}

}

func getSha(db database.Db, str string) (chainhash.Hash, error) {
	argtype, idx, sha, err := parsesha(str)
	if err != nil {
		log.Warnf("unable to decode [%v] %v", str, err)
		return chainhash.Hash{}, err
	}

	switch argtype {
	case argSha:
		// nothing to do
	case argHeight:
		sha, err = db.FetchBlockShaByHeight(idx)
		if err != nil {
			return chainhash.Hash{}, err
		}
	}
	if sha == nil {
		fmt.Printf("wtf sha is nil but err is %v", err)
	}
	return *sha, nil
}

var ntxcnt int64
var txspendcnt int64
var txgivecnt int64

var errBadShaPrefix = errors.New("invalid prefix")
var errBadShaLen = errors.New("invalid len")
var errBadShaChar = errors.New("invalid character")

func parsesha(argstr string) (argtype int, height int64, psha *chainhash.Hash, err error) {
	var sha chainhash.Hash

	var hashbuf string

	switch len(argstr) {
	case 64:
		hashbuf = argstr
	case 66:
		if argstr[0:2] != "0x" {
			log.Infof("prefix is %v", argstr[0:2])
			err = errBadShaPrefix
			return
		}
		hashbuf = argstr[2:]
	default:
		if len(argstr) <= 16 {
			// assume value is height
			argtype = argHeight
			var h int
			h, err = strconv.Atoi(argstr)
			if err == nil {
				height = int64(h)
				return
			}
			log.Infof("Unable to parse height %v, err %v", height, err)
		}
		err = errBadShaLen
		return
	}

	var buf [32]byte
	for idx, ch := range hashbuf {
		var val rune

		switch {
		case ch >= '0' && ch <= '9':
			val = ch - '0'
		case ch >= 'a' && ch <= 'f':
			val = ch - 'a' + rune(10)
		case ch >= 'A' && ch <= 'F':
			val = ch - 'A' + rune(10)
		default:
			err = errBadShaChar
			return
		}
		b := buf[31-idx/2]
		if idx&1 == 1 {
			b |= byte(val)
		} else {
			b |= (byte(val) << 4)
		}
		buf[31-idx/2] = b
	}
	sha.SetBytes(buf[0:32])
	psha = &sha
	return
}
