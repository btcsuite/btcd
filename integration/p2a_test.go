//go:build rpctest
// +build rpctest

package integration

import (
	"testing"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// TestPayToAnchorSimple tests creating and spending P2A outputs.
func TestPayToAnchorSimple(t *testing.T) {
	t.Parallel()

	// Integration tests require a full harness setup which is
	// resource-intensive.
	if testing.Short() {
		t.Skip("Skipping P2A integration test in short mode")
	}

	h, err := rpctest.New()
	if err != nil {
		t.Fatalf("unable to create test harness: %v", err)
	}
	defer h.TearDown()

	// Start a btcd instance for testing P2A functionality in a controlled
	// environment. The simnet harness accepts non-standard transactions by
	// default, but the sub-dust and non-empty-witness cases below rely on
	// standardness checks running, so we start the node with
	// --rejectnonstd. The test harness has mining enabled to confirm
	// transactions.
	err = h.SetUp(rpctest.SOpts{
		Args:      []string{"--rejectnonstd"},
		UTXOCount: 25,
	})
	if err != nil {
		t.Fatalf("unable to setup test harness: %v", err)
	}

	// Create a P2A output using the helper to get a P2A address. This
	// ensures we're using the same P2A script generation logic.
	p2aAddr, err := address.NewAddressPayToAnchor(&chaincfg.SimNetParams)
	if err != nil {
		t.Fatalf("unable to create P2A address: %v", err)
	}
	p2aPkScript, err := txscript.PayToAddrScript(p2aAddr)
	if err != nil {
		t.Fatalf("unable to build P2A pkScript: %v", err)
	}

	// Use the harness to create a transaction that sends to the P2A
	// address. This handles all the UTXO selection and signing for us.
	amount := btcutil.Amount(10_000)
	createP2ATxHash, err := h.SendOutputs([]*wire.TxOut{
		wire.NewTxOut(int64(amount), p2aPkScript),
	}, 10)
	if err != nil {
		t.Fatalf("unable to send P2A creation transaction: %v", err)
	}

	// Mine a block to confirm the P2A creation.
	blockHashes, err := h.Client.Generate(1)
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
	spendAddr, err := h.NewAddress()
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
	spendTxHash, err := h.Client.SendRawTransaction(spendP2ATx, true)
	if err != nil {
		t.Fatalf("unable to send P2A spend transaction: %v", err)
	}

	// Mine a block to confirm the spend.
	blockHashes, err = h.Client.Generate(1)
	if err != nil {
		t.Fatalf("unable to generate block after spend: %v", err)
	}

	// Ensure the spend transaction was actually mined to prove full P2A
	// support.
	//
	block, err := h.Client.GetBlock(blockHashes[0])
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

	// As a sanity check on the policy gates, also confirm that a P2A
	// spend carrying witness data is rejected by the mempool, and that a
	// sub-dust P2A funding transaction is rejected as non-standard.
	t.Run("non-empty witness rejected", func(t *testing.T) {
		// Fund a fresh P2A output we can attempt to spend.
		fundTxHash, err := h.SendOutputs([]*wire.TxOut{
			wire.NewTxOut(int64(amount), p2aPkScript),
		}, 10)
		if err != nil {
			t.Fatalf("unable to fund second P2A output: %v", err)
		}
		if _, err := h.Client.Generate(1); err != nil {
			t.Fatalf("unable to mine block: %v", err)
		}

		// Build a spend that attaches a non-empty witness, which must
		// be rejected since P2A is anyone-can-spend with empty witness
		// only.
		badTx := wire.NewMsgTx(wire.TxVersion)
		outpoint := wire.NewOutPoint(fundTxHash, 0)
		input := wire.NewTxIn(outpoint, nil, nil)
		input.Witness = wire.TxWitness{{0x00}}
		badTx.AddTxIn(input)
		badTx.AddTxOut(wire.NewTxOut(int64(amount-100), spendScript))

		if _, err := h.Client.SendRawTransaction(
			badTx, true,
		); err == nil {

			t.Fatal("P2A spend with non-empty witness was accepted; " +
				"expected mempool rejection")
		}
	})

	t.Run("sub-dust output rejected", func(t *testing.T) {
		// Below the BIP 433 fixed 240-sat P2A dust threshold.
		const subDust = 100

		// Build a transaction paying sub-dust to P2A from a new
		// harness-funded input. CreateTransaction performs the
		// signing for us; the dust gate fires when we try to relay it.
		dustTx, err := h.CreateTransaction(
			[]*wire.TxOut{wire.NewTxOut(subDust, p2aPkScript)},
			10, true,
		)
		if err != nil {
			t.Fatalf("unable to build sub-dust funding tx: %v", err)
		}

		if _, err := h.Client.SendRawTransaction(
			dustTx, true,
		); err == nil {

			t.Fatal("sub-dust P2A output was accepted; expected " +
				"mempool dust rejection")
		}
	})
}
