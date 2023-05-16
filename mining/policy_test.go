// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mining

import (
	"encoding/hex"
	"testing"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// newHashFromStr converts the passed big-endian hex string into a
// chainhash.Hash.  It only differs from the one available in chainhash in that
// it panics on an error since it will only (and must only) be called with
// hard-coded, and therefore known good, hashes.
func newHashFromStr(hexStr string) *chainhash.Hash {
	hash, err := chainhash.NewHashFromStr(hexStr)
	if err != nil {
		panic("invalid hash in source file: " + hexStr)
	}
	return hash
}

// hexToBytes converts the passed hex string into bytes and will panic if there
// is an error.  This is only provided for the hard-coded constants so errors in
// the source code can be detected. It will only (and must only) be called with
// hard-coded values.
func hexToBytes(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic("invalid hex in source file: " + s)
	}
	return b
}

// newUtxoViewpoint returns a new utxo view populated with outputs of the
// provided source transactions as if there were available at the respective
// block height specified in the heights slice.  The length of the source txns
// and source tx heights must match or it will panic.
func newUtxoViewpoint(sourceTxns []*wire.MsgTx, sourceTxHeights []int32) *blockchain.UtxoViewpoint {
	if len(sourceTxns) != len(sourceTxHeights) {
		panic("each transaction must have its block height specified")
	}

	view := blockchain.NewUtxoViewpoint()
	for i, tx := range sourceTxns {
		view.AddTxOuts(btcutil.NewTx(tx), sourceTxHeights[i])
	}
	return view
}

// TestCalcPriority ensures the priority calculations work as intended.
func TestCalcPriority(t *testing.T) {
	// commonSourceTx1 is a valid transaction used in the tests below as an
	// input to transactions that are having their priority calculated.
	//
	// From block 7 in main blockchain.
	// tx 0437cd7f8525ceed2324359c2d0ba26006d92d856a9c20fa0241106ee5a597c9
	commonSourceTx1 := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: wire.MaxPrevOutIndex,
			},
			SignatureScript: hexToBytes("04ffff001d0134"),
			Sequence:        0xffffffff,
		}},
		TxOut: []*wire.TxOut{{
			Value: 5000000000,
			PkScript: hexToBytes("410411db93e1dcdb8a016b49840f8c5" +
				"3bc1eb68a382e97b1482ecad7b148a6909a5cb2e0ead" +
				"dfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8" +
				"643f656b412a3ac"),
		}},
		LockTime: 0,
	}

	// commonRedeemTx1 is a valid transaction used in the tests below as the
	// transaction to calculate the priority for.
	//
	// It originally came from block 170 in main blockchain.
	commonRedeemTx1 := &wire.MsgTx{
		Version: 1,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Hash: *newHashFromStr("0437cd7f8525ceed232435" +
					"9c2d0ba26006d92d856a9c20fa0241106ee5" +
					"a597c9"),
				Index: 0,
			},
			SignatureScript: hexToBytes("47304402204e45e16932b8af" +
				"514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5f" +
				"b8cd410220181522ec8eca07de4860a4acdd12909d83" +
				"1cc56cbbac4622082221a8768d1d0901"),
			Sequence: 0xffffffff,
		}},
		TxOut: []*wire.TxOut{{
			Value: 1000000000,
			PkScript: hexToBytes("4104ae1a62fe09c5f51b13905f07f06" +
				"b99a2f7159b2225f374cd378d71302fa28414e7aab37" +
				"397f554a7df5f142c21c1b7303b8a0626f1baded5c72" +
				"a704f7e6cd84cac"),
		}, {
			Value: 4000000000,
			PkScript: hexToBytes("410411db93e1dcdb8a016b49840f8c5" +
				"3bc1eb68a382e97b1482ecad7b148a6909a5cb2e0ead" +
				"dfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8" +
				"643f656b412a3ac"),
		}},
		LockTime: 0,
	}

	tests := []struct {
		name       string                    // test description
		tx         *wire.MsgTx               // tx to calc priority for
		utxoView   *blockchain.UtxoViewpoint // inputs to tx
		nextHeight int32                     // height for priority calc
		want       float64                   // expected priority
	}{
		{
			name: "one height 7 input, prio tx height 169",
			tx:   commonRedeemTx1,
			utxoView: newUtxoViewpoint([]*wire.MsgTx{commonSourceTx1},
				[]int32{7}),
			nextHeight: 169,
			want:       5e9,
		},
		{
			name: "one height 100 input, prio tx height 169",
			tx:   commonRedeemTx1,
			utxoView: newUtxoViewpoint([]*wire.MsgTx{commonSourceTx1},
				[]int32{100}),
			nextHeight: 169,
			want:       2129629629.6296296,
		},
		{
			name: "one height 7 input, prio tx height 100000",
			tx:   commonRedeemTx1,
			utxoView: newUtxoViewpoint([]*wire.MsgTx{commonSourceTx1},
				[]int32{7}),
			nextHeight: 100000,
			want:       3086203703703.7036,
		},
		{
			name: "one height 100 input, prio tx height 100000",
			tx:   commonRedeemTx1,
			utxoView: newUtxoViewpoint([]*wire.MsgTx{commonSourceTx1},
				[]int32{100}),
			nextHeight: 100000,
			want:       3083333333333.3335,
		},
	}

	for i, test := range tests {
		got := CalcPriority(test.tx, test.utxoView, test.nextHeight)
		if got != test.want {
			t.Errorf("CalcPriority #%d (%q): unexpected priority "+
				"got %v want %v", i, test.name, got, test.want)
			continue
		}
	}
}
