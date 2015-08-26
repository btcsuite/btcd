// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ffldb

import (
	"github.com/btcsuite/goleveldb/leveldb/iterator"
	"github.com/btcsuite/goleveldb/leveldb/util"
	"github.com/decred/dcrd/database/internal/treap"
)

// ldbTreapIter wraps a treap iterator to provide the additional functionality
// needed to satisfy the leveldb iterator.Iterator interface.
type ldbTreapIter struct {
	*treap.Iterator
	tx       *transaction
	released bool
}

// Enforce ldbTreapIter implements the leveldb iterator.Iterator interface.
var _ iterator.Iterator = (*ldbTreapIter)(nil)

// Error is only provided to satisfy the iterator interface as there are no
// errors for this memory-only structure.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *ldbTreapIter) Error() error {
	return nil
}

// SetReleaser is only provided to satisfy the iterator interface as there is no
// need to override it.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *ldbTreapIter) SetReleaser(releaser util.Releaser) {
}

// Release releases the iterator by removing the underlying treap iterator from
// the list of active iterators against the pending keys treap.
//
// This is part of the leveldb iterator.Iterator interface implementation.
func (iter *ldbTreapIter) Release() {
	if !iter.released {
		iter.tx.removeActiveIter(iter.Iterator)
		iter.released = true
	}
}

// newLdbTreapIter creates a new treap iterator for the given slice against the
// pending keys for the passed transaction and returns it wrapped in an
// ldbTreapIter so it can be used as a leveldb iterator.  It also adds the new
// iterator to the list of active iterators for the transaction.
func newLdbTreapIter(tx *transaction, slice *util.Range) *ldbTreapIter {
	iter := tx.pendingKeys.Iterator(slice.Start, slice.Limit)
	tx.addActiveIter(iter)
	return &ldbTreapIter{Iterator: iter, tx: tx}
}
