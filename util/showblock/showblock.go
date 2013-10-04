// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"errors"
	"flag"
	"fmt"
	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcdb/ldb"
	_ "github.com/conformal/btcdb/sqlite3"
	"github.com/conformal/btcwire"
	"github.com/conformal/seelog"
	"github.com/davecgh/go-spew/spew"
	"io"
	"os"
	"path/filepath"
	"strconv"
)

type ShaHash btcwire.ShaHash

var log seelog.LoggerInterface

const (
	ArgSha = iota
	ArgHeight
)

func main() {
	var err error
	var dbType string
	var datadir string
	var shastring, eshastring, outfile string
	var rflag, fflag, tflag bool
	var progress int
	end := int64(-1)
	flag.StringVar(&dbType, "dbtype", "", "Database backend to use for the Block Chain")
	flag.StringVar(&datadir, "datadir", ".", "Directory to store data")

	flag.StringVar(&shastring, "s", "", "Block sha to process")
	flag.StringVar(&eshastring, "e", "", "Block sha to process")
	flag.StringVar(&outfile, "o", "", "outfile")
	flag.BoolVar(&rflag, "r", false, "raw block")
	flag.BoolVar(&fflag, "f", false, "fmt block")
	flag.BoolVar(&tflag, "t", false, "show transactions")
	flag.IntVar(&progress, "p", 0, "show progress")

	flag.Parse()

	log, err = seelog.LoggerFromWriterWithMinLevel(os.Stdout,
		seelog.InfoLvl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v", err)
		return
	}
	defer log.Flush()
	btcdb.UseLogger(log)

	if len(dbType) == 0 {
		dbType = "sqlite"
	}

	if len(datadir) == 0 {
		datadir = filepath.Join(btcdHomeDir(), "data")
	}
	datadir = filepath.Join(datadir, "mainnet")

	blockDbNamePrefix := "blocks"
	dbName := blockDbNamePrefix + "_" + dbType
	if dbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(datadir, dbName)

	log.Infof("loading db %v", dbType)
	db, err := btcdb.OpenDB(dbType, dbPath)
	if err != nil {
		log.Warnf("db open failed: %v", err)
		return
	}
	defer db.Close()
	log.Infof("db load complete")

	height, err := getHeight(db, shastring)
	if err != nil {
		log.Infof("Invalid block %v", shastring)
		return
	}
	if eshastring != "" {
		end, err = getHeight(db, eshastring)
		if err != nil {
			log.Infof("Invalid end block %v", eshastring)
			return
		}
	} else {
		end = height + 1
	}

	log.Infof("height %v end %v", height, end)

	var fo io.WriteCloser
	if outfile != "" {
		fo, err = os.Create(outfile)
		if err != nil {
			log.Warnf("failed to open file %v, err %v", outfile, err)
		}
		defer func() {
			if err := fo.Close(); err != nil {
				log.Warn("failed to close file %v %v", outfile, err)
			}
		}()
	}

	for ; height < end; height++ {
		if progress != 0 && height%int64(progress) == 0 {
			log.Infof("Processing block %v", height)
		}
		err = DumpBlock(db, height, fo, rflag, fflag, tflag)
		if err != nil {
			break
		}
	}
	if progress != 0 {
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
