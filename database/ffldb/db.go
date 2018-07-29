// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ffldb

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/database/internal/treap"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/goleveldb/leveldb"
	"github.com/btcsuite/goleveldb/leveldb/comparer"
	ldberrors "github.com/btcsuite/goleveldb/leveldb/errors"
	"github.com/btcsuite/goleveldb/leveldb/filter"
	"github.com/btcsuite/goleveldb/leveldb/iterator"
	"github.com/btcsuite/goleveldb/leveldb/opt"
	"github.com/btcsuite/goleveldb/leveldb/util"
)

const (
	// metadataDbName is the name used for the metadata database.
	metadataDbName = "metadata"

	// blockHdrSize is the size of a block header.  This is simply the
	// constant from wire and is only provided here for convenience since
	// wire.MaxBlockHeaderPayload is quite long.
	blockHdrSize = wire.MaxBlockHeaderPayload

	// blockHdrOffset defines the offsets into a block index row for the
	// block header.
	//
	// The serialized block index row format is:
	//   <blocklocation><blockheader>
	blockHdrOffset = blockLocSize
)

var (
	// byteOrder is the preferred byte order used through the database and
	// block files.  Sometimes big endian will be used to allow ordered byte
	// sortable integer values.
	byteOrder = binary.LittleEndian

	// bucketIndexPrefix is the prefix used for all entries in the bucket
	// index.
	bucketIndexPrefix = []byte("bidx")

	// curBucketIDKeyName is the name of the key used to keep track of the
	// current bucket ID counter.
	curBucketIDKeyName = []byte("bidx-cbid")

	// metadataBucketID is the ID of the top-level metadata bucket.
	// It is the value 0 encoded as an unsigned big-endian uint32.
	metadataBucketID = [4]byte{}

	// blockIdxBucketID is the ID of the internal block metadata bucket.
	// It is the value 1 encoded as an unsigned big-endian uint32.
	blockIdxBucketID = [4]byte{0x00, 0x00, 0x00, 0x01}

	// blockIdxBucketName is the bucket used internally to track block
	// metadata.
	blockIdxBucketName = []byte("ffldb-blockidx")

	// writeLocKeyName is the key used to store the current write file
	// location.
	writeLocKeyName = []byte("ffldb-writeloc")
)

// Common error strings.
const (
	// errDbNotOpenStr is the text to use for the database.ErrDbNotOpen
	// error code.
	errDbNotOpenStr = "database is not open"

	// errTxClosedStr is the text to use for the database.ErrTxClosed error
	// code.
	errTxClosedStr = "database tx is closed"
)

// bulkFetchData is allows a block location to be specified along with the
// index it was requested from.  This in turn allows the bulk data loading
// functions to sort the data accesses based on the location to improve
// performance while keeping track of which result the data is for.
type bulkFetchData struct {
	*blockLocation
	replyIndex int
}

// bulkFetchDataSorter implements sort.Interface to allow a slice of
// bulkFetchData to be sorted.  In particular it sorts by file and then
// offset so that reads from files are grouped and linear.
type bulkFetchDataSorter []bulkFetchData

// Len returns the number of items in the slice.  It is part of the
// sort.Interface implementation.
func (s bulkFetchDataSorter) Len() int {
	return len(s)
}

// Swap swaps the items at the passed indices.  It is part of the
// sort.Interface implementation.
func (s bulkFetchDataSorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less returns whether the item with index i should sort before the item with
// index j.  It is part of the sort.Interface implementation.
func (s bulkFetchDataSorter) Less(i, j int) bool {
	if s[i].blockFileNum < s[j].blockFileNum {
		return true
	}
	if s[i].blockFileNum > s[j].blockFileNum {
		return false
	}

	return s[i].fileOffset < s[j].fileOffset
}

// makeDbErr creates a database.Error given a set of arguments.
func makeDbErr(c database.ErrorCode, desc string, err error) database.Error {
	return database.Error{ErrorCode: c, Description: desc, Err: err}
}

// convertErr converts the passed leveldb error into a database error with an
// equivalent error code  and the passed description.  It also sets the passed
// error as the underlying error.
func convertErr(desc string, ldbErr error) database.Error {
	// Use the driver-specific error code by default.  The code below will
	// update this with the converted error if it's recognized.
	var code = database.ErrDriverSpecific

	switch {
	// Database corruption errors.
	case ldberrors.IsCorrupted(ldbErr):
		code = database.ErrCorruption

	// Database open/create errors.
	case ldbErr == leveldb.ErrClosed:
		code = database.ErrDbNotOpen

	// Transaction errors.
	case ldbErr == leveldb.ErrSnapshotReleased:
		code = database.ErrTxClosed
	case ldbErr == leveldb.ErrIterReleased:
		code = database.ErrTxClosed
	}

	return database.Error{ErrorCode: code, Description: desc, Err: ldbErr}
}

// copySlice returns a copy of the passed slice.  This is mostly used to copy
// leveldb iterator keys and values since they are only valid until the iterator
// is moved instead of during the entirety of the transaction.
func copySlice(slice []byte) []byte {
	ret := make([]byte, len(slice))
	copy(ret, slice)
	return ret
}

// cursor is an internal type used to represent a cursor over key/value pairs
// and nested buckets of a bucket and implements the database.Cursor interface.
type cursor struct {
	bucket      *bucket
	dbIter      iterator.Iterator
	pendingIter iterator.Iterator
	currentIter iterator.Iterator
}

// Enforce cursor implements the database.Cursor interface.
var _ database.Cursor = (*cursor)(nil)

// Bucket returns the bucket the cursor was created for.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Bucket() database.Bucket {
	// Ensure transaction state is valid.
	if err := c.bucket.tx.checkClosed(); err != nil {
		return nil
	}

	return c.bucket
}

// Delete removes the current key/value pair the cursor is at without
// invalidating the cursor.
//
// Returns the following errors as required by the interface contract:
//   - ErrIncompatibleValue if attempted when the cursor points to a nested
//     bucket
//   - ErrTxNotWritable if attempted against a read-only transaction
//   - ErrTxClosed if the transaction has already been closed
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Delete() error {
	// Ensure transaction state is valid.
	if err := c.bucket.tx.checkClosed(); err != nil {
		return err
	}

	// Error if the cursor is exhausted.
	if c.currentIter == nil {
		str := "cursor is exhausted"
		return makeDbErr(database.ErrIncompatibleValue, str, nil)
	}

	// Do not allow buckets to be deleted via the cursor.
	key := c.currentIter.Key()
	if bytes.HasPrefix(key, bucketIndexPrefix) {
		str := "buckets may not be deleted from a cursor"
		return makeDbErr(database.ErrIncompatibleValue, str, nil)
	}

	c.bucket.tx.deleteKey(copySlice(key), true)
	return nil
}

// skipPendingUpdates skips any keys at the current database iterator position
// that are being updated by the transaction.  The forwards flag indicates the
// direction the cursor is moving.
func (c *cursor) skipPendingUpdates(forwards bool) {
	for c.dbIter.Valid() {
		var skip bool
		key := c.dbIter.Key()
		if c.bucket.tx.pendingRemove.Has(key) {
			skip = true
		} else if c.bucket.tx.pendingKeys.Has(key) {
			skip = true
		}
		if !skip {
			break
		}

		if forwards {
			c.dbIter.Next()
		} else {
			c.dbIter.Prev()
		}
	}
}

// chooseIterator first skips any entries in the database iterator that are
// being updated by the transaction and sets the current iterator to the
// appropriate iterator depending on their validity and the order they compare
// in while taking into account the direction flag.  When the cursor is being
// moved forwards and both iterators are valid, the iterator with the smaller
// key is chosen and vice versa when the cursor is being moved backwards.
func (c *cursor) chooseIterator(forwards bool) bool {
	// Skip any keys at the current database iterator position that are
	// being updated by the transaction.
	c.skipPendingUpdates(forwards)

	// When both iterators are exhausted, the cursor is exhausted too.
	if !c.dbIter.Valid() && !c.pendingIter.Valid() {
		c.currentIter = nil
		return false
	}

	// Choose the database iterator when the pending keys iterator is
	// exhausted.
	if !c.pendingIter.Valid() {
		c.currentIter = c.dbIter
		return true
	}

	// Choose the pending keys iterator when the database iterator is
	// exhausted.
	if !c.dbIter.Valid() {
		c.currentIter = c.pendingIter
		return true
	}

	// Both iterators are valid, so choose the iterator with either the
	// smaller or larger key depending on the forwards flag.
	compare := bytes.Compare(c.dbIter.Key(), c.pendingIter.Key())
	if (forwards && compare > 0) || (!forwards && compare < 0) {
		c.currentIter = c.pendingIter
	} else {
		c.currentIter = c.dbIter
	}
	return true
}

// First positions the cursor at the first key/value pair and returns whether or
// not the pair exists.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) First() bool {
	// Ensure transaction state is valid.
	if err := c.bucket.tx.checkClosed(); err != nil {
		return false
	}

	// Seek to the first key in both the database and pending iterators and
	// choose the iterator that is both valid and has the smaller key.
	c.dbIter.First()
	c.pendingIter.First()
	return c.chooseIterator(true)
}

// Last positions the cursor at the last key/value pair and returns whether or
// not the pair exists.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Last() bool {
	// Ensure transaction state is valid.
	if err := c.bucket.tx.checkClosed(); err != nil {
		return false
	}

	// Seek to the last key in both the database and pending iterators and
	// choose the iterator that is both valid and has the larger key.
	c.dbIter.Last()
	c.pendingIter.Last()
	return c.chooseIterator(false)
}

// Next moves the cursor one key/value pair forward and returns whether or not
// the pair exists.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Next() bool {
	// Ensure transaction state is valid.
	if err := c.bucket.tx.checkClosed(); err != nil {
		return false
	}

	// Nothing to return if cursor is exhausted.
	if c.currentIter == nil {
		return false
	}

	// Move the current iterator to the next entry and choose the iterator
	// that is both valid and has the smaller key.
	c.currentIter.Next()
	return c.chooseIterator(true)
}

// Prev moves the cursor one key/value pair backward and returns whether or not
// the pair exists.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Prev() bool {
	// Ensure transaction state is valid.
	if err := c.bucket.tx.checkClosed(); err != nil {
		return false
	}

	// Nothing to return if cursor is exhausted.
	if c.currentIter == nil {
		return false
	}

	// Move the current iterator to the previous entry and choose the
	// iterator that is both valid and has the larger key.
	c.currentIter.Prev()
	return c.chooseIterator(false)
}

// Seek positions the cursor at the first key/value pair that is greater than or
// equal to the passed seek key.  Returns false if no suitable key was found.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Seek(seek []byte) bool {
	// Ensure transaction state is valid.
	if err := c.bucket.tx.checkClosed(); err != nil {
		return false
	}

	// Seek to the provided key in both the database and pending iterators
	// then choose the iterator that is both valid and has the larger key.
	seekKey := bucketizedKey(c.bucket.id, seek)
	c.dbIter.Seek(seekKey)
	c.pendingIter.Seek(seekKey)
	return c.chooseIterator(true)
}

// rawKey returns the current key the cursor is pointing to without stripping
// the current bucket prefix or bucket index prefix.
func (c *cursor) rawKey() []byte {
	// Nothing to return if cursor is exhausted.
	if c.currentIter == nil {
		return nil
	}

	return copySlice(c.currentIter.Key())
}

// Key returns the current key the cursor is pointing to.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Key() []byte {
	// Ensure transaction state is valid.
	if err := c.bucket.tx.checkClosed(); err != nil {
		return nil
	}

	// Nothing to return if cursor is exhausted.
	if c.currentIter == nil {
		return nil
	}

	// Slice out the actual key name and make a copy since it is no longer
	// valid after iterating to the next item.
	//
	// The key is after the bucket index prefix and parent ID when the
	// cursor is pointing to a nested bucket.
	key := c.currentIter.Key()
	if bytes.HasPrefix(key, bucketIndexPrefix) {
		key = key[len(bucketIndexPrefix)+4:]
		return copySlice(key)
	}

	// The key is after the bucket ID when the cursor is pointing to a
	// normal entry.
	key = key[len(c.bucket.id):]
	return copySlice(key)
}

// rawValue returns the current value the cursor is pointing to without
// stripping without filtering bucket index values.
func (c *cursor) rawValue() []byte {
	// Nothing to return if cursor is exhausted.
	if c.currentIter == nil {
		return nil
	}

	return copySlice(c.currentIter.Value())
}

// Value returns the current value the cursor is pointing to.  This will be nil
// for nested buckets.
//
// This function is part of the database.Cursor interface implementation.
func (c *cursor) Value() []byte {
	// Ensure transaction state is valid.
	if err := c.bucket.tx.checkClosed(); err != nil {
		return nil
	}

	// Nothing to return if cursor is exhausted.
	if c.currentIter == nil {
		return nil
	}

	// Return nil for the value when the cursor is pointing to a nested
	// bucket.
	if bytes.HasPrefix(c.currentIter.Key(), bucketIndexPrefix) {
		return nil
	}

	return copySlice(c.currentIter.Value())
}

// cursorType defines the type of cursor to create.
type cursorType int

// The following constants define the allowed cursor types.
const (
	// ctKeys iterates through all of the keys in a given bucket.
	ctKeys cursorType = iota

	// ctBuckets iterates through all directly nested buckets in a given
	// bucket.
	ctBuckets

	// ctFull iterates through both the keys and the directly nested buckets
	// in a given bucket.
	ctFull
)

// cursorFinalizer is either invoked when a cursor is being garbage collected or
// called manually to ensure the underlying cursor iterators are released.
func cursorFinalizer(c *cursor) {
	c.dbIter.Release()
	c.pendingIter.Release()
}

// newCursor returns a new cursor for the given bucket, bucket ID, and cursor
// type.
//
// NOTE: The caller is responsible for calling the cursorFinalizer function on
// the returned cursor.
func newCursor(b *bucket, bucketID []byte, cursorTyp cursorType) *cursor {
	var dbIter, pendingIter iterator.Iterator
	switch cursorTyp {
	case ctKeys:
		keyRange := util.BytesPrefix(bucketID)
		dbIter = b.tx.snapshot.NewIterator(keyRange)
		pendingKeyIter := newLdbTreapIter(b.tx, keyRange)
		pendingIter = pendingKeyIter

	case ctBuckets:
		// The serialized bucket index key format is:
		//   <bucketindexprefix><parentbucketid><bucketname>

		// Create an iterator for the both the database and the pending
		// keys which are prefixed by the bucket index identifier and
		// the provided bucket ID.
		prefix := make([]byte, len(bucketIndexPrefix)+4)
		copy(prefix, bucketIndexPrefix)
		copy(prefix[len(bucketIndexPrefix):], bucketID)
		bucketRange := util.BytesPrefix(prefix)

		dbIter = b.tx.snapshot.NewIterator(bucketRange)
		pendingBucketIter := newLdbTreapIter(b.tx, bucketRange)
		pendingIter = pendingBucketIter

	case ctFull:
		fallthrough
	default:
		// The serialized bucket index key format is:
		//   <bucketindexprefix><parentbucketid><bucketname>
		prefix := make([]byte, len(bucketIndexPrefix)+4)
		copy(prefix, bucketIndexPrefix)
		copy(prefix[len(bucketIndexPrefix):], bucketID)
		bucketRange := util.BytesPrefix(prefix)
		keyRange := util.BytesPrefix(bucketID)

		// Since both keys and buckets are needed from the database,
		// create an individual iterator for each prefix and then create
		// a merged iterator from them.
		dbKeyIter := b.tx.snapshot.NewIterator(keyRange)
		dbBucketIter := b.tx.snapshot.NewIterator(bucketRange)
		iters := []iterator.Iterator{dbKeyIter, dbBucketIter}
		dbIter = iterator.NewMergedIterator(iters,
			comparer.DefaultComparer, true)

		// Since both keys and buckets are needed from the pending keys,
		// create an individual iterator for each prefix and then create
		// a merged iterator from them.
		pendingKeyIter := newLdbTreapIter(b.tx, keyRange)
		pendingBucketIter := newLdbTreapIter(b.tx, bucketRange)
		iters = []iterator.Iterator{pendingKeyIter, pendingBucketIter}
		pendingIter = iterator.NewMergedIterator(iters,
			comparer.DefaultComparer, true)
	}

	// Create the cursor using the iterators.
	return &cursor{bucket: b, dbIter: dbIter, pendingIter: pendingIter}
}

// bucket is an internal type used to represent a collection of key/value pairs
// and implements the database.Bucket interface.
type bucket struct {
	tx *transaction
	id [4]byte
}

// Enforce bucket implements the database.Bucket interface.
var _ database.Bucket = (*bucket)(nil)

// bucketIndexKey returns the actual key to use for storing and retrieving a
// child bucket in the bucket index.  This is required because additional
// information is needed to distinguish nested buckets with the same name.
func bucketIndexKey(parentID [4]byte, key []byte) []byte {
	// The serialized bucket index key format is:
	//   <bucketindexprefix><parentbucketid><bucketname>
	indexKey := make([]byte, len(bucketIndexPrefix)+4+len(key))
	copy(indexKey, bucketIndexPrefix)
	copy(indexKey[len(bucketIndexPrefix):], parentID[:])
	copy(indexKey[len(bucketIndexPrefix)+4:], key)
	return indexKey
}

// bucketizedKey returns the actual key to use for storing and retrieving a key
// for the provided bucket ID.  This is required because bucketizing is handled
// through the use of a unique prefix per bucket.
func bucketizedKey(bucketID [4]byte, key []byte) []byte {
	// The serialized block index key format is:
	//   <bucketid><key>
	bKey := make([]byte, 4+len(key))
	copy(bKey, bucketID[:])
	copy(bKey[4:], key)
	return bKey
}

// Bucket retrieves a nested bucket with the given key.  Returns nil if
// the bucket does not exist.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) Bucket(key []byte) database.Bucket {
	// Ensure transaction state is valid.
	if err := b.tx.checkClosed(); err != nil {
		return nil
	}

	// Attempt to fetch the ID for the child bucket.  The bucket does not
	// exist if the bucket index entry does not exist.
	childID := b.tx.fetchKey(bucketIndexKey(b.id, key))
	if childID == nil {
		return nil
	}

	childBucket := &bucket{tx: b.tx}
	copy(childBucket.id[:], childID)
	return childBucket
}

// CreateBucket creates and returns a new nested bucket with the given key.
//
// Returns the following errors as required by the interface contract:
//   - ErrBucketExists if the bucket already exists
//   - ErrBucketNameRequired if the key is empty
//   - ErrIncompatibleValue if the key is otherwise invalid for the particular
//     implementation
//   - ErrTxNotWritable if attempted against a read-only transaction
//   - ErrTxClosed if the transaction has already been closed
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) CreateBucket(key []byte) (database.Bucket, error) {
	// Ensure transaction state is valid.
	if err := b.tx.checkClosed(); err != nil {
		return nil, err
	}

	// Ensure the transaction is writable.
	if !b.tx.writable {
		str := "create bucket requires a writable database transaction"
		return nil, makeDbErr(database.ErrTxNotWritable, str, nil)
	}

	// Ensure a key was provided.
	if len(key) == 0 {
		str := "create bucket requires a key"
		return nil, makeDbErr(database.ErrBucketNameRequired, str, nil)
	}

	// Ensure bucket does not already exist.
	bidxKey := bucketIndexKey(b.id, key)
	if b.tx.hasKey(bidxKey) {
		str := "bucket already exists"
		return nil, makeDbErr(database.ErrBucketExists, str, nil)
	}

	// Find the appropriate next bucket ID to use for the new bucket.  In
	// the case of the special internal block index, keep the fixed ID.
	var childID [4]byte
	if b.id == metadataBucketID && bytes.Equal(key, blockIdxBucketName) {
		childID = blockIdxBucketID
	} else {
		var err error
		childID, err = b.tx.nextBucketID()
		if err != nil {
			return nil, err
		}
	}

	// Add the new bucket to the bucket index.
	if err := b.tx.putKey(bidxKey, childID[:]); err != nil {
		str := fmt.Sprintf("failed to create bucket with key %q", key)
		return nil, convertErr(str, err)
	}
	return &bucket{tx: b.tx, id: childID}, nil
}

// CreateBucketIfNotExists creates and returns a new nested bucket with the
// given key if it does not already exist.
//
// Returns the following errors as required by the interface contract:
//   - ErrBucketNameRequired if the key is empty
//   - ErrIncompatibleValue if the key is otherwise invalid for the particular
//     implementation
//   - ErrTxNotWritable if attempted against a read-only transaction
//   - ErrTxClosed if the transaction has already been closed
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) CreateBucketIfNotExists(key []byte) (database.Bucket, error) {
	// Ensure transaction state is valid.
	if err := b.tx.checkClosed(); err != nil {
		return nil, err
	}

	// Ensure the transaction is writable.
	if !b.tx.writable {
		str := "create bucket requires a writable database transaction"
		return nil, makeDbErr(database.ErrTxNotWritable, str, nil)
	}

	// Return existing bucket if it already exists, otherwise create it.
	if bucket := b.Bucket(key); bucket != nil {
		return bucket, nil
	}
	return b.CreateBucket(key)
}

// DeleteBucket removes a nested bucket with the given key.
//
// Returns the following errors as required by the interface contract:
//   - ErrBucketNotFound if the specified bucket does not exist
//   - ErrTxNotWritable if attempted against a read-only transaction
//   - ErrTxClosed if the transaction has already been closed
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) DeleteBucket(key []byte) error {
	// Ensure transaction state is valid.
	if err := b.tx.checkClosed(); err != nil {
		return err
	}

	// Ensure the transaction is writable.
	if !b.tx.writable {
		str := "delete bucket requires a writable database transaction"
		return makeDbErr(database.ErrTxNotWritable, str, nil)
	}

	// Attempt to fetch the ID for the child bucket.  The bucket does not
	// exist if the bucket index entry does not exist.  In the case of the
	// special internal block index, keep the fixed ID.
	bidxKey := bucketIndexKey(b.id, key)
	childID := b.tx.fetchKey(bidxKey)
	if childID == nil {
		str := fmt.Sprintf("bucket %q does not exist", key)
		return makeDbErr(database.ErrBucketNotFound, str, nil)
	}

	// Remove all nested buckets and their keys.
	childIDs := [][]byte{childID}
	for len(childIDs) > 0 {
		childID = childIDs[len(childIDs)-1]
		childIDs = childIDs[:len(childIDs)-1]

		// Delete all keys in the nested bucket.
		keyCursor := newCursor(b, childID, ctKeys)
		for ok := keyCursor.First(); ok; ok = keyCursor.Next() {
			b.tx.deleteKey(keyCursor.rawKey(), false)
		}
		cursorFinalizer(keyCursor)

		// Iterate through all nested buckets.
		bucketCursor := newCursor(b, childID, ctBuckets)
		for ok := bucketCursor.First(); ok; ok = bucketCursor.Next() {
			// Push the id of the nested bucket onto the stack for
			// the next iteration.
			childID := bucketCursor.rawValue()
			childIDs = append(childIDs, childID)

			// Remove the nested bucket from the bucket index.
			b.tx.deleteKey(bucketCursor.rawKey(), false)
		}
		cursorFinalizer(bucketCursor)
	}

	// Remove the nested bucket from the bucket index.  Any buckets nested
	// under it were already removed above.
	b.tx.deleteKey(bidxKey, true)
	return nil
}

// Cursor returns a new cursor, allowing for iteration over the bucket's
// key/value pairs and nested buckets in forward or backward order.
//
// You must seek to a position using the First, Last, or Seek functions before
// calling the Next, Prev, Key, or Value functions.  Failure to do so will
// result in the same return values as an exhausted cursor, which is false for
// the Prev and Next functions and nil for Key and Value functions.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) Cursor() database.Cursor {
	// Ensure transaction state is valid.
	if err := b.tx.checkClosed(); err != nil {
		return &cursor{bucket: b}
	}

	// Create the cursor and setup a runtime finalizer to ensure the
	// iterators are released when the cursor is garbage collected.
	c := newCursor(b, b.id[:], ctFull)
	runtime.SetFinalizer(c, cursorFinalizer)
	return c
}

// ForEach invokes the passed function with every key/value pair in the bucket.
// This does not include nested buckets or the key/value pairs within those
// nested buckets.
//
// WARNING: It is not safe to mutate data while iterating with this method.
// Doing so may cause the underlying cursor to be invalidated and return
// unexpected keys and/or values.
//
// Returns the following errors as required by the interface contract:
//   - ErrTxClosed if the transaction has already been closed
//
// NOTE: The values returned by this function are only valid during a
// transaction.  Attempting to access them after a transaction has ended will
// likely result in an access violation.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) ForEach(fn func(k, v []byte) error) error {
	// Ensure transaction state is valid.
	if err := b.tx.checkClosed(); err != nil {
		return err
	}

	// Invoke the callback for each cursor item.  Return the error returned
	// from the callback when it is non-nil.
	c := newCursor(b, b.id[:], ctKeys)
	defer cursorFinalizer(c)
	for ok := c.First(); ok; ok = c.Next() {
		err := fn(c.Key(), c.Value())
		if err != nil {
			return err
		}
	}

	return nil
}

// ForEachBucket invokes the passed function with the key of every nested bucket
// in the current bucket.  This does not include any nested buckets within those
// nested buckets.
//
// WARNING: It is not safe to mutate data while iterating with this method.
// Doing so may cause the underlying cursor to be invalidated and return
// unexpected keys.
//
// Returns the following errors as required by the interface contract:
//   - ErrTxClosed if the transaction has already been closed
//
// NOTE: The values returned by this function are only valid during a
// transaction.  Attempting to access them after a transaction has ended will
// likely result in an access violation.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) ForEachBucket(fn func(k []byte) error) error {
	// Ensure transaction state is valid.
	if err := b.tx.checkClosed(); err != nil {
		return err
	}

	// Invoke the callback for each cursor item.  Return the error returned
	// from the callback when it is non-nil.
	c := newCursor(b, b.id[:], ctBuckets)
	defer cursorFinalizer(c)
	for ok := c.First(); ok; ok = c.Next() {
		err := fn(c.Key())
		if err != nil {
			return err
		}
	}

	return nil
}

// Writable returns whether or not the bucket is writable.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) Writable() bool {
	return b.tx.writable
}

// Put saves the specified key/value pair to the bucket.  Keys that do not
// already exist are added and keys that already exist are overwritten.
//
// Returns the following errors as required by the interface contract:
//   - ErrKeyRequired if the key is empty
//   - ErrIncompatibleValue if the key is the same as an existing bucket
//   - ErrTxNotWritable if attempted against a read-only transaction
//   - ErrTxClosed if the transaction has already been closed
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) Put(key, value []byte) error {
	// Ensure transaction state is valid.
	if err := b.tx.checkClosed(); err != nil {
		return err
	}

	// Ensure the transaction is writable.
	if !b.tx.writable {
		str := "setting a key requires a writable database transaction"
		return makeDbErr(database.ErrTxNotWritable, str, nil)
	}

	// Ensure a key was provided.
	if len(key) == 0 {
		str := "put requires a key"
		return makeDbErr(database.ErrKeyRequired, str, nil)
	}

	return b.tx.putKey(bucketizedKey(b.id, key), value)
}

// Get returns the value for the given key.  Returns nil if the key does not
// exist in this bucket.  An empty slice is returned for keys that exist but
// have no value assigned.
//
// NOTE: The value returned by this function is only valid during a transaction.
// Attempting to access it after a transaction has ended results in undefined
// behavior.  Additionally, the value must NOT be modified by the caller.
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) Get(key []byte) []byte {
	// Ensure transaction state is valid.
	if err := b.tx.checkClosed(); err != nil {
		return nil
	}

	// Nothing to return if there is no key.
	if len(key) == 0 {
		return nil
	}

	return b.tx.fetchKey(bucketizedKey(b.id, key))
}

// Delete removes the specified key from the bucket.  Deleting a key that does
// not exist does not return an error.
//
// Returns the following errors as required by the interface contract:
//   - ErrKeyRequired if the key is empty
//   - ErrIncompatibleValue if the key is the same as an existing bucket
//   - ErrTxNotWritable if attempted against a read-only transaction
//   - ErrTxClosed if the transaction has already been closed
//
// This function is part of the database.Bucket interface implementation.
func (b *bucket) Delete(key []byte) error {
	// Ensure transaction state is valid.
	if err := b.tx.checkClosed(); err != nil {
		return err
	}

	// Ensure the transaction is writable.
	if !b.tx.writable {
		str := "deleting a value requires a writable database transaction"
		return makeDbErr(database.ErrTxNotWritable, str, nil)
	}

	// Nothing to do if there is no key.
	if len(key) == 0 {
		return nil
	}

	b.tx.deleteKey(bucketizedKey(b.id, key), true)
	return nil
}

// pendingBlock houses a block that will be written to disk when the database
// transaction is committed.
type pendingBlock struct {
	hash  *chainhash.Hash
	bytes []byte
}

// transaction represents a database transaction.  It can either be read-only or
// read-write and implements the database.Bucket interface.  The transaction
// provides a root bucket against which all read and writes occur.
type transaction struct {
	managed        bool             // Is the transaction managed?
	closed         bool             // Is the transaction closed?
	writable       bool             // Is the transaction writable?
	db             *db              // DB instance the tx was created from.
	snapshot       *dbCacheSnapshot // Underlying snapshot for txns.
	metaBucket     *bucket          // The root metadata bucket.
	blockIdxBucket *bucket          // The block index bucket.

	// Blocks that need to be stored on commit.  The pendingBlocks map is
	// kept to allow quick lookups of pending data by block hash.
	pendingBlocks    map[chainhash.Hash]int
	pendingBlockData []pendingBlock

	// Keys that need to be stored or deleted on commit.
	pendingKeys   *treap.Mutable
	pendingRemove *treap.Mutable

	// Active iterators that need to be notified when the pending keys have
	// been updated so the cursors can properly handle updates to the
	// transaction state.
	activeIterLock sync.RWMutex
	activeIters    []*treap.Iterator
}

// Enforce transaction implements the database.Tx interface.
var _ database.Tx = (*transaction)(nil)

// removeActiveIter removes the passed iterator from the list of active
// iterators against the pending keys treap.
func (tx *transaction) removeActiveIter(iter *treap.Iterator) {
	// An indexing for loop is intentionally used over a range here as range
	// does not reevaluate the slice on each iteration nor does it adjust
	// the index for the modified slice.
	tx.activeIterLock.Lock()
	for i := 0; i < len(tx.activeIters); i++ {
		if tx.activeIters[i] == iter {
			copy(tx.activeIters[i:], tx.activeIters[i+1:])
			tx.activeIters[len(tx.activeIters)-1] = nil
			tx.activeIters = tx.activeIters[:len(tx.activeIters)-1]
		}
	}
	tx.activeIterLock.Unlock()
}

// addActiveIter adds the passed iterator to the list of active iterators for
// the pending keys treap.
func (tx *transaction) addActiveIter(iter *treap.Iterator) {
	tx.activeIterLock.Lock()
	tx.activeIters = append(tx.activeIters, iter)
	tx.activeIterLock.Unlock()
}

// notifyActiveIters notifies all of the active iterators for the pending keys
// treap that it has been updated.
func (tx *transaction) notifyActiveIters() {
	tx.activeIterLock.RLock()
	for _, iter := range tx.activeIters {
		iter.ForceReseek()
	}
	tx.activeIterLock.RUnlock()
}

// checkClosed returns an error if the the database or transaction is closed.
func (tx *transaction) checkClosed() error {
	// The transaction is no longer valid if it has been closed.
	if tx.closed {
		return makeDbErr(database.ErrTxClosed, errTxClosedStr, nil)
	}

	return nil
}

// hasKey returns whether or not the provided key exists in the database while
// taking into account the current transaction state.
func (tx *transaction) hasKey(key []byte) bool {
	// When the transaction is writable, check the pending transaction
	// state first.
	if tx.writable {
		if tx.pendingRemove.Has(key) {
			return false
		}
		if tx.pendingKeys.Has(key) {
			return true
		}
	}

	// Consult the database cache and underlying database.
	return tx.snapshot.Has(key)
}

// putKey adds the provided key to the list of keys to be updated in the
// database when the transaction is committed.
//
// NOTE: This function must only be called on a writable transaction.  Since it
// is an internal helper function, it does not check.
func (tx *transaction) putKey(key, value []byte) error {
	// Prevent the key from being deleted if it was previously scheduled
	// to be deleted on transaction commit.
	tx.pendingRemove.Delete(key)

	// Add the key/value pair to the list to be written on transaction
	// commit.
	tx.pendingKeys.Put(key, value)
	tx.notifyActiveIters()
	return nil
}

// fetchKey attempts to fetch the provided key from the database cache (and
// hence underlying database) while taking into account the current transaction
// state.  Returns nil if the key does not exist.
func (tx *transaction) fetchKey(key []byte) []byte {
	// When the transaction is writable, check the pending transaction
	// state first.
	if tx.writable {
		if tx.pendingRemove.Has(key) {
			return nil
		}
		if value := tx.pendingKeys.Get(key); value != nil {
			return value
		}
	}

	// Consult the database cache and underlying database.
	return tx.snapshot.Get(key)
}

// deleteKey adds the provided key to the list of keys to be deleted from the
// database when the transaction is committed.  The notify iterators flag is
// useful to delay notifying iterators about the changes during bulk deletes.
//
// NOTE: This function must only be called on a writable transaction.  Since it
// is an internal helper function, it does not check.
func (tx *transaction) deleteKey(key []byte, notifyIterators bool) {
	// Remove the key from the list of pendings keys to be written on
	// transaction commit if needed.
	tx.pendingKeys.Delete(key)

	// Add the key to the list to be deleted on transaction	commit.
	tx.pendingRemove.Put(key, nil)

	// Notify the active iterators about the change if the flag is set.
	if notifyIterators {
		tx.notifyActiveIters()
	}
}

// nextBucketID returns the next bucket ID to use for creating a new bucket.
//
// NOTE: This function must only be called on a writable transaction.  Since it
// is an internal helper function, it does not check.
func (tx *transaction) nextBucketID() ([4]byte, error) {
	// Load the currently highest used bucket ID.
	curIDBytes := tx.fetchKey(curBucketIDKeyName)
	curBucketNum := binary.BigEndian.Uint32(curIDBytes)

	// Increment and update the current bucket ID and return it.
	var nextBucketID [4]byte
	binary.BigEndian.PutUint32(nextBucketID[:], curBucketNum+1)
	if err := tx.putKey(curBucketIDKeyName, nextBucketID[:]); err != nil {
		return [4]byte{}, err
	}
	return nextBucketID, nil
}

// Metadata returns the top-most bucket for all metadata storage.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) Metadata() database.Bucket {
	return tx.metaBucket
}

// hasBlock returns whether or not a block with the given hash exists.
func (tx *transaction) hasBlock(hash *chainhash.Hash) bool {
	// Return true if the block is pending to be written on commit since
	// it exists from the viewpoint of this transaction.
	if _, exists := tx.pendingBlocks[*hash]; exists {
		return true
	}

	return tx.hasKey(bucketizedKey(blockIdxBucketID, hash[:]))
}

// StoreBlock stores the provided block into the database.  There are no checks
// to ensure the block connects to a previous block, contains double spends, or
// any additional functionality such as transaction indexing.  It simply stores
// the block in the database.
//
// Returns the following errors as required by the interface contract:
//   - ErrBlockExists when the block hash already exists
//   - ErrTxNotWritable if attempted against a read-only transaction
//   - ErrTxClosed if the transaction has already been closed
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) StoreBlock(block *btcutil.Block) error {
	// Ensure transaction state is valid.
	if err := tx.checkClosed(); err != nil {
		return err
	}

	// Ensure the transaction is writable.
	if !tx.writable {
		str := "store block requires a writable database transaction"
		return makeDbErr(database.ErrTxNotWritable, str, nil)
	}

	// Reject the block if it already exists.
	blockHash := block.Hash()
	if tx.hasBlock(blockHash) {
		str := fmt.Sprintf("block %s already exists", blockHash)
		return makeDbErr(database.ErrBlockExists, str, nil)
	}

	blockBytes, err := block.Bytes()
	if err != nil {
		str := fmt.Sprintf("failed to get serialized bytes for block %s",
			blockHash)
		return makeDbErr(database.ErrDriverSpecific, str, err)
	}

	// Add the block to be stored to the list of pending blocks to store
	// when the transaction is committed.  Also, add it to pending blocks
	// map so it is easy to determine the block is pending based on the
	// block hash.
	if tx.pendingBlocks == nil {
		tx.pendingBlocks = make(map[chainhash.Hash]int)
	}
	tx.pendingBlocks[*blockHash] = len(tx.pendingBlockData)
	tx.pendingBlockData = append(tx.pendingBlockData, pendingBlock{
		hash:  blockHash,
		bytes: blockBytes,
	})
	log.Tracef("Added block %s to pending blocks", blockHash)

	return nil
}

// HasBlock returns whether or not a block with the given hash exists in the
// database.
//
// Returns the following errors as required by the interface contract:
//   - ErrTxClosed if the transaction has already been closed
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) HasBlock(hash *chainhash.Hash) (bool, error) {
	// Ensure transaction state is valid.
	if err := tx.checkClosed(); err != nil {
		return false, err
	}

	return tx.hasBlock(hash), nil
}

// HasBlocks returns whether or not the blocks with the provided hashes
// exist in the database.
//
// Returns the following errors as required by the interface contract:
//   - ErrTxClosed if the transaction has already been closed
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) HasBlocks(hashes []chainhash.Hash) ([]bool, error) {
	// Ensure transaction state is valid.
	if err := tx.checkClosed(); err != nil {
		return nil, err
	}

	results := make([]bool, len(hashes))
	for i := range hashes {
		results[i] = tx.hasBlock(&hashes[i])
	}

	return results, nil
}

// fetchBlockRow fetches the metadata stored in the block index for the provided
// hash.  It will return ErrBlockNotFound if there is no entry.
func (tx *transaction) fetchBlockRow(hash *chainhash.Hash) ([]byte, error) {
	blockRow := tx.blockIdxBucket.Get(hash[:])
	if blockRow == nil {
		str := fmt.Sprintf("block %s does not exist", hash)
		return nil, makeDbErr(database.ErrBlockNotFound, str, nil)
	}

	return blockRow, nil
}

// FetchBlockHeader returns the raw serialized bytes for the block header
// identified by the given hash.  The raw bytes are in the format returned by
// Serialize on a wire.BlockHeader.
//
// Returns the following errors as required by the interface contract:
//   - ErrBlockNotFound if the requested block hash does not exist
//   - ErrTxClosed if the transaction has already been closed
//   - ErrCorruption if the database has somehow become corrupted
//
// NOTE: The data returned by this function is only valid during a
// database transaction.  Attempting to access it after a transaction
// has ended results in undefined behavior.  This constraint prevents
// additional data copies and allows support for memory-mapped database
// implementations.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) FetchBlockHeader(hash *chainhash.Hash) ([]byte, error) {
	return tx.FetchBlockRegion(&database.BlockRegion{
		Hash:   hash,
		Offset: 0,
		Len:    blockHdrSize,
	})
}

// FetchBlockHeaders returns the raw serialized bytes for the block headers
// identified by the given hashes.  The raw bytes are in the format returned by
// Serialize on a wire.BlockHeader.
//
// Returns the following errors as required by the interface contract:
//   - ErrBlockNotFound if the any of the requested block hashes do not exist
//   - ErrTxClosed if the transaction has already been closed
//   - ErrCorruption if the database has somehow become corrupted
//
// NOTE: The data returned by this function is only valid during a database
// transaction.  Attempting to access it after a transaction has ended results
// in undefined behavior.  This constraint prevents additional data copies and
// allows support for memory-mapped database implementations.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) FetchBlockHeaders(hashes []chainhash.Hash) ([][]byte, error) {
	regions := make([]database.BlockRegion, len(hashes))
	for i := range hashes {
		regions[i].Hash = &hashes[i]
		regions[i].Offset = 0
		regions[i].Len = blockHdrSize
	}
	return tx.FetchBlockRegions(regions)
}

// FetchBlock returns the raw serialized bytes for the block identified by the
// given hash.  The raw bytes are in the format returned by Serialize on a
// wire.MsgBlock.
//
// Returns the following errors as required by the interface contract:
//   - ErrBlockNotFound if the requested block hash does not exist
//   - ErrTxClosed if the transaction has already been closed
//   - ErrCorruption if the database has somehow become corrupted
//
// In addition, returns ErrDriverSpecific if any failures occur when reading the
// block files.
//
// NOTE: The data returned by this function is only valid during a database
// transaction.  Attempting to access it after a transaction has ended results
// in undefined behavior.  This constraint prevents additional data copies and
// allows support for memory-mapped database implementations.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) FetchBlock(hash *chainhash.Hash) ([]byte, error) {
	// Ensure transaction state is valid.
	if err := tx.checkClosed(); err != nil {
		return nil, err
	}

	// When the block is pending to be written on commit return the bytes
	// from there.
	if idx, exists := tx.pendingBlocks[*hash]; exists {
		return tx.pendingBlockData[idx].bytes, nil
	}

	// Lookup the location of the block in the files from the block index.
	blockRow, err := tx.fetchBlockRow(hash)
	if err != nil {
		return nil, err
	}
	location := deserializeBlockLoc(blockRow)

	// Read the block from the appropriate location.  The function also
	// performs a checksum over the data to detect data corruption.
	blockBytes, err := tx.db.store.readBlock(hash, location)
	if err != nil {
		return nil, err
	}

	return blockBytes, nil
}

// FetchBlocks returns the raw serialized bytes for the blocks identified by the
// given hashes.  The raw bytes are in the format returned by Serialize on a
// wire.MsgBlock.
//
// Returns the following errors as required by the interface contract:
//   - ErrBlockNotFound if any of the requested block hashed do not exist
//   - ErrTxClosed if the transaction has already been closed
//   - ErrCorruption if the database has somehow become corrupted
//
// In addition, returns ErrDriverSpecific if any failures occur when reading the
// block files.
//
// NOTE: The data returned by this function is only valid during a database
// transaction.  Attempting to access it after a transaction has ended results
// in undefined behavior.  This constraint prevents additional data copies and
// allows support for memory-mapped database implementations.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) FetchBlocks(hashes []chainhash.Hash) ([][]byte, error) {
	// Ensure transaction state is valid.
	if err := tx.checkClosed(); err != nil {
		return nil, err
	}

	// NOTE: This could check for the existence of all blocks before loading
	// any of them which would be faster in the failure case, however
	// callers will not typically be calling this function with invalid
	// values, so optimize for the common case.

	// Load the blocks.
	blocks := make([][]byte, len(hashes))
	for i := range hashes {
		var err error
		blocks[i], err = tx.FetchBlock(&hashes[i])
		if err != nil {
			return nil, err
		}
	}

	return blocks, nil
}

// fetchPendingRegion attempts to fetch the provided region from any block which
// are pending to be written on commit.  It will return nil for the byte slice
// when the region references a block which is not pending.  When the region
// does reference a pending block, it is bounds checked and returns
// ErrBlockRegionInvalid if invalid.
func (tx *transaction) fetchPendingRegion(region *database.BlockRegion) ([]byte, error) {
	// Nothing to do if the block is not pending to be written on commit.
	idx, exists := tx.pendingBlocks[*region.Hash]
	if !exists {
		return nil, nil
	}

	// Ensure the region is within the bounds of the block.
	blockBytes := tx.pendingBlockData[idx].bytes
	blockLen := uint32(len(blockBytes))
	endOffset := region.Offset + region.Len
	if endOffset < region.Offset || endOffset > blockLen {
		str := fmt.Sprintf("block %s region offset %d, length %d "+
			"exceeds block length of %d", region.Hash,
			region.Offset, region.Len, blockLen)
		return nil, makeDbErr(database.ErrBlockRegionInvalid, str, nil)
	}

	// Return the bytes from the pending block.
	return blockBytes[region.Offset:endOffset:endOffset], nil
}

// FetchBlockRegion returns the raw serialized bytes for the given block region.
//
// For example, it is possible to directly extract Bitcoin transactions and/or
// scripts from a block with this function.  Depending on the backend
// implementation, this can provide significant savings by avoiding the need to
// load entire blocks.
//
// The raw bytes are in the format returned by Serialize on a wire.MsgBlock and
// the Offset field in the provided BlockRegion is zero-based and relative to
// the start of the block (byte 0).
//
// Returns the following errors as required by the interface contract:
//   - ErrBlockNotFound if the requested block hash does not exist
//   - ErrBlockRegionInvalid if the region exceeds the bounds of the associated
//     block
//   - ErrTxClosed if the transaction has already been closed
//   - ErrCorruption if the database has somehow become corrupted
//
// In addition, returns ErrDriverSpecific if any failures occur when reading the
// block files.
//
// NOTE: The data returned by this function is only valid during a database
// transaction.  Attempting to access it after a transaction has ended results
// in undefined behavior.  This constraint prevents additional data copies and
// allows support for memory-mapped database implementations.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) FetchBlockRegion(region *database.BlockRegion) ([]byte, error) {
	// Ensure transaction state is valid.
	if err := tx.checkClosed(); err != nil {
		return nil, err
	}

	// When the block is pending to be written on commit return the bytes
	// from there.
	if tx.pendingBlocks != nil {
		regionBytes, err := tx.fetchPendingRegion(region)
		if err != nil {
			return nil, err
		}
		if regionBytes != nil {
			return regionBytes, nil
		}
	}

	// Lookup the location of the block in the files from the block index.
	blockRow, err := tx.fetchBlockRow(region.Hash)
	if err != nil {
		return nil, err
	}
	location := deserializeBlockLoc(blockRow)

	// Ensure the region is within the bounds of the block.
	endOffset := region.Offset + region.Len
	if endOffset < region.Offset || endOffset > location.blockLen {
		str := fmt.Sprintf("block %s region offset %d, length %d "+
			"exceeds block length of %d", region.Hash,
			region.Offset, region.Len, location.blockLen)
		return nil, makeDbErr(database.ErrBlockRegionInvalid, str, nil)

	}

	// Read the region from the appropriate disk block file.
	regionBytes, err := tx.db.store.readBlockRegion(location, region.Offset,
		region.Len)
	if err != nil {
		return nil, err
	}

	return regionBytes, nil
}

// FetchBlockRegions returns the raw serialized bytes for the given block
// regions.
//
// For example, it is possible to directly extract Bitcoin transactions and/or
// scripts from various blocks with this function.  Depending on the backend
// implementation, this can provide significant savings by avoiding the need to
// load entire blocks.
//
// The raw bytes are in the format returned by Serialize on a wire.MsgBlock and
// the Offset fields in the provided BlockRegions are zero-based and relative to
// the start of the block (byte 0).
//
// Returns the following errors as required by the interface contract:
//   - ErrBlockNotFound if any of the request block hashes do not exist
//   - ErrBlockRegionInvalid if one or more region exceed the bounds of the
//     associated block
//   - ErrTxClosed if the transaction has already been closed
//   - ErrCorruption if the database has somehow become corrupted
//
// In addition, returns ErrDriverSpecific if any failures occur when reading the
// block files.
//
// NOTE: The data returned by this function is only valid during a database
// transaction.  Attempting to access it after a transaction has ended results
// in undefined behavior.  This constraint prevents additional data copies and
// allows support for memory-mapped database implementations.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) FetchBlockRegions(regions []database.BlockRegion) ([][]byte, error) {
	// Ensure transaction state is valid.
	if err := tx.checkClosed(); err != nil {
		return nil, err
	}

	// NOTE: This could check for the existence of all blocks before
	// deserializing the locations and building up the fetch list which
	// would be faster in the failure case, however callers will not
	// typically be calling this function with invalid values, so optimize
	// for the common case.

	// NOTE: A potential optimization here would be to combine adjacent
	// regions to reduce the number of reads.

	// In order to improve efficiency of loading the bulk data, first grab
	// the block location for all of the requested block hashes and sort
	// the reads by filenum:offset so that all reads are grouped by file
	// and linear within each file.  This can result in quite a significant
	// performance increase depending on how spread out the requested hashes
	// are by reducing the number of file open/closes and random accesses
	// needed.  The fetchList is intentionally allocated with a cap because
	// some of the regions might be fetched from the pending blocks and
	// hence there is no need to fetch those from disk.
	blockRegions := make([][]byte, len(regions))
	fetchList := make([]bulkFetchData, 0, len(regions))
	for i := range regions {
		region := &regions[i]

		// When the block is pending to be written on commit grab the
		// bytes from there.
		if tx.pendingBlocks != nil {
			regionBytes, err := tx.fetchPendingRegion(region)
			if err != nil {
				return nil, err
			}
			if regionBytes != nil {
				blockRegions[i] = regionBytes
				continue
			}
		}

		// Lookup the location of the block in the files from the block
		// index.
		blockRow, err := tx.fetchBlockRow(region.Hash)
		if err != nil {
			return nil, err
		}
		location := deserializeBlockLoc(blockRow)

		// Ensure the region is within the bounds of the block.
		endOffset := region.Offset + region.Len
		if endOffset < region.Offset || endOffset > location.blockLen {
			str := fmt.Sprintf("block %s region offset %d, length "+
				"%d exceeds block length of %d", region.Hash,
				region.Offset, region.Len, location.blockLen)
			return nil, makeDbErr(database.ErrBlockRegionInvalid, str, nil)
		}

		fetchList = append(fetchList, bulkFetchData{&location, i})
	}
	sort.Sort(bulkFetchDataSorter(fetchList))

	// Read all of the regions in the fetch list and set the results.
	for i := range fetchList {
		fetchData := &fetchList[i]
		ri := fetchData.replyIndex
		region := &regions[ri]
		location := fetchData.blockLocation
		regionBytes, err := tx.db.store.readBlockRegion(*location,
			region.Offset, region.Len)
		if err != nil {
			return nil, err
		}
		blockRegions[ri] = regionBytes
	}

	return blockRegions, nil
}

// close marks the transaction closed then releases any pending data, the
// underlying snapshot, the transaction read lock, and the write lock when the
// transaction is writable.
func (tx *transaction) close() {
	tx.closed = true

	// Clear pending blocks that would have been written on commit.
	tx.pendingBlocks = nil
	tx.pendingBlockData = nil

	// Clear pending keys that would have been written or deleted on commit.
	tx.pendingKeys = nil
	tx.pendingRemove = nil

	// Release the snapshot.
	if tx.snapshot != nil {
		tx.snapshot.Release()
		tx.snapshot = nil
	}

	tx.db.closeLock.RUnlock()

	// Release the writer lock for writable transactions to unblock any
	// other write transaction which are possibly waiting.
	if tx.writable {
		tx.db.writeLock.Unlock()
	}
}

// writePendingAndCommit writes pending block data to the flat block files,
// updates the metadata with their locations as well as the new current write
// location, and commits the metadata to the memory database cache.  It also
// properly handles rollback in the case of failures.
//
// This function MUST only be called when there is pending data to be written.
func (tx *transaction) writePendingAndCommit() error {
	// Save the current block store write position for potential rollback.
	// These variables are only updated here in this function and there can
	// only be one write transaction active at a time, so it's safe to store
	// them for potential rollback.
	wc := tx.db.store.writeCursor
	wc.RLock()
	oldBlkFileNum := wc.curFileNum
	oldBlkOffset := wc.curOffset
	wc.RUnlock()

	// rollback is a closure that is used to rollback all writes to the
	// block files.
	rollback := func() {
		// Rollback any modifications made to the block files if needed.
		tx.db.store.handleRollback(oldBlkFileNum, oldBlkOffset)
	}

	// Loop through all of the pending blocks to store and write them.
	for _, blockData := range tx.pendingBlockData {
		log.Tracef("Storing block %s", blockData.hash)
		location, err := tx.db.store.writeBlock(blockData.bytes)
		if err != nil {
			rollback()
			return err
		}

		// Add a record in the block index for the block.  The record
		// includes the location information needed to locate the block
		// on the filesystem as well as the block header since they are
		// so commonly needed.
		blockRow := serializeBlockLoc(location)
		err = tx.blockIdxBucket.Put(blockData.hash[:], blockRow)
		if err != nil {
			rollback()
			return err
		}
	}

	// Update the metadata for the current write file and offset.
	writeRow := serializeWriteRow(wc.curFileNum, wc.curOffset)
	if err := tx.metaBucket.Put(writeLocKeyName, writeRow); err != nil {
		rollback()
		return convertErr("failed to store write cursor", err)
	}

	// Atomically update the database cache.  The cache automatically
	// handles flushing to the underlying persistent storage database.
	return tx.db.cache.commitTx(tx)
}

// Commit commits all changes that have been made to the root metadata bucket
// and all of its sub-buckets to the database cache which is periodically synced
// to persistent storage.  In addition, it commits all new blocks directly to
// persistent storage bypassing the db cache.  Blocks can be rather large, so
// this help increase the amount of cache available for the metadata updates and
// is safe since blocks are immutable.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) Commit() error {
	// Prevent commits on managed transactions.
	if tx.managed {
		tx.close()
		panic("managed transaction commit not allowed")
	}

	// Ensure transaction state is valid.
	if err := tx.checkClosed(); err != nil {
		return err
	}

	// Regardless of whether the commit succeeds, the transaction is closed
	// on return.
	defer tx.close()

	// Ensure the transaction is writable.
	if !tx.writable {
		str := "Commit requires a writable database transaction"
		return makeDbErr(database.ErrTxNotWritable, str, nil)
	}

	// Write pending data.  The function will rollback if any errors occur.
	return tx.writePendingAndCommit()
}

// Rollback undoes all changes that have been made to the root bucket and all of
// its sub-buckets.
//
// This function is part of the database.Tx interface implementation.
func (tx *transaction) Rollback() error {
	// Prevent rollbacks on managed transactions.
	if tx.managed {
		tx.close()
		panic("managed transaction rollback not allowed")
	}

	// Ensure transaction state is valid.
	if err := tx.checkClosed(); err != nil {
		return err
	}

	tx.close()
	return nil
}

// db represents a collection of namespaces which are persisted and implements
// the database.DB interface.  All database access is performed through
// transactions which are obtained through the specific Namespace.
type db struct {
	writeLock sync.Mutex   // Limit to one write transaction at a time.
	closeLock sync.RWMutex // Make database close block while txns active.
	closed    bool         // Is the database closed?
	store     *blockStore  // Handles read/writing blocks to flat files.
	cache     *dbCache     // Cache layer which wraps underlying leveldb DB.
}

// Enforce db implements the database.DB interface.
var _ database.DB = (*db)(nil)

// Type returns the database driver type the current database instance was
// created with.
//
// This function is part of the database.DB interface implementation.
func (db *db) Type() string {
	return dbType
}

// begin is the implementation function for the Begin database method.  See its
// documentation for more details.
//
// This function is only separate because it returns the internal transaction
// which is used by the managed transaction code while the database method
// returns the interface.
func (db *db) begin(writable bool) (*transaction, error) {
	// Whenever a new writable transaction is started, grab the write lock
	// to ensure only a single write transaction can be active at the same
	// time.  This lock will not be released until the transaction is
	// closed (via Rollback or Commit).
	if writable {
		db.writeLock.Lock()
	}

	// Whenever a new transaction is started, grab a read lock against the
	// database to ensure Close will wait for the transaction to finish.
	// This lock will not be released until the transaction is closed (via
	// Rollback or Commit).
	db.closeLock.RLock()
	if db.closed {
		db.closeLock.RUnlock()
		if writable {
			db.writeLock.Unlock()
		}
		return nil, makeDbErr(database.ErrDbNotOpen, errDbNotOpenStr,
			nil)
	}

	// Grab a snapshot of the database cache (which in turn also handles the
	// underlying database).
	snapshot, err := db.cache.Snapshot()
	if err != nil {
		db.closeLock.RUnlock()
		if writable {
			db.writeLock.Unlock()
		}

		return nil, err
	}

	// The metadata and block index buckets are internal-only buckets, so
	// they have defined IDs.
	tx := &transaction{
		writable:      writable,
		db:            db,
		snapshot:      snapshot,
		pendingKeys:   treap.NewMutable(),
		pendingRemove: treap.NewMutable(),
	}
	tx.metaBucket = &bucket{tx: tx, id: metadataBucketID}
	tx.blockIdxBucket = &bucket{tx: tx, id: blockIdxBucketID}
	return tx, nil
}

// Begin starts a transaction which is either read-only or read-write depending
// on the specified flag.  Multiple read-only transactions can be started
// simultaneously while only a single read-write transaction can be started at a
// time.  The call will block when starting a read-write transaction when one is
// already open.
//
// NOTE: The transaction must be closed by calling Rollback or Commit on it when
// it is no longer needed.  Failure to do so will result in unclaimed memory.
//
// This function is part of the database.DB interface implementation.
func (db *db) Begin(writable bool) (database.Tx, error) {
	return db.begin(writable)
}

// rollbackOnPanic rolls the passed transaction back if the code in the calling
// function panics.  This is needed since the mutex on a transaction must be
// released and a panic in called code would prevent that from happening.
//
// NOTE: This can only be handled manually for managed transactions since they
// control the life-cycle of the transaction.  As the documentation on Begin
// calls out, callers opting to use manual transactions will have to ensure the
// transaction is rolled back on panic if it desires that functionality as well
// or the database will fail to close since the read-lock will never be
// released.
func rollbackOnPanic(tx *transaction) {
	if err := recover(); err != nil {
		tx.managed = false
		_ = tx.Rollback()
		panic(err)
	}
}

// View invokes the passed function in the context of a managed read-only
// transaction with the root bucket for the namespace.  Any errors returned from
// the user-supplied function are returned from this function.
//
// This function is part of the database.DB interface implementation.
func (db *db) View(fn func(database.Tx) error) error {
	// Start a read-only transaction.
	tx, err := db.begin(false)
	if err != nil {
		return err
	}

	// Since the user-provided function might panic, ensure the transaction
	// releases all mutexes and resources.  There is no guarantee the caller
	// won't use recover and keep going.  Thus, the database must still be
	// in a usable state on panics due to caller issues.
	defer rollbackOnPanic(tx)

	tx.managed = true
	err = fn(tx)
	tx.managed = false
	if err != nil {
		// The error is ignored here because nothing was written yet
		// and regardless of a rollback failure, the tx is closed now
		// anyways.
		_ = tx.Rollback()
		return err
	}

	return tx.Rollback()
}

// Update invokes the passed function in the context of a managed read-write
// transaction with the root bucket for the namespace.  Any errors returned from
// the user-supplied function will cause the transaction to be rolled back and
// are returned from this function.  Otherwise, the transaction is committed
// when the user-supplied function returns a nil error.
//
// This function is part of the database.DB interface implementation.
func (db *db) Update(fn func(database.Tx) error) error {
	// Start a read-write transaction.
	tx, err := db.begin(true)
	if err != nil {
		return err
	}

	// Since the user-provided function might panic, ensure the transaction
	// releases all mutexes and resources.  There is no guarantee the caller
	// won't use recover and keep going.  Thus, the database must still be
	// in a usable state on panics due to caller issues.
	defer rollbackOnPanic(tx)

	tx.managed = true
	err = fn(tx)
	tx.managed = false
	if err != nil {
		// The error is ignored here because nothing was written yet
		// and regardless of a rollback failure, the tx is closed now
		// anyways.
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
}

// Close cleanly shuts down the database and syncs all data.  It will block
// until all database transactions have been finalized (rolled back or
// committed).
//
// This function is part of the database.DB interface implementation.
func (db *db) Close() error {
	// Since all transactions have a read lock on this mutex, this will
	// cause Close to wait for all readers to complete.
	db.closeLock.Lock()
	defer db.closeLock.Unlock()

	if db.closed {
		return makeDbErr(database.ErrDbNotOpen, errDbNotOpenStr, nil)
	}
	db.closed = true

	// NOTE: Since the above lock waits for all transactions to finish and
	// prevents any new ones from being started, it is safe to flush the
	// cache and clear all state without the individual locks.

	// Close the database cache which will flush any existing entries to
	// disk and close the underlying leveldb database.  Any error is saved
	// and returned at the end after the remaining cleanup since the
	// database will be marked closed even if this fails given there is no
	// good way for the caller to recover from a failure here anyways.
	closeErr := db.cache.Close()

	// Close any open flat files that house the blocks.
	wc := db.store.writeCursor
	if wc.curFile.file != nil {
		_ = wc.curFile.file.Close()
		wc.curFile.file = nil
	}
	for _, blockFile := range db.store.openBlockFiles {
		_ = blockFile.file.Close()
	}
	db.store.openBlockFiles = nil
	db.store.openBlocksLRU.Init()
	db.store.fileNumToLRUElem = nil

	return closeErr
}

// filesExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// initDB creates the initial buckets and values used by the package.  This is
// mainly in a separate function for testing purposes.
func initDB(ldb *leveldb.DB) error {
	// The starting block file write cursor location is file num 0, offset
	// 0.
	batch := new(leveldb.Batch)
	batch.Put(bucketizedKey(metadataBucketID, writeLocKeyName),
		serializeWriteRow(0, 0))

	// Create block index bucket and set the current bucket id.
	//
	// NOTE: Since buckets are virtualized through the use of prefixes,
	// there is no need to store the bucket index data for the metadata
	// bucket in the database.  However, the first bucket ID to use does
	// need to account for it to ensure there are no key collisions.
	batch.Put(bucketIndexKey(metadataBucketID, blockIdxBucketName),
		blockIdxBucketID[:])
	batch.Put(curBucketIDKeyName, blockIdxBucketID[:])

	// Write everything as a single batch.
	if err := ldb.Write(batch, nil); err != nil {
		str := fmt.Sprintf("failed to initialize metadata database: %v",
			err)
		return convertErr(str, err)
	}

	return nil
}

// openDB opens the database at the provided path.  database.ErrDbDoesNotExist
// is returned if the database doesn't exist and the create flag is not set.
func openDB(dbPath string, network wire.BitcoinNet, create bool) (database.DB, error) {
	// Error if the database doesn't exist and the create flag is not set.
	metadataDbPath := filepath.Join(dbPath, metadataDbName)
	dbExists := fileExists(metadataDbPath)
	if !create && !dbExists {
		str := fmt.Sprintf("database %q does not exist", metadataDbPath)
		return nil, makeDbErr(database.ErrDbDoesNotExist, str, nil)
	}

	// Ensure the full path to the database exists.
	if !dbExists {
		// The error can be ignored here since the call to
		// leveldb.OpenFile will fail if the directory couldn't be
		// created.
		_ = os.MkdirAll(dbPath, 0700)
	}

	// Open the metadata database (will create it if needed).
	opts := opt.Options{
		ErrorIfExist: create,
		Strict:       opt.DefaultStrict,
		Compression:  opt.NoCompression,
		Filter:       filter.NewBloomFilter(10),
	}
	ldb, err := leveldb.OpenFile(metadataDbPath, &opts)
	if err != nil {
		return nil, convertErr(err.Error(), err)
	}

	// Create the block store which includes scanning the existing flat
	// block files to find what the current write cursor position is
	// according to the data that is actually on disk.  Also create the
	// database cache which wraps the underlying leveldb database to provide
	// write caching.
	store := newBlockStore(dbPath, network)
	cache := newDbCache(ldb, store, defaultCacheSize, defaultFlushSecs)
	pdb := &db{store: store, cache: cache}

	// Perform any reconciliation needed between the block and metadata as
	// well as database initialization, if needed.
	return reconcileDB(pdb, create)
}
