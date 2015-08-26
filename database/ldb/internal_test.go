// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package ldb

import (
	"bytes"

	"testing"

	"github.com/decred/dcrd/database"
	"github.com/decred/dcrutil"

	"github.com/btcsuite/golangcrypto/ripemd160"
)

func TestAddrIndexKeySerialization(t *testing.T) {
	var hash160Bytes [ripemd160.Size]byte
	var packedIndex [35]byte

	fakeHash160 := dcrutil.Hash160([]byte("testing"))
	copy(fakeHash160, hash160Bytes[:])

	fakeIndex := database.TxAddrIndex{
		Hash160:  hash160Bytes,
		Height:   1,
		TxOffset: 5,
		TxLen:    360,
	}

	serializedKey := addrIndexToKey(&fakeIndex)
	copy(packedIndex[:], serializedKey[0:35])
	unpackedIndex := unpackTxIndex(packedIndex)

	if unpackedIndex.Height != fakeIndex.Height {
		t.Errorf("Incorrect block height. Unpack addr index key"+
			"serialization failed. Expected %d, received %d",
			1, unpackedIndex.Height)
	}

	if unpackedIndex.TxOffset != fakeIndex.TxOffset {
		t.Errorf("Incorrect tx offset. Unpack addr index key"+
			"serialization failed. Expected %d, received %d",
			5, unpackedIndex.TxOffset)
	}

	if unpackedIndex.TxLen != fakeIndex.TxLen {
		t.Errorf("Incorrect tx len. Unpack addr index key"+
			"serialization failed. Expected %d, received %d",
			360, unpackedIndex.TxLen)
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
