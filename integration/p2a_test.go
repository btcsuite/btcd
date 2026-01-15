//go:build rpctest
// +build rpctest

package integration

import (
	"testing"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// TestPayToAnchorSimple tests creating and spending P2A outputs.
func TestPayToAnchorSimple(t *testing.T) {
	t.Parallel()

	// Integration tests require a full harness setup which is
	// resource-intensive.
	if testing.Short() {
		t.Skip("Skipping P2A integration test in short mode")
	}

	// Create a btcd instance for testing P2A functionality in a controlled
	// environment.
	harness, err := rpctest.New(
		&chaincfg.SimNetParams, nil, nil, "",
	)
	if err != nil {
		t.Fatalf("unable to create test harness: %v", err)
	}
	defer harness.TearDown()

	// Initialize the test harness with mining enabled to confirm
	// transactions.
	err = harness.SetUp(true, 25)
	if err != nil {
		t.Fatalf("unable to setup test harness: %v", err)
	}

	// Create a P2A output using the helper to get a P2A address. This
	// ensures we're using the same P2A script generation logic.
	p2aAddr, err := btcutil.NewAddressPayToAnchor(&chaincfg.SimNetParams)
	if err != nil {
		t.Fatalf("unable to create P2A address: %v", err)
	}

	// Use the harness to create a transaction that sends to the P2A
	// address. This handles all the UTXO selection and signing for us.
	amount := btcutil.Amount(10_000)
	createP2ATxHash, err := harness.SendOutputs([]*wire.TxOut{
		wire.NewTxOut(int64(amount), p2aAddr.ScriptAddress()),
	}, 10)
	if err != nil {
		t.Fatalf("unable to send P2A creation transaction: %v", err)
	}

	// Mine a block to confirm the P2A creation.
	blockHashes, err := harness.Client.Generate(1)
	if err != nil {
		t.Fatalf("unable to generate block: %v", err)
	}
	if len(blockHashes) != 1 {
		t.Fatalf("expected 1 block hash, got %d", len(blockHashes))
	}

	// Test spending the P2A output to verify it works as anyone-can-spend.
	spendP2ATx := wire.NewMsgTx(wire.TxVersion)

	// Reference the P2A output we just created.
	p2aOutpoint := wire.NewOutPoint(createP2ATxHash, 0)
	p2aInput := wire.NewTxIn(p2aOutpoint, nil, nil)

	// P2A outputs are designed to be spent without signatures for CPFP fee
	// bumping. The signature script is completely empty for P2A outputs.
	p2aInput.SignatureScript = []byte{}
	spendP2ATx.AddTxIn(p2aInput)

	// Send the P2A funds to a regular address.
	spendAddr, err := harness.NewAddress()
	if err != nil {
		t.Fatalf("unable to get spend address: %v", err)
	}
	spendScript, err := txscript.PayToAddrScript(spendAddr)
	if err != nil {
		t.Fatalf("unable to create spend script: %v", err)
	}

	// Deduct a small fee from the P2A output value.
	spendOut := wire.NewTxOut(int64(amount-100), spendScript)
	spendP2ATx.AddTxOut(spendOut)

	// Broadcast the spend transaction to verify network acceptance. P2A
	// outputs are witness programs and are validated through the normal
	// transaction validation path in the mempool and consensus.
	spendTxHash, err := harness.Client.SendRawTransaction(spendP2ATx, true)
	if err != nil {
		t.Fatalf("unable to send P2A spend transaction: %v", err)
	}

	// Mine a block to confirm the spend.
	blockHashes, err = harness.Client.Generate(1)
	if err != nil {
		t.Fatalf("unable to generate block after spend: %v", err)
	}

	// Ensure the spend transaction was actually mined to prove full P2A
	// support.
	//
	block, err := harness.Client.GetBlock(blockHashes[0])
	if err != nil {
		t.Fatalf("unable to get block: %v", err)
	}

	// Confirm the spend transaction exists in the confirmed block.
	found := false
	for _, tx := range block.Transactions {
		txHash := tx.TxHash()
		if txHash.IsEqual(spendTxHash) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("P2A spend transaction not found in block")
	}

	t.Logf("Successfully created P2A output in tx %v and spent it in tx %v",
		createP2ATxHash, spendTxHash)
}

