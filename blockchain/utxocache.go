// Copyright (c) 2015-2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"container/list"
	"fmt"
	"sort"
	"sync"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

const (
	// The utxo writes big amounts of data to the database.  In order to limit
	// the size of individual database transactions, it works in batches.

	// utxoBatchSizeEntries is the maximum number of utxo entries to be written
	// in a single transaction.
	utxoBatchSizeEntries = 200000

	// utxoBatchSizeBlocks is the maximum number of blocks to be processed in a
	// single transaction.
	utxoBatchSizeBlocks = 50

	// utxoFlushPeriodicThreshold is the threshold percentage at which a flush is
	// performed when the flush mode FlushPeriodic is used.
	utxoFlushPeriodicThreshold = 90

	// This value is calculated by running the following on a 64-bit system:
	//   unsafe.Sizeof(UtxoEntry{})
	baseEntrySize = uint64(40)

	// This value is calculated by running the following on a 64-bit system:
	//   unsafe.Sizeof(wire.OutPoint{})
	outpointSize = uint64(36)

	// pubKeyHashLen is the length of a P2PKH script.
	pubKeyHashLen = 25
)

// txoFlags is a bitmask defining additional information and state for a
// transaction output in a utxo view.
type txoFlags uint8

const (
	// tfCoinBase indicates that a txout was contained in a coinbase tx.
	tfCoinBase txoFlags = 1 << iota

	// tfSpent indicates that a txout is spent.
	tfSpent

	// tfModified indicates that a txout has been modified since it was
	// loaded.
	tfModified

	// tfFresh indicates that the entry is fresh.  This means that the parent
	// view never saw this entry.  Note that tfFresh is a performance
	// optimization with which we can erase entries that are fully spent if we
	// know we do not need to commit them.  It is always safe to not mark
	// tfFresh if that condition is not guaranteed.
	tfFresh
)

// UtxoEntry houses details about an individual transaction output in a utxo
// view such as whether or not it was contained in a coinbase tx, the height of
// the block that contains the tx, whether or not it is spent, its public key
// script, and how much it pays.
type UtxoEntry struct {
	// NOTE: Additions, deletions, or modifications to the order of the
	// definitions in this struct should not be changed without considering
	// how it affects alignment on 64-bit platforms.  The current order is
	// specifically crafted to result in minimal padding.  There will be a
	// lot of these in memory, so a few extra bytes of padding adds up.
	// Any changes here should also be reflected in the memoryUsage() function.

	amount      int64
	pkScript    []byte // The public key script for the output.
	blockHeight int32  // Height of block containing tx.

	// packedFlags contains additional info about output such as whether it
	// is a coinbase, whether it is spent, and whether it has been modified
	// since it was loaded.  This approach is used in order to reduce memory
	// usage since there will be a lot of these in memory.
	packedFlags txoFlags
}

// IsCoinBase returns whether or not the output was contained in a coinbase
// transaction.
func (entry *UtxoEntry) IsCoinBase() bool {
	return entry.packedFlags&tfCoinBase == tfCoinBase
}

// IsSpent returns whether or not the output has been spent based upon the
// current state of the unspent transaction output view it was obtained from.
func (entry *UtxoEntry) IsSpent() bool {
	return entry.packedFlags&tfSpent == tfSpent
}

// isModified returns whether or not the output has been modified since it was
// loaded.
func (entry *UtxoEntry) isModified() bool {
	return entry.packedFlags&tfModified == tfModified
}

// isFresh returns whether or not it's certain the output has never previously
// been stored in the database.
func (entry *UtxoEntry) isFresh() bool {
	return entry.packedFlags&tfFresh == tfFresh
}

// BlockHeight returns the height of the block containing the output.
func (entry *UtxoEntry) BlockHeight() int32 {
	return entry.blockHeight
}

// Amount returns the amount of the output.
func (entry *UtxoEntry) Amount() int64 {
	return entry.amount
}

// PkScript returns the public key script for the output.
func (entry *UtxoEntry) PkScript() []byte {
	return entry.pkScript
}

// memoryUsage returns the memory usage in bytes of the UTXO entry.
// It returns 0 for the nil element.
func (entry *UtxoEntry) memoryUsage() uint64 {
	if entry == nil {
		return 0
	}

	return baseEntrySize + uint64(len(entry.pkScript))
}

// Spend marks the output as spent.  Spending an output that is already spent
// has no effect.
func (entry *UtxoEntry) Spend() {
	// Nothing to do if the output is already spent.
	if entry.IsSpent() {
		return
	}

	// Mark the output as spent and modified.
	entry.packedFlags |= tfSpent | tfModified
}

// Clone returns a shallow copy of the utxo entry.
func (entry *UtxoEntry) Clone() *UtxoEntry {
	if entry == nil {
		return nil
	}

	return &UtxoEntry{
		amount:      entry.amount,
		pkScript:    entry.pkScript,
		blockHeight: entry.blockHeight,
		packedFlags: entry.packedFlags,
	}
}

// utxoView is a common interface for structures that implement a UTXO view.
type utxoView interface {
	// getEntry tries to get an entry from the view.  If the entry is not
	// in the view, both the returned entry and the error are nil.
	getEntry(outpoint wire.OutPoint) (*UtxoEntry, error)

	// addEntry adds a new entry to the view.  Set overwrite to true if
	// this entry should overwrite any existing entry for the same
	// outpoint.
	addEntry(outpoint wire.OutPoint, entry *UtxoEntry, overwrite bool) error

	// spendEntry marks an entry as spent.
	spendEntry(outpoint wire.OutPoint, entry *UtxoEntry) error
}

// utxoByHashSource is an interface that allows fetching UTXO entries by
// transaction hash.
// This interface exists due to the legacy spend journal database structure.
type utxoByHashSource interface {
	// getEntryByHash looks for an entry with the given transaction hash.
	// This method exists due to the legacy spend journal database structure.
	// Its execution is very inefficient, but it's almost never used.
	getEntryByHash(hash *chainhash.Hash) (*UtxoEntry, error)
}

// utxoBatcher is an interface that allows a caller to batch fetch UTXOs from
// an underlying view.
type utxoBatcher interface {
	utxoView

	// getEntries attempts to fetch a serise of entries from the UTXO view.
	// If a single entry is not found within the view, then an error is
	// returned.
	getEntries(outpoints map[wire.OutPoint]struct{}) (map[wire.OutPoint]*UtxoEntry, error)
}

// utxoCache is a cached utxo view in the chainstate of a BlockChain.
//
// It implements the utxoView interface, but should only be used as such with the
// state mutex held.  It also implements the utxoByHashSource interface.
type utxoCache struct {
	db database.DB

	// maxTotalMemoryUsage is the maximum memory usage in bytes that the state
	// should contain in normal circumstances.
	maxTotalMemoryUsage uint64

	// This mutex protects the internal state.
	// A simple mutex instead of a read-write mutex is chosen because the main
	// read method also possibly does a write on a cache miss.
	mtx sync.Mutex

	// cachedEntries keeps the internal cache of the utxo state.  The tfModified
	// flag indicates that the state of the entry (potentially) deviates from the
	// state in the database.  Explicit nil values in the map are used to
	// indicate that the database does not contain the entry.
	cachedEntries    map[wire.OutPoint]*UtxoEntry
	totalEntryMemory uint64 // Total memory usage in bytes.
	lastFlushHash    chainhash.Hash
}

// newUtxoCache initiates a new utxo cache instance with its memory usage limited
// to the given maximum.
func newUtxoCache(db database.DB, maxTotalMemoryUsage uint64) *utxoCache {
	avgEntrySize := outpointSize + 8 + baseEntrySize + pubKeyHashLen
	numMaxElements := maxTotalMemoryUsage / avgEntrySize
	log.Info("Pre-alloacting for %v entries: ", numMaxElements)

	return &utxoCache{
		db:                  db,
		maxTotalMemoryUsage: maxTotalMemoryUsage,
		cachedEntries:       make(map[wire.OutPoint]*UtxoEntry, numMaxElements),
	}
}

// totalMemoryUsage returns the total memory usage in bytes of the UTXO cache.
//
// This method should be called with the state lock held.
func (s *utxoCache) totalMemoryUsage() uint64 {
	// Total memory is all the keys plus the total memory of all the entries.
	nbEntries := uint64(len(s.cachedEntries))

	// Total size is total size of the keys + total size of the pointers in the
	// map + total size of the elements held in the pointers.
	return nbEntries*outpointSize + nbEntries*8 + s.totalEntryMemory
}

// TotalMemoryUsage returns the total memory usage in bytes of the UTXO cache.
//
// This method is safe for concurrent access.
func (s *utxoCache) TotalMemoryUsage() uint64 {
	s.mtx.Lock()
	tmu := s.totalMemoryUsage()
	s.mtx.Unlock()
	return tmu
}

// seekAndCacheEntries attempts to fetch a series of entries from the
// database.  In any aren't found, then nil is returned. All entries found are
// cached in the process.
//
// This method should be called with the state lock held.
func (s *utxoCache) seekAndCacheEntries(ops ...wire.OutPoint) (
	map[wire.OutPoint]*UtxoEntry, error) {

	sort.Slice(ops, func(i, j int) bool {
		return bytes.Compare(ops[i].Hash[:], ops[j].Hash[:]) < 0 &&
			ops[i].Index < ops[j].Index
	})

	entries := make(map[wire.OutPoint]*UtxoEntry, len(ops))
	err := s.db.View(func(dbTx database.Tx) error {
		utxoBucket := dbTx.Metadata().Bucket(utxoSetBucketName)

		cursor := utxoBucket.Cursor()
		for _, op := range ops {
			entry, err := dbSeekUtxoEntry(cursor, &op)
			if err != nil {
				return err
			}

			entries[op] = entry
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
	for outpoint, entry := range entries {
		s.cachedEntries[outpoint] = entry
		s.totalEntryMemory += entry.memoryUsage()
	}

	return entries, nil
}

// fetchAndCacheEntries attempts to fetch a series of entries from the
// database.  In any aren't found, then nil is returned. All entries found are
// cached in the process.
//
// This method should be called with the state lock held.
func (s *utxoCache) fetchAndCacheEntries(ops map[wire.OutPoint]struct{}) (
	map[wire.OutPoint]*UtxoEntry, error) {

	entries := make(map[wire.OutPoint]*UtxoEntry, len(ops))
	err := s.db.View(func(dbTx database.Tx) error {
		utxoBucket := dbTx.Metadata().Bucket(utxoSetBucketName)

		for op := range ops {
			entry, err := dbFetchUtxoEntry(dbTx, utxoBucket, op)
			if err != nil {
				return err
			}

			entries[op] = entry
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
	for outpoint, entry := range entries {
		s.cachedEntries[outpoint] = entry
		s.totalEntryMemory += entry.memoryUsage()
	}

	return entries, nil
}

// getEntry returns the UTXO entry for the given outpoint.  It returns nil if
// there is no entry for the outpoint in the UTXO state.
//
// This method is part of the utxoView interface.
// This method should be called with the state lock held.
// The returned entry is NOT safe for concurrent access.
func (s *utxoCache) getEntry(outpoint wire.OutPoint) (*UtxoEntry, error) {
	// First, we'll check out in-memory cache to see if we already have the
	// entry.
	if entry, found := s.cachedEntries[outpoint]; found {
		return entry, nil
	}

	// If not, then we'll fetch it from the databse.
	entries, err := s.seekAndCacheEntries(outpoint)
	if err != nil {
		return nil, err
	}

	return entries[outpoint], nil
}

// getEntry returns the UTXO entry for the given outpoint.  It returns nil if
// there is no entry for the outpoint in the UTXO state.
//
// This method is part of the utxoView interface.
// This method should be called with the state lock held.
// The returned entry is NOT safe for concurrent access.
func (s *utxoCache) getEntries(outpoints map[wire.OutPoint]struct{}) (
	map[wire.OutPoint]*UtxoEntry, error) {

	entries := make(map[wire.OutPoint]*UtxoEntry, len(outpoints))
	var missingEntries []wire.OutPoint
	for op := range outpoints {
		if entry, ok := s.cachedEntries[op]; ok {
			entries[op] = entry
			continue
		}

		missingEntries = append(missingEntries, op)
	}

	dbEntries, err := s.seekAndCacheEntries(missingEntries...)
	if err != nil {
		return nil, err
	}

	for op, entry := range dbEntries {
		entries[op] = entry
	}

	return entries, nil
}

// FetchEntry returns the UTXO entry for the given outpoint.  It returns nil if
// there is no entry for the outpoint in the UTXO state.
//
// This method is safe for concurrent access.
func (s *utxoCache) FetchEntry(outpoint wire.OutPoint) (*UtxoEntry, error) {
	s.mtx.Lock()
	entries, err := s.getEntries(map[wire.OutPoint]struct{}{
		outpoint: struct{}{},
	})

	if entry, ok := entries[outpoint]; ok {
		s.mtx.Unlock()
		return entry.Clone(), nil
	}

	s.mtx.Unlock()

	return nil, err
}

// FetchUtxoEntry returns the requested unspent transaction output from the point
// of view of the end of the main chain.
//
// NOTE: Requesting an output for which there is no data will NOT return an
// error.  Instead both the entry and the error will be nil.  This is done to
// allow pruning of spent transaction outputs.  In practice this means the
// caller must check if the returned entry is nil before invoking methods on it.
//
// This function is safe for concurrent access.
func (b *BlockChain) FetchUtxoEntry(outpoint wire.OutPoint) (*UtxoEntry, error) {
	b.chainLock.RLock()
	entry, err := b.utxoCache.FetchEntry(outpoint)
	b.chainLock.RUnlock()
	return entry, err
}

// getEntryByHash attempts to find any available UTXO for the given hash by
// searching the entire set of possible outputs for the given hash.
//
// This method is part of the utxoByHashSource interface.
// This method should be called with the state lock held.
func (s *utxoCache) getEntryByHash(hash *chainhash.Hash) (*UtxoEntry, error) {
	// First attempt to find a utxo with the provided hash in the cache.
	prevOut := wire.OutPoint{Hash: *hash}
	for idx := uint32(0); idx < MaxOutputsPerBlock; idx++ {
		prevOut.Index = idx
		if entry, _ := s.cachedEntries[prevOut]; entry != nil {
			return entry.Clone(), nil
		}
	}

	// Then fall back to the database.
	var entry *UtxoEntry
	err := s.db.View(func(dbTx database.Tx) error {
		var err error
		entry, err = dbFetchUtxoEntryByHash(dbTx, hash)
		return err
	})

	// Since we don't know the entries outpoint, we can't cache it.
	return entry, err
}

// FetchEntryByHash attempts to find any available UTXO for the given hash by
// searching the entire set of possible outputs for the given hash.
//
// This method is safe for concurrent access.
func (s *utxoCache) FetchEntryByHash(hash *chainhash.Hash) (*UtxoEntry, error) {
	s.mtx.Lock()
	entry, err := s.getEntryByHash(hash)
	s.mtx.Unlock()
	return entry.Clone(), err
}

// spendEntry marks the output as spent.  Spending an output that is already
// spent has no effect.  Entries that need not be stored anymore after being
// spent will be removed from the cache.
//
// This method is part of the utxoView interface.
// This method should be called with the state lock held.
func (s *utxoCache) spendEntry(outpoint wire.OutPoint, addIfNil *UtxoEntry) error {
	entry := s.cachedEntries[outpoint]

	// If we don't have an entry in cache and an entry was provided, we add it.
	if entry == nil && addIfNil != nil {
		if err := s.addEntry(outpoint, addIfNil, false); err != nil {
			return err
		}
		entry = addIfNil
	}

	// If it's nil or already spent, nothing to do.
	if entry == nil || entry.IsSpent() {
		return nil
	}

	// If an entry is fresh, meaning that there hasn't been a flush since it was
	// introduced, it can simply be removed.
	if entry.isFresh() {
		// We don't delete it from the map, but set the value to nil, so that
		// later lookups for the entry know that the entry does not exist in the
		// database.
		s.cachedEntries[outpoint] = nil
		s.totalEntryMemory -= entry.memoryUsage()
		return nil
	}

	// Mark the output as spent and modified.
	entry.packedFlags |= tfSpent | tfModified

	//TODO(stevenroose) check if it's ok to drop the pkScript
	// Since we don't need it anymore, drop the pkScript value of the entry.
	s.totalEntryMemory -= entry.memoryUsage()
	entry.pkScript = nil
	s.totalEntryMemory += entry.memoryUsage()

	return nil
}

// addEntry adds a new unspent entry if it is not probably unspendable.  Set
// overwrite to true to skip validity and freshness checks and simply add the
// item, possibly overwriting another entry that is not-fully-spent.
//
// This method is part of the utxoView interface.
// This method should be called with the state lock held.
func (s *utxoCache) addEntry(outpoint wire.OutPoint, entry *UtxoEntry, overwrite bool) error {
	// Don't add provably unspendable outputs.
	if txscript.IsUnspendable(entry.pkScript) {
		return nil
	}

	cachedEntry, _ := s.cachedEntries[outpoint]

	// In overwrite mode, simply add the entry without doing these checks.
	if !overwrite {
		// Prevent overwriting not-fully-spent entries.  Note that this is not
		// a consensus check.
		if cachedEntry != nil && !cachedEntry.IsSpent() {
			log.Warnf("utxo entry %s attempted to overwrite existing unspent "+
				"entry (pre-bip30?) ", outpoint)
			return nil
		}

		// If we didn't have an entry for the outpoint and the existing entry is
		// not marked modified, we can mark it fresh as the database does not
		// know about this entry.  This will allow us to erase it when it gets
		// spent before the next flush.
		if cachedEntry == nil && !entry.isModified() {
			entry.packedFlags |= tfFresh
		}
	}

	entry.packedFlags |= tfModified
	s.cachedEntries[outpoint] = entry
	s.totalEntryMemory -= cachedEntry.memoryUsage() // 0 for nil
	s.totalEntryMemory += entry.memoryUsage()
	return nil
}

// FetchTxView returns a local view on the utxo state for the given transaction.
//
// This method is safe for concurrent access.
func (s *utxoCache) FetchTxView(tx *btcutil.Tx) (*UtxoViewpoint, error) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	// TODO(roasbeef): no need to try to fech outputs

	view := NewUtxoViewpoint()
	viewEntries := view.Entries()
	if !IsCoinBase(tx) {
		for _, txIn := range tx.MsgTx().TxIn {
			entry, err := s.getEntry(txIn.PreviousOutPoint)
			if err != nil {
				return nil, err
			}

			viewEntries[txIn.PreviousOutPoint] = entry.Clone()
		}
	}
	prevOut := wire.OutPoint{Hash: *tx.Hash()}
	for txOutIdx := range tx.MsgTx().TxOut {
		prevOut.Index = uint32(txOutIdx)

		entry, err := s.getEntry(prevOut)
		if err != nil {
			return nil, err
		}

		viewEntries[prevOut] = entry.Clone()
	}

	return view, nil
}

// FetchUtxoView loads unspent transaction outputs for the inputs referenced by
// the passed transaction from the point of view of the end of the main chain.
// It also attempts to get the utxos for the outputs of the transaction itself
// so the returned view can be examined for duplicate transactions.
//
// This function is safe for concurrent access however the returned view is NOT.
func (b *BlockChain) FetchUtxoView(tx *btcutil.Tx) (*UtxoViewpoint, error) {
	b.chainLock.RLock()
	view, err := b.utxoCache.FetchTxView(tx)
	b.chainLock.RUnlock()
	return view, err
}

// Commit commits all the entries in the view to the cache.
//
// This method should be called with the state lock held.
func (s *utxoCache) Commit(view *UtxoViewpoint) error {
	for outpoint, entry := range view.Entries() {
		// No need to update the database if the entry was not modified or fresh.
		if entry == nil || (!entry.isModified() && !entry.isFresh()) {
			continue
		}

		// We can't use the view entry directly because it can be modified
		// later on.
		ourEntry := s.cachedEntries[outpoint]
		if ourEntry == nil {
			ourEntry = entry.Clone()
		}

		// Remove the utxo entry if it is spent.
		if entry.IsSpent() {
			if err := s.spendEntry(outpoint, ourEntry); err != nil {
				return err
			}
			continue
		}

		// It's possible if we disconnected this UTXO at some point, removing it from
		// the UTXO set, only to have a future block add it back. In that case it could
		// be going from being marked spent to needing to be marked unspent so we handle
		// that case by overriding here.
		if ourEntry.IsSpent() && !entry.IsSpent() {
			ourEntry = entry
		}

		// Store the entry we don't know.
		//
		// TODO(roasbeef): should have already added when adding all
		// inputs?
		if err := s.addEntry(outpoint, ourEntry, false); err != nil {
			return err
		}
	}

	view.prune()
	return nil
}

// flush flushes the UTXO state to the database.
//
// This method should be called with the state lock held.
func (s *utxoCache) flush(bestState *BestState) error {
	// If we performed a flush in the current best state, we have nothing to do.
	if bestState.Hash == s.lastFlushHash {
		return nil
	}

	// Add one to round up the integer division.
	totalMiB := s.totalMemoryUsage()/(1024*1024) + 1
	log.Infof("Flushing UTXO cache of ~%v MiB to disk. For large sizes, "+
		"this can take up to several minutes...", totalMiB)

	// First update the database to indicate that a utxo state flush is started.
	// This allows us to recover when the node shuts down in the middle of this
	// method.
	err := s.db.Update(func(dbTx database.Tx) error {
		return dbPutUtxoStateConsistency(dbTx, ucsFlushOngoing, &s.lastFlushHash)
	})
	if err != nil {
		return err
	}

	// Store all entries in batches.
	flushBatch := func(dbTx database.Tx) error {
		var (
			// Form a batch by storing all entries to be put and deleted.
			nbBatchEntries = 0
			entriesPut     = make(map[wire.OutPoint]*UtxoEntry)
			entriesDelete  = make([]wire.OutPoint, 0)
		)
		for outpoint, entry := range s.cachedEntries {
			// Nil entries or unmodified entries can just be pruned.
			// They don't count for the batch size.
			if entry == nil || !entry.isModified() {
				s.totalEntryMemory -= entry.memoryUsage()
				delete(s.cachedEntries, outpoint)
				continue
			}

			if entry.IsSpent() {
				entriesDelete = append(entriesDelete, outpoint)
			} else {
				entriesPut[outpoint] = entry
			}
			nbBatchEntries++

			s.totalEntryMemory -= entry.memoryUsage()
			delete(s.cachedEntries, outpoint)

			// End this batch when the maximum number of entries per batch has
			// been reached.
			if nbBatchEntries >= utxoBatchSizeEntries {
				break
			}
		}

		// Apply the batched additions and deletions.
		if err := dbPutUtxoEntries(dbTx, entriesPut); err != nil {
			return err
		}
		if err := dbDeleteUtxoEntries(dbTx, entriesDelete); err != nil {
			return err
		}
		return nil
	}
	for len(s.cachedEntries) > 0 {
		log.Tracef("Flushing %d more entries...", len(s.cachedEntries))
		err := s.db.Update(func(dbTx database.Tx) error {
			return flushBatch(dbTx)
		})
		if err != nil {
			return err
		}
	}

	// When done, store the best state hash in the database to indicate the state
	// is consistent until that hash.
	err = s.db.Update(func(dbTx database.Tx) error {
		return dbPutUtxoStateConsistency(dbTx, ucsConsistent, &bestState.Hash)
	})
	if err != nil {
		return err
	}

	log.Debug("Done flushing UTXO cache to disk")
	return nil
}

// Flush flushes the UTXO state to the database.
//
// This function is safe for concurrent access.
func (s *utxoCache) Flush(mode FlushMode, bestState *BestState) error {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var threshold uint64
	switch mode {
	case FlushRequired:
		threshold = 0

	case FlushIfNeeded:
		threshold = s.maxTotalMemoryUsage

	case FlushPeriodic:
		threshold = (utxoFlushPeriodicThreshold * s.maxTotalMemoryUsage) / 100
	}

	if s.totalMemoryUsage() > threshold {
		return s.flush(bestState)
	}
	return nil
}

// rollBackBlock rolls back the effects of the block when the state was left in
// an inconsistent state.  This means that no errors will be raised when the
// state is invalid.
//
// This method should be called with the state lock held.
func (s *utxoCache) rollBackBlock(block *btcutil.Block, stxos []SpentTxOut) error {
	return disconnectTransactions(s, block, stxos, s)
}

// rollForwardBlock rolls forward the effects of the block when the state was
// left in an inconsistent state.  This means that no errors will be raised when
// the state is invalid.
//
// This method should be called with the state lock held.
func (s *utxoCache) rollForwardBlock(block *btcutil.Block) error {
	// We don't need the collect stxos and we allow overwriting existing entries.
	return connectTransactions(s, block, nil, true)
}

// InitConsistentState checks the consistency status of the utxo state and
// replays blocks if it lags behind the best state of the blockchain.
//
// It needs to be ensured that the chainView passed to this method does not
// get changed during the execution of this method.
func (s *utxoCache) InitConsistentState(tip *blockNode, interrupt <-chan struct{}) error {
	// Load the consistency status from the database.
	var statusCode byte
	var statusHash *chainhash.Hash
	err := s.db.View(func(dbTx database.Tx) error {
		var err error
		statusCode, statusHash, err = dbFetchUtxoStateConsistency(dbTx)

		return err
	})
	if err != nil {
		return err
	}

	log.Tracef("UTXO cache consistency status from disk: [%d] hash %v",
		statusCode, statusHash)

	// We can set this variable now already because it will always be valid
	// unless an error is returned, in which case the state is entirely invalid.
	// Doing it here prevents forgetting it later.
	s.lastFlushHash = tip.hash

	// If no status was found, the database is old and didn't have a cached utxo
	// state yet. In that case, we set the status to the best state and write
	// this to the database.
	if statusCode == ucsEmpty {
		log.Debugf("Database didn't specify UTXO state consistency: consistent "+
			"to best chain tip (%v)", tip.hash)
		err := s.db.Update(func(dbTx database.Tx) error {
			return dbPutUtxoStateConsistency(dbTx, ucsConsistent, &tip.hash)
		})

		return err
	}

	// If state is consistent, we are done.
	if statusCode == ucsConsistent && *statusHash == tip.hash {
		log.Debugf("UTXO state consistent (%d:%v)", tip.height, tip.hash)

		return nil
	}

	log.Info("Reconstructing UTXO state after unclean shutdown. This may take " +
		"a long time...")

	// Even though this should always be true, make sure the fetched hash is in
	// the best chain.
	var statusNode *blockNode
	var statusNodeNext *blockNode // the first one higher than the statusNode
	attachNodes := list.New()
	for node := tip; node.height >= 0; node = node.parent {
		if node.hash == *statusHash {
			statusNode = node
			break
		}
		attachNodes.PushFront(node)
		statusNodeNext = node
	}

	if statusNode == nil {
		return AssertError(fmt.Sprintf("last utxo consistency status contains "+
			"hash that is not in best chain: %v", statusHash))
	}

	// If data was in the middle of a flush, we have to roll back all
	// blocks from the last best block all the way back to the last
	// consistent block.
	log.Debugf("Rolling back %d blocks to rebuild the UTXO state...",
		tip.height-statusNode.height)

	// Roll back blocks in batches.
	rollbackBatch := func(dbTx database.Tx, node *blockNode) (*blockNode, error) {
		nbBatchBlocks := 0
		for ; node.height > statusNode.height; node = node.parent {
			block, err := dbFetchBlockByNode(dbTx, node)
			if err != nil {
				return nil, err
			}

			stxos, err := dbFetchSpendJournalEntry(dbTx, block)
			if err != nil {
				return nil, err
			}

			if err := s.rollBackBlock(block, stxos); err != nil {
				return nil, err
			}

			nbBatchBlocks++

			if nbBatchBlocks >= utxoBatchSizeBlocks {
				break
			}
		}

		return node, nil
	}

	for node := tip; node.height > statusNode.height; {
		log.Tracef("Rolling back %d more blocks...",
			node.height-statusNode.height)
		err := s.db.Update(func(dbTx database.Tx) error {
			var err error
			node, err = rollbackBatch(dbTx, node)

			return err
		})
		if err != nil {
			return err
		}

		if interruptRequested(interrupt) {
			log.Warn("UTXO state reconstruction interrupted")

			return errInterruptRequested
		}
	}

	// Now we can update the status already to avoid redoing this work when
	// interrupted.
	err = s.db.Update(func(dbTx database.Tx) error {
		return dbPutUtxoStateConsistency(dbTx, ucsConsistent, statusHash)
	})
	if err != nil {
		return err
	}

	log.Debugf("Replaying %d blocks to rebuild UTXO state...",
		tip.height-statusNodeNext.height+1)

	// Then we replay the blocks from the last consistent state up to the best
	// state. Iterate forward from the consistent node to the tip of the best
	// chain. After every batch, we can also update the consistency state to
	// avoid redoing the work when interrupted.
	rollforwardBatch := func(dbTx database.Tx, node *blockNode) (*blockNode, error) {
		nbBatchBlocks := 0
		for e := attachNodes.Front(); e != nil; e = e.Next() {
			node = e.Value.(*blockNode)
			attachNodes.Remove(e)

			block, err := dbFetchBlockByNode(dbTx, node)
			if err != nil {
				return nil, err
			}

			if err := s.rollForwardBlock(block); err != nil {
				return nil, err
			}
			nbBatchBlocks++

			if nbBatchBlocks >= utxoBatchSizeBlocks {
				break
			}
		}

		// We can update this after each batch to avoid having to redo the work
		// when interrupted.
		return node, dbPutUtxoStateConsistency(dbTx, ucsConsistent, &node.hash)
	}

	for node := statusNodeNext; node.height <= tip.height; {
		log.Tracef("Replaying %d more blocks...", tip.height-node.height+1)
		err := s.db.Update(func(dbTx database.Tx) error {
			var err error
			node, err = rollforwardBatch(dbTx, node)

			return err
		})
		if err != nil {
			return err
		}

		if interruptRequested(interrupt) {
			log.Warn("UTXO state reconstruction interrupted")

			return errInterruptRequested
		}
		if node.height == tip.height {
			break
		}
	}

	log.Debug("UTXO state reconstruction done")

	return nil
}
