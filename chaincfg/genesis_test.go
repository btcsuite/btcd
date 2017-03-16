// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaincfg

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/davecgh/go-spew/spew"
)

// TestGenesisBlock tests the genesis block of the main network for validity by
// checking the encoded bytes and hashes.
func TestGenesisBlock(t *testing.T) {

	genesisBlockBytes, _ := hex.DecodeString("0100000000000000000000000000" +
		"000000000000000000000000000000000000000000000dc101dfc3c6a2eb10ca0" +
		"c5374e10d28feb53f7eabcc850511ceadb99174aa660000000000000000000000" +
		"00000000000000000000000000000000000000000000000000000000000000000" +
		"000000000ffff011b00c2eb0b000000000000000000000000a0d7b85600000000" +
		"00000000000000000000000000000000000000000000000000000000000000000" +
		"00000000101000000010000000000000000000000000000000000000000000000" +
		"000000000000000000ffffffff00ffffffff01000000000000000000002080167" +
		"9e98561ada96caec2949a5d41c4cab3851eb740d951c10ecbcf265c1fd9000000" +
		"000000000001ffffffffffffffff00000000ffffffff02000000")

	// Encode the genesis block to raw bytes.
	var buf bytes.Buffer
	err := MainNetParams.GenesisBlock.Serialize(&buf)
	if err != nil {
		t.Fatalf("TestGenesisBlock: %v", err)
	}

	// Ensure the encoded block matches the expected bytes.
	if !bytes.Equal(buf.Bytes(), genesisBlockBytes) {
		t.Fatalf("TestGenesisBlock: Genesis block does not appear valid - "+
			"got %v, want %v", spew.Sdump(buf.Bytes()),
			spew.Sdump(genesisBlockBytes))
	}

	// Check hash of the block against expected hash.
	hash := MainNetParams.GenesisBlock.BlockHash()
	if !MainNetParams.GenesisHash.IsEqual(&hash) {
		t.Fatalf("TestGenesisBlock: Genesis block hash does not "+
			"appear valid - got %v, want %v", spew.Sdump(hash),
			spew.Sdump(MainNetParams.GenesisHash))
	}
}

// TestTestNetGenesisBlock tests the genesis block of the test network (version
// 9) for validity by checking the encoded bytes and hashes.
func TestTestNetGenesisBlock(t *testing.T) {
	// Encode the genesis block to raw bytes.
	var buf bytes.Buffer
	err := TestNet2Params.GenesisBlock.Serialize(&buf)
	if err != nil {
		t.Fatalf("TestTestNetGenesisBlock: %v", err)
	}

	testNetGenesisBlockBytes, _ := hex.DecodeString("0400000000000000000" +
		"000000000000000000000000000000000000000000000000000000571a3f8bb" +
		"1903091a18edd3efbc324c79876764af2424071a480d3f04ea16a2000000000" +
		"000000000000000000000000000000000000000000000000000000000000000" +
		"000000000000000000000000ffff001e002d310100000000000000000000000" +
		"040bcc8581aa4ae180000000000000000000000000000000000000000000000" +
		"000000000000000000000000000101000000010000000000000000000000000" +
		"000000000000000000000000000000000000000ffffffff00ffffffff010000" +
		"0000000000000000434104678afdb0fe5548271967f1a67130b7105cd6a828e" +
		"03909a67962e0ea1f61deb649f6bc3f4cef38c4f35504e51ec112de5c384df7" +
		"ba0b8d578a4c702b6bf11d5fac0000000000000000010000000000000000000" +
		"00000000000004d04ffff001d0104455468652054696d65732030332f4a616e" +
		"2f32303039204368616e63656c6c6f72206f6e206272696e6b206f662073656" +
		"36f6e64206261696c6f757420666f722062616e6b7300")

	// Ensure the encoded block matches the expected bytes.
	if !bytes.Equal(buf.Bytes(), testNetGenesisBlockBytes) {
		t.Fatalf("TestTestNetGenesisBlock: Genesis block does not "+
			"appear valid - got %v, want %v",
			spew.Sdump(buf.Bytes()),
			spew.Sdump(testNetGenesisBlockBytes))
	}

	// Check hash of the block against expected hash.
	hash := TestNet2Params.GenesisBlock.BlockHash()
	if !TestNet2Params.GenesisHash.IsEqual(&hash) {
		t.Fatalf("TestTestNetGenesisBlock: Genesis block hash does "+
			"not appear valid - got %v, want %v", spew.Sdump(hash),
			spew.Sdump(TestNet2Params.GenesisHash))
	}
}

// TestSimNetGenesisBlock tests the genesis block of the simulation test network
// for validity by checking the encoded bytes and hashes.
func TestSimNetGenesisBlock(t *testing.T) {
	// Encode the genesis block to raw bytes.
	var buf bytes.Buffer
	err := SimNetParams.GenesisBlock.Serialize(&buf)
	if err != nil {
		t.Fatalf("TestSimNetGenesisBlock: %v", err)
	}

	simNetGenesisBlockBytes, _ := hex.DecodeString("0100000000000000000" +
		"000000000000000000000000000000000000000000000000000000dc101dfc" +
		"3c6a2eb10ca0c5374e10d28feb53f7eabcc850511ceadb99174aa660000000" +
		"00000000000000000000000000000000000000000000000000000000000000" +
		"000000000000000000000000000ffff7f20000000000000000000000000000" +
		"00000450686530000000000000000000000000000000000000000000000000" +
		"00000000000000000000000000000000101000000010000000000000000000" +
		"000000000000000000000000000000000000000000000ffffffff00fffffff" +
		"f0100000000000000000000434104678afdb0fe5548271967f1a67130b7105" +
		"cd6a828e03909a67962e0ea1f61deb649f6bc3f4cef38c4f35504e51ec112d" +
		"e5c384df7ba0b8d578a4c702b6bf11d5fac000000000000000001000000000" +
		"000000000000000000000004d04ffff001d0104455468652054696d6573203" +
		"0332f4a616e2f32303039204368616e63656c6c6f72206f6e206272696e6b2" +
		"06f66207365636f6e64206261696c6f757420666f722062616e6b7300")

	// Ensure the encoded block matches the expected bytes.
	if !bytes.Equal(buf.Bytes(), simNetGenesisBlockBytes) {
		t.Fatalf("TestSimNetGenesisBlock: Genesis block does not "+
			"appear valid - got %v, want %v",
			spew.Sdump(buf.Bytes()),
			spew.Sdump(simNetGenesisBlockBytes))
	}

	// Check hash of the block against expected hash.
	hash := SimNetParams.GenesisBlock.BlockHash()
	if !SimNetParams.GenesisHash.IsEqual(&hash) {
		t.Fatalf("TestSimNetGenesisBlock: Genesis block hash does "+
			"not appear valid - got %v, want %v", spew.Sdump(hash),
			spew.Sdump(SimNetParams.GenesisHash))
	}
}
