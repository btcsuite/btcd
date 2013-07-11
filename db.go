// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcdb

import (
	"errors"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// Errors that the various database functions may return.
var (
	PrevShaMissing = errors.New("Previous sha missing from database")
	TxShaMissing   = errors.New("Requested transaction does not exist")
	DuplicateSha   = errors.New("Duplicate insert attempted")
	DbDoesNotExist = errors.New("Non-existent database")
	DbUnknownType  = errors.New("Non-existent database type")
)

// AllShas is a special value that can be used as the final sha when requesting
// a range of shas by height to request them all.
const AllShas = int64(^uint64(0) >> 1)

// InsertMode represents a hint to the database about how much data the
// application is expecting to send to the database in a short period of time.
// This in turn provides the database with the opportunity to work in optimized
// modes when it will be very busy such as during the initial block chain
// download.
type InsertMode int

// Constants used to indicate the database insert mode hint.  See InsertMode.
const (
	InsertNormal InsertMode = iota
	InsertFast
	InsertValidatedInput
)

// Db defines a generic interface that is used to request and insert data into
// the bitcoin block chain.  This interface is intended to be agnostic to actual
// mechanism used for backend data storage.  The AddDBDriver function can be
// used to add a new backend data storage method.
type Db interface {
	// Close cleanly shuts down the database and syncs all data.
	Close()

	// DropAfterBlockBySha will remove any blocks from the database after
	// the given block.  It terminates any existing transaction and performs
	// its operations in an atomic transaction which is commited before
	// the function returns.
	DropAfterBlockBySha(*btcwire.ShaHash) (err error)

	// ExistsSha returns whether or not the given block hash is present in
	// the database.
	ExistsSha(sha *btcwire.ShaHash) (exists bool)

	// FetchBlockBySha returns a btcutil Block.  The implementation may
	// cache the underlying data if desired.
	FetchBlockBySha(sha *btcwire.ShaHash) (blk *btcutil.Block, err error)

	// FetchBlockShaByHeight returns a block hash based on its height in the
	// block chain.
	FetchBlockShaByHeight(height int64) (sha *btcwire.ShaHash, err error)

	// FetchHeightRange looks up a range of blocks by the start and ending
	// heights.  Fetch is inclusive of the start height and exclusive of the
	// ending height. To fetch all hashes from the start height until no
	// more are present, use the special id `AllShas'.
	FetchHeightRange(startHeight, endHeight int64) (rshalist []btcwire.ShaHash, err error)

	// ExistsTxSha returns whether or not the given tx hash is present in
	// the database
	ExistsTxSha(sha *btcwire.ShaHash) (exists bool)

	// FetchTxAllBySha returns several pieces of data for a given
	// transaction hash.  The implementation may cache the underlying data
	// if desired.
	FetchTxAllBySha(txsha *btcwire.ShaHash) (rtx *btcwire.MsgTx, rtxbuf []byte, rpver uint32, rblksha *btcwire.ShaHash, err error)

	// FetchTxBufBySha returns the raw bytes and associated protocol version
	// for the transaction with the requested hash.  The implementation may
	// cache the underlying data if desired.
	FetchTxBufBySha(txsha *btcwire.ShaHash) (txbuf []byte, rpver uint32, err error)

	// FetchTxBySha returns some data for the given transaction hash. The
	// implementation may cache the underlying data if desired.
	FetchTxBySha(txsha *btcwire.ShaHash) (rtx *btcwire.MsgTx, rpver uint32, blksha *btcwire.ShaHash, err error)

	// FetchTxByShaList returns a TxListReply given an array of transaction
	// hashes.  The implementation may cache the underlying data if desired.
	FetchTxByShaList(txShaList []*btcwire.ShaHash) []*TxListReply

	// FetchTxUsedBySha returns the used/spent buffer for a given
	// transaction hash.
	FetchTxUsedBySha(txsha *btcwire.ShaHash) (spentbuf []byte, err error)

	// InsertBlock inserts raw block and transaction data from a block
	// into the database.
	InsertBlock(block *btcutil.Block) (height int64, err error)

	// InsertBlockData stores a block hash and its associated data block
	// with the given previous hash and protocol version into the database.
	InsertBlockData(sha *btcwire.ShaHash, prevSha *btcwire.ShaHash, pver uint32, buf []byte) (blockid int64, err error)

	// InsertTx stores a transaction hash and its associated data into the
	// database.
	InsertTx(txsha *btcwire.ShaHash, blockHeight int64, txoff int, txlen int, usedbuf []byte) (err error)

	// InvalidateBlockCache releases all cached blocks.
	InvalidateBlockCache()

	// InvalidateCache releases all cached blocks and transactions.
	InvalidateCache()

	// InvalidateTxCache releases all cached transactions.
	InvalidateTxCache()

	// NewIterateBlocks returns an iterator for all blocks in database.
	NewIterateBlocks() (pbi BlockIterator, err error)

	// NewestSha provides an interface to quickly look up the hash of
	// the most recent (end) block of the block chain.
	NewestSha() (sha *btcwire.ShaHash, height int64, err error)

	// RollbackClose discards the recent database changes to the previously
	// saved data at last Sync and closes the database.
	RollbackClose()

	// SetDBInsertMode provides hints to the database about how the
	// application is running.  This allows the database to work in
	// optimized modes when the database may be very busy.
	SetDBInsertMode(InsertMode)

	// Sync verifies that the database is coherent on disk and no
	// outstanding transactions are in flight.
	Sync()
}

// BlockIterator defines a generic interface for an iterator through the block
// chain.
type BlockIterator interface {
	// Close shuts down the iterator when done walking blocks in the database.
	Close()

	// NextRow iterates thru all blocks in database.
	NextRow() bool

	// Row returns row data for block iterator.
	Row() (key *btcwire.ShaHash, pver uint32, buf []byte, err error)
}

// DriverDB defines a structure for backend drivers to use when they registered
// themselves as a backend which implements the Db interface.
type DriverDB struct {
	DbType string
	Create func(argstr string) (pbdb Db, err error)
	Open   func(filepath string) (pbdb Db, err error)
}

// TxListReply is used to return individual transaction information when
// data about multiple transactions is requested in a single call.
type TxListReply struct {
	Sha     *btcwire.ShaHash
	Tx      *btcwire.MsgTx
	Height  int64
	TxSpent []bool
	Err     error
}

// driverList holds all of the registered database backends.
var driverList []DriverDB

// AddDBDriver adds a back end database driver to available interfaces.
func AddDBDriver(instance DriverDB) {
	// TODO(drahn) Does this really need to check for duplicate names ?
	for _, drv := range driverList {
		// TODO(drahn) should duplicates be an error?
		if drv.DbType == instance.DbType {
			return
		}
	}
	driverList = append(driverList, instance)
}

// CreateDB intializes and opens a database.
func CreateDB(dbtype string, argstr string) (pbdb Db, err error) {
	for _, drv := range driverList {
		if drv.DbType == dbtype {
			return drv.Create(argstr)
		}
	}
	return nil, DbUnknownType
}

// OpenDB opens an existing database.
func OpenDB(dbtype string, argstr string) (pbdb Db, err error) {
	for _, drv := range driverList {
		if drv.DbType == dbtype {
			return drv.Open(argstr)
		}
	}
	return nil, DbUnknownType
}
