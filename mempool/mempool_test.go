// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package mempool

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"reflect"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/chaingen"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainec"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

const (
	// singleInputTicketSize is the typical size of a normal P2PKH ticket
	// in bytes when the ticket has one input, rounded up.
	singleInputTicketSize int64 = 300
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
func txOutToSpendableOut(tx *dcrutil.Tx, outputNum uint32, tree int8) spendableOutput {
	return spendableOutput{
		outPoint: wire.OutPoint{Hash: *tx.Hash(), Index: outputNum, Tree: tree},
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
	signKey     *secp256k1.PrivateKey
	payAddr     dcrutil.Address
	payScript   []byte
	chainParams *chaincfg.Params

	chain  *fakeChain
	txPool *TxPool
}

// GetScript is the pool harness' implementation of the ScriptDB interface.
// It returns the pool harness' payment redeen script for any address
// passed in.
func (p *poolHarness) GetScript(addr dcrutil.Address) ([]byte, error) {
	return p.payScript, nil
}

// GetKey is the pool harness' implementation of the KeyDB interface.
// It returns the pool harness' signature key for any address passed in.
func (p *poolHarness) GetKey(addr dcrutil.Address) (chainec.PrivateKey, bool, error) {
	return p.signKey, true, nil
}

// AddFakeUTXO creates a fake mined uxto for the provided transaction.
func (p *poolHarness) AddFakeUTXO(tx *dcrutil.Tx, blockHeight int64) {
	p.chain.utxos.AddTxOuts(tx, blockHeight, wire.NullBlockIndex)
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
func (p *poolHarness) CreateSignedTx(inputs []spendableOutput, numOutputs uint32, expiry uint32) (*dcrutil.Tx, error) {
	// Calculate the total input amount and split it amongst the requested
	// number of outputs.
	var totalInput dcrutil.Amount
	for _, input := range inputs {
		totalInput += input.amount
	}
	amountPerOutput := int64(totalInput) / int64(numOutputs)
	remainder := int64(totalInput) - amountPerOutput*int64(numOutputs)

	tx := wire.NewMsgTx()
	tx.Expiry = expiry
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
			ValueIn:          int64(spendableAmount),
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

// CreateTx creates a zero-fee regular transaction from the provided spendable
// output.
func (p *poolHarness) CreateTx(out spendableOutput) (*dcrutil.Tx, error) {
	txns, err := p.CreateTxChain(out, 1)
	if err != nil {
		return nil, err
	}
	return txns[0], err
}

// CreateTicketPurchase creates a ticket purchase spending the first output of
// the provided transaction.
func (p *poolHarness) CreateTicketPurchase(sourceTx *dcrutil.Tx, cost int64) (*dcrutil.Tx, error) {
	ticketfee := dcrutil.Amount(singleInputTicketSize)
	ticketPrice := dcrutil.Amount(cost)

	// Generate the p2sh, commitment and change scripts of the ticket.
	pkScript, err := txscript.PayToSStx(p.payAddr)
	if err != nil {
		return nil, err
	}
	commitScript := chaingen.PurchaseCommitmentScript(p.payAddr,
		ticketPrice+ticketfee, 0, ticketPrice)
	change := dcrutil.Amount(sourceTx.MsgTx().TxOut[0].Value) -
		ticketPrice - ticketfee
	changeScript, err := txscript.PayToSStxChange(p.payAddr)
	if err != nil {
		return nil, err
	}

	// Generate the ticket purchase.
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  *sourceTx.Hash(),
			Index: 0,
			Tree:  wire.TxTreeRegular,
		},
		Sequence:    wire.MaxTxInSequenceNum,
		ValueIn:     sourceTx.MsgTx().TxOut[0].Value,
		BlockHeight: uint32(p.chain.BestHeight()),
	})

	tx.AddTxOut(wire.NewTxOut(int64(ticketPrice), pkScript))
	tx.AddTxOut(wire.NewTxOut(0, commitScript))
	tx.AddTxOut(wire.NewTxOut(int64(change), changeScript))

	// Sign the ticket purchase.
	sigScript, err := txscript.SignatureScript(tx, 0,
		sourceTx.MsgTx().TxOut[0].PkScript, txscript.SigHashAll, p.signKey, true)
	if err != nil {
		return nil, err
	}
	tx.TxIn[0].SignatureScript = sigScript

	return dcrutil.NewTx(tx), nil
}

// newVoteScript generates a voting script from the passed VoteBits, for
// use in a vote.
func newVoteScript(voteBits stake.VoteBits) ([]byte, error) {
	b := make([]byte, 2+len(voteBits.ExtendedBits))
	binary.LittleEndian.PutUint16(b[0:2], voteBits.Bits)
	copy(b[2:], voteBits.ExtendedBits[:])
	return txscript.GenerateProvablyPruneableOut(b)
}

// CreateVote creates a vote transaction using the provided ticket.
func (p *poolHarness) CreateVote(ticket *dcrutil.Tx) (*dcrutil.Tx, error) {
	// Calculate the vote subsidy
	subsidy := blockchain.CalcStakeVoteSubsidy(p.txPool.cfg.SubsidyCache,
		p.chain.BestHeight(), p.chainParams)
	// Parse the ticket purchase transaction and generate the vote reward.
	ticketPayKinds, ticketHash160s, ticketValues, _, _, _ :=
		stake.TxSStxStakeOutputInfo(ticket.MsgTx())
	voteRewardValues := stake.CalculateRewards(ticketValues,
		ticket.MsgTx().TxOut[0].Value, subsidy)

	// Add the stakebase input.
	vote := wire.NewMsgTx()
	stakebaseOutPoint := wire.NewOutPoint(&chainhash.Hash{}, ^uint32(0),
		wire.TxTreeRegular)
	stakebaseInput := wire.NewTxIn(stakebaseOutPoint, subsidy, nil)
	vote.AddTxIn(stakebaseInput)

	// Add the ticket input.
	spendOut := txOutToSpendableOut(ticket, 0, wire.TxTreeStake)
	ticketInput := wire.NewTxIn(&spendOut.outPoint, int64(spendOut.amount), nil)
	ticketInput.BlockHeight = uint32(p.chain.BestHeight())
	ticketInput.BlockIndex = 5
	vote.AddTxIn(ticketInput)

	// Add the block reference output.
	blockRefScript, _ := txscript.GenerateSSGenBlockRef(chainhash.Hash{},
		uint32(p.chain.BestHeight()))
	vote.AddTxOut(wire.NewTxOut(0, blockRefScript))

	// Create the vote script.
	voteBits := stake.VoteBits{Bits: uint16(0xff), ExtendedBits: []byte{}}
	voteScript, err := newVoteScript(voteBits)
	if err != nil {
		return nil, err
	}
	vote.AddTxOut(wire.NewTxOut(0, voteScript))

	// Create P2SH scripts for the ticket outputs.
	for i, hash160 := range ticketHash160s {
		scriptFn := txscript.PayToSSGenPKHDirect
		if ticketPayKinds[i] { // P2SH
			scriptFn = txscript.PayToSSGenSHDirect
		}
		// Error is checking for a nil hash160, just ignore it.
		script, _ := scriptFn(hash160)
		vote.AddTxOut(wire.NewTxOut(voteRewardValues[i], script))
	}

	// Sign the input.
	inputToSign := 1
	redeemTicketScript := ticket.MsgTx().TxOut[0].PkScript
	signedScript, err := txscript.SignTxOutput(p.chainParams, vote, inputToSign,
		redeemTicketScript, txscript.SigHashAll, p,
		p, vote.TxIn[inputToSign].SignatureScript, dcrec.STEcdsaSecp256k1)
	if err != nil {
		return nil, err
	}

	vote.TxIn[0].SignatureScript = p.chainParams.StakeBaseSigScript
	vote.TxIn[1].SignatureScript = signedScript

	return dcrutil.NewTx(vote), nil
}

// CreateRevocation creates a revocation using the provided ticket.
func (p *poolHarness) CreateRevocation(ticket *dcrutil.Tx) (*dcrutil.Tx, error) {
	ticketPurchase := ticket.MsgTx()
	ticketHash := ticketPurchase.TxHash()

	// Parse the ticket purchase transaction and generate the revocation value.
	ticketPayKinds, ticketHash160s, ticketValues, _, _, _ :=
		stake.TxSStxStakeOutputInfo(ticketPurchase)
	revocationValues := stake.CalculateRewards(ticketValues,
		ticketPurchase.TxOut[0].Value, 0)

	// Add the ticket input.
	revocation := wire.NewMsgTx()
	ticketOutPoint := wire.NewOutPoint(&ticketHash, 0, wire.TxTreeStake)
	ticketInput := wire.NewTxIn(ticketOutPoint,
		ticketPurchase.TxOut[ticketOutPoint.Index].Value, nil)
	revocation.AddTxIn(ticketInput)

	// All remaining outputs pay to the output destinations and amounts tagged
	// by the ticket purchase.
	for i, hash160 := range ticketHash160s {
		scriptFn := txscript.PayToSSRtxPKHDirect
		if ticketPayKinds[i] { // P2SH
			scriptFn = txscript.PayToSSRtxSHDirect
		}
		// Error is checking for a nil hash160, just ignore it.
		script, _ := scriptFn(hash160)
		revocation.AddTxOut(wire.NewTxOut(revocationValues[i], script))
	}

	// Sign the input.
	inputToSign := 0
	redeemTicketScript := ticket.MsgTx().TxOut[0].PkScript
	signedScript, err := txscript.SignTxOutput(p.chainParams, revocation, inputToSign,
		redeemTicketScript, txscript.SigHashAll, p,
		p, revocation.TxIn[inputToSign].SignatureScript, dcrec.STEcdsaSecp256k1)
	if err != nil {
		return nil, err
	}

	revocation.TxIn[0].SignatureScript = signedScript

	return dcrutil.NewTx(revocation), nil
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
		outputs = append(outputs, txOutToSpendableOut(coinbase, i, wire.TxTreeRegular))
	}
	harness.chain.SetHeight(int64(chainParams.CoinbaseMaturity) + curHeight)
	harness.chain.SetPastMedianTime(time.Now())

	return &harness, outputs, nil
}

// testContext houses a test-related state that is useful to pass to helper
// functions as a single argument.
type testContext struct {
	t       *testing.T
	harness *poolHarness
}

// testPoolMembership tests the transaction pool associated with the provided
// test context to determine if the passed transaction matches the provided
// orphan pool and transaction pool status.  It also further determines if it
// should be reported as available by the HaveTransaction function based upon
// the two flags and tests that condition as well.
func testPoolMembership(tc *testContext, tx *dcrutil.Tx, inOrphanPool, inTxPool bool) {
	txHash := tx.Hash()
	gotOrphanPool := tc.harness.txPool.IsOrphanInPool(txHash)
	if inOrphanPool != gotOrphanPool {
		_, file, line, _ := runtime.Caller(1)
		tc.t.Fatalf("%s:%d -- IsOrphanInPool: want %v, got %v", file,
			line, inOrphanPool, gotOrphanPool)
	}

	gotTxPool := tc.harness.txPool.IsTransactionInPool(txHash)
	if inTxPool != gotTxPool {
		_, file, line, _ := runtime.Caller(1)
		tc.t.Fatalf("%s:%d -- IsTransactionInPool: want %v, got %v",
			file, line, inTxPool, gotTxPool)
	}

	gotHaveTx := tc.harness.txPool.HaveTransaction(txHash)
	wantHaveTx := inOrphanPool || inTxPool
	if wantHaveTx != gotHaveTx {
		_, file, line, _ := runtime.Caller(1)
		tc.t.Fatalf("%s:%d -- HaveTransaction: want %v, got %v", file,
			line, wantHaveTx, gotHaveTx)
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

		// Ensure the transaction is in the orphan pool, is not in the
		// transaction pool, and is reported as available.
		testPoolMembership(tc, tx, true, false)
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
		// Ensure the transaction is no longer in the orphan pool, is
		// now in the transaction pool, and is reported as available.
		testPoolMembership(tc, tx, false, true)
	}
}

// TestTicketPurchaseOrphan ensures that ticket purchases are orphaned when
// referenced outputs spent are from missing transactions.
func TestTicketPurchaseOrphan(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a regular transaction from the first spendable output
	// provided by the harness.
	tx, err := harness.CreateTx(spendableOuts[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Create a ticket purchase transaction spending the outputs of the
	// prior regular transaction.
	ticket, err := harness.CreateTicketPurchase(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction %v", err)
	}

	// Ensure the ticket purchase is accepted as an orphan.
	acceptedTxns, err := harness.txPool.ProcessTransaction(ticket, true,
		false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid orphan %v", err)
	}
	testPoolMembership(tc, ticket, true, false)

	if len(acceptedTxns) > 0 {
		t.Fatalf("ProcessTransaction: expected zero accepted transactions "+
			"got %v", len(acceptedTxns))
	}

	// Add the regular transaction whose outputs are spent by the ticket purchase
	// and ensure they all get accepted.  Notice the accept orphans flag is also
	// false here to ensure it has no bearing on whether or not already existing
	// orphans in the pool are linked.
	_, err = harness.txPool.ProcessTransaction(tx, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)
	testPoolMembership(tc, ticket, false, true)
}

// TestVoteOrphan ensures that votes are orphaned when referenced outputs
// spent are from missing transactions.
func TestVoteOrphan(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a regular transaction from the first spendable output
	// provided by the harness.
	tx, err := harness.CreateTx(spendableOuts[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Create a ticket purchase transaction spending the outputs of the
	// prior regular transaction.
	ticket, err := harness.CreateTicketPurchase(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction: %v", err)
	}

	harness.chain.SetHeight(harness.chainParams.StakeValidationHeight)

	vote, err := harness.CreateVote(ticket)
	if err != nil {
		t.Fatalf("unable to create vote: %v", err)
	}

	// Ensure the vote is rejected because it is an orphan.
	_, err = harness.txPool.ProcessTransaction(vote, false, false, true)
	if err == nil {
		t.Fatalf("ProcessTransaction: accepted transaction references " +
			"outputs of unknown or fully-spent transaction")
	}
	testPoolMembership(tc, vote, false, false)

	// Ensure the ticket is accepted as an orphan.
	_, err = harness.txPool.ProcessTransaction(ticket, true, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, ticket, true, false)

	// Ensure the regular tx whose ouput is spent by the ticket is accepted.
	_, err = harness.txPool.ProcessTransaction(tx, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)

	// Generate a fake mined utxo for the ticket created.
	harness.AddFakeUTXO(ticket, int64(ticket.MsgTx().TxIn[0].BlockHeight))

	// Ensure the previously rejected vote is accepted now since all referenced
	// utxos can now be found.
	_, err = harness.txPool.ProcessTransaction(vote, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept orphan transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)
	testPoolMembership(tc, ticket, false, true)
	testPoolMembership(tc, vote, false, true)
}

// TestRevocationOrphan ensures that revocations are orphaned when
// referenced outputs spent are from missing transactions.
func TestRevocationOrphan(t *testing.T) {
	t.Parallel()

	harness, spendableOuts, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create a regular transaction from the first spendable output
	// provided by the harness.
	tx, err := harness.CreateTx(spendableOuts[0])
	if err != nil {
		t.Fatalf("unable to create transaction: %v", err)
	}

	// Create a ticket purchase transaction spending the outputs of the
	// prior regular transaction.
	ticket, err := harness.CreateTicketPurchase(tx, 40000)
	if err != nil {
		t.Fatalf("unable to create ticket purchase transaction: %v", err)
	}

	harness.chain.SetHeight(harness.chainParams.StakeValidationHeight + 1)

	revocation, err := harness.CreateRevocation(ticket)
	if err != nil {
		t.Fatalf("unable to create revocation: %v", err)
	}

	// Ensure the vote is rejected because it is an orphan.
	_, err = harness.txPool.ProcessTransaction(revocation, false, false, true)
	if err == nil {
		t.Fatalf("ProcessTransaction: accepted transaction references " +
			"outputs of unknown or fully-spent transaction")
	}
	testPoolMembership(tc, revocation, false, false)

	// Ensure the ticket is accepted as an orphan.
	_, err = harness.txPool.ProcessTransaction(ticket, true, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, ticket, true, false)

	// Ensure the regular tx whose ouput is spent by the ticket is accepted.
	_, err = harness.txPool.ProcessTransaction(tx, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)

	// Generate a fake mined utxos for the ticket created.
	harness.AddFakeUTXO(ticket, int64(ticket.MsgTx().TxIn[0].BlockHeight))

	// Ensure the previously rejected revocation is accepted now since all referenced
	// utxos can now be found.
	_, err = harness.txPool.ProcessTransaction(revocation, false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept orphan transaction %v",
			err)
	}
	testPoolMembership(tc, tx, false, true)
	testPoolMembership(tc, ticket, false, true)
	testPoolMembership(tc, revocation, false, true)
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

		// Ensure the transaction is in the orphan pool, is not in the
		// transaction pool, and is reported as available.
		testPoolMembership(tc, tx, true, false)
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

	// Ensure none of the evicted transactions ended up in the transaction
	// pool.
	for _, tx := range evictedTxns {
		testPoolMembership(tc, tx, false, false)
	}
}

// TestExpirationPruning ensures that transactions that expire without being
// mined are removed.
func TestExpirationPruning(t *testing.T) {
	harness, outputs, err := newPoolHarness(&chaincfg.MainNetParams)
	if err != nil {
		t.Fatalf("unable to create test pool: %v", err)
	}
	tc := &testContext{t, harness}

	// Create and add a transaction with several outputs that spends the first
	// spendable output provided by the harness and ensure it is not the orphan
	// pool, is in the transaction pool, and is reported as available.
	//
	// These outputs will be used as inputs to transactions with expirations.
	const numTxns = 5
	multiOutputTx, err := harness.CreateSignedTx([]spendableOutput{outputs[0]},
		numTxns, wire.NoExpiryValue)
	if err != nil {
		t.Fatalf("unable to create signed tx: %v", err)
	}
	acceptedTxns, err := harness.txPool.ProcessTransaction(multiOutputTx,
		true, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid tx: %v", err)
	}
	if len(acceptedTxns) != 1 {
		t.Fatalf("ProcessTransaction: reported %d accepted transactions from "+
			"what should be 1", len(acceptedTxns))
	}
	testPoolMembership(tc, multiOutputTx, false, true)

	// Create several transactions such that each transaction has an expiration
	// one block after the previous and the first one expires in the block after
	// the next one.
	nextBlockHeight := harness.chain.BestHeight() + 1
	expiringTxns := make([]*dcrutil.Tx, 0, numTxns)
	for i := 0; i < numTxns; i++ {
		tx, err := harness.CreateSignedTx([]spendableOutput{
			txOutToSpendableOut(multiOutputTx, uint32(i), wire.TxTreeRegular),
		}, 1, uint32(nextBlockHeight+int64(i)+1))
		if err != nil {
			t.Fatalf("unable to create signed tx: %v", err)
		}
		expiringTxns = append(expiringTxns, tx)
	}

	// Ensure expiration pruning is working properly by adding each expiring
	// transaction just before the point at which it will expire and advancing
	// the chain so that the transaction becomes expired and thus should be
	// pruned.
	for _, tx := range expiringTxns {
		acceptedTxns, err := harness.txPool.ProcessTransaction(tx, true, false,
			true)
		if err != nil {
			t.Fatalf("ProcessTransaction: failed to accept valid tx: %v", err)
		}

		// Ensure the transaction was reported as accepted, is not in the orphan
		// pool, is in the transaction pool, and is reported as available.
		if len(acceptedTxns) != 1 {
			t.Fatalf("ProcessTransaction: reported %d accepted transactions "+
				"from what should be 1", len(acceptedTxns))
		}
		testPoolMembership(tc, tx, false, true)

		// Simulate processing a new block that did not mine any of the txns.
		harness.chain.SetHeight(harness.chain.BestHeight() + 1)

		// Prune any transactions that are now expired and ensure that the tx
		// that was just added was pruned by checking that it is not in the
		// orphan pool, not in the transaction pool, and not reported as
		// available.
		harness.txPool.PruneExpiredTx()
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

		// Ensure the transaction is in the orphan pool, not in the
		// transaction pool, and reported as available.
		testPoolMembership(tc, tx, true, false)
	}

	// Attempt to remove an orphan that has no redeemers and is not present,
	// and ensure the state of all other orphans are unaffected.
	nonChainedOrphanTx, err := harness.CreateSignedTx([]spendableOutput{{
		amount:   dcrutil.Amount(5000000000),
		outPoint: wire.OutPoint{Hash: chainhash.Hash{}, Index: 0},
	}}, 1, uint32(harness.chain.BestHeight()+1))
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
			false, true)
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
		txOutToSpendableOut(chainedTxns[1], 0, wire.TxTreeRegular),
		txOutToSpendableOut(chainedTxns[maxOrphans], 0, wire.TxTreeRegular),
	}, 1, wire.NoExpiryValue)
	if err != nil {
		t.Fatalf("unable to create signed tx: %v", err)
	}
	acceptedTxns, err := harness.txPool.ProcessTransaction(doubleSpendTx,
		true, false, true)
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
		false, false, true)
	if err != nil {
		t.Fatalf("ProcessTransaction: failed to accept valid tx %v", err)
	}
	if len(acceptedTxns) != maxOrphans {
		t.Fatalf("ProcessTransaction: reported accepted transactions "+
			"length does not match expected -- got %d, want %d",
			len(acceptedTxns), maxOrphans)
	}
	for _, tx := range acceptedTxns {
		// Ensure the transaction is no longer in the orphan pool, is
		// in the transaction pool, and is reported as available.
		testPoolMembership(tc, tx, false, true)
	}

	// Ensure the double spending orphan is no longer in the orphan pool and
	// was not moved to the transaction pool.
	testPoolMembership(tc, doubleSpendTx, false, false)
}
