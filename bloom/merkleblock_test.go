// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bloom_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/decred/dcrd/bloom"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
)

func TestMerkleBlock3(t *testing.T) {
	blockStr := "0100000073cf056852529ffadc50b49589218795adc4d3f24170950d49f201000000000033fd46dda0acfa5c0651c58bee00362b04186c5b4d1045d37751b25779148649000000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000ffff011b00c2eb0b00000000000100007e0100006614b956bee4fc44442bf144050552b3010000000000000000000000000000000000000000000000000000000101000000010000000000000000000000000000000000000000000000000000000000000000ffffffff00ffffffff03fa1a981200000000000017a914f5916158e3e2c4551c1796708db8367207ed13bb8700000000000000000000266a24000100000000000000000000000000000000000000000000000000004dfb774daa8f3a76dea1906f0000000000001976a914b74b7476bdbcf03d18f12cca1766b0ddfd030cdd88ac000000000000000001d8bc28820000000000000000ffffffff0800002f646372642f00"

	blockBytes, err := hex.DecodeString(blockStr)
	if err != nil {
		t.Errorf("TestMerkleBlock3 DecodeString failed: %v", err)
		return
	}
	blk, err := dcrutil.NewBlockFromBytes(blockBytes)
	if err != nil {
		t.Errorf("TestMerkleBlock3 NewBlockFromBytes failed: %v", err)
		return
	}
	f := bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)

	inputStr := "4986147957b25177d345104d5b6c18042b3600ee8bc551065cfaaca0dd46fd33"
	hash, err := chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestMerkleBlock3 NewHashFromStr failed: %v", err)
		return
	}

	f.AddHash(hash)

	mBlock, _ := bloom.NewMerkleBlock(blk, f)
	wantStr := "0100000073cf056852529ffadc50b49589218795adc4d3f24170950d49f201000000000033fd46dda0acfa5c0651c58bee00362b04186c5b4d1045d37751b25779148649000000000000000000000000000000000000000000000000000000000000000001000000000000000000000000000000ffff011b00c2eb0b00000000000100007e0100006614b956bee4fc44442bf144050552b3010000000000000000000000000000000000000000000000000000000100000001d0f51e5a4978d736eb3d4a8d615bee74943756b4745d29b27ab2943bc1307cc800000000000100"
	want, err := hex.DecodeString(wantStr)
	if err != nil {
		t.Errorf("TestMerkleBlock3 DecodeString failed: %v", err)
		return
	}

	got := bytes.NewBuffer(nil)
	err = mBlock.BtcEncode(got, wire.ProtocolVersion)
	if err != nil {
		t.Errorf("TestMerkleBlock3 BtcEncode failed: %v", err)
		return
	}

	if !bytes.Equal(want, got.Bytes()) {
		t.Errorf("TestMerkleBlock3 failed merkle block comparison: "+
			"got %v want %v", got.Bytes(), want)
		return
	}
}
