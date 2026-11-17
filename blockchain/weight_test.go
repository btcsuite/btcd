// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

// TestGetSigOpCostPropagatesP2SHSigOpError verifies that GetSigOpCost
// propagates the RuleError returned by CountP2SHSigOps instead of returning
// (0, nil) when a referenced previous output is missing.
func TestGetSigOpCostPropagatesP2SHSigOpError(t *testing.T) {
	prevOut := wire.OutPoint{
		Hash:  chainhash.Hash{0xaa},
		Index: 0,
	}

	msgTx := wire.NewMsgTx(wire.TxVersion)
	msgTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: prevOut,
	})
	msgTx.AddTxOut(&wire.TxOut{
		Value:    1000,
		PkScript: []byte{txscript.OP_CHECKSIG},
	})

	tx := btcutil.NewTx(msgTx)

	// Positive control: when the referenced output exists in the view,
	// GetSigOpCost succeeds and returns the scaled legacy sigop cost.
	populatedView := NewUtxoViewpoint()
	populatedView.Entries()[prevOut] = NewUtxoEntry(
		&wire.TxOut{
			Value:    2000,
			PkScript: []byte{txscript.OP_TRUE},
		},
		100,
		false,
	)

	cost, err := GetSigOpCost(tx, false, populatedView, true, false)
	require.NoError(t, err)
	require.Equal(t, WitnessScaleFactor, cost)

	// Regression check: pass bip16=true and segWit=false with an empty
	// view to isolate the CountP2SHSigOps error path and verify that
	// ErrMissingTxOut is propagated.
	emptyView := NewUtxoViewpoint()
	cost, err = GetSigOpCost(tx, false, emptyView, true, false)

	var ruleErr RuleError
	require.ErrorAs(t, err, &ruleErr)
	require.Equal(t, ErrMissingTxOut, ruleErr.ErrorCode)
	require.Zero(t, cost)
}
