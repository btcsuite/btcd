package pebbledb

import (
	"github.com/btcsuite/btcd/database/engine"
	"github.com/cockroachdb/pebble"
)

func NewIterator(iter *pebble.Iterator) engine.Iterator {
	return &Iterator{Iterator: iter}
}

type Iterator struct {
	*pebble.Iterator
	released bool
}

func (i *Iterator) Seek(key []byte) bool {
	return i.Iterator.SeekGE(key)
}

func (i *Iterator) Key() []byte {
	if !i.Iterator.Valid() { // return nil if the iterator is exhausted
		return nil
	}
	return i.Iterator.Key()
}

func (i *Iterator) Value() []byte {
	if !i.Iterator.Valid() { // return nil if the iterator is exhausted
		return nil
	}
	return i.Iterator.Value()
}

func (i *Iterator) Release() {
	if !i.released {
		i.released = true
		i.Iterator.Close()
	}
}

func (i *Iterator) Error() error {
	if i.released {
		return engine.ErrIterReleased
	}
	return i.Iterator.Error()
}
