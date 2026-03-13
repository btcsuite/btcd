//go:build rpctest
// +build rpctest

package integration

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestV3TransactionPolicy tests that v3 transactions are accepted by the
// mempool and can be mined into blocks.
func TestV3TransactionPolicy(t *testing.T) {
	t.Parallel()

	// Setup the test harness, and initialize the harness with mature
	// coinbase outputs for spending.
	btcdCfg := []string{"--rejectnonstd", "--debuglevel=debug"}
	r, err := rpctest.New(&chaincfg.SimNetParams, nil, btcdCfg, "")
	require.NoError(t, err)

	require.NoError(t, r.SetUp(true, 100))
	t.Cleanup(func() {
		require.NoError(t, r.TearDown())
	})

	// Make sure that normal v1 transactions are still accepted.
	t.Run("v1 transaction accepted", func(t *testing.T) {
		testVersionedTransaction(t, r, 1, true)
	})

	// Make sure that v2 transactions are still accepted.
	t.Run("v2 transaction accepted", func(t *testing.T) {
		testVersionedTransaction(t, r, 2, true)
	})

	// Ensure that v3 transactions are accepted.
	t.Run("v3 transaction accepted", func(t *testing.T) {
		testVersionedTransaction(t, r, 3, true)
	})

	// Ensure that v4 transactions are rejected.
	t.Run("v4 transaction rejected", func(t *testing.T) {
		testVersionedTransaction(t, r, 4, false)
	})

	// Test that v3 transactions can be mined.
	t.Run("v3 transaction mined in block", func(t *testing.T) {
		testV3TransactionMining(t, r)
	})
}

// testVersionedTransaction creates a transaction with the specified version
// and tests whether it's accepted by the mempool.
func testVersionedTransaction(t *testing.T, r *rpctest.Harness, version int32,
	shouldBeAccepted bool) {

	addr, err := r.NewAddress()
	require.NoError(t, err)

	script, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	output := &wire.TxOut{
		PkScript: script,
		Value:    50000000,
	}

	tx, err := r.CreateTransaction(
		[]*wire.TxOut{output}, 10, true, rpctest.WithTxVersion(version),
	)
	require.NoError(t, err)
	require.Equal(t, version, tx.Version)

	// Test mempool acceptance without submission.
	results, err := r.Client.TestMempoolAccept([]*wire.MsgTx{tx}, 0)
	require.NoError(t, err)
	require.Len(t, results, 1)

	if shouldBeAccepted {
		require.True(
			t, results[0].Allowed,
			"v%d transaction should be accepted, but got: %s",
			version, results[0].RejectReason,
		)

		// Try to actually submit the transaction.
		txHash, err := r.Client.SendRawTransaction(tx, true)
		require.NoError(t, err)
		require.NotNil(t, txHash)

		// Verify it's in the mempool.
		mempool, err := r.Client.GetRawMempool()
		require.NoError(t, err)
		require.Contains(t, mempool, txHash)

		return
	}

	require.False(
		t, results[0].Allowed,
		"v%d transaction should be rejected", version,
	)
	require.Contains(
		t, results[0].RejectReason, "version",
		"rejection reason should mention version",
	)
}

// testV3TransactionMining tests that v3 transactions can be successfully mined
// into blocks.
func testV3TransactionMining(t *testing.T, r *rpctest.Harness) {
	// Create a new random address to send to.
	addr, err := r.NewAddress()
	require.NoError(t, err)
	script, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	output := &wire.TxOut{
		PkScript: script,
		Value:    100000000,
	}

	// Create a v3 transaction using the WithTxVersion option.
	v3Tx, err := r.CreateTransaction(
		[]*wire.TxOut{output}, 10, true,
		rpctest.WithTxVersion(3),
	)
	require.NoError(t, err)
	require.Equal(t, int32(3), v3Tx.Version)

	// Submit the v3 transaction to the mempool, this should work np.
	v3TxHash, err := r.Client.SendRawTransaction(v3Tx, true)
	require.NoError(t, err)
	require.NotNil(t, v3TxHash)

	// Verify it's in the mempool.
	mempool, err := r.Client.GetRawMempool()
	require.NoError(t, err)
	require.Contains(t, mempool, v3TxHash)

	blocks, err := r.Client.Generate(1)
	require.NoError(t, err)
	require.Len(t, blocks, 1)

	block, err := r.Client.GetBlock(blocks[0])
	require.NoError(t, err)

	// Verify the v3 transaction is included in the block.
	found := false
	for _, tx := range block.Transactions {
		txHash := tx.TxHash()
		if txHash.IsEqual(v3TxHash) {
			found = true

			// Verify it's still a v3 transaction in the block.
			require.Equal(t, int32(3), tx.Version)
			break
		}
	}
	require.True(
		t, found, "v3 transaction should be included in the mined "+
			"block",
	)

	// Verify the transaction is no longer in the mempool.
	mempool, err = r.Client.GetRawMempool()
	require.NoError(t, err)
	require.NotContains(t, mempool, v3TxHash)
}

// TestV3TransactionRPCSubmission tests v3 transaction submission via RPC.
func TestV3TransactionRPCSubmission(t *testing.T) {
	t.Parallel()

	// Setup the test harness.
	btcdCfg := []string{"--rejectnonstd", "--debuglevel=debug"}
	r, err := rpctest.New(&chaincfg.SimNetParams, nil, btcdCfg, "")
	require.NoError(t, err)

	// Initialize the harness with mature coinbase outputs for spending.
	require.NoError(t, r.SetUp(true, 100))
	t.Cleanup(func() {
		require.NoError(t, r.TearDown())
	})

	t.Run("v3 transaction via SendOutputs", func(t *testing.T) {
		addr, err := r.NewAddress()
		require.NoError(t, err)
		script, err := txscript.PayToAddrScript(addr)
		require.NoError(t, err)

		output := &wire.TxOut{
			PkScript: script,
			Value:    25000000,
		}

		// Send outputs with v3 transaction version.
		txHash, err := r.SendOutputs(
			[]*wire.TxOut{output}, 10, rpctest.WithTxVersion(3),
		)
		require.NoError(t, err)
		require.NotNil(t, txHash)

		// Get the transaction from mempool and verify its version.
		tx, err := r.Client.GetRawTransaction(txHash)
		require.NoError(t, err)
		require.Equal(t, int32(3), tx.MsgTx().Version)

		// Mine a block to confirm, then verify transaction is no
		// longer in mempool (meaning it was mined).
		blocks, err := r.Client.Generate(1)
		require.NoError(t, err)
		require.Len(t, blocks, 1)

		mempool, err := r.Client.GetRawMempool()
		require.NoError(t, err)
		require.NotContains(t, mempool, txHash)
	})
}

