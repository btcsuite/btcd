// Copyright (c) 2025 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestValidateSubmittedPackage tests package topology and limit validation.
func TestValidateSubmittedPackage(t *testing.T) {
	t.Parallel()

	// Create some test transactions.
	createTx := func(version int32) *btcutil.Tx {
		mtx := wire.NewMsgTx(version)
		mtx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{Index: 0},
		})
		mtx.AddTxOut(&wire.TxOut{Value: 1000, PkScript: []byte{0x51}})
		return btcutil.NewTx(mtx)
	}

	t.Run("empty package rejected", func(t *testing.T) {
		err := validateSubmittedPackage([]*btcutil.Tx{})
		require.Error(t, err)
		require.Contains(t, err.Error(), "must contain at least one")
	})

	t.Run("single transaction accepted", func(t *testing.T) {
		tx := createTx(2)
		err := validateSubmittedPackage([]*btcutil.Tx{tx})
		require.NoError(t, err)
	})

	t.Run("topologically sorted package accepted", func(t *testing.T) {
		parent := createTx(3)

		// Child spends parent's output.
		childTx := wire.NewMsgTx(3)
		childTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  *parent.Hash(),
				Index: 0,
			},
		})
		childTx.AddTxOut(&wire.TxOut{Value: 500, PkScript: []byte{0x51}})
		child := btcutil.NewTx(childTx)

		// Parent before child (correct order).
		err := validateSubmittedPackage([]*btcutil.Tx{parent, child})
		require.NoError(t, err)
	})

	t.Run("not topologically sorted rejected", func(t *testing.T) {
		parent := createTx(3)

		// Child spends parent's output.
		childTx := wire.NewMsgTx(3)
		childTx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{
				Hash:  *parent.Hash(),
				Index: 0,
			},
		})
		childTx.AddTxOut(&wire.TxOut{Value: 500, PkScript: []byte{0x51}})
		child := btcutil.NewTx(childTx)

		// Child before parent (wrong order).
		err := validateSubmittedPackage([]*btcutil.Tx{child, parent})
		require.Error(t, err)
		require.Contains(t, err.Error(), "not topologically sorted")
	})

	t.Run("package too large rejected", func(t *testing.T) {
		// Create 26 transactions (exceeds MaxPackageCount=25).
		txs := make([]*btcutil.Tx, 26)
		for i := 0; i < 26; i++ {
			txs[i] = createTx(2)
		}

		err := validateSubmittedPackage(txs)
		require.Error(t, err)
		require.Contains(t, err.Error(), "max 25 allowed")
	})

	t.Run("package too heavy rejected", func(t *testing.T) {
		// Construct a single transaction with a very large output to exceed
		// the 404k weight limit (101k vbytes).
		largeScript := bytes.Repeat([]byte{0x6a}, MaxPackageWeight/4+1)

		mtx := wire.NewMsgTx(2)
		mtx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: wire.OutPoint{Index: 0},
		})
		mtx.AddTxOut(&wire.TxOut{
			Value:    1000,
			PkScript: largeScript,
		})

		tx := btcutil.NewTx(mtx)
		err := validateSubmittedPackage([]*btcutil.Tx{tx})
		require.Error(t, err)
		require.Contains(t, err.Error(), "package weight")
	})
}
