// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math/big"

	database "github.com/btcsuite/btcd/database2"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

var (
	// hashIndexBucketName is the name of the db bucket used to house to the
	// block hash -> block height index.
	hashIndexBucketName = []byte("hashidx")

	// heightIndexBucketName is the name of the db bucket used to house to
	// the block height -> block hash index.
	heightIndexBucketName = []byte("heightidx")

	// chainStateKeyName is the name of the db key used to store the best
	// chain state.
	chainStateKeyName = []byte("chainstate")

	// txIndexBucketName is the name of the db bucket used to house the
	// transaction index.
	txIndexBucketName = []byte("txidx")

	// spendBucketName  is the name of the db bucket used to house the
	// spend information.
	// NOTE: This is only temporary for testing purposes.  Ultimately this
	// will be replaced with a utxoset bucket.
	spendBucketName = []byte("spend")

	// byteOrder is the preferred byte order used for serializing numeric
	// fields for storage in the database.
	byteOrder = binary.LittleEndian
)

// errNotInMainChain signifies that a block hash or height that is not in the
// main chain was requested.
type errNotInMainChain string

// Error implements the error interface.
func (e errNotInMainChain) Error() string {
	return string(e)
}

// isNotInMainChainErr return whether or not the passed error is an
// errNotInMainChain error.
func isNotInMainChainErr(err error) bool {
	_, ok := err.(errNotInMainChain)
	return ok
}

// -----------------------------------------------------------------------------
// The transaction index consists of an entry for every transaction in the main
// chain.  Since it possible for multiple transactions to have the same hash as
// long as the previous transaction with the same hash is fully spent, each
// entry consists of one or more records.
//
// The serialized format is:
//
//   <num entries>[<block hash><start offset><tx length>,...]
//
//   Field           Type           Size
//   num entries     uint8          1 byte
//   block hash      wire.ShaHash   wire.HashSize
//   start offset    uint32         4 bytes
//   tx length       uint32         4 bytes
// -----------------------------------------------------------------------------

// dbHasTxIndexEntry uses an existing database transaction to return whether or
// not the transaction index contains the provided hash.
func dbHasTxIndexEntry(dbTx database.Tx, txHash *wire.ShaHash) bool {
	txIndex := dbTx.Metadata().Bucket(txIndexBucketName)
	return txIndex.Get(txHash[:]) != nil
}

// dbPutTxIndexEntry uses an existing database transaction to update the
// transaction index given the provided values.
func dbPutTxIndexEntry(dbTx database.Tx, txHash, blockHash *wire.ShaHash, txLoc wire.TxLoc) error {
	txIndex := dbTx.Metadata().Bucket(txIndexBucketName)
	existing := txIndex.Get(txHash[:])

	// Since a new entry is being added, there will be one more than is
	// already there (if any).
	numEntries := uint8(0)
	if len(existing) > 0 {
		numEntries = existing[0]
	}
	numEntries++

	// Serialize the entry.
	serializedData := make([]byte, (uint32(numEntries)*(wire.HashSize+8))+1)
	serializedData[0] = numEntries
	offset := uint32(1)
	if len(existing) > 0 {
		copy(serializedData, existing[1:])
		offset += uint32(len(existing) - 1)
	}
	copy(serializedData[offset:], blockHash[:])
	offset += wire.HashSize
	byteOrder.PutUint32(serializedData[offset:], uint32(txLoc.TxStart))
	offset += 4
	byteOrder.PutUint32(serializedData[offset:], uint32(txLoc.TxLen))

	return txIndex.Put(txHash[:], serializedData)
}

// dbFetchTxIndexEntry uses an existing database transaction to fetch the block
// region for the provided transaction hash from the transaction index.  When
// there is no entry for the provided hash, nil will be returned for the both
// the region and the error.  When there are multiple entries, the block region
// for the latest one will be returned.
func dbFetchTxIndexEntry(dbTx database.Tx, txHash *wire.ShaHash) (*database.BlockRegion, error) {
	// Load the record from the database and return now if it doesn't exist.
	txIndex := dbTx.Metadata().Bucket(txIndexBucketName)
	serializedData := txIndex.Get(txHash[:])
	if len(serializedData) == 0 {
		return nil, nil
	}

	// Ensure the serialized data has enough bytes to properly deserialize.
	numEntries := uint8(serializedData[0])
	if len(serializedData) < (int(numEntries)*(wire.HashSize+8) + 1) {
		return nil, database.Error{
			ErrorCode: database.ErrCorruption,
			Description: fmt.Sprintf("corrupt transaction index "+
				"entry for %s", txHash),
		}
	}

	// Deserialize the final entry.
	region := database.BlockRegion{Hash: &wire.ShaHash{}}
	offset := (uint32(numEntries)-1)*(wire.HashSize+8) + 1
	copy(region.Hash[:], serializedData[offset:offset+wire.HashSize])
	offset += wire.HashSize
	region.Offset = byteOrder.Uint32(serializedData[offset : offset+4])
	offset += 4
	region.Len = byteOrder.Uint32(serializedData[offset : offset+4])

	return &region, nil
}

// dbAddTxIndexEntries uses an existing database transaction to add a
// transaction index entry for every transaction in the passed block.
func dbAddTxIndexEntries(dbTx database.Tx, block *btcutil.Block) error {
	txLocs, err := block.TxLoc()
	if err != nil {
		return err
	}

	for i, tx := range block.Transactions() {
		err := dbPutTxIndexEntry(dbTx, tx.Sha(), block.Sha(), txLocs[i])
		if err != nil {
			return err
		}
	}

	return nil
}

// dbRemoveTxIndexEntry uses an existing database transaction to remove the most
// recent transaction index entry for the given hash.
func dbRemoveTxIndexEntry(dbTx database.Tx, txHash *wire.ShaHash) error {
	txIndex := dbTx.Metadata().Bucket(txIndexBucketName)
	serializedData := txIndex.Get(txHash[:])
	if len(serializedData) == 0 {
		return fmt.Errorf("can't remove non-existent transaction %s "+
			"from the transaction index", txHash)
	}

	// Remove the key altogether if there was only one entry.
	numEntries := uint8(serializedData[0])
	if numEntries == 1 {
		return txIndex.Delete(txHash[:])
	}

	// Ensure the serialized data has enough bytes to properly deserialize.
	if len(serializedData) < (int(numEntries)*(wire.HashSize+8) + 1) {
		return database.Error{
			ErrorCode: database.ErrCorruption,
			Description: fmt.Sprintf("corrupt transaction index "+
				"entry for %s", txHash),
		}
	}

	// Remove the latest entry since there is more than one.
	numEntries--
	serializedData[0] = numEntries
	offset := (numEntries-1)*(wire.HashSize+8) + 1
	serializedData = serializedData[:offset]
	return txIndex.Put(txHash[:], serializedData)
}

// dbRemoveTxIndexEntries uses an existing database transaction to remove the
// latest transaction entry for every transaction in the passed block.
func dbRemoveTxIndexEntries(dbTx database.Tx, block *btcutil.Block) error {
	for _, tx := range block.Transactions() {
		err := dbRemoveTxIndexEntry(dbTx, tx.Sha())
		if err != nil {
			return err
		}
	}

	return nil
}

// -----------------------------------------------------------------------------
// The block index consists of two buckets with an entry for every block in the
// main chain.  One bucket is for the hash to height mapping and the other is
// for the height to hash mapping.
//
// The serialized format for values in the hash to height bucket is:
//   <height>
//
//   Field      Type     Size
//   height     uint32   4 bytes
//
// The serialized format for values in the height to hash bucket is:
//   <hash>
//
//   Field      Type           Size
//   hash       wire.ShaHash   wire.HashSize
// -----------------------------------------------------------------------------

// dbPutBlockIndex uses an existing database transaction to update or add the
// block index entries for the hash to height and height to hash mappings for
// the provided values.
func dbPutBlockIndex(dbTx database.Tx, hash *wire.ShaHash, height int32) error {
	// Serialize the height for use in the index entries.
	var serializedHeight [4]byte
	byteOrder.PutUint32(serializedHeight[:], uint32(height))

	// Add the block hash to height mapping to the index.
	meta := dbTx.Metadata()
	hashIndex := meta.Bucket(hashIndexBucketName)
	if err := hashIndex.Put(hash[:], serializedHeight[:]); err != nil {
		return err
	}

	// Add the block height to hash mapping to the index.
	heightIndex := meta.Bucket(heightIndexBucketName)
	return heightIndex.Put(serializedHeight[:], hash[:])
}

// dbRemoveBlockIndex uses an existing database transaction remove block index
// entries from the hash to height and height to hash mappings for the provided
// values.
func dbRemoveBlockIndex(dbTx database.Tx, hash *wire.ShaHash, height int32) error {
	// Remove the block hash to height mapping.
	meta := dbTx.Metadata()
	hashIndex := meta.Bucket(hashIndexBucketName)
	if err := hashIndex.Delete(hash[:]); err != nil {
		return err
	}

	// Remove the block height to hash mapping.
	var serializedHeight [4]byte
	byteOrder.PutUint32(serializedHeight[:], uint32(height))
	heightIndex := meta.Bucket(heightIndexBucketName)
	return heightIndex.Delete(serializedHeight[:])
}

// dbFetchHeightByHash uses an existing database transaction to retrieve the
// height for the provided hash from the index.
func dbFetchHeightByHash(dbTx database.Tx, hash *wire.ShaHash) (int32, error) {
	meta := dbTx.Metadata()
	hashIndex := meta.Bucket(hashIndexBucketName)
	serializedHeight := hashIndex.Get(hash[:])
	if serializedHeight == nil {
		str := fmt.Sprintf("block %s is not in the main chain", hash)
		return 0, errNotInMainChain(str)
	}

	return int32(byteOrder.Uint32(serializedHeight)), nil
}

// dbFetchHashByHeight uses an existing database transaction to retrieve the
// hash for the provided height from the index.
func dbFetchHashByHeight(dbTx database.Tx, height int32) (*wire.ShaHash, error) {
	var serializedHeight [4]byte
	byteOrder.PutUint32(serializedHeight[:], uint32(height))

	meta := dbTx.Metadata()
	heightIndex := meta.Bucket(heightIndexBucketName)
	hashBytes := heightIndex.Get(serializedHeight[:])
	if hashBytes == nil {
		str := fmt.Sprintf("no block at height %d exists", height)
		return nil, errNotInMainChain(str)
	}

	var hash wire.ShaHash
	copy(hash[:], hashBytes)
	return &hash, nil
}

// -----------------------------------------------------------------------------
// The best chain state consists of the best block hash and height, the total
// number of transactions up to and including those in the best block, and the
// accumulated work sum up to and including the best block.
//
// The serialized format is:
//
//   <block hash><block height><total txns><work sum length><work sum>
//
//   Field             Type           Size
//   block hash        wire.ShaHash   wire.HashSize
//   block height      uint32         4 bytes
//   total txns        uint64         8 bytes
//   work sum length   uint32         4 bytes
//   work sum          big.Int        work sum length
// -----------------------------------------------------------------------------

// bestChainState represents the data to be stored the database for the current
// best chain state.
type bestChainState struct {
	hash      wire.ShaHash
	height    uint32
	totalTxns uint64
	workSum   *big.Int
}

// serializeBestChainState returns the serialization of the passed block best
// chain state.  This is data to be stored in the chain state bucket.
func serializeBestChainState(state bestChainState) []byte {
	// Calculate the full size needed to serialize the chain state.
	workSumBytes := state.workSum.Bytes()
	workSumBytesLen := uint32(len(workSumBytes))
	serializedLen := wire.HashSize + 4 + 8 + 4 + workSumBytesLen

	// Serialize the chain state.
	serializedData := make([]byte, serializedLen)
	copy(serializedData[0:wire.HashSize], state.hash[:])
	offset := uint32(wire.HashSize)
	byteOrder.PutUint32(serializedData[offset:], state.height)
	offset += 4
	byteOrder.PutUint64(serializedData[offset:], state.totalTxns)
	offset += 8
	byteOrder.PutUint32(serializedData[offset:], workSumBytesLen)
	offset += 4
	copy(serializedData[offset:], workSumBytes)
	return serializedData[:]
}

// deserializeBestChainState deserializes the passed serialized best chain
// state.  This is data stored in the chain state bucket and is updated after
// every block is connected or disconnected form the main chain.
// block.
func deserializeBestChainState(serializedData []byte) (bestChainState, error) {
	// Ensure the serialized data has enough bytes to properly deserialize
	// the hash, height, total transactions, and work sum length.
	if len(serializedData) < wire.HashSize+16 {
		return bestChainState{}, database.Error{
			ErrorCode:   database.ErrCorruption,
			Description: "corrupt best chain state",
		}
	}

	state := bestChainState{}
	copy(state.hash[:], serializedData[0:wire.HashSize])
	offset := uint32(wire.HashSize)
	state.height = byteOrder.Uint32(serializedData[offset : offset+4])
	offset += 4
	state.totalTxns = byteOrder.Uint64(serializedData[offset : offset+8])
	offset += 8
	workSumBytesLen := byteOrder.Uint32(serializedData[offset : offset+4])
	offset += 4

	// Ensure the serialized data has enough bytes to deserialize the work
	// sum.
	if uint32(len(serializedData[offset:])) < workSumBytesLen {
		return bestChainState{}, database.Error{
			ErrorCode:   database.ErrCorruption,
			Description: "corrupt best chain state",
		}
	}
	workSumBytes := serializedData[offset : offset+workSumBytesLen]
	state.workSum = new(big.Int).SetBytes(workSumBytes)

	return state, nil
}

// dbPutBestState uses an existing database transaction to update the best chain
// state with the given parameters.
func dbPutBestState(dbTx database.Tx, snapshot *BestState, workSum *big.Int) error {
	// Serialize the current best chain state.
	serializedData := serializeBestChainState(bestChainState{
		hash:      *snapshot.Hash,
		height:    uint32(snapshot.Height),
		totalTxns: snapshot.TotalTxns,
		workSum:   workSum,
	})

	// Store the current best chain state into the database.
	return dbTx.Metadata().Put(chainStateKeyName, serializedData)
}

// createChainState initializes both the database and the chain state to the
// genesis block.  This includes creating the necessary buckets and inserting
// the genesis block, so it must only be called on an uninitialized database.
func (b *BlockChain) createChainState() error {
	// Create a new node from the genesis block and set it as both the root
	// node and the best node.
	genesisBlock := btcutil.NewBlock(b.chainParams.GenesisBlock)
	header := &genesisBlock.MsgBlock().Header
	node := newBlockNode(header, genesisBlock.Sha(), 0)
	node.inMainChain = true
	b.bestNode = node
	b.root = node

	// Add the new node to the index which is used for faster lookups.
	b.index[*node.hash] = node

	// Initialize the state related to the best block.
	numTxns := uint64(len(genesisBlock.MsgBlock().Transactions))
	blockSize := uint64(genesisBlock.MsgBlock().SerializeSize())
	b.stateSnapshot = newBestState(b.bestNode, blockSize, numTxns, numTxns)

	// Create the initial the database chain state including creating the
	// necessary index buckets and inserting the genesis block.
	err := b.db.Update(func(dbTx database.Tx) error {
		// Create the bucket that houses the chain block hash to height
		// index.
		meta := dbTx.Metadata()
		_, err := meta.CreateBucket(hashIndexBucketName)
		if err != nil {
			return err
		}

		// Create the bucket that houses the chain block height to hash
		// index.
		_, err = meta.CreateBucket(heightIndexBucketName)
		if err != nil {
			return err
		}

		// Create the bucket that houses the transaction index.
		// TODO remove this once the txindex has been converted to a
		// blockchain index.
		_, err = meta.CreateBucket(txIndexBucketName)
		if err != nil {
			return err
		}

		// Create the bucket that houses the spend data.
		// NOTE: This is only temporary for testing purposes.
		// Ultimately this will be replaced with a utxoset bucket.
		_, err = meta.CreateBucket(spendBucketName)
		if err != nil {
			return err
		}

		// Create the bucket that houses the index buckets.
		_, err = meta.CreateBucket(indexesBucketName)
		if err != nil {
			return err
		}
		// Create the bucket that houses the index tips.
		_, err = meta.CreateBucket(indexTipsBucketName)
		if err != nil {
			return err
		}

		// Add the genesis block hash to height and height to hash
		// mappings to the index.
		dbPutBlockIndex(dbTx, b.bestNode.hash, b.bestNode.height)

		// Store the current best chain state into the database.
		dbPutBestState(dbTx, b.stateSnapshot, b.bestNode.workSum)

		// Store the genesis block into the database.
		return dbTx.StoreBlock(genesisBlock)
	})
	return err
}

// initChainState attempts to load and initialize the chain state from the
// database.  When the db does not yet contain any chain state, both it and the
// chain state are initialized to the genesis block.
func (b *BlockChain) initChainState() error {
	// Attempt to load the chain state from the database.
	var isStateInitialized bool
	err := b.db.View(func(dbTx database.Tx) error {
		// Fetch the stored chain state from the database metadata.
		// When it doesn't exist, it means the database hasn't been
		// initialized for use with chain yet, so break out now to allow
		// that to happen under a writable database transaction.
		serializedData := dbTx.Metadata().Get(chainStateKeyName)
		if serializedData == nil {
			return nil
		}
		log.Tracef("Serialized chain state: %x", serializedData)
		state, err := deserializeBestChainState(serializedData)
		if err != nil {
			return err
		}

		// Load the raw block bytes for the best block.
		blockBytes, err := dbTx.FetchBlock(&state.hash)
		if err != nil {
			return err
		}
		var block wire.MsgBlock
		err = block.Deserialize(bytes.NewReader(blockBytes))
		if err != nil {
			return err
		}

		// Create a new node and set it as both the root node and the
		// best node.  The preceding nodes will be loaded on demand as
		// needed.
		header := &block.Header
		node := newBlockNode(header, &state.hash, int32(state.height))
		node.inMainChain = true
		node.workSum = state.workSum
		b.bestNode = node
		b.root = node

		// Add the new node to the indices for faster lookups.
		prevHash := node.parentHash
		b.index[*node.hash] = node
		b.depNodes[*prevHash] = append(b.depNodes[*prevHash], node)

		// Initialize the state related to the best block.
		blockSize := uint64(len(blockBytes))
		numTxns := uint64(len(block.Transactions))
		b.stateSnapshot = newBestState(b.bestNode, blockSize, numTxns,
			state.totalTxns)

		isStateInitialized = true
		return nil
	})
	if err != nil {
		return err
	}

	// There is nothing more to do if the chain state was initialized.
	if isStateInitialized {
		return nil
	}

	// At this point the database has not already been initialized, so
	// initialize both it and the chain state to the genesis block.
	return b.createChainState()
}

// dbFetchUtxoEntry uses an existing database transaction to fetch all unspent
// outputs for the provided Bitcoin transaction hash.
//
// NOTE: This is currently temporary code and as a result returns all outputs
// instead of only the unspent one.  The spent outputs will have the spent flag
// set.
//
// When there is no entry for the provided hash, nil will be returned for the
// both the entry and the error.
func dbFetchUtxoEntry(dbTx database.Tx, hash *wire.ShaHash) (*UtxoEntry, error) {
	// Lookup the raw block location of the transaction.
	region, err := dbFetchTxIndexEntry(dbTx, hash)
	if err != nil {
		return nil, err
	}
	if region == nil {
		return nil, nil
	}

	// NOTE: This is only temporary and is essentially copied from the old
	// database code to quickly mock a working fake utxo set for testing
	// purposes.  Ultimately this will be replaced with a utxoset bucket and
	// compressed serialization along with a journal for handling reorgs
	// without any depending on the txindex.
	spendBucket := dbTx.Metadata().Bucket(spendBucketName)
	spendDataBytes := spendBucket.Get(hash[:])
	if spendDataBytes == nil {
		return nil, nil
	}
	numOutputs := byteOrder.Uint32(spendDataBytes[0:4])
	spendBits := spendDataBytes[4:]
	spendData := make([]bool, numOutputs)
	for i := uint32(0); i < numOutputs; i++ {
		byteIdx := i / 8
		byteOff := uint(i % 8)
		spendData[i] = (spendBits[byteIdx] & (byte(1) << byteOff)) != 0
	}

	// Lookup the block height for transaction.
	blockHeight, err := dbFetchHeightByHash(dbTx, region.Hash)
	if err != nil {
		return nil, err
	}

	// Load the raw transaction from the database.
	rawTx, err := dbTx.FetchBlockRegion(region)
	if err != nil {
		return nil, err
	}

	// Deserialize the raw transaction.
	var msgTx wire.MsgTx
	if err := msgTx.Deserialize(bytes.NewReader(rawTx)); err != nil {
		return nil, err
	}

	// Create a new utxo entry and add all outputs along with their spent
	// state.
	//
	// NOTE: As previously mentioned, this code is temporary until an actual
	// utxo set and reorg journal is implemented.  As a result, it adds all
	// of the referenced transaction's outputs instead of only the unspent
	// ones.
	entry := newUtxoEntry(msgTx.Version, IsCoinBaseTx(&msgTx), blockHeight)
	for txOutIdx, txOut := range msgTx.TxOut {
		packedIndex := uint32(len(entry.outputs))
		entry.outputs = append(entry.outputs, utxoOutput{
			spent:    spendData[txOutIdx],
			pkScript: txOut.PkScript,
			amount:   txOut.Value,
		})
		entry.sparseOutputs[uint32(txOutIdx)] = packedIndex
	}

	return entry, nil
}

// dbSpendBlock uses an existing database transaction to spend all Bitcoin
// transactions in the passed block.
func dbSpendBlock(dbTx database.Tx, block *btcutil.Block) error {
	spendBucket := dbTx.Metadata().Bucket(spendBucketName)
	for txIdx, tx := range block.Transactions() {
		// Add all of the outputs for this transaction as available
		// utxos.
		numOutputs := uint32(len(tx.MsgTx().TxOut))
		serializedData := make([]byte, 4+((numOutputs+7)/8))
		byteOrder.PutUint32(serializedData[:4], numOutputs)
		err := spendBucket.Put(tx.Sha()[:], serializedData)
		if err != nil {
			return err
		}

		// Loop through all of the transaction inputs (except for the
		// coinbase which has no inputs) and spend the referenced
		// output.
		if txIdx == 0 {
			continue
		}
		for _, txIn := range tx.MsgTx().TxIn {
			originHash := &txIn.PreviousOutPoint.Hash
			originIndex := txIn.PreviousOutPoint.Index
			spendDataBytes := spendBucket.Get(originHash[:])
			byteIdx := originIndex / 8
			byteOff := uint(originIndex % 8)
			spendDataBytes[byteIdx+4] |= (byte(1) << byteOff)
			err := spendBucket.Put(originHash[:], spendDataBytes)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// dbRemoveBlockSpends uses an existing database transaction to unspend all
// Bitcoin transactions in the passed block.
func dbRemoveBlockSpends(dbTx database.Tx, block *btcutil.Block) error {
	// Loop backwards through all transactions so everything is unspent in
	// reverse order.  This is necessary since transactions later in a block
	// can spend from previous ones.
	spendBucket := dbTx.Metadata().Bucket(spendBucketName)
	transactions := block.Transactions()
	for txIdx := len(transactions) - 1; txIdx > -1; txIdx-- {
		tx := transactions[txIdx]

		// NOTE: Technically this and the code which inserts the spend
		// data should have the ability to handle multiple entries per
		// tx hash, but since this code is going away before the branch
		// is merged to master, ignore that for now.
		err := spendBucket.Delete(tx.Sha()[:])
		if err != nil {
			return err
		}

		// Loop through all of the transaction inputs (except for the
		// coinbase which has no inputs) and spend the referenced
		// output.
		if txIdx == 0 {
			continue
		}
		for _, txIn := range tx.MsgTx().TxIn {
			originHash := &txIn.PreviousOutPoint.Hash
			originIndex := txIn.PreviousOutPoint.Index
			spendDataBytes := spendBucket.Get(originHash[:])
			byteIdx := originIndex / 8
			byteOff := uint(originIndex % 8)
			spendDataBytes[byteIdx+4] &= ^(byte(1) << byteOff)
			err := spendBucket.Put(originHash[:], spendDataBytes)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// dbFetchHeaderByHash uses an existing database transaction to retrieve the
// block header for the provided hash.
func dbFetchHeaderByHash(dbTx database.Tx, hash *wire.ShaHash) (*wire.BlockHeader, error) {
	headerBytes, err := dbTx.FetchBlockHeader(hash)
	if err != nil {
		return nil, err
	}

	var header wire.BlockHeader
	err = header.Deserialize(bytes.NewReader(headerBytes))
	if err != nil {
		return nil, err
	}

	return &header, nil
}

// dbFetchHeaderByHeight uses an existing database transaction to retrieve the
// block header for the provided height.
func dbFetchHeaderByHeight(dbTx database.Tx, height int32) (*wire.BlockHeader, error) {
	hash, err := dbFetchHashByHeight(dbTx, height)
	if err != nil {
		return nil, err
	}

	return dbFetchHeaderByHash(dbTx, hash)
}

// dbFetchBlockByHash uses an existing database transaction to retrieve the raw
// block for the provided hash, deserialize it, retrieve the appropriate height
// from the index, and return a btcutil.Block with the height set.
func dbFetchBlockByHash(dbTx database.Tx, hash *wire.ShaHash) (*btcutil.Block, error) {
	// First find the height associated with the provided hash in the index.
	blockHeight, err := dbFetchHeightByHash(dbTx, hash)
	if err != nil {
		return nil, err
	}

	// Load the raw block bytes from the database.
	blockBytes, err := dbTx.FetchBlock(hash)
	if err != nil {
		return nil, err
	}

	// Create the encapsulated block and set the height appropriately.
	block, err := btcutil.NewBlockFromBytes(blockBytes)
	if err != nil {
		return nil, err
	}
	block.SetHeight(blockHeight)

	return block, nil
}

// dbFetchBlockByHeight uses an existing database transaction to retrieve the
// raw block for the provided height, deserialize it, and return a btcutil.Block
// with the height set.
func dbFetchBlockByHeight(dbTx database.Tx, height int32) (*btcutil.Block, error) {
	// First find the hash associated with the provided height in the index.
	hash, err := dbFetchHashByHeight(dbTx, height)
	if err != nil {
		return nil, err
	}

	// Load the raw block bytes from the database.
	blockBytes, err := dbTx.FetchBlock(hash)
	if err != nil {
		return nil, err
	}

	// Create the encapsulated block and set the height appropriately.
	block, err := btcutil.NewBlockFromBytes(blockBytes)
	if err != nil {
		return nil, err
	}
	block.SetHeight(height)

	return block, nil
}

// dbMainChainHasBlock uses an existing database transaction to return whether
// or not the main chain contains the block identified by the provided hash.
func dbMainChainHasBlock(dbTx database.Tx, hash *wire.ShaHash) bool {
	hashIndex := dbTx.Metadata().Bucket(hashIndexBucketName)
	if hashIndex.Get(hash[:]) == nil {
		return false
	}

	return true
}

// BlockHeightByHash returns the height of the block with the given hash in the
// main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockHeightByHash(hash *wire.ShaHash) (int32, error) {
	var height int32
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		height, err = dbFetchHeightByHash(dbTx, hash)
		return err
	})
	return height, err
}

// BlockHashByHeight returns the hash of the block at the given height in the
// main chain.
//
// The database transaction parameter can be nil in which case a a new one will
// be used.
//
// This function is safe for concurrent access.  However, keep in mind that
// database transactions can't be shared across threads.
func (b *BlockChain) BlockHashByHeight(dbTx database.Tx, blockHeight int32) (*wire.ShaHash, error) {
	// Use existing database transaction if provided.
	if dbTx != nil {
		return dbFetchHashByHeight(dbTx, blockHeight)
	}

	var hash *wire.ShaHash
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		hash, err = dbFetchHashByHeight(dbTx, blockHeight)
		return err
	})
	return hash, err
}

// BlockByHeight returns the block at the given height in the main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockByHeight(blockHeight int32) (*btcutil.Block, error) {
	var block *btcutil.Block
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		block, err = dbFetchBlockByHeight(dbTx, blockHeight)
		return err
	})
	return block, err
}

// BlockByHash returns the block from the main chain with the given hash with
// the appropriate chain height set.
//
// This function is safe for concurrent access.
func (b *BlockChain) BlockByHash(hash *wire.ShaHash) (*btcutil.Block, error) {
	var block *btcutil.Block
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		block, err = dbFetchBlockByHash(dbTx, hash)
		return err
	})
	return block, err
}

// MainChainHasBlock returns whether or not the main chain contains the block
// identified by the provided hash.
//
// The database transaction parameter can be nil in which case a a new one will
// be used.
//
// This function is safe for concurrent access.
func (b *BlockChain) MainChainHasBlock(dbTx database.Tx, hash *wire.ShaHash) (bool, error) {
	if dbTx != nil {
		return dbMainChainHasBlock(dbTx, hash), nil
	}

	var res bool
	err := b.db.View(func(dbTx database.Tx) error {
		res = dbMainChainHasBlock(dbTx, hash)
		return nil
	})
	return res, err
}

// HeightRange returns a range of block hashes for the given start and end
// heights.  It is inclusive of the start height and exclusive of the end
// height.  The end height will be limited to the current main chain height.
//
// This function is safe for concurrent access.
func (b *BlockChain) HeightRange(startHeight, endHeight int32) ([]wire.ShaHash, error) {
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

	// There is nothing to do when the start and end heights are the same,
	// so return now to avoid the chain lock and a database transaction.
	if startHeight == endHeight {
		return nil, nil
	}

	// Grab a lock on the chain to prevent it from changing due to a reorg
	// while building the hashes.
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	// When the requested start height is after the most recent best chain
	// height, there is nothing to do.
	latestHeight := b.bestNode.height
	if startHeight > latestHeight {
		return nil, nil
	}

	// Limit the ending height to the latest height of the chain.
	if endHeight > latestHeight+1 {
		endHeight = latestHeight + 1
	}

	// Fetch as many as are available within the specified range.
	var hashList []wire.ShaHash
	err := b.db.View(func(dbTx database.Tx) error {
		hashes := make([]wire.ShaHash, 0, endHeight-startHeight)
		for i := startHeight; i < endHeight; i++ {
			hash, err := dbFetchHashByHeight(dbTx, i)
			if err != nil {
				return err
			}
			hashes = append(hashes, *hash)
		}

		// Set the list to be returned to the constructed list.
		hashList = hashes
		return nil
	})
	return hashList, err
}

// TxBlockRegion returns the block region for the provided transaction hash
// from the transaction index.  The block region can in turn be used to load the
// raw transaction bytes.  When there is no entry for the provided hash, nil
// will be returned for the both the entry and the error.
//
// The database transaction parameter can be nil in which case a a new one will
// be used.
//
// This function is safe for concurrent access.
func (b *BlockChain) TxBlockRegion(dbTx database.Tx, hash *wire.ShaHash) (*database.BlockRegion, error) {
	if dbTx != nil {
		return dbFetchTxIndexEntry(dbTx, hash)
	}

	var region *database.BlockRegion
	err := b.db.View(func(dbTx database.Tx) error {
		var err error
		region, err = dbFetchTxIndexEntry(dbTx, hash)
		return err
	})
	return region, err
}

// ExistsTx returns whether or not the provided transaction hash exists from the
// viewpoint of the end of the main chain.
//
// This function is safe for concurrent access.
func (b *BlockChain) ExistsTx(hash *wire.ShaHash) (bool, error) {
	var exists bool
	err := b.db.View(func(dbTx database.Tx) error {
		exists = dbHasTxIndexEntry(dbTx, hash)
		return nil
	})
	return exists, err
}
