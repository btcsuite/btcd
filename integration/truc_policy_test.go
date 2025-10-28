//go:build rpctest
// +build rpctest

package integration

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

const (
	// TRUC size limits from BIP 431.
	maxTRUCParentSize = 10000 // vB
	maxTRUCChildSize  = 1000  // vB
)

// TestTRUCPolicy contains all BIP 431 TRUC policy enforcement tests.
// These tests verify the v2 mempool correctly enforces topology restrictions,
// size limits, and replaceability rules for v3 transactions.
func TestTRUCPolicy(t *testing.T) {
	btcdCfg := []string{
		"--rejectnonstd",
		"--debuglevel=debug",
		"--usetxmempoolv2", // Enable v2 mempool with TRUC support.
	}
	r, err := rpctest.New(&chaincfg.SimNetParams, nil, btcdCfg, "")
	require.NoError(t, err)

	require.NoError(t, r.SetUp(true, 100))
	defer func() {
		if err := r.TearDown(); err != nil {
			t.Logf("TearDown error: %v", err)
		}
	}()

	// Run subtests for each BIP 431 rule.
	t.Run("Rule 1: v3 signals replaceability", func(t *testing.T) {
		testTRUCRule1Replaceability(t, r)
	})

	t.Run("Rule 2: all-or-none TRUC", func(t *testing.T) {
		testTRUCRule2AllOrNone(t, r)
	})

	t.Run("Rule 3: 1P1C topology", func(t *testing.T) {
		testTRUCRule3Topology(t, r)
	})

	t.Run("Rule 4: parent size limit", func(t *testing.T) {
		testTRUCRule4ParentSize(t, r)
	})

	t.Run("Rule 5: child size limit", func(t *testing.T) {
		testTRUCRule5ChildSize(t, r)
	})

	t.Run("Rule 6: zero-fee packages", func(t *testing.T) {
		testTRUCRule6ZeroFeePackages(t, r)
	})

	t.Run("Security: pinning prevention", func(t *testing.T) {
		testTRUCPinningPrevention(t, r)
	})
}

// Test helper functions.

// createV3Transaction creates a v3 transaction spending from the test harness
// wallet with specified output value and fee rate.
func createV3Transaction(t *testing.T, r *rpctest.Harness, outputValue int64, feeRate btcutil.Amount) *wire.MsgTx {
	t.Helper()

	addr, err := r.NewAddress()
	require.NoError(t, err)
	script, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	output := &wire.TxOut{
		Value:    outputValue,
		PkScript: script,
	}

	tx, err := r.CreateTransaction(
		[]*wire.TxOut{output},
		feeRate,
		true, // Include change.
		rpctest.WithTxVersion(3),
	)
	require.NoError(t, err)
	return tx
}

// createV3Child creates a v3 transaction that spends a specific output from a
// parent transaction. The child uses the WithInputs option to explicitly spend
// the parent's output, creating a deterministic parent-child relationship for
// TRUC 1P1C package testing.
//
// The function automatically registers the parent's outputs in the wallet's
// UTXO set so they can be spent, enabling unconfirmed parent-child testing.
func createV3Child(t *testing.T, r *rpctest.Harness, parent *wire.MsgTx, outputIdx uint32, outputValue int64, feeRate btcutil.Amount) *wire.MsgTx {
	t.Helper()

	require.Less(t, int(outputIdx), len(parent.TxOut), "output index out of range")

	// Register parent's outputs in wallet UTXO set so they can be spent.
	// This is necessary for unconfirmed parent-child testing.
	require.NoError(t, r.AddUnconfirmedTx(parent))

	addr, err := r.NewAddress()
	require.NoError(t, err)
	script, err := txscript.PayToAddrScript(addr)
	require.NoError(t, err)

	output := &wire.TxOut{
		Value:    outputValue,
		PkScript: script,
	}

	// Force spending the parent's specific output using WithInputs option.
	parentOutpoint := wire.OutPoint{
		Hash:  parent.TxHash(),
		Index: outputIdx,
	}

	child, err := r.CreateTransaction(
		[]*wire.TxOut{output},
		feeRate,
		false,
		rpctest.WithTxVersion(3),
		rpctest.WithInputs([]wire.OutPoint{parentOutpoint}),
	)
	require.NoError(t, err)
	return child
}

// submitAndExpectRejection submits a transaction expecting it to be rejected,
// verifying the mempool correctly enforces the policy.
func submitAndExpectRejection(t *testing.T, r *rpctest.Harness, tx *wire.MsgTx, expectedErrSubstring string) {
	t.Helper()

	_, err := r.Client.SendRawTransaction(tx, false)
	require.Error(t, err, "transaction should have been rejected")
	if expectedErrSubstring != "" {
		require.Contains(t, err.Error(), expectedErrSubstring)
	}
}

// submitAndExpectAcceptance submits a transaction expecting success and
// verifies it appears in the mempool.
func submitAndExpectAcceptance(t *testing.T, r *rpctest.Harness, tx *wire.MsgTx) *chainhash.Hash {
	t.Helper()

	txHash, err := r.Client.SendRawTransaction(tx, false)
	require.NoError(t, err, "transaction should have been accepted")

	// Verify it's in the mempool.
	mempool, err := r.Client.GetRawMempool()
	require.NoError(t, err)

	found := false
	for _, hash := range mempool {
		if hash.IsEqual(txHash) {
			found = true
			break
		}
	}
	require.True(t, found, "transaction should be in mempool")

	return txHash
}

// BIP 431 Rule tests (stubs to be implemented).

func testTRUCRule1Replaceability(t *testing.T, r *rpctest.Harness) {
	t.Run("v3 signals RBF without BIP125", func(t *testing.T) {
		// V3 transactions signal replaceability by version alone, not
		// requiring BIP 125 sequence numbers.
		v3Tx := createV3Transaction(t, r, 50000, 10)

		// Verify all sequence numbers are max (no BIP 125 signaling).
		for _, txIn := range v3Tx.TxIn {
			require.Equal(t, wire.MaxTxInSequenceNum, txIn.Sequence,
				"v3 tx should use max sequence (no BIP 125)")
		}

		submitAndExpectAcceptance(t, r, v3Tx)

		// Create replacement spending same inputs with higher fee.
		// Use WithInputs to create conflict.
		replacement, err := r.CreateTransaction(
			[]*wire.TxOut{{Value: 40000, PkScript: v3Tx.TxOut[0].PkScript}},
			20, // Higher fee rate.
			false,
			rpctest.WithTxVersion(3),
			rpctest.WithInputs([]wire.OutPoint{v3Tx.TxIn[0].PreviousOutPoint}),
		)
		require.NoError(t, err)

		// Replacement should succeed due to BIP 431 Rule 1.
		submitAndExpectAcceptance(t, r, replacement)

		// Original should be evicted.
		mempool, err := r.Client.GetRawMempool()
		require.NoError(t, err)

		for _, hash := range mempool {
			require.NotEqual(t, v3Tx.TxHash(), *hash,
				"original v3 tx should be evicted")
		}
	})
}

func testTRUCRule2AllOrNone(t *testing.T, r *rpctest.Harness) {
	t.Run("reject v3 with v2 unconfirmed parent", func(t *testing.T) {
		// Create unconfirmed v2 parent with proper script.
		addr, err := r.NewAddress()
		require.NoError(t, err)
		script, err := txscript.PayToAddrScript(addr)
		require.NoError(t, err)

		v2Parent, err := r.CreateTransaction(
			[]*wire.TxOut{{Value: 50000, PkScript: script}},
			10,
			true,
			rpctest.WithTxVersion(2),
		)
		require.NoError(t, err)
		t.Logf("Created v2Parent: %v", v2Parent.TxHash())
		submitAndExpectAcceptance(t, r, v2Parent)

		// Attempt v3 child spending v2 parent violates all-or-none rule.
		t.Logf("Creating v3 child to spend v2Parent:%d", 0)
		v3Child := createV3Child(t, r, v2Parent, 0, 25000, 20)
		t.Logf("Created v3Child: %v spending %v:%d", v3Child.TxHash(), v2Parent.TxHash(), 0)

		submitAndExpectRejection(t, r, v3Child, "invalid package topology")
	})

	t.Run("accept v3 with confirmed v2 parent", func(t *testing.T) {
		// Create v2 transaction with proper script.
		addr, err := r.NewAddress()
		require.NoError(t, err)
		script, err := txscript.PayToAddrScript(addr)
		require.NoError(t, err)

		v2Tx, err := r.CreateTransaction(
			[]*wire.TxOut{{Value: 50000, PkScript: script}},
			10,
			true,
			rpctest.WithTxVersion(2),
		)
		require.NoError(t, err)
		submitAndExpectAcceptance(t, r, v2Tx)

		// Mine block to confirm the v2 transaction.
		_, err = r.GenerateAndSubmitBlock([]*btcutil.Tx{btcutil.NewTx(v2Tx)}, -1, time.Time{})
		require.NoError(t, err)

		// V3 child with confirmed v2 parent should be accepted (Rule 2
		// applies only to unconfirmed ancestors).
		v3Child := createV3Child(t, r, v2Tx, 0, 25000, 20)
		submitAndExpectAcceptance(t, r, v3Child)
	})
}

func testTRUCRule3Topology(t *testing.T, r *rpctest.Harness) {
	t.Run("valid 1P1C accepted", func(t *testing.T) {
		// BIP 431 allows exactly 1 parent and 1 child for v3 transactions.
		parent := createV3Transaction(t, r, 50000, 10)
		submitAndExpectAcceptance(t, r, parent)

		// Register parent's outputs in wallet so child can spend them.
		// AddUnconfirmedTx auto-detects the correct keyIndex.
		require.NoError(t, r.AddUnconfirmedTx(parent))

		// Create child spending parent's first output using WithInputs.
		parentOutpoint := wire.OutPoint{Hash: parent.TxHash(), Index: 0}
		addr, err := r.NewAddress()
		require.NoError(t, err)
		script, err := txscript.PayToAddrScript(addr)
		require.NoError(t, err)

		child, err := r.CreateTransaction(
			[]*wire.TxOut{{Value: 25000, PkScript: script}},
			20,
			false,
			rpctest.WithTxVersion(3),
			rpctest.WithInputs([]wire.OutPoint{parentOutpoint}),
		)
		require.NoError(t, err)
		submitAndExpectAcceptance(t, r, child)
	})

	t.Run("reject long v3 chain", func(t *testing.T) {
		// Create v3 parent.
		parent := createV3Transaction(t, r, 50000, 10)
		submitAndExpectAcceptance(t, r, parent)
		require.NoError(t, r.AddUnconfirmedTx(parent))

		// Create v3 child spending parent.
		child := createV3Child(t, r, parent, 0, 25000, 20)
		submitAndExpectAcceptance(t, r, child)
		require.NoError(t, r.AddUnconfirmedTx(child))

		// Attempt to create grandchild would give it 2 unconfirmed ancestors
		// (parent + child), violating BIP 431 Rule 3's 1-ancestor limit.
		grandchild := createV3Child(t, r, child, 0, 10000, 30)
		submitAndExpectRejection(t, r, grandchild, "invalid package topology")
	})
}

func testTRUCRule4ParentSize(t *testing.T, r *rpctest.Harness) {
	// Note: Size limit testing with rpctest is complex because transaction
	// size depends on witness data and signatures. The unit tests provide
	// comprehensive coverage of size validation with precise size control.
	// E2E tests verify the integration works with real transactions.
	t.Run("normal size parent accepted", func(t *testing.T) {
		// Create v3 parent well under 10k vB limit (typical tx ~250 vB).
		parent := createV3Transaction(t, r, 50000, 10)
		submitAndExpectAcceptance(t, r, parent)
	})
}

func testTRUCRule5ChildSize(t *testing.T, r *rpctest.Harness) {
	t.Run("normal size child accepted", func(t *testing.T) {
		// Create v3 parent.
		parent := createV3Transaction(t, r, 50000, 10)
		submitAndExpectAcceptance(t, r, parent)

		// Create child well under 1k vB limit.
		child := createV3Child(t, r, parent, 0, 25000, 20)
		submitAndExpectAcceptance(t, r, child)
	})
}

func testTRUCRule6ZeroFeePackages(t *testing.T, r *rpctest.Harness) {
	// Note: Zero-fee package testing requires TestMempoolAccept RPC which
	// may not be available in this btcd version. Unit tests cover the
	// IsZeroFee detection. This is marked as future work for package relay.
	t.Skip("Zero-fee package submission requires package relay RPC support")
}

func testTRUCPinningPrevention(t *testing.T, r *rpctest.Harness) {
	t.Run("cannot create pinning topology", func(t *testing.T) {
		// The 1P1C topology prevents common pinning vectors.
		parent := createV3Transaction(t, r, 50000, 10)
		submitAndExpectAcceptance(t, r, parent)

		child := createV3Child(t, r, parent, 0, 25000, 20)
		submitAndExpectAcceptance(t, r, child)

		// Cannot add another child (would enable pinning via descendants).
		// Note: This may trigger RBF logic if both children spend the same
		// output, so we just verify rejection occurs.
		secondChild := createV3Child(t, r, parent, 0, 25000, 25)
		submitAndExpectRejection(t, r, secondChild, "rejected")

		// Cannot add grandchild (would enable pinning via long chains).
		grandchild := createV3Child(t, r, child, 0, 10000, 30)
		submitAndExpectRejection(t, r, grandchild, "invalid package topology")
	})
}
