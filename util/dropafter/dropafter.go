// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/conformal/btclog"
	flags "github.com/conformal/go-flags"
	"github.com/mably/btcdb"
	_ "github.com/mably/btcdb/ldb"
	"github.com/mably/btcnet"
	"github.com/mably/btcutil"
	"github.com/mably/btcwire"
)

type ShaHash btcwire.ShaHash

type config struct {
	DataDir        string `short:"b" long:"datadir" description:"Directory to store data"`
	DbType         string `long:"dbtype" description:"Database backend"`
	TestNet3       bool   `long:"testnet" description:"Use the test network"`
	RegressionTest bool   `long:"regtest" description:"Use the regression test network"`
	SimNet         bool   `long:"simnet" description:"Use the simulation test network"`
	ShaString      string `short:"s" description:"Block SHA to process" required:"true"`
}

var (
	btcdHomeDir     = btcutil.AppDataDir("ppcd", false)
	defaultDataDir  = filepath.Join(btcdHomeDir, "data")
	log             btclog.Logger
	activeNetParams = &btcnet.MainNetParams
)

const (
	ArgSha = iota
	ArgHeight
)

// netName returns the name used when referring to a bitcoin network.  At the
// time of writing, btcd currently places blocks for testnet version 3 in the
// data and log directory "testnet", which does not match the Name field of the
// btcnet parameters.  This function can be used to override this directory name
// as "testnet" when the passed active network matches btcwire.TestNet3.
//
// A proper upgrade to move the data and log directories for this network to
// "testnet3" is planned for the future, at which point this function can be
// removed and the network parameter's name used instead.
func netName(netParams *btcnet.Params) string {
	switch netParams.Net {
	case btcwire.TestNet3:
		return "testnet"
	default:
		return netParams.Name
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
	btcdb.UseLogger(log)

	// Multiple networks can't be selected simultaneously.
	funcName := "main"
	numNets := 0
	// Count number of network flags passed; assign active network params
	// while we're at it
	if cfg.TestNet3 {
		numNets++
		activeNetParams = &btcnet.TestNet3Params
	}
	if cfg.RegressionTest {
		numNets++
		activeNetParams = &btcnet.RegressionNetParams
	}
	if cfg.SimNet {
		numNets++
		activeNetParams = &btcnet.SimNetParams
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
	db, err := btcdb.OpenDB(cfg.DbType, dbPath)
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

func getSha(db btcdb.Db, str string) (btcwire.ShaHash, error) {
	argtype, idx, sha, err := parsesha(str)
	if err != nil {
		log.Warnf("unable to decode [%v] %v", str, err)
		return btcwire.ShaHash{}, err
	}

	switch argtype {
	case ArgSha:
		// nothing to do
	case ArgHeight:
		sha, err = db.FetchBlockShaByHeight(idx)
		if err != nil {
			return btcwire.ShaHash{}, err
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

var ErrBadShaPrefix = errors.New("invalid prefix")
var ErrBadShaLen = errors.New("invalid len")
var ErrBadShaChar = errors.New("invalid character")

func parsesha(argstr string) (argtype int, height int64, psha *btcwire.ShaHash, err error) {
	var sha btcwire.ShaHash

	var hashbuf string

	switch len(argstr) {
	case 64:
		hashbuf = argstr
	case 66:
		if argstr[0:2] != "0x" {
			log.Infof("prefix is %v", argstr[0:2])
			err = ErrBadShaPrefix
			return
		}
		hashbuf = argstr[2:]
	default:
		if len(argstr) <= 16 {
			// assume value is height
			argtype = ArgHeight
			var h int
			h, err = strconv.Atoi(argstr)
			if err == nil {
				height = int64(h)
				return
			}
			log.Infof("Unable to parse height %v, err %v", height, err)
		}
		err = ErrBadShaLen
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
			err = ErrBadShaChar
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
