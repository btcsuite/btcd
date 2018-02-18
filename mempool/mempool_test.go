// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainec"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec/secp256k1"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

// fakeChain is used by the pool harness to provide generated test utxos and
// a current faked chain height to the pool callbacks.  This, in turn, allows
// transactions to be appear as though they are spending completely valid utxos.
type fakeChain struct {
	sync.RWMutex
	nextStakeDiff int64
	utxos         *blockchain.UtxoViewpoint
	blocks        map[chainhash.Hash]*dcrutil.Block
	currentHash   chainhash.Hash
	currentHeight int64
	medianTime    time.Time
	scriptFlags   txscript.ScriptFlags
}

// NextStakeDifficulty returns the next stake difficulty associated with the
// fake chain instance.
func (s *fakeChain) NextStakeDifficulty() (int64, error) {
	s.RLock()
	nextStakeDiff := s.nextStakeDiff
	s.RUnlock()
	return nextStakeDiff, nil
}

// SetNextStakeDifficulty sets the next stake difficulty associated with the
// fake chain instance.
func (s *fakeChain) SetNextStakeDifficulty(nextStakeDiff int64) {
	s.Lock()
	s.nextStakeDiff = nextStakeDiff
	s.Unlock()
}

// FetchUtxoView loads utxo details about the input transactions referenced by
// the passed transaction from the point of view of the fake chain.
// It also attempts to fetch the utxo details for the transaction itself so the
// returned view can be examined for duplicate unspent transaction outputs.
//
// This function is safe for concurrent access however the returned view is NOT.
func (s *fakeChain) FetchUtxoView(tx *dcrutil.Tx, treeValid bool) (*blockchain.UtxoViewpoint, error) {
	s.RLock()
	defer s.RUnlock()

	// All entries are cloned to ensure modifications to the returned view
	// do not affect the fake chain's view.

	// Add an entry for the tx itself to the new view.
	viewpoint := blockchain.NewUtxoViewpoint()
	entry := s.utxos.LookupEntry(tx.Hash())
	viewpoint.Entries()[*tx.Hash()] = entry.Clone()

	// Add entries for all of the inputs to the tx to the new view.
	for _, txIn := range tx.MsgTx().TxIn {
		originHash := &txIn.PreviousOutPoint.Hash
		entry := s.utxos.LookupEntry(originHash)
		viewpoint.Entries()[*originHash] = entry.Clone()
	}

	return viewpoint, nil
}

// BlockByHash returns the block with the given hash from the fake chain
// instance.  Blocks can be added to the instance with the AddBlock function.
func (s *fakeChain) BlockByHash(hash *chainhash.Hash) (*dcrutil.Block, error) {
	s.RLock()
	block, ok := s.blocks[*hash]
	s.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unable to find block %v in fake chain",
			hash)
	}
	return block, nil
}

// AddBlock adds a block that will be available to the BlockByHash function to
// the fake chain instance.
func (s *fakeChain) AddBlock(block *dcrutil.Block) {
	s.Lock()
	s.blocks[*block.Hash()] = block
	s.Unlock()
}

// BestHash returns the current best hash associated with the fake chain
// instance.
func (s *fakeChain) BestHash() *chainhash.Hash {
	s.RLock()
	hash := &s.currentHash
	s.RUnlock()
	return hash
}

// SetHash sets the current best hash associated with the fake chain instance.
func (s *fakeChain) SetBestHash(hash *chainhash.Hash) {
	s.Lock()
	s.currentHash = *hash
	s.Unlock()
}

// BestHeight returns the current height associated with the fake chain
// instance.
func (s *fakeChain) BestHeight() int64 {
	s.RLock()
	height := s.currentHeight
	s.RUnlock()
	return height
}

// SetHeight sets the current height associated with the fake chain instance.
func (s *fakeChain) SetHeight(height int64) {
	s.Lock()
	s.currentHeight = height
	s.Unlock()
}

// PastMedianTime returns the current median time associated with the fake chain
// instance.
func (s *fakeChain) PastMedianTime() time.Time {
	s.RLock()
	medianTime := s.medianTime
	s.RUnlock()
	return medianTime
}

// SetPastMedianTime sets the current median time associated with the fake chain
// instance.
func (s *fakeChain) SetPastMedianTime(medianTime time.Time) {
	s.Lock()
	s.medianTime = medianTime
	s.Unlock()
}

// CalcSequenceLock returns the current sequence lock for the passed transaction
// associated with the fake chain instance.
func (s *fakeChain) CalcSequenceLock(tx *dcrutil.Tx, view *blockchain.UtxoViewpoint) (*blockchain.SequenceLock, error) {
	return &blockchain.SequenceLock{
		MinHeight: -1,
		MinTime:   -1,
	}, nil
}

// StandardVerifyFlags returns the standard verification script flags associated
// with the fake chain instance.
func (s *fakeChain) StandardVerifyFlags() (txscript.ScriptFlags, error) {
	return s.scriptFlags, nil
}

// SetStandardVerifyFlags sets the standard verification script flags associated
// with the fake chain instance.
func (s *fakeChain) SetStandardVerifyFlags(flags txscript.ScriptFlags) {
	s.scriptFlags = flags
}

// spendableOutput is a convenience type that houses a particular utxo and the
// amount associated with it.
type spendableOutput struct {
	outPoint wire.OutPoint
	amount   dcrutil.Amount
}

// txOutToSpendableOut returns a spendable output given a transaction and index
// of the output to use.  This is useful as a convenience when creating test
// transactions.
func txOutToSpendableOut(tx *dcrutil.Tx, outputNum uint32) spendableOutput {
	return spendableOutput{
		outPoint: wire.OutPoint{Hash: *tx.Hash(), Index: outputNum},
		amount:   dcrutil.Amount(tx.MsgTx().TxOut[outputNum].Value),
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
	signKey     chainec.PrivateKey
	payAddr     dcrutil.Address
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
func (p *poolHarness) CreateCoinbaseTx(blockHeight int64, numOutputs uint32) (*dcrutil.Tx, error) {
	// Create standard coinbase script.
	extraNonce := int64(0)
	coinbaseScript, err := txscript.NewScriptBuilder().
		AddInt64(blockHeight).AddInt64(extraNonce).Script()
	if err != nil {
		return nil, err
	}

	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		SignatureScript: coinbaseScript,
		Sequence:        wire.MaxTxInSequenceNum,
	})
	totalInput := p.txPool.cfg.SubsidyCache.CalcBlockSubsidy(blockHeight)
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

	return dcrutil.NewTx(tx), nil
}

// CreateSignedTx creates a new signed transaction that consumes the provided
// inputs and generates the provided number of outputs by evenly splitting the
// total input amount.  All outputs will be to the payment script associated
// with the harness and all inputs are assumed to do the same.
func (p *poolHarness) CreateSignedTx(inputs []spendableOutput, numOutputs uint32) (*dcrutil.Tx, error) {
	// Calculate the total input amount and split it amongst the requested
	// number of outputs.
	var totalInput dcrutil.Amount
	for _, input := range inputs {
		totalInput += input.amount
	}
	amountPerOutput := int64(totalInput) / int64(numOutputs)
	remainder := int64(totalInput) - amountPerOutput*int64(numOutputs)

	tx := wire.NewMsgTx()
	for _, input := range inputs {
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: input.outPoint,
			SignatureScript:  nil,
			Sequence:         wire.MaxTxInSequenceNum,
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

	return dcrutil.NewTx(tx), nil
}

// CreateTxChain creates a chain of zero-fee transactions (each subsequent
// transaction spends the entire amount from the previous one) with the first
// one spending the provided outpoint.  Each transaction spends the entire
// amount of the previous one and as such does not include any fees.
func (p *poolHarness) CreateTxChain(firstOutput spendableOutput, numTxns uint32) ([]*dcrutil.Tx, error) {
	txChain := make([]*dcrutil.Tx, 0, numTxns)
	prevOutPoint := firstOutput.outPoint
	spendableAmount := firstOutput.amount
	for i := uint32(0); i < numTxns; i++ {
		// Create the transaction using the previous transaction output
		// and paying the full amount to the payment address associated
		// with the harness.
		tx := wire.NewMsgTx()
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

		txChain = append(txChain, dcrutil.NewTx(tx))

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
	signKey, signPub := secp256k1.PrivKeyFromBytes(keyBytes)

	// Generate associated pay-to-script-hash address and resulting payment
	// script.
	pubKeyBytes := signPub.SerializeCompressed()
	payPubKeyAddr, err := dcrutil.NewAddressSecpPubKey(pubKeyBytes,
		chainParams)
	if err != nil {
		return nil, nil, err
	}
	payAddr := payPubKeyAddr.AddressPubKeyHash()
	pkScript, err := txscript.PayToAddrScript(payAddr)
	if err != nil {
		return nil, nil, err
	}

	// Create a new fake chain and harness bound to it.
	subsidyCache := blockchain.NewSubsidyCache(0, chainParams)
	chain := &fakeChain{
		utxos:       blockchain.NewUtxoViewpoint(),
		blocks:      make(map[chainhash.Hash]*dcrutil.Block),
		scriptFlags: BaseStandardVerifyFlags,
	}
	harness := poolHarness{
		signKey:     signKey,
		payAddr:     payAddr,
		payScript:   pkScript,
		chainParams: chainParams,

		chain: chain,
		txPool: New(&Config{
			Policy: Policy{
				MaxTxVersion:         wire.TxVersion,
				DisableRelayPriority: true,
				FreeTxRelayLimit:     15.0,
				MaxOrphanTxs:         5,
				MaxOrphanTxSize:      1000,
				MaxSigOpsPerTx:       blockchain.MaxSigOpsPerBlock / 5,
				MinRelayTxFee:        1000, // 1 Satoshi per byte
				StandardVerifyFlags:  chain.StandardVerifyFlags,
			},
			ChainParams:         chainParams,
			NextStakeDifficulty: chain.NextStakeDifficulty,
			FetchUtxoView:       chain.FetchUtxoView,
			BlockByHash:         chain.BlockByHash,
			BestHash:            chain.BestHash,
			BestHeight:          chain.BestHeight,
			PastMedianTime:      chain.PastMedianTime,
			CalcSequenceLock:    chain.CalcSequenceLock,
			SubsidyCache:        subsidyCache,
			SigCache:            nil,
			AddrIndex:           nil,
			ExistsAddrIndex:     nil,
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
	harness.chain.utxos.AddTxOuts(coinbase, curHeight+1, wire.NullBlockIndex)
	for i := uint32(0); i < numOutputs; i++ {
		outputs = append(outputs, txOutToSpendableOut(coinbase, i))
	}
	harness.chain.SetHeight(int64(chainParams.CoinbaseMaturity) + curHeight)
	harness.chain.SetPastMedianTime(time.Now())

	return &harness, outputs, nil
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
			false, true)
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

		// Ensure the transaction is in the orphan pool.
		if !harness.txPool.IsOrphanInPool(tx.Hash()) {
			t.Fatal("IsOrphanInPool: false for accepted orphan")
		}

		// Ensure the transaction is not in the transaction pool.
		if harness.txPool.IsTransactionInPool(tx.Hash()) {
			t.Fatal("IsTransactionInPool: true for accepted orphan")
		}

		// Ensure the transaction is reported as available.
		if !harness.txPool.HaveTransaction(tx.Hash()) {
			t.Fatal("HaveTransaction: false for accepted orphan")
		}
	}

	// Add the transaction which completes the orphan chain and ensure they
	// all get accepted.  Notice the accept orphans flag is also false here
	// to ensure it has no bearing on whether or not already existing
	// orphans in the pool are linked.
	acceptedTxns, err := harness.txPool.ProcessTransaction(chainedTxns[0],
		false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid "+
			"orphan %v", err)
	}
	if len(acceptedTxns) != len(chainedTxns) {
		t.Fatalf("ProcessTransaction: reported accepted transactions "+
			"length does not match expected -- got %d, want %d",
			len(acceptedTxns), len(chainedTxns))
	}
	for _, tx := range acceptedTxns {
		// Ensure none of the transactions are still in the orphan pool.
		if harness.txPool.IsOrphanInPool(tx.Hash()) {
			t.Fatalf("IsOrphanInPool: true for accepted tx %v",
				tx.Hash())
		}

		// Ensure all of the transactions are now in the transaction
		// pool.
		if !harness.txPool.IsTransactionInPool(tx.Hash()) {
			t.Fatalf("IsTransactionInPool: false for accepted tx %v",
				tx.Hash())
		}
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
			false, true)
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

		// Ensure the transaction is not in the orphan pool.
		if harness.txPool.IsOrphanInPool(tx.Hash()) {
			t.Fatal("IsOrphanInPool: true for rejected orphan")
		}

		// Ensure the transaction is not in the transaction pool.
		if harness.txPool.IsTransactionInPool(tx.Hash()) {
			t.Fatal("IsTransactionInPool: true for rejected orphan")
		}

		// Ensure the transaction is not reported as available.
		if harness.txPool.HaveTransaction(tx.Hash()) {
			t.Fatal("HaveTransaction: true for rejected orphan")
		}
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
			false, true)
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

		// Ensure the transaction is in the orphan pool.
		if !harness.txPool.IsOrphanInPool(tx.Hash()) {
			t.Fatal("IsOrphanInPool: false for accepted orphan")
		}

		// Ensure the transaction is not in the transaction pool.
		if harness.txPool.IsTransactionInPool(tx.Hash()) {
			t.Fatal("IsTransactionInPool: true for accepted orphan")
		}

		// Ensure the transaction is reported as available.
		if !harness.txPool.HaveTransaction(tx.Hash()) {
			t.Fatal("HaveTransaction: false for accepted orphan")
		}
	}

	// Figure out which transactions were evicted and make sure the number
	// evicted matches the expected number.
	var evictedTxns []*dcrutil.Tx
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

	// Ensure none of the evicted transactioned ended up the transaction
	// pool.
	for _, tx := range evictedTxns {
		if harness.txPool.IsTransactionInPool(tx.Hash()) {
			t.Fatalf("IsTransactionInPool: true for evicted orphan")
		}
	}
}
