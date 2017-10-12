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

// TestFilterFPRange checks that new filters made with out of range
// false positive targets result in either max or min false positive rates.
func TestFilterFPRange(t *testing.T) {
	tests := []struct {
		name   string
		hash   string
		want   string
		filter *bloom.Filter
	}{
		{
			name:   "fprates > 1 should be clipped at 1",
			hash:   "02981fa052f0481dbc5868f4fc2166035a10f27a03cfd2de67326471df5bc041",
			want:   "00000000000000000001",
			filter: bloom.NewFilter(1, 0, 20.9999999769, wire.BloomUpdateAll),
		},
		{
			name:   "fprates less than 1e-9 should be clipped at min",
			hash:   "02981fa052f0481dbc5868f4fc2166035a10f27a03cfd2de67326471df5bc041",
			want:   "0566d97a91a91b0000000000000001",
			filter: bloom.NewFilter(1, 0, 0, wire.BloomUpdateAll),
		},
		{
			name:   "negative fprates should be clipped at min",
			hash:   "02981fa052f0481dbc5868f4fc2166035a10f27a03cfd2de67326471df5bc041",
			want:   "0566d97a91a91b0000000000000001",
			filter: bloom.NewFilter(1, 0, -1, wire.BloomUpdateAll),
		},
	}

	for _, test := range tests {
		// Convert test input to appropriate types.
		hash, err := chainhash.NewHashFromStr(test.hash)
		if err != nil {
			t.Errorf("NewHashFromStr unexpected error: %v", err)
			continue
		}
		want, err := hex.DecodeString(test.want)
		if err != nil {
			t.Errorf("DecodeString unexpected error: %v\n", err)
			continue
		}

		// Add the test hash to the bloom filter and ensure the
		// filter serializes to the expected bytes.
		f := test.filter
		f.AddHash(hash)
		got := bytes.NewBuffer(nil)
		err = f.MsgFilterLoad().BtcEncode(got, wire.ProtocolVersion)
		if err != nil {
			t.Errorf("BtcDecode unexpected error: %v\n", err)
			continue
		}
		if !bytes.Equal(got.Bytes(), want) {
			t.Errorf("serialized filter mismatch: got %x want %x\n",
				got.Bytes(), want)
			continue
		}
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
	// tx 2 from blk 10000
	str := "0100000001a4fbbbca2416ba4c10c94be9f4a650d37fc4f9a1a4ecded9cc2" +
		"714aa0a529a750000000000ffffffff02c2d0b32f0000000000001976a91" +
		"499678d10a90c8df40e4c9af742aa6ebc7764a60e88acbe01611c0000000" +
		"000001976a9147701528df10cf0c14f9e53925031bd398796c1f988ac000" +
		"000000000000001e0b52b4c0000000003270000020000006b48304502210" +
		"08003ce072e4b67f9a98129ac2f58e3de6e06f47a15e248d4375d19dfb52" +
		"7a02d02204ab0a0dfe7c69024ae8e524e01d1c45183efda945a0d411e4e9" +
		"4b69be21efbe601210270c906c3ba64ba5eb3943cc012a3b142ef169f066" +
		"002515bf9ec1bd9b7e27f0d"
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
	spendingStr := "01000000018c5e1f62f83d750a0ee228c228731eae241e6b483e5b63be199" +
		"12846eb2d11500000000000ffffffff02ed44871d0000000000001976a91" +
		"461788151a27fad1a9c609fa29a2bd43886e2dd4088ac75a815120000000" +
		"000001976a91483419547ee3db5c0ee29f347740ff7f448e8ab2c88ac000" +
		"000000000000001c2d0b32f0000000010270000010000006b48304502210" +
		"0aca38b780893b6be3287efa908ace8bb8b91af0477ab433f101889b86bb" +
		"d9c2d0220789a177956f91c75141ea527573294a20f6fc0ea8bd5cc33550" +
		"4a0654ae197e30121025516815b900e10e51824ea1f451fd197fb11209af" +
		"60c5c52f9a8cf3edad5dc09"
	spendingTxBytes, err := hex.DecodeString(spendingStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failure: %v", err)
		return
	}
	spendingTx, err := dcrutil.NewTxFromBytes(spendingTxBytes)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewTxFromBytes failure: %v", err)
		return
	}

	f := bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr := "50112deb46289119be635b3e486b1e24ae1e7328c228e20e0a753df8621f5e8c"
	hash, err := chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewHashFromStr failed: %v\n", err)
		return
	}
	f.AddHash(hash)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match hash %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "8c5e1f62f83d750a0ee228c228731eae241e6b483e5b63be19912846eb2d1150"
	hashBytes, err := hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failed: %v\n", err)
		return
	}
	f.Add(hashBytes)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match hash %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "30450221008003ce072e4b67f9a98129ac2f58e3de6e06f47a15e248d43" +
		"75d19dfb527a02d02204ab0a0dfe7c69024ae8e524e01d1c45183efda945a0" +
		"d411e4e94b69be21efbe601"
	hashBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failed: %v\n", err)
		return
	}
	f.Add(hashBytes)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match input signature %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "0270c906c3ba64ba5eb3943cc012a3b142ef169f066002515bf9ec1bd9b7e27f0d"
	hashBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failed: %v\n", err)
		return
	}
	f.Add(hashBytes)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match input pubkey %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "99678d10a90c8df40e4c9af742aa6ebc7764a60e"
	hashBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failed: %v\n", err)
		return
	}

	f.Add(hashBytes)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match output address %s", inputStr)
	}
	if !f.MatchTxAndUpdate(spendingTx) {
		t.Errorf("TestFilterBloomMatch spendingTx didn't match output address %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "7701528df10cf0c14f9e53925031bd398796c1f9"
	hashBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failed: %v\n", err)
		return
	}
	f.Add(hashBytes)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match output address %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "759a520aaa1427ccd9deeca4a1f9c47fd350a6f4e94bc9104cba1624cabbfba4"
	hash, err = chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewHashFromStr failed: %v\n", err)
		return
	}
	outpoint := wire.NewOutPoint(hash, 0, wire.TxTreeRegular)
	f.AddOutPoint(outpoint)
	if !f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch didn't match outpoint %s", inputStr)
	}
	// XXX unchanged from btcd
	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "00000009e784f32f62ef849763d4f45b98e07ba658647343b915ff832b110436"
	hash, err = chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewHashFromStr failed: %v\n", err)
		return
	}
	f.AddHash(hash)
	if f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch matched hash %s", inputStr)
	}

	// XXX unchanged from btcd
	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "0000006d2965547608b9e15d9032a7b9d64fa431"
	hashBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch DecodeString failed: %v\n", err)
		return
	}
	f.Add(hashBytes)
	if f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch matched address %s", inputStr)
	}

	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "759a520aaa1427ccd9deeca4a1f9c47fd350a6f4e94bc9104cba1624cabbfba4"
	hash, err = chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewHashFromStr failed: %v\n", err)
		return
	}
	outpoint = wire.NewOutPoint(hash, 1, wire.TxTreeRegular)
	f.AddOutPoint(outpoint)
	if f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch matched outpoint %s", inputStr)
	}

	// XXX unchanged from btcd
	f = bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)
	inputStr = "000000d70786e899529d71dbeba91ba216982fb6ba58f3bdaab65e73b7e9260b"
	hash, err = chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterBloomMatch NewHashFromStr failed: %v\n", err)
		return
	}
	outpoint = wire.NewOutPoint(hash, 0, wire.TxTreeRegular)
	f.AddOutPoint(outpoint)
	if f.MatchTxAndUpdate(tx) {
		t.Errorf("TestFilterBloomMatch matched outpoint %s", inputStr)
	}
}

func TestFilterInsertUpdateNone(t *testing.T) {
	f := bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateNone)

	// Add the generation pubkey
	inputStr := "0270c906c3ba64ba5eb3943cc012a3b142ef169f066002515bf9ec1bd9b7e27f0d"
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
	hash, err := chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterInsertUpdateNone NewHashFromStr failed: %v", err)
		return
	}
	outpoint := wire.NewOutPoint(hash, 0, wire.TxTreeRegular)

	if f.MatchesOutPoint(outpoint) {
		t.Errorf("TestFilterInsertUpdateNone matched outpoint %s", inputStr)
		return
	}

	inputStr = "02981fa052f0481dbc5868f4fc2166035a10f27a03cfd2de67326471df5bc041"
	hash, err = chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestFilterInsertUpdateNone NewHashFromStr failed: %v", err)
		return
	}
	outpoint = wire.NewOutPoint(hash, 0, wire.TxTreeRegular)

	if f.MatchesOutPoint(outpoint) {
		t.Errorf("TestFilterInsertUpdateNone matched outpoint %s", inputStr)
		return
	}
}

func TestFilterInsertP2PubKeyOnly(t *testing.T) {
	blockStr := "000000003ffad804971ce6b8a13c8162287222d91395fa77b6bea6b4" +
		"626b4827000000004259bd8a4d5d5f8469f390903d27b9cab2ea03822fbe" +
		"616478756ada751a283be8e4aa58eeb75810e22031cc2756442b7f68c77b" +
		"3735522fc5490be1676253c301005f031703bb61050000005c1300006fb2" +
		"381c30dd35680000000091940000fc0900000cadf15600d9781d3a5f237b" +
		"083799410f00000000000000000000000000000000000000000000000000" +
		"000002010000000100000000000000000000000000000000000000000000" +
		"00000000000000000000ffffffff00ffffffff032727750c000000000000" +
		"17a9144fa6cbd0dbe5ec407fe4c8ad374e667771fa0d4487000000000000" +
		"00000000266a249194000000000000000000000000000000000000000000" +
		"0000000000b90c53023ee736e6d7eebe4a0000000000001976a914bfe0f6" +
		"3eda5d7db3ee05661051c026f5296ffc7888ac0000000000000000011512" +
		"34570000000000000000ffffffff0800002f646372642f01000000020c57" +
		"441e66eaa72bc76ab54faaa0ec87941f4423a463c5e34d7f23f247d65e9d" +
		"0200000001ffffffff31b7ffaec935e787dc03670d4d13ee0066717cb750" +
		"2cb4d925b09b7f692a73580400000001ffffffff0200ca9a3b0000000000" +
		"0023210351b64c01163b9184637671e27d6d29b3a205203710f6fbc5a6e1" +
		"b646a11984d6ac81001f060000000000001976a9144feba3e04d91d9dc90" +
		"ebb13ce91bf9bcfe32ff9288ac000000000000000002c29f493300000000" +
		"8f9400000a0000006a4730440220186741de60d6fe75d2206b62780edabe" +
		"8a0a13f823aaeca58f0d5a8dedfa99f202201dc84a1bd26d14723ddf2fe4" +
		"216d4a24d788597fa22255d144ce119d7544ce39012103879a3b3666bf03" +
		"88ab33e6e24e77c3c219044f7c5d778b71ac1837817095830da72e700e00" +
		"000000909400000e0000006b4830450221008cc441f58be0eedc713b867d" +
		"c8dfff21e2f6c2e38a906a0af5aa0625b2683f6b0220308bcaf6628b3c68" +
		"19da76ebb2ca5e5d1ddde6e5035193d75e3e667731deadc4012102a871fe" +
		"4e4f77121f1cbaf2db62190539687092b465e252a3bd732791ca442d5405" +
		"010000000200000000000000000000000000000000000000000000000000" +
		"00000000000000ffffffff00ffffffff91ed802c4a23cca0517dbc2d89f7" +
		"b6f8a2d1491a27ed25e3d3f621bac03316da0000000001ffffffff030000" +
		"0000000000000000266a243ffad804971ce6b8a13c8162287222d91395fa" +
		"77b6bea6b4626b4827000000009094000000000000000000000000046a02" +
		"010003d0db190100000000001abb76a914339f1f6a41d7ef65ffd80bbb8c" +
		"daaa215a16472d88ac000000000000000002e47d79070000000000000000" +
		"ffffffff04deadbeef1f52621201000000938b0000090000006a47304402" +
		"207184b4d7a95a559b1142f50cfde3e40bf460572aea4d5eeabad062e45c" +
		"9a8f5102202304acbcc9cc55e58770e51d4fc9ac81af9c790a65d251ea4d" +
		"556bc7ee96fb47012103c46bdbf6b24be97eac230ca9b23e928e2e76750d" +
		"3d3c8b29e15cebb64ba01f86010000000200000000000000000000000000" +
		"00000000000000000000000000000000000000ffffffff00ffffffff17be" +
		"9d003549030c20c018a71745af6f6a02d96637f49f5a548ee7fe0ac84420" +
		"0000000001ffffffff0300000000000000000000266a243ffad804971ce6" +
		"b8a13c8162287222d91395fa77b6bea6b4626b4827000000009094000000" +
		"000000000000000000046a02010071c339cf0000000000001abb76a9147f" +
		"1aaac9f04febbf9dadd262b1c710729970445988ac000000000000000002" +
		"e47d79070000000000000000ffffffff04deadbeef8d45c0c70000000099" +
		"8f0000070000006a473044022054a09af013f1a74960686bf5c36f3eb822" +
		"5ffc96e76a54dc1aac9fdea65cff2b022010359dfcd16ac77f3a6ab47309" +
		"e106a885fa2d031d5bac0824f4432a77ccdae001210389e445603d66ce44" +
		"92ec74d9b1974268a2e83413c84df87895184bc6f46ae1d7010000000200" +
		"000000000000000000000000000000000000000000000000000000000000" +
		"00ffffffff00ffffffff49a883465f82c0483b3ff7da057cab44a42ecbc8" +
		"27c344a960a2a33c95263f760000000001ffffffff030000000000000000" +
		"0000266a243ffad804971ce6b8a13c8162287222d91395fa77b6bea6b462" +
		"6b4827000000009094000000000000000000000000046a0201002c9d63bf" +
		"0000000000001abb76a91449d402e31fc08414f565febaa1b69eccc2fb8d" +
		"2688ac000000000000000002e47d79070000000000000000ffffffff04de" +
		"adbeef481feab70000000064870000120000006a473044022008daa49081" +
		"d7dc6c466a4d3fd4184927e8b77fd1f5721f206bf0d13a2c00b07802202d" +
		"89841ad5346de14db95c3e5b15a0d82e0df1f50a235f67aeec08b98f4b36" +
		"7a012102907261a4670a9d0ff918724af50f2ecb7947831dad30ac285afa" +
		"1b90549aa282010000000200000000000000000000000000000000000000" +
		"00000000000000000000000000ffffffff00ffffffffb6df4ea74c958f39" +
		"65ecc90b9226cfdc2d7986b1df30998cf3e827d7e9fbf4b80000000001ff" +
		"ffffff0300000000000000000000266a243ffad804971ce6b8a13c816228" +
		"7222d91395fa77b6bea6b4626b4827000000009094000000000000000000" +
		"000000046a020100f3a613b90000000000001abb76a9147f9321c65ee805" +
		"d87f67b0b0e9dac4f0779c1d3a88ac000000000000000002e47d79070000" +
		"000000000000ffffffff04deadbeef0f299ab100000000408d0000130000" +
		"006b4830450221008c5759dd9f0c40c61f61f5b67c0b3f860d4d59859fb2" +
		"451dbeab9956b3ceb05602203e81d5b88b83e279dc7feca13c8e5eb58676" +
		"158243058ec43d6366933397be4a0121031e6757ee86e94a02e567db7d4e" +
		"c4ba5088f6b5fdbfe9b2794b70b42b52b879200100000002000000000000" +
		"0000000000000000000000000000000000000000000000000000ffffffff" +
		"00ffffffff9e74ead771c313f0c0bb739860eba39d04a8cce201cda626dc" +
		"6a7dd4e83b3f770000000001ffffffff0400000000000000000000266a24" +
		"3ffad804971ce6b8a13c8162287222d91395fa77b6bea6b4626b48270000" +
		"00009094000000000000000000000000046a0201000793a0e00000000000" +
		"001abb76a914c8c5a2cbd183871718dcb14b5f198561450d157e88ac596e" +
		"8f250000000000001abb76a9147112f7d015baeb129e732b9c4805e492ef" +
		"d0a2fb88ac000000000000000002e47d79070000000000000000ffffffff" +
		"04deadbeef7d83b6fe0000000022920000070000006a4730440220160ff7" +
		"bd84190d8d837ac6bd782be475d23d50832872626453bd70ea63d9a6a002" +
		"20603afdd51fc8e3fead1fd9dbae410b8d0617969ad5119e6dc2b0855d7b" +
		"48e73501210297ab850cae270e9438693353e40444c7e714e179505e8b12" +
		"1c042f4869e42072"
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
	inputStr := "0351b64c01163b9184637671e27d6d29b3a205203710f6fbc5a6e1b646a11984d6"
	inputBytes, err := hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterInsertP2PubKeyOnly DecodeString failed: %v", err)
		return
	}
	f.Add(inputBytes)

	// Output address of 2nd transaction
	inputStr = "4feba3e04d91d9dc90ebb13ce91bf9bcfe32ff92"
	inputBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterInsertP2PubKeyOnly DecodeString failed: %v", err)
		return
	}
	f.Add(inputBytes)

	// Ignore return value -- this is just used to update the filter.
	_, _ = bloom.NewMerkleBlock(block, f)

	// We should match the generation pubkey
	inputStr = "8199f30ccc006056ed79cf0a3cd0b67a195ffd46903d42adc7babe2ed2f2e371"
	hash, err := chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestMerkleBlockP2PubKeyOnly NewHashFromStr failed: %v", err)
		return
	}
	outpoint := wire.NewOutPoint(hash, 0, wire.TxTreeRegular)
	if !f.MatchesOutPoint(outpoint) {
		t.Errorf("TestMerkleBlockP2PubKeyOnly didn't match the generation "+
			"outpoint %s", inputStr)
		return
	}

	// We should not match the 4th transaction, which is not p2pk
	inputStr = "d7314aaf54253c651e8258c7d22c574af8804611f0dcea79fd9a47f4565d85ad"
	hash, err = chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestMerkleBlockP2PubKeyOnly NewHashFromStr failed: %v", err)
		return
	}
	outpoint = wire.NewOutPoint(hash, 0, wire.TxTreeRegular)
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
