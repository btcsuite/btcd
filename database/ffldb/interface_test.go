// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// This file intended to be copied into each backend driver directory.  Each
// driver should have their own driver_test.go file which creates a database and
// invokes the testInterface function in this file to ensure the driver properly
// implements the interface.
//
// NOTE: When copying this file into the backend driver folder, the package name
// will need to be changed accordingly.

package ffldb_test

import (
	"bytes"
	"compress/bzip2"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	"github.com/btcsuite/btcd/wire"
)

var (
	// blockDataNet is the expected network in the test block data.
	blockDataNet = wire.MainNet

	// blockDataFile is the path to a file containing the first 256 blocks
	// of the block chain.
	blockDataFile = filepath.Join("..", "testdata", "blocks1-256.bz2")

	// errSubTestFail is used to signal that a sub test returned false.
	errSubTestFail = fmt.Errorf("sub test failure")
)

// loadBlocks loads the blocks contained in the testdata directory and returns
// a slice of them.
func loadBlocks(t *testing.T, dataFile string, network wire.BitcoinNet) ([]*btcutil.Block, error) {
	// Open the file that contains the blocks for reading.
	fi, err := os.Open(dataFile)
	if err != nil {
		t.Errorf("failed to open file %v, err %v", dataFile, err)
		return nil, err
	}
	defer func() {
		if err := fi.Close(); err != nil {
			t.Errorf("failed to close file %v %v", dataFile,
				err)
		}
	}()
	dr := bzip2.NewReader(fi)

	// Set the first block as the genesis block.
	blocks := make([]*btcutil.Block, 0, 256)
	genesis := btcutil.NewBlock(chaincfg.MainNetParams.GenesisBlock)
	blocks = append(blocks, genesis)

	// Load the remaining blocks.
	for height := 1; ; height++ {
		var net uint32
		err := binary.Read(dr, binary.LittleEndian, &net)
		if err == io.EOF {
			// Hit end of file at the expected offset.  No error.
			break
		}
		if err != nil {
			t.Errorf("Failed to load network type for block %d: %v",
				height, err)
			return nil, err
		}
		if net != uint32(network) {
			t.Errorf("Block doesn't match network: %v expects %v",
				net, network)
			return nil, err
		}

		var blockLen uint32
		err = binary.Read(dr, binary.LittleEndian, &blockLen)
		if err != nil {
			t.Errorf("Failed to load block size for block %d: %v",
				height, err)
			return nil, err
		}

		// Read the block.
		blockBytes := make([]byte, blockLen)
		_, err = io.ReadFull(dr, blockBytes)
		if err != nil {
			t.Errorf("Failed to load block %d: %v", height, err)
			return nil, err
		}

		// Deserialize and store the block.
		block, err := btcutil.NewBlockFromBytes(blockBytes)
		if err != nil {
			t.Errorf("Failed to parse block %v: %v", height, err)
			return nil, err
		}
		blocks = append(blocks, block)
	}

	return blocks, nil
}

// checkDbError ensures the passed error is a database.Error with an error code
// that matches the passed  error code.
func checkDbError(t *testing.T, testName string, gotErr error, wantErrCode database.ErrorCode) bool {
	dbErr, ok := gotErr.(database.Error)
	if !ok {
		t.Errorf("%s: unexpected error type - got %T, want %T",
			testName, gotErr, database.Error{})
		return false
	}
	if dbErr.ErrorCode != wantErrCode {
		t.Errorf("%s: unexpected error code - got %s (%s), want %s",
			testName, dbErr.ErrorCode, dbErr.Description,
			wantErrCode)
		return false
	}

	return true
}

// testContext is used to store context information about a running test which
// is passed into helper functions.
type testContext struct {
	t           *testing.T
	db          database.DB
	bucketDepth int
	isWritable  bool
	blocks      []*btcutil.Block
}

// keyPair houses a key/value pair.  It is used over maps so ordering can be
// maintained.
type keyPair struct {
	key   []byte
	value []byte
}

// lookupKey is a convenience method to lookup the requested key from the
// provided keypair slice along with whether or not the key was found.
func lookupKey(key []byte, values []keyPair) ([]byte, bool) {
	for _, item := range values {
		if bytes.Equal(item.key, key) {
			return item.value, true
		}
	}

	return nil, false
}

// toGetValues returns a copy of the provided keypairs with all of the nil
// values set to an empty byte slice.  This is used to ensure that keys set to
// nil values result in empty byte slices when retrieved instead of nil.
func toGetValues(values []keyPair) []keyPair {
	ret := make([]keyPair, len(values))
	copy(ret, values)
	for i := range ret {
		if ret[i].value == nil {
			ret[i].value = make([]byte, 0)
		}
	}
	return ret
}

// rollbackValues returns a copy of the provided keypairs with all values set to
// nil.  This is used to test that values are properly rolled back.
func rollbackValues(values []keyPair) []keyPair {
	ret := make([]keyPair, len(values))
	copy(ret, values)
	for i := range ret {
		ret[i].value = nil
	}
	return ret
}

// testCursorKeyPair checks that the provide key and value match the expected
// keypair at the provided index.  It also ensures the index is in range for the
// provided slice of expected keypairs.
func testCursorKeyPair(tc *testContext, k, v []byte, index int, values []keyPair) bool {
	if index >= len(values) || index < 0 {
		tc.t.Errorf("Cursor: exceeded the expected range of values - "+
			"index %d, num values %d", index, len(values))
		return false
	}

	pair := &values[index]
	if !bytes.Equal(k, pair.key) {
		tc.t.Errorf("Mismatched cursor key: index %d does not match "+
			"the expected key - got %q, want %q", index, k,
			pair.key)
		return false
	}
	if !bytes.Equal(v, pair.value) {
		tc.t.Errorf("Mismatched cursor value: index %d does not match "+
			"the expected value - got %q, want %q", index, v,
			pair.value)
		return false
	}

	return true
}

// testGetValues checks that all of the provided key/value pairs can be
// retrieved from the database and the retrieved values match the provided
// values.
func testGetValues(tc *testContext, bucket database.Bucket, values []keyPair) bool {
	for _, item := range values {
		gotValue := bucket.Get(item.key)
		if !reflect.DeepEqual(gotValue, item.value) {
			tc.t.Errorf("Get: unexpected value for %q - got %q, "+
				"want %q", item.key, gotValue, item.value)
			return false
		}
	}

	return true
}

// testPutValues stores all of the provided key/value pairs in the provided
// bucket while checking for errors.
func testPutValues(tc *testContext, bucket database.Bucket, values []keyPair) bool {
	for _, item := range values {
		if err := bucket.Put(item.key, item.value); err != nil {
			tc.t.Errorf("Put: unexpected error: %v", err)
			return false
		}
	}

	return true
}

// testDeleteValues removes all of the provided key/value pairs from the
// provided bucket.
func testDeleteValues(tc *testContext, bucket database.Bucket, values []keyPair) bool {
	for _, item := range values {
		if err := bucket.Delete(item.key); err != nil {
			tc.t.Errorf("Delete: unexpected error: %v", err)
			return false
		}
	}

	return true
}

// testCursorInterface ensures the cursor itnerface is working properly by
// exercising all of its functions on the passed bucket.
func testCursorInterface(tc *testContext, bucket database.Bucket) bool {
	// Ensure a cursor can be obtained for the bucket.
	cursor := bucket.Cursor()
	if cursor == nil {
		tc.t.Error("Bucket.Cursor: unexpected nil cursor returned")
		return false
	}

	// Ensure the cursor returns the same bucket it was created for.
	if cursor.Bucket() != bucket {
		tc.t.Error("Cursor.Bucket: does not match the bucket it was " +
			"created for")
		return false
	}

	if tc.isWritable {
		unsortedValues := []keyPair{
			{[]byte("cursor"), []byte("val1")},
			{[]byte("abcd"), []byte("val2")},
			{[]byte("bcd"), []byte("val3")},
			{[]byte("defg"), nil},
		}
		sortedValues := []keyPair{
			{[]byte("abcd"), []byte("val2")},
			{[]byte("bcd"), []byte("val3")},
			{[]byte("cursor"), []byte("val1")},
			{[]byte("defg"), nil},
		}

		// Store the values to be used in the cursor tests in unsorted
		// order and ensure they were actually stored.
		if !testPutValues(tc, bucket, unsortedValues) {
			return false
		}
		if !testGetValues(tc, bucket, toGetValues(unsortedValues)) {
			return false
		}

		// Ensure the cursor returns all items in byte-sorted order when
		// iterating forward.
		curIdx := 0
		for ok := cursor.First(); ok; ok = cursor.Next() {
			k, v := cursor.Key(), cursor.Value()
			if !testCursorKeyPair(tc, k, v, curIdx, sortedValues) {
				return false
			}
			curIdx++
		}
		if curIdx != len(unsortedValues) {
			tc.t.Errorf("Cursor: expected to iterate %d values, "+
				"but only iterated %d", len(unsortedValues),
				curIdx)
			return false
		}

		// Ensure the cursor returns all items in reverse byte-sorted
		// order when iterating in reverse.
		curIdx = len(sortedValues) - 1
		for ok := cursor.Last(); ok; ok = cursor.Prev() {
			k, v := cursor.Key(), cursor.Value()
			if !testCursorKeyPair(tc, k, v, curIdx, sortedValues) {
				return false
			}
			curIdx--
		}
		if curIdx > -1 {
			tc.t.Errorf("Reverse cursor: expected to iterate %d "+
				"values, but only iterated %d",
				len(sortedValues), len(sortedValues)-(curIdx+1))
			return false
		}

		// Ensure forward iteration works as expected after seeking.
		middleIdx := (len(sortedValues) - 1) / 2
		seekKey := sortedValues[middleIdx].key
		curIdx = middleIdx
		for ok := cursor.Seek(seekKey); ok; ok = cursor.Next() {
			k, v := cursor.Key(), cursor.Value()
			if !testCursorKeyPair(tc, k, v, curIdx, sortedValues) {
				return false
			}
			curIdx++
		}
		if curIdx != len(sortedValues) {
			tc.t.Errorf("Cursor after seek: expected to iterate "+
				"%d values, but only iterated %d",
				len(sortedValues)-middleIdx, curIdx-middleIdx)
			return false
		}

		// Ensure reverse iteration works as expected after seeking.
		curIdx = middleIdx
		for ok := cursor.Seek(seekKey); ok; ok = cursor.Prev() {
			k, v := cursor.Key(), cursor.Value()
			if !testCursorKeyPair(tc, k, v, curIdx, sortedValues) {
				return false
			}
			curIdx--
		}
		if curIdx > -1 {
			tc.t.Errorf("Reverse cursor after seek: expected to "+
				"iterate %d values, but only iterated %d",
				len(sortedValues)-middleIdx, middleIdx-curIdx)
			return false
		}

		// Ensure the cursor deletes items properly.
		if !cursor.First() {
			tc.t.Errorf("Cursor.First: no value")
			return false
		}
		k := cursor.Key()
		if err := cursor.Delete(); err != nil {
			tc.t.Errorf("Cursor.Delete: unexpected error: %v", err)
			return false
		}
		if val := bucket.Get(k); val != nil {
			tc.t.Errorf("Cursor.Delete: value for key %q was not "+
				"deleted", k)
			return false
		}
	}

	return true
}

// testNestedBucket reruns the testBucketInterface against a nested bucket along
// with a counter to only test a couple of level deep.
func testNestedBucket(tc *testContext, testBucket database.Bucket) bool {
	// Don't go more than 2 nested levels deep.
	if tc.bucketDepth > 1 {
		return true
	}

	tc.bucketDepth++
	defer func() {
		tc.bucketDepth--
	}()
	return testBucketInterface(tc, testBucket)
}

// testBucketInterface ensures the bucket interface is working properly by
// exercising all of its functions.  This includes the cursor interface for the
// cursor returned from the bucket.
func testBucketInterface(tc *testContext, bucket database.Bucket) bool {
	if bucket.Writable() != tc.isWritable {
		tc.t.Errorf("Bucket writable state does not match.")
		return false
	}

	if tc.isWritable {
		// keyValues holds the keys and values to use when putting
		// values into the bucket.
		keyValues := []keyPair{
			{[]byte("bucketkey1"), []byte("foo1")},
			{[]byte("bucketkey2"), []byte("foo2")},
			{[]byte("bucketkey3"), []byte("foo3")},
			{[]byte("bucketkey4"), nil},
		}
		expectedKeyValues := toGetValues(keyValues)
		if !testPutValues(tc, bucket, keyValues) {
			return false
		}

		if !testGetValues(tc, bucket, expectedKeyValues) {
			return false
		}

		// Ensure errors returned from the user-supplied ForEach
		// function are returned.
		forEachError := fmt.Errorf("example foreach error")
		err := bucket.ForEach(func(k, v []byte) error {
			return forEachError
		})
		if err != forEachError {
			tc.t.Errorf("ForEach: inner function error not "+
				"returned - got %v, want %v", err, forEachError)
			return false
		}

		// Iterate all of the keys using ForEach while making sure the
		// stored values are the expected values.
		keysFound := make(map[string]struct{}, len(keyValues))
		err = bucket.ForEach(func(k, v []byte) error {
			wantV, found := lookupKey(k, expectedKeyValues)
			if !found {
				return fmt.Errorf("ForEach: key '%s' should "+
					"exist", k)
			}

			if !reflect.DeepEqual(v, wantV) {
				return fmt.Errorf("ForEach: value for key '%s' "+
					"does not match - got %s, want %s", k,
					v, wantV)
			}

			keysFound[string(k)] = struct{}{}
			return nil
		})
		if err != nil {
			tc.t.Errorf("%v", err)
			return false
		}

		// Ensure all keys were iterated.
		for _, item := range keyValues {
			if _, ok := keysFound[string(item.key)]; !ok {
				tc.t.Errorf("ForEach: key '%s' was not iterated "+
					"when it should have been", item.key)
				return false
			}
		}

		// Delete the keys and ensure they were deleted.
		if !testDeleteValues(tc, bucket, keyValues) {
			return false
		}
		if !testGetValues(tc, bucket, rollbackValues(keyValues)) {
			return false
		}

		// Ensure creating a new bucket works as expected.
		testBucketName := []byte("testbucket")
		testBucket, err := bucket.CreateBucket(testBucketName)
		if err != nil {
			tc.t.Errorf("CreateBucket: unexpected error: %v", err)
			return false
		}
		if !testNestedBucket(tc, testBucket) {
			return false
		}

		// Ensure errors returned from the user-supplied ForEachBucket
		// function are returned.
		err = bucket.ForEachBucket(func(k []byte) error {
			return forEachError
		})
		if err != forEachError {
			tc.t.Errorf("ForEachBucket: inner function error not "+
				"returned - got %v, want %v", err, forEachError)
			return false
		}

		// Ensure creating a bucket that already exists fails with the
		// expected error.
		wantErrCode := database.ErrBucketExists
		_, err = bucket.CreateBucket(testBucketName)
		if !checkDbError(tc.t, "CreateBucket", err, wantErrCode) {
			return false
		}

		// Ensure CreateBucketIfNotExists returns an existing bucket.
		testBucket, err = bucket.CreateBucketIfNotExists(testBucketName)
		if err != nil {
			tc.t.Errorf("CreateBucketIfNotExists: unexpected "+
				"error: %v", err)
			return false
		}
		if !testNestedBucket(tc, testBucket) {
			return false
		}

		// Ensure retrieving an existing bucket works as expected.
		testBucket = bucket.Bucket(testBucketName)
		if !testNestedBucket(tc, testBucket) {
			return false
		}

		// Ensure deleting a bucket works as intended.
		if err := bucket.DeleteBucket(testBucketName); err != nil {
			tc.t.Errorf("DeleteBucket: unexpected error: %v", err)
			return false
		}
		if b := bucket.Bucket(testBucketName); b != nil {
			tc.t.Errorf("DeleteBucket: bucket '%s' still exists",
				testBucketName)
			return false
		}

		// Ensure deleting a bucket that doesn't exist returns the
		// expected error.
		wantErrCode = database.ErrBucketNotFound
		err = bucket.DeleteBucket(testBucketName)
		if !checkDbError(tc.t, "DeleteBucket", err, wantErrCode) {
			return false
		}

		// Ensure CreateBucketIfNotExists creates a new bucket when
		// it doesn't already exist.
		testBucket, err = bucket.CreateBucketIfNotExists(testBucketName)
		if err != nil {
			tc.t.Errorf("CreateBucketIfNotExists: unexpected "+
				"error: %v", err)
			return false
		}
		if !testNestedBucket(tc, testBucket) {
			return false
		}

		// Ensure the cursor interface works as expected.
		if !testCursorInterface(tc, testBucket) {
			return false
		}

		// Delete the test bucket to avoid leaving it around for future
		// calls.
		if err := bucket.DeleteBucket(testBucketName); err != nil {
			tc.t.Errorf("DeleteBucket: unexpected error: %v", err)
			return false
		}
		if b := bucket.Bucket(testBucketName); b != nil {
			tc.t.Errorf("DeleteBucket: bucket '%s' still exists",
				testBucketName)
			return false
		}
	} else {
		// Put should fail with bucket that is not writable.
		testName := "unwritable tx put"
		wantErrCode := database.ErrTxNotWritable
		failBytes := []byte("fail")
		err := bucket.Put(failBytes, failBytes)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// Delete should fail with bucket that is not writable.
		testName = "unwritable tx delete"
		err = bucket.Delete(failBytes)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// CreateBucket should fail with bucket that is not writable.
		testName = "unwritable tx create bucket"
		_, err = bucket.CreateBucket(failBytes)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// CreateBucketIfNotExists should fail with bucket that is not
		// writable.
		testName = "unwritable tx create bucket if not exists"
		_, err = bucket.CreateBucketIfNotExists(failBytes)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// DeleteBucket should fail with bucket that is not writable.
		testName = "unwritable tx delete bucket"
		err = bucket.DeleteBucket(failBytes)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// Ensure the cursor interface works as expected with read-only
		// buckets.
		if !testCursorInterface(tc, bucket) {
			return false
		}
	}

	return true
}

// rollbackOnPanic rolls the passed transaction back if the code in the calling
// function panics.  This is useful in case the tests unexpectedly panic which
// would leave any manually created transactions with the database mutex locked
// thereby leading to a deadlock and masking the real reason for the panic.  It
// also logs a test error and repanics so the original panic can be traced.
func rollbackOnPanic(t *testing.T, tx database.Tx) {
	if err := recover(); err != nil {
		t.Errorf("Unexpected panic: %v", err)
		_ = tx.Rollback()
		panic(err)
	}
}

// testMetadataManualTxInterface ensures that the manual transactions metadata
// interface works as expected.
func testMetadataManualTxInterface(tc *testContext) bool {
	// populateValues tests that populating values works as expected.
	//
	// When the writable flag is false, a read-only tranasction is created,
	// standard bucket tests for read-only transactions are performed, and
	// the Commit function is checked to ensure it fails as expected.
	//
	// Otherwise, a read-write transaction is created, the values are
	// written, standard bucket tests for read-write transactions are
	// performed, and then the transaction is either committed or rolled
	// back depending on the flag.
	bucket1Name := []byte("bucket1")
	populateValues := func(writable, rollback bool, putValues []keyPair) bool {
		tx, err := tc.db.Begin(writable)
		if err != nil {
			tc.t.Errorf("Begin: unexpected error %v", err)
			return false
		}
		defer rollbackOnPanic(tc.t, tx)

		metadataBucket := tx.Metadata()
		if metadataBucket == nil {
			tc.t.Errorf("Metadata: unexpected nil bucket")
			_ = tx.Rollback()
			return false
		}

		bucket1 := metadataBucket.Bucket(bucket1Name)
		if bucket1 == nil {
			tc.t.Errorf("Bucket1: unexpected nil bucket")
			return false
		}

		tc.isWritable = writable
		if !testBucketInterface(tc, bucket1) {
			_ = tx.Rollback()
			return false
		}

		if !writable {
			// The transaction is not writable, so it should fail
			// the commit.
			testName := "unwritable tx commit"
			wantErrCode := database.ErrTxNotWritable
			err := tx.Commit()
			if !checkDbError(tc.t, testName, err, wantErrCode) {
				_ = tx.Rollback()
				return false
			}
		} else {
			if !testPutValues(tc, bucket1, putValues) {
				return false
			}

			if rollback {
				// Rollback the transaction.
				if err := tx.Rollback(); err != nil {
					tc.t.Errorf("Rollback: unexpected "+
						"error %v", err)
					return false
				}
			} else {
				// The commit should succeed.
				if err := tx.Commit(); err != nil {
					tc.t.Errorf("Commit: unexpected error "+
						"%v", err)
					return false
				}
			}
		}

		return true
	}

	// checkValues starts a read-only transaction and checks that all of
	// the key/value pairs specified in the expectedValues parameter match
	// what's in the database.
	checkValues := func(expectedValues []keyPair) bool {
		tx, err := tc.db.Begin(false)
		if err != nil {
			tc.t.Errorf("Begin: unexpected error %v", err)
			return false
		}
		defer rollbackOnPanic(tc.t, tx)

		metadataBucket := tx.Metadata()
		if metadataBucket == nil {
			tc.t.Errorf("Metadata: unexpected nil bucket")
			_ = tx.Rollback()
			return false
		}

		bucket1 := metadataBucket.Bucket(bucket1Name)
		if bucket1 == nil {
			tc.t.Errorf("Bucket1: unexpected nil bucket")
			return false
		}

		if !testGetValues(tc, bucket1, expectedValues) {
			_ = tx.Rollback()
			return false
		}

		// Rollback the read-only transaction.
		if err := tx.Rollback(); err != nil {
			tc.t.Errorf("Commit: unexpected error %v", err)
			return false
		}

		return true
	}

	// deleteValues starts a read-write transaction and deletes the keys
	// in the passed key/value pairs.
	deleteValues := func(values []keyPair) bool {
		tx, err := tc.db.Begin(true)
		if err != nil {

		}
		defer rollbackOnPanic(tc.t, tx)

		metadataBucket := tx.Metadata()
		if metadataBucket == nil {
			tc.t.Errorf("Metadata: unexpected nil bucket")
			_ = tx.Rollback()
			return false
		}

		bucket1 := metadataBucket.Bucket(bucket1Name)
		if bucket1 == nil {
			tc.t.Errorf("Bucket1: unexpected nil bucket")
			return false
		}

		// Delete the keys and ensure they were deleted.
		if !testDeleteValues(tc, bucket1, values) {
			_ = tx.Rollback()
			return false
		}
		if !testGetValues(tc, bucket1, rollbackValues(values)) {
			_ = tx.Rollback()
			return false
		}

		// Commit the changes and ensure it was successful.
		if err := tx.Commit(); err != nil {
			tc.t.Errorf("Commit: unexpected error %v", err)
			return false
		}

		return true
	}

	// keyValues holds the keys and values to use when putting values into a
	// bucket.
	var keyValues = []keyPair{
		{[]byte("umtxkey1"), []byte("foo1")},
		{[]byte("umtxkey2"), []byte("foo2")},
		{[]byte("umtxkey3"), []byte("foo3")},
		{[]byte("umtxkey4"), nil},
	}

	// Ensure that attempting populating the values using a read-only
	// transaction fails as expected.
	if !populateValues(false, true, keyValues) {
		return false
	}
	if !checkValues(rollbackValues(keyValues)) {
		return false
	}

	// Ensure that attempting populating the values using a read-write
	// transaction and then rolling it back yields the expected values.
	if !populateValues(true, true, keyValues) {
		return false
	}
	if !checkValues(rollbackValues(keyValues)) {
		return false
	}

	// Ensure that attempting populating the values using a read-write
	// transaction and then committing it stores the expected values.
	if !populateValues(true, false, keyValues) {
		return false
	}
	if !checkValues(toGetValues(keyValues)) {
		return false
	}

	// Clean up the keys.
	if !deleteValues(keyValues) {
		return false
	}

	return true
}

// testManagedTxPanics ensures calling Rollback of Commit inside a managed
// transaction panics.
func testManagedTxPanics(tc *testContext) bool {
	testPanic := func(fn func()) (paniced bool) {
		// Setup a defer to catch the expected panic and update the
		// return variable.
		defer func() {
			if err := recover(); err != nil {
				paniced = true
			}
		}()

		fn()
		return false
	}

	// Ensure calling Commit on a managed read-only transaction panics.
	paniced := testPanic(func() {
		tc.db.View(func(tx database.Tx) error {
			tx.Commit()
			return nil
		})
	})
	if !paniced {
		tc.t.Error("Commit called inside View did not panic")
		return false
	}

	// Ensure calling Rollback on a managed read-only transaction panics.
	paniced = testPanic(func() {
		tc.db.View(func(tx database.Tx) error {
			tx.Rollback()
			return nil
		})
	})
	if !paniced {
		tc.t.Error("Rollback called inside View did not panic")
		return false
	}

	// Ensure calling Commit on a managed read-write transaction panics.
	paniced = testPanic(func() {
		tc.db.Update(func(tx database.Tx) error {
			tx.Commit()
			return nil
		})
	})
	if !paniced {
		tc.t.Error("Commit called inside Update did not panic")
		return false
	}

	// Ensure calling Rollback on a managed read-write transaction panics.
	paniced = testPanic(func() {
		tc.db.Update(func(tx database.Tx) error {
			tx.Rollback()
			return nil
		})
	})
	if !paniced {
		tc.t.Error("Rollback called inside Update did not panic")
		return false
	}

	return true
}

// testMetadataTxInterface tests all facets of the managed read/write and
// manual transaction metadata interfaces as well as the bucket interfaces under
// them.
func testMetadataTxInterface(tc *testContext) bool {
	if !testManagedTxPanics(tc) {
		return false
	}

	bucket1Name := []byte("bucket1")
	err := tc.db.Update(func(tx database.Tx) error {
		_, err := tx.Metadata().CreateBucket(bucket1Name)
		return err
	})
	if err != nil {
		tc.t.Errorf("Update: unexpected error creating bucket: %v", err)
		return false
	}

	if !testMetadataManualTxInterface(tc) {
		return false
	}

	// keyValues holds the keys and values to use when putting values
	// into a bucket.
	keyValues := []keyPair{
		{[]byte("mtxkey1"), []byte("foo1")},
		{[]byte("mtxkey2"), []byte("foo2")},
		{[]byte("mtxkey3"), []byte("foo3")},
		{[]byte("mtxkey4"), nil},
	}

	// Test the bucket interface via a managed read-only transaction.
	err = tc.db.View(func(tx database.Tx) error {
		metadataBucket := tx.Metadata()
		if metadataBucket == nil {
			return fmt.Errorf("Metadata: unexpected nil bucket")
		}

		bucket1 := metadataBucket.Bucket(bucket1Name)
		if bucket1 == nil {
			return fmt.Errorf("Bucket1: unexpected nil bucket")
		}

		tc.isWritable = false
		if !testBucketInterface(tc, bucket1) {
			return errSubTestFail
		}

		return nil
	})
	if err != nil {
		if err != errSubTestFail {
			tc.t.Errorf("%v", err)
		}
		return false
	}

	// Ensure errors returned from the user-supplied View function are
	// returned.
	viewError := fmt.Errorf("example view error")
	err = tc.db.View(func(tx database.Tx) error {
		return viewError
	})
	if err != viewError {
		tc.t.Errorf("View: inner function error not returned - got "+
			"%v, want %v", err, viewError)
		return false
	}

	// Test the bucket interface via a managed read-write transaction.
	// Also, put a series of values and force a rollback so the following
	// code can ensure the values were not stored.
	forceRollbackError := fmt.Errorf("force rollback")
	err = tc.db.Update(func(tx database.Tx) error {
		metadataBucket := tx.Metadata()
		if metadataBucket == nil {
			return fmt.Errorf("Metadata: unexpected nil bucket")
		}

		bucket1 := metadataBucket.Bucket(bucket1Name)
		if bucket1 == nil {
			return fmt.Errorf("Bucket1: unexpected nil bucket")
		}

		tc.isWritable = true
		if !testBucketInterface(tc, bucket1) {
			return errSubTestFail
		}

		if !testPutValues(tc, bucket1, keyValues) {
			return errSubTestFail
		}

		// Return an error to force a rollback.
		return forceRollbackError
	})
	if err != forceRollbackError {
		if err == errSubTestFail {
			return false
		}

		tc.t.Errorf("Update: inner function error not returned - got "+
			"%v, want %v", err, forceRollbackError)
		return false
	}

	// Ensure the values that should not have been stored due to the forced
	// rollback above were not actually stored.
	err = tc.db.View(func(tx database.Tx) error {
		metadataBucket := tx.Metadata()
		if metadataBucket == nil {
			return fmt.Errorf("Metadata: unexpected nil bucket")
		}

		if !testGetValues(tc, metadataBucket, rollbackValues(keyValues)) {
			return errSubTestFail
		}

		return nil
	})
	if err != nil {
		if err != errSubTestFail {
			tc.t.Errorf("%v", err)
		}
		return false
	}

	// Store a series of values via a managed read-write transaction.
	err = tc.db.Update(func(tx database.Tx) error {
		metadataBucket := tx.Metadata()
		if metadataBucket == nil {
			return fmt.Errorf("Metadata: unexpected nil bucket")
		}

		bucket1 := metadataBucket.Bucket(bucket1Name)
		if bucket1 == nil {
			return fmt.Errorf("Bucket1: unexpected nil bucket")
		}

		if !testPutValues(tc, bucket1, keyValues) {
			return errSubTestFail
		}

		return nil
	})
	if err != nil {
		if err != errSubTestFail {
			tc.t.Errorf("%v", err)
		}
		return false
	}

	// Ensure the values stored above were committed as expected.
	err = tc.db.View(func(tx database.Tx) error {
		metadataBucket := tx.Metadata()
		if metadataBucket == nil {
			return fmt.Errorf("Metadata: unexpected nil bucket")
		}

		bucket1 := metadataBucket.Bucket(bucket1Name)
		if bucket1 == nil {
			return fmt.Errorf("Bucket1: unexpected nil bucket")
		}

		if !testGetValues(tc, bucket1, toGetValues(keyValues)) {
			return errSubTestFail
		}

		return nil
	})
	if err != nil {
		if err != errSubTestFail {
			tc.t.Errorf("%v", err)
		}
		return false
	}

	// Clean up the values stored above in a managed read-write transaction.
	err = tc.db.Update(func(tx database.Tx) error {
		metadataBucket := tx.Metadata()
		if metadataBucket == nil {
			return fmt.Errorf("Metadata: unexpected nil bucket")
		}

		bucket1 := metadataBucket.Bucket(bucket1Name)
		if bucket1 == nil {
			return fmt.Errorf("Bucket1: unexpected nil bucket")
		}

		if !testDeleteValues(tc, bucket1, keyValues) {
			return errSubTestFail
		}

		return nil
	})
	if err != nil {
		if err != errSubTestFail {
			tc.t.Errorf("%v", err)
		}
		return false
	}

	return true
}

// testFetchBlockIOMissing ensures that all of the block retrieval API functions
// work as expected when requesting blocks that don't exist.
func testFetchBlockIOMissing(tc *testContext, tx database.Tx) bool {
	wantErrCode := database.ErrBlockNotFound

	// ---------------------
	// Non-bulk Block IO API
	// ---------------------

	// Test the individual block APIs one block at a time to ensure they
	// return the expected error.  Also, build the data needed to test the
	// bulk APIs below while looping.
	allBlockHashes := make([]chainhash.Hash, len(tc.blocks))
	allBlockRegions := make([]database.BlockRegion, len(tc.blocks))
	for i, block := range tc.blocks {
		blockHash := block.Hash()
		allBlockHashes[i] = *blockHash

		txLocs, err := block.TxLoc()
		if err != nil {
			tc.t.Errorf("block.TxLoc(%d): unexpected error: %v", i,
				err)
			return false
		}

		// Ensure FetchBlock returns expected error.
		testName := fmt.Sprintf("FetchBlock #%d on missing block", i)
		_, err = tx.FetchBlock(blockHash)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// Ensure FetchBlockHeader returns expected error.
		testName = fmt.Sprintf("FetchBlockHeader #%d on missing block",
			i)
		_, err = tx.FetchBlockHeader(blockHash)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// Ensure the first transaction fetched as a block region from
		// the database returns the expected error.
		region := database.BlockRegion{
			Hash:   blockHash,
			Offset: uint32(txLocs[0].TxStart),
			Len:    uint32(txLocs[0].TxLen),
		}
		allBlockRegions[i] = region
		_, err = tx.FetchBlockRegion(&region)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// Ensure HasBlock returns false.
		hasBlock, err := tx.HasBlock(blockHash)
		if err != nil {
			tc.t.Errorf("HasBlock #%d: unexpected err: %v", i, err)
			return false
		}
		if hasBlock {
			tc.t.Errorf("HasBlock #%d: should not have block", i)
			return false
		}
	}

	// -----------------
	// Bulk Block IO API
	// -----------------

	// Ensure FetchBlocks returns expected error.
	testName := "FetchBlocks on missing blocks"
	_, err := tx.FetchBlocks(allBlockHashes)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure FetchBlockHeaders returns expected error.
	testName = "FetchBlockHeaders on missing blocks"
	_, err = tx.FetchBlockHeaders(allBlockHashes)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure FetchBlockRegions returns expected error.
	testName = "FetchBlockRegions on missing blocks"
	_, err = tx.FetchBlockRegions(allBlockRegions)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure HasBlocks returns false for all blocks.
	hasBlocks, err := tx.HasBlocks(allBlockHashes)
	if err != nil {
		tc.t.Errorf("HasBlocks: unexpected err: %v", err)
	}
	for i, hasBlock := range hasBlocks {
		if hasBlock {
			tc.t.Errorf("HasBlocks #%d: should not have block", i)
			return false
		}
	}

	return true
}

// testFetchBlockIO ensures all of the block retrieval API functions work as
// expected for the provide set of blocks.  The blocks must already be stored in
// the database, or at least stored into the passed transaction.  It also
// tests several error conditions such as ensuring the expected errors are
// returned when fetching blocks, headers, and regions that don't exist.
func testFetchBlockIO(tc *testContext, tx database.Tx) bool {
	// ---------------------
	// Non-bulk Block IO API
	// ---------------------

	// Test the individual block APIs one block at a time.  Also, build the
	// data needed to test the bulk APIs below while looping.
	allBlockHashes := make([]chainhash.Hash, len(tc.blocks))
	allBlockBytes := make([][]byte, len(tc.blocks))
	allBlockTxLocs := make([][]wire.TxLoc, len(tc.blocks))
	allBlockRegions := make([]database.BlockRegion, len(tc.blocks))
	for i, block := range tc.blocks {
		blockHash := block.Hash()
		allBlockHashes[i] = *blockHash

		blockBytes, err := block.Bytes()
		if err != nil {
			tc.t.Errorf("block.Bytes(%d): unexpected error: %v", i,
				err)
			return false
		}
		allBlockBytes[i] = blockBytes

		txLocs, err := block.TxLoc()
		if err != nil {
			tc.t.Errorf("block.TxLoc(%d): unexpected error: %v", i,
				err)
			return false
		}
		allBlockTxLocs[i] = txLocs

		// Ensure the block data fetched from the database matches the
		// expected bytes.
		gotBlockBytes, err := tx.FetchBlock(blockHash)
		if err != nil {
			tc.t.Errorf("FetchBlock(%s): unexpected error: %v",
				blockHash, err)
			return false
		}
		if !bytes.Equal(gotBlockBytes, blockBytes) {
			tc.t.Errorf("FetchBlock(%s): bytes mismatch: got %x, "+
				"want %x", blockHash, gotBlockBytes, blockBytes)
			return false
		}

		// Ensure the block header fetched from the database matches the
		// expected bytes.
		wantHeaderBytes := blockBytes[0:wire.MaxBlockHeaderPayload]
		gotHeaderBytes, err := tx.FetchBlockHeader(blockHash)
		if err != nil {
			tc.t.Errorf("FetchBlockHeader(%s): unexpected error: %v",
				blockHash, err)
			return false
		}
		if !bytes.Equal(gotHeaderBytes, wantHeaderBytes) {
			tc.t.Errorf("FetchBlockHeader(%s): bytes mismatch: "+
				"got %x, want %x", blockHash, gotHeaderBytes,
				wantHeaderBytes)
			return false
		}

		// Ensure the first transaction fetched as a block region from
		// the database matches the expected bytes.
		region := database.BlockRegion{
			Hash:   blockHash,
			Offset: uint32(txLocs[0].TxStart),
			Len:    uint32(txLocs[0].TxLen),
		}
		allBlockRegions[i] = region
		endRegionOffset := region.Offset + region.Len
		wantRegionBytes := blockBytes[region.Offset:endRegionOffset]
		gotRegionBytes, err := tx.FetchBlockRegion(&region)
		if err != nil {
			tc.t.Errorf("FetchBlockRegion(%s): unexpected error: %v",
				blockHash, err)
			return false
		}
		if !bytes.Equal(gotRegionBytes, wantRegionBytes) {
			tc.t.Errorf("FetchBlockRegion(%s): bytes mismatch: "+
				"got %x, want %x", blockHash, gotRegionBytes,
				wantRegionBytes)
			return false
		}

		// Ensure block hash exists as expected.
		hasBlock, err := tx.HasBlock(blockHash)
		if err != nil {
			tc.t.Errorf("HasBlock(%s): unexpected error: %v",
				blockHash, err)
			return false
		}
		if !hasBlock {
			tc.t.Errorf("HasBlock(%s): database claims it doesn't "+
				"have the block when it should", blockHash)
			return false
		}

		// -----------------------
		// Invalid blocks/regions.
		// -----------------------

		// Ensure fetching a block that doesn't exist returns the
		// expected error.
		badBlockHash := &chainhash.Hash{}
		testName := fmt.Sprintf("FetchBlock(%s) invalid block",
			badBlockHash)
		wantErrCode := database.ErrBlockNotFound
		_, err = tx.FetchBlock(badBlockHash)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// Ensure fetching a block header that doesn't exist returns
		// the expected error.
		testName = fmt.Sprintf("FetchBlockHeader(%s) invalid block",
			badBlockHash)
		_, err = tx.FetchBlockHeader(badBlockHash)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// Ensure fetching a block region in a block that doesn't exist
		// return the expected error.
		testName = fmt.Sprintf("FetchBlockRegion(%s) invalid hash",
			badBlockHash)
		wantErrCode = database.ErrBlockNotFound
		region.Hash = badBlockHash
		region.Offset = ^uint32(0)
		_, err = tx.FetchBlockRegion(&region)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// Ensure fetching a block region that is out of bounds returns
		// the expected error.
		testName = fmt.Sprintf("FetchBlockRegion(%s) invalid region",
			blockHash)
		wantErrCode = database.ErrBlockRegionInvalid
		region.Hash = blockHash
		region.Offset = ^uint32(0)
		_, err = tx.FetchBlockRegion(&region)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}
	}

	// -----------------
	// Bulk Block IO API
	// -----------------

	// Ensure the bulk block data fetched from the database matches the
	// expected bytes.
	blockData, err := tx.FetchBlocks(allBlockHashes)
	if err != nil {
		tc.t.Errorf("FetchBlocks: unexpected error: %v", err)
		return false
	}
	if len(blockData) != len(allBlockBytes) {
		tc.t.Errorf("FetchBlocks: unexpected number of results - got "+
			"%d, want %d", len(blockData), len(allBlockBytes))
		return false
	}
	for i := 0; i < len(blockData); i++ {
		blockHash := allBlockHashes[i]
		wantBlockBytes := allBlockBytes[i]
		gotBlockBytes := blockData[i]
		if !bytes.Equal(gotBlockBytes, wantBlockBytes) {
			tc.t.Errorf("FetchBlocks(%s): bytes mismatch: got %x, "+
				"want %x", blockHash, gotBlockBytes,
				wantBlockBytes)
			return false
		}
	}

	// Ensure the bulk block headers fetched from the database match the
	// expected bytes.
	blockHeaderData, err := tx.FetchBlockHeaders(allBlockHashes)
	if err != nil {
		tc.t.Errorf("FetchBlockHeaders: unexpected error: %v", err)
		return false
	}
	if len(blockHeaderData) != len(allBlockBytes) {
		tc.t.Errorf("FetchBlockHeaders: unexpected number of results "+
			"- got %d, want %d", len(blockHeaderData),
			len(allBlockBytes))
		return false
	}
	for i := 0; i < len(blockHeaderData); i++ {
		blockHash := allBlockHashes[i]
		wantHeaderBytes := allBlockBytes[i][0:wire.MaxBlockHeaderPayload]
		gotHeaderBytes := blockHeaderData[i]
		if !bytes.Equal(gotHeaderBytes, wantHeaderBytes) {
			tc.t.Errorf("FetchBlockHeaders(%s): bytes mismatch: "+
				"got %x, want %x", blockHash, gotHeaderBytes,
				wantHeaderBytes)
			return false
		}
	}

	// Ensure the first transaction of every block fetched in bulk block
	// regions from the database matches the expected bytes.
	allRegionBytes, err := tx.FetchBlockRegions(allBlockRegions)
	if err != nil {
		tc.t.Errorf("FetchBlockRegions: unexpected error: %v", err)
		return false

	}
	if len(allRegionBytes) != len(allBlockRegions) {
		tc.t.Errorf("FetchBlockRegions: unexpected number of results "+
			"- got %d, want %d", len(allRegionBytes),
			len(allBlockRegions))
		return false
	}
	for i, gotRegionBytes := range allRegionBytes {
		region := &allBlockRegions[i]
		endRegionOffset := region.Offset + region.Len
		wantRegionBytes := blockData[i][region.Offset:endRegionOffset]
		if !bytes.Equal(gotRegionBytes, wantRegionBytes) {
			tc.t.Errorf("FetchBlockRegions(%d): bytes mismatch: "+
				"got %x, want %x", i, gotRegionBytes,
				wantRegionBytes)
			return false
		}
	}

	// Ensure the bulk determination of whether a set of block hashes are in
	// the database returns true for all loaded blocks.
	hasBlocks, err := tx.HasBlocks(allBlockHashes)
	if err != nil {
		tc.t.Errorf("HasBlocks: unexpected error: %v", err)
		return false
	}
	for i, hasBlock := range hasBlocks {
		if !hasBlock {
			tc.t.Errorf("HasBlocks(%d): should have block", i)
			return false
		}
	}

	// -----------------------
	// Invalid blocks/regions.
	// -----------------------

	// Ensure fetching blocks for which one doesn't exist returns the
	// expected error.
	testName := "FetchBlocks invalid hash"
	badBlockHashes := make([]chainhash.Hash, len(allBlockHashes)+1)
	copy(badBlockHashes, allBlockHashes)
	badBlockHashes[len(badBlockHashes)-1] = chainhash.Hash{}
	wantErrCode := database.ErrBlockNotFound
	_, err = tx.FetchBlocks(badBlockHashes)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure fetching block headers for which one doesn't exist returns the
	// expected error.
	testName = "FetchBlockHeaders invalid hash"
	_, err = tx.FetchBlockHeaders(badBlockHashes)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure fetching block regions for which one of blocks doesn't exist
	// returns expected error.
	testName = "FetchBlockRegions invalid hash"
	badBlockRegions := make([]database.BlockRegion, len(allBlockRegions)+1)
	copy(badBlockRegions, allBlockRegions)
	badBlockRegions[len(badBlockRegions)-1].Hash = &chainhash.Hash{}
	wantErrCode = database.ErrBlockNotFound
	_, err = tx.FetchBlockRegions(badBlockRegions)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure fetching block regions that are out of bounds returns the
	// expected error.
	testName = "FetchBlockRegions invalid regions"
	badBlockRegions = badBlockRegions[:len(badBlockRegions)-1]
	for i := range badBlockRegions {
		badBlockRegions[i].Offset = ^uint32(0)
	}
	wantErrCode = database.ErrBlockRegionInvalid
	_, err = tx.FetchBlockRegions(badBlockRegions)
	return checkDbError(tc.t, testName, err, wantErrCode)
}

// testBlockIOTxInterface ensures that the block IO interface works as expected
// for both managed read/write and manual transactions.  This function leaves
// all of the stored blocks in the database.
func testBlockIOTxInterface(tc *testContext) bool {
	// Ensure attempting to store a block with a read-only transaction fails
	// with the expected error.
	err := tc.db.View(func(tx database.Tx) error {
		wantErrCode := database.ErrTxNotWritable
		for i, block := range tc.blocks {
			testName := fmt.Sprintf("StoreBlock(%d) on ro tx", i)
			err := tx.StoreBlock(block)
			if !checkDbError(tc.t, testName, err, wantErrCode) {
				return errSubTestFail
			}
		}

		return nil
	})
	if err != nil {
		if err != errSubTestFail {
			tc.t.Errorf("%v", err)
		}
		return false
	}

	// Populate the database with loaded blocks and ensure all of the data
	// fetching APIs work properly on them within the transaction before a
	// commit or rollback.  Then, force a rollback so the code below can
	// ensure none of the data actually gets stored.
	forceRollbackError := fmt.Errorf("force rollback")
	err = tc.db.Update(func(tx database.Tx) error {
		// Store all blocks in the same transaction.
		for i, block := range tc.blocks {
			err := tx.StoreBlock(block)
			if err != nil {
				tc.t.Errorf("StoreBlock #%d: unexpected error: "+
					"%v", i, err)
				return errSubTestFail
			}
		}

		// Ensure attempting to store the same block again, before the
		// transaction has been committed, returns the expected error.
		wantErrCode := database.ErrBlockExists
		for i, block := range tc.blocks {
			testName := fmt.Sprintf("duplicate block entry #%d "+
				"(before commit)", i)
			err := tx.StoreBlock(block)
			if !checkDbError(tc.t, testName, err, wantErrCode) {
				return errSubTestFail
			}
		}

		// Ensure that all data fetches from the stored blocks before
		// the transaction has been committed work as expected.
		if !testFetchBlockIO(tc, tx) {
			return errSubTestFail
		}

		return forceRollbackError
	})
	if err != forceRollbackError {
		if err == errSubTestFail {
			return false
		}

		tc.t.Errorf("Update: inner function error not returned - got "+
			"%v, want %v", err, forceRollbackError)
		return false
	}

	// Ensure rollback was successful
	err = tc.db.View(func(tx database.Tx) error {
		if !testFetchBlockIOMissing(tc, tx) {
			return errSubTestFail
		}
		return nil
	})
	if err != nil {
		if err != errSubTestFail {
			tc.t.Errorf("%v", err)
		}
		return false
	}

	// Populate the database with loaded blocks and ensure all of the data
	// fetching APIs work properly.
	err = tc.db.Update(func(tx database.Tx) error {
		// Store a bunch of blocks in the same transaction.
		for i, block := range tc.blocks {
			err := tx.StoreBlock(block)
			if err != nil {
				tc.t.Errorf("StoreBlock #%d: unexpected error: "+
					"%v", i, err)
				return errSubTestFail
			}
		}

		// Ensure attempting to store the same block again while in the
		// same transaction, but before it has been committed, returns
		// the expected error.
		for i, block := range tc.blocks {
			testName := fmt.Sprintf("duplicate block entry #%d "+
				"(before commit)", i)
			wantErrCode := database.ErrBlockExists
			err := tx.StoreBlock(block)
			if !checkDbError(tc.t, testName, err, wantErrCode) {
				return errSubTestFail
			}
		}

		// Ensure that all data fetches from the stored blocks before
		// the transaction has been committed work as expected.
		if !testFetchBlockIO(tc, tx) {
			return errSubTestFail
		}

		return nil
	})
	if err != nil {
		if err != errSubTestFail {
			tc.t.Errorf("%v", err)
		}
		return false
	}

	// Ensure all data fetch tests work as expected using a managed
	// read-only transaction after the data was successfully committed
	// above.
	err = tc.db.View(func(tx database.Tx) error {
		if !testFetchBlockIO(tc, tx) {
			return errSubTestFail
		}

		return nil
	})
	if err != nil {
		if err != errSubTestFail {
			tc.t.Errorf("%v", err)
		}
		return false
	}

	// Ensure all data fetch tests work as expected using a managed
	// read-write transaction after the data was successfully committed
	// above.
	err = tc.db.Update(func(tx database.Tx) error {
		if !testFetchBlockIO(tc, tx) {
			return errSubTestFail
		}

		// Ensure attempting to store existing blocks again returns the
		// expected error.  Note that this is different from the
		// previous version since this is a new transaction after the
		// blocks have been committed.
		wantErrCode := database.ErrBlockExists
		for i, block := range tc.blocks {
			testName := fmt.Sprintf("duplicate block entry #%d "+
				"(before commit)", i)
			err := tx.StoreBlock(block)
			if !checkDbError(tc.t, testName, err, wantErrCode) {
				return errSubTestFail
			}
		}

		return nil
	})
	if err != nil {
		if err != errSubTestFail {
			tc.t.Errorf("%v", err)
		}
		return false
	}

	return true
}

// testClosedTxInterface ensures that both the metadata and block IO API
// functions behave as expected when attempted against a closed transaction.
func testClosedTxInterface(tc *testContext, tx database.Tx) bool {
	wantErrCode := database.ErrTxClosed
	bucket := tx.Metadata()
	cursor := tx.Metadata().Cursor()
	bucketName := []byte("closedtxbucket")
	keyName := []byte("closedtxkey")

	// ------------
	// Metadata API
	// ------------

	// Ensure that attempting to get an existing bucket returns nil when the
	// transaction is closed.
	if b := bucket.Bucket(bucketName); b != nil {
		tc.t.Errorf("Bucket: did not return nil on closed tx")
		return false
	}

	// Ensure CreateBucket returns expected error.
	testName := "CreateBucket on closed tx"
	_, err := bucket.CreateBucket(bucketName)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure CreateBucketIfNotExists returns expected error.
	testName = "CreateBucketIfNotExists on closed tx"
	_, err = bucket.CreateBucketIfNotExists(bucketName)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure Delete returns expected error.
	testName = "Delete on closed tx"
	err = bucket.Delete(keyName)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure DeleteBucket returns expected error.
	testName = "DeleteBucket on closed tx"
	err = bucket.DeleteBucket(bucketName)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure ForEach returns expected error.
	testName = "ForEach on closed tx"
	err = bucket.ForEach(nil)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure ForEachBucket returns expected error.
	testName = "ForEachBucket on closed tx"
	err = bucket.ForEachBucket(nil)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure Get returns expected error.
	testName = "Get on closed tx"
	if k := bucket.Get(keyName); k != nil {
		tc.t.Errorf("Get: did not return nil on closed tx")
		return false
	}

	// Ensure Put returns expected error.
	testName = "Put on closed tx"
	err = bucket.Put(keyName, []byte("test"))
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// -------------------
	// Metadata Cursor API
	// -------------------

	// Ensure attempting to get a bucket from a cursor on a closed tx gives
	// back nil.
	if b := cursor.Bucket(); b != nil {
		tc.t.Error("Cursor.Bucket: returned non-nil on closed tx")
		return false
	}

	// Ensure Cursor.Delete returns expected error.
	testName = "Cursor.Delete on closed tx"
	err = cursor.Delete()
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure Cursor.First on a closed tx returns false and nil key/value.
	if cursor.First() {
		tc.t.Error("Cursor.First: claims ok on closed tx")
		return false
	}
	if cursor.Key() != nil || cursor.Value() != nil {
		tc.t.Error("Cursor.First: key and/or value are not nil on " +
			"closed tx")
		return false
	}

	// Ensure Cursor.Last on a closed tx returns false and nil key/value.
	if cursor.Last() {
		tc.t.Error("Cursor.Last: claims ok on closed tx")
		return false
	}
	if cursor.Key() != nil || cursor.Value() != nil {
		tc.t.Error("Cursor.Last: key and/or value are not nil on " +
			"closed tx")
		return false
	}

	// Ensure Cursor.Next on a closed tx returns false and nil key/value.
	if cursor.Next() {
		tc.t.Error("Cursor.Next: claims ok on closed tx")
		return false
	}
	if cursor.Key() != nil || cursor.Value() != nil {
		tc.t.Error("Cursor.Next: key and/or value are not nil on " +
			"closed tx")
		return false
	}

	// Ensure Cursor.Prev on a closed tx returns false and nil key/value.
	if cursor.Prev() {
		tc.t.Error("Cursor.Prev: claims ok on closed tx")
		return false
	}
	if cursor.Key() != nil || cursor.Value() != nil {
		tc.t.Error("Cursor.Prev: key and/or value are not nil on " +
			"closed tx")
		return false
	}

	// Ensure Cursor.Seek on a closed tx returns false and nil key/value.
	if cursor.Seek([]byte{}) {
		tc.t.Error("Cursor.Seek: claims ok on closed tx")
		return false
	}
	if cursor.Key() != nil || cursor.Value() != nil {
		tc.t.Error("Cursor.Seek: key and/or value are not nil on " +
			"closed tx")
		return false
	}

	// ---------------------
	// Non-bulk Block IO API
	// ---------------------

	// Test the individual block APIs one block at a time to ensure they
	// return the expected error.  Also, build the data needed to test the
	// bulk APIs below while looping.
	allBlockHashes := make([]chainhash.Hash, len(tc.blocks))
	allBlockRegions := make([]database.BlockRegion, len(tc.blocks))
	for i, block := range tc.blocks {
		blockHash := block.Hash()
		allBlockHashes[i] = *blockHash

		txLocs, err := block.TxLoc()
		if err != nil {
			tc.t.Errorf("block.TxLoc(%d): unexpected error: %v", i,
				err)
			return false
		}

		// Ensure StoreBlock returns expected error.
		testName = "StoreBlock on closed tx"
		err = tx.StoreBlock(block)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// Ensure FetchBlock returns expected error.
		testName = fmt.Sprintf("FetchBlock #%d on closed tx", i)
		_, err = tx.FetchBlock(blockHash)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// Ensure FetchBlockHeader returns expected error.
		testName = fmt.Sprintf("FetchBlockHeader #%d on closed tx", i)
		_, err = tx.FetchBlockHeader(blockHash)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// Ensure the first transaction fetched as a block region from
		// the database returns the expected error.
		region := database.BlockRegion{
			Hash:   blockHash,
			Offset: uint32(txLocs[0].TxStart),
			Len:    uint32(txLocs[0].TxLen),
		}
		allBlockRegions[i] = region
		_, err = tx.FetchBlockRegion(&region)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}

		// Ensure HasBlock returns expected error.
		testName = fmt.Sprintf("HasBlock #%d on closed tx", i)
		_, err = tx.HasBlock(blockHash)
		if !checkDbError(tc.t, testName, err, wantErrCode) {
			return false
		}
	}

	// -----------------
	// Bulk Block IO API
	// -----------------

	// Ensure FetchBlocks returns expected error.
	testName = "FetchBlocks on closed tx"
	_, err = tx.FetchBlocks(allBlockHashes)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure FetchBlockHeaders returns expected error.
	testName = "FetchBlockHeaders on closed tx"
	_, err = tx.FetchBlockHeaders(allBlockHashes)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure FetchBlockRegions returns expected error.
	testName = "FetchBlockRegions on closed tx"
	_, err = tx.FetchBlockRegions(allBlockRegions)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// Ensure HasBlocks returns expected error.
	testName = "HasBlocks on closed tx"
	_, err = tx.HasBlocks(allBlockHashes)
	if !checkDbError(tc.t, testName, err, wantErrCode) {
		return false
	}

	// ---------------
	// Commit/Rollback
	// ---------------

	// Ensure that attempting to rollback or commit a transaction that is
	// already closed returns the expected error.
	err = tx.Rollback()
	if !checkDbError(tc.t, "closed tx rollback", err, wantErrCode) {
		return false
	}
	err = tx.Commit()
	return checkDbError(tc.t, "closed tx commit", err, wantErrCode)
}

// testTxClosed ensures that both the metadata and block IO API functions behave
// as expected when attempted against both read-only and read-write
// transactions.
func testTxClosed(tc *testContext) bool {
	bucketName := []byte("closedtxbucket")
	keyName := []byte("closedtxkey")

	// Start a transaction, create a bucket and key used for testing, and
	// immediately perform a commit on it so it is closed.
	tx, err := tc.db.Begin(true)
	if err != nil {
		tc.t.Errorf("Begin(true): unexpected error: %v", err)
		return false
	}
	defer rollbackOnPanic(tc.t, tx)
	if _, err := tx.Metadata().CreateBucket(bucketName); err != nil {
		tc.t.Errorf("CreateBucket: unexpected error: %v", err)
		return false
	}
	if err := tx.Metadata().Put(keyName, []byte("test")); err != nil {
		tc.t.Errorf("Put: unexpected error: %v", err)
		return false
	}
	if err := tx.Commit(); err != nil {
		tc.t.Errorf("Commit: unexpected error: %v", err)
		return false
	}

	// Ensure invoking all of the functions on the closed read-write
	// transaction behave as expected.
	if !testClosedTxInterface(tc, tx) {
		return false
	}

	// Repeat the tests with a rolled-back read-only transaction.
	tx, err = tc.db.Begin(false)
	if err != nil {
		tc.t.Errorf("Begin(false): unexpected error: %v", err)
		return false
	}
	defer rollbackOnPanic(tc.t, tx)
	if err := tx.Rollback(); err != nil {
		tc.t.Errorf("Rollback: unexpected error: %v", err)
		return false
	}

	// Ensure invoking all of the functions on the closed read-only
	// transaction behave as expected.
	return testClosedTxInterface(tc, tx)
}

// testConcurrecy ensure the database properly supports concurrent readers and
// only a single writer.  It also ensures views act as snapshots at the time
// they are acquired.
func testConcurrecy(tc *testContext) bool {
	// sleepTime is how long each of the concurrent readers should sleep to
	// aid in detection of whether or not the data is actually being read
	// concurrently.  It starts with a sane lower bound.
	var sleepTime = time.Millisecond * 250

	// Determine about how long it takes for a single block read.  When it's
	// longer than the default minimum sleep time, adjust the sleep time to
	// help prevent durations that are too short which would cause erroneous
	// test failures on slower systems.
	startTime := time.Now()
	err := tc.db.View(func(tx database.Tx) error {
		_, err := tx.FetchBlock(tc.blocks[0].Hash())
		return err
	})
	if err != nil {
		tc.t.Errorf("Unexpected error in view: %v", err)
		return false
	}
	elapsed := time.Since(startTime)
	if sleepTime < elapsed {
		sleepTime = elapsed
	}
	tc.t.Logf("Time to load block 0: %v, using sleep time: %v", elapsed,
		sleepTime)

	// reader takes a block number to load and channel to return the result
	// of the operation on.  It is used below to launch multiple concurrent
	// readers.
	numReaders := len(tc.blocks)
	resultChan := make(chan bool, numReaders)
	reader := func(blockNum int) {
		err := tc.db.View(func(tx database.Tx) error {
			time.Sleep(sleepTime)
			_, err := tx.FetchBlock(tc.blocks[blockNum].Hash())
			return err
		})
		if err != nil {
			tc.t.Errorf("Unexpected error in concurrent view: %v",
				err)
			resultChan <- false
		}
		resultChan <- true
	}

	// Start up several concurrent readers for the same block and wait for
	// the results.
	startTime = time.Now()
	for i := 0; i < numReaders; i++ {
		go reader(0)
	}
	for i := 0; i < numReaders; i++ {
		if result := <-resultChan; !result {
			return false
		}
	}
	elapsed = time.Since(startTime)
	tc.t.Logf("%d concurrent reads of same block elapsed: %v", numReaders,
		elapsed)

	// Consider it a failure if it took longer than half the time it would
	// take with no concurrency.
	if elapsed > sleepTime*time.Duration(numReaders/2) {
		tc.t.Errorf("Concurrent views for same block did not appear to "+
			"run simultaneously: elapsed %v", elapsed)
		return false
	}

	// Start up several concurrent readers for different blocks and wait for
	// the results.
	startTime = time.Now()
	for i := 0; i < numReaders; i++ {
		go reader(i)
	}
	for i := 0; i < numReaders; i++ {
		if result := <-resultChan; !result {
			return false
		}
	}
	elapsed = time.Since(startTime)
	tc.t.Logf("%d concurrent reads of different blocks elapsed: %v",
		numReaders, elapsed)

	// Consider it a failure if it took longer than half the time it would
	// take with no concurrency.
	if elapsed > sleepTime*time.Duration(numReaders/2) {
		tc.t.Errorf("Concurrent views for different blocks did not "+
			"appear to run simultaneously: elapsed %v", elapsed)
		return false
	}

	// Start up a few readers and wait for them to acquire views.  Each
	// reader waits for a signal from the writer to be finished to ensure
	// that the data written by the writer is not seen by the view since it
	// was started before the data was set.
	concurrentKey := []byte("notthere")
	concurrentVal := []byte("someval")
	started := make(chan struct{})
	writeComplete := make(chan struct{})
	reader = func(blockNum int) {
		err := tc.db.View(func(tx database.Tx) error {
			started <- struct{}{}

			// Wait for the writer to complete.
			<-writeComplete

			// Since this reader was created before the write took
			// place, the data it added should not be visible.
			val := tx.Metadata().Get(concurrentKey)
			if val != nil {
				return fmt.Errorf("%s should not be visible",
					concurrentKey)
			}
			return nil
		})
		if err != nil {
			tc.t.Errorf("Unexpected error in concurrent view: %v",
				err)
			resultChan <- false
		}
		resultChan <- true
	}
	for i := 0; i < numReaders; i++ {
		go reader(0)
	}
	for i := 0; i < numReaders; i++ {
		<-started
	}

	// All readers are started and waiting for completion of the writer.
	// Set some data the readers are expecting to not find and signal the
	// readers the write is done by closing the writeComplete channel.
	err = tc.db.Update(func(tx database.Tx) error {
		return tx.Metadata().Put(concurrentKey, concurrentVal)
	})
	if err != nil {
		tc.t.Errorf("Unexpected error in update: %v", err)
		return false
	}
	close(writeComplete)

	// Wait for reader results.
	for i := 0; i < numReaders; i++ {
		if result := <-resultChan; !result {
			return false
		}
	}

	// Start a few writers and ensure the total time is at least the
	// writeSleepTime * numWriters.  This ensures only one write transaction
	// can be active at a time.
	writeSleepTime := time.Millisecond * 250
	writer := func() {
		err := tc.db.Update(func(tx database.Tx) error {
			time.Sleep(writeSleepTime)
			return nil
		})
		if err != nil {
			tc.t.Errorf("Unexpected error in concurrent view: %v",
				err)
			resultChan <- false
		}
		resultChan <- true
	}
	numWriters := 3
	startTime = time.Now()
	for i := 0; i < numWriters; i++ {
		go writer()
	}
	for i := 0; i < numWriters; i++ {
		if result := <-resultChan; !result {
			return false
		}
	}
	elapsed = time.Since(startTime)
	tc.t.Logf("%d concurrent writers elapsed using sleep time %v: %v",
		numWriters, writeSleepTime, elapsed)

	// The total time must have been at least the sum of all sleeps if the
	// writes blocked properly.
	if elapsed < writeSleepTime*time.Duration(numWriters) {
		tc.t.Errorf("Concurrent writes appeared to run simultaneously: "+
			"elapsed %v", elapsed)
		return false
	}

	return true
}

// testConcurrentClose ensures that closing the database with open transactions
// blocks until the transactions are finished.
//
// The database will be closed upon returning from this function.
func testConcurrentClose(tc *testContext) bool {
	// Start up a few readers and wait for them to acquire views.  Each
	// reader waits for a signal to complete to ensure the transactions stay
	// open until they are explicitly signalled to be closed.
	var activeReaders int32
	numReaders := 3
	started := make(chan struct{})
	finishReaders := make(chan struct{})
	resultChan := make(chan bool, numReaders+1)
	reader := func() {
		err := tc.db.View(func(tx database.Tx) error {
			atomic.AddInt32(&activeReaders, 1)
			started <- struct{}{}
			<-finishReaders
			atomic.AddInt32(&activeReaders, -1)
			return nil
		})
		if err != nil {
			tc.t.Errorf("Unexpected error in concurrent view: %v",
				err)
			resultChan <- false
		}
		resultChan <- true
	}
	for i := 0; i < numReaders; i++ {
		go reader()
	}
	for i := 0; i < numReaders; i++ {
		<-started
	}

	// Close the database in a separate goroutine.  This should block until
	// the transactions are finished.  Once the close has taken place, the
	// dbClosed channel is closed to signal the main goroutine below.
	dbClosed := make(chan struct{})
	go func() {
		started <- struct{}{}
		err := tc.db.Close()
		if err != nil {
			tc.t.Errorf("Unexpected error in concurrent view: %v",
				err)
			resultChan <- false
		}
		close(dbClosed)
		resultChan <- true
	}()
	<-started

	// Wait a short period and then signal the reader transactions to
	// finish.  When the db closed channel is received, ensure there are no
	// active readers open.
	time.AfterFunc(time.Millisecond*250, func() { close(finishReaders) })
	<-dbClosed
	if nr := atomic.LoadInt32(&activeReaders); nr != 0 {
		tc.t.Errorf("Close did not appear to block with active "+
			"readers: %d active", nr)
		return false
	}

	// Wait for all results.
	for i := 0; i < numReaders+1; i++ {
		if result := <-resultChan; !result {
			return false
		}
	}

	return true
}

// testInterface tests performs tests for the various interfaces of the database
// package which require state in the database for the given database type.
func testInterface(t *testing.T, db database.DB) {
	// Create a test context to pass around.
	context := testContext{t: t, db: db}

	// Load the test blocks and store in the test context for use throughout
	// the tests.
	blocks, err := loadBlocks(t, blockDataFile, blockDataNet)
	if err != nil {
		t.Errorf("loadBlocks: Unexpected error: %v", err)
		return
	}
	context.blocks = blocks

	// Test the transaction metadata interface including managed and manual
	// transactions as well as buckets.
	if !testMetadataTxInterface(&context) {
		return
	}

	// Test the transaction block IO interface using managed and manual
	// transactions.  This function leaves all of the stored blocks in the
	// database since they're used later.
	if !testBlockIOTxInterface(&context) {
		return
	}

	// Test all of the transaction interface functions against a closed
	// transaction work as expected.
	if !testTxClosed(&context) {
		return
	}

	// Test the database properly supports concurrency.
	if !testConcurrecy(&context) {
		return
	}

	// Test that closing the database with open transactions blocks until
	// the transactions are finished.
	//
	// The database will be closed upon returning from this function, so it
	// must be the last thing called.
	testConcurrentClose(&context)
}
