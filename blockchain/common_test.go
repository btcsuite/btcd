// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"compress/bzip2"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ldb"
	_ "github.com/decred/dcrd/database/memdb"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

// testDbType is the database backend type to use for the tests.
const testDbType = "leveldb"

// testDbRoot is the root directory used to create all test databases.
const testDbRoot = "testdbs"

// filesExists returns whether or not the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// isSupportedDbType returns whether or not the passed database type is
// currently supported.
func isSupportedDbType(dbType string) bool {
	supportedDBs := database.SupportedDBs()
	for _, sDbType := range supportedDBs {
		if dbType == sDbType {
			return true
		}
	}

	return false
}

// chainSetup is used to create a new db and chain instance with the genesis
// block already inserted.  In addition to the new chain instnce, it returns
// a teardown function the caller should invoke when done testing to clean up.
func chainSetup(dbName string, params *chaincfg.Params) (*blockchain.BlockChain, func(), error) {
	if !isSupportedDbType(testDbType) {
		return nil, nil, fmt.Errorf("unsupported db type %v", testDbType)
	}

	// Handle memory database specially since it doesn't need the disk
	// specific handling.
	var db database.Db
	tmdb := new(stake.TicketDB)

	var teardown func()
	if testDbType == "memdb" {
		ndb, err := database.CreateDB(testDbType)
		if err != nil {
			return nil, nil, fmt.Errorf("error creating db: %v", err)
		}
		db = ndb

		// Setup a teardown function for cleaning up.  This function is
		// returned to the caller to be invoked when it is done testing.
		teardown = func() {
			tmdb.Close()
			db.Close()
		}
	} else {
		// Create the root directory for test databases.
		if !fileExists(testDbRoot) {
			if err := os.MkdirAll(testDbRoot, 0700); err != nil {
				err := fmt.Errorf("unable to create test db "+
					"root: %v", err)
				return nil, nil, err
			}
		}

		// Create a new database to store the accepted blocks into.
		dbPath := filepath.Join(testDbRoot, dbName)
		_ = os.RemoveAll(dbPath)
		ndb, err := database.CreateDB(testDbType, dbPath)
		if err != nil {
			return nil, nil, fmt.Errorf("error creating db: %v", err)
		}
		db = ndb

		// Setup a teardown function for cleaning up.  This function is
		// returned to the caller to be invoked when it is done testing.
		teardown = func() {
			dbVersionPath := filepath.Join(testDbRoot, dbName+".ver")
			tmdb.Close()
			db.Sync()
			db.Close()
			os.RemoveAll(dbPath)
			os.Remove(dbVersionPath)
			os.RemoveAll(testDbRoot)
		}
	}

	// Insert the main network genesis block.  This is part of the initial
	// database setup.
	genesisBlock := dcrutil.NewBlock(params.GenesisBlock)
	genesisBlock.SetHeight(int64(0))
	_, err := db.InsertBlock(genesisBlock)
	if err != nil {
		teardown()
		err := fmt.Errorf("failed to insert genesis block: %v", err)
		return nil, nil, err
	}

	// Start the ticket database.
	tmdb.Initialize(params, db)
	tmdb.RescanTicketDB()

	chain := blockchain.New(db, tmdb, params, nil, nil)
	return chain, teardown, nil
}

// loadTxStore returns a transaction store loaded from a file.
func loadTxStore(filename string) (blockchain.TxStore, error) {
	// The txstore file format is:
	// <num tx data entries> <tx length> <serialized tx> <blk height>
	// <num spent bits> <spent bits>
	//
	// All num and length fields are little-endian uint32s.  The spent bits
	// field is padded to a byte boundary.

	filename = filepath.Join("testdata/", filename)
	fi, err := os.Open(filename)
	if err != nil {
		return nil, err
	}

	// Choose read based on whether the file is compressed or not.
	var r io.Reader
	if strings.HasSuffix(filename, ".bz2") {
		r = bzip2.NewReader(fi)
	} else {
		r = fi
	}
	defer fi.Close()

	// Num of transaction store objects.
	var numItems uint32
	if err := binary.Read(r, binary.LittleEndian, &numItems); err != nil {
		return nil, err
	}

	txStore := make(blockchain.TxStore)
	var uintBuf uint32
	for height := uint32(0); height < numItems; height++ {
		txD := blockchain.TxData{}

		// Serialized transaction length.
		err = binary.Read(r, binary.LittleEndian, &uintBuf)
		if err != nil {
			return nil, err
		}
		serializedTxLen := uintBuf
		if serializedTxLen > wire.MaxBlockPayload {
			return nil, fmt.Errorf("Read serialized transaction "+
				"length of %d is larger max allowed %d",
				serializedTxLen, wire.MaxBlockPayload)
		}

		// Transaction.
		var msgTx wire.MsgTx
		err = msgTx.Deserialize(r)
		if err != nil {
			return nil, err
		}
		txD.Tx = dcrutil.NewTx(&msgTx)

		// Transaction hash.
		txHash := msgTx.TxSha()
		txD.Hash = &txHash

		// Block height the transaction came from.
		err = binary.Read(r, binary.LittleEndian, &uintBuf)
		if err != nil {
			return nil, err
		}
		txD.BlockHeight = int64(uintBuf)

		// Num spent bits.
		err = binary.Read(r, binary.LittleEndian, &uintBuf)
		if err != nil {
			return nil, err
		}
		numSpentBits := uintBuf
		numSpentBytes := numSpentBits / 8
		if numSpentBits%8 != 0 {
			numSpentBytes++
		}

		// Packed spent bytes.
		spentBytes := make([]byte, numSpentBytes)
		_, err = io.ReadFull(r, spentBytes)
		if err != nil {
			return nil, err
		}

		// Populate spent data based on spent bits.
		txD.Spent = make([]bool, numSpentBits)
		for byteNum, spentByte := range spentBytes {
			for bit := 0; bit < 8; bit++ {
				if uint32((byteNum*8)+bit) < numSpentBits {
					if spentByte&(1<<uint(bit)) != 0 {
						txD.Spent[(byteNum*8)+bit] = true
					}
				}
			}
		}

		txStore[*txD.Hash] = &txD
	}

	return txStore, nil
}
