// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package database

import (
	"errors"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"

	"github.com/btcsuite/golangcrypto/ripemd160"
)

// Errors that the various database functions may return.
var (
	ErrAddrIndexDoesNotExist = errors.New("address index hasn't been built " +
		"or is an older version")
	ErrUnsupportedAddressType = errors.New("address type is not supported " +
		"by the address-index")
	ErrPrevShaMissing  = errors.New("previous sha missing from database")
	ErrTxShaMissing    = errors.New("requested transaction does not exist")
	ErrBlockShaMissing = errors.New("requested block does not exist")
	ErrDuplicateSha    = errors.New("duplicate insert attempted")
	ErrDbDoesNotExist  = errors.New("non-existent database")
	ErrDbUnknownType   = errors.New("non-existent database type")
	ErrDbInconsistency = errors.New("inconsistent database")
	ErrNotImplemented  = errors.New("method has not yet been implemented")
)

// AllShas is a special value that can be used as the final sha when requesting
// a range of shas by height to request them all.
const AllShas = int64(^uint64(0) >> 1)

// Db defines a generic interface that is used to request and insert data into
// the decred block chain.  This interface is intended to be agnostic to actual
// mechanism used for backend data storage.  The AddDBDriver function can be
// used to add a new backend data storage method.
type Db interface {
	// Close cleanly shuts down the database and syncs all data.
	Close() (err error)

	// DropAfterBlockBySha will remove any blocks from the database after
	// the given block.  It terminates any existing transaction and performs
	// its operations in an atomic transaction which is committed before
	// the function returns.
	DropAfterBlockBySha(*chainhash.Hash) (err error)

	// ExistsSha returns whether or not the given block hash is present in
	// the database.
	ExistsSha(sha *chainhash.Hash) (exists bool, err error)

	// FetchBlockBySha returns a dcrutil Block.  The implementation may
	// cache the underlying data if desired.
	FetchBlockBySha(sha *chainhash.Hash) (blk *dcrutil.Block, err error)

	// FetchBlockHeightBySha returns the block height for the given hash.
	FetchBlockHeightBySha(sha *chainhash.Hash) (height int64, err error)

	// FetchBlockHeaderBySha returns a wire.BlockHeader for the given
	// sha.  The implementation may cache the underlying data if desired.
	FetchBlockHeaderBySha(sha *chainhash.Hash) (bh *wire.BlockHeader, err error)

	// FetchBlockShaByHeight returns a block hash based on its height in the
	// block chain.
	FetchBlockShaByHeight(height int64) (sha *chainhash.Hash, err error)

	// FetchHeightRange looks up a range of blocks by the start and ending
	// heights.  Fetch is inclusive of the start height and exclusive of the
	// ending height. To fetch all hashes from the start height until no
	// more are present, use the special id `AllShas'.
	FetchHeightRange(startHeight, endHeight int64) (rshalist []chainhash.Hash, err error)

	// ExistsTxSha returns whether or not the given tx hash is present in
	// the database
	ExistsTxSha(sha *chainhash.Hash) (exists bool, err error)

	// FetchTxBySha returns some data for the given transaction hash. The
	// implementation may cache the underlying data if desired.
	FetchTxBySha(txsha *chainhash.Hash) ([]*TxListReply, error)

	// FetchTxByShaList returns a TxListReply given an array of transaction
	// hashes.  The implementation may cache the underlying data if desired.
	// This differs from FetchUnSpentTxByShaList in that it will return
	// the most recent known Tx, if it is fully spent or not.
	//
	// NOTE: This function does not return an error directly since it MUST
	// return at least one TxListReply instance for each requested
	// transaction.  Each TxListReply instance then contains an Err field
	// which can be used to detect errors.
	FetchTxByShaList(txShaList []*chainhash.Hash) []*TxListReply

	// FetchUnSpentTxByShaList returns a TxListReply given an array of
	// transaction hashes.  The implementation may cache the underlying
	// data if desired. Fully spent transactions will not normally not
	// be returned in this operation.
	//
	// NOTE: This function does not return an error directly since it MUST
	// return at least one TxListReply instance for each requested
	// transaction.  Each TxListReply instance then contains an Err field
	// which can be used to detect errors.
	FetchUnSpentTxByShaList(txShaList []*chainhash.Hash) []*TxListReply

	// InsertBlock inserts raw block and transaction data from a block
	// into the database.  The first block inserted into the database
	// will be treated as the genesis block.  Every subsequent block insert
	// requires the referenced parent block to already exist.
	InsertBlock(block *dcrutil.Block) (height int64, err error)

	// NewestSha returns the hash and block height of the most recent (end)
	// block of the block chain.  It will return the zero hash, -1 for
	// the block height, and no error (nil) if there are not any blocks in
	// the database yet.
	NewestSha() (sha *chainhash.Hash, height int64, err error)

	// FetchAddrIndexTip returns the hash and block height of the most recent
	// block which has had its address index populated. It will return
	// ErrAddrIndexDoesNotExist along with a zero hash, and -1 if the
	// addrindex hasn't yet been built up.
	FetchAddrIndexTip() (sha *chainhash.Hash, height int64, err error)

	// UpdateAddrIndexForBlock updates the stored addrindex with passed
	// index information for a particular block height. Additionally, it
	// will update the stored meta-data related to the curent tip of the
	// addr index. These two operations are performed in an atomic
	// transaction which is committed before the function returns.
	// Addresses are indexed by the raw bytes of their base58 decoded
	// hash160.
	UpdateAddrIndexForBlock(blkSha *chainhash.Hash, height int64,
		addrIndex BlockAddrIndex) error

	// DropAddrIndexForBlock removes all passed address indexes and sets
	// the current block index below the previous HEAD.
	DropAddrIndexForBlock(blkSha *chainhash.Hash, height int64,
		addrIndex BlockAddrIndex) error

	// FetchTxsForAddr looks up and returns all transactions which either
	// spend a previously created output of the passed address, or create
	// a new output locked to the passed address. The, `limit` parameter
	// should be the max number of transactions to be returned.
	// Additionally, if the caller wishes to skip forward in the results
	// some amount, the 'seek' represents how many results to skip.
	// The transactions are returned in chronological order by block height
	// from old to new, or from new to old if `reverse` is set.
	// NOTE: Values for both `seek` and `limit` MUST be positive.
	// It will return the array of fetched transactions, along with the amount
	// of transactions that were actually skipped.
	FetchTxsForAddr(addr dcrutil.Address, skip int, limit int, reverse bool) ([]*TxListReply, int, error)

	// PurgeAddrIndex deletes the entire addrindex stored within the DB.
	PurgeAddrIndex() error

	// RollbackClose discards the recent database changes to the previously
	// saved data at last Sync and closes the database.
	RollbackClose() (err error)

	// Sync verifies that the database is coherent on disk and no
	// outstanding transactions are in flight.
	Sync() (err error)
}

// DriverDB defines a structure for backend drivers to use when they registered
// themselves as a backend which implements the Db interface.
type DriverDB struct {
	DbType   string
	CreateDB func(args ...interface{}) (pbdb Db, err error)
	OpenDB   func(args ...interface{}) (pbdb Db, err error)
}

// TxListReply is used to return individual transaction information when
// data about multiple transactions is requested in a single call.
type TxListReply struct {
	Sha     *chainhash.Hash
	Tx      *wire.MsgTx
	BlkSha  *chainhash.Hash
	Height  int64
	Index   uint32
	TxSpent []bool
	Err     error
}

// TxAddrIndex is the location of a transaction containing an address or script
// hash reference inside a transaction, as given by the block it is found in.
type TxAddrIndex struct {
	Hash160  [ripemd160.Size]byte
	Height   uint32
	TxOffset uint32
	TxLen    uint32
}

// AddrIndexKeySize is the number of bytes used by keys into the BlockAddrIndex.
// 3 byte prefix ([]byte("a+-"))
// 20 byte RIPEMD160 hash
// 4 byte block height
// 4 byte txoffset
// 4 byte txlen
const AddrIndexKeySize = 3 + ripemd160.Size + 4 + 4 + 4

// BlockAddrIndex represents the indexing structure for addresses.
// It maps a hash160 to a list of transaction locations within a block that
// either pays to or spends from the passed UTXO for the hash160.
type BlockAddrIndex []*TxAddrIndex

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

// CreateDB initializes and opens a database.
func CreateDB(dbtype string, args ...interface{}) (pbdb Db, err error) {
	for _, drv := range driverList {
		if drv.DbType == dbtype {
			return drv.CreateDB(args...)
		}
	}
	return nil, ErrDbUnknownType
}

// OpenDB opens an existing database.
func OpenDB(dbtype string, args ...interface{}) (pbdb Db, err error) {
	for _, drv := range driverList {
		if drv.DbType == dbtype {
			return drv.OpenDB(args...)
		}
	}
	return nil, ErrDbUnknownType
}

// SupportedDBs returns a slice of strings that represent the database drivers
// that have been registered and are therefore supported.
func SupportedDBs() []string {
	var supportedDBs []string
	for _, drv := range driverList {
		supportedDBs = append(supportedDBs, drv.DbType)
	}
	return supportedDBs
}
