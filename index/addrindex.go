// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package index

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sync"

	"github.com/btcsuite/btcd/blockchain"
	database "github.com/btcsuite/btcd/database2"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

var (
	// errUnsupportedAddressType is an error that is used to signal an
	// unsupported address type has been used.
	errUnsupportedAddressType = errors.New("address type is not supported " +
		"by the address index")

	// addrIndexName is the name of the db bucket used to house the
	// address index.
	addrIndexName = "txbyaddr"

	// byteOrder is the preferred byte order used for serializing numeric
	// fields for storage in the database.
	byteOrder = binary.LittleEndian
)

const (
	// Maximum number of txs that are stored in level 0 of an address.
	// Subsequent levels store maximum double of the previous level.
	firstLevelMaxSize = 8

	// Size of an address key: 1 byte of address type plus 20 bytes of
	// hash160
	addrKeySize = 1 + 20

	// Size of a level key: one addrKey + 1 byte for level number
	levelKeySize = addrKeySize + 1

	// Size of a transaction entry
	txEntrySize = 4 + 4 + 4
)

type addrKey [addrKeySize]byte
type levelKey [levelKeySize]byte

// -----------------------------------------------------------------------------
// The address index maps addresses referenced in the blockchain to a list of
// all the transactions involving that address. Transactions are stored
// according to order of appearance in the blockchain: first by block height and
// then by offset inside the block.
//
// Every address has one or more entries in the addrindex bucket, identified by
// a 'level' starting from 0. Level 0 holds maximum maxEntriesFirstLevel txs,
// and next levels hold maximum twice as much as the previous level.
//
// When inserting a new tx, it's apended into level 0. If level 0 becomes full,
// the whole data from level 0 is appended to level 1 and level 0 becomes empty.
// In this case level 1 may also become full, in this case it's appended to
// level 2 and emptied, and so on.
//
// Lower levels contain newer txs, inside each level txs are ordered from old
// to new.
//
// The intent of this approach is to get a balance between storing one entry
// per transaction (wastes space because the same address hash is stored
// per every tx as a key) and storing one entry per address (most space
// efficient, but indexing cost grows quadratically with the number of txs in
// with the same address). Insertion cost is amortized logarithmic, and
// retrieval is fast too because the number of levels grows logarithmically.
// This is similar to how leveldb works internally.
//
// The serialized key format is:
//
//   <addr type><addr hash><level>
//
//   Field           Type      Size
//   Addr type       uint8     1 byte
//   Addr hash       hash160   20 bytes
//   Level           uint8     1 byte
//   Total: 22 bytes
//
// The serialized value format is:
//
//   <block height><start offset><tx length>,...
//
//   Field           Type      Size
//   block height    uint32    4 bytes
//   start offset    uint32    4 bytes
//   tx length       uint32    4 bytes
//   Total: 12 bytes per indexed tx
//
// -----------------------------------------------------------------------------

// addrToKey converts known address types to an addrindex key (type byte +
// the hash160, see above for details).
// An error is returned for unsupported types.
func addrToKey(addr btcutil.Address) (*addrKey, error) {
	switch addr := addr.(type) {
	case *btcutil.AddressPubKeyHash:
		var res addrKey
		res[0] = 0
		copy(res[1:], addr.Hash160()[:])
		return &res, nil

	case *btcutil.AddressScriptHash:
		var res addrKey
		res[0] = 1
		copy(res[1:], addr.Hash160()[:])
		return &res, nil

	case *btcutil.AddressPubKey:
		var res addrKey
		res[0] = 0
		copy(res[1:], addr.AddressPubKeyHash().Hash160()[:])
		return &res, nil
	}

	return nil, errUnsupportedAddressType
}

func addrKeyToLevelKey(key *addrKey, level uint8) *levelKey {
	var res levelKey
	copy(res[:], key[:])
	res[addrKeySize] = level
	return &res
}

type addrIndexTxEntry struct {
	blockHeight int32
	txLoc       wire.TxLoc
}

// serializeAddrIndexEntry serializes a tx entry. The format is described in
// detail above.
func serializeAddrIndexTxEntry(e addrIndexTxEntry) []byte {
	serializedData := make([]byte, txEntrySize)
	offset := 0
	byteOrder.PutUint32(serializedData[offset:], uint32(e.blockHeight))
	offset += 4
	byteOrder.PutUint32(serializedData[offset:], uint32(e.txLoc.TxStart))
	offset += 4
	byteOrder.PutUint32(serializedData[offset:], uint32(e.txLoc.TxLen))

	return serializedData
}

func deserializeAddrIndexTxEntry(serializedData []byte) (addrIndexTxEntry, error) {
	var res addrIndexTxEntry
	offset := 0
	res.blockHeight = int32(byteOrder.Uint32(serializedData[offset:]))
	offset += 4
	res.txLoc.TxStart = int(byteOrder.Uint32(serializedData[offset:]))
	offset += 4
	res.txLoc.TxStart = int(byteOrder.Uint32(serializedData[offset:]))

	return res, nil
}

// dbAppendToAddrIndexEntry uses an existing database transaction to update the
// address index given the provided values.  When there is already an entry for
// existing hash, a new record will be added.
func dbAppendToAddrIndexEntry(bucket database.Bucket, key *addrKey, entry addrIndexTxEntry) error {
	// Serialize the entry to append
	dataToAppend := serializeAddrIndexTxEntry(entry)

	// Start with level 0, with the initial max size
	level := uint8(0)
	maxLevelSize := firstLevelMaxSize

	// Loop over all levels.
	for true {
		// Get the level key for the current level.
		levelKey := addrKeyToLevelKey(key, level)
		// Get the old data. If it does not exist, it will return nil,
		// which is convenient because it's treated as a zero-length slice.
		oldData := bucket.Get(levelKey[:])

		// Concat oldData and dataToAppend into newData.
		newData := make([]byte, len(oldData)+len(dataToAppend))
		copy(newData, oldData)
		copy(newData[len(oldData):], dataToAppend)

		// Check new data length against the maximum.
		if len(newData) <= maxLevelSize*txEntrySize {
			// If it fits, save it and we're done.
			err := bucket.Put(levelKey[:], newData)
			if err != nil {
				return err
			}
			break
		} else {
			// If it doesn't fit, clear it...
			err := bucket.Put(levelKey[:], []byte{})
			if err != nil {
				return err
			}
			// and save everything to append into a higher level.
			dataToAppend = newData
		}
		level++
		maxLevelSize *= 2
	}

	return nil
}

// dbAppendToAddrIndexEntry uses an existing database transaction to update the
// address index given the provided values.  When there is already an entry for
// existing hash, a new record will be added.
func dbRemoveFromAddrIndexEntry(bucket database.Bucket, key *addrKey, count int) error {
	// Start with level 0, with the initial max size
	level := uint8(0)

	// Loop over levels until we have no more entries to remove.
	for count > 0 {
		// Get the level key for the current level.
		levelKey := addrKeyToLevelKey(key, level)
		// Get the old data.
		levelData := bucket.Get(levelKey[:])

		// Calculate how many entries to remove.
		levelCount := len(levelData) / txEntrySize
		removeCount := levelCount
		if removeCount > count {
			removeCount = count
		}

		levelData = levelData[:len(levelData)-removeCount*txEntrySize]

		count -= removeCount
	}

	return nil
}

// BlockRegion specifies a particular region of a block identified by the
// specified hash, given an offset and length.
type addrIndexResult struct {
	Height int32
	Offset uint32
	Len    uint32
}

// Returns block regions for all referenced transactions and the number of
// entries skipped since it could have been less in the case there are less
// total entries than the requested number of entries to skip.
// This function returns block heights instead of hashes to avoid a dependency
// on BlockChain. See fetchAddrIndexEntries below for a version that
// returns block hashes.
func dbFetchAddrIndexEntries(bucket database.Bucket, key *addrKey, numToSkip, numRequested int, reverse bool) ([]addrIndexResult, int, error) {
	// Load all
	level := uint8(0)
	var serializedData []byte

	// If reverse is false, we need to fetch all the levels because numToSkip
	// and numRequested are counted from oldest transactions (highest level),
	// so we need to know the total count.
	// If reverse is true, they're counted from lowest level, so we can stop
	// fetching from database as soon as we have enough transactions.
	for !reverse || len(serializedData) < int(numToSkip+numRequested)*txEntrySize {
		levelData := bucket.Get(addrKeyToLevelKey(key, level)[:])
		if levelData == nil {
			// If we have no more levels, stop.
			break
		}
		// Append the new data to the beginning, since it's older data.
		serializedData = append(levelData, serializedData...)
		level++
	}

	// When the requested number of entries to skip is larger than the
	// number available, skip them all and return now with the actual number
	// skipped.
	numEntries := len(serializedData) / txEntrySize
	if numToSkip >= numEntries {
		return nil, numEntries, nil
	}

	// Nothing more to do there are no requested entries.
	if numRequested == 0 {
		return nil, numToSkip, nil
	}

	// Limit the number to load based on the number of available entries,
	// the number to skip, and the number requested.
	numToLoad := numEntries - numToSkip
	if numToLoad > numRequested {
		numToLoad = numRequested
	}

	// Start the offset after all skipped entries and load the calculated
	// number.
	results := make([]addrIndexResult, numToLoad)
	for i := 0; i < numToLoad; i++ {
		var offset int
		// Calculate the offset we need to read from, according to the
		// reverse flag.
		if reverse {
			offset = (numEntries - numToSkip - i - 1) * txEntrySize
		} else {
			offset = (numToSkip + i) * txEntrySize
		}

		// Deserialize and populate the result.
		result := &results[i]
		result.Height = int32(byteOrder.Uint32(serializedData[offset:]))
		offset += 4
		result.Offset = byteOrder.Uint32(serializedData[offset:])
		offset += 4
		result.Len = byteOrder.Uint32(serializedData[offset:])
		offset += 4
	}

	return results, numToSkip, nil
}

// FetchBlockRegionsForAddr returns block regions and heights for transactions
// involving the given addr. It also returns how many txs were actually skipped,
// since it may be lower than numToSkip if there are not enough txs.
func (idx *AddrIndex) FetchBlockRegionsForAddr(dbTx database.Tx, addr btcutil.Address, numToSkip, numRequested int, reverse bool) ([]database.BlockRegion, []int32, int, error) {
	key, err := addrToKey(addr)
	if err != nil {
		return nil, nil, 0, err
	}
	bucket := idx.chain.IndexBucket(dbTx, addrIndexName)
	res, skipped, err := dbFetchAddrIndexEntries(bucket, key, numToSkip, numRequested, reverse)
	if err != nil {
		return nil, nil, 0, err
	}

	regions := make([]database.BlockRegion, len(res))
	heights := make([]int32, len(res))
	for i := 0; i < len(res); i++ {
		// Fetch the hash associated with the height.
		regions[i].Hash, err = idx.chain.BlockHashByHeight(dbTx, res[i].Height)
		if err != nil {
			return nil, nil, 0, err
		}
		regions[i].Len = res[i].Len
		regions[i].Offset = res[i].Offset
		heights[i] = res[i].Height
	}

	return regions, heights, skipped, nil
}

// writeIndexData represents the address index data to be written from one block
type writeIndexData map[addrKey][]wire.TxLoc

// indexScriptPubKey indexes the tx as relevant for all the addresses found in
// the SPK.
func (idx *AddrIndex) indexScriptPubKey(idxData writeIndexData, scriptPubKey []byte, loc wire.TxLoc) error {
	// Any errors are intentionally ignored: if the tx is non-standard, it
	// simply won't be indexed
	_, addrs, _, _ := txscript.ExtractPkScriptAddrs(scriptPubKey, idx.chain.Params())

	for _, addr := range addrs {
		addrKey, err := addrToKey(addr)
		if err != nil {
			// If the address type is not supported, just ignore it.
			continue
		}

		// Avoid inserting the tx twice. The txs are indexed serially: if an
		// address appears multiple times in a tx, all the appearances will be
		// indexed in a row, so checking the last txLoc indexed for equality is
		// enough.
		if len(idxData[*addrKey]) != 0 && idxData[*addrKey][len(idxData[*addrKey])-1] == loc {
			continue
		}
		idxData[*addrKey] = append(idxData[*addrKey], loc)
	}
	return nil
}

// indexBlockAddrs returns a populated index of the all the transactions in the
// passed block based on the addresses involved in each transaction.
func (idx *AddrIndex) indexBlockAddrs(dbTx database.Tx, blk *btcutil.Block, view *blockchain.UtxoViewpoint) (writeIndexData, error) {
	addrIndex := make(writeIndexData)
	txLocs, err := blk.TxLoc()
	if err != nil {
		return nil, err
	}
	for txIdx, tx := range blk.Transactions() {
		// Tx's offset and length in the block.
		locInBlock := txLocs[txIdx]

		// Coinbases don't have any inputs.
		if !blockchain.IsCoinBase(tx) {
			// Index the SPK's of each input's previous outpoint
			// transaction.
			for _, txIn := range tx.MsgTx().TxIn {
				// Lookup and fetch the referenced output's tx.
				prevOut := txIn.PreviousOutPoint

				var pkScript []byte
				if view != nil {
					// If we have an UtxoViewpoint, fetch the prevout from there.
					// The UtxoViewpoint is guaranteed to have all the prevouts
					// from the block being connected/disconnected in memory.
					pkScript = view.LookupEntry(&prevOut.Hash).PkScriptByIndex(prevOut.Index)
				} else {
					if idx.txIndex == nil {
						return nil, fmt.Errorf("Transaction-by-address index does not support catchup without a transaction-by-hash index.")
					}

					// If not, look up the location of the transaction using
					// the txindex.
					blockRegion, err := idx.txIndex.TxBlockRegion(dbTx, &prevOut.Hash)
					if err != nil {
						log.Errorf("Error fetching tx %v: %v",
							prevOut.Hash, err)
						return nil, err
					}
					if blockRegion == nil {
						return nil, fmt.Errorf("transaction %v not found",
							prevOut.Hash)
					}

					// Load the raw transaction bytes from the database.
					txBytes, err := dbTx.FetchBlockRegion(blockRegion)
					if err != nil {
						log.Errorf("Error fetching tx %v: %v",
							prevOut.Hash, err)
						return nil, err
					}

					// Deserialize the transaction
					var prevOutTx wire.MsgTx
					err = prevOutTx.Deserialize(bytes.NewReader(txBytes))

					inputOutPoint := prevOutTx.TxOut[prevOut.Index]
					pkScript = inputOutPoint.PkScript
				}
				idx.indexScriptPubKey(addrIndex, pkScript, locInBlock)
			}
		}

		for _, txOut := range tx.MsgTx().TxOut {
			idx.indexScriptPubKey(addrIndex, txOut.PkScript, locInBlock)
		}
	}
	return addrIndex, nil
}

// appendAddrIndexDataForBlock uses an existing database transaction to write
// the addrindex data from one block to the database. The addrindex tip before
// calling this function should be the block previous to the one being written.
func dbAppendAddrIndexDataForBlock(bucket database.Bucket, blk *btcutil.Block, data writeIndexData) error {
	for addr, txs := range data {
		for _, tx := range txs {
			err := dbAppendToAddrIndexEntry(bucket, &addr, addrIndexTxEntry{blockHeight: blk.Height(), txLoc: tx})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// removeAddrIndexDataForBlock uses an existing database transaction to erase
// the addrindex data from one block to the database. The addrindex tip before
// calling this function should be the block being erased, after the function
// finishes the tip will be the block before.
func dbRemoveAddrIndexDataForBlock(bucket database.Bucket, blk *btcutil.Block, data writeIndexData) error {
	for addr, txs := range data {
		err := dbRemoveFromAddrIndexEntry(bucket, &addr, len(txs))
		if err != nil {
			return err
		}
	}
	return nil
}

// AddrIndex indexes transactions by all the addresses appearing in them.
type AddrIndex struct {
	chain   *blockchain.BlockChain
	txIndex *TxIndex

	// mempool stores for each address a map of txhash to tx.
	mempool map[addrKey]map[wire.ShaHash]*btcutil.Tx
	// mempoolRemove stores for each txid the list of addresses it's indexed in.
	// It's used for removing txs from the mempool index.
	mempoolRemove map[wire.ShaHash]map[addrKey]struct{}
	mpLock        sync.RWMutex
}

// Init initializes the AddrIndex with a given blockchain.
func (idx *AddrIndex) Init(b *blockchain.BlockChain) {
	idx.chain = b
}

// Name returns the index's name. It should be unique per index type.
// It is used to identify
func (idx *AddrIndex) Name() string {
	return addrIndexName
}

// Version returns the index's version.
func (idx *AddrIndex) Version() int32 {
	return 1
}

// Create initializes the necessary data structures for the index
// in the database using an existing transaction. It creates buckets and
// fills them with initial data as needed.
func (idx *AddrIndex) Create(dbTx database.Tx, bucket database.Bucket) error {
	// Nothing needs to be inserted.
	return nil
}

// ConnectBlock indexes the given block using an existing database
// transaction.
func (idx *AddrIndex) ConnectBlock(dbTx database.Tx, bucket database.Bucket, block *btcutil.Block, view *blockchain.UtxoViewpoint) error {
	data, err := idx.indexBlockAddrs(dbTx, block, view)
	if err != nil {
		return err
	}

	return dbAppendAddrIndexDataForBlock(bucket, block, data)
}

// DisconnectBlock de-indexes the given block using an existing database
// transaction.
func (idx *AddrIndex) DisconnectBlock(dbTx database.Tx, bucket database.Bucket, block *btcutil.Block, view *blockchain.UtxoViewpoint) error {
	data, err := idx.indexBlockAddrs(dbTx, block, view)
	if err != nil {
		return err
	}

	return dbRemoveAddrIndexDataForBlock(bucket, block, data)
}

// FetchMempoolTxsForAddr returns all the transactions in the mempool that
// involve the given address.
func (idx *AddrIndex) FetchMempoolTxsForAddr(addr btcutil.Address) ([]*btcutil.Tx, error) {
	idx.mpLock.RLock()
	defer idx.mpLock.RUnlock()

	key, err := addrToKey(addr)
	if err != nil {
		// If the addr type is not supported, ignore it.
		return nil, nil
	}

	txMap := idx.mempool[*key]
	res := make([]*btcutil.Tx, len(txMap))
	i := 0
	for _, tx := range txMap {
		res[i] = tx
		i++
	}
	return res, nil
}

// AddMempoolTx is called when a tx is added to the mempool, it gets added
// to the index data structures if needed.
func (idx *AddrIndex) AddMempoolTx(tx *btcutil.Tx, utxoView *blockchain.UtxoViewpoint) error {
	idx.mpLock.Lock()
	defer idx.mpLock.Unlock()
	log.Debugf("Indexed mempool tx %v", tx.Sha())

	// Index addresses of all referenced previous output tx's.
	for _, txIn := range tx.MsgTx().TxIn {
		entry := utxoView.LookupEntry(&txIn.PreviousOutPoint.Hash)
		pkScript := entry.PkScriptByIndex(txIn.PreviousOutPoint.Index)
		idx.indexScriptAddressToTx(pkScript, tx)
	}

	// Index addresses of all created outputs.
	for _, txOut := range tx.MsgTx().TxOut {
		idx.indexScriptAddressToTx(txOut.PkScript, tx)
	}
	return nil
}

// indexScriptAddressToTx alters our address index by adding or removing the tx
// from the mempool addr index.
func (idx *AddrIndex) indexScriptAddressToTx(pkScript []byte, tx *btcutil.Tx) error {
	// Any errors are intentionally ignored: if the tx is non-standard, it
	// simply won't be indexed
	_, addresses, _, _ := txscript.ExtractPkScriptAddrs(pkScript,
		idx.chain.Params())

	for _, addr := range addresses {
		log.Debugf("Indexed mempool tx %v to addr %s", tx.Sha(), addr.EncodeAddress())

		key, err := addrToKey(addr)
		if err != nil {
			// If the addr type is not supported, ignore it.
			continue
		}

		if idx.mempool[*key] == nil {
			idx.mempool[*key] = make(map[wire.ShaHash]*btcutil.Tx)
		}
		idx.mempool[*key][*tx.Sha()] = tx
		if idx.mempoolRemove[*tx.Sha()] == nil {
			idx.mempoolRemove[*tx.Sha()] = make(map[addrKey]struct{})
		}
		idx.mempoolRemove[*tx.Sha()][*key] = struct{}{}
	}

	return nil
}

// RemoveMempoolTx is called when a tx is removed from the mempool, it gets
// removed from the index data structures if needed.
func (idx *AddrIndex) RemoveMempoolTx(tx *btcutil.Tx) error {
	idx.mpLock.Lock()
	defer idx.mpLock.Unlock()
	// When adding txs from the mempool, we also store in idx.mempoolRemove the
	// necessary info to undo the adding. This is because when removing a tx we
	// can't have access to the utxoview.
	for key := range idx.mempoolRemove[*tx.Sha()] {
		delete(idx.mempool[key], *tx.Sha())
		if len(idx.mempool[key]) == 0 {
			delete(idx.mempool, key)
		}
	}
	delete(idx.mempoolRemove, *tx.Sha())
	return nil
}

var _ blockchain.Index = (*AddrIndex)(nil)

// NewAddrIndex creates a new AddrIndex. The passed TxIndex will be used to
// fetch transactions by hash during catchup phase. It may be nil, in that case
// the returned AddrIndex will not support catchup.
func NewAddrIndex(txIndex *TxIndex) *AddrIndex {
	return &AddrIndex{
		txIndex:       txIndex,
		mempool:       make(map[addrKey]map[wire.ShaHash]*btcutil.Tx),
		mempoolRemove: make(map[wire.ShaHash]map[addrKey]struct{}),
	}
}
