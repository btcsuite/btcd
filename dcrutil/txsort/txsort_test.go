// Copyright (c) 2015-2016 The btcsuite developers
// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txsort

import (
	"bytes"
	"encoding/hex"
	"io/ioutil"
	"path/filepath"
	"testing"

	"github.com/decred/dcrd/wire"
)

// TestSort ensures the transaction sorting works as expected.
func TestSort(t *testing.T) {
	tests := []struct {
		name         string
		hexFile      string
		isSorted     bool
		unsortedHash string
		sortedHash   string
	}{
		{
			name:         "block 100004 tx[4] - already sorted",
			hexFile:      "tx100004-4.hex",
			isSorted:     true,
			unsortedHash: "b0f540d02eb5cf030e14c08b3b0092648a87f4c6b141dcf34723815a1d01a975",
			sortedHash:   "b0f540d02eb5cf030e14c08b3b0092648a87f4c6b141dcf34723815a1d01a975",
		},
		{
			name:         "block 101790 tx[3] - sorts inputs only, based on tree",
			hexFile:      "tx101790-3.hex",
			isSorted:     false,
			unsortedHash: "832f1df32817e2f1a1c655c658c2c93926a79b4f3ecc0e8a1f9e4447d990c69d",
			sortedHash:   "93d21689d1c25935afb2e3d0c9b0ee16ee4eebde98133248e0a916594e7244a5",
		},
		{
			name:         "block 150007 tx[23] - sorts inputs only, based on hash",
			hexFile:      "tx150007-23.hex",
			isSorted:     false,
			unsortedHash: "06fce11b0670038e1e7ec5fd5b84ff1e4fc07bc373f48060f21b9ff79e851a26",
			sortedHash:   "48c8586124eb874ce5f9043acc3d5861f214e4bcc2949590cf8f8bd84743dee9",
		},
		{
			name:         "block 108930 tx[1] - sorts inputs only, based on index",
			hexFile:      "tx108930-1.hex",
			isSorted:     false,
			unsortedHash: "08f620fbd2d7b5eb3d6c2940a859ce12968547f661be0ce24c4f116dce1f1f44",
			sortedHash:   "1071ce9732b58355616f815d08ff573a607f451bc1b722ab0ec5a094e376ea0a",
		},
		{
			name:         "block 100082 tx[5] - sorts outputs only, based on amount",
			hexFile:      "tx100082-5.hex",
			isSorted:     false,
			unsortedHash: "765bb2f1fa79222ebc381b6e32a2996e73d5b0d01b443d871f04060def245dcd",
			sortedHash:   "dddbea422361700c93899ae99ca8c3a5860efe1861df439de2f820787d8a1622",
		},
		{
			// Tx manually modified to make the first output (output 0)
			// have script version 1.
			name:         "modified block 150043 tx[14] - sorts outputs only, based on script version",
			hexFile:      "tx150043-14m.hex",
			isSorted:     false,
			unsortedHash: "b416cbe932c3419465de359b1fc1e07833fd2c49ddee36b61257d10046c98df4",
			sortedHash:   "ff198ee44011742f6c37580430bc13c05b2e91ea356ac36edfaaa037e245638d",
		},
		{
			name:         "block 150043 tx[14] - sorts outputs only, based on output script",
			hexFile:      "tx150043-14.hex",
			isSorted:     false,
			unsortedHash: "aa4c208c85a9576954d11dd566009cdad98b1c65dcd63b4f6ca11c3e8dff6684",
			sortedHash:   "bb5f9b338c0244e51182b10f36b5ca4c6eeaa2eae98a7bfc106d1f108f154525",
		},
		{
			name:         "block 150626 tx[24] - sorts outputs only, based on amount and output script ",
			hexFile:      "tx150626-24.hex",
			isSorted:     false,
			unsortedHash: "60542bf8ff4acd9ddb28fd9a2df33d23dd2b70a9aaca1f3dd5fe737879205e56",
			sortedHash:   "63709777a56ce88b146d90e8341501ac3cf8d7a2e16c56c74cacd2ec3f961f97",
		},
		{
			name:         "block 150002 tx[7] - sorts both inputs and outputs",
			hexFile:      "tx150002-7.hex",
			isSorted:     false,
			unsortedHash: "267d9700a090710127efa60c5523f56a0f268e6483737800d193109f5d76276b",
			sortedHash:   "0203d3e9c27ee42d670217685fa9ddb53990c4d0381a06de21ca37ec44087234",
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
		if got := IsSorted(&tx); got != test.isSorted {
			t.Errorf("IsSorted (%s): sort does not match "+
				"expected - got %v, want %v", test.name, got,
				test.isSorted)
			continue
		}

		// Sort the transaction and ensure the resulting hash is the
		// expected value.
		sortedTx := Sort(&tx)
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
		InPlaceSort(&tx)
		if got := tx.TxHash().String(); got != test.sortedHash {
			t.Errorf("SortMutate (%s): sorted hash does not match "+
				"expected - got %v, want %v", test.name, got,
				test.sortedHash)
			continue
		}
	}
}
