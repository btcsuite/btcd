// Copyright (c) 2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcutil_test

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// TestSortTx tests SortTx function
func TestSortTx(t *testing.T) {
	//	 Use block 100,000 transaction 1.  Already sorted.
	testTx := Block100000.Transactions[1]
	sortedTx := btcutil.TxSort(testTx)

	testTxid := testTx.TxSha()
	sortedTxid := sortedTx.TxSha()
	if !testTxid.IsEqual(&sortedTxid) {
		t.Errorf("Sorted TxSha mismatch - got %v, want %v",
			testTxid.String(), sortedTxid.String())
	}
	if !btcutil.TxIsSorted(testTx) {
		t.Errorf("testTx %v is sorted but reported as unsorted",
			testTxid.String())
	}

	// Example 1 0a6a357e2f7796444e02638749d9611c008b253fb55f5dc88b739b230ed0c4c3
	// test transaction 0a6a357e... which is the first test case of BIPLI01
	li01Tx1bytes, err := hex.DecodeString(LI01Hex1)
	if err != nil {
		t.Errorf("Error in hardcoded hex")
	}

	var li01Tx1 wire.MsgTx
	err = li01Tx1.Deserialize(bytes.NewReader(li01Tx1bytes))
	if err != nil {
		t.Errorf("Failed to Deserialize li01Tx from byte slice")
	}

	if btcutil.TxIsSorted(&li01Tx1) {
		t.Errorf("LI01 Test Transaction 1 seen as sorted, but isn't")
	}

	li01Tx1Sorted := btcutil.TxSort(&li01Tx1)
	// txid of 0a6a357e... becomes 839503c... when sorted
	wantSha := newShaHashFromStr("839503cb611a3e3734bd521c608f881be2293ff77b7384057ab994c794fce623")

	li01Tx1SortedSha := li01Tx1Sorted.TxSha()
	if !wantSha.IsEqual(&li01Tx1SortedSha) {
		t.Errorf("Sorted tx 1 txid mismatch. Got %v, want %v",
			li01Tx1SortedSha, wantSha)
	}

	// check that original transaction is not modified
	wantSha = newShaHashFromStr("0a6a357e2f7796444e02638749d9611c008b253fb55f5dc88b739b230ed0c4c3")

	li01Tx1Sha := li01Tx1.TxSha()
	if !wantSha.IsEqual(&li01Tx1Sha) {
		t.Errorf("Original tx 1 txid mismatch. Got %v, want %v",
			li01Tx1Sha, wantSha)
	}

	// Example 2 28204cad1d7fc1d199e8ef4fa22f182de6258a3eaafe1bbe56ebdcacd3069a5f
	// test transaction 28204cad..., the second test case of BIPLI01
	li01Tx2bytes, err := hex.DecodeString(LI01Hex2)
	if err != nil {
		t.Errorf("Error in hardcoded hex")
	}

	var li01Tx2 wire.MsgTx
	err = li01Tx2.Deserialize(bytes.NewReader(li01Tx2bytes))
	if err != nil {
		t.Errorf("Failed to Deserialize li01Tx from byte slice")
	}
	if !btcutil.TxIsSorted(&li01Tx2) {
		t.Errorf("LI01 Test Transaction 2 seen as unsorted, but it is sorted")
	}

	li01Tx2Sorted := btcutil.TxSort(&li01Tx2)

	// txid of 28204cad... stays the same when sorted
	wantSha = newShaHashFromStr("28204cad1d7fc1d199e8ef4fa22f182de6258a3eaafe1bbe56ebdcacd3069a5f")

	li01Tx2SortedSha := li01Tx2Sorted.TxSha()
	if !wantSha.IsEqual(&li01Tx2SortedSha) {
		t.Errorf("Example tx 2 txid mismatch. Got %v, want %v",
			li01Tx2SortedSha, wantSha)
	}

	// Example 3 8131ffb0a2c945ecaf9b9063e59558784f9c3a74741ce6ae2a18d0571dac15bb
	li01Tx3bytes, err := hex.DecodeString(LI01Hex3)
	if err != nil {
		t.Errorf("Error in hardcoded hex")
	}

	var li01Tx3 wire.MsgTx
	err = li01Tx3.Deserialize(bytes.NewReader(li01Tx3bytes))
	if err != nil {
		t.Errorf("Failed to Deserialize li01Tx3 from byte slice")
	}
	if btcutil.TxIsSorted(&li01Tx3) {
		t.Errorf("LI01 Test Transaction 3 seen as sorted, but isn't")
	}

	li01Tx3Sorted := btcutil.TxSort(&li01Tx3)

	// txid of 8131ffb0... changes to 0a8c246... when sorted
	wantSha = newShaHashFromStr("0a8c246c55f6b82f094d211f4f57167bf2ea4898741d218b09bdb2536fd8d13f")

	li01Tx3SortedSha := li01Tx3Sorted.TxSha()
	if !wantSha.IsEqual(&li01Tx3SortedSha) {
		t.Errorf("Example tx 3 txid mismatch. Got %v, want %v",
			li01Tx3SortedSha, wantSha)
	}

	// Example 4 fbde5d03b027d2b9ba4cf5d4fecab9a99864df2637b25ea4cbcb1796ff6550ca
	li01Tx4bytes, err := hex.DecodeString(LI01Hex4)
	if err != nil {
		t.Errorf("Error in hardcoded hex")
	}

	var li01Tx4 wire.MsgTx
	err = li01Tx4.Deserialize(bytes.NewReader(li01Tx4bytes))
	if err != nil {
		t.Errorf("Failed to Deserialize li01Tx4 from byte slice")
	}
	if btcutil.TxIsSorted(&li01Tx4) {
		t.Errorf("LI01 Test Transaction 4 seen as sorted, but isn't")
	}

	li01Tx4Sorted := btcutil.TxSort(&li01Tx4)

	// txid of 8131ffb0... changes to 0a8c246... when sorted
	wantSha = newShaHashFromStr("a3196553b928b0b6154b002fa9a1ce875adabc486fedaaaf4c17430fd4486329")

	li01Tx4SortedSha := li01Tx4Sorted.TxSha()
	if !wantSha.IsEqual(&li01Tx4SortedSha) {
		t.Errorf("Example tx 4 txid mismatch. Got %v, want %v",
			li01Tx4SortedSha, wantSha)
	}

	// Example 5 ff85e8fc92e71bbc217e3ea9a3bacb86b435e52b6df0b089d67302c293a2b81d
	li01Tx5bytes, err := hex.DecodeString(LI01Hex5)
	if err != nil {
		t.Errorf("Error in hardcoded hex")
	}

	var li01Tx5 wire.MsgTx
	err = li01Tx5.Deserialize(bytes.NewReader(li01Tx5bytes))
	if err != nil {
		t.Errorf("Failed to Deserialize li01Tx5 from byte slice")
	}
	if btcutil.TxIsSorted(&li01Tx5) {
		t.Errorf("LI01 Test Transaction 5 seen as sorted, but isn't")
	}

	li01Tx5Sorted := btcutil.TxSort(&li01Tx5)

	// txid of ff85e8f... changes to 9a6c247... when sorted
	wantSha = newShaHashFromStr("9a6c24746de024f77cac9b2138694f11101d1c66289261224ca52a25155a7c94")

	li01Tx5SortedSha := li01Tx5Sorted.TxSha()
	if !wantSha.IsEqual(&li01Tx5SortedSha) {
		t.Errorf("Example tx 5 txid mismatch. Got %v, want %v",
			li01Tx5SortedSha, wantSha)
	}
}

// newShaHashFromStr converts the passed big-endian hex string into a
// wire.ShaHash.  It only differs from the one available in wire in that
// it panics on an error since it will only (and must only) be called with
// hard-coded, and therefore known good, hashes.
// Copied from chaincfg tests
func newShaHashFromStr(hexStr string) *wire.ShaHash {
	sha, err := wire.NewShaHashFromStr(hexStr)
	if err != nil {
		panic(err)
	}
	return sha
}

// Example 1 sorts inputs, but leaves outputs unchanged
var LI01Hex1 = "0100000011aad553bb1650007e9982a8ac79d227cd8c831e1573b11f25573a37664e5f3e64000000006a47304402205438cedd30ee828b0938a863e08d810526123746c1f4abee5b7bc2312373450c02207f26914f4275f8f0040ab3375bacc8c5d610c095db8ed0785de5dc57456591a601210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffffc26f3eb7932f7acddc5ddd26602b77e7516079b03090a16e2c2f5485d1fde028000000006b483045022100f81d98c1de9bb61063a5e6671d191b400fda3a07d886e663799760393405439d0220234303c9af4bad3d665f00277fe70cdd26cd56679f114a40d9107249d29c979401210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffff456a9e597129f5df2e11b842833fc19a94c563f57449281d3cd01249a830a1f0000000006a47304402202310b00924794ef68a8f09564fd0bb128838c66bc45d1a3f95c5cab52680f166022039fc99138c29f6c434012b14aca651b1c02d97324d6bd9dd0ffced0782c7e3bd01210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffff571fb3e02278217852dd5d299947e2b7354a639adc32ec1fa7b82cfb5dec530e000000006b483045022100d276251f1f4479d8521269ec8b1b45c6f0e779fcf1658ec627689fa8a55a9ca50220212a1e307e6182479818c543e1b47d62e4fc3ce6cc7fc78183c7071d245839df01210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffff5d8de50362ff33d3526ac3602e9ee25c1a349def086a7fc1d9941aaeb9e91d38010000006b4830450221008768eeb1240451c127b88d89047dd387d13357ce5496726fc7813edc6acd55ac022015187451c3fb66629af38fdb061dfb39899244b15c45e4a7ccc31064a059730d01210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffff60ad3408b89ea19caf3abd5e74e7a084344987c64b1563af52242e9d2a8320f3000000006b4830450221009be4261ec050ebf33fa3d47248c7086e4c247cafbb100ea7cee4aa81cd1383f5022008a70d6402b153560096c849d7da6fe61c771a60e41ff457aac30673ceceafee01210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffffe9b483a8ac4129780c88d1babe41e89dc10a26dedbf14f80a28474e9a11104de010000006b4830450221009bc40eee321b39b5dc26883f79cd1f5a226fc6eed9e79e21d828f4c23190c57e022078182fd6086e265589105023d9efa4cba83f38c674a499481bd54eee196b033f01210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffffe28db9462d3004e21e765e03a45ecb147f136a20ba8bca78ba60ebfc8e2f8b3b000000006a47304402200fb572b7c6916515452e370c2b6f97fcae54abe0793d804a5a53e419983fae1602205191984b6928bf4a1e25b00e5b5569a0ce1ecb82db2dea75fe4378673b53b9e801210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffff7a1ef65ff1b7b7740c662ab6c9735ace4a16279c23a1db5709ed652918ffff54010000006a47304402206bc218a925f7280d615c8ea4f0131a9f26e7fc64cff6eeeb44edb88aba14f1910220779d5d67231bc2d2d93c3c5ab74dcd193dd3d04023e58709ad7ffbf95161be6201210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffff850cecf958468ca7ffa6a490afe13b8c271b1326b0ddc1fdfdf9f3c7e365fdba000000006a473044022047df98cc26bd2bfdc5b2b97c27aead78a214810ff023e721339292d5ce50823d02205fe99dc5f667908974dae40cc7a9475af7fa6671ba44f64a00fcd01fa12ab523012102ca46fa75454650afba1784bc7b079d687e808634411e4beff1f70e44596308a1ffffffff8640e312040e476cf6727c60ca3f4a3ad51623500aacdda96e7728dbdd99e8a5000000006a47304402205566aa84d3d84226d5ab93e6f253b57b3ef37eb09bb73441dae35de86271352a02206ee0b7f800f73695a2073a2967c9ad99e19f6ddf18ce877adf822e408ba9291e01210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffff91c1889c5c24b93b56e643121f7a05a34c10c5495c450504c7b5afcb37e11d7a000000006b483045022100df61d45bbaa4571cdd6c5c822cba458cdc55285cdf7ba9cd5bb9fc18096deb9102201caf8c771204df7fd7c920c4489da7bc3a60e1d23c1a97e237c63afe53250b4a01210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffff2470947216eb81ea0eeeb4fe19362ec05767db01c3aa3006bb499e8b6d6eaa26010000006a473044022031501a0b2846b8822a32b9947b058d89d32fc758e009fc2130c2e5effc925af70220574ef3c9e350cef726c75114f0701fd8b188c6ec5f84adce0ed5c393828a5ae001210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffff0abcd77d65cc14363f8262898335f184d6da5ad060ff9e40bf201741022c2b40010000006b483045022100a6ac110802b699f9a2bff0eea252d32e3d572b19214d49d8bb7405efa2af28f1022033b7563eb595f6d7ed7ec01734e17b505214fe0851352ed9c3c8120d53268e9a01210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffffa43bebbebf07452a893a95bfea1d5db338d23579be172fe803dce02eeb7c037d010000006b483045022100ebc77ed0f11d15fe630fe533dc350c2ddc1c81cfeb81d5a27d0587163f58a28c02200983b2a32a1014bab633bfc9258083ac282b79566b6b3fa45c1e6758610444f401210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffffb102113fa46ce949616d9cda00f6b10231336b3928eaaac6bfe42d1bf3561d6c010000006a473044022010f8731929a55c1c49610722e965635529ed895b2292d781b183d465799906b20220098359adcbc669cd4b294cc129b110fe035d2f76517248f4b7129f3bf793d07f01210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffffb861fab2cde188499758346be46b5fbec635addfc4e7b0c8a07c0a908f2b11b4000000006a47304402207328142bb02ef5d6496a210300f4aea71f67683b842fa3df32cae6c88b49a9bb022020f56ddff5042260cfda2c9f39b7dec858cc2f4a76a987cd2dc25945b04e15fe01210391064d5b2d1c70f264969046fcff853a7e2bfde5d121d38dc5ebd7bc37c2b210ffffffff027064d817000000001976a9144a5fba237213a062f6f57978f796390bdcf8d01588ac00902f50090000001976a9145be32612930b8323add2212a4ec03c1562084f8488ac00000000"

// Example 2 is already sorted
var LI01Hex2 = "010000000255605dc6f5c3dc148b6da58442b0b2cd422be385eab2ebea4119ee9c268d28350000000049483045022100aa46504baa86df8a33b1192b1b9367b4d729dc41e389f2c04f3e5c7f0559aae702205e82253a54bf5c4f65b7428551554b2045167d6d206dfe6a2e198127d3f7df1501ffffffff55605dc6f5c3dc148b6da58442b0b2cd422be385eab2ebea4119ee9c268d2835010000004847304402202329484c35fa9d6bb32a55a70c0982f606ce0e3634b69006138683bcd12cbb6602200c28feb1e2555c3210f1dddb299738b4ff8bbe9667b68cb8764b5ac17b7adf0001ffffffff0200e1f505000000004341046a0765b5865641ce08dd39690aade26dfbf5511430ca428a3089261361cef170e3929a68aee3d8d4848b0c5111b0a37b82b86ad559fd2a745b44d8e8d9dfdc0cac00180d8f000000004341044a656f065871a353f216ca26cef8dde2f03e8c16202d2e8ad769f02032cb86a5eb5e56842e92e19141d60a01928f8dd2c875a390f67c1f6c94cfc617c0ea45afac00000000"

// Example 3 sorts outputs only.  Block 10001 tx[2]
var LI01Hex3 = "0100000001d992e5a888a86d4c7a6a69167a4728ee69497509740fc5f456a24528c340219a000000008b483045022100f0519bdc9282ff476da1323b8ef7ffe33f495c1a8d52cc522b437022d83f6a230220159b61d197fbae01b4a66622a23bc3f1def65d5fa24efd5c26fa872f3a246b8e014104839f9023296a1fabb133140128ca2709f6818c7d099491690bd8ac0fd55279def6a2ceb6ab7b5e4a71889b6e739f09509565eec789e86886f6f936fa42097adeffffffff02000fe208010000001976a914948c765a6914d43f2a7ac177da2c2f6b52de3d7c88ac00e32321000000001976a9140c34f4e29ab5a615d5ea28d4817f12b137d62ed588ac00000000"

// Example 4 sorts both inputs and outputs.  Block 10001 tx[1]
var LI01Hex4 = "01000000059daf0abe7a92618546a9dbcfd65869b6178c66ec21ccfda878c1175979cfd9ef000000004a493046022100c2f7f25be5de6ce88ac3c1a519514379e91f39b31ddff279a3db0b1a229b708b022100b29efbdbd9837cc6a6c7318aa4900ed7e4d65662c34d1622a2035a3a5534a99a01ffffffffd516330ebdf075948da56db13d22632a4fb941122df2884397dda45d451acefb0000000048473044022051243debe6d4f2b433bee0cee78c5c4073ead0e3bde54296dbed6176e128659c022044417bfe16f44eb7b6eb0cdf077b9ce972a332e15395c09ca5e4f602958d266101ffffffffe1f5aa33961227b3c344e57179417ce01b7ccd421117fe2336289b70489883f900000000484730440220593252bb992ce3c85baf28d6e3aa32065816271d2c822398fe7ee28a856bc943022066d429dd5025d3c86fd8fd8a58e183a844bd94aa312cefe00388f57c85b0ca3201ffffffffe207e83718129505e6a7484831442f668164ae659fddb82e9e5421a081fb90d50000000049483045022067cf27eb733e5bcae412a586b25a74417c237161a084167c2a0b439abfebdcb2022100efcc6baa6824b4c5205aa967e0b76d31abf89e738d4b6b014e788c9a8cccaf0c01ffffffffe23b8d9d80a9e9d977fab3c94dbe37befee63822443c3ec5ae5a713ede66c3940000000049483045022020f2eb35036666b1debe0d1d2e77a36d5d9c4e96c1dba23f5100f193dbf524790221008ce79bc1321fb4357c6daee818038d41544749127751726e46b2b320c8b565a201ffffffff0200ba1dd2050000001976a914366a27645806e817a6cd40bc869bdad92fe5509188ac40420f00000000001976a914ee8bd501094a7d5ca318da2506de35e1cb025ddc88ac00000000"

// Example 5 Sorts outputs only, based on output script.  Block 100998 tx[6]
var LI01Hex5 = "01000000011f636d0003f673b3aeea4971daef16b8eed784cf6e8019a5ae7da4985fbb06e5000000008a47304402205103941e2b11e746dfa817888d422f6e7f4d16dbbfb8ffa61d15ffb924a84b8802202fe861b0f23f17139d15a3374bfc6c7196d371f3d1a324e31cc0aadbba87e53c0141049e7e1b251a7e26cae9ee7553b278ef58ef3c28b4b20134d51b747d9b18b0a19b94b66cef320e2549dec0ea3d725cb4c742f368928b1fb74b4603e24a1e262c80ffffffff0240420f00000000001976a914bcfa0e27218a7c97257b351b03a9eac95c25a23988ac40420f00000000001976a9140c6a68f20bafc678164d171ee4f077adfa9b091688ac00000000"
