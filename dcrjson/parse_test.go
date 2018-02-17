// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrjson_test

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"strings"
	"testing"

	"github.com/decred/dcrd/blockchain/stake"
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

func TestEncodeConcatenatedHashes(t *testing.T) {
	// Input Hash slice. Data taken from Decred's first three mainnet blocks.
	hashSlice := []chainhash.Hash{
		decodeHash("298e5cc3d985bfe7f81dc135f360abe089edd4396b86d2de66b0cef42b21d980"),
		decodeHash("000000000000437482b6d47f82f374cde539440ddb108b0a76886f0d87d126b9"),
		decodeHash("000000000000c41019872ff7db8fd2e9bfa05f42d3f8fee8e895e8c1e5b8dcba"),
	}
	hashLen := hex.EncodedLen(len(hashSlice[0]))

	// Expected output. The string representations of the underlying byte arrays
	// in the input []chainhash.Hash
	blockHashes := []string{
		"80d9212bf4ceb066ded2866b39d4ed89e0ab60f335c11df8e7bf85d9c35c8e29",
		"b926d1870d6f88760a8b10db0d4439e5cd74f3827fd4b6827443000000000000",
		"badcb8e5c1e895e8e8fef8d3425fa0bfe9d28fdbf72f871910c4000000000000",
	}
	concatenatedHashes := strings.Join(blockHashes, "")

	// Test from 0 to N of the hashes
	for j := 0; j < len(hashSlice)+1; j++ {
		// Expected output string
		concatRef := concatenatedHashes[:j*hashLen]

		// Encode to string
		concatenated := dcrjson.EncodeConcatenatedHashes(hashSlice[:j])
		// Verify output
		if concatenated != concatRef {
			t.Fatalf("EncodeConcatenatedHashes failed (%v!=%v)",
				concatenated, concatRef)
		}
	}
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

func TestEncodeConcatenatedVoteBits(t *testing.T) {
	testVbs := []stake.VoteBits{
		{Bits: 0, ExtendedBits: []byte{}},
		{Bits: 0, ExtendedBits: []byte{0x00}},
		{Bits: 0x1223, ExtendedBits: []byte{0x01, 0x02, 0x03, 0x04}},
		{Bits: 0xaaaa, ExtendedBits: []byte{0x01, 0x02, 0x03, 0x04, 0x05}},
	}
	encodedResults, err := dcrjson.EncodeConcatenatedVoteBits(testVbs)
	if err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	expectedEncoded := []byte{
		0x02, 0x00, 0x00, 0x03,
		0x00, 0x00, 0x00, 0x06,
		0x23, 0x12, 0x01, 0x02,
		0x03, 0x04, 0x07, 0xaa,
		0xaa, 0x01, 0x02, 0x03,
		0x04, 0x05,
	}

	encodedResultsStr, _ := hex.DecodeString(encodedResults)
	if !bytes.Equal(expectedEncoded, encodedResultsStr) {
		t.Fatalf("Encoded votebits `%x` does not match expected `%x`",
			encodedResults, expectedEncoded)
	}

	// Test too long voteBits extended.
	testVbs = []stake.VoteBits{
		{Bits: 0, ExtendedBits: bytes.Repeat([]byte{0x00}, 74)},
	}
	_, err = dcrjson.EncodeConcatenatedVoteBits(testVbs)
	if err == nil {
		t.Fatalf("expected too long error")
	}
}

func TestDecodeConcatenatedVoteBits(t *testing.T) {
	encodedBytes := []byte{
		0x03, 0x00, 0x00, 0x00,
		0x06, 0x23, 0x12, 0x01,
		0x02, 0x03, 0x04, 0x07,
		0xaa, 0xaa, 0x01, 0x02,
		0x03, 0x04, 0x05,
	}
	encodedBytesStr := hex.EncodeToString(encodedBytes)

	expectedVbs := []stake.VoteBits{
		{Bits: 0, ExtendedBits: []byte{0x00}},
		{Bits: 0x1223, ExtendedBits: []byte{0x01, 0x02, 0x03, 0x04}},
		{Bits: 0xaaaa, ExtendedBits: []byte{0x01, 0x02, 0x03, 0x04, 0x05}},
	}

	decodedSlice, err :=
		dcrjson.DecodeConcatenatedVoteBits(encodedBytesStr)
	if err != nil {
		t.Fatalf("unexpected error decoding votebits: %v", err.Error())
	}

	if !reflect.DeepEqual(expectedVbs, decodedSlice) {
		t.Fatalf("Decoded votebits `%v` does not match expected `%v`",
			decodedSlice, expectedVbs)
	}

	// Test short read.
	encodedBytes = []byte{
		0x03, 0x00, 0x00, 0x00,
		0x06, 0x23, 0x12, 0x01,
		0x02, 0x03, 0x04, 0x07,
		0xaa, 0xaa, 0x01, 0x02,
		0x03, 0x04,
	}
	encodedBytesStr = hex.EncodeToString(encodedBytes)

	_, err = dcrjson.DecodeConcatenatedVoteBits(encodedBytesStr)
	if err == nil {
		t.Fatalf("expected short read error")
	}

	// Test too long read.
	encodedBytes = []byte{
		0x03, 0x00, 0x00, 0x00,
		0x06, 0x23, 0x12, 0x01,
		0x02, 0x03, 0x04, 0x07,
		0xaa, 0xaa, 0x01, 0x02,
		0x03, 0x04, 0x05, 0x06,
	}
	encodedBytesStr = hex.EncodeToString(encodedBytes)

	_, err = dcrjson.DecodeConcatenatedVoteBits(encodedBytesStr)
	if err == nil {
		t.Fatalf("expected corruption error")
	}

	// Test invalid length.
	encodedBytes = []byte{
		0x01, 0x00, 0x00, 0x00,
		0x06, 0x23, 0x12, 0x01,
		0x02, 0x03, 0x04, 0x07,
		0xaa, 0xaa, 0x01, 0x02,
		0x03, 0x04, 0x05, 0x06,
	}
	encodedBytesStr = hex.EncodeToString(encodedBytes)

	_, err = dcrjson.DecodeConcatenatedVoteBits(encodedBytesStr)
	if err == nil {
		t.Fatalf("expected corruption error")
	}
}

func TestInvalidDecodeConcatenatedHashes(t *testing.T) {
	testStrings := []struct {
		str string
		err dcrjson.RPCError
	}{
		{
			// length of 1
			"0",
			dcrjson.RPCError{
				Code: dcrjson.ErrRPCInvalidParameter,
			},
		}, {
			// not hex
			"ffffgfffffffffffffffffffffffffff" +
				"ffffffffffffffffffffffffffffffff",
			dcrjson.RPCError{
				Code: dcrjson.ErrRPCDecodeHexString,
			},
		}, {
			// invalid length
			"298e5cc3d985bfe811edd4396b86d2de66b0cef4" +
				"2b21d980096b86d2de96b86d2",
			dcrjson.RPCError{
				Code: dcrjson.ErrRPCInvalidParameter,
			},
		},
	}
	for _, str := range testStrings {
		_, err := dcrjson.DecodeConcatenatedHashes(str.str)
		if err == nil {
			t.Fatalf("DecodeConcatenatedHashes passed on '%s' "+
				"when it should have failed", str.str)
		}
		rpcError, ok := err.(*dcrjson.RPCError)
		if !ok {
			t.Fatalf("DecodeConcatenatedHashes error is not "+
				"expected type *dcrjson.RPCError: %T", err)
		}
		if rpcError.Code != str.err.Code {
			t.Fatalf("DecodeConcatenatedHashes returned "+
				"unexpected error code: want %v, got %v",
				str.err.Code, rpcError.Code)
		}
	}
}
