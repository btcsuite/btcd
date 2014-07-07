// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package memdb

import (
	"errors"
	"fmt"
	"math"
	"sync"

	"github.com/conformal/btcdb"
	"github.com/conformal/btcutil"
	"github.com/conformal/btcwire"
)

// Errors that the various database functions may return.
var (
	ErrDbClosed = errors.New("Database is closed")
)

var (
	zeroHash = btcwire.ShaHash{}

	// The following two hashes are ones that must be specially handled.
	// See the comments where they're used for more details.
	dupTxHash91842 = newShaHashFromStr("d5d27987d2a3dfc724e359870c6644b40e497bdc0589a033220fe15429d88599")
	dupTxHash91880 = newShaHashFromStr("e3bf3d07d4b0375638d5f1db5255fe07ba2c4cb067cd81b84ee974b6585fb468")
)

// tTxInsertData holds information about the location and spent status of
// a transaction.
type tTxInsertData struct {
	blockHeight int64
	offset      int
	spentBuf    []bool
}

// newShaHashFromStr converts the passed big-endian hex string into a
// btcwire.ShaHash.  It only differs from the one available in btcwire in that
// it ignores the error since it will only (and must only) be called with
// hard-coded, and therefore known good, hashes.
func newShaHashFromStr(hexStr string) *btcwire.ShaHash {
	sha, _ := btcwire.NewShaHashFromStr(hexStr)
	return sha
}

// isCoinbaseInput returns whether or not the passed transaction input is a
// coinbase input.  A coinbase is a special transaction created by miners that
// has no inputs.  This is represented in the block chain by a transaction with
// a single input that has a previous output transaction index set to the
// maximum value along with a zero hash.
func isCoinbaseInput(txIn *btcwire.TxIn) bool {
	prevOut := &txIn.PreviousOutpoint
	if prevOut.Index == math.MaxUint32 && prevOut.Hash.IsEqual(&zeroHash) {
		return true
	}

	return false
}

// isFullySpent returns whether or not a transaction represented by the passed
// transaction insert data is fully spent.  A fully spent transaction is one
// where all outputs are spent.
func isFullySpent(txD *tTxInsertData) bool {
	for _, spent := range txD.spentBuf {
		if !spent {
			return false
		}
	}

	return true
}

// MemDb is a concrete implementation of the btcdb.Db interface which provides
// a memory-only database.  Since it is memory-only, it is obviously not
// persistent and is mostly only useful for testing purposes.
type MemDb struct {
	// Embed a mutex for safe concurrent access.
	sync.Mutex

	// blocks holds all of the bitcoin blocks that will be in the memory
	// database.
	blocks []*btcwire.MsgBlock

	// blocksBySha keeps track of block heights by hash.  The height can
	// be used as an index into the blocks slice.
	blocksBySha map[btcwire.ShaHash]int64

	// txns holds information about transactions such as which their
	// block height and spent status of all their outputs.
	txns map[btcwire.ShaHash][]*tTxInsertData

	// closed indicates whether or not the database has been closed and is
	// therefore invalidated.
	closed bool
}

// removeTx removes the passed transaction including unspending it.
func (db *MemDb) removeTx(msgTx *btcwire.MsgTx, txHash *btcwire.ShaHash) {
	// Undo all of the spends for the transaction.
	for _, txIn := range msgTx.TxIn {
		if isCoinbaseInput(txIn) {
			continue
		}

		prevOut := &txIn.PreviousOutpoint
		originTxns, exists := db.txns[prevOut.Hash]
		if !exists {
			log.Warnf("Unable to find input transaction %s to "+
				"unspend %s index %d", prevOut.Hash, txHash,
				prevOut.Index)
			continue
		}

		originTxD := originTxns[len(originTxns)-1]
		originTxD.spentBuf[prevOut.Index] = false
	}

	// Remove the info for the most recent version of the transaction.
	txns := db.txns[*txHash]
	lastIndex := len(txns) - 1
	txns[lastIndex] = nil
	txns = txns[:lastIndex]
	db.txns[*txHash] = txns

	// Remove the info entry from the map altogether if there not any older
	// versions of the transaction.
	if len(txns) == 0 {
		delete(db.txns, *txHash)
	}

}

// Close cleanly shuts down database.  This is part of the btcdb.Db interface
// implementation.
//
// All data is purged upon close with this implementation since it is a
// memory-only database.
func (db *MemDb) Close() error {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return ErrDbClosed
	}

	db.blocks = nil
	db.blocksBySha = nil
	db.txns = nil
	db.closed = true
	return nil
}

// DropAfterBlockBySha removes any blocks from the database after the given
// block.  This is different than a simple truncate since the spend information
// for each block must also be unwound.  This is part of the btcdb.Db interface
// implementation.
func (db *MemDb) DropAfterBlockBySha(sha *btcwire.ShaHash) error {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return ErrDbClosed
	}

	// Begin by attempting to find the height associated with the passed
	// hash.
	height, exists := db.blocksBySha[*sha]
	if !exists {
		return fmt.Errorf("block %v does not exist in the database",
			sha)
	}

	// The spend information has to be undone in reverse order, so loop
	// backwards from the last block through the block just after the passed
	// block.  While doing this unspend all transactions in each block and
	// remove the block.
	endHeight := int64(len(db.blocks) - 1)
	for i := endHeight; i > height; i-- {
		// Unspend and remove each transaction in reverse order because
		// later transactions in a block can reference earlier ones.
		transactions := db.blocks[i].Transactions
		for j := len(transactions) - 1; j >= 0; j-- {
			tx := transactions[j]
			txHash, _ := tx.TxSha()
			db.removeTx(tx, &txHash)
		}

		db.blocks[i] = nil
		db.blocks = db.blocks[:i]
	}

	return nil
}

// ExistsSha returns whether or not the given block hash is present in the
// database.  This is part of the btcdb.Db interface implementation.
func (db *MemDb) ExistsSha(sha *btcwire.ShaHash) (bool, error) {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return false, ErrDbClosed
	}

	if _, exists := db.blocksBySha[*sha]; exists {
		return true, nil
	}

	return false, nil
}

// FetchBlockBySha returns a btcutil.Block.  The implementation may cache the
// underlying data if desired.  This is part of the btcdb.Db interface
// implementation.
//
// This implementation does not use any additional cache since the entire
// database is already in memory.
func (db *MemDb) FetchBlockBySha(sha *btcwire.ShaHash) (*btcutil.Block, error) {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return nil, ErrDbClosed
	}

	if blockHeight, exists := db.blocksBySha[*sha]; exists {
		block := btcutil.NewBlock(db.blocks[int(blockHeight)])
		block.SetHeight(blockHeight)
		return block, nil
	}

	return nil, fmt.Errorf("block %v is not in database", sha)
}

// FetchBlockHeightBySha returns the block height for the given hash.  This is
// part of the btcdb.Db interface implementation.
func (db *MemDb) FetchBlockHeightBySha(sha *btcwire.ShaHash) (int64, error) {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return 0, ErrDbClosed
	}

	if blockHeight, exists := db.blocksBySha[*sha]; exists {
		return blockHeight, nil
	}

	return 0, fmt.Errorf("block %v is not in database", sha)
}

// FetchBlockHeaderBySha returns a btcwire.BlockHeader for the given sha.  The
// implementation may cache the underlying data if desired.  This is part of the
// btcdb.Db interface implementation.
//
// This implementation does not use any additional cache since the entire
// database is already in memory.
func (db *MemDb) FetchBlockHeaderBySha(sha *btcwire.ShaHash) (*btcwire.BlockHeader, error) {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return nil, ErrDbClosed
	}

	if blockHeight, exists := db.blocksBySha[*sha]; exists {
		return &db.blocks[int(blockHeight)].Header, nil
	}

	return nil, fmt.Errorf("block header %v is not in database", sha)
}

// FetchBlockShaByHeight returns a block hash based on its height in the block
// chain.  This is part of the btcdb.Db interface implementation.
func (db *MemDb) FetchBlockShaByHeight(height int64) (*btcwire.ShaHash, error) {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return nil, ErrDbClosed
	}

	numBlocks := int64(len(db.blocks))
	if height < 0 || height > numBlocks-1 {
		return nil, fmt.Errorf("unable to fetch block height %d since "+
			"it is not within the valid range (%d-%d)", height, 0,
			numBlocks-1)
	}

	msgBlock := db.blocks[height]
	blockHash, err := msgBlock.BlockSha()
	if err != nil {
		return nil, err
	}

	return &blockHash, nil
}

// FetchHeightRange looks up a range of blocks by the start and ending heights.
// Fetch is inclusive of the start height and exclusive of the ending height.
// To fetch all hashes from the start height until no more are present, use the
// special id `AllShas'.  This is part of the btcdb.Db interface implementation.
func (db *MemDb) FetchHeightRange(startHeight, endHeight int64) ([]btcwire.ShaHash, error) {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return nil, ErrDbClosed
	}

	// When the user passes the special AllShas id, adjust the end height
	// accordingly.
	if endHeight == btcdb.AllShas {
		endHeight = int64(len(db.blocks))
	}

	// Ensure requested heights are sane.
	if startHeight < 0 {
		return nil, fmt.Errorf("start height of fetch range must not "+
			"be less than zero - got %d", startHeight)
	}
	if endHeight < startHeight {
		return nil, fmt.Errorf("end height of fetch range must not "+
			"be less than the start height - got start %d, end %d",
			startHeight, endHeight)
	}

	// Fetch as many as are availalbe within the specified range.
	lastBlockIndex := int64(len(db.blocks) - 1)
	hashList := make([]btcwire.ShaHash, 0, endHeight-startHeight)
	for i := startHeight; i < endHeight; i++ {
		if i > lastBlockIndex {
			break
		}

		msgBlock := db.blocks[i]
		blockHash, err := msgBlock.BlockSha()
		if err != nil {
			return nil, err
		}
		hashList = append(hashList, blockHash)
	}

	return hashList, nil
}

// ExistsTxSha returns whether or not the given transaction hash is present in
// the database and is not fully spent.  This is part of the btcdb.Db interface
// implementation.
func (db *MemDb) ExistsTxSha(sha *btcwire.ShaHash) (bool, error) {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return false, ErrDbClosed
	}

	if txns, exists := db.txns[*sha]; exists {
		return !isFullySpent(txns[len(txns)-1]), nil
	}

	return false, nil
}

// FetchTxBySha returns some data for the given transaction hash. The
// implementation may cache the underlying data if desired.  This is part of the
// btcdb.Db interface implementation.
//
// This implementation does not use any additional cache since the entire
// database is already in memory.
func (db *MemDb) FetchTxBySha(txHash *btcwire.ShaHash) ([]*btcdb.TxListReply, error) {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return nil, ErrDbClosed
	}

	txns, exists := db.txns[*txHash]
	if !exists {
		log.Warnf("FetchTxBySha: requested hash of %s does not exist",
			txHash)
		return nil, btcdb.TxShaMissing
	}

	txHashCopy := *txHash
	replyList := make([]*btcdb.TxListReply, len(txns))
	for i, txD := range txns {
		msgBlock := db.blocks[txD.blockHeight]
		blockSha, err := msgBlock.BlockSha()
		if err != nil {
			return nil, err
		}

		spentBuf := make([]bool, len(txD.spentBuf))
		copy(spentBuf, txD.spentBuf)
		reply := btcdb.TxListReply{
			Sha:     &txHashCopy,
			Tx:      msgBlock.Transactions[txD.offset],
			BlkSha:  &blockSha,
			Height:  txD.blockHeight,
			TxSpent: spentBuf,
			Err:     nil,
		}
		replyList[i] = &reply
	}

	return replyList, nil
}

// fetchTxByShaList fetches transactions and information about them given an
// array of transaction hashes.  The result is a slice of of TxListReply objects
// which contain the transaction and information about it such as what block and
// block height it's contained in and which outputs are spent.
//
// The includeSpent flag indicates whether or not information about transactions
// which are fully spent should be returned.  When the flag is not set, the
// corresponding entry in the TxListReply slice for fully spent transactions
// will indicate the transaction does not exist.
//
// This function must be called with the db lock held.
func (db *MemDb) fetchTxByShaList(txShaList []*btcwire.ShaHash, includeSpent bool) []*btcdb.TxListReply {
	replyList := make([]*btcdb.TxListReply, 0, len(txShaList))
	for i, hash := range txShaList {
		// Every requested entry needs a response, so start with nothing
		// more than a response with the requested hash marked missing.
		// The reply will be updated below with the appropriate
		// information if the transaction exists.
		reply := btcdb.TxListReply{
			Sha: txShaList[i],
			Err: btcdb.TxShaMissing,
		}
		replyList = append(replyList, &reply)

		if db.closed {
			reply.Err = ErrDbClosed
			continue
		}

		if txns, exists := db.txns[*hash]; exists {
			// A given transaction may have duplicates so long as the
			// previous one is fully spent.  We are only interested
			// in the most recent version of the transaction for
			// this function.  The FetchTxBySha function can be
			// used to get all versions of a transaction.
			txD := txns[len(txns)-1]
			if !includeSpent && isFullySpent(txD) {
				continue
			}

			// Look up the referenced block and get its hash.  Set
			// the reply error appropriately and go to the next
			// requested transaction if anything goes wrong.
			msgBlock := db.blocks[txD.blockHeight]
			blockSha, err := msgBlock.BlockSha()
			if err != nil {
				reply.Err = err
				continue
			}

			// Make a copy of the spent buf to return so the caller
			// can't accidentally modify it.
			spentBuf := make([]bool, len(txD.spentBuf))
			copy(spentBuf, txD.spentBuf)

			// Populate the reply.
			reply.Tx = msgBlock.Transactions[txD.offset]
			reply.BlkSha = &blockSha
			reply.Height = txD.blockHeight
			reply.TxSpent = spentBuf
			reply.Err = nil
		}
	}

	return replyList
}

// FetchTxByShaList returns a TxListReply given an array of transaction hashes.
// The implementation may cache the underlying data if desired.  This is part of
// the btcdb.Db interface implementation.
//
// This implementation does not use any additional cache since the entire
// database is already in memory.

// FetchTxByShaList returns a TxListReply given an array of transaction
// hashes.  This function differs from FetchUnSpentTxByShaList in that it
// returns the most recent version of fully spent transactions.  Due to the
// increased number of transaction fetches, this function is typically more
// expensive than the unspent counterpart, however the specific performance
// details depend on the concrete implementation.  The implementation may cache
// the underlying data if desired.  This is part of the btcdb.Db interface
// implementation.
//
// To fetch all versions of a specific transaction, call FetchTxBySha.
//
// This implementation does not use any additional cache since the entire
// database is already in memory.
func (db *MemDb) FetchTxByShaList(txShaList []*btcwire.ShaHash) []*btcdb.TxListReply {
	db.Lock()
	defer db.Unlock()

	return db.fetchTxByShaList(txShaList, true)
}

// FetchUnSpentTxByShaList returns a TxListReply given an array of transaction
// hashes.  Any transactions which are fully spent will indicate they do not
// exist by setting the Err field to TxShaMissing.  The implementation may cache
// the underlying data if desired.  This is part of the btcdb.Db interface
// implementation.
//
// To obtain results which do contain the most recent version of a fully spent
// transactions, call FetchTxByShaList.  To fetch all versions of a specific
// transaction, call FetchTxBySha.
//
// This implementation does not use any additional cache since the entire
// database is already in memory.
func (db *MemDb) FetchUnSpentTxByShaList(txShaList []*btcwire.ShaHash) []*btcdb.TxListReply {
	db.Lock()
	defer db.Unlock()

	return db.fetchTxByShaList(txShaList, false)
}

// InsertBlock inserts raw block and transaction data from a block into the
// database.  The first block inserted into the database will be treated as the
// genesis block.  Every subsequent block insert requires the referenced parent
// block to already exist.  This is part of the btcdb.Db interface
// implementation.
func (db *MemDb) InsertBlock(block *btcutil.Block) (int64, error) {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return 0, ErrDbClosed
	}

	blockHash, err := block.Sha()
	if err != nil {
		return 0, err
	}

	// Reject the insert if the previously reference block does not exist
	// except in the case there are no blocks inserted yet where the first
	// inserted block is assumed to be a genesis block.
	msgBlock := block.MsgBlock()
	if _, exists := db.blocksBySha[msgBlock.Header.PrevBlock]; !exists {
		if len(db.blocks) > 0 {
			return 0, btcdb.PrevShaMissing
		}
	}

	// Build a map of in-flight transactions because some of the inputs in
	// this block could be referencing other transactions earlier in this
	// block which are not yet in the chain.
	txInFlight := map[btcwire.ShaHash]int{}
	transactions := block.Transactions()
	for i, tx := range transactions {
		txInFlight[*tx.Sha()] = i
	}

	// Loop through all transactions and inputs to ensure there are no error
	// conditions that would prevent them from be inserted into the db.
	// Although these checks could could be done in the loop below, checking
	// for error conditions up front means the code below doesn't have to
	// deal with rollback on errors.
	newHeight := int64(len(db.blocks))
	for i, tx := range transactions {
		// Two old blocks contain duplicate transactions due to being
		// mined by faulty miners and accepted by the origin Satoshi
		// client.  Rules have since been added to the ensure this
		// problem can no longer happen, but the two duplicate
		// transactions which were originally accepted are forever in
		// the block chain history and must be dealth with specially.
		// http://blockexplorer.com/b/91842
		// http://blockexplorer.com/b/91880
		if newHeight == 91842 && tx.Sha().IsEqual(dupTxHash91842) {
			continue
		}

		if newHeight == 91880 && tx.Sha().IsEqual(dupTxHash91880) {
			continue
		}

		for _, txIn := range tx.MsgTx().TxIn {
			if isCoinbaseInput(txIn) {
				continue
			}

			// It is acceptable for a transaction input to reference
			// the output of another transaction in this block only
			// if the referenced transaction comes before the
			// current one in this block.
			prevOut := &txIn.PreviousOutpoint
			if inFlightIndex, ok := txInFlight[prevOut.Hash]; ok {
				if i <= inFlightIndex {
					log.Warnf("InsertBlock: requested hash "+
						" of %s does not exist in-flight",
						tx.Sha())
					return 0, btcdb.TxShaMissing
				}
			} else {
				originTxns, exists := db.txns[prevOut.Hash]
				if !exists {
					log.Warnf("InsertBlock: requested hash "+
						"of %s by %s does not exist",
						prevOut.Hash, tx.Sha())
					return 0, btcdb.TxShaMissing
				}
				originTxD := originTxns[len(originTxns)-1]
				if prevOut.Index > uint32(len(originTxD.spentBuf)) {
					log.Warnf("InsertBlock: requested hash "+
						"of %s with index %d does not "+
						"exist", tx.Sha(), prevOut.Index)
					return 0, btcdb.TxShaMissing
				}
			}
		}

		// Prevent duplicate transactions in the same block.
		if inFlightIndex, exists := txInFlight[*tx.Sha()]; exists &&
			inFlightIndex < i {
			log.Warnf("Block contains duplicate transaction %s",
				tx.Sha())
			return 0, btcdb.DuplicateSha
		}

		// Prevent duplicate transactions unless the old one is fully
		// spent.
		if txns, exists := db.txns[*tx.Sha()]; exists {
			txD := txns[len(txns)-1]
			if !isFullySpent(txD) {
				log.Warnf("Attempt to insert duplicate "+
					"transaction %s", tx.Sha())
				return 0, btcdb.DuplicateSha
			}
		}
	}

	db.blocks = append(db.blocks, msgBlock)
	db.blocksBySha[*blockHash] = newHeight

	// Insert information about eacj transaction and spend all of the
	// outputs referenced by the inputs to the transactions.
	for i, tx := range block.Transactions() {
		// Insert the transaction data.
		txD := tTxInsertData{
			blockHeight: newHeight,
			offset:      i,
			spentBuf:    make([]bool, len(tx.MsgTx().TxOut)),
		}
		db.txns[*tx.Sha()] = append(db.txns[*tx.Sha()], &txD)

		// Spend all of the inputs.
		for _, txIn := range tx.MsgTx().TxIn {
			// Coinbase transaction has no inputs.
			if isCoinbaseInput(txIn) {
				continue
			}

			// Already checked for existing and valid ranges above.
			prevOut := &txIn.PreviousOutpoint
			originTxns := db.txns[prevOut.Hash]
			originTxD := originTxns[len(originTxns)-1]
			originTxD.spentBuf[prevOut.Index] = true
		}
	}

	return newHeight, nil
}

// NewestSha returns the hash and block height of the most recent (end) block of
// the block chain.  It will return the zero hash, -1 for the block height, and
// no error (nil) if there are not any blocks in the database yet.  This is part
// of the btcdb.Db interface implementation.
func (db *MemDb) NewestSha() (*btcwire.ShaHash, int64, error) {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return nil, 0, ErrDbClosed
	}

	// When the database has not had a genesis block inserted yet, return
	// values specified by interface contract.
	numBlocks := len(db.blocks)
	if numBlocks == 0 {
		return &zeroHash, -1, nil
	}

	blockSha, err := db.blocks[numBlocks-1].BlockSha()
	if err != nil {
		return nil, -1, err
	}

	return &blockSha, int64(numBlocks - 1), nil
}

// RollbackClose discards the recent database changes to the previously saved
// data at last Sync and closes the database.  This is part of the btcdb.Db
// interface implementation.
//
// The database is completely purged on close with this implementation since the
// entire database is only in memory.  As a result, this function behaves no
// differently than Close.
func (db *MemDb) RollbackClose() error {
	// Rollback doesn't apply to a memory database, so just call Close.
	// Close handles the mutex locks.
	return db.Close()
}

// Sync verifies that the database is coherent on disk and no outstanding
// transactions are in flight.  This is part of the btcdb.Db interface
// implementation.
//
// This implementation does not write any data to disk, so this function only
// grabs a lock to ensure it doesn't return until other operations are complete.
func (db *MemDb) Sync() error {
	db.Lock()
	defer db.Unlock()

	if db.closed {
		return ErrDbClosed
	}

	// There is nothing extra to do to sync the memory database.  However,
	// the lock is still grabbed to ensure the function does not return
	// until other operations are complete.
	return nil
}

// newMemDb returns a new memory-only database ready for block inserts.
func newMemDb() *MemDb {
	db := MemDb{
		blocks:      make([]*btcwire.MsgBlock, 0, 200000),
		blocksBySha: make(map[btcwire.ShaHash]int64),
		txns:        make(map[btcwire.ShaHash][]*tTxInsertData),
	}
	return &db
}
