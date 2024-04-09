//go:build rpctest
// +build rpctest

package integration

import (
	"testing"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestGetTxSpendingPrevOut checks that `GetTxSpendingPrevOut` behaves as
// expected.
// - an error is returned when invalid params are used.
// - orphan tx is rejected.
// - fee rate above the max is rejected.
// - a mixed of both allowed and rejected can be returned in the same response.
func TestGetTxSpendingPrevOut(t *testing.T) {
	t.Parallel()

	// Boilerplate codetestDir to make a pruned node.
	btcdCfg := []string{"--rejectnonstd", "--debuglevel=debug"}
	r, err := rpctest.New(&chaincfg.SimNetParams, nil, btcdCfg, "")
	require.NoError(t, err)

	// Setup the node.
	require.NoError(t, r.SetUp(true, 100))
	t.Cleanup(func() {
		require.NoError(t, r.TearDown())
	})

	// Create a tx and testing outpoints.
	tx := createTxInMempool(t, r)
	opInMempool := tx.TxIn[0].PreviousOutPoint
	opNotInMempool := wire.OutPoint{
		Hash:  tx.TxHash(),
		Index: 0,
	}

	testCases := []struct {
		name           string
		outpoints      []wire.OutPoint
		expectedErr    error
		expectedResult []*btcjson.GetTxSpendingPrevOutResult
	}{
		{
			// When no outpoints are provided, the method should
			// return an error.
			name:           "empty outpoints",
			expectedErr:    rpcclient.ErrInvalidParam,
			expectedResult: nil,
		},
		{
			// When there are outpoints provided, check the
			// expceted results are returned.
			name: "outpoints",
			outpoints: []wire.OutPoint{
				opInMempool, opNotInMempool,
			},
			expectedErr: nil,
			expectedResult: []*btcjson.GetTxSpendingPrevOutResult{
				{
					Txid:         opInMempool.Hash.String(),
					Vout:         opInMempool.Index,
					SpendingTxid: tx.TxHash().String(),
				},
				{
					Txid: opNotInMempool.Hash.String(),
					Vout: opNotInMempool.Index,
				},
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			require := require.New(t)

			results, err := r.Client.GetTxSpendingPrevOut(
				tc.outpoints,
			)

			require.ErrorIs(err, tc.expectedErr)
			require.Len(results, len(tc.expectedResult))

			// Check each item is returned as expected.
			for i, r := range results {
				e := tc.expectedResult[i]

				require.Equal(e.Txid, r.Txid)
				require.Equal(e.Vout, r.Vout)
				require.Equal(e.SpendingTxid, r.SpendingTxid)
			}
		})
	}
}

// createTxInMempool creates a tx and puts it in the mempool.
func createTxInMempool(t *testing.T, r *rpctest.Harness) *wire.MsgTx {
	// Create a fresh output for usage within the test below.
	const outputValue = btcutil.SatoshiPerBitcoin
	outputKey, testOutput, testPkScript, err := makeTestOutput(
		r, t, outputValue,
	)
	require.NoError(t, err)

	// Create a new transaction with a lock-time past the current known
	// MTP.
	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *testOutput,
	})

	// Fetch a fresh address from the harness, we'll use this address to
	// send funds back into the Harness.
	addr, err := r.NewAddress()
	require.NoError(t, err)

	addrScript, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	tx.AddTxOut(&wire.TxOut{
		PkScript: addrScript,
		Value:    outputValue - 1000,
	})

	sigScript, err := txscript.SignatureScript(
		tx, 0, testPkScript, txscript.SigHashAll, outputKey, true,
	)
	require.NoError(t, err)
	tx.TxIn[0].SignatureScript = sigScript

	// Send the tx.
	_, err = r.Client.SendRawTransaction(tx, true)
	require.NoError(t, err)

	return tx
}
