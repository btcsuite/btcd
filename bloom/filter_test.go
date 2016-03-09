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
	//2a94d783a177460fd00c633ce3011f0e172a721097887ab2de983d741dc8d8f5"
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
	inputStr = "8c5e1f62f83d750a0ee228c228731eae241e6b483e5b63be19912846eb2d1150"
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
	inputStr = "30450221008003ce072e4b67f9a98129ac2f58e3de6e06f47a15e248d43" +
		"75d19dfb527a02d02204ab0a0dfe7c69024ae8e524e01d1c45183efda945a0" +
		"d411e4e94b69be21efbe601"
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
	inputStr = "0270c906c3ba64ba5eb3943cc012a3b142ef169f066002515bf9ec1bd9b7e27f0d"
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
	inputStr = "99678d10a90c8df40e4c9af742aa6ebc7764a60e"
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
	inputStr = "7701528df10cf0c14f9e53925031bd398796c1f9"
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
	inputStr = "759a520aaa1427ccd9deeca4a1f9c47fd350a6f4e94bc9104cba1624cabbfba4"
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
	// XXX unchanged from btcd
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

	// XXX unchanged from btcd
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
	inputStr = "759a520aaa1427ccd9deeca4a1f9c47fd350a6f4e94bc9104cba1624cabbfba4"
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

	// XXX unchanged from btcd
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

/*  XXX This test is commented out because we don't have a pay to pubkey tx to use
func TestFilterInsertP2PubKeyOnly(t *testing.T) {
	blockStr := "0100000010e21dfb17cb74db95913afb88178fc0f807e1dbdc103d7d951e000" +
		"000000000d77800ef9ca953feda5dfbc2732080a36f5bef139f5e6967ed70eb2e66" +
		"2db6158d773a15b0be63cb6d2c9ae2624327cc23cd048c80a378bcdf99882d006e1" +
		"656010057025082b9fe0500010086a50000a9483b1a3b60bd550000000010270000" +
		"460b00008d58e556d105d8d10d00000002883001e30003ed0000000000000000000" +
		"0000000000000000000000000000003010000000100000000000000000000000000" +
		"00000000000000000000000000000000000000ffffffff00ffffffff03c2f968120" +
		"0000000000017a914f5916158e3e2c4551c1796708db8367207ed13bb8700000000" +
		"000000000000266a241027000000000000000000000000000000000000000000000" +
		"000000033c029e4037975438fecef6e0000000000001976a914a53c75f1b00401d1" +
		"f6b99378b42a510fba047cf488ac00000000000000000151d4de800000000000000" +
		"000ffffffff0800002f646372642f0100000001a4fbbbca2416ba4c10c94be9f4a6" +
		"50d37fc4f9a1a4ecded9cc2714aa0a529a750000000000ffffffff02c2d0b32f000" +
		"0000000001976a91499678d10a90c8df40e4c9af742aa6ebc7764a60e88acbe0161" +
		"1c0000000000001976a9147701528df10cf0c14f9e53925031bd398796c1f988ac0" +
		"00000000000000001e0b52b4c0000000003270000020000006b4830450221008003" +
		"ce072e4b67f9a98129ac2f58e3de6e06f47a15e248d4375d19dfb527a02d02204ab" +
		"0a0dfe7c69024ae8e524e01d1c45183efda945a0d411e4e94b69be21efbe6012102" +
		"70c906c3ba64ba5eb3943cc012a3b142ef169f066002515bf9ec1bd9b7e27f0d010" +
		"00000012255285989c87ae46390d6f9264b0caec6a5dcd0b617ab91751abd2c7399" +
		"96f00000000000ffffffff02c68c7c3b0000000000001976a9140bda096c8d1dbff" +
		"2f8d8cbb1128d75a583757d7888aca1b4a61d0000000000001976a9147dbb01c8a8" +
		"4270cb216b9876116be20f3a50842988ac000000000000000001c7243a590000000" +
		"00e270000010000006b483045022100b8dd3bfe1f2615a49c1e036fd96c0317bbde" +
		"b6969ef2ef5e3b76cf08e2a9ded80220445a7784234e22345b1eba645064de19c06" +
		"a4aca2bab900bae73e9db3a2d0714012102d2e8fce341baf5ca38a4a1b326d83e88" +
		"2351bb636a27ae8254c5f3a7522a894d06010000000200000000000000000000000" +
		"00000000000000000000000000000000000000000ffffffff00fffffffffc791ced" +
		"ee5775a758b8e6e923229ee8e95a10ea3d3a67664534d347fa3953450000000001f" +
		"fffffff0300000000000000000000266a2410e21dfb17cb74db95913afb88178fc0" +
		"f807e1dbdc103d7d951e0000000000000f27000000000000000000000000046a020" +
		"1000e8bf7160000000000001abb76a914f663ba38f4dd832c44fb2f542ede4c2666" +
		"363d1888ac0000000000000000020ec90b0b0000000000000000ffffffff0200000" +
		"0c2eb0b000000001a010000100000006b483045022100e8e6e72446fa2dc3b7a7bf" +
		"0b4432c80a4e22f210ae189a0072ba4b969ce55eab02207d8cc45ddaa162fee8140" +
		"eb6e022e0e51254460f09bfbe5376b41b6e3f791c7d01210347deaf15765d2beafa" +
		"dbe889e0f54c166e373d999c257c5f7179976ed9b70d6c010000000200000000000" +
		"00000000000000000000000000000000000000000000000000000ffffffff00ffff" +
		"ffff495a0ed5592ba2b26b0acfb7b9e325dca51fa1a93bf6336492a4267c7182c27" +
		"80000000001ffffffff0300000000000000000000266a2410e21dfb17cb74db9591" +
		"3afb88178fc0f807e1dbdc103d7d951e0000000000000f270000000000000000000" +
		"00000046a0201000e8bf7160000000000001abb76a914b3653f61256f0370d9c46c" +
		"beaf2678098ecaae8c88ac0000000000000000020ec90b0b0000000000000000fff" +
		"fffff02000000c2eb0b00000000ff030000120000006b483045022100de2129a98d" +
		"10632cdc8131b77b88cf8fcdc68104b1c31578d1f07c372b6bb37f02204f692b1bf" +
		"326d120f462ae786fd054259269e95f83dcd43bf37f3f359766848c012102a997b5" +
		"3a4deee42ca96822adcbb5b63980c08187257eb7138c5f5967ea9b5e84010000000" +
		"20000000000000000000000000000000000000000000000000000000000000000ff" +
		"ffffff00ffffffff636c0e52ed7d6afea4828e7147600ce778557ef424028b150ca" +
		"b895516cb36450000000001ffffffff0300000000000000000000266a2410e21dfb" +
		"17cb74db95913afb88178fc0f807e1dbdc103d7d951e0000000000000f270000000" +
		"00000000000000000046a020100f3a93b310000000000001abb76a9144f93a2c1b4" +
		"09e6aa399d37e4a435f0086b87c43088ac0000000000000000020ec90b0b0000000" +
		"000000000ffffffff020000e5e02f2600000000ab1f00000f0000006b4830450221" +
		"00a121c61f6b5d5012de930614a84ca6e7f798889069e3b8a92ebe5fbcd94c2d1d0" +
		"2205f5e8a42269dab2f10b6c5a528265355156ff4d2eba6620c262c1a73e1ce2838" +
		"012102553f2256d720656e16dc2cecb1bbd0ea94280f78617090b53e37534c4afaf" +
		"12b0100000002000000000000000000000000000000000000000000000000000000" +
		"0000000000ffffffff00ffffffff7df80e77cb5de63fb8626c8026f7cc4b59247f3" +
		"cb57ba80d7509775e3bf6347a0000000001ffffffff030000000000000000000026" +
		"6a2410e21dfb17cb74db95913afb88178fc0f807e1dbdc103d7d951e00000000000" +
		"00f27000000000000000000000000046a0201000e8bf7160000000000001abb76a9" +
		"14ca61454827fa8ec70c0b50932146a3c06b40fd9788ac0000000000000000020ec" +
		"90b0b0000000000000000ffffffff02000000c2eb0b000000001513000005000000" +
		"6a47304402200d73da1175b966c1cf79589cd6d0e0ecac74e7c34f83977924de127" +
		"b1224260a02202861be0cb6287ed56cf9b82f6612dd63413fecb9ad10c5961ada87" +
		"602125d5a4012102db8c515901c9a241d3fec1d5ece0a3bb8f86801b1fd71e24a42" +
		"b2c09e72a0dd6010000000200000000000000000000000000000000000000000000" +
		"00000000000000000000ffffffff00ffffffff2835a038696cad2553a3971acd220" +
		"a30be80aedb7d0a3aeb5357c9a7a666cdb70000000001ffffffff03000000000000" +
		"00000000266a2410e21dfb17cb74db95913afb88178fc0f807e1dbdc103d7d951e0" +
		"000000000000f27000000000000000000000000046a0201000e8bf7160000000000" +
		"001abb76a9140d7fc0368c66be6431599ee1e677fc37d142b1c788ac00000000000" +
		"00000020ec90b0b0000000000000000ffffffff02000000c2eb0b00000000770300" +
		"00090000006a47304402203bdf3e84676c6049c17149c581f8a0a8d64173dde8bbd" +
		"77f139f5896bab9f77702203287ee60b18d564aedd5223e886b88f0fb62813a577c" +
		"7e68ed4661880428846001210302a52bbbe12b2f66d3d98bf6d680aaa99256d88c4" +
		"fda136084f182221a24e8c001000000013fa4b56a1b8656d3168195c90977bb7466" +
		"3c6afdb240a964362c10a18f3e63d90200000001ffffffff033b60bd55000000000" +
		"0001aba76a914842aafd3611db1dcee0f277675590b4763dbd03a88ac0000000000" +
		"0000000000206a1e1c7603e2af492e7b721a330ae80e6858c2faf0cb7bab0956000" +
		"000000058c1f38f4f0000000000001abd76a914c41686ff7f20728574b4203e0c64" +
		"ab835383155788ac0000000000000000013c9f99a5000000000f270000060000006" +
		"b483045022100dbbff39a1e8dad70505aaf800c22c3f63322905a5604e4e23d514a" +
		"36f7367f9102203a5689ce4eb8dfc08c09d71fd288504b36accc5ac92a0372f7bef" +
		"0ed4fee1d0c012102f1cc83e3cac89e660b4a7d3c217b4ec78a47d982b6cddbbcf0" +
		"6860d2d522a386"
	blockBytes, err := hex.DecodeString(blockStr)
	if err != nil {
		t.Errorf("TestFilterInsertP2PubKeyOnly DecodeString failed: %v", err)
		return
	}
	fmt.Println(blockStr)
	block, err := dcrutil.NewBlockFromBytes(blockBytes)
	if err != nil {
		t.Errorf("TestFilterInsertP2PubKeyOnly NewBlockFromBytes failed: %v", err)
		return
	}
	spew.Dump(block)
	f := bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateP2PubkeyOnly)

	// Generation pubkey
	inputStr := "0270c906c3ba64ba5eb3943cc012a3b142ef169f066002515bf9ec1bd9b7e27f0d"
	inputBytes, err := hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterInsertP2PubKeyOnly DecodeString failed: %v", err)
		return
	}
	f.Add(inputBytes)

	// Output address of 2nd transaction
	inputStr = "99678d10a90c8df40e4c9af742aa6ebc7764a60e"
	inputBytes, err = hex.DecodeString(inputStr)
	if err != nil {
		t.Errorf("TestFilterInsertP2PubKeyOnly DecodeString failed: %v", err)
		return
	}
	f.Add(inputBytes)

	// Ignore return value -- this is just used to update the filter.
	_, _ = bloom.NewMerkleBlock(block, f)

	// We should match the generation pubkey
	inputStr = "391fb1bf339518e41c5a6d124590e776db1d35fffa9205e31dd9a30b60633d79"
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
	inputStr = "391fb1bf339518e41c5a6d124590e776db1d35fffa9205e31dd9a30b60633d79"
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
*/
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
