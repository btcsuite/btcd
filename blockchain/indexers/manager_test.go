// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/wire/v2"
)

type bucketBase struct {
	database.Bucket
}

// errorBucket wraps a database bucket and injects errors into operations used
// while cataloging and deleting index buckets.
type errorBucket struct {
	bucketBase
	forEachBucketErr error
	deleteBucketErr  error
}

// Bucket returns a wrapped nested bucket that preserves error injection.
func (b *errorBucket) Bucket(key []byte) database.Bucket {
	bucket := b.bucketBase.Bucket.Bucket(key)
	if bucket == nil {
		return nil
	}

	return &errorBucket{
		bucketBase:       bucketBase{Bucket: bucket},
		forEachBucketErr: b.forEachBucketErr,
		deleteBucketErr:  b.deleteBucketErr,
	}
}

// ForEachBucket returns the configured cataloging error when present.
func (b *errorBucket) ForEachBucket(fn func(k []byte) error) error {
	if b.forEachBucketErr != nil {
		return b.forEachBucketErr
	}

	return b.bucketBase.Bucket.ForEachBucket(fn)
}

// DeleteBucket returns the configured bucket deletion error when present.
func (b *errorBucket) DeleteBucket(key []byte) error {
	if b.deleteBucketErr != nil {
		return b.deleteBucketErr
	}

	return b.bucketBase.Bucket.DeleteBucket(key)
}

// errorTx wraps the metadata bucket of a database transaction so errors can
// be injected into bucket operations.
type errorTx struct {
	database.Tx
	forEachBucketErr error
	deleteBucketErr  error
}

// Metadata returns the wrapped top-level metadata bucket.
func (tx *errorTx) Metadata() database.Bucket {
	return &errorBucket{
		bucketBase:       bucketBase{Bucket: tx.Tx.Metadata()},
		forEachBucketErr: tx.forEachBucketErr,
		deleteBucketErr:  tx.deleteBucketErr,
	}
}

// errorDB wraps all managed transactions with the configured error injector.
type errorDB struct {
	database.DB
	forEachBucketErr error
	deleteBucketErr  error
}

// View invokes the view function with an error-injecting transaction.
func (db *errorDB) View(fn func(database.Tx) error) error {
	return db.DB.View(func(tx database.Tx) error {
		return fn(&errorTx{
			Tx:               tx,
			forEachBucketErr: db.forEachBucketErr,
			deleteBucketErr:  db.deleteBucketErr,
		})
	})
}

// Update invokes the update function with an error-injecting transaction.
func (db *errorDB) Update(fn func(database.Tx) error) error {
	return db.DB.Update(func(tx database.Tx) error {
		return fn(&errorTx{
			Tx:               tx,
			forEachBucketErr: db.forEachBucketErr,
			deleteBucketErr:  db.deleteBucketErr,
		})
	})
}

// createDropIndexTestDB creates a database with the metadata required to drop
// a test index.
func createDropIndexTestDB(t *testing.T, idxKey []byte) database.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "blocks_ffldb")
	db, err := database.Create("ffldb", dbPath, wire.MainNet)
	if err != nil {
		t.Fatalf("unable to create test database: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("unable to close test database: %v", err)
		}
	})

	err = db.Update(func(tx database.Tx) error {
		meta := tx.Metadata()
		indexesBucket, err := meta.CreateBucket(indexTipsBucketName)
		if err != nil {
			return err
		}
		if err := indexesBucket.Put(idxKey, []byte{0x01}); err != nil {
			return err
		}

		_, err = meta.CreateBucket(idxKey)
		return err
	})
	if err != nil {
		t.Fatalf("unable to initialize test index: %v", err)
	}

	return db
}

// TestDropIndexCatalogError ensures errors encountered while cataloging index
// buckets are returned to the caller.
func TestDropIndexCatalogError(t *testing.T) {
	idxKey := []byte("testidx")
	testErr := errors.New("test catalog error")
	db := &errorDB{
		DB:               createDropIndexTestDB(t, idxKey),
		forEachBucketErr: testErr,
	}

	err := dropIndex(db, idxKey, "test index", nil)
	if !errors.Is(err, testErr) {
		t.Fatalf("unexpected error: got %v, want %v", err, testErr)
	}
}

// TestDropIndexDeleteBucketError ensures errors encountered while deleting an
// index bucket are returned to the caller.
func TestDropIndexDeleteBucketError(t *testing.T) {
	idxKey := []byte("testidx")
	testErr := errors.New("test bucket deletion error")
	db := &errorDB{
		DB:              createDropIndexTestDB(t, idxKey),
		deleteBucketErr: testErr,
	}

	err := dropIndex(db, idxKey, "test index", nil)
	if !errors.Is(err, testErr) {
		t.Fatalf("unexpected error: got %v, want %v", err, testErr)
	}
}
