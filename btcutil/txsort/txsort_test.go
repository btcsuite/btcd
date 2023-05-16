// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txsort_test

import (
	"bytes"
	"encoding/hex"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/btcsuite/btcd/btcutil/txsort"
	"github.com/btcsuite/btcd/wire"
)

// TestSort ensures the transaction sorting works according to the BIP.
func TestSort(t *testing.T) {
	tests := []struct {
		name         string
		hexFile      string
		isSorted     bool
		unsortedHash string
		sortedHash   string
	}{
		{
			name:         "first test case from BIP 69 - sorts inputs only, based on hash",
			hexFile:      "bip69-1.hex",
			isSorted:     false,
			unsortedHash: "0a6a357e2f7796444e02638749d9611c008b253fb55f5dc88b739b230ed0c4c3",
			sortedHash:   "839503cb611a3e3734bd521c608f881be2293ff77b7384057ab994c794fce623",
		},
		{
			name:         "second test case from BIP 69 - already sorted",
			hexFile:      "bip69-2.hex",
			isSorted:     true,
			unsortedHash: "28204cad1d7fc1d199e8ef4fa22f182de6258a3eaafe1bbe56ebdcacd3069a5f",
			sortedHash:   "28204cad1d7fc1d199e8ef4fa22f182de6258a3eaafe1bbe56ebdcacd3069a5f",
		},
		{
			name:         "block 100001 tx[1] - sorts outputs only, based on amount",
			hexFile:      "bip69-3.hex",
			isSorted:     false,
			unsortedHash: "fbde5d03b027d2b9ba4cf5d4fecab9a99864df2637b25ea4cbcb1796ff6550ca",
			sortedHash:   "0a8c246c55f6b82f094d211f4f57167bf2ea4898741d218b09bdb2536fd8d13f",
		},
		{
			name:         "block 100001 tx[2] - sorts both inputs and outputs",
			hexFile:      "bip69-4.hex",
			isSorted:     false,
			unsortedHash: "8131ffb0a2c945ecaf9b9063e59558784f9c3a74741ce6ae2a18d0571dac15bb",
			sortedHash:   "a3196553b928b0b6154b002fa9a1ce875adabc486fedaaaf4c17430fd4486329",
		},
		{
			name:         "block 100998 tx[6] - sorts outputs only, based on output script",
			hexFile:      "bip69-5.hex",
			isSorted:     false,
			unsortedHash: "ff85e8fc92e71bbc217e3ea9a3bacb86b435e52b6df0b089d67302c293a2b81d",
			sortedHash:   "9a6c24746de024f77cac9b2138694f11101d1c66289261224ca52a25155a7c94",
		},
	}

	for _, test := range tests {
		// Load and deserialize the test transaction.
		filePath := filepath.Join("testdata", test.hexFile)
		txHexBytes, err := ioutil.ReadFile(filePath)
		if err != nil {
			t.Errorf("ReadFile (%s): failed to read test file: %v",
				test.name, err)
			continue
		}
		txBytes, err := hex.DecodeString(string(txHexBytes))
		if err != nil {
			t.Errorf("DecodeString (%s): failed to decode tx: %v",
				test.name, err)
			continue
		}
		var tx wire.MsgTx
		err = tx.Deserialize(bytes.NewReader(txBytes))
		if err != nil {
			t.Errorf("Deserialize (%s): unexpected error %v",
				test.name, err)
			continue
		}

		// Ensure the sort order of the original transaction matches the
		// expected value.
		if got := txsort.IsSorted(&tx); got != test.isSorted {
			t.Errorf("IsSorted (%s): sort does not match "+
				"expected - got %v, want %v", test.name, got,
				test.isSorted)
			continue
		}

		// Sort the transaction and ensure the resulting hash is the
		// expected value.
		sortedTx := txsort.Sort(&tx)
		if got := sortedTx.TxHash().String(); got != test.sortedHash {
			t.Errorf("Sort (%s): sorted hash does not match "+
				"expected - got %v, want %v", test.name, got,
				test.sortedHash)
			continue
		}

		// Ensure the original transaction is not modified.
		if got := tx.TxHash().String(); got != test.unsortedHash {
			t.Errorf("Sort (%s): unsorted hash does not match "+
				"expected - got %v, want %v", test.name, got,
				test.unsortedHash)
			continue
		}

		// Now sort the transaction using the mutable version and ensure
		// the resulting hash is the expected value.
		txsort.InPlaceSort(&tx)
		if got := tx.TxHash().String(); got != test.sortedHash {
			t.Errorf("SortMutate (%s): sorted hash does not match "+
				"expected - got %v, want %v", test.name, got,
				test.sortedHash)
			continue
		}
	}
}
