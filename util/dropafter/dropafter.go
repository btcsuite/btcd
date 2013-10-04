// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"errors"
	"flag"
	"fmt"
	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcdb/ldb"
	"github.com/conformal/btcwire"
	"github.com/conformal/seelog"
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
	var shastring string
	flag.StringVar(&dbType, "dbtype", "", "Database backend to use for the Block Chain")
	flag.StringVar(&datadir, "datadir", "", "Directory to store data")
	flag.StringVar(&shastring, "s", "", "Block sha to process")

	flag.Parse()

	log, err = seelog.LoggerFromWriterWithMinLevel(os.Stdout,
		seelog.TraceLvl)
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

	log.Infof("loading db")
	db, err := btcdb.OpenDB(dbType, dbPath)
	if err != nil {
		log.Warnf("db open failed: %v", err)
		return
	}
	defer db.Close()
	log.Infof("db load complete")

	_, height, err := db.NewestSha()
	log.Infof("loaded block height %v", height)

	sha, err := getSha(db, shastring)
	if err != nil {
		log.Infof("Invalid block %v", shastring)
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
