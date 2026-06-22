// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestIsSwiftSyncActive tests the isSwiftSyncActive function.
func TestIsSwiftSyncActive(t *testing.T) {
	swiftSyncData := &chaincfg.SwiftSyncData{
		Height: 100,
	}

	tests := []struct {
		name             string
		swiftSyncData    *chaincfg.SwiftSyncData
		swiftSyncEnabled bool
		height           int32
		want             bool
	}{
		{
			name:             "disabled - no swift sync data",
			swiftSyncData:    nil,
			swiftSyncEnabled: false,
			height:           50,
			want:             false,
		},
		{
			name:             "disabled - swift sync not enabled",
			swiftSyncData:    swiftSyncData,
			swiftSyncEnabled: false,
			height:           50,
			want:             false,
		},
		{
			name:             "active - within range",
			swiftSyncData:    swiftSyncData,
			swiftSyncEnabled: true,
			height:           50,
			want:             true,
		},
		{
			name:             "active - at boundary",
			swiftSyncData:    swiftSyncData,
			swiftSyncEnabled: true,
			height:           100,
			want:             true,
		},
		{
			name:             "inactive - height 0",
			swiftSyncData:    swiftSyncData,
			swiftSyncEnabled: true,
			height:           0,
			want:             false,
		},
		{
			name:             "inactive - height above range",
			swiftSyncData:    swiftSyncData,
			swiftSyncEnabled: true,
			height:           101,
			want:             false,
		},
		{
			name:             "active - at height 1",
			swiftSyncData:    swiftSyncData,
			swiftSyncEnabled: true,
			height:           1,
			want:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &BlockChain{
				swiftSync:        tt.swiftSyncData,
				swiftSyncEnabled: tt.swiftSyncEnabled,
			}

			got := b.isSwiftSyncActive(tt.height)
			if got != tt.want {
				t.Errorf("isSwiftSyncActive(%d) = %v, want %v",
					tt.height, got, tt.want)
			}
		})
	}
}

// TestIsHintsfileUnspendableOutput verifies the hintsfile unspendability
// filter that must be applied identically by both the hintsfile producer and
// consumer.
func TestIsHintsfileUnspendableOutput(t *testing.T) {
	standardTxOut := wire.NewTxOut(0, []byte{txscript.OP_TRUE})
	require.False(t, IsHintsfileUnspendableOutput(
		wire.MainNet, nil, 1, 0, standardTxOut,
	))

	opReturnTxOut := wire.NewTxOut(0, []byte{txscript.OP_RETURN})
	require.True(t, IsHintsfileUnspendableOutput(
		wire.MainNet, nil, 1, 0, opReturnTxOut,
	))

	oversizedTxOut := wire.NewTxOut(0, make([]byte, txscript.MaxScriptSize+1))
	require.True(t, IsHintsfileUnspendableOutput(
		wire.MainNet, nil, 1, 0, oversizedTxOut,
	))

	bip30Hash, err := chainhash.NewHashFromStr(
		"00000000000271a2dc26e7667f8419f2e15416dc6955e5a6c6cdf3f2574dd08e",
	)
	require.NoError(t, err)

	// The BIP30 coinbase output is unspendable only at txIndex 0 on mainnet.
	require.True(t, IsHintsfileUnspendableOutput(
		wire.MainNet, bip30Hash, 91722, 0, standardTxOut,
	))
	require.False(t, IsHintsfileUnspendableOutput(
		wire.MainNet, bip30Hash, 91722, 1, standardTxOut,
	))

	// Same hash on a non-mainnet network must NOT be treated as BIP30.
	require.False(t, IsHintsfileUnspendableOutput(
		wire.TestNet3, bip30Hash, 91722, 0, standardTxOut,
	))
}

// swiftSyncTestTx builds a transaction.  With no inputs it is a coinbase (a
// single null-outpoint input); otherwise it spends the given outpoints.  Each
// script becomes a 1000-value output.
func swiftSyncTestTx(inputs []wire.OutPoint, scripts ...[]byte) *wire.MsgTx {
	tx := wire.NewMsgTx(wire.TxVersion)
	if len(inputs) == 0 {
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{Index: 0xffffffff},
		})
	} else {
		for _, op := range inputs {
			tx.AddTxIn(&wire.TxIn{PreviousOutPoint: op})
		}
	}
	for _, script := range scripts {
		tx.AddTxOut(&wire.TxOut{Value: 1000, PkScript: script})
	}
	return tx
}

// swiftSyncTestBlock wraps the transactions in a block at the given height.
func swiftSyncTestBlock(height int32, txs ...*wire.MsgTx) *btcutil.Block {
	msg := &wire.MsgBlock{Header: wire.BlockHeader{Version: 1}}
	for _, tx := range txs {
		msg.AddTransaction(tx)
	}
	block := btcutil.NewBlock(msg)
	block.SetHeight(height)
	return block
}

// buildSwiftSyncHintsData serializes a hintsfile with the given per-block
// unspent spendable-index lists and parses it into a SwiftSyncData.
func buildSwiftSyncHintsData(t *testing.T,
	hintsByHeight [][]uint32) *chaincfg.SwiftSyncData {

	t.Helper()

	var blob bytes.Buffer
	blob.Write([]byte{0x55, 0x54, 0x58, 0x4f}) // magic "UTXO"
	blob.WriteByte(0x01)                       // version
	var heightLE [4]byte
	binary.LittleEndian.PutUint32(heightLE[:], uint32(len(hintsByHeight)))
	blob.Write(heightLE[:])

	for _, hints := range hintsByHeight {
		var payload bytes.Buffer
		var next uint64
		for _, idx := range hints {
			require.NoError(t, wire.WriteVarInt(&payload, 0, uint64(idx)-next))
			next = uint64(idx) + 1
		}
		require.NoError(t, wire.WriteVarInt(&blob, 0, uint64(payload.Len())))
		blob.Write(payload.Bytes())
	}

	data, err := chaincfg.ParseSwiftSyncHints(
		blob.Bytes(), int32(len(hintsByHeight)),
	)
	require.NoError(t, err)
	return data
}

// TestSwiftSyncAggregateCancels builds a synthetic three-block chain exercising
// outputs spent later, an output spent in the next block, unspent outputs, a
// coinbase output spent within range, and an OP_RETURN output, then verifies
// the aggregate nets to zero with correct hints and is non-zero when a hint is
// wrong.
func TestSwiftSyncAggregateCancels(t *testing.T) {
	opTrue := []byte{txscript.OP_TRUE}
	opReturn := []byte{txscript.OP_RETURN}

	// Block 1: coinbase cb1, whose output is spent in block 2.
	cb1 := swiftSyncTestTx(nil, opTrue)
	cb1Hash := cb1.TxHash()

	// Block 2: coinbase cb2 (unspent); tx A spends cb1:0 and creates A:0
	// (spent in block 3) and A:1 (spent in block 3).
	cb2 := swiftSyncTestTx(nil, opTrue)
	txA := swiftSyncTestTx(
		[]wire.OutPoint{{Hash: cb1Hash, Index: 0}}, opTrue, opTrue,
	)
	txAHash := txA.TxHash()

	// Block 3 (boundary): coinbase cb3 (unspent); tx B spends A:0 and creates
	// B:0 (unspent); tx C spends A:1 and creates an OP_RETURN (unspendable,
	// skipped) and a normal output (unspent).
	cb3 := swiftSyncTestTx(nil, opTrue)
	txB := swiftSyncTestTx([]wire.OutPoint{{Hash: txAHash, Index: 0}}, opTrue)
	txC := swiftSyncTestTx(
		[]wire.OutPoint{{Hash: txAHash, Index: 1}}, opReturn, opTrue,
	)

	blocks := []*btcutil.Block{
		swiftSyncTestBlock(1, cb1),
		swiftSyncTestBlock(2, cb2, txA),
		swiftSyncTestBlock(3, cb3, txB, txC),
	}

	var salt [32]byte
	salt[0] = 0x42

	run := func(hints [][]uint32) Agg512 {
		b := &BlockChain{
			chainParams:      &chaincfg.SimNetParams,
			swiftSync:        buildSwiftSyncHintsData(t, hints),
			swiftSyncEnabled: true,
			swiftSyncSalt:    salt,
		}
		cache := newUtxoCache(nil, 100*1024*1024)
		for _, block := range blocks {
			require.NoError(t, cache.swiftSyncConnectTransactions(
				b, block, true,
			))
		}
		return b.swiftSyncAgg
	}

	// Correct hints: spent outputs (cb1:0, A:0, A:1) are absent; every other
	// spendable output is unspent.  The aggregate must cancel to zero.
	good := run([][]uint32{{}, {0}, {0, 1, 2}})
	require.True(t, good.IsZero(), "correct hints did not cancel: %v", good)

	// Wrong hints: marking the spent output A:0 (block 2 index 1) as unspent
	// leaves its spend uncancelled, so the aggregate must be non-zero.
	bad := run([][]uint32{{}, {0, 1}, {0, 1, 2}})
	require.False(t, bad.IsZero(), "wrong hints must not cancel to zero")
}

// TestSwiftSyncConnectNoAgg verifies the resume path: with aggregate=false the
// unspent outputs are still added but the aggregate is left untouched, whereas
// the aggregating path updates it.
func TestSwiftSyncConnectNoAgg(t *testing.T) {
	opTrue := []byte{txscript.OP_TRUE}

	// A non-coinbase tx spending an output created outside the block, so the
	// aggregating path subtracts that outpoint and leaves a non-zero
	// aggregate.  Both outputs are unspent.
	cb := swiftSyncTestTx(nil, opTrue)
	spend := swiftSyncTestTx(
		[]wire.OutPoint{{Hash: chainhash.HashH([]byte("external")), Index: 0}},
		opTrue,
	)
	block := swiftSyncTestBlock(1, cb, spend)

	run := func(aggregate bool) Agg512 {
		b := &BlockChain{
			chainParams:      &chaincfg.SimNetParams,
			swiftSync:        buildSwiftSyncHintsData(t, [][]uint32{{0, 1}}),
			swiftSyncEnabled: true,
		}
		cache := newUtxoCache(nil, 100*1024*1024)
		require.NoError(t, cache.swiftSyncConnectTransactions(b, block, aggregate))
		return b.swiftSyncAgg
	}

	aggregated := run(true)
	require.False(t, aggregated.IsZero(),
		"aggregate=true should update the aggregate")

	notAggregated := run(false)
	require.True(t, notAggregated.IsZero(),
		"aggregate=false should leave the aggregate untouched")
}
