package leveldb

import (
	"github.com/btcsuite/btcd/database/engine"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func NewDB(dbPath string, create bool) (engine.Engine, error) {
	opts := opt.Options{
		ErrorIfExist: create,
		Strict:       opt.DefaultStrict,
		Compression:  opt.NoCompression,
		Filter:       filter.NewBloomFilter(10),
	}
	ldb, err := leveldb.OpenFile(dbPath, &opts)
	if err != nil {
		return nil, err
	}
	return &DB{DB: ldb}, nil
}

type DB struct {
	*leveldb.DB
}

func (d *DB) Transaction() (engine.Transaction, error) {
	tx, err := d.DB.OpenTransaction()
	if err != nil {
		return nil, err
	}
	return NewTransaction(tx), nil
}

func (d *DB) Snapshot() (engine.Snapshot, error) {
	snapshot, err := d.DB.GetSnapshot()
	if err != nil {
		return nil, err
	}
	return NewSnapshot(snapshot), nil
}

func (d *DB) Close() error {
	return d.DB.Close()
}
