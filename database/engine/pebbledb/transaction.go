package pebbledb

import (
	"github.com/btcsuite/btcd/database/engine"
	"github.com/cockroachdb/pebble"
)

func NewTransaction(batch *pebble.Batch) engine.Transaction {
	return &Transaction{Batch: batch}
}

type Transaction struct {
	*pebble.Batch
	released bool
}

func (t *Transaction) Put(key, value []byte) error {
	if t.released {
		return ErrTxClosed
	}
	return t.Batch.Set(key, value, pebble.NoSync)
}

func (t *Transaction) Delete(key []byte) error {
	if t.released {
		return ErrTxClosed
	}

	return t.Batch.Delete(key, pebble.NoSync)
}

func (t *Transaction) Discard() {
	if !t.released {
		t.released = true
		t.Batch.Close()
	}
}

func (t *Transaction) Commit() error {
	if t.released {
		return ErrTxClosed
	}
	return t.Batch.Commit(pebble.Sync)
}
