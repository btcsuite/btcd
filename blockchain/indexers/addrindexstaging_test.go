// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package indexers

import (
	"sort"
	"testing"

	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestAddrStagingRecord ensures a staging record holds the address key, block
// ID and serialized entry it was made from, and that sorting orders records by
// address key.
func TestAddrStagingRecord(t *testing.T) {
	t.Parallel()

	// The keys differ in the first byte of the address hash, and first has
	// the larger key, so it sorts after second.
	var firstKey, secondKey [addrKeySize]byte
	firstKey[1] = 2
	secondKey[1] = 1
	first := makeAddrRecord(
		&firstKey, 10, wire.TxLoc{TxStart: 20, TxLen: 30},
	)
	second := makeAddrRecord(
		&secondKey, 11, wire.TxLoc{TxStart: 21, TxLen: 31},
	)

	require.Equal(t, firstKey, first.key())
	require.Equal(t, uint32(10), first.blockID())
	require.Equal(t, serializeAddrIndexEntry(
		10, wire.TxLoc{TxStart: 20, TxLen: 30},
	), first[addrKeySize:])

	records := addrRecords{first, second}
	sort.Sort(records)
	require.Equal(t, secondKey, records[0].key())
}
