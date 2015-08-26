// Copyright (c) 2013 Conformal Systems LLC.
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/btcsuite/btclog"
	flags "github.com/btcsuite/go-flags"
	"github.com/davecgh/go-spew/spew"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ldb"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

// Hash is a block hash.
type Hash chainhash.Hash

type config struct {
	DataDir    string `short:"b" long:"datadir" description:"Directory to store data"`
	DbType     string `long:"dbtype" description:"Database backend"`
	TestNet    bool   `long:"testnet" description:"Use the test network"`
	SimNet     bool   `long:"simnet" description:"Use the simulation test network"`
	OutFile    string `short:"o" description:"outfile"`
	Progress   bool   `short:"p" description:"show progress"`
	ShaString  string `short:"s" description:"Block SHA to process" required:"true"`
	EShaString string `short:"e" description:"End Block SHA to process"`
	RawBlock   bool   `short:"r" description:"Raw Block"`
	FmtBlock   bool   `short:"f" description:"Format Block"`
	ShowTx     bool   `short:"t" description:"Show transaction"`
}

var (
	dcrdHomeDir     = dcrutil.AppDataDir("dcrd", false)
	defaultDataDir  = filepath.Join(dcrdHomeDir, "data")
	log             btclog.Logger
	activeNetParams = &chaincfg.MainNetParams
)

// Arguments to use for picking block, either sha or height.
const (
	ArgSha = iota
	ArgHeight
)

// netName returns the name used when referring to a bitcoin network.  At the
// time of writing, dcrd currently places blocks for testnet version 0 in the
// data and log directory "testnet", which does not match the Name field of the
// dcrnet parameters.  This function can be used to override this directory name
// as "testnet" when the passed active network matches wire.TestNet.
//
// A proper upgrade to move the data and log directories for this network to
// "testnet" is planned for the future, at which point this function can be
// removed and the network parameter's name used instead.
func netName(netParams *chaincfg.Params) string {
	switch netParams.Net {
	case wire.TestNet:
		return "testnet"
	default:
		return netParams.Name
	}
}

func main() {
	end := int64(-1)

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

	log.Infof("loading db %v", cfg.DbType)
	database, err := database.OpenDB(cfg.DbType, dbPath)
	if err != nil {
		log.Warnf("db open failed: %v", err)
		return
	}
	defer database.Close()
	log.Infof("db load complete")

	height, err := getHeight(database, cfg.ShaString)
	if err != nil {
		log.Infof("Invalid block %v", cfg.ShaString)
		return
	}
	if cfg.EShaString != "" {
		end, err = getHeight(database, cfg.EShaString)
		if err != nil {
			log.Infof("Invalid end block %v", cfg.EShaString)
			return
		}
	} else {
		end = height + 1
	}

	log.Infof("height %v end %v", height, end)

	var fo io.WriteCloser
	if cfg.OutFile != "" {
		fo, err = os.Create(cfg.OutFile)
		if err != nil {
			log.Warnf("failed to open file %v, err %v", cfg.OutFile, err)
		}
		defer func() {
			if err := fo.Close(); err != nil {
				log.Warn("failed to close file %v %v", cfg.OutFile, err)
			}
		}()
	}

	for ; height < end; height++ {
		if cfg.Progress && height%int64(1) == 0 {
			log.Infof("Processing block %v", height)
		}
		err = DumpBlock(database, height, fo, cfg.RawBlock, cfg.FmtBlock, cfg.ShowTx)
		if err != nil {
			break
		}
	}
	if cfg.Progress {
		height--
		log.Infof("Processing block %v", height)
	}
}

func getHeight(database database.Db, str string) (int64, error) {
	argtype, idx, sha, err := parsesha(str)
	if err != nil {
		log.Warnf("unable to decode [%v] %v", str, err)
		return 0, err
	}

	switch argtype {
	case ArgSha:
		// nothing to do
		blk, err := database.FetchBlockBySha(sha)
		if err != nil {
			log.Warnf("unable to locate block sha %v err %v",
				sha, err)
			return 0, err
		}
		idx = blk.Height()
	case ArgHeight:
	}
	return idx, nil
}

// DumpBlock dumps the specified block.
func DumpBlock(database database.Db, height int64, fo io.Writer, rflag bool, fflag bool, tflag bool) error {
	sha, err := database.FetchBlockShaByHeight(height)

	if err != nil {
		return err
	}
	blk, err := database.FetchBlockBySha(sha)
	if err != nil {
		log.Warnf("Failed to fetch block %v, err %v", sha, err)
		return err
	}
	rblk, err := blk.Bytes()
	blkid := blk.Height()

	if rflag {
		log.Infof("Block %v depth %v %v", sha, blkid, spew.Sdump(rblk))
	}

	mblk := blk.MsgBlock()
	if fflag {
		log.Infof("Block %v depth %v %v", sha, blkid, spew.Sdump(mblk))
	}
	if tflag {
		log.Infof("Num transactions %v", len(mblk.Transactions))
		for i, tx := range mblk.Transactions {
			txsha := tx.TxSha()
			log.Infof("tx %v: %v", i, &txsha)

		}
	}
	if fo != nil {
		// generate and write header values
		binary.Write(fo, binary.LittleEndian, uint32(wire.SimNet))
		binary.Write(fo, binary.LittleEndian, uint32(len(rblk)))

		// write block
		fo.Write(rblk)
	}
	return nil
}

var ntxcnt int64
var txspendcnt int64
var txgivecnt int64

// ErrBadShaPrefix is the error for an invalid prefix in a sha.
var ErrBadShaPrefix = errors.New("invalid prefix")

// ErrBadShaLen is the error for an invalid length sha.
var ErrBadShaLen = errors.New("invalid len")

// ErrBadShaChar is the error for an invalid character in a sha.
var ErrBadShaChar = errors.New("invalid character")

func parsesha(argstr string) (argtype int, height int64, psha *chainhash.Hash, err error) {
	var sha chainhash.Hash

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
