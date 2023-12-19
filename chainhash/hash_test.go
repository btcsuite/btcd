// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chainhash

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"testing"
)

// mainNetGenesisHash is the hash of the first block in the block chain for the
// main network (genesis block).
var mainNetGenesisHash = Hash([HashSize]byte{ // Make go vet happy.
	0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
	0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
	0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
	0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00,
})

// TestHash tests the Hash API.
func TestHash(t *testing.T) {
	// Hash of block 234439.
	blockHashStr := "14a0810ac680a3eb3f82edc878cea25ec41d6b790744e5daeef"
	blockHash, err := NewHashFromStr(blockHashStr)
	if err != nil {
		t.Errorf("NewHashFromStr: %v", err)
	}

	// Hash of block 234440 as byte slice.
	buf := []byte{
		0x79, 0xa6, 0x1a, 0xdb, 0xc6, 0xe5, 0xa2, 0xe1,
		0x39, 0xd2, 0x71, 0x3a, 0x54, 0x6e, 0xc7, 0xc8,
		0x75, 0x63, 0x2e, 0x75, 0xf1, 0xdf, 0x9c, 0x3f,
		0xa6, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}

	hash, err := NewHash(buf)
	if err != nil {
		t.Errorf("NewHash: unexpected error %v", err)
	}

	// Ensure proper size.
	if len(hash) != HashSize {
		t.Errorf("NewHash: hash length mismatch - got: %v, want: %v",
			len(hash), HashSize)
	}

	// Ensure contents match.
	if !bytes.Equal(hash[:], buf) {
		t.Errorf("NewHash: hash contents mismatch - got: %v, want: %v",
			hash[:], buf)
	}

	// Ensure contents of hash of block 234440 don't match 234439.
	if hash.IsEqual(blockHash) {
		t.Errorf("IsEqual: hash contents should not match - got: %v, want: %v",
			hash, blockHash)
	}

	// Set hash from byte slice and ensure contents match.
	err = hash.SetBytes(blockHash.CloneBytes())
	if err != nil {
		t.Errorf("SetBytes: %v", err)
	}
	if !hash.IsEqual(blockHash) {
		t.Errorf("IsEqual: hash contents mismatch - got: %v, want: %v",
			hash, blockHash)
	}

	// Ensure nil hashes are handled properly.
	if !(*Hash)(nil).IsEqual(nil) {
		t.Error("IsEqual: nil hashes should match")
	}
	if hash.IsEqual(nil) {
		t.Error("IsEqual: non-nil hash matches nil hash")
	}

	// Invalid size for SetBytes.
	err = hash.SetBytes([]byte{0x00})
	if err == nil {
		t.Errorf("SetBytes: failed to received expected err - got: nil")
	}

	// Invalid size for NewHash.
	invalidHash := make([]byte, HashSize+1)
	_, err = NewHash(invalidHash)
	if err == nil {
		t.Errorf("NewHash: failed to received expected err - got: nil")
	}
}

// TestHashString  tests the stringized output for hashes.
func TestHashString(t *testing.T) {
	// Block 100000 hash.
	wantStr := "000000000003ba27aa200b1cecaad478d2b00432346c3f1f3986da1afd33e506"
	hash := Hash([HashSize]byte{ // Make go vet happy.
		0x06, 0xe5, 0x33, 0xfd, 0x1a, 0xda, 0x86, 0x39,
		0x1f, 0x3f, 0x6c, 0x34, 0x32, 0x04, 0xb0, 0xd2,
		0x78, 0xd4, 0xaa, 0xec, 0x1c, 0x0b, 0x20, 0xaa,
		0x27, 0xba, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00,
	})

	hashStr := hash.String()
	if hashStr != wantStr {
		t.Errorf("String: wrong hash string - got %v, want %v",
			hashStr, wantStr)
	}
}

// TestNewHashFromStr executes tests against the NewHashFromStr function.
func TestNewHashFromStr(t *testing.T) {
	tests := []struct {
		in   string
		want Hash
		err  error
	}{
		// Genesis hash.
		{
			"000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f",
			mainNetGenesisHash,
			nil,
		},

		// Genesis hash with stripped leading zeros.
		{
			"19d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f",
			mainNetGenesisHash,
			nil,
		},

		// Empty string.
		{
			"",
			Hash{},
			nil,
		},

		// Single digit hash.
		{
			"1",
			Hash([HashSize]byte{ // Make go vet happy.
				0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
				0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			}),
			nil,
		},

		// Block 203707 with stripped leading zeros.
		{
			"3264bc2ac36a60840790ba1d475d01367e7c723da941069e9dc",
			Hash([HashSize]byte{ // Make go vet happy.
				0xdc, 0xe9, 0x69, 0x10, 0x94, 0xda, 0x23, 0xc7,
				0xe7, 0x67, 0x13, 0xd0, 0x75, 0xd4, 0xa1, 0x0b,
				0x79, 0x40, 0x08, 0xa6, 0x36, 0xac, 0xc2, 0x4b,
				0x26, 0x03, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			}),
			nil,
		},

		// Hash string that is too long.
		{
			"01234567890123456789012345678901234567890123456789012345678912345",
			Hash{},
			ErrHashStrSize,
		},

		// Hash string that is contains non-hex chars.
		{
			"abcdefg",
			Hash{},
			hex.InvalidByteError('g'),
		},
	}

	unexpectedErrStr := "NewHashFromStr #%d failed to detect expected error - got: %v want: %v"
	unexpectedResultStr := "NewHashFromStr #%d got: %v want: %v"
	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result, err := NewHashFromStr(test.in)
		if err != test.err {
			t.Errorf(unexpectedErrStr, i, err, test.err)
			continue
		} else if err != nil {
			// Got expected error. Move on to the next test.
			continue
		}
		if !test.want.IsEqual(result) {
			t.Errorf(unexpectedResultStr, i, result, &test.want)
			continue
		}
	}
}

// TestHashJsonMarshal tests json marshal and unmarshal.
func TestHashJsonMarshal(t *testing.T) {
	hashStr := "000000000003ba27aa200b1cecaad478d2b00432346c3f1f3986da1afd33e506"
	legacyHashStr := []byte("[6,229,51,253,26,218,134,57,31,63,108,52,50,4,176,210,120,212,170,236,28,11,32,170,39,186,3,0,0,0,0,0]")

	hash, err := NewHashFromStr(hashStr)
	if err != nil {
		t.Errorf("NewHashFromStr error:%v, hashStr:%s", err, hashStr)
	}

	hashBytes, err := json.Marshal(hash)
	if err != nil {
		t.Errorf("Marshal json error:%v, hash:%v", err, hashBytes)
	}

	var newHash Hash
	err = json.Unmarshal(hashBytes, &newHash)
	if err != nil {
		t.Errorf("Unmarshal json error:%v, hash:%v", err, hashBytes)
	}

	if !hash.IsEqual(&newHash) {
		t.Errorf("String: wrong hash string - got %v, want %v", newHash.String(), hashStr)
	}

	err = newHash.UnmarshalJSON(legacyHashStr)
	if err != nil {
		t.Errorf("Unmarshal legacy json error:%v, hash:%v", err, legacyHashStr)
	}

	if !hash.IsEqual(&newHash) {
		t.Errorf("String: wrong hash string - got %v, want %v", newHash.String(), hashStr)
	}
}
