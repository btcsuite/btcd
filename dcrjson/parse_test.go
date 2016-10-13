// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson_test

import (
	"encoding/hex"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrjson"
)

func decodeHash(reversedHash string) chainhash.Hash {
	h, err := chainhash.NewHashFromStr(reversedHash)
	if err != nil {
		panic(err)
	}
	return *h
}

func TestDecodeConcatenatedHashes(t *testing.T) {
	// Test data taken from Decred's first three mainnet blocks
	testHashes := []chainhash.Hash{
		decodeHash("298e5cc3d985bfe7f81dc135f360abe089edd4396b86d2de66b0cef42b21d980"),
		decodeHash("000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9"),
		decodeHash("000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba"),
	}
	var concatenatedHashBytes []byte
	for _, h := range testHashes {
		concatenatedHashBytes = append(concatenatedHashBytes, h[:]...)
	}
	concatenatedHashes := hex.EncodeToString(concatenatedHashBytes)
	decodedHashes, err := dcrjson.DecodeConcatenatedHashes(concatenatedHashes)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}
	if len(testHashes) != len(decodedHashes) {
		t.Fatalf("Got wrong number of decoded hashes (%v)", len(decodedHashes))
	}
	for i, expected := range testHashes {
		if expected != decodedHashes[i] {
			t.Fatalf("Decoded hash %d `%v` does not match expected `%v`",
				i, decodedHashes[i], expected)
		}
	}
}
