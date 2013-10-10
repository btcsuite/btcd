// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"encoding/binary"
	"fmt"
	"github.com/conformal/btcdb"
	_ "github.com/conformal/btcdb/ldb"
	_ "github.com/conformal/btcdb/sqlite3"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
	"github.com/conformal/go-flags"
	"github.com/conformal/seelog"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
)

type ShaHash btcwire.ShaHash

type config struct {
	DataDir  string `short:"b" long:"datadir" description:"Directory to store data"`
	DbType   string `long:"dbtype" description:"Database backend"`
	TestNet3 bool   `long:"testnet" description:"Use the test network"`
	Progress bool   `short:"p" description:"show progress"`
	InFile   string `short:"i" long:"infile" description:"File containing the block(s)" required:"true"`
}

var log seelog.LoggerInterface

const (
	ArgSha = iota
	ArgHeight
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
		DataDir: filepath.Join(btcdHomeDir(), "data"),
	}
	parser := flags.NewParser(&cfg, flags.Default)
	_, err := parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			parser.WriteHelp(os.Stderr)
		}
		return
	}

	runtime.GOMAXPROCS(runtime.NumCPU())

	log, err = seelog.LoggerFromWriterWithMinLevel(os.Stdout,
		seelog.InfoLvl)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v", err)
		return
	}
	defer log.Flush()
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

					if cfg.Progress && eheight%int64(1) == 0 {
						log.Infof("Processing block %v", eheight)
					}
					eheight++

					if eheight%2000 == 0 {
						f, err := os.Create(fmt.Sprintf("profile.%d", eheight))
						if err == nil {
							pprof.WriteHeapProfile(f)
							f.Close()
						} else {
							log.Warnf("profile failed %v", err)
						}
					}
				} else {
					break
				}
			}
		}
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

// newLogger creates a new seelog logger using the provided logging level and
// log message prefix.
func newLogger(level string, prefix string) seelog.LoggerInterface {
	fmtstring := `
        <seelog type="adaptive" mininterval="2000000" maxinterval="100000000"
                critmsgcount="500" minlevel="%s">
                <outputs formatid="all">
                        <console/>
                </outputs>
                <formats>
                        <format id="all" format="[%%Time %%Date] [%%LEV] [%s] %%Msg%%n" />
                </formats>
        </seelog>`
	config := fmt.Sprintf(fmtstring, level, prefix)

	logger, err := seelog.LoggerFromConfigAsString(config)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create logger: %v", err)
		os.Exit(1)
	}

	return logger
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
