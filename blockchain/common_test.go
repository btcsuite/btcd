// Copyright (c) 2013-2015 The btcsuite developers
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

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	database "github.com/btcsuite/btcd/database2"
	_ "github.com/btcsuite/btcd/database2/ffldb"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

const (
	// testDbType is the database backend type to use for the tests.
	testDbType = "ffldb"

	// testDbRoot is the root directory used to create all test databases.
	testDbRoot = "testdbs"

	// blockDataNet is the expected network in the test block data.
	blockDataNet = wire.MainNet
)

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
	supportedDrivers := database.SupportedDrivers()
	for _, driver := range supportedDrivers {
		if dbType == driver {
			return true
		}
	}

	return false
}

// chainSetup is used to create a new db and chain instance with the genesis
// block already inserted.  In addition to the new chain instnce, it returns
// a teardown function the caller should invoke when done testing to clean up.
func chainSetup(dbName string) (*blockchain.BlockChain, func(), error) {
	if !isSupportedDbType(testDbType) {
		return nil, nil, fmt.Errorf("unsupported db type %v", testDbType)
	}

	// Handle memory database specially since it doesn't need the disk
	// specific handling.
	var db database.DB
	var teardown func()
	if testDbType == "memdb" {
		ndb, err := database.Create(testDbType)
		if err != nil {
			return nil, nil, fmt.Errorf("error creating db: %v", err)
		}
		db = ndb

		// Setup a teardown function for cleaning up.  This function is
		// returned to the caller to be invoked when it is done testing.
		teardown = func() {
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
		ndb, err := database.Create(testDbType, dbPath, blockDataNet)
		if err != nil {
			return nil, nil, fmt.Errorf("error creating db: %v", err)
		}
		db = ndb

		// Setup a teardown function for cleaning up.  This function is
		// returned to the caller to be invoked when it is done testing.
		teardown = func() {
			db.Close()
			os.RemoveAll(dbPath)
			os.RemoveAll(testDbRoot)
		}
	}

	// Create the main chain instance.
	chain, err := blockchain.New(db, &chaincfg.MainNetParams, nil, nil)
	if err != nil {
		teardown()
		err := fmt.Errorf("failed to create chain instance: %v", err)
		return nil, nil, err
	}
	return chain, teardown, nil
}

// loadUtxoView returns a utxo view loaded from a file.
func loadUtxoView(filename string) (*blockchain.UtxoViewpoint, error) {
	// TODO(davec): Once the the new utxo serialization format and code is
	// done, this and the data file should be updated to use it instead.

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

	utxoView := blockchain.NewUtxoViewpoint()
	var uintBuf uint32
	for height := uint32(0); height < numItems; height++ {
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

		// Block height the transaction came from.
		err = binary.Read(r, binary.LittleEndian, &uintBuf)
		if err != nil {
			return nil, err
		}

		// Add all of the transaction outputs as available.
		utxoView.AddTxOuts(btcutil.NewTx(&msgTx), int32(uintBuf))

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

		// Spend the outputs based on spent bits.
		txHash := msgTx.TxSha()
		entry := utxoView.LookupEntry(&txHash)
		for byteNum, spentByte := range spentBytes {
			for bit := 0; bit < 8; bit++ {
				outputIndex := uint32((byteNum * 8) + bit)
				if outputIndex < numSpentBits {
					if spentByte&(1<<uint(bit)) != 0 {
						entry.SpendOutput(outputIndex)
					}
				}
			}
		}
	}

	return utxoView, nil
}
