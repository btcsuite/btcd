/*
 * Copyright (c) 2013 Conformal Systems LLC.
 */

package main

import (
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/conformal/btcdb"
	"github.com/conformal/btcutil"
	_ "github.com/conformal/btcdb/sqlite3"
	_ "github.com/conformal/btcdb/ldb"
	"github.com/conformal/btcwire"
	"github.com/conformal/seelog"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/pprof"
)

type ShaHash btcwire.ShaHash

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
	height int64
	blk *btcutil.Block
}

func main() {
	var err error
	var dbType string
	var datadir string
	var infile string
	var progress int
	flag.StringVar(&dbType, "dbtype", "", "Database backend to use for the Block Chain")
	flag.StringVar(&datadir, "datadir", "", "Directory to store data")
	flag.StringVar(&infile, "i", "", "infile")
	flag.IntVar(&progress, "p", 0, "show progress")

	flag.Parse()

	runtime.GOMAXPROCS(runtime.NumCPU())

	if len (infile) == 0 {
		fmt.Printf("Must specify inputfile")
		return
	}

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

	err = os.MkdirAll(datadir, 0700)
	if err != nil {
		fmt.Printf("unable to create db repo area %v, %v", datadir, err)
	}


	blockDbNamePrefix := "blocks"
        dbName := blockDbNamePrefix + "_" + dbType
	if dbType == "sqlite" {
		dbName = dbName + ".db"
	}
	dbPath := filepath.Join(datadir, dbName)

	log.Infof("loading db")
	db, err := btcdb.CreateDB(dbType, dbPath)
	if err != nil {
		log.Warnf("db open failed: %v", err)
		return
	}
	defer db.Close()
	log.Infof("db created")


	var fi io.ReadCloser

	fi, err = os.Open(infile)
	if err != nil {
		log.Warnf("failed to open file %v, err %v", infile, err)
	}
	defer func() {
		if err := fi.Close(); err != nil {
			log.Warn("failed to close file %v %v", infile, err)
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
	doneMap := map [int64] *blkQueue {}
	for {

		select {
		case blkM := <- blkqueue:
			doneMap[blkM.height] = blkM

			for {
				if blkP, ok := doneMap[eheight]; ok {
					delete(doneMap, eheight)
					blkP.complete <- true
					db.InsertBlock(blkP.blk)

					if progress != 0 && eheight%int64(progress) == 0 {
						log.Infof("Processing block %v", eheight)
					}
					eheight++

					if eheight % 2000 == 0 {
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
	complete := make (chan bool)
	for {
		select {
		case bq := <- bufqueue:
			var blkmsg blkQueue

			blkmsg.height = bq.height

			if len(bq.blkbuf) == 0 {
				// we are done
				blkqueue <- &blkmsg
			}

			blk, err :=  btcutil.NewBlockFromBytes(bq.blkbuf)
			if err != nil {
				fmt.Printf("failed to parse block %v", bq.height)
				return
			}
			blkmsg.blk = blk
			blkmsg.complete = complete
			blkqueue <- &blkmsg
			select {
			case <- complete:
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
