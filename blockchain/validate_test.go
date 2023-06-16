// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcd/btcutil"
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

// TestCheckConnectBlockTemplate tests the CheckConnectBlockTemplate function to
// ensure it fails.
func TestCheckConnectBlockTemplate(t *testing.T) {
	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("checkconnectblocktemplate",
		&chaincfg.MainNetParams)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// Since we're not dealing with the real block chain, set the coinbase
	// maturity to 1.
	chain.TstSetCoinbaseMaturity(1)

	// Load up blocks such that there is a side chain.
	// (genesis block) -> 1 -> 2 -> 3 -> 4
	//                          \-> 3a
	testFiles := []string{
		"blk_0_to_4.dat.bz2",
		"blk_3A.dat.bz2",
	}

	var blocks []*btcutil.Block
	for _, file := range testFiles {
		blockTmp, err := loadBlocks(file)
		if err != nil {
			t.Fatalf("Error loading file: %v\n", err)
		}
		blocks = append(blocks, blockTmp...)
	}

	for i := 1; i <= 3; i++ {
		isMainChain, _, err := chain.ProcessBlock(blocks[i], BFNone)
		if err != nil {
			t.Fatalf("CheckConnectBlockTemplate: Received unexpected error "+
				"processing block %d: %v", i, err)
		}
		if !isMainChain {
			t.Fatalf("CheckConnectBlockTemplate: Expected block %d to connect "+
				"to main chain", i)
		}
	}

	// Block 3 should fail to connect since it's already inserted.
	err = chain.CheckConnectBlockTemplate(blocks[3])
	if err == nil {
		t.Fatal("CheckConnectBlockTemplate: Did not received expected error " +
			"on block 3")
	}

	// Block 4 should connect successfully to tip of chain.
	err = chain.CheckConnectBlockTemplate(blocks[4])
	if err != nil {
		t.Fatalf("CheckConnectBlockTemplate: Received unexpected error on "+
			"block 4: %v", err)
	}

	// Block 3a should fail to connect since does not build on chain tip.
	err = chain.CheckConnectBlockTemplate(blocks[5])
	if err == nil {
		t.Fatal("CheckConnectBlockTemplate: Did not received expected error " +
			"on block 3a")
	}

	// Block 4 should connect even if proof of work is invalid.
	invalidPowBlock := *blocks[4].MsgBlock()
	invalidPowBlock.Header.Nonce++
	err = chain.CheckConnectBlockTemplate(btcutil.NewBlock(&invalidPowBlock))
	if err != nil {
		t.Fatalf("CheckConnectBlockTemplate: Received unexpected error on "+
			"block 4 with bad nonce: %v", err)
	}

	// Invalid block building on chain tip should fail to connect.
	invalidBlock := *blocks[4].MsgBlock()
	invalidBlock.Header.Bits--
	err = chain.CheckConnectBlockTemplate(btcutil.NewBlock(&invalidBlock))
	if err == nil {
		t.Fatal("CheckConnectBlockTemplate: Did not received expected error " +
			"on block 4 with invalid difficulty bits")
	}
}

// TestCheckBlockSanity tests the CheckBlockSanity function to ensure it works
// as expected.
func TestCheckBlockSanity(t *testing.T) {
	powLimit := chaincfg.MainNetParams.PowLimit
	block := btcutil.NewBlock(&Block100000)
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

// Block100000 defines block 100,000 of the block chain.  It is used to
// test Block operations.
var Block100000 = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version: 1,
		PrevBlock: chainhash.Hash([32]byte{ // Make go vet happy.
			0x50, 0x12, 0x01, 0x19, 0x17, 0x2a, 0x61, 0x04,
			0x21, 0xa6, 0xc3, 0x01, 0x1d, 0xd3, 0x30, 0xd9,
			0xdf, 0x07, 0xb6, 0x36, 0x16, 0xc2, 0xcc, 0x1f,
			0x1c, 0xd0, 0x02, 0x00, 0x00, 0x00, 0x00, 0x00,
		}), // 000000000002d01c1fccc21636b607dfd930d31d01c3a62104612a1719011250
		MerkleRoot: chainhash.Hash([32]byte{ // Make go vet happy.
			0x66, 0x57, 0xa9, 0x25, 0x2a, 0xac, 0xd5, 0xc0,
			0xb2, 0x94, 0x09, 0x96, 0xec, 0xff, 0x95, 0x22,
			0x28, 0xc3, 0x06, 0x7c, 0xc3, 0x8d, 0x48, 0x85,
			0xef, 0xb5, 0xa4, 0xac, 0x42, 0x47, 0xe9, 0xf3,
		}), // f3e94742aca4b5ef85488dc37c06c3282295ffec960994b2c0d5ac2a25a95766
		Timestamp: time.Unix(1293623863, 0), // 2010-12-29 11:57:43 +0000 UTC
		Bits:      0x1b04864c,               // 453281356
		Nonce:     0x10572b0f,               // 274148111
	},
	Transactions: []*wire.MsgTx{
		{
			Version: 1,
			TxIn: []*wire.TxIn{
				{
					PreviousOutPoint: wire.OutPoint{
						Hash:  chainhash.Hash{},
						Index: 0xffffffff,
					},
					SignatureScript: []byte{
						0x04, 0x4c, 0x86, 0x04, 0x1b, 0x02, 0x06, 0x02,
					},
					Sequence: 0xffffffff,
				},
			},
			TxOut: []*wire.TxOut{
				{
					Value: 0x12a05f200, // 5000000000
					PkScript: []byte{
						0x41, // OP_DATA_65
						0x04, 0x1b, 0x0e, 0x8c, 0x25, 0x67, 0xc1, 0x25,
						0x36, 0xaa, 0x13, 0x35, 0x7b, 0x79, 0xa0, 0x73,
						0xdc, 0x44, 0x44, 0xac, 0xb8, 0x3c, 0x4e, 0xc7,
						0xa0, 0xe2, 0xf9, 0x9d, 0xd7, 0x45, 0x75, 0x16,
						0xc5, 0x81, 0x72, 0x42, 0xda, 0x79, 0x69, 0x24,
						0xca, 0x4e, 0x99, 0x94, 0x7d, 0x08, 0x7f, 0xed,
						0xf9, 0xce, 0x46, 0x7c, 0xb9, 0xf7, 0xc6, 0x28,
						0x70, 0x78, 0xf8, 0x01, 0xdf, 0x27, 0x6f, 0xdf,
						0x84, // 65-byte signature
						0xac, // OP_CHECKSIG
					},
				},
			},
			LockTime: 0,
		},
		{
			Version: 1,
			TxIn: []*wire.TxIn{
				{
					PreviousOutPoint: wire.OutPoint{
						Hash: chainhash.Hash([32]byte{ // Make go vet happy.
							0x03, 0x2e, 0x38, 0xe9, 0xc0, 0xa8, 0x4c, 0x60,
							0x46, 0xd6, 0x87, 0xd1, 0x05, 0x56, 0xdc, 0xac,
							0xc4, 0x1d, 0x27, 0x5e, 0xc5, 0x5f, 0xc0, 0x07,
							0x79, 0xac, 0x88, 0xfd, 0xf3, 0x57, 0xa1, 0x87,
						}), // 87a157f3fd88ac7907c05fc55e271dc4acdc5605d187d646604ca8c0e9382e03
						Index: 0,
					},
					SignatureScript: []byte{
						0x49, // OP_DATA_73
						0x30, 0x46, 0x02, 0x21, 0x00, 0xc3, 0x52, 0xd3,
						0xdd, 0x99, 0x3a, 0x98, 0x1b, 0xeb, 0xa4, 0xa6,
						0x3a, 0xd1, 0x5c, 0x20, 0x92, 0x75, 0xca, 0x94,
						0x70, 0xab, 0xfc, 0xd5, 0x7d, 0xa9, 0x3b, 0x58,
						0xe4, 0xeb, 0x5d, 0xce, 0x82, 0x02, 0x21, 0x00,
						0x84, 0x07, 0x92, 0xbc, 0x1f, 0x45, 0x60, 0x62,
						0x81, 0x9f, 0x15, 0xd3, 0x3e, 0xe7, 0x05, 0x5c,
						0xf7, 0xb5, 0xee, 0x1a, 0xf1, 0xeb, 0xcc, 0x60,
						0x28, 0xd9, 0xcd, 0xb1, 0xc3, 0xaf, 0x77, 0x48,
						0x01, // 73-byte signature
						0x41, // OP_DATA_65
						0x04, 0xf4, 0x6d, 0xb5, 0xe9, 0xd6, 0x1a, 0x9d,
						0xc2, 0x7b, 0x8d, 0x64, 0xad, 0x23, 0xe7, 0x38,
						0x3a, 0x4e, 0x6c, 0xa1, 0x64, 0x59, 0x3c, 0x25,
						0x27, 0xc0, 0x38, 0xc0, 0x85, 0x7e, 0xb6, 0x7e,
						0xe8, 0xe8, 0x25, 0xdc, 0xa6, 0x50, 0x46, 0xb8,
						0x2c, 0x93, 0x31, 0x58, 0x6c, 0x82, 0xe0, 0xfd,
						0x1f, 0x63, 0x3f, 0x25, 0xf8, 0x7c, 0x16, 0x1b,
						0xc6, 0xf8, 0xa6, 0x30, 0x12, 0x1d, 0xf2, 0xb3,
						0xd3, // 65-byte pubkey
					},
					Sequence: 0xffffffff,
				},
			},
			TxOut: []*wire.TxOut{
				{
					Value: 0x2123e300, // 556000000
					PkScript: []byte{
						0x76, // OP_DUP
						0xa9, // OP_HASH160
						0x14, // OP_DATA_20
						0xc3, 0x98, 0xef, 0xa9, 0xc3, 0x92, 0xba, 0x60,
						0x13, 0xc5, 0xe0, 0x4e, 0xe7, 0x29, 0x75, 0x5e,
						0xf7, 0xf5, 0x8b, 0x32,
						0x88, // OP_EQUALVERIFY
						0xac, // OP_CHECKSIG
					},
				},
				{
					Value: 0x108e20f00, // 4444000000
					PkScript: []byte{
						0x76, // OP_DUP
						0xa9, // OP_HASH160
						0x14, // OP_DATA_20
						0x94, 0x8c, 0x76, 0x5a, 0x69, 0x14, 0xd4, 0x3f,
						0x2a, 0x7a, 0xc1, 0x77, 0xda, 0x2c, 0x2f, 0x6b,
						0x52, 0xde, 0x3d, 0x7c,
						0x88, // OP_EQUALVERIFY
						0xac, // OP_CHECKSIG
					},
				},
			},
			LockTime: 0,
		},
		{
			Version: 1,
			TxIn: []*wire.TxIn{
				{
					PreviousOutPoint: wire.OutPoint{
						Hash: chainhash.Hash([32]byte{ // Make go vet happy.
							0xc3, 0x3e, 0xbf, 0xf2, 0xa7, 0x09, 0xf1, 0x3d,
							0x9f, 0x9a, 0x75, 0x69, 0xab, 0x16, 0xa3, 0x27,
							0x86, 0xaf, 0x7d, 0x7e, 0x2d, 0xe0, 0x92, 0x65,
							0xe4, 0x1c, 0x61, 0xd0, 0x78, 0x29, 0x4e, 0xcf,
						}), // cf4e2978d0611ce46592e02d7e7daf8627a316ab69759a9f3df109a7f2bf3ec3
						Index: 1,
					},
					SignatureScript: []byte{
						0x47, // OP_DATA_71
						0x30, 0x44, 0x02, 0x20, 0x03, 0x2d, 0x30, 0xdf,
						0x5e, 0xe6, 0xf5, 0x7f, 0xa4, 0x6c, 0xdd, 0xb5,
						0xeb, 0x8d, 0x0d, 0x9f, 0xe8, 0xde, 0x6b, 0x34,
						0x2d, 0x27, 0x94, 0x2a, 0xe9, 0x0a, 0x32, 0x31,
						0xe0, 0xba, 0x33, 0x3e, 0x02, 0x20, 0x3d, 0xee,
						0xe8, 0x06, 0x0f, 0xdc, 0x70, 0x23, 0x0a, 0x7f,
						0x5b, 0x4a, 0xd7, 0xd7, 0xbc, 0x3e, 0x62, 0x8c,
						0xbe, 0x21, 0x9a, 0x88, 0x6b, 0x84, 0x26, 0x9e,
						0xae, 0xb8, 0x1e, 0x26, 0xb4, 0xfe, 0x01,
						0x41, // OP_DATA_65
						0x04, 0xae, 0x31, 0xc3, 0x1b, 0xf9, 0x12, 0x78,
						0xd9, 0x9b, 0x83, 0x77, 0xa3, 0x5b, 0xbc, 0xe5,
						0xb2, 0x7d, 0x9f, 0xff, 0x15, 0x45, 0x68, 0x39,
						0xe9, 0x19, 0x45, 0x3f, 0xc7, 0xb3, 0xf7, 0x21,
						0xf0, 0xba, 0x40, 0x3f, 0xf9, 0x6c, 0x9d, 0xee,
						0xb6, 0x80, 0xe5, 0xfd, 0x34, 0x1c, 0x0f, 0xc3,
						0xa7, 0xb9, 0x0d, 0xa4, 0x63, 0x1e, 0xe3, 0x95,
						0x60, 0x63, 0x9d, 0xb4, 0x62, 0xe9, 0xcb, 0x85,
						0x0f, // 65-byte pubkey
					},
					Sequence: 0xffffffff,
				},
			},
			TxOut: []*wire.TxOut{
				{
					Value: 0xf4240, // 1000000
					PkScript: []byte{
						0x76, // OP_DUP
						0xa9, // OP_HASH160
						0x14, // OP_DATA_20
						0xb0, 0xdc, 0xbf, 0x97, 0xea, 0xbf, 0x44, 0x04,
						0xe3, 0x1d, 0x95, 0x24, 0x77, 0xce, 0x82, 0x2d,
						0xad, 0xbe, 0x7e, 0x10,
						0x88, // OP_EQUALVERIFY
						0xac, // OP_CHECKSIG
					},
				},
				{
					Value: 0x11d260c0, // 299000000
					PkScript: []byte{
						0x76, // OP_DUP
						0xa9, // OP_HASH160
						0x14, // OP_DATA_20
						0x6b, 0x12, 0x81, 0xee, 0xc2, 0x5a, 0xb4, 0xe1,
						0xe0, 0x79, 0x3f, 0xf4, 0xe0, 0x8a, 0xb1, 0xab,
						0xb3, 0x40, 0x9c, 0xd9,
						0x88, // OP_EQUALVERIFY
						0xac, // OP_CHECKSIG
					},
				},
			},
			LockTime: 0,
		},
		{
			Version: 1,
			TxIn: []*wire.TxIn{
				{
					PreviousOutPoint: wire.OutPoint{
						Hash: chainhash.Hash([32]byte{ // Make go vet happy.
							0x0b, 0x60, 0x72, 0xb3, 0x86, 0xd4, 0xa7, 0x73,
							0x23, 0x52, 0x37, 0xf6, 0x4c, 0x11, 0x26, 0xac,
							0x3b, 0x24, 0x0c, 0x84, 0xb9, 0x17, 0xa3, 0x90,
							0x9b, 0xa1, 0xc4, 0x3d, 0xed, 0x5f, 0x51, 0xf4,
						}), // f4515fed3dc4a19b90a317b9840c243bac26114cf637522373a7d486b372600b
						Index: 0,
					},
					SignatureScript: []byte{
						0x49, // OP_DATA_73
						0x30, 0x46, 0x02, 0x21, 0x00, 0xbb, 0x1a, 0xd2,
						0x6d, 0xf9, 0x30, 0xa5, 0x1c, 0xce, 0x11, 0x0c,
						0xf4, 0x4f, 0x7a, 0x48, 0xc3, 0xc5, 0x61, 0xfd,
						0x97, 0x75, 0x00, 0xb1, 0xae, 0x5d, 0x6b, 0x6f,
						0xd1, 0x3d, 0x0b, 0x3f, 0x4a, 0x02, 0x21, 0x00,
						0xc5, 0xb4, 0x29, 0x51, 0xac, 0xed, 0xff, 0x14,
						0xab, 0xba, 0x27, 0x36, 0xfd, 0x57, 0x4b, 0xdb,
						0x46, 0x5f, 0x3e, 0x6f, 0x8d, 0xa1, 0x2e, 0x2c,
						0x53, 0x03, 0x95, 0x4a, 0xca, 0x7f, 0x78, 0xf3,
						0x01, // 73-byte signature
						0x41, // OP_DATA_65
						0x04, 0xa7, 0x13, 0x5b, 0xfe, 0x82, 0x4c, 0x97,
						0xec, 0xc0, 0x1e, 0xc7, 0xd7, 0xe3, 0x36, 0x18,
						0x5c, 0x81, 0xe2, 0xaa, 0x2c, 0x41, 0xab, 0x17,
						0x54, 0x07, 0xc0, 0x94, 0x84, 0xce, 0x96, 0x94,
						0xb4, 0x49, 0x53, 0xfc, 0xb7, 0x51, 0x20, 0x65,
						0x64, 0xa9, 0xc2, 0x4d, 0xd0, 0x94, 0xd4, 0x2f,
						0xdb, 0xfd, 0xd5, 0xaa, 0xd3, 0xe0, 0x63, 0xce,
						0x6a, 0xf4, 0xcf, 0xaa, 0xea, 0x4e, 0xa1, 0x4f,
						0xbb, // 65-byte pubkey
					},
					Sequence: 0xffffffff,
				},
			},
			TxOut: []*wire.TxOut{
				{
					Value: 0xf4240, // 1000000
					PkScript: []byte{
						0x76, // OP_DUP
						0xa9, // OP_HASH160
						0x14, // OP_DATA_20
						0x39, 0xaa, 0x3d, 0x56, 0x9e, 0x06, 0xa1, 0xd7,
						0x92, 0x6d, 0xc4, 0xbe, 0x11, 0x93, 0xc9, 0x9b,
						0xf2, 0xeb, 0x9e, 0xe0,
						0x88, // OP_EQUALVERIFY
						0xac, // OP_CHECKSIG
					},
				},
			},
			LockTime: 0,
		},
	},
}
