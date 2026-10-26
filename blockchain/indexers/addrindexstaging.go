// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"bytes"

	"github.com/btcsuite/btcd/wire/v2"
)

const (
	// addrRecordSize is an address key followed by one serialized address
	// index entry.
	addrRecordSize = addrKeySize + txEntrySize
)

// addrRecord is the fixed size staging representation of an address index
// entry. Its trailing bytes use the address index value encoding. On disk, the
// block ID field holds the block height, which the write pass replaces with the
// block ID before it writes the records to the database.
type addrRecord [addrRecordSize]byte

// makeAddrRecord returns the staging record for the provided address key,
// block ID and transaction location.  The bytes after the address key are the
// entry serializeAddrIndexEntry returns for the same block ID and transaction
// location.
func makeAddrRecord(addrKey *[addrKeySize]byte, blockID uint32,
	txLoc wire.TxLoc) addrRecord {

	var record addrRecord
	copy(record[:addrKeySize], addrKey[:])
	byteOrder.PutUint32(record[addrKeySize:], blockID)
	byteOrder.PutUint32(record[addrKeySize+4:], uint32(txLoc.TxStart))
	byteOrder.PutUint32(record[addrKeySize+8:], uint32(txLoc.TxLen))
	return record
}

// key returns the address key of the record.
func (r *addrRecord) key() [addrKeySize]byte {
	var key [addrKeySize]byte
	copy(key[:], r[:addrKeySize])
	return key
}

// blockID returns the block ID field of the record, which holds the block
// height until the write pass replaces it with the block ID.
func (r *addrRecord) blockID() uint32 {
	return byteOrder.Uint32(r[addrKeySize:])
}

// less orders records by address key, block height and position in the block.
// This is the order ConnectBlock adds the entries of an address in, since
// indexBlock visits the transactions of a block in order.  Records that compare
// equal are identical, so the unstable sort.Sort gives one result.
func (r *addrRecord) less(other *addrRecord) bool {
	cmp := bytes.Compare(r[:addrKeySize], other[:addrKeySize])
	if cmp != 0 {
		return cmp < 0
	}
	if r.blockID() != other.blockID() {
		return r.blockID() < other.blockID()
	}
	return byteOrder.Uint32(r[addrKeySize+4:]) <
		byteOrder.Uint32(other[addrKeySize+4:])
}

// addrRecords implements sort.Interface to allow a slice of staging records to
// be sorted in the order defined by less.
type addrRecords []addrRecord

// Len returns the number of records in the slice.  It is part of the
// sort.Interface implementation.
func (r addrRecords) Len() int {
	return len(r)
}

// Less returns whether the record with index i should sort before the record
// with index j.  It is part of the sort.Interface implementation.
func (r addrRecords) Less(i, j int) bool {
	return r[i].less(&r[j])
}

// Swap swaps the records at the passed indices.  It is part of the
// sort.Interface implementation.
func (r addrRecords) Swap(i, j int) {
	r[i], r[j] = r[j], r[i]
}
