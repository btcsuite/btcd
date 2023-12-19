// Copyright (c) 2023 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"container/list"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// mapSlice is a slice of maps for utxo entries.  The slice of maps are needed to
// guarantee that the map will only take up N amount of bytes.  As of v1.20, the
// go runtime will allocate 2^N + few extra buckets, meaning that for large N, we'll
// allocate a lot of extra memory if the amount of entries goes over the previously
// allocated buckets.  A slice of maps allows us to have a better control of how much
// total memory gets allocated by all the maps.
type mapSlice struct {
	// mtx protects against concurrent access for the map slice.
	mtx sync.Mutex

	// maps are the underlying maps in the slice of maps.
	maps []map[wire.OutPoint]*UtxoEntry

	// maxEntries is the maximum amount of elements that the map is allocated for.
	maxEntries []int

	// maxTotalMemoryUsage is the maximum memory usage in bytes that the state
	// should contain in normal circumstances.
	maxTotalMemoryUsage uint64
}

// length returns the length of all the maps in the map slice added together.
//
// This function is safe for concurrent access.
func (ms *mapSlice) length() int {
	ms.mtx.Lock()
	defer ms.mtx.Unlock()

	var l int
	for _, m := range ms.maps {
		l += len(m)
	}

	return l
}

// size returns the size of all the maps in the map slice added together.
//
// This function is safe for concurrent access.
func (ms *mapSlice) size() int {
	ms.mtx.Lock()
	defer ms.mtx.Unlock()

	var size int
	for _, num := range ms.maxEntries {
		size += calculateRoughMapSize(num, bucketSize)
	}

	return size
}

// get looks for the outpoint in all the maps in the map slice and returns
// the entry.  nil and false is returned if the outpoint is not found.
//
// This function is safe for concurrent access.
func (ms *mapSlice) get(op wire.OutPoint) (*UtxoEntry, bool) {
	ms.mtx.Lock()
	defer ms.mtx.Unlock()

	var entry *UtxoEntry
	var found bool

	for _, m := range ms.maps {
		entry, found = m[op]
		if found {
			return entry, found
		}
	}

	return nil, false
}

// put puts the outpoint and the entry into one of the maps in the map slice.  If the
// existing maps are all full, it will allocate a new map based on how much memory we
// have left over.  Leftover memory is calculated as:
// maxTotalMemoryUsage - (totalEntryMemory + mapSlice.size())
//
// This function is safe for concurrent access.
func (ms *mapSlice) put(op wire.OutPoint, entry *UtxoEntry, totalEntryMemory uint64) {
	ms.mtx.Lock()
	defer ms.mtx.Unlock()

	for i, maxNum := range ms.maxEntries {
		m := ms.maps[i]
		_, found := m[op]
		if found {
			// If the key is found, overwrite it.
			m[op] = entry
			return // Return as we were successful in adding the entry.
		}
		if len(m) >= maxNum {
			// Don't try to insert if the map already at max since
			// that'll force the map to allocate double the memory it's
			// currently taking up.
			continue
		}

		m[op] = entry
		return // Return as we were successful in adding the entry.
	}

	// We only reach this code if we've failed to insert into the map above as
	// all the current maps were full.  We thus make a new map and insert into
	// it.
	m := ms.makeNewMap(totalEntryMemory)
	m[op] = entry
}

// delete attempts to delete the given outpoint in all of the maps. No-op if the
// outpoint doesn't exist.
//
// This function is safe for concurrent access.
func (ms *mapSlice) delete(op wire.OutPoint) {
	ms.mtx.Lock()
	defer ms.mtx.Unlock()

	for i := 0; i < len(ms.maps); i++ {
		delete(ms.maps[i], op)
	}
}

// makeNewMap makes and appends the new map into the map slice.
//
// This function is NOT safe for concurrent access and must be called with the
// lock held.
func (ms *mapSlice) makeNewMap(totalEntryMemory uint64) map[wire.OutPoint]*UtxoEntry {
	// Get the size of the leftover memory.
	memSize := ms.maxTotalMemoryUsage - totalEntryMemory
	for _, maxNum := range ms.maxEntries {
		memSize -= uint64(calculateRoughMapSize(maxNum, bucketSize))
	}

	// Get a new map that's sized to house inside the leftover memory.
	// -1 on the returned value will make the map allocate half as much total
	// bytes.  This is done to make sure there's still room left for utxo
	// entries to take up.
	numMaxElements := calculateMinEntries(int(memSize), bucketSize+avgEntrySize)
	numMaxElements -= 1
	ms.maxEntries = append(ms.maxEntries, numMaxElements)
	ms.maps = append(ms.maps, make(map[wire.OutPoint]*UtxoEntry, numMaxElements))

	return ms.maps[len(ms.maps)-1]
}

// deleteMaps deletes all maps except for the first one which should be the biggest.
//
// This function is safe for concurrent access.
func (ms *mapSlice) deleteMaps() {
	ms.mtx.Lock()
	defer ms.mtx.Unlock()

	size := ms.maxEntries[0]
	ms.maxEntries = []int{size}
	ms.maps = ms.maps[:1]
}

const (
	// utxoFlushPeriodicInterval is the interval at which a flush is performed
	// when the flush mode FlushPeriodic is used.  This is used when the initial
	// block download is complete and it's useful to flush periodically in case
	// of unforseen shutdowns.
	utxoFlushPeriodicInterval = time.Minute * 5
)

// FlushMode is used to indicate the different urgency types for a flush.
type FlushMode uint8

const (
	// FlushRequired is the flush mode that means a flush must be performed
	// regardless of the cache state.  For example right before shutting down.
	FlushRequired FlushMode = iota

	// FlushPeriodic is the flush mode that means a flush can be performed
	// when it would be almost needed.  This is used to periodically signal when
	// no I/O heavy operations are expected soon, so there is time to flush.
	FlushPeriodic

	// FlushIfNeeded is the flush mode that means a flush must be performed only
	// if the cache is exceeding a safety threshold very close to its maximum
	// size.  This is used mostly internally in between operations that can
	// increase the cache size.
	FlushIfNeeded
)

// utxoCache is a cached utxo view in the chainstate of a BlockChain.
type utxoCache struct {
	db database.DB

	// maxTotalMemoryUsage is the maximum memory usage in bytes that the state
	// should contain in normal circumstances.
	maxTotalMemoryUsage uint64

	// cachedEntries keeps the internal cache of the utxo state.  The tfModified
	// flag indicates that the state of the entry (potentially) deviates from the
	// state in the database.  Explicit nil values in the map are used to
	// indicate that the database does not contain the entry.
	cachedEntries    mapSlice
	totalEntryMemory uint64 // Total memory usage in bytes.

	// Below fields are used to indicate when the last flush happened.
	lastFlushHash chainhash.Hash
	lastFlushTime time.Time
}

// newUtxoCache initiates a new utxo cache instance with its memory usage limited
// to the given maximum.
func newUtxoCache(db database.DB, maxTotalMemoryUsage uint64) *utxoCache {
	// While the entry isn't included in the map size, add the average size to the
	// bucket size so we get some leftover space for entries to take up.
	numMaxElements := calculateMinEntries(int(maxTotalMemoryUsage), bucketSize+avgEntrySize)
	numMaxElements -= 1

	log.Infof("Pre-alloacting for %d MiB: ", maxTotalMemoryUsage/(1024*1024)+1)

	m := make(map[wire.OutPoint]*UtxoEntry, numMaxElements)

	return &utxoCache{
		db:                  db,
		maxTotalMemoryUsage: maxTotalMemoryUsage,
		cachedEntries: mapSlice{
			maps:                []map[wire.OutPoint]*UtxoEntry{m},
			maxEntries:          []int{numMaxElements},
			maxTotalMemoryUsage: maxTotalMemoryUsage,
		},
	}
}

// totalMemoryUsage returns the total memory usage in bytes of the UTXO cache.
func (s *utxoCache) totalMemoryUsage() uint64 {
	// Total memory is the map size + the size that the utxo entries are
	// taking up.
	size := uint64(s.cachedEntries.size())
	size += s.totalEntryMemory

	return size
}

// fetchEntries returns the UTXO entries for the given outpoints.  The function always
// returns as many entries as there are outpoints and the returns entries are in the
// same order as the outpoints.  It returns nil if there is no entry for the outpoint
// in the UTXO set.
//
// The returned entries are NOT safe for concurrent access.
func (s *utxoCache) fetchEntries(outpoints []wire.OutPoint) ([]*UtxoEntry, error) {
	entries := make([]*UtxoEntry, len(outpoints))
	var (
		missingOps    []wire.OutPoint
		missingOpsIdx []int
	)
	for i := range outpoints {
		if entry, ok := s.cachedEntries.get(outpoints[i]); ok {
			entries[i] = entry
			continue
		}

		// At this point, we have missing outpoints.  Allocate them now
		// so that we never allocate if the cache never misses.
		if len(missingOps) == 0 {
			missingOps = make([]wire.OutPoint, 0, len(outpoints))
			missingOpsIdx = make([]int, 0, len(outpoints))
		}

		missingOpsIdx = append(missingOpsIdx, i)
		missingOps = append(missingOps, outpoints[i])
	}

	// Return early and don't attempt access the database if we don't have any
	// missing outpoints.
	if len(missingOps) == 0 {
		return entries, nil
	}

	// Fetch the missing outpoints in the cache from the database.
	dbEntries := make([]*UtxoEntry, len(missingOps))
	err := s.db.View(func(dbTx database.Tx) error {
		utxoBucket := dbTx.Metadata().Bucket(utxoSetBucketName)

		for i := range missingOps {
			entry, err := dbFetchUtxoEntry(dbTx, utxoBucket, missingOps[i])
			if err != nil {
				return err
			}

			dbEntries[i] = entry
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	// Add each of the entries to the UTXO cache and update their memory
	// usage.
	//
	// NOTE: When the fetched entry is nil, it is still added to the cache
	// as a miss; this prevents future lookups to perform the same database
	// fetch.
	for i := range dbEntries {
		s.cachedEntries.put(missingOps[i], dbEntries[i], s.totalEntryMemory)
		s.totalEntryMemory += dbEntries[i].memoryUsage()
	}

	// Fill in the entries with the ones fetched from the database.
	for i := range missingOpsIdx {
		entries[missingOpsIdx[i]] = dbEntries[i]
	}

	return entries, nil
}

// addTxOut adds the specified output to the cache if it is not provably
// unspendable.  When the cache already has an entry for the output, it will be
// overwritten with the given output.  All fields will be updated for existing
// entries since it's possible it has changed during a reorg.
func (s *utxoCache) addTxOut(outpoint wire.OutPoint, txOut *wire.TxOut, isCoinBase bool,
	blockHeight int32) error {

	// Don't add provably unspendable outputs.
	if txscript.IsUnspendable(txOut.PkScript) {
		return nil
	}

	entry := new(UtxoEntry)
	entry.amount = txOut.Value

	// Deep copy the script when the script in the entry differs from the one in
	// the txout.  This is required since the txout script is a subslice of the
	// overall contiguous buffer that the msg tx houses for all scripts within
	// the tx.  It is deep copied here since this entry may be added to the utxo
	// cache, and we don't want the utxo cache holding the entry to prevent all
	// of the other tx scripts from getting garbage collected.
	entry.pkScript = make([]byte, len(txOut.PkScript))
	copy(entry.pkScript, txOut.PkScript)

	entry.blockHeight = blockHeight
	entry.packedFlags = tfFresh | tfModified
	if isCoinBase {
		entry.packedFlags |= tfCoinBase
	}

	s.cachedEntries.put(outpoint, entry, s.totalEntryMemory)
	s.totalEntryMemory += entry.memoryUsage()

	return nil
}

// addTxOuts adds all outputs in the passed transaction which are not provably
// unspendable to the view.  When the view already has entries for any of the
// outputs, they are simply marked unspent.  All fields will be updated for
// existing entries since it's possible it has changed during a reorg.
func (s *utxoCache) addTxOuts(tx *btcutil.Tx, blockHeight int32) error {
	// Loop all of the transaction outputs and add those which are not
	// provably unspendable.
	isCoinBase := IsCoinBase(tx)
	prevOut := wire.OutPoint{Hash: *tx.Hash()}
	for txOutIdx, txOut := range tx.MsgTx().TxOut {
		// Update existing entries.  All fields are updated because it's
		// possible (although extremely unlikely) that the existing
		// entry is being replaced by a different transaction with the
		// same hash.  This is allowed so long as the previous
		// transaction is fully spent.
		prevOut.Index = uint32(txOutIdx)
		err := s.addTxOut(prevOut, txOut, isCoinBase, blockHeight)
		if err != nil {
			return err
		}
	}

	return nil
}

// addTxIn will add the given input to the cache if the previous outpoint the txin
// is pointing to exists in the utxo set.  The utxo that is being spent by the input
// will be marked as spent and if the utxo is fresh (meaning that the database on disk
// never saw it), it will be removed from the cache.
func (s *utxoCache) addTxIn(txIn *wire.TxIn, stxos *[]SpentTxOut) error {
	// Ensure the referenced utxo exists in the view.  This should
	// never happen unless there is a bug is introduced in the code.
	entries, err := s.fetchEntries([]wire.OutPoint{txIn.PreviousOutPoint})
	if err != nil {
		return err
	}
	if len(entries) != 1 || entries[0] == nil {
		return AssertError(fmt.Sprintf("missing input %v",
			txIn.PreviousOutPoint))
	}

	// Only create the stxo details if requested.
	entry := entries[0]
	if stxos != nil {
		// Populate the stxo details using the utxo entry.
		stxo := SpentTxOut{
			Amount:     entry.Amount(),
			PkScript:   entry.PkScript(),
			Height:     entry.BlockHeight(),
			IsCoinBase: entry.IsCoinBase(),
		}

		*stxos = append(*stxos, stxo)
	}

	// Mark the entry as spent.
	entry.Spend()

	// If an entry is fresh it indicates that this entry was spent before it could be
	// flushed to the database. Because of this, we can just delete it from the map of
	// cached entries.
	if entry.isFresh() {
		// If the entry is fresh, we will always have it in the cache.
		s.cachedEntries.delete(txIn.PreviousOutPoint)
		s.totalEntryMemory -= entry.memoryUsage()
	} else {
		// Can leave the entry to be garbage collected as the only purpose
		// of this entry now is so that the entry on disk can be deleted.
		entry = nil
		s.totalEntryMemory -= entry.memoryUsage()
	}

	return nil
}

// addTxIns will add the given inputs of the tx if it's not a coinbase tx and if
// the previous output that the input is pointing to exists in the utxo set.  The
// utxo that is being spent by the input will be marked as spent and if the utxo
// is fresh (meaning that the database on disk never saw it), it will be removed
// from the cache.
func (s *utxoCache) addTxIns(tx *btcutil.Tx, stxos *[]SpentTxOut) error {
	// Coinbase transactions don't have any inputs to spend.
	if IsCoinBase(tx) {
		return nil
	}

	for _, txIn := range tx.MsgTx().TxIn {
		err := s.addTxIn(txIn, stxos)
		if err != nil {
			return err
		}
	}

	return nil
}

// connectTransaction updates the cache by adding all new utxos created by the
// passed transaction and marking and/or removing all utxos that the transactions
// spend as spent.  In addition, when the 'stxos' argument is not nil, it will
// be updated to append an entry for each spent txout.  An error will be returned
// if the cache and the database does not contain the required utxos.
func (s *utxoCache) connectTransaction(
	tx *btcutil.Tx, blockHeight int32, stxos *[]SpentTxOut) error {

	err := s.addTxIns(tx, stxos)
	if err != nil {
		return err
	}

	// Add the transaction's outputs as available utxos.
	return s.addTxOuts(tx, blockHeight)
}

// connectTransactions updates the cache by adding all new utxos created by all
// of the transactions in the passed block, marking and/or removing all utxos
// the transactions spend as spent, and setting the best hash for the view to
// the passed block.  In addition, when the 'stxos' argument is not nil, it will
// be updated to append an entry for each spent txout.
func (s *utxoCache) connectTransactions(block *btcutil.Block, stxos *[]SpentTxOut) error {
	for _, tx := range block.Transactions() {
		err := s.connectTransaction(tx, block.Height(), stxos)
		if err != nil {
			return err
		}
	}

	return nil
}

// writeCache writes all the entries that are cached in memory to the database atomically.
func (s *utxoCache) writeCache(dbTx database.Tx, bestState *BestState) error {
	// Update commits and flushes the cache to the database.
	// NOTE: The database has its own cache which gets atomically written
	// to leveldb.
	utxoBucket := dbTx.Metadata().Bucket(utxoSetBucketName)
	for i := range s.cachedEntries.maps {
		for outpoint, entry := range s.cachedEntries.maps[i] {
			switch {
			// If the entry is nil or spent, remove the entry from the database
			// and the cache.
			case entry == nil || entry.IsSpent():
				err := dbDeleteUtxoEntry(utxoBucket, outpoint)
				if err != nil {
					return err
				}

			// No need to update the cache if the entry was not modified.
			case !entry.isModified():
			default:
				// Entry is fresh and needs to be put into the database.
				err := dbPutUtxoEntry(utxoBucket, outpoint, entry)
				if err != nil {
					return err
				}
			}

			delete(s.cachedEntries.maps[i], outpoint)
		}
	}
	s.cachedEntries.deleteMaps()
	s.totalEntryMemory = 0

	// When done, store the best state hash in the database to indicate the state
	// is consistent until that hash.
	err := dbPutUtxoStateConsistency(dbTx, &bestState.Hash)
	if err != nil {
		return err
	}

	// The best state is the new last flush hash.
	s.lastFlushHash = bestState.Hash
	s.lastFlushTime = time.Now()

	return nil
}

// flush flushes the UTXO state to the database if a flush is needed with the given flush mode.
//
// This function MUST be called with the chain state lock held (for writes).
func (s *utxoCache) flush(dbTx database.Tx, mode FlushMode, bestState *BestState) error {
	var threshold uint64
	switch mode {
	case FlushRequired:
		threshold = 0

	case FlushIfNeeded:
		// If we performed a flush in the current best state, we have nothing to do.
		if bestState.Hash == s.lastFlushHash {
			return nil
		}

		threshold = s.maxTotalMemoryUsage

	case FlushPeriodic:
		// If the time since the last flush is over the periodic interval,
		// force a flush.  Otherwise just flush when the cache is full.
		if time.Since(s.lastFlushTime) > utxoFlushPeriodicInterval {
			threshold = 0
		} else {
			threshold = s.maxTotalMemoryUsage
		}
	}

	if s.totalMemoryUsage() >= threshold {
		// Add one to round up the integer division.
		totalMiB := s.totalMemoryUsage() / ((1024 * 1024) + 1)
		log.Infof("Flushing UTXO cache of %d MiB with %d entries to disk. For large sizes, "+
			"this can take up to several minutes...", totalMiB, s.cachedEntries.length())

		return s.writeCache(dbTx, bestState)
	}

	return nil
}

// FlushUtxoCache flushes the UTXO state to the database if a flush is needed with the
// given flush mode.
//
// This function is safe for concurrent access.
func (b *BlockChain) FlushUtxoCache(mode FlushMode) error {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()

	return b.db.Update(func(dbTx database.Tx) error {
		return b.utxoCache.flush(dbTx, mode, b.BestSnapshot())
	})
}

// InitConsistentState checks the consistency status of the utxo state and
// replays blocks if it lags behind the best state of the blockchain.
//
// It needs to be ensured that the chainView passed to this method does not
// get changed during the execution of this method.
func (b *BlockChain) InitConsistentState(tip *blockNode, interrupt <-chan struct{}) error {
	s := b.utxoCache

	// Load the consistency status from the database.
	var statusBytes []byte
	s.db.View(func(dbTx database.Tx) error {
		statusBytes = dbFetchUtxoStateConsistency(dbTx)
		return nil
	})

	// If no status was found, the database is old and didn't have a cached utxo
	// state yet. In that case, we set the status to the best state and write
	// this to the database.
	if statusBytes == nil {
		err := s.db.Update(func(dbTx database.Tx) error {
			return dbPutUtxoStateConsistency(dbTx, &tip.hash)
		})

		// Set the last flush hash as it's the default value of 0s.
		s.lastFlushHash = tip.hash

		return err
	}

	statusHash, err := chainhash.NewHash(statusBytes)
	if err != nil {
		return err
	}

	// If state is consistent, we are done.
	if statusHash.IsEqual(&tip.hash) {
		log.Debugf("UTXO state consistent at (%d:%v)", tip.height, tip.hash)

		// The last flush hash is set to the default value of all 0s. Set
		// it to the tip since we checked it's consistent.
		s.lastFlushHash = tip.hash

		return nil
	}

	lastFlushNode := b.index.LookupNode(statusHash)
	log.Infof("Reconstructing UTXO state after an unclean shutdown. The UTXO state is "+
		"consistent at block %s (%d) but the chainstate is at block %s (%d),  This may "+
		"take a long time...", statusHash.String(), lastFlushNode.height,
		tip.hash.String(), tip.height)

	// Even though this should always be true, make sure the fetched hash is in
	// the best chain.
	fork := b.bestChain.FindFork(lastFlushNode)
	if fork == nil {
		return AssertError(fmt.Sprintf("last utxo consistency status contains "+
			"hash that is not in best chain: %v", statusHash))
	}

	// We never disconnect blocks as they cannot be inconsistent during a reorganization.
	// This is because The cache is flushed before the reorganization begins and the utxo
	// set at each block disconnect is written atomically to the database.
	node := lastFlushNode

	// We replay the blocks from the last consistent state up to the best
	// state. Iterate forward from the consistent node to the tip of the best
	// chain.
	attachNodes := list.New()
	for n := tip; n.height >= 0; n = n.parent {
		if n == fork {
			break
		}
		attachNodes.PushFront(n)
	}

	for e := attachNodes.Front(); e != nil; e = e.Next() {
		node = e.Value.(*blockNode)

		var block *btcutil.Block
		err := s.db.View(func(dbTx database.Tx) error {
			block, err = dbFetchBlockByNode(dbTx, node)
			if err != nil {
				return err
			}

			return err
		})
		if err != nil {
			return err
		}

		err = b.utxoCache.connectTransactions(block, nil)
		if err != nil {
			return err
		}

		// Flush the utxo cache if needed.  This will in turn update the
		// consistent state to this block.
		err = s.db.Update(func(dbTx database.Tx) error {
			return s.flush(dbTx, FlushIfNeeded, &BestState{Hash: node.hash, Height: node.height})
		})
		if err != nil {
			return err
		}

		if interruptRequested(interrupt) {
			log.Warn("UTXO state reconstruction interrupted")

			return errInterruptRequested
		}
	}
	log.Debug("UTXO state reconstruction done")

	// Set the last flush hash as it's the default value of 0s.
	s.lastFlushHash = tip.hash
	s.lastFlushTime = time.Now()

	return nil
}

// flushNeededAfterPrune returns true if the utxo cache needs to be flushed after a prune
// of the block storage.  In the case of an unexpected shutdown, the utxo cache needs
// to be reconstructed from where the utxo cache was last flushed.  In order for the
// utxo cache to be reconstructed, we always need to have the blocks since the utxo cache
// flush last happened.
//
// Example: if the last flush hash was at height 100 and one of the deleted blocks was at
// height 98, this function will return true.
func (b *BlockChain) flushNeededAfterPrune(deletedBlockHashes []chainhash.Hash) (bool, error) {
	lastFlushHeight, err := b.BlockHeightByHash(&b.utxoCache.lastFlushHash)
	if err != nil {
		return false, err
	}

	// Loop through all the block hashes and find out what the highest block height
	// among the deleted hashes is.
	highestDeletedHeight := int32(-1)
	for _, deletedBlockHash := range deletedBlockHashes {
		height, err := b.BlockHeightByHash(&deletedBlockHash)
		if err != nil {
			return false, err
		}

		if height > highestDeletedHeight {
			highestDeletedHeight = height
		}
	}

	return highestDeletedHeight >= lastFlushHeight, nil
}
