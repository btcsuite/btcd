// Copyright (c) 2013-2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb

import (
	"bytes"

	"testing"

	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/golangcrypto/ripemd160"
)

func TestAddrIndexKeySerialization(t *testing.T) {
	var hash160Bytes [ripemd160.Size]byte
	var packedIndex [12]byte

	fakeHash160 := btcutil.Hash160([]byte("testing"))
	copy(fakeHash160, hash160Bytes[:])

	fakeIndex := txAddrIndex{
		hash160:   hash160Bytes,
		blkHeight: 1,
		txoffset:  5,
		txlen:     360,
	}

	serializedKey := addrIndexToKey(&fakeIndex)
	copy(packedIndex[:], serializedKey[23:35])
	unpackedIndex := unpackTxIndex(packedIndex)

	if unpackedIndex.blkHeight != fakeIndex.blkHeight {
		t.Errorf("Incorrect block height. Unpack addr index key"+
			"serialization failed. Expected %d, received %d",
			1, unpackedIndex.blkHeight)
	}

	if unpackedIndex.txoffset != fakeIndex.txoffset {
		t.Errorf("Incorrect tx offset. Unpack addr index key"+
			"serialization failed. Expected %d, received %d",
			5, unpackedIndex.txoffset)
	}

	if unpackedIndex.txlen != fakeIndex.txlen {
		t.Errorf("Incorrect tx len. Unpack addr index key"+
			"serialization failed. Expected %d, received %d",
			360, unpackedIndex.txlen)
	}
}

func TestBytesPrefix(t *testing.T) {
	testKey := []byte("a")

	prefixRange := bytesPrefix(testKey)
	if !bytes.Equal(prefixRange.Start, []byte("a")) {
		t.Errorf("Wrong prefix start, got %d, expected %d", prefixRange.Start,
			[]byte("a"))
	}

	if !bytes.Equal(prefixRange.Limit, []byte("b")) {
		t.Errorf("Wrong prefix end, got %d, expected %d", prefixRange.Limit,
			[]byte("b"))
	}
}
