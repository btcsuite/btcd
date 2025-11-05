//go:build rpctest
// +build rpctest

package integration

import (
	"testing"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// TestSubmitPackage tests the submitpackage RPC call with various scenarios.
func TestSubmitPackage(t *testing.T) {
	btcdCfg := []string{
		"--rejectnonstd",
		"--debuglevel=debug",
		"--usetxmempoolv2", // Enable v2 mempool with package support.
	}
	r, err := rpctest.New(&chaincfg.SimNetParams, nil, btcdCfg, "")
	require.NoError(t, err)

	require.NoError(t, r.SetUp(true, 100))
	defer func() {
		if err := r.TearDown(); err != nil {
			t.Logf("TearDown error: %v", err)
		}
	}()

	t.Run("valid 1P1C v3 package", func(t *testing.T) {
		// Ensure mempool starts empty by mining any lingering transactions.
		mempool, err := r.Client.GetRawMempool()
		require.NoError(t, err)
		if len(mempool) > 0 {
			t.Logf("Clearing %d transactions from mempool", len(mempool))
			_, err := r.Client.Generate(1)
			require.NoError(t, err)
		}

		// Verify mempool is now empty.
		mempool, err = r.Client.GetRawMempool()
		require.NoError(t, err)
		require.Empty(t, mempool, "mempool must be empty at test start")

		// Create a v3 parent transaction.
		parent := createV3Transaction(t, r, 50000, 10)
		t.Logf("Created parent: %s", parent.TxHash())

		// Create a v3 child spending the parent.
		child := createV3Child(t, r, parent, 0, 25000, 20)
		t.Logf("Created child: %s", child.TxHash())

		// Submit package via RPC.
		result, err := r.Client.SubmitPackage([]*wire.MsgTx{parent, child}, nil, nil)
		require.NoError(t, err)

		// Log package result.
		t.Logf("Package result: %s", result.PackageMsg)

		// Verify success message.
		require.Equal(t, "success", result.PackageMsg)

		// Verify both transactions have results.
		require.Len(t, result.TxResults, 2)

		parentWtxid := parent.WitnessHash().String()
		childWtxid := child.WitnessHash().String()

		// Verify parent result exists and was accepted.
		parentResult, ok := result.TxResults[parentWtxid]
		require.True(t, ok, "parent result should be present")
		require.NotNil(t, parentResult, "parent result should not be nil")
		if parentResult.Error != nil {
			t.Fatalf("Parent rejected: %s", *parentResult.Error)
		}
		require.Equal(t, parent.TxHash().String(), parentResult.TxID)

		// Verify child result exists and was accepted.
		childResult, ok := result.TxResults[childWtxid]
		require.True(t, ok, "child result should be present")
		require.NotNil(t, childResult, "child result should not be nil")
		if childResult.Error != nil {
			t.Fatalf("Child rejected: %s", *childResult.Error)
		}
		require.Equal(t, child.TxHash().String(), childResult.TxID)

		// Verify fees were calculated.
		require.NotNil(t, parentResult.Fees)
		require.NotNil(t, childResult.Fees)
		require.Greater(t, parentResult.Fees.Base, float64(0))
		require.Greater(t, childResult.Fees.Base, float64(0))

		// CRITICAL: Verify transactions are actually in mempool.
		mempool, err = r.Client.GetRawMempool()
		require.NoError(t, err)

		t.Logf("Mempool now has %d transactions:", len(mempool))
		for _, hash := range mempool {
			t.Logf("  - %s", hash)
		}

		// Convert to map for proper comparison (mempool returns []*chainhash.Hash).
		mempoolMap := make(map[chainhash.Hash]bool)
		for _, hash := range mempool {
			mempoolMap[*hash] = true
		}

		require.True(t, mempoolMap[parent.TxHash()],
			"BUG: parent %s marked accepted but not in mempool!", parent.TxHash())
		require.True(t, mempoolMap[child.TxHash()],
			"BUG: child %s marked accepted but not in mempool!", child.TxHash())
	})

	t.Run("package with invalid topology rejected", func(t *testing.T) {
		// Create parent and child.
		parent := createV3Transaction(t, r, 50000, 10)
		child := createV3Child(t, r, parent, 0, 25000, 20)

		// Submit in wrong order (child before parent).
		_, err := r.Client.SubmitPackage([]*wire.MsgTx{child, parent}, nil, nil)
		require.Error(t, err, "should reject package with bad topology")
		require.Contains(t, err.Error(), "topologically sorted")
	})

	t.Run("zero-fee v3 parent with high-fee child (BIP 431 Rule 6)", func(t *testing.T) {
		// Clear mempool before test.
		mempool, err := r.Client.GetRawMempool()
		require.NoError(t, err)
		if len(mempool) > 0 {
			_, err = r.Client.Generate(1)
			require.NoError(t, err)
		}

		// Create a zero-fee v3 parent.
		parent := createV3Transaction(t, r, 50000, 0)

		// Create high-fee v3 child to pay for both (CPFP).
		// Child fee should be enough to cover both txs at min relay rate.
		child := createV3Child(t, r, parent, 0, 25000, 30)

		// Submit package - should succeed due to BIP 431 Rule 6.
		result, err := r.Client.SubmitPackage([]*wire.MsgTx{parent, child}, nil, nil)
		require.NoError(t, err,
			"BIP 431 Rule 6: zero-fee TRUC parent should be accepted in package")

		require.Equal(t, "success", result.PackageMsg)

		// Verify parent was accepted despite zero fee (BIP 431 Rule 6).
		parentWtxid := parent.WitnessHash().String()
		parentResult := result.TxResults[parentWtxid]
		require.NotNil(t, parentResult)
		require.Nil(t, parentResult.Error, "zero-fee TRUC parent should be accepted")
		require.Equal(t, parent.TxHash().String(), parentResult.TxID)

		// Verify child was accepted.
		childWtxid := child.WitnessHash().String()
		childResult := result.TxResults[childWtxid]
		require.NotNil(t, childResult)
		require.Nil(t, childResult.Error)
		require.Greater(t, childResult.Fees.Base, float64(0), "child should have non-zero fee")
	})

	t.Run("package RBF replacement [A,B] → [A,B']", func(t *testing.T) {
		// Clear mempool.
		mempool, err := r.Client.GetRawMempool()
		require.NoError(t, err)
		if len(mempool) > 0 {
			_, err = r.Client.Generate(1)
			require.NoError(t, err)
		}

		// Step 1: Submit package [A, B] where A has zero fee, B pays modestly.
		// A creates a LARGE output so B' can pay substantial fees.
		parentA := createV3Transaction(t, r, 5000000, 0) // Zero fee, 5M sat output
		childB := createV3Child(t, r, parentA, 0, 4900000, 5) // Low fee rate initially

		result1, err := r.Client.SubmitPackage([]*wire.MsgTx{parentA, childB}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "success", result1.PackageMsg)

		// Verify both in mempool.
		mempool, err = r.Client.GetRawMempool()
		require.NoError(t, err)
		mempoolMap := make(map[chainhash.Hash]bool)
		for _, hash := range mempool {
			mempoolMap[*hash] = true
		}
		require.True(t, mempoolMap[parentA.TxHash()], "A should be in mempool")
		require.True(t, mempoolMap[childB.TxHash()], "B should be in mempool")

		// Step 2: Create B' that spends same output from A but pays MUCH higher fee.
		// B' conflicts with B (spends same parent output).
		// For package RBF to succeed: package [A, B'] fee rate must beat B's rate.
		// A: ~225 vbytes, 0 fee
		// B: ~190 vbytes, ~5*190 = ~950 sats, rate ~5000 sat/kvB
		// Package [A, B']: (0 + B'_fee) / (225 + 190) * 1000 > B's ~5000 sat/kvB
		// B'_fee > 5000 * 415 / 1000 = 2075 sats
		// Use much higher to ensure passing: B' outputs 4800000, fee = 100000 sats
		childBPrime := createV3Child(t, r, parentA, 0, 4700000, 1000) // Extremely high fee for RBF

		// Step 3: Submit package [A, B'] - should succeed via package RBF.
		result2, err := r.Client.SubmitPackage([]*wire.MsgTx{parentA, childBPrime}, nil, nil)
		require.NoError(t, err, "Package RBF should succeed with sufficient fees")
		require.Equal(t, "success", result2.PackageMsg)

		// Verify A was deduplicated (marked accepted).
		aWtxid := parentA.WitnessHash().String()
		aResult := result2.TxResults[aWtxid]
		require.NotNil(t, aResult)
		require.Nil(t, aResult.Error, "A should be accepted (deduplication)")

		// Verify B' was accepted.
		bPrimeWtxid := childBPrime.WitnessHash().String()
		bPrimeResult := result2.TxResults[bPrimeWtxid]
		require.NotNil(t, bPrimeResult)
		require.Nil(t, bPrimeResult.Error, "B' should be accepted via individual RBF")

		// Step 4: Verify final mempool state - should have A + B', not B.
		mempool, err = r.Client.GetRawMempool()
		require.NoError(t, err)
		mempoolMap = make(map[chainhash.Hash]bool)
		for _, hash := range mempool {
			mempoolMap[*hash] = true
		}

		require.True(t, mempoolMap[parentA.TxHash()],
			"A should still be in mempool")
		require.False(t, mempoolMap[childB.TxHash()],
			"B should be replaced (removed from mempool)")
		require.True(t, mempoolMap[childBPrime.TxHash()],
			"B' should be in mempool (replaced B)")

		t.Logf("✅ Package RBF successfully replaced B with B'")
		t.Logf("   Original package: A (zero-fee) + B (low fee)")
		t.Logf("   Replacement package: A (deduplicated) + B' (high fee)")
		t.Logf("   Mechanism: Package-level RBF validation + atomic replacement")
	})

	t.Run("true package RBF [A,B] → [A',B''] (both conflict)", func(t *testing.T) {
		// Clear mempool.
		mempool, err := r.Client.GetRawMempool()
		require.NoError(t, err)
		if len(mempool) > 0 {
			_, err = r.Client.Generate(1)
			require.NoError(t, err)
		}

		// Step 1: Submit original package [A, B] with modest fees.
		parentA := createV3Transaction(t, r, 5000000, 10)

		// Collect A's inputs to create conflicting A'.
		aInputs := make([]wire.OutPoint, len(parentA.TxIn))
		for i, txIn := range parentA.TxIn {
			aInputs[i] = txIn.PreviousOutPoint
		}

		childB := createV3Child(t, r, parentA, 0, 4900000, 10)

		result1, err := r.Client.SubmitPackage([]*wire.MsgTx{parentA, childB}, nil, nil)
		require.NoError(t, err)
		require.Equal(t, "success", result1.PackageMsg)

		// Verify both in mempool.
		mempool, err = r.Client.GetRawMempool()
		require.NoError(t, err)
		mempoolMap := make(map[chainhash.Hash]bool)
		for _, hash := range mempool {
			mempoolMap[*hash] = true
		}
		require.True(t, mempoolMap[parentA.TxHash()], "A should be in mempool")
		require.True(t, mempoolMap[childB.TxHash()], "B should be in mempool")

		// Step 2: Create A' that spends the SAME inputs as A (creating a conflict).
		// Use CreateTransaction with WithInputs to force same inputs, wallet signs it.
		aPrimeAddr, err := r.NewAddress()
		require.NoError(t, err)
		aPrimeScript, err := txscript.PayToAddrScript(aPrimeAddr)
		require.NoError(t, err)

		aPrimeOutput := &wire.TxOut{
			Value:    4800000, // Lower output = higher fee
			PkScript: aPrimeScript,
		}

		// Create A' spending same inputs as A with higher fee.
		parentAPrime, err := r.CreateTransaction(
			[]*wire.TxOut{aPrimeOutput},
			500,           // High fee rate
			false,         // No change
			rpctest.WithTxVersion(3),
			rpctest.WithInputs(aInputs), // Force same inputs as A!
		)
		require.NoError(t, err)

		// Create B'' spending A'.
		childBDoublePrime := createV3Child(t, r, parentAPrime, 0, 4700000, 500)

		// Step 3: Submit replacement package [A', B''].
		result2, err := r.Client.SubmitPackage([]*wire.MsgTx{parentAPrime, childBDoublePrime}, nil, nil)
		require.NoError(t, err, "RPC call should succeed")

		// Debug: Check for errors.
		for wtxid, txRes := range result2.TxResults {
			if txRes.Error != nil {
				t.Logf("TX %s: REJECTED - %v", wtxid, *txRes.Error)
			}
		}

		require.Equal(t, "success", result2.PackageMsg,
			"Package should be accepted, got: %s", result2.PackageMsg)

		// Step 4: Verify final mempool state.
		mempool, err = r.Client.GetRawMempool()
		require.NoError(t, err)
		mempoolMap = make(map[chainhash.Hash]bool)
		for _, hash := range mempool {
			mempoolMap[*hash] = true
		}

		// Original package should be completely replaced.
		require.False(t, mempoolMap[parentA.TxHash()],
			"Original A should be replaced by A'")
		require.False(t, mempoolMap[childB.TxHash()],
			"Original B should be replaced")

		// Replacement package should be present.
		require.True(t, mempoolMap[parentAPrime.TxHash()],
			"A' should be in mempool")
		require.True(t, mempoolMap[childBDoublePrime.TxHash()],
			"B'' should be in mempool")

		t.Logf("✅ True package RBF: [A,B] fully replaced by [A',B'']")
		t.Logf("   Replaced %d transactions", len(result2.ReplacedTransactions))
	})
}
