// Copyright (c) 2013, 2014 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bloom_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
	"github.com/decred/dcrutil/bloom"
)

func TestMerkleBlock3(t *testing.T) {
	blockStr := "000000004ad131bae9cb9f74b8bcd928" +
		"a60dfe4dadabeb31b1e79403385f9ac4" +
		"ccc28b7400429e56f7df2872aaaa0c16" +
		"221cb09059bd3ea897de156ff51202ff" +
		"72b2cd8d000000000000000000000000" +
		"00000000000000000000000000000000" +
		"00000000010000000000000000000000" +
		"22000000ffff7f20002d310100000000" +
		"640000007601000063a0815601000000" +
		"00000000000000000000000000000000" +
		"00000000000000000000000000000000" +
		"00000000010100000001000000000000" +
		"00000000000000000000000000000000" +
		"00000000000000000000ffffffff00ff" +
		"ffffff0380b2e60e00000000000017a9" +
		"144fa6cbd0dbe5ec407fe4c8ad374e66" +
		"7771fa0d448700000000000000000000" +
		"226a2000000000000000000000000000" +
		"0000009e0453a6ab10610e17a7a5fadc" +
		"f6c34f002f68590000000000001976a9" +
		"141b79e6496226f89ad4e049667c1344" +
		"c16a75815188ac000000000000000001" +
		"000000000000000000000000ffffffff" +
		"04deadbeef00"

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

	inputStr := "4797be83be7b4c4f833739c3542c2c1c403ffb01f0b721b5bc5dee3ff655a856"
	sha, err := chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestMerkleBlock3 NewShaHashFromStr failed: %v", err)
		return
	}

	f.AddShaHash(sha)

	mBlock, _ := bloom.NewMerkleBlock(blk, f)

	wantStr := "0100000079cda856b143d9db2c1caff01d1aecc8630d30625d10e8b4" +
		"b8b0000000000000b50cc069d6a3e33e3ff84a5c41d9d3febe7c770fdcc" +
		"96b2c3ff60abe184f196367291b4d4c86041b8fa45d630100000001b50c" +
		"c069d6a3e33e3ff84a5c41d9d3febe7c770fdcc96b2c3ff60abe184f196" +
		"30101"
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
			"got %v want %v", got.Bytes, want)
		return
	}
}
