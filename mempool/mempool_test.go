// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"encoding/hex"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

// fakeChain is used by the pool harness to provide generated test utxos and
// a current faked chain height to the pool callbacks.  This, in turn, allows
// transactions to appear as though they are spending completely valid utxos.
type fakeChain struct {
	sync.RWMutex
	utxos          *blockchain.UtxoViewpoint
	currentHeight  int32
	medianTimePast time.Time
}

// FetchUtxoView loads utxo details about the inputs referenced by the passed
// transaction from the point of view of the fake chain.  It also attempts to
// fetch the utxos for the outputs of the transaction itself so the returned
// view can be examined for duplicate transactions.
//
// This function is safe for concurrent access however the returned view is NOT.
func (s *fakeChain) FetchUtxoView(tx *btcutil.Tx) (*blockchain.UtxoViewpoint, error) {
	s.RLock()
	defer s.RUnlock()

	// All entries are cloned to ensure modifications to the returned view
	// do not affect the fake chain's view.

	// Add an entry for the tx itself to the new view.
	viewpoint := blockchain.NewUtxoViewpoint()
	prevOut := wire.OutPoint{Hash: *tx.Hash()}
	for txOutIdx := range tx.MsgTx().TxOut {
		prevOut.Index = uint32(txOutIdx)
		entry := s.utxos.LookupEntry(prevOut)
		viewpoint.Entries()[prevOut] = entry.Clone()
	}

	// Add entries for all of the inputs to the tx to the new view.
	for _, txIn := range tx.MsgTx().TxIn {
		entry := s.utxos.LookupEntry(txIn.PreviousOutPoint)
		viewpoint.Entries()[txIn.PreviousOutPoint] = entry.Clone()
	}

	return viewpoint, nil
}

// BestHeight returns the current height associated with the fake chain
// instance.
func (s *fakeChain) BestHeight() int32 {
	s.RLock()
	height := s.currentHeight
	s.RUnlock()
	return height
}

// SetHeight sets the current height associated with the fake chain instance.
func (s *fakeChain) SetHeight(height int32) {
	s.Lock()
	s.currentHeight = height
	s.Unlock()
}

// MedianTimePast returns the current median time past associated with the fake
// chain instance.
func (s *fakeChain) MedianTimePast() time.Time {
	s.RLock()
	mtp := s.medianTimePast
	s.RUnlock()
	return mtp
}

// SetMedianTimePast sets the current median time past associated with the fake
// chain instance.
func (s *fakeChain) SetMedianTimePast(mtp time.Time) {
	s.Lock()
	s.medianTimePast = mtp
	s.Unlock()
}

// CalcSequenceLock returns the current sequence lock for the passed
// transaction associated with the fake chain instance.
func (s *fakeChain) CalcSequenceLock(tx *btcutil.Tx,
	view *blockchain.UtxoViewpoint) (*blockchain.SequenceLock, error) {

	return &blockchain.SequenceLock{
		Seconds:     -1,
		BlockHeight: -1,
	}, nil
}

// spendableOutput is a convenience type that houses a particular utxo and the
// amount associated with it.
type spendableOutput struct {
	outPoint wire.OutPoint
	amount   btcutil.Amount
}

// txOutToSpendableOut returns a spendable output given a transaction and index
// of the output to use.  This is useful as a convenience when creating test
// transactions.
func txOutToSpendableOut(tx *btcutil.Tx, outputNum uint32) spendableOutput {
	return spendableOutput{
		outPoint: wire.OutPoint{Hash: *tx.Hash(), Index: outputNum},
		amount:   btcutil.Amount(tx.MsgTx().TxOut[outputNum].Value),
	}
}

// poolHarness provides a harness that includes functionality for creating and
// signing transactions as well as a fake chain that provides utxos for use in
// generating valid transactions.
type poolHarness struct {
	// signKey is the signing key used for creating transactions throughout
	// the tests.
	//
	// payAddr is the p2sh address for the signing key and is used for the
	// payment address throughout the tests.
	signKey     *btcec.PrivateKey
	payAddr     btcutil.Address
	payScript   []byte
	chainParams *chaincfg.Params

	chain  *fakeChain
	txPool *TxPool
}

// CreateCoinbaseTx returns a coinbase transaction with the requested number of
// outputs paying an appropriate subsidy based on the passed block height to the
// address associated with the harness.  It automatically uses a standard
// signature script that starts with the block height that is required by
// version 2 blocks.
func (p *poolHarness) CreateCoinbaseTx(blockHeight int32, numOutputs uint32) (*btcutil.Tx, error) {
	// Create standard coinbase script.
	extraNonce := int64(0)
	coinbaseScript, err := txscript.NewScriptBuilder().
		AddInt64(int64(blockHeight)).AddInt64(extraNonce).Script()
	if err != nil {
		return nil, err
	}

	tx := wire.NewMsgTx(wire.TxVersion)
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex),
		SignatureScript: coinbaseScript,
		Sequence:        wire.MaxTxInSequenceNum,
	})
	totalInput := blockchain.CalcBlockSubsidy(blockHeight, p.chainParams)
	amountPerOutput := totalInput / int64(numOutputs)
	remainder := totalInput - amountPerOutput*int64(numOutputs)
	for i := uint32(0); i < numOutputs; i++ {
		// Ensure the final output accounts for any remainder that might
		// be left from splitting the input amount.
		amount := amountPerOutput
		if i == numOutputs-1 {
			amount = amountPerOutput + remainder
		}
		tx.AddTxOut(&wire.TxOut{
			PkScript: p.payScript,
			Value:    amount,
		})
	}

	return btcutil.NewTx(tx), nil
}

// CreateSignedTx creates a new signed transaction that consumes the provided
// inputs and generates the provided number of outputs by evenly splitting the
// total input amount.  All outputs will be to the payment script associated
// with the harness and all inputs are assumed to do the same.
func (p *poolHarness) CreateSignedTx(inputs []spendableOutput,
	numOutputs uint32, fee btcutil.Amount,
	signalsReplacement bool) (*btcutil.Tx, error) {

	// Calculate the total input amount and split it amongst the requested
	// number of outputs.
	var totalInput btcutil.Amount
	for _, input := range inputs {
		totalInput += input.amount
	}
	totalInput -= fee
	amountPerOutput := int64(totalInput) / int64(numOutputs)
	remainder := int64(totalInput) - amountPerOutput*int64(numOutputs)

	tx := wire.NewMsgTx(wire.TxVersion)
	sequence := wire.MaxTxInSequenceNum
	if signalsReplacement {
		sequence = MaxRBFSequence
	}
	for _, input := range inputs {
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: input.outPoint,
			SignatureScript:  nil,
			Sequence:         sequence,
		})
	}
	for i := uint32(0); i < numOutputs; i++ {
		// Ensure the final output accounts for any remainder that might
		// be left from splitting the input amount.
		amount := amountPerOutput
		if i == numOutputs-1 {
			amount = amountPerOutput + remainder
		}
		tx.AddTxOut(&wire.TxOut{
			PkScript: p.payScript,
			Value:    amount,
		})
	}

	// Sign the new transaction.
	for i := range tx.TxIn {
		sigScript, err := txscript.SignatureScript(tx, i, p.payScript,
			txscript.SigHashAll, p.signKey, true)
		if err != nil {
			return nil, err
		}
		tx.TxIn[i].SignatureScript = sigScript
	}

	return btcutil.NewTx(tx), nil
}

// CreateTxChain creates a chain of zero-fee transactions (each subsequent
// transaction spends the entire amount from the previous one) with the first
// one spending the provided outpoint.  Each transaction spends the entire
// amount of the previous one and as such does not include any fees.
func (p *poolHarness) CreateTxChain(firstOutput spendableOutput, numTxns uint32) ([]*btcutil.Tx, error) {
	txChain := make([]*btcutil.Tx, 0, numTxns)
	prevOutPoint := firstOutput.outPoint
	spendableAmount := firstOutput.amount
	for i := uint32(0); i < numTxns; i++ {
		// Create the transaction using the previous transaction output
		// and paying the full amount to the payment address associated
		// with the harness.
		tx := wire.NewMsgTx(wire.TxVersion)
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: prevOutPoint,
			SignatureScript:  nil,
			Sequence:         wire.MaxTxInSequenceNum,
		})
		tx.AddTxOut(&wire.TxOut{
			PkScript: p.payScript,
			Value:    int64(spendableAmount),
		})

		// Sign the new transaction.
		sigScript, err := txscript.SignatureScript(tx, 0, p.payScript,
			txscript.SigHashAll, p.signKey, true)
		if err != nil {
			return nil, err
		}
		tx.TxIn[0].SignatureScript = sigScript

		txChain = append(txChain, btcutil.NewTx(tx))

		// Next transaction uses outputs from this one.
		prevOutPoint = wire.OutPoint{Hash: tx.TxHash(), Index: 0}
	}

	return txChain, nil
}

// newPoolHarness returns a new instance of a pool harness initialized with a
// fake chain and a TxPool bound to it that is configured with a policy suitable
// for testing.  Also, the fake chain is populated with the returned spendable
// outputs so the caller can easily create new valid transactions which build
// off of it.
func newPoolHarness(chainParams *chaincfg.Params) (*poolHarness, []spendableOutput, error) {
	// Use a hard coded key pair for deterministic results.
	keyBytes, err := hex.DecodeString("700868df1838811ffbdf918fb482c1f7e" +
		"ad62db4b97bd7012c23e726485e577d")
	if err != nil {
		return nil, nil, err
	}
	signKey, signPub := btcec.PrivKeyFromBytes(btcec.S256(), keyBytes)

	// Generate associated pay-to-script-hash address and resulting payment
	// script.
	pubKeyBytes := signPub.SerializeCompressed()
	payPubKeyAddr, err := btcutil.NewAddressPubKey(pubKeyBytes, chainParams)
	if err != nil {
		return nil, nil, err
	}
	payAddr := payPubKeyAddr.AddressPubKeyHash()
	pkScript, err := txscript.PayToAddrScript(payAddr)
	if err != nil {
		return nil, nil, err
	}

	// Create a new fake chain and harness bound to it.
	chain := &fakeChain{utxos: blockchain.NewUtxoViewpoint()}
	harness := poolHarness{
		signKey:     signKey,
		payAddr:     payAddr,
		payScript:   pkScript,
		chainParams: chainParams,

		chain: chain,
		txPool: New(&Config{
			Policy: Policy{
				DisableRelayPriority: true,
				FreeTxRelayLimit:     15.0,
				MaxOrphanTxs:         5,
				MaxOrphanTxSize:      1000,
				MaxSigOpCostPerTx:    blockchain.MaxBlockSigOpsCost / 4,
				MinRelayTxFee:        1000, // 1 Satoshi per byte
				MaxTxVersion:         1,
			},
			ChainParams:      chainParams,
			FetchUtxoView:    chain.FetchUtxoView,
			BestHeight:       chain.BestHeight,
			MedianTimePast:   chain.MedianTimePast,
			CalcSequenceLock: chain.CalcSequenceLock,
			SigCache:         nil,
			AddrIndex:        nil,
		}),
	}

	// Create a single coinbase transaction and add it to the harness
	// chain's utxo set and set the harness chain height such that the
	// coinbase will mature in the next block.  This ensures the txpool
	// accepts transactions which spend immature coinbases that will become
	// mature in the next block.
	numOutputs := uint32(1)
	outputs := make([]spendableOutput, 0, numOutputs)
	curHeight := harness.chain.BestHeight()
	coinbase, err := harness.CreateCoinbaseTx(curHeight+1, numOutputs)
	if err != nil {
		return nil, nil, err
	}
	harness.chain.utxos.AddTxOuts(coinbase, curHeight+1)
	for i := uint32(0); i < numOutputs; i++ {
		outputs = append(outputs, txOutToSpendableOut(coinbase, i))
	}
	harness.chain.SetHeight(int32(chainParams.CoinbaseMaturity) + curHeight)
	harness.chain.SetMedianTimePast(time.Now())

	return &harness, outputs, nil
}

// testContext houses a test-related state that is useful to pass to helper
// functions as a single argument.
type testContext struct {
	t       *testing.T
	harness *poolHarness
}

// addCoinbaseTx adds a spendable coinbase transaction to the test context's
// mock chain.
func (ctx *testContext) addCoinbaseTx(numOutputs uint32) *btcutil.Tx {
	ctx.t.Helper()

	coinbaseHeight := ctx.harness.chain.BestHeight() + 1
	coinbase, err := ctx.harness.CreateCoinbaseTx(coinbaseHeight, numOutputs)
	if err != nil {
		ctx.t.Fatalf("unable to create coinbase: %v", err)
	}

	ctx.harness.chain.utxos.AddTxOuts(coinbase, coinbaseHeight)
	maturity := int32(ctx.harness.chainParams.CoinbaseMaturity)
	ctx.harness.chain.SetHeight(coinbaseHeight + maturity)
	ctx.harness.chain.SetMedianTimePast(time.Now())

	return coinbase
}

// addSignedTx creates a transaction that spends the inputs with the given fee.
// It can be added to the test context's mempool or mock chain based on the
// confirmed boolean.
func (ctx *testContext) addSignedTx(inputs []spendableOutput,
	numOutputs uint32, fee btcutil.Amount,
	signalsReplacement, confirmed bool) *btcutil.Tx {

	ctx.t.Helper()

	tx, err := ctx.harness.CreateSignedTx(
		inputs, numOutputs, fee, signalsReplacement,
	)
	if err != nil {
		ctx.t.Fatalf("unable to create transaction: %v", err)
	}

	if confirmed {
		newHeight := ctx.harness.chain.BestHeight() + 1
		ctx.harness.chain.utxos.AddTxOuts(tx, newHeight)
		ctx.harness.chain.SetHeight(newHeight)
		ctx.harness.chain.SetMedianTimePast(time.Now())
	} else {
		acceptedTxns, err := ctx.harness.txPool.ProcessTransaction(
			tx, true, false, 0,
		)
		if err != nil {
			ctx.t.Fatalf("unable to process transaction: %v", err)
		}
		if len(acceptedTxns) != 1 {
			ctx.t.Fatalf("expected one accepted transaction, got %d",
				len(acceptedTxns))
		}
		testPoolMembership(ctx, tx, false, true)
	}

	return tx
}

// testPoolMembership tests the transaction pool associated with the provided
// test context to determine if the passed transaction matches the provided
// orphan pool and transaction pool status.  It also further determines if it
// should be reported as available by the HaveTransaction function based upon
// the two flags and tests that condition as well.
func testPoolMembership(tc *testContext, tx *btcutil.Tx, inOrphanPool, inTxPool bool) {
	tc.t.Helper()

	txHash := tx.Hash()
	gotOrphanPool := tc.harness.txPool.IsOrphanInPool(txHash)
	if inOrphanPool != gotOrphanPool {
		tc.t.Fatalf("IsOrphanInPool: want %v, got %v", inOrphanPool,
			gotOrphanPool)
	}

	gotTxPool := tc.harness.txPool.IsTransactionInPool(txHash)
	if inTxPool != gotTxPool {
		tc.t.Fatalf("IsTransactionInPool: want %v, got %v", inTxPool,
			gotTxPool)
	}

	gotHaveTx := tc.harness.txPool.HaveTransaction(txHash)
	wantHaveTx := inOrphanPool || inTxPool
	if wantHaveTx != gotHaveTx {
		tc.t.Fatalf("HaveTransaction: want %v, got %v", wantHaveTx,
			gotHaveTx)
	}
}

// TestSimpleOrphanChain ensures that a simple chain of orphans is handled
// properly.  In particular, it generates a chain of single input, single output
// transactions and inserts them while skipping the first linking transaction so
// they are all orphans.  Finally, it adds the linking transaction and ensures
// the entire orphan chain is moved to the transaction pool.
func TestSimpleOrphanChain(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness.
	maxOrphans := uint32(harness.txPool.cfg.Policy.MaxOrphanTxs)
	chainedTxns, err := harness.CreateTxChain(spendableOuts[0], maxOrphans+1)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Ensure the orphans are accepted (only up to the maximum allowed so
	// none are evicted).
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true,
			false, 0)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid "+
				"orphan %v", err)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted "+
				"transactions from what should be an orphan",
				len(acceptedTxns))
		}

		// Ensure the transaction is in the orphan pool, is not in the
		// transaction pool, and is reported as available.
		testPoolMembership(tc, tx, true, false)
	}

	// Add the transaction which completes the orphan chain and ensure they
	// all get accepted.  Notice the accept orphans flag is also false here
	// to ensure it has no bearing on whether or not already existing
	// orphans in the pool are linked.
	acceptedTxns, err := harness.txPool.ProcessTransaction(chainedTxns[0],
		false, false, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid "+
			"orphan %v", err)
	}
	if len(acceptedTxns) != len(chainedTxns) {
		t.Fatalf("ProcessTransaction: reported accepted transactions "+
			"length does not match expected -- got %d, want %d",
			len(acceptedTxns), len(chainedTxns))
	}
	for _, txD := range acceptedTxns {
		// Ensure the transaction is no longer in the orphan pool, is
		// now in the transaction pool, and is reported as available.
		testPoolMembership(tc, txD.Tx, false, true)
	}
}

// TestOrphanReject ensures that orphans are properly rejected when the allow
// orphans flag is not set on ProcessTransaction.
func TestOrphanReject(t *testing.T) {
	t.Parallel()

	harness, outputs, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness.
	maxOrphans := uint32(harness.txPool.cfg.Policy.MaxOrphanTxs)
	chainedTxns, err := harness.CreateTxChain(outputs[0], maxOrphans+1)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Ensure orphans are rejected when the allow orphans flag is not set.
	for _, tx := range chainedTxns[1:] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, false,
			false, 0)
		if err == nil {
			t.Fatalf("ProcessTransaction: did not fail on orphan "+
				"%v when allow orphans flag is false", tx.Hash())
		}
		expectedErr := RuleError{}
		if reflect.TypeOf(err) != reflect.TypeOf(expectedErr) {
			t.Fatalf("ProcessTransaction: wrong error got: <%T> %v, "+
				"want: <%T>", err, err, expectedErr)
		}
		code, extracted := extractRejectCode(err)
		if !extracted {
			t.Fatalf("ProcessTransaction: failed to extract reject "+
				"code from error %q", err)
		}
		if code != wire.RejectDuplicate {
			t.Fatalf("ProcessTransaction: unexpected reject code "+
				"-- got %v, want %v", code, wire.RejectDuplicate)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatal("ProcessTransaction: reported %d accepted "+
				"transactions from failed orphan attempt",
				len(acceptedTxns))
		}

		// Ensure the transaction is not in the orphan pool, not in the
		// transaction pool, and not reported as available
		testPoolMembership(tc, tx, false, false)
	}
}

// TestOrphanEviction ensures that exceeding the maximum number of orphans
// evicts entries to make room for the new ones.
func TestOrphanEviction(t *testing.T) {
	t.Parallel()

	harness, outputs, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness that is long enough to be able to force
	// several orphan evictions.
	maxOrphans := uint32(harness.txPool.cfg.Policy.MaxOrphanTxs)
	chainedTxns, err := harness.CreateTxChain(outputs[0], maxOrphans+5)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Add enough orphans to exceed the max allowed while ensuring they are
	// all accepted.  This will cause an eviction.
	for _, tx := range chainedTxns[1:] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true,
			false, 0)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid "+
				"orphan %v", err)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted "+
				"transactions from what should be an orphan",
				len(acceptedTxns))
		}

		// Ensure the transaction is in the orphan pool, is not in the
		// transaction pool, and is reported as available.
		testPoolMembership(tc, tx, true, false)
	}

	// Figure out which transactions were evicted and make sure the number
	// evicted matches the expected number.
	var evictedTxns []*btcutil.Tx
	for _, tx := range chainedTxns[1:] {
		if !harness.txPool.IsOrphanInPool(tx.Hash()) {
			evictedTxns = append(evictedTxns, tx)
		}
	}
	expectedEvictions := len(chainedTxns) - 1 - int(maxOrphans)
	if len(evictedTxns) != expectedEvictions {
		t.Fatalf("unexpected number of evictions -- got %d, want %d",
			len(evictedTxns), expectedEvictions)
	}

	// Ensure none of the evicted transactions ended up in the transaction
	// pool.
	for _, tx := range evictedTxns {
		testPoolMembership(tc, tx, false, false)
	}
}

// TestBasicOrphanRemoval ensure that orphan removal works as expected when an
// orphan that doesn't exist is removed  both when there is another orphan that
// redeems it and when there is not.
func TestBasicOrphanRemoval(t *testing.T) {
	t.Parallel()

	const maxOrphans = 4
	harness, spendableOuts, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	harness.txPool.cfg.Policy.MaxOrphanTxs = maxOrphans
	tc := &testContext{t, harness}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness.
	chainedTxns, err := harness.CreateTxChain(spendableOuts[0], maxOrphans+1)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Ensure the orphans are accepted (only up to the maximum allowed so
	// none are evicted).
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true,
			false, 0)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid "+
				"orphan %v", err)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted "+
				"transactions from what should be an orphan",
				len(acceptedTxns))
		}

		// Ensure the transaction is in the orphan pool, not in the
		// transaction pool, and reported as available.
		testPoolMembership(tc, tx, true, false)
	}

	// Attempt to remove an orphan that has no redeemers and is not present,
	// and ensure the state of all other orphans are unaffected.
	nonChainedOrphanTx, err := harness.CreateSignedTx([]spendableOutput{{
		amount:   btcutil.Amount(5000000000),
		outPoint: wire.OutPoint{Hash: chainhash.Hash{}, Index: 0},
	}}, 1, 0, false)
	if err != nil {
		t.Fatalf("unable to create signed tx: %v", err)
	}

	harness.txPool.RemoveOrphan(nonChainedOrphanTx)
	testPoolMembership(tc, nonChainedOrphanTx, false, false)
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		testPoolMembership(tc, tx, true, false)
	}

	// Attempt to remove an orphan that has a existing redeemer but itself
	// is not present and ensure the state of all other orphans (including
	// the one that redeems it) are unaffected.
	harness.txPool.RemoveOrphan(chainedTxns[0])
	testPoolMembership(tc, chainedTxns[0], false, false)
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		testPoolMembership(tc, tx, true, false)
	}

	// Remove each orphan one-by-one and ensure they are removed as
	// expected.
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		harness.txPool.RemoveOrphan(tx)
		testPoolMembership(tc, tx, false, false)
	}
}

// TestOrphanChainRemoval ensure that orphan chains (orphans that spend outputs
// from other orphans) are removed as expected.
func TestOrphanChainRemoval(t *testing.T) {
	t.Parallel()

	const maxOrphans = 10
	harness, spendableOuts, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	harness.txPool.cfg.Policy.MaxOrphanTxs = maxOrphans
	tc := &testContext{t, harness}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness.
	chainedTxns, err := harness.CreateTxChain(spendableOuts[0], maxOrphans+1)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Ensure the orphans are accepted (only up to the maximum allowed so
	// none are evicted).
	for _, tx := range chainedTxns[1 : maxOrphans+1] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true,
			false, 0)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid "+
				"orphan %v", err)
		}

		// Ensure no transactions were reported as accepted.
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted "+
				"transactions from what should be an orphan",
				len(acceptedTxns))
		}

		// Ensure the transaction is in the orphan pool, not in the
		// transaction pool, and reported as available.
		testPoolMembership(tc, tx, true, false)
	}

	// Remove the first orphan that starts the orphan chain without the
	// remove redeemer flag set and ensure that only the first orphan was
	// removed.
	harness.txPool.mtx.Lock()
	harness.txPool.removeOrphan(chainedTxns[1], false)
	harness.txPool.mtx.Unlock()
	testPoolMembership(tc, chainedTxns[1], false, false)
	for _, tx := range chainedTxns[2 : maxOrphans+1] {
		testPoolMembership(tc, tx, true, false)
	}

	// Remove the first remaining orphan that starts the orphan chain with
	// the remove redeemer flag set and ensure they are all removed.
	harness.txPool.mtx.Lock()
	harness.txPool.removeOrphan(chainedTxns[2], true)
	harness.txPool.mtx.Unlock()
	for _, tx := range chainedTxns[2 : maxOrphans+1] {
		testPoolMembership(tc, tx, false, false)
	}
}

// TestMultiInputOrphanDoubleSpend ensures that orphans that spend from an
// output that is spend by another transaction entering the pool are removed.
func TestMultiInputOrphanDoubleSpend(t *testing.T) {
	t.Parallel()

	const maxOrphans = 4
	harness, outputs, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	harness.txPool.cfg.Policy.MaxOrphanTxs = maxOrphans
	tc := &testContext{t, harness}

	// Create a chain of transactions rooted with the first spendable output
	// provided by the harness.
	chainedTxns, err := harness.CreateTxChain(outputs[0], maxOrphans+1)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}

	// Start by adding the orphan transactions from the generated chain
	// except the final one.
	for _, tx := range chainedTxns[1:maxOrphans] {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true,
			false, 0)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid "+
				"orphan %v", err)
		}
		if len(acceptedTxns) != 0 {
			t.Fatalf("ProcessTransaction: reported %d accepted transactions "+
				"from what should be an orphan", len(acceptedTxns))
		}
		testPoolMembership(tc, tx, true, false)
	}

	// Ensure a transaction that contains a double spend of the same output
	// as the second orphan that was just added as well as a valid spend
	// from that last orphan in the chain generated above (and is not in the
	// orphan pool) is accepted to the orphan pool.  This must be allowed
	// since it would otherwise be possible for a malicious actor to disrupt
	// tx chains.
	doubleSpendTx, err := harness.CreateSignedTx([]spendableOutput{
		txOutToSpendableOut(chainedTxns[1], 0),
		txOutToSpendableOut(chainedTxns[maxOrphans], 0),
	}, 1, 0, false)
	if err != nil {
		t.Fatalf("unable to create signed tx: %v", err)
	}
	acceptedTxns, err := harness.txPool.ProcessTransaction(doubleSpendTx,
		true, false, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid orphan %v",
			err)
	}
	if len(acceptedTxns) != 0 {
		t.Fatalf("ProcessTransaction: reported %d accepted transactions "+
			"from what should be an orphan", len(acceptedTxns))
	}
	testPoolMembership(tc, doubleSpendTx, true, false)

	// Add the transaction which completes the orphan chain and ensure the
	// chain gets accepted.  Notice the accept orphans flag is also false
	// here to ensure it has no bearing on whether or not already existing
	// orphans in the pool are linked.
	//
	// This will cause the shared output to become a concrete spend which
	// will in turn must cause the double spending orphan to be removed.
	acceptedTxns, err = harness.txPool.ProcessTransaction(chainedTxns[0],
		false, false, 0)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid tx %v", err)
	}
	if len(acceptedTxns) != maxOrphans {
		t.Fatalf("ProcessTransaction: reported accepted transactions "+
			"length does not match expected -- got %d, want %d",
			len(acceptedTxns), maxOrphans)
	}
	for _, txD := range acceptedTxns {
		// Ensure the transaction is no longer in the orphan pool, is
		// in the transaction pool, and is reported as available.
		testPoolMembership(tc, txD.Tx, false, true)
	}

	// Ensure the double spending orphan is no longer in the orphan pool and
	// was not moved to the transaction pool.
	testPoolMembership(tc, doubleSpendTx, false, false)
}

// TestCheckSpend tests that CheckSpend returns the expected spends found in
// the mempool.
func TestCheckSpend(t *testing.T) {
	t.Parallel()

	harness, outputs, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}

	// The mempool is empty, so none of the spendable outputs should have a
	// spend there.
	for _, op := range outputs {
		spend := harness.txPool.CheckSpend(op.outPoint)
		if spend != nil {
			t.Fatalf("Unexpeced spend found in pool: %v", spend)
		}
	}

	// Create a chain of transactions rooted with the first spendable
	// output provided by the harness.
	const txChainLength = 5
	chainedTxns, err := harness.CreateTxChain(outputs[0], txChainLength)
	if err != nil {
		t.Fatalf("unable to create transaction chain: %v", err)
	}
	for _, tx := range chainedTxns {
		_, err := harness.txPool.ProcessTransaction(tx, true,
			false, 0)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept "+
				"tx: %v", err)
		}
	}

	// The first tx in the chain should be the spend of the spendable
	// output.
	op := outputs[0].outPoint
	spend := harness.txPool.CheckSpend(op)
	if spend != chainedTxns[0] {
		t.Fatalf("expected %v to be spent by %v, instead "+
			"got %v", op, chainedTxns[0], spend)
	}

	// Now all but the last tx should be spent by the next.
	for i := 0; i < len(chainedTxns)-1; i++ {
		op = wire.OutPoint{
			Hash:  *chainedTxns[i].Hash(),
			Index: 0,
		}
		expSpend := chainedTxns[i+1]
		spend = harness.txPool.CheckSpend(op)
		if spend != expSpend {
			t.Fatalf("expected %v to be spent by %v, instead "+
				"got %v", op, expSpend, spend)
		}
	}

	// The last tx should have no spend.
	op = wire.OutPoint{
		Hash:  *chainedTxns[txChainLength-1].Hash(),
		Index: 0,
	}
	spend = harness.txPool.CheckSpend(op)
	if spend != nil {
		t.Fatalf("Unexpeced spend found in pool: %v", spend)
	}
}

// TestSignalsReplacement tests that transactions properly signal they can be
// replaced using RBF.
func TestSignalsReplacement(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name               string
		setup              func(ctx *testContext) *btcutil.Tx
		signalsReplacement bool
	}{
		{
			// Transactions can signal replacement through
			// inheritance if any of its ancestors does.
			name: "non-signaling with unconfirmed non-signaling parent",
			setup: func(ctx *testContext) *btcutil.Tx {
				coinbase := ctx.addCoinbaseTx(1)

				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				parent := ctx.addSignedTx(outs, 1, 0, false, false)

				parentOut := txOutToSpendableOut(parent, 0)
				outs = []spendableOutput{parentOut}
				return ctx.addSignedTx(outs, 1, 0, false, false)
			},
			signalsReplacement: false,
		},
		{
			// Transactions can signal replacement through
			// inheritance if any of its ancestors does, but they
			// must be unconfirmed.
			name: "non-signaling with confirmed signaling parent",
			setup: func(ctx *testContext) *btcutil.Tx {
				coinbase := ctx.addCoinbaseTx(1)

				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				parent := ctx.addSignedTx(outs, 1, 0, true, true)

				parentOut := txOutToSpendableOut(parent, 0)
				outs = []spendableOutput{parentOut}
				return ctx.addSignedTx(outs, 1, 0, false, false)
			},
			signalsReplacement: false,
		},
		{
			name: "inherited signaling",
			setup: func(ctx *testContext) *btcutil.Tx {
				coinbase := ctx.addCoinbaseTx(1)

				// We'll create a chain of transactions
				// A -> B -> C where C is the transaction we'll
				// be checking for replacement signaling. The
				// transaction can signal replacement through
				// any of its ancestors as long as they also
				// signal replacement.
				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				a := ctx.addSignedTx(outs, 1, 0, true, false)

				aOut := txOutToSpendableOut(a, 0)
				outs = []spendableOutput{aOut}
				b := ctx.addSignedTx(outs, 1, 0, false, false)

				bOut := txOutToSpendableOut(b, 0)
				outs = []spendableOutput{bOut}
				return ctx.addSignedTx(outs, 1, 0, false, false)
			},
			signalsReplacement: true,
		},
		{
			name: "explicit signaling",
			setup: func(ctx *testContext) *btcutil.Tx {
				coinbase := ctx.addCoinbaseTx(1)
				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				return ctx.addSignedTx(outs, 1, 0, true, false)
			},
			signalsReplacement: true,
		},
	}

	for _, testCase := range testCases {
		success := t.Run(testCase.name, func(t *testing.T) {
			// We'll start each test by creating our mempool
			// harness.
			harness, _, err := newPoolHarness(&chaincfg.MainNetParams)
			if err != nil {
				t.Fatalf("unable to create test pool: %v", err)
			}
			ctx := &testContext{t, harness}

			// Each test includes a setup method, which will set up
			// its required dependencies. The transaction returned
			// is the one we'll be using to determine if it signals
			// replacement support.
			tx := testCase.setup(ctx)

			// Each test should match the expected response.
			signalsReplacement := ctx.harness.txPool.signalsReplacement(
				tx, nil,
			)
			if signalsReplacement && !testCase.signalsReplacement {
				ctx.t.Fatalf("expected transaction %v to not "+
					"signal replacement", tx.Hash())
			}
			if !signalsReplacement && testCase.signalsReplacement {
				ctx.t.Fatalf("expected transaction %v to "+
					"signal replacement", tx.Hash())
			}
		})
		if !success {
			break
		}
	}
}

// TestCheckPoolDoubleSpend ensures that the mempool can properly detect
// unconfirmed double spends in the case of replacement and non-replacement
// transactions.
func TestCheckPoolDoubleSpend(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		setup         func(ctx *testContext) *btcutil.Tx
		isReplacement bool
	}{
		{
			// Transactions that don't double spend any inputs,
			// regardless of whether they signal replacement or not,
			// are valid.
			name: "no double spend",
			setup: func(ctx *testContext) *btcutil.Tx {
				coinbase := ctx.addCoinbaseTx(1)

				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				parent := ctx.addSignedTx(outs, 1, 0, false, false)

				parentOut := txOutToSpendableOut(parent, 0)
				outs = []spendableOutput{parentOut}
				return ctx.addSignedTx(outs, 2, 0, false, false)
			},
			isReplacement: false,
		},
		{
			// Transactions that don't signal replacement and double
			// spend inputs are invalid.
			name: "non-replacement double spend",
			setup: func(ctx *testContext) *btcutil.Tx {
				coinbase1 := ctx.addCoinbaseTx(1)
				coinbaseOut1 := txOutToSpendableOut(coinbase1, 0)
				outs := []spendableOutput{coinbaseOut1}
				ctx.addSignedTx(outs, 1, 0, true, false)

				coinbase2 := ctx.addCoinbaseTx(1)
				coinbaseOut2 := txOutToSpendableOut(coinbase2, 0)
				outs = []spendableOutput{coinbaseOut2}
				ctx.addSignedTx(outs, 1, 0, false, false)

				// Create a transaction that spends both
				// coinbase outputs that were spent above. This
				// should be detected as a double spend as one
				// of the transactions doesn't signal
				// replacement.
				outs = []spendableOutput{coinbaseOut1, coinbaseOut2}
				tx, err := ctx.harness.CreateSignedTx(
					outs, 1, 0, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx
			},
			isReplacement: false,
		},
		{
			// Transactions that double spend inputs and signal
			// replacement are invalid if the mempool's policy
			// rejects replacements.
			name: "reject replacement policy",
			setup: func(ctx *testContext) *btcutil.Tx {
				// Set the mempool's policy to reject
				// replacements. Even if we have a transaction
				// that spends inputs that signal replacement,
				// it should still be rejected.
				ctx.harness.txPool.cfg.Policy.RejectReplacement = true

				coinbase := ctx.addCoinbaseTx(1)

				// Create a replaceable parent that spends the
				// coinbase output.
				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				parent := ctx.addSignedTx(outs, 1, 0, true, false)

				parentOut := txOutToSpendableOut(parent, 0)
				outs = []spendableOutput{parentOut}
				ctx.addSignedTx(outs, 1, 0, false, false)

				// Create another transaction that spends the
				// same coinbase output. Since the original
				// spender of this output, all of its spends
				// should also be conflicts.
				outs = []spendableOutput{coinbaseOut}
				tx, err := ctx.harness.CreateSignedTx(
					outs, 2, 0, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx
			},
			isReplacement: false,
		},
		{
			// Transactions that double spend inputs and signal
			// replacement are valid as long as the mempool's policy
			// accepts them.
			name: "replacement double spend",
			setup: func(ctx *testContext) *btcutil.Tx {
				coinbase := ctx.addCoinbaseTx(1)

				// Create a replaceable parent that spends the
				// coinbase output.
				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				parent := ctx.addSignedTx(outs, 1, 0, true, false)

				parentOut := txOutToSpendableOut(parent, 0)
				outs = []spendableOutput{parentOut}
				ctx.addSignedTx(outs, 1, 0, false, false)

				// Create another transaction that spends the
				// same coinbase output. Since the original
				// spender of this output, all of its spends
				// should also be conflicts.
				outs = []spendableOutput{coinbaseOut}
				tx, err := ctx.harness.CreateSignedTx(
					outs, 2, 0, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx
			},
			isReplacement: true,
		},
	}

	for _, testCase := range testCases {
		success := t.Run(testCase.name, func(t *testing.T) {
			// We'll start each test by creating our mempool
			// harness.
			harness, _, err := newPoolHarness(&chaincfg.MainNetParams)
			if err != nil {
				t.Fatalf("unable to create test pool: %v", err)
			}
			ctx := &testContext{t, harness}

			// Each test includes a setup method, which will set up
			// its required dependencies. The transaction returned
			// is the one we'll be querying for the expected
			// conflicts.
			tx := testCase.setup(ctx)

			// Ensure that the mempool properly detected the double
			// spend unless this is a replacement transaction.
			isReplacement, err :=
				ctx.harness.txPool.checkPoolDoubleSpend(tx)
			if testCase.isReplacement && err != nil {
				t.Fatalf("expected no error for replacement "+
					"transaction, got: %v", err)
			}
			if isReplacement && !testCase.isReplacement {
				t.Fatalf("expected replacement transaction")
			}
			if !isReplacement && testCase.isReplacement {
				t.Fatalf("expected non-replacement transaction")
			}
		})
		if !success {
			break
		}
	}
}

// TestConflicts ensures that the mempool can properly detect conflicts when
// processing new incoming transactions.
func TestConflicts(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name string

		// setup sets up the required dependencies for each test. It
		// returns the transaction we'll check for conflicts and its
		// expected unique conflicts.
		setup func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx)
	}{
		{
			// Create a transaction that would introduce no
			// conflicts in the mempool. This is done by not
			// spending any outputs that are currently being spent
			// within the mempool.
			name: "no conflicts",
			setup: func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx) {
				coinbase := ctx.addCoinbaseTx(1)

				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				parent := ctx.addSignedTx(outs, 1, 0, false, false)

				parentOut := txOutToSpendableOut(parent, 0)
				outs = []spendableOutput{parentOut}
				tx, err := ctx.harness.CreateSignedTx(
					outs, 2, 0, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx, nil
			},
		},
		{
			// Create a transaction that would introduce two
			// conflicts in the mempool by spending two outputs
			// which are each already being spent by a different
			// transaction within the mempool.
			name: "conflicts",
			setup: func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx) {
				coinbase1 := ctx.addCoinbaseTx(1)
				coinbaseOut1 := txOutToSpendableOut(coinbase1, 0)
				outs := []spendableOutput{coinbaseOut1}
				conflict1 := ctx.addSignedTx(
					outs, 1, 0, false, false,
				)

				coinbase2 := ctx.addCoinbaseTx(1)
				coinbaseOut2 := txOutToSpendableOut(coinbase2, 0)
				outs = []spendableOutput{coinbaseOut2}
				conflict2 := ctx.addSignedTx(
					outs, 1, 0, false, false,
				)

				// Create a transaction that spends both
				// coinbase outputs that were spent above.
				outs = []spendableOutput{coinbaseOut1, coinbaseOut2}
				tx, err := ctx.harness.CreateSignedTx(
					outs, 1, 0, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx, []*btcutil.Tx{conflict1, conflict2}
			},
		},
		{
			// Create a transaction that would introduce two
			// conflicts in the mempool by spending an output
			// already being spent in the mempool by a different
			// transaction. The second conflict stems from spending
			// the transaction that spends the original spender of
			// the output, i.e., a descendant of the original
			// spender.
			name: "descendant conflicts",
			setup: func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx) {
				coinbase := ctx.addCoinbaseTx(1)

				// Create a replaceable parent that spends the
				// coinbase output.
				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				parent := ctx.addSignedTx(outs, 1, 0, false, false)

				parentOut := txOutToSpendableOut(parent, 0)
				outs = []spendableOutput{parentOut}
				child := ctx.addSignedTx(outs, 1, 0, false, false)

				// Create another transaction that spends the
				// same coinbase output. Since the original
				// spender of this output has descendants, they
				// should also be conflicts.
				outs = []spendableOutput{coinbaseOut}
				tx, err := ctx.harness.CreateSignedTx(
					outs, 2, 0, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx, []*btcutil.Tx{parent, child}
			},
		},
	}

	for _, testCase := range testCases {
		success := t.Run(testCase.name, func(t *testing.T) {
			// We'll start each test by creating our mempool
			// harness.
			harness, _, err := newPoolHarness(&chaincfg.MainNetParams)
			if err != nil {
				t.Fatalf("unable to create test pool: %v", err)
			}
			ctx := &testContext{t, harness}

			// Each test includes a setup method, which will set up
			// its required dependencies. The transaction returned
			// is the one we'll be querying for the expected
			// conflicts.
			tx, conflicts := testCase.setup(ctx)

			// Assert the expected conflicts are returned.
			txConflicts := ctx.harness.txPool.txConflicts(tx)
			if len(txConflicts) != len(conflicts) {
				ctx.t.Fatalf("expected %d conflicts, got %d",
					len(conflicts), len(txConflicts))
			}
			for _, conflict := range conflicts {
				conflictHash := *conflict.Hash()
				if _, ok := txConflicts[conflictHash]; !ok {
					ctx.t.Fatalf("expected %v to be found "+
						"as a conflict", conflictHash)
				}
			}
		})
		if !success {
			break
		}
	}
}

// TestAncestorsDescendants ensures that we can properly retrieve the
// unconfirmed ancestors and descendants of a transaction.
func TestAncestorsDescendants(t *testing.T) {
	t.Parallel()

	// We'll start the test by initializing our mempool harness.
	harness, outputs, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	ctx := &testContext{t, harness}

	// We'll be creating the following chain of unconfirmed transactions:
	//
	//       B ----
	//     /        \
	//   A            E
	//     \        /
	//       C -- D
	//
	// where B and C spend A, D spends C, and E spends B and D. We set up a
	// chain like so to properly detect ancestors and descendants past a
	// single parent/child.
	aInputs := outputs[:1]
	a := ctx.addSignedTx(aInputs, 2, 0, false, false)

	bInputs := []spendableOutput{txOutToSpendableOut(a, 0)}
	b := ctx.addSignedTx(bInputs, 1, 0, false, false)

	cInputs := []spendableOutput{txOutToSpendableOut(a, 1)}
	c := ctx.addSignedTx(cInputs, 1, 0, false, false)

	dInputs := []spendableOutput{txOutToSpendableOut(c, 0)}
	d := ctx.addSignedTx(dInputs, 1, 0, false, false)

	eInputs := []spendableOutput{
		txOutToSpendableOut(b, 0), txOutToSpendableOut(d, 0),
	}
	e := ctx.addSignedTx(eInputs, 1, 0, false, false)

	// We'll be querying for the ancestors of E. We should expect to see all
	// of the transactions that it depends on.
	expectedAncestors := map[chainhash.Hash]struct{}{
		*a.Hash(): struct{}{}, *b.Hash(): struct{}{},
		*c.Hash(): struct{}{}, *d.Hash(): struct{}{},
	}
	ancestors := ctx.harness.txPool.txAncestors(e, nil)
	if len(ancestors) != len(expectedAncestors) {
		ctx.t.Fatalf("expected %d ancestors, got %d",
			len(expectedAncestors), len(ancestors))
	}
	for ancestorHash := range ancestors {
		if _, ok := expectedAncestors[ancestorHash]; !ok {
			ctx.t.Fatalf("found unexpected ancestor %v",
				ancestorHash)
		}
	}

	// Then, we'll query for the descendants of A. We should expect to see
	// all of the transactions that depend on it.
	expectedDescendants := map[chainhash.Hash]struct{}{
		*b.Hash(): struct{}{}, *c.Hash(): struct{}{},
		*d.Hash(): struct{}{}, *e.Hash(): struct{}{},
	}
	descendants := ctx.harness.txPool.txDescendants(a, nil)
	if len(descendants) != len(expectedDescendants) {
		ctx.t.Fatalf("expected %d descendants, got %d",
			len(expectedDescendants), len(descendants))
	}
	for descendantHash := range descendants {
		if _, ok := expectedDescendants[descendantHash]; !ok {
			ctx.t.Fatalf("found unexpected descendant %v",
				descendantHash)
		}
	}
}

// TestRBF tests the different cases required for a transaction to properly
// replace its conflicts given that they all signal replacement.
func TestRBF(t *testing.T) {
	t.Parallel()

	const defaultFee = btcutil.SatoshiPerBitcoin

	testCases := []struct {
		name  string
		setup func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx)
		err   string
	}{
		{
			// A transaction cannot replace another if it doesn't
			// signal replacement.
			name: "non-replaceable parent",
			setup: func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx) {
				coinbase := ctx.addCoinbaseTx(1)

				// Create a transaction that spends the coinbase
				// output and doesn't signal for replacement.
				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				ctx.addSignedTx(outs, 1, defaultFee, false, false)

				// Attempting to create another transaction that
				// spends the same output should fail since the
				// original transaction spending it doesn't
				// signal replacement.
				tx, err := ctx.harness.CreateSignedTx(
					outs, 2, defaultFee, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx, nil
			},
			err: "already spent by transaction",
		},
		{
			// A transaction cannot replace another if we don't
			// allow accepting replacement transactions.
			name: "reject replacement policy",
			setup: func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx) {
				ctx.harness.txPool.cfg.Policy.RejectReplacement = true

				coinbase := ctx.addCoinbaseTx(1)

				// Create a transaction that spends the coinbase
				// output and doesn't signal for replacement.
				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				ctx.addSignedTx(outs, 1, defaultFee, true, false)

				// Attempting to create another transaction that
				// spends the same output should fail since the
				// original transaction spending it doesn't
				// signal replacement.
				tx, err := ctx.harness.CreateSignedTx(
					outs, 2, defaultFee, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx, nil
			},
			err: "already spent by transaction",
		},
		{
			// A transaction cannot replace another if doing so
			// would cause more than 100 transactions being
			// replaced.
			name: "exceeds maximum conflicts",
			setup: func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx) {
				const numDescendants = 100
				coinbaseOuts := make(
					[]spendableOutput, numDescendants,
				)
				for i := 0; i < numDescendants; i++ {
					tx := ctx.addCoinbaseTx(1)
					coinbaseOuts[i] = txOutToSpendableOut(tx, 0)
				}
				parent := ctx.addSignedTx(
					coinbaseOuts, numDescendants,
					defaultFee, true, false,
				)

				// We'll then spend each output of the parent
				// transaction with a distinct transaction.
				for i := uint32(0); i < numDescendants; i++ {
					out := txOutToSpendableOut(parent, i)
					outs := []spendableOutput{out}
					ctx.addSignedTx(
						outs, 1, defaultFee, false, false,
					)
				}

				// We'll then create a replacement transaction
				// by spending one of the coinbase outputs.
				// Replacing the original spender of the
				// coinbase output would evict the maximum
				// number of transactions from the mempool,
				// however, so we should reject it.
				tx, err := ctx.harness.CreateSignedTx(
					coinbaseOuts[:1], 1, defaultFee, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx, nil
			},
			err: "evicts more transactions than permitted",
		},
		{
			// A transaction cannot replace another if the
			// replacement ends up spending an output that belongs
			// to one of the transactions it replaces.
			name: "replacement spends parent transaction",
			setup: func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx) {
				coinbase := ctx.addCoinbaseTx(1)

				// Create a transaction that spends the coinbase
				// output and signals replacement.
				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				parent := ctx.addSignedTx(
					outs, 1, defaultFee, true, false,
				)

				// Attempting to create another transaction that
				// spends it, but also replaces it, should be
				// invalid.
				parentOut := txOutToSpendableOut(parent, 0)
				outs = []spendableOutput{coinbaseOut, parentOut}
				tx, err := ctx.harness.CreateSignedTx(
					outs, 2, defaultFee, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx, nil
			},
			err: "spends parent transaction",
		},
		{
			// A transaction cannot replace another if it has a
			// lower fee rate than any of the transactions it
			// intends to replace.
			name: "insufficient fee rate",
			setup: func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx) {
				coinbase1 := ctx.addCoinbaseTx(1)
				coinbase2 := ctx.addCoinbaseTx(1)

				// We'll create two transactions that each spend
				// one of the coinbase outputs. The first will
				// have a higher fee rate than the second.
				coinbaseOut1 := txOutToSpendableOut(coinbase1, 0)
				outs := []spendableOutput{coinbaseOut1}
				ctx.addSignedTx(outs, 1, defaultFee*2, true, false)

				coinbaseOut2 := txOutToSpendableOut(coinbase2, 0)
				outs = []spendableOutput{coinbaseOut2}
				ctx.addSignedTx(outs, 1, defaultFee, true, false)

				// We'll then create the replacement transaction
				// by spending the coinbase outputs. It will be
				// an invalid one however, since it won't have a
				// higher fee rate than the first transaction.
				outs = []spendableOutput{coinbaseOut1, coinbaseOut2}
				tx, err := ctx.harness.CreateSignedTx(
					outs, 1, defaultFee*2, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx, nil
			},
			err: "insufficient fee rate",
		},
		{
			// A transaction cannot replace another if it doesn't
			// have an absolute greater than the transactions its
			// replacing _plus_ the replacement transaction's
			// minimum relay fee.
			name: "insufficient absolute fee",
			setup: func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx) {
				coinbase := ctx.addCoinbaseTx(1)

				// We'll create a transaction with two outputs
				// and the default fee.
				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				ctx.addSignedTx(outs, 2, defaultFee, true, false)

				// We'll create a replacement transaction with
				// one output, which should cause the
				// transaction's absolute fee to be lower than
				// the above's, so it'll be invalid.
				tx, err := ctx.harness.CreateSignedTx(
					outs, 1, defaultFee, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx, nil
			},
			err: "insufficient absolute fee",
		},
		{
			// A transaction cannot replace another if it introduces
			// a new unconfirmed input that was not already in any
			// of the transactions it's directly replacing.
			name: "spends new unconfirmed input",
			setup: func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx) {
				coinbase1 := ctx.addCoinbaseTx(1)
				coinbase2 := ctx.addCoinbaseTx(1)

				// We'll create two unconfirmed transactions
				// from our coinbase transactions.
				coinbaseOut1 := txOutToSpendableOut(coinbase1, 0)
				outs := []spendableOutput{coinbaseOut1}
				ctx.addSignedTx(outs, 1, defaultFee, true, false)

				coinbaseOut2 := txOutToSpendableOut(coinbase2, 0)
				outs = []spendableOutput{coinbaseOut2}
				newTx := ctx.addSignedTx(
					outs, 1, defaultFee, false, false,
				)

				// We should not be able to accept a replacement
				// transaction that spends an unconfirmed input
				// that was not previously included.
				newTxOut := txOutToSpendableOut(newTx, 0)
				outs = []spendableOutput{coinbaseOut1, newTxOut}
				tx, err := ctx.harness.CreateSignedTx(
					outs, 1, defaultFee*2, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx, nil
			},
			err: "spends new unconfirmed input",
		},
		{
			// A transaction can replace another with a higher fee.
			name: "higher fee",
			setup: func(ctx *testContext) (*btcutil.Tx, []*btcutil.Tx) {
				coinbase := ctx.addCoinbaseTx(1)

				// Create a transaction that we'll directly
				// replace.
				coinbaseOut := txOutToSpendableOut(coinbase, 0)
				outs := []spendableOutput{coinbaseOut}
				parent := ctx.addSignedTx(
					outs, 1, defaultFee, true, false,
				)

				// Spend the parent transaction to create a
				// descendant that will be indirectly replaced.
				parentOut := txOutToSpendableOut(parent, 0)
				outs = []spendableOutput{parentOut}
				child := ctx.addSignedTx(
					outs, 1, defaultFee, false, false,
				)

				// The replacement transaction should replace
				// both transactions above since it has a higher
				// fee and doesn't violate any other conditions
				// within the RBF policy.
				outs = []spendableOutput{coinbaseOut}
				tx, err := ctx.harness.CreateSignedTx(
					outs, 1, defaultFee*3, false,
				)
				if err != nil {
					ctx.t.Fatalf("unable to create "+
						"transaction: %v", err)
				}

				return tx, []*btcutil.Tx{parent, child}
			},
			err: "",
		},
	}

	for _, testCase := range testCases {
		success := t.Run(testCase.name, func(t *testing.T) {
			// We'll start each test by creating our mempool
			// harness.
			harness, _, err := newPoolHarness(&chaincfg.MainNetParams)
			if err != nil {
				t.Fatalf("unable to create test pool: %v", err)
			}

			// We'll enable relay priority to ensure we can properly
			// test fees between replacement transactions and the
			// transactions it replaces.
			harness.txPool.cfg.Policy.DisableRelayPriority = false

			// Each test includes a setup method, which will set up
			// its required dependencies. The transaction returned
			// is the intended replacement, which should replace the
			// expected list of transactions.
			ctx := &testContext{t, harness}
			replacementTx, replacedTxs := testCase.setup(ctx)

			// Attempt to process the replacement transaction. If
			// it's not a valid one, we should see the error
			// expected by the test.
			_, err = ctx.harness.txPool.ProcessTransaction(
				replacementTx, false, false, 0,
			)
			if testCase.err == "" && err != nil {
				ctx.t.Fatalf("expected no error when "+
					"processing replacement transaction, "+
					"got: %v", err)
			}
			if testCase.err != "" && err == nil {
				ctx.t.Fatalf("expected error when processing "+
					"replacement transaction: %v",
					testCase.err)
			}
			if testCase.err != "" && err != nil {
				if !strings.Contains(err.Error(), testCase.err) {
					ctx.t.Fatalf("expected error: %v\n"+
						"got: %v", testCase.err, err)
				}
			}

			// If the replacement transaction is valid, we'll check
			// that it has been included in the mempool and its
			// conflicts have been removed. Otherwise, the conflicts
			// should remain in the mempool.
			valid := testCase.err == ""
			for _, tx := range replacedTxs {
				testPoolMembership(ctx, tx, false, !valid)
			}
			testPoolMembership(ctx, replacementTx, false, valid)
		})
		if !success {
			break
		}
	}
}
