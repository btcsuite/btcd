// Copyright (c) 2013 Conformal Systems LLC.
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

	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcdb/ldb"
	"github.com/conformal/btclog"
	"github.com/conformal/btcnet"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	flags "github.com/conformal/go-flags"
	"github.com/davecgh/go-spew/spew"
)

type ShaHash btcwire.ShaHash

type config struct {
	DataDir        string `short:"b" long:"datadir" description:"Directory to store data"`
	DbType         string `long:"dbtype" description:"Database backend"`
	TestNet3       bool   `long:"testnet" description:"Use the test network"`
	RegressionTest bool   `long:"regtest" description:"Use the regression test network"`
	SimNet         bool   `long:"simnet" description:"Use the simulation test network"`
	OutFile        string `short:"o" description:"outfile"`
	Progress       bool   `short:"p" description:"show progress"`
	ShaString      string `short:"s" description:"Block SHA to process" required:"true"`
	EShaString     string `short:"e" description:"End Block SHA to process"`
	RawBlock       bool   `short:"r" description:"Raw Block"`
	FmtBlock       bool   `short:"f" description:"Format Block"`
	ShowTx         bool   `short:"t" description:"Show transaction"`
}

var (
	btcdHomeDir     = btcutil.AppDataDir("btcd", false)
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

	log.Infof("loading db %v", cfg.DbType)
	db, err := btcdb.OpenDB(cfg.DbType, dbPath)
	if err != nil {
		log.Warnf("db open failed: %v", err)
		return
	}
	defer db.Close()
	log.Infof("db load complete")

	height, err := getHeight(db, cfg.ShaString)
	if err != nil {
		log.Infof("Invalid block %v", cfg.ShaString)
		return
	}
	if cfg.EShaString != "" {
		end, err = getHeight(db, cfg.EShaString)
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
		err = DumpBlock(db, height, fo, cfg.RawBlock, cfg.FmtBlock, cfg.ShowTx)
		if err != nil {
			break
		}
	}
	if cfg.Progress {
		height--
		log.Infof("Processing block %v", height)
	}
}

func getHeight(db btcdb.Db, str string) (int64, error) {
	argtype, idx, sha, err := parsesha(str)
	if err != nil {
		log.Warnf("unable to decode [%v] %v", str, err)
		return 0, err
	}

	switch argtype {
	case ArgSha:
		// nothing to do
		blk, err := db.FetchBlockBySha(sha)
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

func DumpBlock(db btcdb.Db, height int64, fo io.Writer, rflag bool, fflag bool, tflag bool) error {
	sha, err := db.FetchBlockShaByHeight(height)

	if err != nil {
		return err
	}
	blk, err := db.FetchBlockBySha(sha)
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
			txsha, err := tx.TxSha()
			if err != nil {
				continue
			}
			log.Infof("tx %v: %v", i, &txsha)

		}
	}
	if fo != nil {
		// generate and write header values
		binary.Write(fo, binary.LittleEndian, uint32(btcwire.MainNet))
		binary.Write(fo, binary.LittleEndian, uint32(len(rblk)))

		// write block
		fo.Write(rblk)
	}
	return nil
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
