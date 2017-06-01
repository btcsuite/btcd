// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ffldb

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/btcsuite/goleveldb/leveldb"
	"github.com/btcsuite/goleveldb/leveldb/iterator"
	"github.com/btcsuite/goleveldb/leveldb/util"
	"github.com/decred/dcrd/database/internal/treap"
)

const (
	// defaultCacheSize is the default size for the database cache.
	defaultCacheSize = 100 * 1024 * 1024 // 100 MB

	// defaultFlushSecs is the default number of seconds to use as a
	// threshold in between database cache flushes when the cache size has
	// not been exceeded.
	defaultFlushSecs = 300 // 5 minutes
)

// ldbCacheIter wraps a treap iterator to provide the additional functionality
// needed to satisfy the leveldb iterator.Iterator interface.
type ldbCacheIter struct {
	*treap.Iterator
}

// Enforce ldbCacheIterator implements the leveldb iterator.Iterator interface.
var _ iterator.Iterator = (*ldbCacheIter)(nil)

// Error is only provided to satisfy the iterator interface as there are no
// errors for this memory-only structure.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *ldbCacheIter) Error() error {
	return nil
}

// SetReleaser is only provided to satisfy the iterator interface as there is no
// need to override it.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *ldbCacheIter) SetReleaser(releaser util.Releaser) {
}

// Release is only provided to satisfy the iterator interface.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *ldbCacheIter) Release() {
}

// newLdbCacheIter creates a new treap iterator for the given slice against the
// pending keys for the passed cache snapshot and returns it wrapped in an
// ldbCacheIter so it can be used as a leveldb iterator.
func newLdbCacheIter(snap *dbCacheSnapshot, slice *util.Range) *ldbCacheIter {
	iter := snap.pendingKeys.Iterator(slice.Start, slice.Limit)
	return &ldbCacheIter{Iterator: iter}
}

// dbCacheIterator defines an iterator over the key/value pairs in the database
// cache and underlying database.
type dbCacheIterator struct {
	cacheSnapshot *dbCacheSnapshot
	dbIter        iterator.Iterator
	cacheIter     iterator.Iterator
	currentIter   iterator.Iterator
	released      bool
}

// Enforce dbCacheIterator implements the leveldb iterator.Iterator interface.
var _ iterator.Iterator = (*dbCacheIterator)(nil)

// skipPendingUpdates skips any keys at the current database iterator position
// that are being updated by the cache.  The forwards flag indicates the
// direction the iterator is moving.
func (iter *dbCacheIterator) skipPendingUpdates(forwards bool) {
	for iter.dbIter.Valid() {
		var skip bool
		key := iter.dbIter.Key()
		if iter.cacheSnapshot.pendingRemove.Has(key) {
			skip = true
		} else if iter.cacheSnapshot.pendingKeys.Has(key) {
			skip = true
		}
		if !skip {
			break
		}

		if forwards {
			iter.dbIter.Next()
		} else {
			iter.dbIter.Prev()
		}
	}
}

// chooseIterator first skips any entries in the database iterator that are
// being updated by the cache and sets the current iterator to the appropriate
// iterator depending on their validity and the order they compare in while taking
// into account the direction flag.  When the iterator is being moved forwards
// and both iterators are valid, the iterator with the smaller key is chosen and
// vice versa when the iterator is being moved backwards.
func (iter *dbCacheIterator) chooseIterator(forwards bool) bool {
	// Skip any keys at the current database iterator position that are
	// being updated by the cache.
	iter.skipPendingUpdates(forwards)

	// When both iterators are exhausted, the iterator is exhausted too.
	if !iter.dbIter.Valid() && !iter.cacheIter.Valid() {
		iter.currentIter = nil
		return false
	}

	// Choose the database iterator when the cache iterator is exhausted.
	if !iter.cacheIter.Valid() {
		iter.currentIter = iter.dbIter
		return true
	}

	// Choose the cache iterator when the database iterator is exhausted.
	if !iter.dbIter.Valid() {
		iter.currentIter = iter.cacheIter
		return true
	}

	// Both iterators are valid, so choose the iterator with either the
	// smaller or larger key depending on the forwards flag.
	compare := bytes.Compare(iter.dbIter.Key(), iter.cacheIter.Key())
	if (forwards && compare > 0) || (!forwards && compare < 0) {
		iter.currentIter = iter.cacheIter
	} else {
		iter.currentIter = iter.dbIter
	}
	return true
}

// First positions the iterator at the first key/value pair and returns whether
// or not the pair exists.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *dbCacheIterator) First() bool {
	// Seek to the first key in both the database and cache iterators and
	// choose the iterator that is both valid and has the smaller key.
	iter.dbIter.First()
	iter.cacheIter.First()
	return iter.chooseIterator(true)
}

// Last positions the iterator at the last key/value pair and returns whether or
// not the pair exists.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *dbCacheIterator) Last() bool {
	// Seek to the last key in both the database and cache iterators and
	// choose the iterator that is both valid and has the larger key.
	iter.dbIter.Last()
	iter.cacheIter.Last()
	return iter.chooseIterator(false)
}

// Next moves the iterator one key/value pair forward and returns whether or not
// the pair exists.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *dbCacheIterator) Next() bool {
	// Nothing to return if cursor is exhausted.
	if iter.currentIter == nil {
		return false
	}

	// Move the current iterator to the next entry and choose the iterator
	// that is both valid and has the smaller key.
	iter.currentIter.Next()
	return iter.chooseIterator(true)
}

// Prev moves the iterator one key/value pair backward and returns whether or
// not the pair exists.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *dbCacheIterator) Prev() bool {
	// Nothing to return if cursor is exhausted.
	if iter.currentIter == nil {
		return false
	}

	// Move the current iterator to the previous entry and choose the
	// iterator that is both valid and has the larger key.
	iter.currentIter.Prev()
	return iter.chooseIterator(false)
}

// Seek positions the iterator at the first key/value pair that is greater than
// or equal to the passed seek key.  Returns false if no suitable key was found.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *dbCacheIterator) Seek(key []byte) bool {
	// Seek to the provided key in both the database and cache iterators
	// then choose the iterator that is both valid and has the larger key.
	iter.dbIter.Seek(key)
	iter.cacheIter.Seek(key)
	return iter.chooseIterator(true)
}

// Valid indicates whether the iterator is positioned at a valid key/value pair.
// It will be considered invalid when the iterator is newly created or exhausted.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *dbCacheIterator) Valid() bool {
	return iter.currentIter != nil
}

// Key returns the current key the iterator is pointing to.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *dbCacheIterator) Key() []byte {
	// Nothing to return if iterator is exhausted.
	if iter.currentIter == nil {
		return nil
	}

	return iter.currentIter.Key()
}

// Value returns the current value the iterator is pointing to.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *dbCacheIterator) Value() []byte {
	// Nothing to return if iterator is exhausted.
	if iter.currentIter == nil {
		return nil
	}

	return iter.currentIter.Value()
}

// SetReleaser is only provided to satisfy the iterator interface as there is no
// need to override it.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *dbCacheIterator) SetReleaser(releaser util.Releaser) {
}

// Release releases the iterator by removing the underlying treap iterator from
// the list of active iterators against the pending keys treap.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *dbCacheIterator) Release() {
	if !iter.released {
		iter.dbIter.Release()
		iter.cacheIter.Release()
		iter.currentIter = nil
		iter.released = true
	}
}

// Error is only provided to satisfy the iterator interface as there are no
// errors for this memory-only structure.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *dbCacheIterator) Error() error {
	return nil
}

// dbCacheSnapshot defines a snapshot of the database cache and underlying
// database at a particular point in time.
type dbCacheSnapshot struct {
	dbSnapshot    *leveldb.Snapshot
	pendingKeys   *treap.Immutable
	pendingRemove *treap.Immutable
}

// Has returns whether or not the passed key exists.
func (snap *dbCacheSnapshot) Has(key []byte) bool {
	// Check the cached entries first.
	if snap.pendingRemove.Has(key) {
		return false
	}
	if snap.pendingKeys.Has(key) {
		return true
	}

	// Consult the database.
	hasKey, _ := snap.dbSnapshot.Has(key, nil)
	return hasKey
}

// Get returns the value for the passed key.  The function will return nil when
// the key does not exist.
func (snap *dbCacheSnapshot) Get(key []byte) []byte {
	// Check the cached entries first.
	if snap.pendingRemove.Has(key) {
		return nil
	}
	if value := snap.pendingKeys.Get(key); value != nil {
		return value
	}

	// Consult the database.
	value, err := snap.dbSnapshot.Get(key, nil)
	if err != nil {
		return nil
	}
	return value
}

// Release releases the snapshot.
func (snap *dbCacheSnapshot) Release() {
	snap.dbSnapshot.Release()
	snap.pendingKeys = nil
	snap.pendingRemove = nil
}

// NewIterator returns a new iterator for the snapshot.  The newly returned
// iterator is not pointing to a valid item until a call to one of the methods
// to position it is made.
//
// The slice parameter allows the iterator to be limited to a range of keys.
// The start key is inclusive and the limit key is exclusive.  Either or both
// can be nil if the functionality is not desired.
func (snap *dbCacheSnapshot) NewIterator(slice *util.Range) *dbCacheIterator {
	return &dbCacheIterator{
		dbIter:        snap.dbSnapshot.NewIterator(slice, nil),
		cacheIter:     newLdbCacheIter(snap, slice),
		cacheSnapshot: snap,
	}
}

// dbCache provides a database cache layer backed by an underlying database.  It
// allows a maximum cache size and flush interval to be specified such that the
// cache is flushed to the database when the cache size exceeds the maximum
// configured value or it has been longer than the configured interval since the
// last flush.  This effectively provides transaction batching so that callers
// can commit transactions at will without incurring large performance hits due
// to frequent disk syncs.
type dbCache struct {
	// ldb is the underlying leveldb DB for metadata.
	ldb *leveldb.DB

	// store is used to sync blocks to flat files.
	store *blockStore

	// The following fields are related to flushing the cache to persistent
	// storage.  Note that all flushing is performed in an opportunistic
	// fashion.  This means that it is only flushed during a transaction or
	// when the database cache is closed.
	//
	// maxSize is the maximum size threshold the cache can grow to before
	// it is flushed.
	//
	// flushInterval is the threshold interval of time that is allowed to
	// pass before the cache is flushed.
	//
	// lastFlush is the time the cache was last flushed.  It is used in
	// conjunction with the current time and the flush interval.
	//
	// NOTE: These flush related fields are protected by the database write
	// lock.
	maxSize       uint64
	flushInterval time.Duration
	lastFlush     time.Time

	// The following fields hold the keys that need to be stored or deleted
	// from the underlying database once the cache is full, enough time has
	// passed, or when the database is shutting down.  Note that these are
	// stored using immutable treaps to support O(1) MVCC snapshots against
	// the cached data.  The cacheLock is used to protect concurrent access
	// for cache updates and snapshots.
	cacheLock    sync.RWMutex
	cachedKeys   *treap.Immutable
	cachedRemove *treap.Immutable
}

// Snapshot returns a snapshot of the database cache and underlying database at
// a particular point in time.
//
// The snapshot must be released after use by calling Release.
func (c *dbCache) Snapshot() (*dbCacheSnapshot, error) {
	dbSnapshot, err := c.ldb.GetSnapshot()
	if err != nil {
		str := "failed to open transaction"
		return nil, convertErr(str, err)
	}

	// Since the cached keys to be added and removed use an immutable treap,
	// a snapshot is simply obtaining the root of the tree under the lock
	// which is used to atomically swap the root.
	c.cacheLock.RLock()
	cacheSnapshot := &dbCacheSnapshot{
		dbSnapshot:    dbSnapshot,
		pendingKeys:   c.cachedKeys,
		pendingRemove: c.cachedRemove,
	}
	c.cacheLock.RUnlock()
	return cacheSnapshot, nil
}

// updateDB invokes the passed function in the context of a managed leveldb
// transaction.  Any errors returned from the user-supplied function will cause
// the transaction to be rolled back and are returned from this function.
// Otherwise, the transaction is committed when the user-supplied function
// returns a nil error.
func (c *dbCache) updateDB(fn func(ldbTx *leveldb.Transaction) error) error {
	// Start a leveldb transaction.
	ldbTx, err := c.ldb.OpenTransaction()
	if err != nil {
		return convertErr("failed to open ldb transaction", err)
	}

	if err := fn(ldbTx); err != nil {
		ldbTx.Discard()
		return err
	}

	// Commit the leveldb transaction and convert any errors as needed.
	if err := ldbTx.Commit(); err != nil {
		return convertErr("failed to commit leveldb transaction", err)
	}
	return nil
}

// TreapForEacher is an interface which allows iteration of a treap in ascending
// order using a user-supplied callback for each key/value pair.  It mainly
// exists so both mutable and immutable treaps can be atomically committed to
// the database with the same function.
type TreapForEacher interface {
	ForEach(func(k, v []byte) bool)
}

// commitTreaps atomically commits all of the passed pending add/update/remove
// updates to the underlying database.
func (c *dbCache) commitTreaps(pendingKeys, pendingRemove TreapForEacher) error {
	// Perform all leveldb updates using an atomic transaction.
	return c.updateDB(func(ldbTx *leveldb.Transaction) error {
		var innerErr error
		pendingKeys.ForEach(func(k, v []byte) bool {
			if dbErr := ldbTx.Put(k, v, nil); dbErr != nil {
				str := fmt.Sprintf("failed to put key %q to "+
					"ldb transaction", k)
				innerErr = convertErr(str, dbErr)
				return false
			}
			return true
		})
		if innerErr != nil {
			return innerErr
		}

		pendingRemove.ForEach(func(k, v []byte) bool {
			if dbErr := ldbTx.Delete(k, nil); dbErr != nil {
				str := fmt.Sprintf("failed to delete "+
					"key %q from ldb transaction",
					k)
				innerErr = convertErr(str, dbErr)
				return false
			}
			return true
		})
		return innerErr
	})
}

// flush flushes the database cache to persistent storage.  This involes syncing
// the block store and replaying all transactions that have been applied to the
// cache to the underlying database.
//
// This function MUST be called with the database write lock held.
func (c *dbCache) flush() error {
	c.lastFlush = time.Now()

	// Sync the current write file associated with the block store.  This is
	// necessary before writing the metadata to prevent the case where the
	// metadata contains information about a block which actually hasn't
	// been written yet in unexpected shutdown scenarios.
	if err := c.store.syncBlocks(); err != nil {
		return err
	}

	// Since the cached keys to be added and removed use an immutable treap,
	// a snapshot is simply obtaining the root of the tree under the lock
	// which is used to atomically swap the root.
	c.cacheLock.RLock()
	cachedKeys := c.cachedKeys
	cachedRemove := c.cachedRemove
	c.cacheLock.RUnlock()

	// Nothing to do if there is no data to flush.
	if cachedKeys.Len() == 0 && cachedRemove.Len() == 0 {
		return nil
	}

	// Perform all leveldb updates using an atomic transaction.
	if err := c.commitTreaps(cachedKeys, cachedRemove); err != nil {
		return err
	}

	// Clear the cache since it has been flushed.
	c.cacheLock.Lock()
	c.cachedKeys = treap.NewImmutable()
	c.cachedRemove = treap.NewImmutable()
	c.cacheLock.Unlock()

	return nil
}

// needsFlush returns whether or not the database cache needs to be flushed to
// persistent storage based on its current size, whether or not adding all of
// the entries in the passed database transaction would cause it to exceed the
// configured limit, and how much time has elapsed since the last time the cache
// was flushed.
//
// This function MUST be called with the database write lock held.
func (c *dbCache) needsFlush(tx *transaction) bool {
	// A flush is needed when more time has elapsed than the configured
	// flush interval.
	if time.Since(c.lastFlush) > c.flushInterval {
		return true
	}

	// A flush is needed when the size of the database cache exceeds the
	// specified max cache size.  The total calculated size is multiplied by
	// 1.5 here to account for additional memory consumption that will be
	// needed during the flush as well as old nodes in the cache that are
	// referenced by the snapshot used by the transaction.
	snap := tx.snapshot
	totalSize := snap.pendingKeys.Size() + snap.pendingRemove.Size()
	totalSize = uint64(float64(totalSize) * 1.5)

	return totalSize > c.maxSize
}

// commitTx atomically adds all of the pending keys to add and remove into the
// database cache.  When adding the pending keys would cause the size of the
// cache to exceed the max cache size, or the time since the last flush exceeds
// the configured flush interval, the cache will be flushed to the underlying
// persistent database.
//
// This is an atomic operation with respect to the cache in that either all of
// the pending keys to add and remove in the transaction will be applied or none
// of them will.
//
// The database cache itself might be flushed to the underlying persistent
// database even if the transaction fails to apply, but it will only be the
// state of the cache without the transaction applied.
//
// This function MUST be called during a database write transaction which in
// turn implies the database write lock will be held.
func (c *dbCache) commitTx(tx *transaction) error {
	// Flush the cache and write the current transaction directly to the
	// database if a flush is needed.
	if c.needsFlush(tx) {
		if err := c.flush(); err != nil {
			return err
		}

		// Perform all leveldb updates using an atomic transaction.
		err := c.commitTreaps(tx.pendingKeys, tx.pendingRemove)
		if err != nil {
			return err
		}

		// Clear the transaction entries since they have been committed.
		tx.pendingKeys = nil
		tx.pendingRemove = nil
		return nil
	}

	// At this point a database flush is not needed, so atomically commit
	// the transaction to the cache.

	// Since the cached keys to be added and removed use an immutable treap,
	// a snapshot is simply obtaining the root of the tree under the lock
	// which is used to atomically swap the root.
	c.cacheLock.RLock()
	newCachedKeys := c.cachedKeys
	newCachedRemove := c.cachedRemove
	c.cacheLock.RUnlock()

	// Apply every key to add in the database transaction to the cache.
	tx.pendingKeys.ForEach(func(k, v []byte) bool {
		newCachedRemove = newCachedRemove.Delete(k)
		newCachedKeys = newCachedKeys.Put(k, v)
		return true
	})
	tx.pendingKeys = nil

	// Apply every key to remove in the database transaction to the cache.
	tx.pendingRemove.ForEach(func(k, v []byte) bool {
		newCachedKeys = newCachedKeys.Delete(k)
		newCachedRemove = newCachedRemove.Put(k, nil)
		return true
	})
	tx.pendingRemove = nil

	// Atomically replace the immutable treaps which hold the cached keys to
	// add and delete.
	c.cacheLock.Lock()
	c.cachedKeys = newCachedKeys
	c.cachedRemove = newCachedRemove
	c.cacheLock.Unlock()
	return nil
}

// Close cleanly shuts down the database cache by syncing all data and closing
// the underlying leveldb database.
//
// This function MUST be called with the database write lock held.
func (c *dbCache) Close() error {
	// Flush any outstanding cached entries to disk.
	if err := c.flush(); err != nil {
		// Even if there is an error while flushing, attempt to close
		// the underlying database.  The error is ignored since it would
		// mask the flush error.
		_ = c.ldb.Close()
		return err
	}

	// Close the underlying leveldb database.
	if err := c.ldb.Close(); err != nil {
		str := "failed to close underlying leveldb database"
		return convertErr(str, err)
	}

	return nil
}

// newDbCache returns a new database cache instance backed by the provided
// leveldb instance.  The cache will be flushed to leveldb when the max size
// exceeds the provided value or it has been longer than the provided interval
// since the last flush.
func newDbCache(ldb *leveldb.DB, store *blockStore, maxSize uint64, flushIntervalSecs uint32) *dbCache {
	return &dbCache{
		ldb:           ldb,
		store:         store,
		maxSize:       maxSize,
		flushInterval: time.Second * time.Duration(flushIntervalSecs),
		lastFlush:     time.Now(),
		cachedKeys:    treap.NewImmutable(),
		cachedRemove:  treap.NewImmutable(),
	}
}
