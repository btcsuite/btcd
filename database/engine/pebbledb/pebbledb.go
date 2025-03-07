package pebbledb

import (
	"errors"
	"runtime"
	"sync/atomic"

	"github.com/btcsuite/btcd/database/engine"
	"github.com/cockroachdb/pebble"
	"github.com/cockroachdb/pebble/bloom"
)

var (
	ErrDbClosed         = errors.New("pebbledb: closed")
	ErrTxClosed         = errors.New("pebbledb: transaction already closed")
	ErrSnapshotReleased = errors.New("pebbledb: snapshot released")
	ErrIteratorReleased = errors.New("pebbledb: iterator released")
)

const (
	DefaultCache   = 64
	DefaultHandles = 16
)

func NewDB(dbPath string, create bool, cache, handles int) (engine.Engine, error) {
	if cache <= 0 {
		cache = DefaultCache
	}
	if handles <= 0 {
		handles = DefaultHandles
	}

	opts := &pebble.Options{
		Cache:                    pebble.NewCache(int64(cache * 1024 * 1024)), // cache MB
		ErrorIfExists:            create,                                      // Fail if the database exists and create is true
		MaxOpenFiles:             handles,
		MaxConcurrentCompactions: runtime.NumCPU,
		Levels: []pebble.LevelOptions{
			{TargetFileSize: 2 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 4 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 8 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 16 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 32 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 64 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
			{TargetFileSize: 128 * 1024 * 1024, FilterPolicy: bloom.FilterPolicy(10)},
		},
	}
	opts.Experimental.ReadSamplingMultiplier = -1
	dbEngine, err := pebble.Open(dbPath, opts)
	if err != nil {
		return nil, err
	}

	return &DB{DB: dbEngine}, nil
}

type DB struct {
	*pebble.DB

	closed atomic.Bool
}

// Set closed flag; return true if not already closed.
func (db *DB) setClosed() bool {
	return !db.closed.Swap(true)
}

// Check whether DB was closed.
func (db *DB) isClosed() bool {
	return db.closed.Load()
}

func (d *DB) Transaction() (engine.Transaction, error) {
	if d.isClosed() {
		return nil, ErrDbClosed
	}
	return NewTransaction(d.DB.NewBatch()), nil
}

func (d *DB) Snapshot() (engine.Snapshot, error) {
	if d.isClosed() {
		return nil, ErrDbClosed
	}
	return NewSnapshot(d.DB.NewSnapshot()), nil
}

func (d *DB) Close() error {
	if !d.setClosed() {
		return ErrDbClosed
	}
	return d.DB.Close()
}
