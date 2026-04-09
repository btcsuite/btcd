// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bloom_test

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/btcutil/bloom"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

func TestMerkleBlock3(t *testing.T) {
	blockStr := "0100000079cda856b143d9db2c1caff01d1aecc8630d30625d10e8b" +
		"4b8b0000000000000b50cc069d6a3e33e3ff84a5c41d9d3febe7c770fdc" +
		"c96b2c3ff60abe184f196367291b4d4c86041b8fa45d630101000000010" +
		"00000000000000000000000000000000000000000000000000000000000" +
		"0000ffffffff08044c86041b020a02ffffffff0100f2052a01000000434" +
		"104ecd3229b0571c3be876feaac0442a9f13c5a572742927af1dc623353" +
		"ecf8c202225f64868137a18cdd85cbbb4c74fbccfd4f49639cf1bdc94a5" +
		"672bb15ad5d4cac00000000"
	blockBytes, err := hex.DecodeString(blockStr)
	if err != nil {
		t.Errorf("TestMerkleBlock3 DecodeString failed: %v", err)
		return
	}
	blk, err := btcutil.NewBlockFromBytes(blockBytes)
	if err != nil {
		t.Errorf("TestMerkleBlock3 NewBlockFromBytes failed: %v", err)
		return
	}

	f := bloom.NewFilter(10, 0, 0.000001, wire.BloomUpdateAll)

	inputStr := "63194f18be0af63f2c6bc9dc0f777cbefed3d9415c4af83f3ee3a3d669c00cb5"
	hash, err := chainhash.NewHashFromStr(inputStr)
	if err != nil {
		t.Errorf("TestMerkleBlock3 NewHashFromStr failed: %v", err)
		return
	}

	f.AddHash(hash)

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
	err = mBlock.BtcEncode(got, wire.ProtocolVersion, wire.LatestEncoding)
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

func testMerkleProofBlock(numTx int) *btcutil.Block {
	txs := make([]*wire.MsgTx, 0, numTx)
	utilTxs := make([]*btcutil.Tx, 0, numTx)
	for i := 0; i < numTx; i++ {
		tx := &wire.MsgTx{
			Version: 1,
			TxIn: []*wire.TxIn{{
				PreviousOutPoint: wire.OutPoint{Index: wire.MaxPrevOutIndex},
				SignatureScript:  []byte{byte(i + 1)},
				Sequence:         wire.MaxTxInSequenceNum,
			}},
			TxOut: []*wire.TxOut{{
				Value:    int64(i + 1),
				PkScript: []byte{txscript.OP_TRUE},
			}},
		}
		txs = append(txs, tx)
		utilTxs = append(utilTxs, btcutil.NewTx(tx))
	}

	block := btcutil.NewBlock(&wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:   1,
			Timestamp: time.Unix(1700000000, 0),
			Bits:      0x1d00ffff,
		},
		Transactions: txs,
	})
	block.MsgBlock().Header.MerkleRoot = blockchain.CalcMerkleRoot(utilTxs, false)
	return block
}

func TestNewMerkleBlockFromTxHashes(t *testing.T) {
	t.Parallel()

	block := testMerkleProofBlock(5)
	txHashes := []*chainhash.Hash{
		block.Transactions()[3].Hash(),
		block.Transactions()[1].Hash(),
	}

	merkleBlock, matchedIndices, err := bloom.NewMerkleBlockFromTxHashes(block, txHashes)
	if err != nil {
		t.Fatalf("NewMerkleBlockFromTxHashes: unexpected error: %v", err)
	}
	if len(matchedIndices) != 2 || matchedIndices[0] != 1 || matchedIndices[1] != 3 {
		t.Fatalf("NewMerkleBlockFromTxHashes: unexpected matched indices %v", matchedIndices)
	}

	matches, err := merkleBlock.ExtractMatches()
	if err != nil {
		t.Fatalf("ExtractMatches: unexpected error: %v", err)
	}
	wantMatches := []chainhash.Hash{
		*block.Transactions()[1].Hash(),
		*block.Transactions()[3].Hash(),
	}
	if len(matches) != len(wantMatches) {
		t.Fatalf("ExtractMatches: unexpected match count %d", len(matches))
	}
	for i := range matches {
		if matches[i] != wantMatches[i] {
			t.Fatalf("ExtractMatches: unexpected matches %v", matches)
		}
	}
}

func TestNewMerkleBlockFromTxHashesMissingTx(t *testing.T) {
	t.Parallel()

	block := testMerkleProofBlock(3)
	missingHash := chainhash.DoubleHashH([]byte("missing"))

	_, _, err := bloom.NewMerkleBlockFromTxHashes(block, []*chainhash.Hash{&missingHash})
	if err == nil {
		t.Fatal("NewMerkleBlockFromTxHashes: expected error")
	}
}

func TestNewMerkleBlockFromTxHashesRejectsDuplicateTx(t *testing.T) {
	t.Parallel()

	block := testMerkleProofBlock(2)
	txHash := block.Transactions()[0].Hash()

	_, _, err := bloom.NewMerkleBlockFromTxHashes(block, []*chainhash.Hash{txHash, txHash})
	if err == nil {
		t.Fatal("NewMerkleBlockFromTxHashes: expected duplicate transaction error")
	}
}
