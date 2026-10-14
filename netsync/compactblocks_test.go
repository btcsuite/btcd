// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"bytes"
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// makeCompactBlockTestBlock creates a test block with
// numTxns transactions.
func makeCompactBlockTestBlock(numTxns int) *btcutil.Block {
	msgBlock := &wire.MsgBlock{
		Header: wire.BlockHeader{
			Version: 1,
			Bits:    0x1d00ffff,
			Nonce:   1,
		},
		Transactions: make([]*wire.MsgTx, numTxns),
	}

	txns := make([]*btcutil.Tx, numTxns)
	for i := 0; i < numTxns; i++ {
		msgTx := makeCompactBlockTestTx(byte(i))
		msgBlock.Transactions[i] = msgTx
		txns[i] = btcutil.NewTx(msgTx)
	}

	msgBlock.Header.MerkleRoot = blockchain.CalcMerkleRoot(txns, false)
	return btcutil.NewBlock(msgBlock)
}

// makeCompactBlockTestTx creates a simple test transaction
// identified by tag.
func makeCompactBlockTestTx(tag byte) *wire.MsgTx {
	prevHash := chainhash.Hash{tag}
	msgTx := wire.NewMsgTx(1)
	msgTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  prevHash,
			Index: uint32(tag),
		},
		SignatureScript: []byte{tag},
		Sequence:        0xffffffff,
	})
	msgTx.AddTxOut(&wire.TxOut{
		Value:    int64(tag) + 1,
		PkScript: []byte{0x51, tag},
	})

	return msgTx
}

// makeCompactBlockTestBlockWithWitness creates a test block where
// non-coinbase transactions are witness.
func makeCompactBlockTestBlockWithWitness(numTxns int) *btcutil.Block {
	msgBlock := &wire.MsgBlock{
		Header: wire.BlockHeader{
			Version: 1,
			Bits:    0x1d00ffff,
			Nonce:   2,
		},
		Transactions: make([]*wire.MsgTx, numTxns),
	}

	txns := make([]*btcutil.Tx, numTxns)
	for i := 0; i < numTxns; i++ {
		var msgTx *wire.MsgTx
		if i == 0 {
			// Coinbase — no witness.
			msgTx = makeCompactBlockTestTx(byte(i))
		} else {
			msgTx = makeCompactBlockTestTxWithWitness(byte(i))
		}
		msgBlock.Transactions[i] = msgTx
		txns[i] = btcutil.NewTx(msgTx)
	}

	msgBlock.Header.MerkleRoot = blockchain.CalcMerkleRoot(txns, false)
	return btcutil.NewBlock(msgBlock)
}

// makeCompactBlockTestTxWithWitness creates a test witness transaction
// identified by tag.
func makeCompactBlockTestTxWithWitness(tag byte) *wire.MsgTx {
	prevHash := chainhash.Hash{tag}
	msgTx := wire.NewMsgTx(1)
	msgTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  prevHash,
			Index: uint32(tag),
		},
		SignatureScript: nil,
		Sequence:        0xffffffff,
		Witness:         wire.TxWitness{[]byte{tag, tag + 1}},
	})
	msgTx.AddTxOut(&wire.TxOut{
		Value:    int64(tag) + 1,
		PkScript: []byte{0x51, tag},
	})

	return msgTx
}

// TestBuildCompactBlockWireRoundTrip serializes a cmpctblock message using the
// Bitcoin P2P wire encoding and deserializes it again, verifying that the
// header, nonce, short IDs, and prefilled transactions survive the round trip.
func TestBuildCompactBlockWireRoundTrip(t *testing.T) {
	block := makeCompactBlockTestBlock(4)
	const nonce = uint64(0xdeadbeef)

	original, err := BuildCompactBlock(block, nonce, []uint32{0}, false)
	require.NoError(t, err)

	// Encode
	var buf bytes.Buffer
	err = original.BtcEncode(&buf, wire.ShortIdsBlocksVersion,
		wire.BaseEncoding)
	require.NoError(t, err)

	// Decode
	decoded := &wire.MsgCmpctBlock{}
	err = decoded.BtcDecode(&buf, wire.ShortIdsBlocksVersion,
		wire.BaseEncoding)
	require.NoError(t, err)

	// Header must match
	origHash := original.Header.BlockHash()
	decHash := decoded.Header.BlockHash()
	require.Equal(t, origHash, decHash)

	// Nonce must match
	require.Equal(t, original.Nonce, decoded.Nonce)

	// Short ID count and values must match
	require.Equal(t, original.ShortIDs, decoded.ShortIDs)

	// Prefilled transaction count and indexes must match
	require.Len(t, decoded.PrefilledTxns, len(original.PrefilledTxns))
	for i := range original.PrefilledTxns {
		require.Equal(t, original.PrefilledTxns[i].Index, decoded.PrefilledTxns[i].Index)
		require.Equal(t, original.PrefilledTxns[i].Tx.TxHash(), decoded.PrefilledTxns[i].Tx.TxHash())
	}
}

// TestBuildCompactBlock verifies that BuildCompactBlock correctly
// partitions transactions into short IDs and prefilled entries.
func TestBuildCompactBlock(t *testing.T) {
	block := makeCompactBlockTestBlock(3)

	msg, err := BuildCompactBlock(block, 99, []uint32{2, 0}, false)
	require.NoError(t, err)

	require.Len(t, msg.ShortIDs, 1)
	require.Len(t, msg.PrefilledTxns, 2)

	gotIndexes := []uint32{
		msg.PrefilledTxns[0].Index,
		msg.PrefilledTxns[1].Index,
	}
	require.Equal(t, []uint32{0, 2}, gotIndexes)
}

// TestCompactBlockReconstructorCompleteFromMempool verifies that a reconstructor
// is complete when the mempool satisfies all short IDs.
func TestCompactBlockReconstructorCompleteFromMempool(t *testing.T) {
	block := makeCompactBlockTestBlock(3)
	msg, err := BuildCompactBlock(block, 99, nil, false)
	require.NoError(t, err)

	recon, err := NewCompactBlockReconstructor(
		msg, block.Transactions()[1:], false,
	)
	require.NoError(t, err)

	require.True(t, recon.IsComplete(),
		"reconstructor should be complete, missing %v", recon.MissingIndexes())

	reconstructed, err := recon.Block()
	require.NoError(t, err)
	require.Equal(t, block.Hash(), reconstructed.Hash())
}

// TestCompactBlockReconstructorWithBlockTxn verifies that missing transactions
// can be supplied via a blocktxn response.
func TestCompactBlockReconstructorWithBlockTxn(t *testing.T) {
	block := makeCompactBlockTestBlock(3)
	msg, err := BuildCompactBlock(block, 99, nil, false)
	require.NoError(t, err)

	recon, err := NewCompactBlockReconstructor(
		msg, block.Transactions()[1:2], false,
	)
	require.NoError(t, err)

	wantMissing := []uint32{2}
	require.Equal(t, wantMissing, recon.MissingIndexes())

	req := recon.GetBlockTxnRequest()
	require.Equal(t, wantMissing, req.Indexes)

	resp := wire.NewMsgBlockTxn(&req.BlockHash)
	err = resp.AddTransaction(block.MsgBlock().Transactions[2])
	require.NoError(t, err)

	reconstructed, err := recon.ProvideBlockTxn(resp)
	require.NoError(t, err)
	require.Equal(t, block.Hash(), reconstructed.Hash())
}

// TestCompactBlockReconstructorBadMerkleRoot verifies that providing a wrong
// transaction returns ErrCompactBlockBadMerkleRoot.
func TestCompactBlockReconstructorBadMerkleRoot(t *testing.T) {
	block := makeCompactBlockTestBlock(3)
	msg, err := BuildCompactBlock(block, 99, nil, false)
	require.NoError(t, err)

	recon, err := NewCompactBlockReconstructor(
		msg, block.Transactions()[1:2], false,
	)
	require.NoError(t, err)

	req := recon.GetBlockTxnRequest()
	resp := wire.NewMsgBlockTxn(&req.BlockHash)
	resp.Transactions = []*wire.MsgTx{makeCompactBlockTestTx(42)}

	_, err = recon.ProvideBlockTxn(resp)
	require.ErrorIs(t, err, ErrCompactBlockBadMerkleRoot)
}

// TestShortIDNonceDependence verifies that short IDs are deterministic
// for the same nonce and differ across nonces.
func TestShortIDNonceDependence(t *testing.T) {
	block := makeCompactBlockTestBlock(3)

	msg1, err := BuildCompactBlock(block, 1, nil, false)
	require.NoError(t, err)

	msg2, err := BuildCompactBlock(block, 2, nil, false)
	require.NoError(t, err)

	// Same nonce must produce identical short IDs.
	msg1b, err := BuildCompactBlock(block, 1, nil, false)
	require.NoError(t, err)
	require.Equal(t, msg1.ShortIDs, msg1b.ShortIDs)

	// Different nonces must produce different short IDs.
	require.NotEqual(t, msg1.ShortIDs, msg2.ShortIDs)
}

// TestShortIDSize verifies that every short ID in the compact block has
// the correct wire size.
func TestShortIDSize(t *testing.T) {
	block := makeCompactBlockTestBlock(5)
	msg, err := BuildCompactBlock(block, 42, nil, false)
	require.NoError(t, err)

	for i, id := range msg.ShortIDs {
		require.Len(t, id, wire.ShortIDSize)
		// Verify the same ID is returned by CompactBlockShortIDFromHash.
		tx := block.Transactions()[i+1] // skip coinbase
		got, err := CompactBlockShortID(&block.MsgBlock().Header,
			42, tx, false)
		require.NoError(t, err)
		require.Equal(t, id, got)
	}
}

// TestBuildCompactBlockAllPrefilled verifies that when every transaction is
// marked as prefilled the resulting message has no short IDs.
func TestBuildCompactBlockAllPrefilled(t *testing.T) {
	const numTxns = 4
	block := makeCompactBlockTestBlock(numTxns)

	allIndexes := make([]uint32, numTxns)
	for i := range allIndexes {
		allIndexes[i] = uint32(i)
	}

	msg, err := BuildCompactBlock(block, 0, allIndexes, false)
	require.NoError(t, err)

	require.Empty(t, msg.ShortIDs)
	require.Len(t, msg.PrefilledTxns, numTxns)

	// The block must be reconstructable from the prefilled txns alone
	// (empty mempool).
	recon, err := NewCompactBlockReconstructor(msg, nil, false)
	require.NoError(t, err)
	require.True(t, recon.IsComplete(),
		"reconstructor should be complete, missing %v", recon.MissingIndexes())
	reconstructed, err := recon.Block()
	require.NoError(t, err)
	require.Equal(t, block.Hash(), reconstructed.Hash())
}

// TestCompactBlockReconstructorEmptyMempool verifies that when no mempool
// transactions are provided every non-prefilled transaction appears in
// MissingIndexes and they can be supplied via a blocktxn response.
func TestCompactBlockReconstructorEmptyMempool(t *testing.T) {
	block := makeCompactBlockTestBlock(4)
	msg, err := BuildCompactBlock(block, 7, nil, false)
	require.NoError(t, err)

	// No mempool — all non-prefilled txns should be missing.
	recon, err := NewCompactBlockReconstructor(msg, nil, false)
	require.NoError(t, err)
	require.False(t, recon.IsComplete(),
		"reconstructor should not be complete with empty mempool")

	// Indexes 1, 2, 3 should be missing (0 is the prefilled coinbase).
	wantMissing := []uint32{1, 2, 3}
	require.Equal(t, wantMissing, recon.MissingIndexes())

	// Respond with all missing transactions.
	req := recon.GetBlockTxnRequest()
	resp := wire.NewMsgBlockTxn(&req.BlockHash)
	for _, idx := range req.Indexes {
		err := resp.AddTransaction(block.MsgBlock().Transactions[idx])
		require.NoError(t, err)
	}

	reconstructed, err := recon.ProvideBlockTxn(resp)
	require.NoError(t, err)
	require.Equal(t, block.Hash(), reconstructed.Hash())
}

// TestCompactBlockReconstructorWitness verifies that version 2 compact blocks
// use the witness transaction ID (wtxid) for short ID computation rather than
// the plain txid, as required by BIP 152.
func TestCompactBlockReconstructorWitness(t *testing.T) {
	block := makeCompactBlockTestBlockWithWitness(3)

	// Build with witness=true (v2).
	msg, err := BuildCompactBlock(block, 55, nil, true)
	require.NoError(t, err)

	// Building with witness=false must produce DIFFERENT short IDs for txns
	// that have witnesses, because txid ≠ wtxid for segwit transactions.
	msgNoWitness, err := BuildCompactBlock(block, 55, nil, false)
	require.NoError(t, err)
	require.NotEqual(t, msg.ShortIDs, msgNoWitness.ShortIDs)

	// Reconstruct using witness=true and the block's own transactions as
	// the mempool source.
	recon, err := NewCompactBlockReconstructor(
		msg, block.Transactions()[1:], true,
	)
	require.NoError(t, err)
	require.True(t, recon.IsComplete(),
		"reconstructor should be complete, missing %v", recon.MissingIndexes())

	reconstructed, err := recon.Block()
	require.NoError(t, err)
	require.Equal(t, block.Hash(), reconstructed.Hash())
}

// TestCompactBlockVector is a test vector inspired in rust-bitcoin
// unit test suite for BIP-152. The test uses a fixed raw block hexadecimal
// data and compare it with a raw compact block hexadecimal data after build the
// compact block
func TestCompactBlockVector(t *testing.T) {
	const rawBlockHex = "000000206c750a364035aefd5f81508a08769975116d9195312ee4520dceac39e1fdc62c4dc67473b8e354358c1e610afeaff7410858bd45df43e2940f8a62bd3d5e3ac943c2975cffff7f200000000002020000000001010000000000000000000000000000000000000000000000000000000000000000ffffffff04016b0101ffffffff020006062a0100000001510000000000000000266a24aa21a9ed4a3d9f3343dafcc0d6f6d4310f2ee5ce273ed34edca6c75db3a73e7f368734200120000000000000000000000000000000000000000000000000000000000000000000000000020000000001021fc20ba2bd745507b8e00679e3b362558f9457db374ca28ffa5243f4c23a4d5f00000000171600147c9dea14ffbcaec4b575e03f05ceb7a81cd3fcbffdffffff915d689be87b43337f42e26033df59807b768223368f189a023d0242d837768900000000171600147c9dea14ffbcaec4b575e03f05ceb7a81cd3fcbffdffffff0200cdf5050000000017a9146803c72d9154a6a20f404bed6d3dcee07986235a8700e1f5050000000017a9144e6a4c7cb5b5562904843bdf816342f4db9f5797870247304402205e9bf6e70eb0e4b495bf483fd8e6e02da64900f290ef8aaa64bb32600d973c450220670896f5d0e5f33473e5f399ab680cc1d25c2d2afd15abd722f04978f28be887012103e4e4d9312b2261af508b367d8ba9be4f01b61d6d6e78bec499845b4f410bcf2702473044022045ac80596a6ac9c8c572f94708709adaf106677221122e08daf8b9741a04f66a022003ccd52a3b78f8fd08058fc04fc0cffa5f4c196c84eae9e37e2a85babe731b57012103e4e4d9312b2261af508b367d8ba9be4f01b61d6d6e78bec499845b4f410bcf276a000000"
	const rawCompactHex = "000000206c750a364035aefd5f81508a08769975116d9195312ee4520dceac39e1fdc62c4dc67473b8e354358c1e610afeaff7410858bd45df43e2940f8a62bd3d5e3ac943c2975cffff7f2000000000a4df3c3744da89fa010a6979e971450100020000000001010000000000000000000000000000000000000000000000000000000000000000ffffffff04016b0101ffffffff020006062a0100000001510000000000000000266a24aa21a9ed4a3d9f3343dafcc0d6f6d4310f2ee5ce273ed34edca6c75db3a73e7f368734200120000000000000000000000000000000000000000000000000000000000000000000000000"

	rawBlock, err := hex.DecodeString(rawBlockHex)
	require.NoError(t, err)
	rawCompact, err := hex.DecodeString(rawCompactHex)
	require.NoError(t, err)

	// Decode the raw block
	msgBlock := &wire.MsgBlock{}
	err = msgBlock.Deserialize(bytes.NewReader(rawBlock))
	require.NoError(t, err)
	block := btcutil.NewBlock(msgBlock)

	const nonce = uint64(18_053_200_567_810_711_460)

	// Build the compact block
	compact, err := BuildCompactBlock(block, nonce, nil, true)
	require.NoError(t, err)

	// Serialize the compact block with witness encoding so that the witness
	// data in the prefilled coinbase is included (matches the reference vector)
	var buf bytes.Buffer
	err = compact.BtcEncode(&buf, wire.ShortIdsBlocksVersion, wire.WitnessEncoding)
	require.NoError(t, err)

	// Compare with the expected compact block encoding
	require.Equal(t, rawCompact, buf.Bytes())
}

// TestShortIDCollisionInMempool verifies that when two mempool transactions
// produce the same short ID (collision), neither is used to fill the block
// slot and the index is reported as missing.  This tests the tolerance
// requirement in BIP 152 blocktxn: "nodes MUST NOT be penalized for such
// collisions, wherever they appear."
func TestShortIDCollisionInMempool(t *testing.T) {
	// Build a 3-tx block (coinbase prefilled, tx[1] and tx[2] as short IDs).
	block := makeCompactBlockTestBlock(3)
	const nonce = uint64(99)

	msg, err := BuildCompactBlock(block, nonce, nil, false)
	require.NoError(t, err)

	tx1 := block.Transactions()[1]
	tx2 := block.Transactions()[2]

	// Mempool has tx[1] twice and tx[2] once.  The duplicate tx[1] should
	// cancel both entries (collision), leaving only tx[2] resolved.
	mempoolTxns := []*btcutil.Tx{tx1, tx1, tx2}

	recon, err := NewCompactBlockReconstructor(msg, mempoolTxns, false)
	require.NoError(t, err)

	wantMissing := []uint32{1}
	require.Equal(t, wantMissing, recon.MissingIndexes())

	// The node must be able to recover by requesting the missing tx.
	req := recon.GetBlockTxnRequest()
	resp := wire.NewMsgBlockTxn(&req.BlockHash)
	err = resp.AddTransaction(block.MsgBlock().Transactions[1])
	require.NoError(t, err)

	reconstructed, err := recon.ProvideBlockTxn(resp)
	require.NoError(t, err)
	require.Equal(t, block.Hash(), reconstructed.Hash())
}
