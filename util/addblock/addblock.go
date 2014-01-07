// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"fmt"
	"github.com/conformal/btcd/limits"
	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcdb/ldb"
	"github.com/conformal/btclog"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/go-flags"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

type ShaHash btcwire.ShaHash

type config struct {
	DataDir  string `short:"b" long:"datadir" description:"Directory to store data"`
	DbType   string `long:"dbtype" description:"Database backend"`
	TestNet3 bool   `long:"testnet" description:"Use the test network"`
	Progress bool   `short:"p" description:"show progress"`
	InFile   string `short:"i" long:"infile" description:"File containing the block(s)" required:"true"`
}

const (
	ArgSha = iota
	ArgHeight
)

var (
	btcdHomeDir    = btcutil.AppDataDir("btcd", false)
	defaultDataDir = filepath.Join(btcdHomeDir, "data")
	log            btclog.Logger
)

type bufQueue struct {
	height int64
	blkbuf []byte
}

type blkQueue struct {
	complete chan bool
	height   int64
	blk      *btcutil.Block
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

	// Use all processor cores.
	runtime.GOMAXPROCS(runtime.NumCPU())

	// Up some limits.
	if err := limits.SetLimits(); err != nil {
		os.Exit(1)
	}

	backendLogger := btclog.NewDefaultBackendLogger()
	defer backendLogger.Flush()
	log = btclog.NewSubsystemLogger(backendLogger, "")
	btcdb.UseLogger(log)

	var testnet string
	if cfg.TestNet3 {
		testnet = "testnet"
	} else {
		testnet = "mainnet"
	}

	cfg.DataDir = filepath.Join(cfg.DataDir, testnet)

	err = os.MkdirAll(cfg.DataDir, 0700)
	if err != nil {
		fmt.Printf("unable to create db repo area %v, %v", cfg.DataDir, err)
	}

	blockDbNamePrefix := "blocks"
	dbName := blockDbNamePrefix + "_" + cfg.DbType
	if cfg.DbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(cfg.DataDir, dbName)

	log.Infof("loading db")
	db, err := btcdb.CreateDB(cfg.DbType, dbPath)
	if err != nil {
		log.Warnf("db open failed: %v", err)
		return
	}
	defer db.Close()
	log.Infof("db created")

	var fi io.ReadCloser

	fi, err = os.Open(cfg.InFile)
	if err != nil {
		log.Warnf("failed to open file %v, err %v", cfg.InFile, err)
	}
	defer func() {
		if err := fi.Close(); err != nil {
			log.Warn("failed to close file %v %v", cfg.InFile, err)
		}
	}()

	bufqueue := make(chan *bufQueue, 2)
	blkqueue := make(chan *blkQueue, 2)

	for i := 0; i < runtime.NumCPU(); i++ {
		go processBuf(i, bufqueue, blkqueue)
	}
	go processBuf(0, bufqueue, blkqueue)

	go readBlocks(fi, bufqueue)

	var eheight int64
	doneMap := map[int64]*blkQueue{}
	for {

		select {
		case blkM := <-blkqueue:
			doneMap[blkM.height] = blkM

			for {
				if blkP, ok := doneMap[eheight]; ok {
					delete(doneMap, eheight)
					blkP.complete <- true
					db.InsertBlock(blkP.blk)

					if cfg.Progress && eheight%int64(10000) == 0 {
						log.Infof("Processing block %v", eheight)
					}
					eheight++
				} else {
					break
				}
			}
		}
	}
	if cfg.Progress {
		log.Infof("Processing block %v", eheight)
	}
}

func processBuf(idx int, bufqueue chan *bufQueue, blkqueue chan *blkQueue) {
	complete := make(chan bool)
	for {
		select {
		case bq := <-bufqueue:
			var blkmsg blkQueue

			blkmsg.height = bq.height

			if len(bq.blkbuf) == 0 {
				// we are done
				blkqueue <- &blkmsg
			}

			blk, err := btcutil.NewBlockFromBytes(bq.blkbuf)
			if err != nil {
				fmt.Printf("failed to parse block %v", bq.height)
				return
			}
			blkmsg.blk = blk
			blkmsg.complete = complete
			blkqueue <- &blkmsg
			select {
			case <-complete:
			}
		}
	}
}

func readBlocks(fi io.Reader, bufqueue chan *bufQueue) {
	var height int64
	for {
		var net, blen uint32

		var bufM bufQueue
		bufM.height = height

		// generate and write header values
		err := binary.Read(fi, binary.LittleEndian, &net)
		if err != nil {
			break
			bufqueue <- &bufM
		}
		if net != uint32(btcwire.MainNet) {
			fmt.Printf("network mismatch %v %v",
				net, uint32(btcwire.MainNet))

			bufqueue <- &bufM
		}
		err = binary.Read(fi, binary.LittleEndian, &blen)
		if err != nil {
			bufqueue <- &bufM
		}
		blkbuf := make([]byte, blen)
		err = binary.Read(fi, binary.LittleEndian, blkbuf)
		bufM.blkbuf = blkbuf
		bufqueue <- &bufM
		height++
	}
}
