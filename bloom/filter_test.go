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

// TestFilterLarge ensures a maximum sized filter can be created.
func TestFilterLarge(t *testing.T) {
	f := bloom.NewFilter(100000000, 0, 0.01, wire.BloomUpdateNone)
	if len(f.MsgFilterLoad().Filter) > wire.MaxFilterLoadFilterSize {
		t.Errorf("TestFilterLarge test failed: %d > %d",
			len(f.MsgFilterLoad().Filter), wire.MaxFilterLoadFilterSize)
	}
}

// TestFilterLoad ensures loading and unloading of a filter pass.
func TestFilterLoad(t *testing.T) {
	merkle := wire.MsgFilterLoad{}

	f := bloom.LoadFilter(&merkle)
	if !f.IsLoaded() {
		t.Errorf("TestFilterLoad IsLoaded test failed: want %v got %v",
			true, !f.IsLoaded())
		return
	}
	f.Unload()
	if f.IsLoaded() {
		t.Errorf("TestFilterLoad IsLoaded test failed: want %v got %v",
			f.IsLoaded(), false)
		return
	}
}

// TestFilterInsert ensures inserting data into the filter causes that data
// to be matched and the resulting serialized MsgFilterLoad is the expected
// value.
func TestFilterInsert(t *testing.T) {
	var tests = []struct {
		hex    string
		insert bool
	}{
		{"99108ad8ed9bb6274d3980bab5a85c048f0950c8", true},
		{"19108ad8ed9bb6274d3980bab5a85c048f0950c8", false},
		{"b5a2c786d9ef4658287ced5914b37a1b4aa32eee", true},
		{"b9300670b4c5366e95b2699e8b18bc75e5f729c5", true},
	}

	f := bloom.NewFilter(3, 0, 0.01, wire.BloomUpdateAll)

	for i, test := range tests {
		data, err := hex.DecodeString(test.hex)
		if err != nil {
			t.Errorf("TestFilterInsert DecodeString failed: %v\n", err)
			return
		}
		if test.insert {
			f.Add(data)
		}

		result := f.Matches(data)
		if test.insert != result {
			t.Errorf("TestFilterInsert Matches test #%d failure: got %v want %v\n",
				i, result, test.insert)
			return
		}
	}

	want, err := hex.DecodeString("03614e9b050000000000000001")
	if err != nil {
		t.Errorf("TestFilterInsert DecodeString failed: %v\n", err)
		return
	}

	got := bytes.NewBuffer(nil)
	err = f.MsgFilterLoad().BtcEncode(got, wire.ProtocolVersion)
	if err != nil {
		t.Errorf("TestFilterInsert BtcDecode failed: %v\n", err)
		return
	}

	if !bytes.Equal(got.Bytes(), want) {
		t.Errorf("TestFilterInsert failure: got %v want %v\n",
			got.Bytes(), want)
		return
	}
}

// TestFilterInsert ensures inserting data into the filter with a tweak causes
// that data to be matched and the resulting serialized MsgFilterLoad is the
// expected value.
func TestFilterInsertWithTweak(t *testing.T) {
	var tests = []struct {
		hex    string
		insert bool
	}{
		{"99108ad8ed9bb6274d3980bab5a85c048f0950c8", true},
		{"19108ad8ed9bb6274d3980bab5a85c048f0950c8", false},
		{"b5a2c786d9ef4658287ced5914b37a1b4aa32eee", true},
		{"b9300670b4c5366e95b2699e8b18bc75e5f729c5", true},
	}

	f := bloom.NewFilter(3, 2147483649, 0.01, wire.BloomUpdateAll)

	for i, test := range tests {
		data, err := hex.DecodeString(test.hex)
		if err != nil {
			t.Errorf("TestFilterInsertWithTweak DecodeString failed: %v\n", err)
			return
		}
		if test.insert {
			f.Add(data)
		}

		result := f.Matches(data)
		if test.insert != result {
			t.Errorf("TestFilterInsertWithTweak Matches test #%d failure: got %v want %v\n",
				i, result, test.insert)
			return
		}
	}

	want, err := hex.DecodeString("03ce4299050000000100008001")
	if err != nil {
		t.Errorf("TestFilterInsertWithTweak DecodeString failed: %v\n", err)
		return
	}
	got := bytes.NewBuffer(nil)
	err = f.MsgFilterLoad().BtcEncode(got, wire.ProtocolVersion)
	if err != nil {
		t.Errorf("TestFilterInsertWithTweak BtcDecode failed: %v\n", err)
		return
	}

	if !bytes.Equal(got.Bytes(), want) {
		t.Errorf("TestFilterInsertWithTweak failure: got %v want %v\n",
			got.Bytes(), want)
		return
	}
}

// TestFilterInsertKey ensures inserting public keys and addresses works as
// expected.
func TestFilterInsertKey(t *testing.T) {
	secret := "PtWU93QdrNBasyWA7GDJ3ycEN5aQRF69EynXJfmnyWDS4G7pzpEvN"

	wif, err := dcrutil.DecodeWIF(secret)
	if err != nil {
		t.Errorf("TestFilterInsertKey DecodeWIF failed: %v", err)
		return
	}

	f := bloom.NewFilter(2, 0, 0.001, wire.BloomUpdateAll)
	f.Add(wif.SerializePubKey())
	f.Add(dcrutil.Hash160(wif.SerializePubKey()))

	want, err := hex.DecodeString("03323f6e080000000000000001")
	if err != nil {
		t.Errorf("TestFilterInsertWithTweak DecodeString failed: %v\n", err)
		return
	}
	got := bytes.NewBuffer(nil)
	err = f.MsgFilterLoad().BtcEncode(got, wire.ProtocolVersion)
	if err != nil {
		t.Errorf("TestFilterInsertWithTweak BtcDecode failed: %v\n", err)
		return
	}

	if !bytes.Equal(got.Bytes(), want) {
		t.Errorf("TestFilterInsertWithTweak failure: got %v want %v\n",
			got.Bytes(), want)
		return
	}
}

func TestFilterBloomMatch(t *testing.T) {
	str := "0100000001cd24a3a215761757d2b565994b5f544be8cc7996e7835a0b326" +
		"feed9f8a3072802000000016a47304402200d1f455ff952f3fae2f9b3d5c" +
		"471371ba9d09789abb5cd80f01c6576c6d760e70220283aaa6e18a043a3b" +
		"5d105d6fccaca8edf31b705b75d82488414366ca48bdda60121030e233e9" +
		"6e98d98336bd2b3f4ec23aaa3c9b5c9b57de6d408490a9ce36c9d4326fff" +
		"fffff0600ca9a3b000000001976a914e6719fd3749c5ed40f193928e8b2a" +
		"d0dd163efe588acf82f357a0a0000001976a91468447782004ae8718618f" +
		"d752f1d02e285f897d488ac00ca9a3b000000001976a9148d2b19f048851" +
		"857afd872acde7ccf93bf16f5d488ac00ca9a3b000000001976a914e5aaf" +
		"268a042555daabf1e33030f12f61d663ea088ac00ca9a3b000000001976a" +
		"914f02e3cb22c39d3e6e0179241e52f21df097b17ae88ac00ca9a3b00000" +
		"0001976a9142ea99ce64965bf8cfb40e8e1b7bf4d2cea89db1988ac00000" +
		"000"
	strBytes, err := hex.DecodeString(str)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failure: %v", err)
		return
	}
	tx, err := dcrutil.NewTxFromBytes(strBytes)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewTxFromBytes failure: %v", err)
		return
	}
	spendingTxBytes := []byte{0x01, 0x00, 0x00, 0x00, 0x01, 0x6b, 0xff, 0x7f,
		0xcd, 0x4f, 0x85, 0x65, 0xef, 0x40, 0x6d, 0xd5, 0xd6,
		0x3d, 0x4f, 0xf9, 0x4f, 0x31, 0x8f, 0xe8, 0x20, 0x27,
		0xfd, 0x4d, 0xc4, 0x51, 0xb0, 0x44, 0x74, 0x01, 0x9f,
		0x74, 0xb4, 0x00, 0x00, 0x00, 0x00, 0x8c, 0x49, 0x30,
		0x46, 0x02, 0x21, 0x00, 0xda, 0x0d, 0xc6, 0xae, 0xce,
		0xfe, 0x1e, 0x06, 0xef, 0xdf, 0x05, 0x77, 0x37, 0x57,
		0xde, 0xb1, 0x68, 0x82, 0x09, 0x30, 0xe3, 0xb0, 0xd0,
		0x3f, 0x46, 0xf5, 0xfc, 0xf1, 0x50, 0xbf, 0x99, 0x0c,
		0x02, 0x21, 0x00, 0xd2, 0x5b, 0x5c, 0x87, 0x04, 0x00,
		0x76, 0xe4, 0xf2, 0x53, 0xf8, 0x26, 0x2e, 0x76, 0x3e,
		0x2d, 0xd5, 0x1e, 0x7f, 0xf0, 0xbe, 0x15, 0x77, 0x27,
		0xc4, 0xbc, 0x42, 0x80, 0x7f, 0x17, 0xbd, 0x39, 0x01,
		0x41, 0x04, 0xe6, 0xc2, 0x6e, 0xf6, 0x7d, 0xc6, 0x10,
		0xd2, 0xcd, 0x19, 0x24, 0x84, 0x78, 0x9a, 0x6c, 0xf9,
		0xae, 0xa9, 0x93, 0x0b, 0x94, 0x4b, 0x7e, 0x2d, 0xb5,
		0x34, 0x2b, 0x9d, 0x9e, 0x5b, 0x9f, 0xf7, 0x9a, 0xff,
		0x9a, 0x2e, 0xe1, 0x97, 0x8d, 0xd7, 0xfd, 0x01, 0xdf,
		0xc5, 0x22, 0xee, 0x02, 0x28, 0x3d, 0x3b, 0x06, 0xa9,
		0xd0, 0x3a, 0xcf, 0x80, 0x96, 0x96, 0x8d, 0x7d, 0xbb,
		0x0f, 0x91, 0x78, 0xff, 0xff, 0xff, 0xff, 0x02, 0x8b,
		0xa7, 0x94, 0x0e, 0x00, 0x00, 0x00, 0x00, 0x19, 0x76,
		0xa9, 0x14, 0xba, 0xde, 0xec, 0xfd, 0xef, 0x05, 0x07,
		0x24, 0x7f, 0xc8, 0xf7, 0x42, 0x41, 0xd7, 0x3b, 0xc0,
		0x39, 0x97, 0x2d, 0x7b, 0x88, 0xac, 0x40, 0x94, 0xa8,
		0x02, 0x00, 0x00, 0x00, 0x00, 0x19, 0x76, 0xa9, 0x14,
		0xc1, 0x09, 0x32, 0x48, 0x3f, 0xec, 0x93, 0xed, 0x51,
		0xf5, 0xfe, 0x95, 0xe7, 0x25, 0x59, 0xf2, 0xcc, 0x70,
		0x43, 0xf9, 0x88, 0xac, 0x00, 0x00, 0x00, 0x00, 0x00}

	spendingTx, err := dcrutil.NewTxFromBytes(spendingTxBytes)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewTxFromBytes failure: %v", err)
		return
	}

	f := bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr := "b4749f017444b051c44dfd2720e88f314ff94f3dd6d56d40ef65854fcd7fff6b"
	sha, err := chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewShaHashFromStr failed: %v\n", err)
		return
	}
	f.AddShaHash(sha)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match sha %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "6bff7fcd4f8565ef406dd5d63d4ff94f318fe82027fd4dc451b04474019f74b4"
	shaBytes, err := hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failed: %v\n", err)
		return
	}
	f.Add(shaBytes)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match sha %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "30450220070aca44506c5cef3a16ed519d7c3c39f8aab192c4e1c90d065" +
		"f37b8a4af6141022100a8e160b856c2d43d27d8fba71e5aef6405b8643" +
		"ac4cb7cb3c462aced7f14711a01"
	shaBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failed: %v\n", err)
		return
	}
	f.Add(shaBytes)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match input signature %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "046d11fee51b0e60666d5049a9101a72741df480b96ee26488a4d3466b95" +
		"c9a40ac5eeef87e10a5cd336c19a84565f80fa6c547957b7700ff4dfbdefe" +
		"76036c339"
	shaBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failed: %v\n", err)
		return
	}
	f.Add(shaBytes)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match input pubkey %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "04943fdd508053c75000106d3bc6e2754dbcff19"
	shaBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failed: %v\n", err)
		return
	}
	f.Add(shaBytes)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match output address %s", inputStr)
	}
	if !f.MatchTxAndUpdate(spendingTx) {
		t.Errorf("TestFilterBloomMatch spendingTx didn't match output address %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "a266436d2965547608b9e15d9032a7b9d64fa431"
	shaBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failed: %v\n", err)
		return
	}
	f.Add(shaBytes)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match output address %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "90c122d70786e899529d71dbeba91ba216982fb6ba58f3bdaab65e73b7e9260b"
	sha, err = chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewShaHashFromStr failed: %v\n", err)
		return
	}
	outpoint := wire.NewOutPoint(sha, 0, dcrutil.TxTreeRegular)
	f.AddOutPoint(outpoint)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match outpoint %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "00000009e784f32f62ef849763d4f45b98e07ba658647343b915ff832b110436"
	sha, err = chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewShaHashFromStr failed: %v\n", err)
		return
	}
	f.AddShaHash(sha)
	if f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch matched sha %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "0000006d2965547608b9e15d9032a7b9d64fa431"
	shaBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failed: %v\n", err)
		return
	}
	f.Add(shaBytes)
	if f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch matched address %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "90c122d70786e899529d71dbeba91ba216982fb6ba58f3bdaab65e73b7e9260b"
	sha, err = chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewShaHashFromStr failed: %v\n", err)
		return
	}
	outpoint = wire.NewOutPoint(sha, 1, dcrutil.TxTreeRegular)
	f.AddOutPoint(outpoint)
	if f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch matched outpoint %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "000000d70786e899529d71dbeba91ba216982fb6ba58f3bdaab65e73b7e9260b"
	sha, err = chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewShaHashFromStr failed: %v\n", err)
		return
	}
	outpoint = wire.NewOutPoint(sha, 0, dcrutil.TxTreeRegular)
	f.AddOutPoint(outpoint)
	if f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch matched outpoint %s", inputStr)
	}
}

func TestFilterInsertUpdateNone(t *testing.T) {
	f := bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateNone)

	// Add the generation pubkey
	inputStr := "04eaafc2314def4ca98ac970241bcab022b9c1e1f4ea423a20f134c" +
		"876f2c01ec0f0dd5b2e86e7168cefe0d81113c3807420ce13ad1357231a" +
		"2252247d97a46a91"
	inputBytes, err := hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterInsertUpdateNone DecodeString failed: %v", err)
		return
	}
	f.Add(inputBytes)

	// Add the output address for the 4th transaction
	inputStr = "b6efd80d99179f4f4ff6f4dd0a007d018c385d21"
	inputBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterInsertUpdateNone DecodeString failed: %v", err)
		return
	}
	f.Add(inputBytes)

	inputStr = "147caa76786596590baa4e98f5d9f48b86c7765e489f7a6ff3360fe5c674360b"
	sha, err := chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterInsertUpdateNone NewShaHashFromStr failed: %v", err)
		return
	}
	outpoint := wire.NewOutPoint(sha, 0, dcrutil.TxTreeRegular)

	if f.MatchesOutPoint(outpoint) {
		t.Errorf("TestFilterInsertUpdateNone matched outpoint %s", inputStr)
		return
	}

	inputStr = "02981fa052f0481dbc5868f4fc2166035a10f27a03cfd2de67326471df5bc041"
	sha, err = chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterInsertUpdateNone NewShaHashFromStr failed: %v", err)
		return
	}
	outpoint = wire.NewOutPoint(sha, 0, dcrutil.TxTreeRegular)

	if f.MatchesOutPoint(outpoint) {
		t.Errorf("TestFilterInsertUpdateNone matched outpoint %s", inputStr)
		return
	}
}

func TestFilterInsertP2PubKeyOnly(t *testing.T) {
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
		t.Errorf("TestFilterInsertP2PubKeyOnly DecodeString failed: %v", err)
		return
	}
	block, err := dcrutil.NewBlockFromBytes(blockBytes)
	if err != nil {
		t.Errorf("TestFilterInsertP2PubKeyOnly NewBlockFromBytes failed: %v", err)
		return
	}

	f := bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateP2PubkeyOnly)

	// Generation pubkey
	inputStr := "04eaafc2314def4ca98ac970241bcab022b9c1e1f4ea423a20f134c" +
		"876f2c01ec0f0dd5b2e86e7168cefe0d81113c3807420ce13ad1357231a" +
		"2252247d97a46a91"
	inputBytes, err := hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterInsertP2PubKeyOnly DecodeString failed: %v", err)
		return
	}
	f.Add(inputBytes)

	// Output address of 4th transaction
	inputStr = "b6efd80d99179f4f4ff6f4dd0a007d018c385d21"
	inputBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterInsertP2PubKeyOnly DecodeString failed: %v", err)
		return
	}
	f.Add(inputBytes)

	// Ignore return value -- this is just used to update the filter.
	_, _ = bloom.NewMerkleBlock(block, f)

	// We should match the generation pubkey
	inputStr = "147caa76786596590baa4e98f5d9f48b86c7765e489f7a6ff3360fe5c674360b"
	sha, err := chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestMerkleBlockP2PubKeyOnly NewShaHashFromStr failed: %v", err)
		return
	}
	outpoint := wire.NewOutPoint(sha, 0, dcrutil.TxTreeRegular)
	if !f.MatchesOutPoint(outpoint) {
		t.Errorf("TestMerkleBlockP2PubKeyOnly didn't match the generation "+
			"outpoint %s", inputStr)
		return
	}

	// We should not match the 4th transaction, which is not p2pk
	inputStr = "02981fa052f0481dbc5868f4fc2166035a10f27a03cfd2de67326471df5bc041"
	sha, err = chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestMerkleBlockP2PubKeyOnly NewShaHashFromStr failed: %v", err)
		return
	}
	outpoint = wire.NewOutPoint(sha, 0, dcrutil.TxTreeRegular)
	if f.MatchesOutPoint(outpoint) {
		t.Errorf("TestMerkleBlockP2PubKeyOnly matched outpoint %s", inputStr)
		return
	}
}

func TestFilterReload(t *testing.T) {
	f := bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)

	bFilter := bloom.LoadFilter(f.MsgFilterLoad())
	if bFilter.MsgFilterLoad() == nil {
		t.Errorf("TestFilterReload LoadFilter test failed")
		return
	}
	bFilter.Reload(nil)

	if bFilter.MsgFilterLoad() != nil {
		t.Errorf("TestFilterReload Reload test failed")
	}
}
