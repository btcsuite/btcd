package rpctest

import (
	"bytes"
	"os"
	"testing"

	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/btcsuite/btcutil/coinset"
)

func TestSetUp(t *testing.T) {
	// Create a new test instance.
	nodeTest, err := New(nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Initiate setup, generated a chain of length 125.
	if err := nodeTest.SetUp(true); err != nil {
		t.Fatalf("Unable to complete rpctest setup: ", err)
	}

	// Chain length is 125, we should have 25 mature coinbase outputs.
	if len(nodeTest.matureCoinbases) != 25 {
		t.Fatalf("incorrect number of mature coinbases, have %v, should be %v",
			len(nodeTest.matureCoinbases), 24)
	}

	// Ensure outputs have correct number of confirmations.
	expectedConfs := int64(124)
	for _, cb := range nodeTest.matureCoinbases {
		numConfs := cb.(*coinset.SimpleCoin).TxNumConfs
		if numConfs != expectedConfs {
			t.Errorf("confirmations for coinbased was not updated. Has "+
				"%v confs should have %v", numConfs, expectedConfs)
		}
		expectedConfs--
	}

	// Current tip should be at height 124.
	if nodeTest.currentTip.Height() != 124 {
		t.Errorf("Chain height is %v, should be %v",
			nodeTest.currentTip.Height(), 124)
	}

	nodeTest.TearDown()

	// Ensure all test directories have been deleted.
	if _, err := os.Stat(nodeTest.testNodeDir); err == nil {
		t.Errorf("Create test datadir was not deleted.")
	}
}

func TestGenerateBlock(t *testing.T) {
	// Create a new test instance.
	nodeTest, err := New(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer nodeTest.TearDown()
	if err := nodeTest.SetUp(true); err != nil {
		t.Fatalf("Unable to complete rpctest setup: ", err)
	}

	prevBlockHeight := nodeTest.currentTip.Height()

	newBlock, err := nodeTest.GenerateAndSubmitBlock(nil)
	if err != nil {
		t.Errorf("Unable to generate block: ", err)
	}

	// New block should have the proper height.
	if newBlock.Height() != prevBlockHeight+1 {
		t.Errorf("Generated block has incorrect height: ", err)
	}

	// We should have one more mature coinbase to spend.
	if len(nodeTest.matureCoinbases) != 26 {
		t.Fatalf("incorrect number of mature coinbases, have %v, should be %v",
			len(nodeTest.matureCoinbases), 24)
	}

	// Confirmations should have been updated.
	numConfsYoungestCb := nodeTest.matureCoinbases[24].(*coinset.SimpleCoin).TxNumConfs
	if numConfsYoungestCb != 101 {
		t.Errorf("confirmations for coinbased was not updated. Has "+
			"%v confs should have %v", numConfsYoungestCb, 101)
	}
}

func TestCraftCoinbaseSpend(t *testing.T) {
	// Create a new test instance.
	nodeTest, err := New(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer nodeTest.TearDown()
	if err := nodeTest.SetUp(true); err != nil {
		t.Fatalf("Unable to complete rpctest setup: ", err)
	}

	// Create a signed tx with a 100k satoshi (1BTC) output.
	var z []byte
	signedTx, err := nodeTest.CraftCoinbaseSpend([]*wire.TxOut{
		wire.NewTxOut(100000000, z),
	})
	if err != nil {
		t.Fatalf("Unable to create coinbase spend: %v", err)
	}

	minedBlock, err := nodeTest.GenerateAndSubmitBlock([]*btcutil.Tx{btcutil.NewTx(signedTx)})
	if err != nil {
		t.Fatalf("Unable to mine block: %v", err)
	}

	// Block should contain our tx.
	sha, _ := signedTx.TxSha()
	if !bytes.Equal(minedBlock.Transactions()[1].Sha()[:], sha.Bytes()) {
		t.Fatalf("Sha doesn't match %v vs %v", sha, minedBlock.Transactions()[1].Sha())
	}

	// Coinbase output should have been removed.
	for _, cbOut := range nodeTest.matureCoinbases {
		if bytes.Equal(cbOut.Hash()[:], signedTx.TxIn[0].PreviousOutPoint.Hash[:]) {
			t.Fatalf("spent coinbase was not removed")
		}
	}
}
