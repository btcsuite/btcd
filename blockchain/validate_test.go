// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"encoding/hex"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/lbryio/lbcd/chaincfg"
	"github.com/lbryio/lbcd/chaincfg/chainhash"
	"github.com/lbryio/lbcd/wire"
	btcutil "github.com/lbryio/lbcutil"
)

// TestSequenceLocksActive tests the SequenceLockActive function to ensure it
// works as expected in all possible combinations/scenarios.
func TestSequenceLocksActive(t *testing.T) {
	seqLock := func(h int32, s int64) *SequenceLock {
		return &SequenceLock{
			Seconds:     s,
			BlockHeight: h,
		}
	}

	tests := []struct {
		seqLock     *SequenceLock
		blockHeight int32
		mtp         time.Time

		want bool
	}{
		// Block based sequence lock with equal block height.
		{seqLock: seqLock(1000, -1), blockHeight: 1001, mtp: time.Unix(9, 0), want: true},

		// Time based sequence lock with mtp past the absolute time.
		{seqLock: seqLock(-1, 30), blockHeight: 2, mtp: time.Unix(31, 0), want: true},

		// Block based sequence lock with current height below seq lock block height.
		{seqLock: seqLock(1000, -1), blockHeight: 90, mtp: time.Unix(9, 0), want: false},

		// Time based sequence lock with current time before lock time.
		{seqLock: seqLock(-1, 30), blockHeight: 2, mtp: time.Unix(29, 0), want: false},

		// Block based sequence lock at the same height, so shouldn't yet be active.
		{seqLock: seqLock(1000, -1), blockHeight: 1000, mtp: time.Unix(9, 0), want: false},

		// Time based sequence lock with current time equal to lock time, so shouldn't yet be active.
		{seqLock: seqLock(-1, 30), blockHeight: 2, mtp: time.Unix(30, 0), want: false},
	}

	t.Logf("Running %d sequence locks tests", len(tests))
	for i, test := range tests {
		got := SequenceLockActive(test.seqLock,
			test.blockHeight, test.mtp)
		if got != test.want {
			t.Fatalf("SequenceLockActive #%d got %v want %v", i,
				got, test.want)
		}
	}
}

// TestCheckBlockSanity tests the CheckBlockSanity function to ensure it works
// as expected.
func TestCheckBlockSanity(t *testing.T) {
	powLimit := chaincfg.MainNetParams.PowLimit
	block := GetBlock100000()
	timeSource := NewMedianTime()
	err := CheckBlockSanity(block, powLimit, timeSource)
	if err != nil {
		t.Errorf("CheckBlockSanity: %v", err)
	}

	// Ensure a block that has a timestamp with a precision higher than one
	// second fails.
	timestamp := block.MsgBlock().Header.Timestamp
	block.MsgBlock().Header.Timestamp = timestamp.Add(time.Nanosecond)
	err = CheckBlockSanity(block, powLimit, timeSource)
	if err == nil {
		t.Errorf("CheckBlockSanity: error is nil when it shouldn't be")
	}
}

// TestCheckSerializedHeight tests the checkSerializedHeight function with
// various serialized heights and also does negative tests to ensure errors
// and handled properly.
func TestCheckSerializedHeight(t *testing.T) {
	// Create an empty coinbase template to be used in the tests below.
	coinbaseOutpoint := wire.NewOutPoint(&chainhash.Hash{}, math.MaxUint32)
	coinbaseTx := wire.NewMsgTx(1)
	coinbaseTx.AddTxIn(wire.NewTxIn(coinbaseOutpoint, nil, nil))

	// Expected rule errors.
	missingHeightError := RuleError{
		ErrorCode: ErrMissingCoinbaseHeight,
	}
	badHeightError := RuleError{
		ErrorCode: ErrBadCoinbaseHeight,
	}

	tests := []struct {
		sigScript  []byte // Serialized data
		wantHeight int32  // Expected height
		err        error  // Expected error type
	}{
		// No serialized height length.
		{[]byte{}, 0, missingHeightError},
		// Serialized height length with no height bytes.
		{[]byte{0x02}, 0, missingHeightError},
		// Serialized height length with too few height bytes.
		{[]byte{0x02, 0x4a}, 0, missingHeightError},
		// Serialized height that needs 2 bytes to encode.
		{[]byte{0x02, 0x4a, 0x52}, 21066, nil},
		// Serialized height that needs 2 bytes to encode, but backwards
		// endianness.
		{[]byte{0x02, 0x4a, 0x52}, 19026, badHeightError},
		// Serialized height that needs 3 bytes to encode.
		{[]byte{0x03, 0x40, 0x0d, 0x03}, 200000, nil},
		// Serialized height that needs 3 bytes to encode, but backwards
		// endianness.
		{[]byte{0x03, 0x40, 0x0d, 0x03}, 1074594560, badHeightError},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		msgTx := coinbaseTx.Copy()
		msgTx.TxIn[0].SignatureScript = test.sigScript
		tx := btcutil.NewTx(msgTx)

		err := checkSerializedHeight(tx, test.wantHeight)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("checkSerializedHeight #%d wrong error type "+
				"got: %v <%T>, want: %T", i, err, err, test.err)
			continue
		}

		if rerr, ok := err.(RuleError); ok {
			trerr := test.err.(RuleError)
			if rerr.ErrorCode != trerr.ErrorCode {
				t.Errorf("checkSerializedHeight #%d wrong "+
					"error code got: %v, want: %v", i,
					rerr.ErrorCode, trerr.ErrorCode)
				continue
			}
		}
	}
}

var block100000Hex = "0000002024cbdc8644ee3983e66b003a0733891c069ca74c114c034c7b3e2e7ad7a12cd67e95e0555c0e056f6f2af538268ff9d21b420e529750d08eacb25c40f1322936637109b8a051157604c1c163cd39237687f6244b4e6d2b3a94e9d816babaecbb10c56058c811041b2b9c43000701000000010000000000000000000000000000000000000000000000000000000000000000ffffffff2003a086010410c56058081011314abf0100000d2f6e6f64655374726174756d2f000000000180354a6e0a0000001976a914b5e74e7cc9e1f480a6599895c92aab1401f870f188ac000000000100000002f1b75decc2c1c59c2178852822de412f574ad9796b65ac9092a79630d8109aaf000000006a47304402202f78ed3bf8dcadb6c17e289cd06e9c96b02c6f23aa1f446a4a7896a31cfd1e4702202862261e2eb59475ac91092c620b3cac5a831372bafc446d5ee450866040b532012103db4f3785354d84311fab7624c52784a61e3046d8f364463d327bdd96281b5b90feffffff987ee6b4bf95548d01e443683261dd0ffdcb2eb335b2f7119df0d41b60756b92010000006a47304402200c422c7560b6418d45443138bb957ec44eb293a639f4b2235a622205ca6cac370220759f15d5dc2543fd1aef80104c93427fcb025847bf895669025d1d82c62fbf6801210201864b998db5436990a0029fc3fb153c09e8c2689214b91c8ed68784995c8da0feffffff022bccfedd000000001976a914738f16132942a01d00cb9699bd058c4925aada3288ac1f4d030c000000001976a914c4292e757f5ff6a27c2f0a87d3a4aea5e46c275a88ac9f86010001000000015fbb26ad6d818186032baeef4d3f475dfe165c6da2d93411b8ec5f9f523cf1a4000000006a4730440220356999ad5a9f6f09b676f17dd4a2617a0af234722d68a50edc3768c983c0623d022056b4e5531608aeb0205fde9c44f741158da3bba1f4c3788c9fe79d36d43ea355012103509893a2a7c765d49ac9ff70126cb1af54871d70baba2c7e39ec9b4096289a9bfeffffff02389332fa080000001976a914f85e054405fbcedc2341cf8bf41ea989090587a288acf9321a41000000001976a914e85e90c048fdfbe1c2117a7132856ff4b39b470188ac9f86010001000000013508189b9bb61ac2aa536b905447b58f6c58c17cdef305240f566caa689d760a010000006a4730440220669a2b86e5abe58bae54829d3c271669540a9ad965c2fb79e8cc1fb609c0b60002202f958403d979138075cb53d8cb5ff6bb14e18d66dfdb6701c7d43a8ceeed0fa80121029935a602205a3fb72446064f3bc3a55ab9cd2e3459bf2ffdf80a48ab029d4b42feffffff02523c2f13000000001976a914c5b2ae398496f0f9ceaf81b83c28a27ddc890e3588ac211958f2000000001976a914ac65f1d16e5a2af37408b5d65406000a7ea143ca88ac9f8601000100000001bdd724322c555a21d5eb62d4aadbdc051663bcd4ec03f8d9005992f299783c21000000006a47304402205448141a2a788f73310025123bd24f5bee01dd8f48f18d7abc62d7c26465008902207ab46e6ddf6ba416decf3fbb97b6946a1428ea0a7c25a55cab47c47110d8e9ce0121029d6ff3b1235f2a08560b23dd4a08b14cc415b544801b885365736ea8ab1d3470feffffff029d104ccf000000001976a914999d5b0e3d5efcf601c711127b91841afbf5c37a88ace5c5a07f070000001976a9144aade372298eb387da9b6ac69d215a213e822f3f88ac9f86010001000000011658304d4ce796cd450228a10fdf647c6ea42295c9f5e1663df11481af1c884d010000006b483045022100a35d5d3ccde85b41559047d976ae6210b8e6ba5653c53aae1adc90048de0761002200d6bd6ebc6d73f97855f435c6fd595009ee71d23bb34870ab83ad53f67eeb22b012102d2f681ebfd1a570416d602986a47ca4254d8dedf2935b3f8c2ba55dcee8e98f4feffffff025ee913e6020000001976a91468076c9380d3c6b468ad1d6109c36770fb181e8f88acb165394f000000001976a9147ae970e81b3657cbb59df26517e372165807be0088ac9f86010001000000018f285109f78524a88ff328a4f94de2ac03224c50984b11c68adda192e8f78efa010000006b483045022100d77f2ac32dd6a3015f02f7115a13f617c57952fc5d53a33a87dc3fc00ffe1864022006455f74cff864b10424e445c530a59243f86d309dc92c5010ec5709e38471ab012102fdac7335b41edcd2846fc7e2166bb48312ee583ed6ff70fb5c27bcb2addaad86feffffff028b7a6d5c000000001976a914c65106d2e7ea4ec6aa8aa30ba4d11cfd1143123388ac5934c228000000001976a914d1c4d190b07edb972b91f33c36c6568b80358dd488ac9f860100"

// GetBlock100000 defines block 100,000 of the block chain.  It is used to
// test Block operations.
func GetBlock100000() *btcutil.Block {
	var block100000Bytes, _ = hex.DecodeString(block100000Hex)
	var results, _ = btcutil.NewBlockFromBytes(block100000Bytes)
	return results
}
