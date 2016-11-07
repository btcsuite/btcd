// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package fullblocktests

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"
	"runtime"
	"sort"
	"time"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainec"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

const (
	// Intentionally defined here rather than using constants from codebase
	// to ensure consensus changes are detected.
	maxBlockSigOps       = 5000
	minCoinbaseScriptLen = 2
	maxCoinbaseScriptLen = 100
	medianTimeBlocks     = 11
	maxScriptElementSize = 2048
)

var (
	// hash256prngSeedConst is a constant derived from the hex
	// representation of pi and is used in conjuction with a caller-provided
	// seed when initializing the deterministic lottery prng.
	hash256prngSeedConst = []byte{0x24, 0x3f, 0x6a, 0x88, 0x85, 0xa3, 0x08,
		0xd3}

	// opTrueScript is a simple public key script that contains the OP_TRUE
	// opcode.  It is defined here to reduce garbage creation.
	opTrueScript = []byte{txscript.OP_TRUE}

	// opTrueRedeemScript is the signature script that can be used to redeem
	// a p2sh output to the opTrueScript.  It is defined here to reduce
	// garbage creation.
	opTrueRedeemScript = []byte{txscript.OP_DATA_1, txscript.OP_TRUE}

	// coinbaseSigScript is the signature script used by the tests when
	// creating standard coinbase transactions.  It is defined here to
	// reduce garbage creation.
	coinbaseSigScript = []byte{txscript.OP_0, txscript.OP_0}

	// lowFee is a single atom and exists to make the test code more
	// readable.
	lowFee = dcrutil.Amount(1)
)

// TestInstance is an interface that describes a specific test instance returned
// by the tests generated in this package.  It should be type asserted to one
// of the concrete test instance types in order to test accordingly.
type TestInstance interface {
	FullBlockTestInstance()
}

// AcceptedBlock defines a test instance that expects a block to be accepted to
// the blockchain either by extending the main chain, on a side chain, or as an
// orphan.
type AcceptedBlock struct {
	Name        string
	Block       *wire.MsgBlock
	IsMainChain bool
	IsOrphan    bool
}

// Ensure AcceptedBlock implements the TestInstance interface.
var _ TestInstance = AcceptedBlock{}

// FullBlockTestInstance only exists to allow the AcceptedBlock to be treated
// as a TestInstance.
//
// This implements the TestInstance interface.
func (b AcceptedBlock) FullBlockTestInstance() {}

// RejectedBlock defines a test instance that expects a block to be rejected by
// the blockchain consensus rules.
type RejectedBlock struct {
	Name       string
	Block      *wire.MsgBlock
	RejectCode blockchain.ErrorCode
}

// Ensure RejectedBlock implements the TestInstance interface.
var _ TestInstance = RejectedBlock{}

// FullBlockTestInstance only exists to allow the RejectedBlock to be treated
// as a TestInstance.
//
// This implements the TestInstance interface.
func (b RejectedBlock) FullBlockTestInstance() {}

// OrphanOrRejectedBlock defines a test instance that expects a block to either
// be accepted as an orphan or rejected.  This is useful since some
// implementations might optimize the immediate rejection of orphan blocks when
// their parent was previously rejected, while others might accept it as an
// orphan that eventually gets flushed (since the parent can never be accepted
// to ultimately link it).
type OrphanOrRejectedBlock struct {
	Name  string
	Block *wire.MsgBlock
}

// Ensure ExpectedTip implements the TestInstance interface.
var _ TestInstance = OrphanOrRejectedBlock{}

// FullBlockTestInstance only exists to allow OrphanOrRejectedBlock to be
// treated as a TestInstance.
//
// This implements the TestInstance interface.
func (b OrphanOrRejectedBlock) FullBlockTestInstance() {}

// ExpectedTip defines a test instance that expects a block to be the current
// tip of the main chain.
type ExpectedTip struct {
	Name  string
	Block *wire.MsgBlock
}

// Ensure ExpectedTip implements the TestInstance interface.
var _ TestInstance = ExpectedTip{}

// FullBlockTestInstance only exists to allow the ExpectedTip to be treated
// as a TestInstance.
//
// This implements the TestInstance interface.
func (b ExpectedTip) FullBlockTestInstance() {}

// spendableOut represents a transaction output that is spendable along with
// additional metadata such as the block its in and how much it pays.
type spendableOut struct {
	prevOut     wire.OutPoint
	blockHeight uint32
	blockIndex  uint32
	amount      dcrutil.Amount
}

// makeSpendableOutForTx returns a spendable output for the given transaction
// block height, transaction index within the block, and transaction output
// index within the transaction.
func makeSpendableOutForTx(tx *wire.MsgTx, blockHeight, txIndex, txOutIndex uint32) spendableOut {
	return spendableOut{
		prevOut: wire.OutPoint{
			Hash:  tx.TxSha(),
			Index: txOutIndex,
			Tree:  dcrutil.TxTreeRegular,
		},
		blockHeight: blockHeight,
		blockIndex:  txIndex,
		amount:      dcrutil.Amount(tx.TxOut[txOutIndex].Value),
	}
}

// makeSpendableOut returns a spendable output for the given block, transaction
// index within the block, and transaction output index within the transaction.
func makeSpendableOut(block *wire.MsgBlock, txIndex, txOutIndex uint32) spendableOut {
	tx := block.Transactions[txIndex]
	return makeSpendableOutForTx(tx, block.Header.Height, txIndex, txOutIndex)
}

// stakeTicket represents a transaction that is an sstx along with the height of
// the block it was mined in and the its index within that block.
type stakeTicket struct {
	tx          *wire.MsgTx
	blockHeight uint32
	blockIndex  uint32
}

// stakeTicketSorter implements sort.Interface to allow a slice of stake tickets
// to be sorted.
type stakeTicketSorter []*stakeTicket

// Len returns the number of stake tickets in the slice.  It is part of the
// sort.Interface implementation.
func (t stakeTicketSorter) Len() int { return len(t) }

// Swap swaps the stake tickets at the passed indices.  It is part of the
// sort.Interface implementation.
func (t stakeTicketSorter) Swap(i, j int) { t[i], t[j] = t[j], t[i] }

// Less returns whether the stake ticket with index i should sort before the
// stake ticket with index j.  It is part of the sort.Interface implementation.
func (t stakeTicketSorter) Less(i, j int) bool {
	iHash := t[i].tx.CachedTxSha()[:]
	jHash := t[j].tx.CachedTxSha()[:]
	return bytes.Compare(iHash, jHash) < 0
}

// testGenerator houses state used to easy the process of generating test blocks
// that build from one another along with housing other useful things such as
// available spendable outputs and generic payment scripts used throughout the
// tests.
type testGenerator struct {
	params           *chaincfg.Params
	tip              *wire.MsgBlock
	tipName          string
	blocks           map[chainhash.Hash]*wire.MsgBlock
	blocksByName     map[string]*wire.MsgBlock
	p2shOpTrueAddr   dcrutil.Address
	p2shOpTrueScript []byte

	// Used for tracking spendable coinbase outputs.
	spendableOuts     [][]spendableOut
	prevCollectedHash chainhash.Hash
}

// makeTestGenerator returns a test generator instance initialized with the
// genesis block as the tip as well as a cached generic pay-to-script-script for
// OP_TRUE.
func makeTestGenerator(params *chaincfg.Params) (testGenerator, error) {
	// Generate a generic pay-to-script-hash script that is a simple
	// OP_TRUE.  This allows the tests to avoid needing to generate and
	// track actual public keys and signatures.
	p2shOpTrueAddr, err := dcrutil.NewAddressScriptHash(opTrueScript, params)
	if err != nil {
		return testGenerator{}, err
	}
	p2shOpTrueScript, err := txscript.PayToAddrScript(p2shOpTrueAddr)
	if err != nil {
		return testGenerator{}, err
	}

	genesis := params.GenesisBlock
	genesisHash := genesis.BlockSha()
	return testGenerator{
		params:           params,
		tip:              genesis,
		tipName:          "genesis",
		blocks:           map[chainhash.Hash]*wire.MsgBlock{genesisHash: genesis},
		blocksByName:     map[string]*wire.MsgBlock{"genesis": genesis},
		p2shOpTrueAddr:   p2shOpTrueAddr,
		p2shOpTrueScript: p2shOpTrueScript,
	}, nil
}

// opReturnScript returns a provably-pruneable OP_RETURN script with the
// provided data.
func opReturnScript(data []byte) []byte {
	builder := txscript.NewScriptBuilder()
	script, err := builder.AddOp(txscript.OP_RETURN).AddData(data).Script()
	if err != nil {
		panic(err)
	}
	return script
}

// uniqueOpReturnScript returns a standard provably-pruneable OP_RETURN script
// with a random uint64 encoded as the data.
func uniqueOpReturnScript() []byte {
	rand, err := wire.RandomUint64()
	if err != nil {
		panic(err)
	}

	data := make([]byte, 8, 8)
	binary.LittleEndian.PutUint64(data[0:8], rand)
	return opReturnScript(data)
}

// calcFullSubsidy returns the full block subsidy for the given block height.
//
// NOTE: This and the other subsidy calculation funcs intentionally are not
// using the blockchain code since the intent is to be able to generate known
// good tests which exercise that code, so it wouldn't make sense to use the
// same code to generate them.
func (g *testGenerator) calcFullSubsidy(blockHeight uint32) dcrutil.Amount {
	iterations := int64(blockHeight) / g.params.ReductionInterval
	subsidy := g.params.BaseSubsidy
	for i := int64(0); i < iterations; i++ {
		subsidy *= g.params.MulSubsidy
		subsidy /= g.params.DivSubsidy
	}
	return dcrutil.Amount(subsidy)
}

// calcPoWSubsidy returns the proof-of-work subsidy portion from a given full
// subsidy, block height, and number of votes that will be included in the
// block.
//
// NOTE: This and the other subsidy calculation funcs intentionally are not
// using the blockchain code since the intent is to be able to generate known
// good tests which exercise that code, so it wouldn't make sense to use the
// same code to generate them.
func (g *testGenerator) calcPoWSubsidy(fullSubsidy dcrutil.Amount, blockHeight uint32, numVotes uint16) dcrutil.Amount {
	powProportion := dcrutil.Amount(g.params.WorkRewardProportion)
	totalProportions := dcrutil.Amount(g.params.TotalSubsidyProportions())
	powSubsidy := (fullSubsidy * powProportion) / totalProportions
	if int64(blockHeight) < g.params.StakeValidationHeight {
		return powSubsidy
	}

	// Reduce the subsidy according to the number of votes.
	ticketsPerBlock := dcrutil.Amount(g.params.TicketsPerBlock)
	return (powSubsidy * dcrutil.Amount(numVotes)) / ticketsPerBlock
}

// calcPoSSubsidy returns the proof-of-stake subsidy portion from a given full
// subsidy and block height.
//
// NOTE: This and the other subsidy calculation funcs intentionally are not
// using the blockchain code since the intent is to be able to generate known
// good tests which exercise that code, so it wouldn't make sense to use the
// same code to generate them.
func (g *testGenerator) calcPoSSubsidy(fullSubsidy dcrutil.Amount, blockHeight uint32) dcrutil.Amount {
	if int64(blockHeight) < g.params.StakeValidationHeight {
		return 0
	}

	posProportion := dcrutil.Amount(g.params.StakeRewardProportion)
	totalProportions := dcrutil.Amount(g.params.TotalSubsidyProportions())
	return (fullSubsidy * posProportion) / totalProportions
}

// calcDevSubsidy returns the dev org subsidy portion from a given full subsidy.
//
// NOTE: This and the other subsidy calculation funcs intentionally are not
// using the blockchain code since the intent is to be able to generate known
// good tests which exercise that code, so it wouldn't make sense to use the
// same code to generate them.
func (g *testGenerator) calcDevSubsidy(fullSubsidy dcrutil.Amount, blockHeight uint32, numVotes uint16) dcrutil.Amount {
	devProportion := dcrutil.Amount(g.params.BlockTaxProportion)
	totalProportions := dcrutil.Amount(g.params.TotalSubsidyProportions())
	devSubsidy := (fullSubsidy * devProportion) / totalProportions
	if int64(blockHeight) < g.params.StakeValidationHeight {
		return devSubsidy
	}

	// Reduce the subsidy according to the number of votes.
	ticketsPerBlock := dcrutil.Amount(g.params.TicketsPerBlock)
	return (devSubsidy * dcrutil.Amount(numVotes)) / ticketsPerBlock
}

// standardCoinbaseOpReturnScript returns a standard script suitable for use as
// the second output of a standard coinbase transaction of a new block.  In
// particular, the serialized data used with the OP_RETURN starts with the block
// height and is followed by 32 bytes which are treated as 4 uint64 extra
// nonces.  This implementation puts a cryptographically random value into the
// final extra nonce position.  The actual format of the data after the block
// height is not defined however this effectivley mirrors the actual mining code
// at the time it was written.
func standardCoinbaseOpReturnScript(blockHeight uint32) []byte {
	rand, err := wire.RandomUint64()
	if err != nil {
		panic(err)
	}

	data := make([]byte, 36, 36)
	binary.LittleEndian.PutUint32(data[0:4], blockHeight)
	binary.LittleEndian.PutUint64(data[28:36], rand)
	return opReturnScript(data)
}

// addCoinbaseTxOutputs adds the following outputs to the provided transaction
// which is assumed to be a coinbase transaction:
// - First output pays the development subsidy portion to the dev org
// - Second output is a standard provably prunable data-only coinbase output
// - Third and subsequent outputs pay the pow subsidy portion to the generic
//   OP_TRUE p2sh script hash
func (g *testGenerator) addCoinbaseTxOutputs(tx *wire.MsgTx, blockHeight uint32, devSubsidy, powSubsidy dcrutil.Amount) {
	// First output is the developer subsidy.
	tx.AddTxOut(&wire.TxOut{
		Value:    int64(devSubsidy),
		Version:  g.params.OrganizationPkScriptVersion,
		PkScript: g.params.OrganizationPkScript,
	})

	// Second output is a provably prunable data-only output that is used
	// to ensure the coinbase is unique.
	tx.AddTxOut(wire.NewTxOut(0, standardCoinbaseOpReturnScript(blockHeight)))

	// Final outputs are the proof-of-work subsidy split into more than one
	// output.  These are in turn used througout the tests as inputs to
	// other transactions such as ticket purchases and additional spend
	// transactions.
	const numPoWOutputs = 6
	amount := powSubsidy / numPoWOutputs
	for i := 0; i < numPoWOutputs; i++ {
		if i == numPoWOutputs-1 {
			amount = powSubsidy - amount*(numPoWOutputs-1)
		}
		tx.AddTxOut(wire.NewTxOut(int64(amount), g.p2shOpTrueScript))
	}
}

// createCoinbaseTx returns a coinbase transaction paying an appropriate
// subsidy based on the passed block height and number of votes to the dev org
// and proof-of-work miner.
//
// See the createCoinbaseTxOutputs documentation for a breakdown of the outputs
// the transaction contains.
func (g *testGenerator) createCoinbaseTx(blockHeight uint32, numVotes uint16) *wire.MsgTx {
	// Calculate the subsidy proportions based on the block height and the
	// number of votes the block will include.
	fullSubsidy := g.calcFullSubsidy(blockHeight)
	devSubsidy := g.calcDevSubsidy(fullSubsidy, blockHeight, numVotes)
	powSubsidy := g.calcPoWSubsidy(fullSubsidy, blockHeight, numVotes)

	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, dcrutil.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(devSubsidy + powSubsidy),
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: coinbaseSigScript,
	})

	g.addCoinbaseTxOutputs(tx, blockHeight, devSubsidy, powSubsidy)

	return tx
}

// purchaseCommitmentScript returns a standard provably-pruneable OP_RETURN
// commitment script suitable for use in a ticket purchase tx (sstx) using the
// provided target address, amount, and fee limits.
func purchaseCommitmentScript(addr dcrutil.Address, amount, voteFeeLimit, revocationFeeLimit dcrutil.Amount) []byte {
	// The limits are defined in terms of the closest base 2 exponent and
	// a bit that must be set to specify the limit is to be applied.  The
	// vote fee exponent is in the bottom 8 bits, while the revocation fee
	// exponent is in the upper 8 bits.
	limits := uint16(0)
	if voteFeeLimit != 0 {
		exp := uint16(math.Ceil(math.Log2(float64(voteFeeLimit))))
		limits |= (exp | 0x40)
	}
	if revocationFeeLimit != 0 {
		exp := uint16(math.Ceil(math.Log2(float64(revocationFeeLimit))))
		limits |= ((exp | 0x40) << 8)
	}

	// The data consists of the 20-byte raw script address for the given
	// address, 8 bytes for the amount to commit to (with the upper bit flag
	// set to indicate a pay-to-script-hash address), and 2 bytes for the
	// fee limits.
	var data [30]byte
	copy(data[:], addr.ScriptAddress())
	binary.LittleEndian.PutUint64(data[20:], uint64(amount))
	data[27] |= 1 << 7
	binary.LittleEndian.PutUint16(data[28:], uint16(limits))
	script, err := txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).
		AddData(data[:]).Script()
	if err != nil {
		panic(err)
	}
	return script
}

// createTicketPurchaseTx creates a new transaction that spends the provided
// output to purchase a stake submission ticket (sstx) at the given ticket
// price.  Both the ticket and the change will go to a p2sh script that is
// composed with a single OP_TRUE.
//
// The transaction consists of the following outputs:
// - First output is an OP_SSTX followed by the OP_TRUE p2sh script hash
// - Second output is an OP_RETURN followed by the commitment script
// - Third output is an OP_SSTXCHANGE followed by the OP_TRUE p2sh script hash
func (g *testGenerator) createTicketPurchaseTx(spend *spendableOut, ticketPrice, fee dcrutil.Amount) *wire.MsgTx {
	// The first output is the voting rights address.  This impl uses the
	// standard pay-to-script-hash to an OP_TRUE.
	pkScript, err := txscript.PayToSStx(g.p2shOpTrueAddr)
	if err != nil {
		panic(err)
	}

	// Generate the commitment script.
	commitScript := purchaseCommitmentScript(g.p2shOpTrueAddr,
		ticketPrice+fee, 0, ticketPrice)

	// Calculate change and generate script to deliver it.
	change := spend.amount - ticketPrice - fee
	changeScript, err := txscript.PayToSStxChange(g.p2shOpTrueAddr)
	if err != nil {
		panic(err)
	}

	// Generate and return the transaction spending from the provided
	// spendable output with the previously described outputs.
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: spend.prevOut,
		Sequence:         wire.MaxTxInSequenceNum,
		ValueIn:          int64(spend.amount),
		BlockHeight:      spend.blockHeight,
		BlockIndex:       spend.blockIndex,
		SignatureScript:  opTrueRedeemScript,
	})
	tx.AddTxOut(wire.NewTxOut(int64(ticketPrice), pkScript))
	tx.AddTxOut(wire.NewTxOut(0, commitScript))
	tx.AddTxOut(wire.NewTxOut(int64(change), changeScript))
	return tx
}

// isTicketPurchaseTx returns whether or not the passed transaction is a stake
// ticket purchase.
//
// NOTE: Like many other functions in this test code, this function
// intentionally does not use the blockchain/stake package code since the intent
// is to be able to generate known good tests which exercise that code, so it
// wouldn't make sense to use the same code to generate them.  It must also be
// noted that this function is NOT robust.  It is the minimum necessary needed
// by the testing framework.
func isTicketPurchaseTx(tx *wire.MsgTx) bool {
	if len(tx.TxOut) == 0 {
		return false
	}
	txOut := tx.TxOut[0]
	scriptClass := txscript.GetScriptClass(txOut.Version, txOut.PkScript)
	return scriptClass == txscript.StakeSubmissionTy
}

// isVoteTx returns whether or not the passed tx is a stake vote (ssgen).
//
// NOTE: Like many other functions in this test code, this function
// intentionally does not use the blockchain/stake package code since the intent
// is to be able to generate known good tests which exercise that code, so it
// wouldn't make sense to use the same code to generate them.  It must also be
// noted that this function is NOT robust.  It is the minimum necessary needed
// by the testing framework.
func isVoteTx(tx *wire.MsgTx) bool {
	if len(tx.TxOut) < 3 {
		return false
	}
	txOut := tx.TxOut[2]
	scriptClass := txscript.GetScriptClass(txOut.Version, txOut.PkScript)
	return scriptClass == txscript.StakeGenTy
}

// isRevocationTx returns whether or not the passed tx is a stake ticket
// revocation (ssrtx).
//
// NOTE: Like many other functions in this test code, this function
// intentionally does not use the blockchain/stake package code since the intent
// is to be able to generate known good tests which exercise that code, so it
// wouldn't make sense to use the same code to generate them.  It must also be
// noted that this function is NOT robust.  It is the minimum necessary needed
// by the testing framework.
func isRevocationTx(tx *wire.MsgTx) bool {
	if len(tx.TxOut) == 0 {
		return false
	}
	txOut := tx.TxOut[0]
	scriptClass := txscript.GetScriptClass(txOut.Version, txOut.PkScript)
	return scriptClass == txscript.StakeRevocationTy
}

// voteBlockScript returns a standard provably-pruneable OP_RETURN script
// suitable for use in a vote tx (ssgen) given the block to vote on.
func voteBlockScript(parentBlock *wire.MsgBlock) []byte {
	var data [36]byte
	parentHash := parentBlock.BlockSha()
	copy(data[:], parentHash[:])
	binary.LittleEndian.PutUint32(data[32:], parentBlock.Header.Height)
	script, err := txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).
		AddData(data[:]).Script()
	if err != nil {
		panic(err)
	}
	return script
}

// voteBitsScript returns a standard provably-pruneable OP_RETURN script
// suitable for use in a vote tx (ssgen) with the appropriate vote bits set
// depending on the provided param.
func voteBitsScript(voteYes bool) []byte {
	bits := uint16(0)
	if voteYes {
		bits = 1
	}

	var data [2]byte
	binary.LittleEndian.PutUint16(data[:], bits)
	script, err := txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).
		AddData(data[:]).Script()
	if err != nil {
		panic(err)
	}
	return script
}

// createVoteTx returns a new transaction (ssgen) paying an appropriate subsidy
// for the given block height (and the number of votes per block) as well as the
// original commitments.
//
// The transaction consists of the following outputs:
// - First output is an OP_RETURN followed by the block hash and height
// - Second output is an OP_RETURN followed by the vote bits
// - Third and subsequent outputs are the payouts according to the ticket
//   commitments and the appropriate proportion of the vote subsidy.
func (g *testGenerator) createVoteTx(parentBlock *wire.MsgBlock, ticket *stakeTicket) *wire.MsgTx {
	// Calculate the proof-of-stake subsidy proportion based on the block
	// height.
	blockHeight := parentBlock.Header.Height + 1
	fullSubsidy := g.calcFullSubsidy(blockHeight)
	posSubsidy := g.calcPoSSubsidy(fullSubsidy, blockHeight)
	voteSubsidy := posSubsidy / dcrutil.Amount(g.params.TicketsPerBlock)
	ticketPrice := dcrutil.Amount(ticket.tx.TxOut[0].Value)

	// The first output is the block (hash and height) the vote is for.
	blockScript := voteBlockScript(parentBlock)

	// The second output is the vote bits.
	voteScript := voteBitsScript(true)

	// The third and subsequent outputs pay the original commitment amounts
	// along with the appropriate portion of the vote subsidy.  This impl
	// uses the standard pay-to-script-hash to an OP_TRUE.
	stakeGenScript, err := txscript.PayToSSGen(g.p2shOpTrueAddr)
	if err != nil {
		panic(err)
	}

	// Generate and return the transaction with the proof-of-stake subsidy
	// coinbase and spending from the provided ticket along with the
	// previously described outputs.
	ticketHash := ticket.tx.TxSha()
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, dcrutil.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(voteSubsidy),
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: g.params.StakeBaseSigScript,
	})
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&ticketHash, 0,
			dcrutil.TxTreeStake),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(ticketPrice),
		BlockHeight:     ticket.blockHeight,
		BlockIndex:      ticket.blockIndex,
		SignatureScript: opTrueRedeemScript,
	})
	tx.AddTxOut(wire.NewTxOut(0, blockScript))
	tx.AddTxOut(wire.NewTxOut(0, voteScript))
	tx.AddTxOut(wire.NewTxOut(int64(voteSubsidy+ticketPrice), stakeGenScript))
	return tx
}

// ancestorBlock returns the ancestor block at the provided height by following
// the chain backwards from the given block.  The returned block will be nil
// when a height is requested that is after the height of the passed block.
// Also, a callback can optionally be provided that is invoked with each block
// as it traverses.
func (g *testGenerator) ancestorBlock(block *wire.MsgBlock, height uint32, f func(*wire.MsgBlock)) *wire.MsgBlock {
	// Nothing to do if the requested height is outside of the valid
	// range.
	if block == nil || height > block.Header.Height {
		return nil
	}

	// Iterate backwards until the requested height is reached.
	for block != nil && block.Header.Height > height {
		block = g.blocks[block.Header.PrevBlock]
		if f != nil && block != nil {
			f(block)
		}
	}

	return block
}

// mergeDifficulty takes an original stake difficulty and two new, scaled
// stake difficulties, merges the new difficulties, and outputs a new
// merged stake difficulty.
func mergeDifficulty(oldDiff int64, newDiff1 int64, newDiff2 int64) int64 {
	newDiff1Big := big.NewInt(newDiff1)
	newDiff2Big := big.NewInt(newDiff2)
	newDiff2Big.Lsh(newDiff2Big, 32)

	oldDiffBig := big.NewInt(oldDiff)
	oldDiffBigLSH := big.NewInt(oldDiff)
	oldDiffBigLSH.Lsh(oldDiffBig, 32)

	newDiff1Big.Div(oldDiffBigLSH, newDiff1Big)
	newDiff2Big.Div(newDiff2Big, oldDiffBig)

	// Combine the two changes in difficulty.
	summedChange := big.NewInt(0)
	summedChange.Set(newDiff2Big)
	summedChange.Lsh(summedChange, 32)
	summedChange.Div(summedChange, newDiff1Big)
	summedChange.Mul(summedChange, oldDiffBig)
	summedChange.Rsh(summedChange, 32)

	return summedChange.Int64()
}

// limitRetarget clamps the passed new difficulty to the old one adjusted by the
// factor specified in the chain parameters.  This ensures the difficulty can
// only move up or down by a limited amount.
func (g *testGenerator) limitRetarget(oldDiff, newDiff int64) int64 {
	maxRetarget := g.params.RetargetAdjustmentFactor
	switch {
	case newDiff == 0:
		fallthrough
	case (oldDiff / newDiff) > (maxRetarget - 1):
		return oldDiff / maxRetarget
	case (newDiff / oldDiff) > (maxRetarget - 1):
		return oldDiff * maxRetarget
	}

	return newDiff
}

// calcNextRequiredDifficulty returns the required proof-of-work difficulty for
// the block after the current tip block the generator is associated with.
//
// An overview of the algorithm is as follows:
// 1) Use the proof-of-work limit for all blocks before the first retarget
//    window
// 2) Use the previous block's difficulty if the next block is not at a retarget
//    interval
// 3) Calculate the ideal retarget difficulty for each window based on the
//    actual timespan of the window versus the target timespan and exponentially
//    weight each difficulty such that the most recent window has the highest
//    weight
// 4) Calculate the final retarget difficulty based on the exponential weighted
//    average and ensure it is limited to the max retarget adjustment factor
func (g *testGenerator) calcNextRequiredDifficulty() uint32 {
	// Target difficulty before the first retarget interval is the pow
	// limit.
	nextHeight := g.tip.Header.Height + 1
	windowSize := g.params.WorkDiffWindowSize
	if int64(nextHeight) < windowSize {
		return g.params.PowLimitBits
	}

	// Return the previous block's difficulty requirements if the next block
	// is not at a difficulty retarget interval.
	curDiff := int64(g.tip.Header.Bits)
	if int64(nextHeight)%windowSize != 0 {
		return uint32(curDiff)
	}

	// Calculate the ideal retarget difficulty for each window based on the
	// actual time between blocks versus the target time and exponentially
	// weight them.
	adjustedTimespan := big.NewInt(0)
	tempBig := big.NewInt(0)
	weightedTimespanSum, weightSum := big.NewInt(0), big.NewInt(0)
	targetTimespan := int64(g.params.TargetTimespan)
	targetTimespanBig := big.NewInt(targetTimespan)
	numWindows := g.params.WorkDiffWindows
	weightAlpha := g.params.WorkDiffAlpha
	block := g.tip
	finalWindowTime := block.Header.Timestamp.UnixNano()
	for i := int64(0); i < numWindows; i++ {
		// Get the timestamp of the block at the start of the window and
		// calculate the actual timespan accordingly.  Use the target
		// timespan if there are not yet enough blocks left to cover the
		// window.
		actualTimespan := targetTimespan
		if int64(block.Header.Height) > windowSize {
			for j := int64(0); j < windowSize; j++ {
				block = g.blocks[block.Header.PrevBlock]
			}
			startWindowTime := block.Header.Timestamp.UnixNano()
			actualTimespan = finalWindowTime - startWindowTime

			// Set final window time for the next window.
			finalWindowTime = startWindowTime
		}

		// Calculate the ideal retarget difficulty for the window based
		// on the actual timespan and weight it exponentially by
		// multiplying it by 2^(window_number) such that the most recent
		// window receives the most weight.
		//
		// Also, since integer division is being used, shift up the
		// number of new tickets 32 bits to avoid losing precision.
		//
		//   windowWeightShift = ((numWindows - i) * weightAlpha)
		//   adjustedTimespan = (actualTimespan << 32) / targetTimespan
		//   weightedTimespanSum += adjustedTimespan << windowWeightShift
		//   weightSum += 1 << windowWeightShift
		windowWeightShift := uint((numWindows - i) * weightAlpha)
		adjustedTimespan.SetInt64(actualTimespan)
		adjustedTimespan.Lsh(adjustedTimespan, 32)
		adjustedTimespan.Div(adjustedTimespan, targetTimespanBig)
		adjustedTimespan.Lsh(adjustedTimespan, windowWeightShift)
		weightedTimespanSum.Add(weightedTimespanSum, adjustedTimespan)
		weight := tempBig.SetInt64(1)
		weight.Lsh(weight, windowWeightShift)
		weightSum.Add(weightSum, weight)
	}

	// Calculate the retarget difficulty based on the exponential weigthed
	// average and shift the result back down 32 bits to account for the
	// previous shift up in order to avoid losing precision.  Then, limit it
	// to the maximum allowed retarget adjustment factor.
	//
	//   nextDiff = (weightedTimespanSum/weightSum * curDiff) >> 32
	curDiffBig := tempBig.SetInt64(curDiff)
	weightedTimespanSum.Div(weightedTimespanSum, weightSum)
	weightedTimespanSum.Mul(weightedTimespanSum, curDiffBig)
	weightedTimespanSum.Rsh(weightedTimespanSum, 32)
	nextDiff := weightedTimespanSum.Int64()
	nextDiff = g.limitRetarget(curDiff, nextDiff)

	if nextDiff > int64(g.params.PowLimitBits) {
		return g.params.PowLimitBits
	}
	return uint32(nextDiff)
}

// calcNextRequiredStakeDifficulty returns the required stake difficulty (aka
// ticket price) for the block after the current tip block the generator is
// associated with.
//
// An overview of the algorithm is as follows:
// 1) Use the minimum value for any blocks before any tickets could have
//    possibly been purchased due to coinbase maturity requirements
// 2) Return 0 if the current tip block stake difficulty is 0.  This is a
//    safety check against a condition that should never actually happen.
// 3) Use the previous block's difficulty if the next block is not at a retarget
//    interval
// 4) Calculate the ideal retarget difficulty for each window based on the
//    actual pool size in the window versus the target pool size skewed by a
//    constant factor to weight the ticket pool size instead of the tickets per
//    block and exponentially weight each difficulty such that the most recent
//    window has the highest weight
// 5) Calculate the pool size retarget difficulty based on the exponential
//    weighted average and ensure it is limited to the max retarget adjustment
//    factor -- This is the first metric used to calculate the final difficulty
// 6) Calculate the ideal retarget difficulty for each window based on the
//    actual new tickets in the window versus the target new tickets per window
//    and exponentially weight each difficulty such that the most recent window
//    has the highest weight
// 7) Calculate the tickets per window retarget difficulty based on the
//    exponential weighted average and ensure it is limited to the max retarget
//    adjustment factor
// 8) Calculate the final difficulty by averaging the pool size retarget
//    difficulty from #5 and the tickets per window retarget difficulty from #7
//    using scaled multiplication and ensure it is limited to the max retarget
//    adjustment factor
//
// NOTE: In order to simplify the test code, this implementation does not use
// big integers so it will NOT match the actual consensus code for really big
// numbers.  However, the parameters on simnet and the pool sizes used in these
// tests are low enough that this is not an issue for the tests.  Anyone looking
// at this code should NOT use it for mainnet calculations as is since it will
// not always yield the correct results.
func (g *testGenerator) calcNextRequiredStakeDifficulty() int64 {
	// Stake difficulty before any tickets could possibly be purchased is
	// the minimum value.
	nextHeight := g.tip.Header.Height + 1
	stakeDiffStartHeight := uint32(g.params.CoinbaseMaturity) + 1
	if nextHeight < stakeDiffStartHeight {
		return g.params.MinimumStakeDiff
	}

	// Return 0 if the current difficulty is already zero since any scaling
	// of 0 is still 0.  This should never really happen since there is a
	// minimum stake difficulty, but the consensus code checks the condition
	// just in case, so follow suit here.
	curDiff := g.tip.Header.SBits
	if curDiff == 0 {
		return 0
	}

	// Return the previous block's difficulty requirements if the next block
	// is not at a difficulty retarget interval.
	windowSize := g.params.StakeDiffWindowSize
	if int64(nextHeight)%windowSize != 0 {
		return curDiff
	}

	// --------------------------------
	// Ideal pool size retarget metric.
	// --------------------------------

	// Calculate the ideal retarget difficulty for each window based on the
	// actual pool size in the window versus the target pool size and
	// exponentially weight them.
	var weightedPoolSizeSum, weightSum uint64
	ticketsPerBlock := int64(g.params.TicketsPerBlock)
	targetPoolSize := ticketsPerBlock * int64(g.params.TicketPoolSize)
	numWindows := g.params.StakeDiffWindows
	weightAlpha := g.params.StakeDiffAlpha
	block := g.tip
	for i := int64(0); i < numWindows; i++ {
		// Get the pool size for the block at the start of the window.
		// Use zero if there are not yet enough blocks left to cover the
		// window.
		prevRetargetHeight := nextHeight - uint32(windowSize*(i+1))
		windowPoolSize := int64(0)
		block = g.ancestorBlock(block, prevRetargetHeight, nil)
		if block != nil {
			windowPoolSize = int64(block.Header.PoolSize)
		}

		// Skew the pool size by the constant weight factor specified in
		// the chain parameters (which is typically the max adjustment
		// factor) in order to help weight the ticket pool size versus
		// tickets per block.  Also, ensure the skewed pool size is a
		// minimum of 1.
		skewedPoolSize := targetPoolSize + (windowPoolSize-
			targetPoolSize)*int64(g.params.TicketPoolSizeWeight)
		if skewedPoolSize <= 0 {
			skewedPoolSize = 1
		}

		// Calculate the ideal retarget difficulty for the window based
		// on the skewed pool size and weight it exponentially by
		// multiplying it by 2^(window_number) such that the most recent
		// window receives the most weight.
		//
		// Also, since integer division is being used, shift up the
		// number of new tickets 32 bits to avoid losing precision.
		//
		// NOTE: The real algorithm uses big ints, but these purpose
		// built tests won't be using large enough values to overflow,
		// so just use uint64s.
		adjusted := (skewedPoolSize << 32) / targetPoolSize
		adjusted = adjusted << uint64((numWindows-i)*weightAlpha)
		weightedPoolSizeSum += uint64(adjusted)
		weightSum += 1 << uint64((numWindows-i)*weightAlpha)
	}

	// Calculate the pool size retarget difficulty based on the exponential
	// weigthed average and shift the result back down 32 bits to account
	// for the previous shift up in order to avoid losing precision.  Then,
	// limit it to the maximum allowed retarget adjustment factor.
	//
	// This is the first metric used in the final calculated difficulty.
	nextPoolSizeDiff := (int64(weightedPoolSizeSum/weightSum) * curDiff) >> 32
	nextPoolSizeDiff = g.limitRetarget(curDiff, nextPoolSizeDiff)

	// -----------------------------------------
	// Ideal tickets per window retarget metric.
	// -----------------------------------------

	// Calculate the ideal retarget difficulty for each window based on the
	// actual number of new tickets in the window versus the target tickets
	// per window and exponentially weight them.
	var weightedTicketsSum uint64
	targetTicketsPerWindow := ticketsPerBlock * windowSize
	block = g.tip
	for i := int64(0); i < numWindows; i++ {
		// Since the difficulty for the next block after the current tip
		// is being calculated and there is no such block yet, the sum
		// of all new tickets in the first window needs to start with
		// the number of new tickets in the tip block.
		var windowNewTickets int64
		if i == 0 {
			windowNewTickets = int64(block.Header.FreshStake)
		}

		// Tally all of the new tickets in all blocks in the window and
		// ensure the number of new tickets is a minimum of 1.
		prevRetargetHeight := nextHeight - uint32(windowSize*(i+1))
		block = g.ancestorBlock(block, prevRetargetHeight, func(blk *wire.MsgBlock) {
			windowNewTickets += int64(blk.Header.FreshStake)
		})
		if windowNewTickets <= 0 {
			windowNewTickets = 1
		}

		// Calculate the ideal retarget difficulty for the window based
		// on the number of new tickets and weight it exponentially by
		// multiplying it by 2^(window_number) such that the most recent
		// window receives the most weight.
		//
		// Also, since integer division is being used, shift up the
		// number of new tickets 32 bits to avoid losing precision.
		//
		// NOTE: The real algorithm uses big ints, but these purpose
		// built tests won't be using large enough values to overflow,
		// so just use uint64s.
		adjusted := (windowNewTickets << 32) / targetTicketsPerWindow
		adjusted = adjusted << uint64((numWindows-i)*weightAlpha)
		weightedTicketsSum += uint64(adjusted)
	}

	// Calculate the tickets per window retarget difficulty based on the
	// exponential weighted average and shift the result back down 32 bits
	// to account for the previous shift up in order to avoid losing
	// precision.  Then, limit it to the maximum allowed retarget adjustment
	// factor.
	//
	// This is the second metric used in the final calculated difficulty.
	nextNewTixDiff := (int64(weightedTicketsSum/weightSum) * curDiff) >> 32
	nextNewTixDiff = g.limitRetarget(curDiff, nextNewTixDiff)

	// Average the previous two metrics using scaled multiplication and
	// ensure the result is limited to both the maximum allowed retarget
	// adjustment factor and the minimum allowed stake difficulty.
	nextDiff := mergeDifficulty(curDiff, nextPoolSizeDiff, nextNewTixDiff)
	nextDiff = g.limitRetarget(curDiff, nextDiff)
	if nextDiff < g.params.MinimumStakeDiff {
		return g.params.MinimumStakeDiff
	}
	return nextDiff
}

// hash256prng is a determinstic pseudorandom number generator that uses a
// 256-bit secure hashing function to generate random uint32s starting from
// an initial seed.
type hash256prng struct {
	seed       chainhash.Hash // Initialization seed
	idx        uint64         // Hash iterator index
	cachedHash chainhash.Hash // Most recently generated hash
	hashOffset int            // Offset into most recently generated hash
}

// newHash256PRNG creates a pointer to a newly created hash256PRNG.
func newHash256PRNG(seed []byte) *hash256prng {
	// The provided seed is initialized by appending a constant derived from
	// the hex representation of pi and hashing the result to give 32 bytes.
	// This ensures the PRNG is always doing a short number of rounds
	// regardless of input since it will only need to hash small messages
	// (less than 64 bytes).
	seedHash := chainhash.HashFunc(append(seed, hash256prngSeedConst...))
	return &hash256prng{
		seed:       seedHash,
		idx:        0,
		cachedHash: seedHash,
	}
}

// State returns a hash that represents the current state of the deterministic
// PRNG.
func (hp *hash256prng) State() chainhash.Hash {
	// The final state is the hash of the most recently generated hash
	// concatenated with both the hash iterator index and the offset into
	// the hash.
	//
	//   hash(hp.cachedHash || hp.idx || hp.hashOffset)
	finalState := make([]byte, len(hp.cachedHash)+4+1)
	copy(finalState, hp.cachedHash[:])
	offset := len(hp.cachedHash)
	binary.BigEndian.PutUint32(finalState[offset:], uint32(hp.idx))
	offset += 4
	finalState[offset] = byte(hp.hashOffset)
	return chainhash.HashFuncH(finalState)
}

// Hash256Rand returns a uint32 random number using the pseudorandom number
// generator and updates the state.
func (hp *hash256prng) Hash256Rand() uint32 {
	offset := hp.hashOffset * 4
	r := binary.BigEndian.Uint32(hp.cachedHash[offset : offset+4])
	hp.hashOffset++

	// Generate a new hash and reset the hash position index once it would
	// overflow the available bytes in the most recently generated hash.
	if hp.hashOffset > 7 {
		// Hash of the seed concatenated with the hash iterator index.
		//   hash(hp.seed || hp.idx)
		data := make([]byte, len(hp.seed)+4)
		copy(data, hp.seed[:])
		binary.BigEndian.PutUint32(data[len(hp.seed):], uint32(hp.idx))
		hp.cachedHash = chainhash.HashFuncH(data)
		hp.idx++
		hp.hashOffset = 0
	}

	// Roll over the entire PRNG by re-hashing the seed when the hash
	// iterator index overlows a uint32.
	if hp.idx > math.MaxUint32 {
		hp.seed = chainhash.HashFuncH(hp.seed[:])
		hp.cachedHash = hp.seed
		hp.idx = 0
	}

	return r
}

// uniformRandom returns a random in the range [0, upperBound) while avoiding
// modulo bias to ensure a normal distribution within the specified range.
func (hp *hash256prng) uniformRandom(upperBound uint32) uint32 {
	if upperBound < 2 {
		return 0
	}

	// (2^32 - (x*2)) % x == 2^32 % x when x <= 2^31
	min := ((math.MaxUint32 - (upperBound * 2)) + 1) % upperBound
	if upperBound > 0x80000000 {
		min = uint32(1 + ^upperBound)
	}

	r := hp.Hash256Rand()
	for r < min {
		r = hp.Hash256Rand()
	}
	return r % upperBound
}

// winningTickets returns a slice of tickets that are required to vote for the
// given block being voted on and live ticket pool and the associated underlying
// deterministic prng state hash.
func winningTickets(voteBlock *wire.MsgBlock, liveTickets []*stakeTicket, numVotes uint16) ([]*stakeTicket, chainhash.Hash, error) {
	// Serialize the parent block header used as the seed to the
	// deterministic pseudo random number generator for vote selection.
	var buf bytes.Buffer
	if err := voteBlock.Header.Serialize(&buf); err != nil {
		return nil, chainhash.Hash{}, err
	}

	// Ensure the number of live tickets is within the allowable range.
	numLiveTickets := uint32(len(liveTickets))
	if numLiveTickets > math.MaxUint32 {
		return nil, chainhash.Hash{}, fmt.Errorf("live ticket pool "+
			"has %d tickets which is more than the max allowed of "+
			"%d", len(liveTickets), math.MaxUint32)
	}
	if uint32(numVotes) > numLiveTickets {
		return nil, chainhash.Hash{}, fmt.Errorf("live ticket pool "+
			"has %d tickets, while %d are needed to vote",
			len(liveTickets), numVotes)
	}

	// Construct list of winners by generating successive values from the
	// deterministic prng and using them as indices into the sorted live
	// ticket pool while skipping any duplicates that might occur.
	prng := newHash256PRNG(buf.Bytes())
	winners := make([]*stakeTicket, 0, numVotes)
	usedOffsets := make(map[uint32]struct{})
	for uint16(len(winners)) < numVotes {
		ticketIndex := prng.uniformRandom(numLiveTickets)
		if _, exists := usedOffsets[ticketIndex]; !exists {
			usedOffsets[ticketIndex] = struct{}{}
			winners = append(winners, liveTickets[ticketIndex])
		}
	}
	return winners, prng.State(), nil
}

// sortedLiveTickets generates a list of sorted live tickets for the provided
// block.  This is a much less efficient approach than is possible because
// it follows the chain all the back to the first possible ticket height and
// generates the entire live ticket map versus using some type of cached live
// tickets.  However, this is desirable for testing purposes because it means
// the caller does not have to be burdened with keeping track of and updating
// the state of the ticket pool when reorganizing.
func (g *testGenerator) sortedLiveTickets(block *wire.MsgBlock) []*stakeTicket {
	// No possible live tickets before the coinbase maturity height + ticket
	// maturity height.
	tipHeight := block.Header.Height
	coinbaseMaturity := uint32(g.params.CoinbaseMaturity)
	ticketMaturity := uint32(g.params.TicketMaturity)
	if tipHeight <= coinbaseMaturity+ticketMaturity {
		return nil
	}

	// Collect all of the blocks since the first block that could have
	// ticket purchases.
	numToProcess := tipHeight - coinbaseMaturity
	blocksToProcess := make([]*wire.MsgBlock, 0, numToProcess)
	numProcessed := uint32(0)
	for block != nil && numProcessed < numToProcess {
		blocksToProcess = append(blocksToProcess, block)
		block = g.blocks[block.Header.PrevBlock]
		numProcessed++
	}

	// Helper closure to generate and return the live tickets from the map
	// of all tickets by including only the tickets that are mature and
	// have not expired.
	ticketExpiry := uint32(g.params.TicketExpiry)
	allTickets := make(map[chainhash.Hash]*stakeTicket)
	genSortedLiveTickets := func(tipHeight uint32) []*stakeTicket {
		liveTickets := make([]*stakeTicket, 0, len(allTickets))
		for _, ticket := range allTickets {
			liveHeight := ticket.blockHeight + ticketMaturity
			expireHeight := liveHeight + ticketExpiry
			if tipHeight >= liveHeight && tipHeight < expireHeight {
				liveTickets = append(liveTickets, ticket)
			}
		}
		sort.Sort(stakeTicketSorter(liveTickets))
		return liveTickets
	}

	// Helper closure to remove the expected votes for a given parent block
	// from the map of all tickets.
	removeExpectedVotes := func(parentBlock *wire.MsgBlock) {
		liveTickets := genSortedLiveTickets(parentBlock.Header.Height)
		winners, _, err := winningTickets(parentBlock, liveTickets,
			g.params.TicketsPerBlock)
		if err != nil {
			panic(err)
		}
		for _, winner := range winners {
			delete(allTickets, winner.tx.TxSha())
		}
	}

	// Process blocks from the oldest to newest while collecting all stake
	// ticket purchases that have not already been selected to vote.
	for i := range blocksToProcess {
		block := blocksToProcess[len(blocksToProcess)-1-i]

		// Remove expected votes once the stake validation height has
		// been reached since even if the tests didn't actually cast the
		// votes, they are still no longer live.
		if int64(block.Header.Height) >= g.params.StakeValidationHeight {
			parentBlock := g.blocks[block.Header.PrevBlock]
			removeExpectedVotes(parentBlock)
		}

		// Add stake ticket purchases to all tickets map.
		for txIdx, tx := range block.STransactions {
			if isTicketPurchaseTx(tx) {
				ticket := &stakeTicket{tx, block.Header.Height,
					uint32(txIdx)}
				allTickets[tx.TxSha()] = ticket
			}
		}
	}

	// Finally, generate and return the live tickets by including only the
	// tickets that are mature and have not expired.
	return genSortedLiveTickets(tipHeight)
}

// calcFinalLotteryState calculates the final lottery state for a set of winning
// tickets and the associated deterministic prng state hash after selecting the
// winners.  It is the first 6 bytes of:
//   blake256(firstTicketHash || ... || lastTicketHash || prngStateHash)
func calcFinalLotteryState(winners []*stakeTicket, prngStateHash chainhash.Hash) [6]byte {
	data := make([]byte, (len(winners)+1)*chainhash.HashSize)
	for i := 0; i < len(winners); i++ {
		h := winners[i].tx.TxSha()
		copy(data[chainhash.HashSize*i:], h[:])
	}
	copy(data[chainhash.HashSize*len(winners):], prngStateHash[:])
	dataHash := chainhash.HashFuncH(data)

	var finalState [6]byte
	copy(finalState[:], dataHash[0:6])
	return finalState
}

// calcMerkleRoot creates a merkle tree from the slice of transactions and
// returns the root of the tree.
func calcMerkleRoot(txns []*wire.MsgTx) chainhash.Hash {
	utilTxns := make([]*dcrutil.Tx, 0, len(txns))
	for _, tx := range txns {
		utilTxns = append(utilTxns, dcrutil.NewTx(tx))
	}
	merkles := blockchain.BuildMerkleTreeStore(utilTxns)
	return *merkles[len(merkles)-1]
}

// solveBlock attempts to find a nonce which makes the passed block header hash
// to a value less than the target difficulty.  When a successful solution is
// found, true is returned and the nonce field of the passed header is updated
// with the solution.  False is returned if no solution exists.
//
// NOTE: This function will never solve blocks with a nonce of 0.  This is done
// so the 'nextBlock' function can properly detect when a nonce was modified by
// a munge function.
func solveBlock(header *wire.BlockHeader) bool {
	// sbResult is used by the solver goroutines to send results.
	type sbResult struct {
		found bool
		nonce uint32
	}

	// solver accepts a block header and a nonce range to test. It is
	// intended to be run as a goroutine.
	targetDifficulty := blockchain.CompactToBig(header.Bits)
	quit := make(chan bool)
	results := make(chan sbResult)
	solver := func(hdr wire.BlockHeader, startNonce, stopNonce uint32) {
		// We need to modify the nonce field of the header, so make sure
		// we work with a copy of the original header.
		for i := startNonce; i >= startNonce && i <= stopNonce; i++ {
			select {
			case <-quit:
				return
			default:
				hdr.Nonce = i
				hash := hdr.BlockSha()
				if blockchain.ShaHashToBig(&hash).Cmp(
					targetDifficulty) <= 0 {

					results <- sbResult{true, i}
					return
				}
			}
		}
		results <- sbResult{false, 0}
	}

	startNonce := uint32(1)
	stopNonce := uint32(math.MaxUint32)
	numCores := uint32(runtime.NumCPU())
	noncesPerCore := (stopNonce - startNonce) / numCores
	for i := uint32(0); i < numCores; i++ {
		rangeStart := startNonce + (noncesPerCore * i)
		rangeStop := startNonce + (noncesPerCore * (i + 1)) - 1
		if i == numCores-1 {
			rangeStop = stopNonce
		}
		go solver(*header, rangeStart, rangeStop)
	}
	for i := uint32(0); i < numCores; i++ {
		result := <-results
		if result.found {
			close(quit)
			header.Nonce = result.nonce
			return true
		}
	}

	return false
}

// additionalCoinbasePoW returns a function that itself takes a block and
// modifies it by adding the provided amount to the first proof-of-work coinbase
// subsidy.
func additionalCoinbasePoW(amount dcrutil.Amount) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		// Increase the first proof-of-work coinbase subsidy by the
		// provided amount.
		b.Transactions[0].TxOut[2].Value += int64(amount)
	}
}

// additionalCoinbaseDev returns a function that itself takes a block and
// modifies it by adding the provided amount to the coinbase developer subsidy.
func additionalCoinbaseDev(amount dcrutil.Amount) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		// Increase the first proof-of-work coinbase subsidy by the
		// provided amount.
		b.Transactions[0].TxOut[0].Value += int64(amount)
	}
}

// additionalSpendFee returns a function that itself takes a block and modifies
// it by adding the provided fee to the spending transaction.
//
// NOTE: The coinbase value is NOT updated to reflect the additional fee.  Use
// 'additionalCoinbasePoW' for that purpose.
func additionalSpendFee(fee dcrutil.Amount) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		// Increase the fee of the spending transaction by reducing the
		// amount paid,
		if int64(fee) > b.Transactions[1].TxOut[0].Value {
			panic(fmt.Sprintf("additionalSpendFee: fee of %d "+
				"exceeds available spend transaction value",
				fee))
		}
		b.Transactions[1].TxOut[0].Value -= int64(fee)
	}
}

// replaceSpendScript returns a function that itself takes a block and modifies
// it by replacing the public key script of the spending transaction.
func replaceSpendScript(pkScript []byte) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].PkScript = pkScript
	}
}

// replaceCoinbaseSigScript returns a function that itself takes a block and
// modifies it by replacing the signature key script of the coinbase.
func replaceCoinbaseSigScript(script []byte) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.Transactions[0].TxIn[0].SignatureScript = script
	}
}

// replaceStakeSigScript returns a function that itself takes a block and
// modifies it by replacing the signature script of the first stake transaction.
func replaceStakeSigScript(sigScript []byte) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.STransactions[0].TxIn[0].SignatureScript = sigScript
	}
}

// replaceDevOrgScript returns a function that itself takes a block and modifies
// it by replacing the public key script of the dev org payout in the coinbase
// transaction.
func replaceDevOrgScript(pkScript []byte) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut[0].PkScript = pkScript
	}
}

// replaceWithNVotes returns a function that itself takes a block and modifies
// it by replacing the votes in the stake tree with specified number of votes.
func (g *testGenerator) replaceWithNVotes(numVotes uint16) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		// Generate the sorted live ticket pool for the parent block.
		parentBlock := g.blocks[b.Header.PrevBlock]
		liveTickets := g.sortedLiveTickets(parentBlock)

		// Get the winning tickets for the specified number of votes.
		winners, _, err := winningTickets(parentBlock, liveTickets,
			numVotes)
		if err != nil {
			panic(err)
		}

		// Generate vote transactions for the winning tickets.
		defaultNumVotes := int(g.params.TicketsPerBlock)
		numExisting := len(b.STransactions) - defaultNumVotes
		stakeTxns := make([]*wire.MsgTx, 0, numExisting+int(numVotes))
		for _, ticket := range winners {
			voteTx := g.createVoteTx(parentBlock, ticket)
			stakeTxns = append(stakeTxns, voteTx)
		}

		// Add back the original stake transactions other than the
		// original stake votes that have been replaced.
		for _, stakeTx := range b.STransactions[defaultNumVotes:] {
			stakeTxns = append(stakeTxns, stakeTx)
		}

		// Update the block with the new stake transactions and the
		// header with the new number of votes.
		b.STransactions = stakeTxns
		b.Header.Voters = numVotes

		// Recalculate the coinbase amount based on the number of new
		// votes and update the coinbase so that the adjustment in
		// subsidy is accounted for.
		height := b.Header.Height
		fullSubsidy := g.calcFullSubsidy(height)
		devSubsidy := g.calcDevSubsidy(fullSubsidy, height, numVotes)
		powSubsidy := g.calcPoWSubsidy(fullSubsidy, height, numVotes)
		cbTx := b.Transactions[0]
		cbTx.TxIn[0].ValueIn = int64(devSubsidy + powSubsidy)
		cbTx.TxOut = nil
		g.addCoinbaseTxOutputs(cbTx, height, devSubsidy, powSubsidy)
	}
}

// additionalPoWTx returns a function that itself takes a block and modifies it
// by adding the the provided transaction to the regular transaction tree.
func additionalPoWTx(tx *wire.MsgTx) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.AddTransaction(tx)
	}
}

// createSpendTx creates a transaction that spends from the provided spendable
// output and includes an additional unique OP_RETURN output to ensure the
// transaction ends up with a unique hash.  The public key script is a simple
// OP_TRUE p2sh script which avoids the need to track addresses and signature
// scripts in the tests.  This signature script the opTrueRedeemScript.
func (g *testGenerator) createSpendTx(spend *spendableOut, fee dcrutil.Amount) *wire.MsgTx {
	spendTx := wire.NewMsgTx()
	spendTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: spend.prevOut,
		Sequence:         wire.MaxTxInSequenceNum,
		ValueIn:          int64(spend.amount),
		BlockHeight:      spend.blockHeight,
		BlockIndex:       spend.blockIndex,
		SignatureScript:  opTrueRedeemScript,
	})
	spendTx.AddTxOut(wire.NewTxOut(int64(spend.amount-fee),
		g.p2shOpTrueScript))
	spendTx.AddTxOut(wire.NewTxOut(0, uniqueOpReturnScript()))
	return spendTx
}

// createSpendTxForTx creates a transaction that spends from the first output of
// the provided transaction and includes an additional unique OP_RETURN output
// to ensure the transaction ends up with a unique hash.  The public key script
// is a simple OP_TRUE p2sh script which avoids the need to track addresses and
// signature scripts in the tests.  This signature script the
// opTrueRedeemScript.
func (g *testGenerator) createSpendTxForTx(tx *wire.MsgTx, blockHeight, txIndex uint32, fee dcrutil.Amount) *wire.MsgTx {
	spend := makeSpendableOutForTx(tx, blockHeight, txIndex, 0)
	return g.createSpendTx(&spend, fee)
}

// nextBlock builds a new block that extends the current tip associated with the
// generator and updates the generator's tip to the newly generated block.
//
// The block will include the following:
// - A coinbase with the following outputs:
//   - One that pays the required 10% subsidy to the dev org
//   - One that contains a standard coinbase OP_RETURN script
//   - Six that pay the required 60% subsidy to an OP_TRUE p2sh script
// - When a spendable output is provided:
//   - A transaction that spends from the provided output the following outputs:
//     - One that pays the inputs amount minus 1 atom to an OP_TRUE p2sh script
// - Once the coinbase maturity has been reached:
//   - A ticket purchase transaction (sstx) for each provided ticket spendable
//     output with the following outputs:
//     - One OP_SSTX output that grants voting rights to an OP_TRUE p2sh script
//     - One OP_RETURN output that contains the required commitment and pays
//       the subsidy to an OP_TRUE p2sh script
//     - One OP_SSTXCHANGE output that sends change to an OP_TRUE p2sh script
// - Once the stake validation height has been reached:
//   - 5 vote transactions (ssgen) as required according to the live ticket
//     pool and vote selection rules with the following outputs:
//     - One OP_RETURN followed by the block hash and height being voted on
//     - One OP_RETURN followed by the vote bits
//     - One or more OP_SSGEN outputs with the payouts according to the original
//       ticket commitments
//
// Additionally, if one or more munge functions are specified, they will be
// invoked with the block prior to solving it.  This provides callers with the
// opportunity to modify the block which is especially useful for testing.
//
// In order to simply the logic in the munge functions, the following rules are
// applied after all munge functions have been invoked:
// - The merkle root will be recalculated unless it was manually changed
// - The stake root will be recalculated unless it was manually changed
// - The size of the block will be recalculated unless it was manually changed
// - The block will be solved unless the nonce was changed
func (g *testGenerator) nextBlock(blockName string, spend *spendableOut, ticketSpends []spendableOut, mungers ...func(*wire.MsgBlock)) *wire.MsgBlock {
	// Generate the sorted live ticket pool for the current tip.
	liveTickets := g.sortedLiveTickets(g.tip)
	nextHeight := g.tip.Header.Height + 1

	// Calculate the next required stake difficulty (aka ticket price).
	ticketPrice := dcrutil.Amount(g.calcNextRequiredStakeDifficulty())

	// Generate the appropriate votes and ticket purchases based on the
	// current tip block and provided ticket spendable outputs.
	var stakeTxns []*wire.MsgTx
	var finalState [6]byte
	if nextHeight > uint32(g.params.CoinbaseMaturity) {
		// Generate votes once the stake validation height has been
		// reached.
		if int64(nextHeight) >= g.params.StakeValidationHeight {
			// Generate and add the vote transactions for the
			// winning tickets to the stake tree.
			numVotes := g.params.TicketsPerBlock
			winners, stateHash, err := winningTickets(g.tip,
				liveTickets, numVotes)
			if err != nil {
				panic(err)
			}
			for _, ticket := range winners {
				voteTx := g.createVoteTx(g.tip, ticket)
				stakeTxns = append(stakeTxns, voteTx)
			}

			// Calculate the final lottery state hash for use in the
			// block header.
			finalState = calcFinalLotteryState(winners, stateHash)
		}

		// Generate ticket purchases (sstx) using the provided spendable
		// outputs.
		if ticketSpends != nil {
			const ticketFee = dcrutil.Amount(2)
			for i := 0; i < len(ticketSpends); i++ {
				out := &ticketSpends[i]
				purchaseTx := g.createTicketPurchaseTx(out,
					ticketPrice, ticketFee)
				stakeTxns = append(stakeTxns, purchaseTx)
			}
		}
	}

	// Count the number of ticket purchases (sstx), votes (ssgen), and
	// ticket revocations (ssrtx) and calculate the total PoW fees generated
	// by the stake transactions.
	var numVotes uint16
	var numTicketPurchases, numTicketRevocations uint8
	var stakeTreeFees dcrutil.Amount
	for _, tx := range stakeTxns {
		switch {
		case isVoteTx(tx):
			numVotes++
		case isTicketPurchaseTx(tx):
			numTicketPurchases++
		case isRevocationTx(tx):
			numTicketRevocations++
		}

		// Calculate any fees for the transaction.
		var inputSum, outputSum dcrutil.Amount
		for _, txIn := range tx.TxIn {
			inputSum += dcrutil.Amount(txIn.ValueIn)
		}
		for _, txOut := range tx.TxOut {
			outputSum += dcrutil.Amount(txOut.Value)
		}
		stakeTreeFees += (inputSum - outputSum)
	}

	// Create a standard coinbase and spending transaction.
	var regularTxns []*wire.MsgTx
	{
		// Create coinbase transaction for the block with no additional
		// dev or pow subsidy.
		coinbaseTx := g.createCoinbaseTx(nextHeight, numVotes)
		regularTxns = []*wire.MsgTx{coinbaseTx}

		// Increase the PoW subsidy to account for any fees in the stake
		// tree.
		coinbaseTx.TxOut[2].Value += int64(stakeTreeFees)

		// Create a transaction to spend the provided utxo if needed.
		if spend != nil {
			// Create the transaction with a fee of 1 atom for the
			// miner and increase the PoW subsidy accordingly.
			fee := dcrutil.Amount(1)
			coinbaseTx.TxOut[2].Value += int64(fee)

			// Create a transaction that spends from the provided
			// spendable output and includes an additional unique
			// OP_RETURN output to ensure the transaction ends up
			// with a unique hash, then add it to the list of
			// transactions to include in the block.  The script is
			// a simple OP_TRUE p2sh script in order to avoid the
			// need to track addresses and signature scripts in the
			// tests.
			spendTx := g.createSpendTx(spend, fee)
			regularTxns = append(regularTxns, spendTx)
		}
	}

	// Use a timestamp that is 7/8 of target timespan after the previous
	// block unless this is the first block in which case the current time
	// is used.  This helps maintain the retarget difficulty low.  Also,
	// ensure the timestamp is limited to one second precision.
	var ts time.Time
	if nextHeight == 1 {
		ts = time.Now()
	} else {

		ts = g.tip.Header.Timestamp.Add(g.params.TargetTimespan * 7 / 8)
	}
	ts = time.Unix(ts.Unix(), 0)

	// Create the unsolved block.
	block := wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:      1,
			PrevBlock:    g.tip.BlockSha(),
			MerkleRoot:   calcMerkleRoot(regularTxns),
			StakeRoot:    calcMerkleRoot(stakeTxns),
			VoteBits:     1,
			FinalState:   finalState,
			Voters:       numVotes,
			FreshStake:   numTicketPurchases,
			Revocations:  numTicketRevocations,
			PoolSize:     uint32(len(liveTickets)),
			Bits:         g.calcNextRequiredDifficulty(),
			SBits:        int64(ticketPrice),
			Height:       nextHeight,
			Size:         0, // Filled in below.
			Timestamp:    ts,
			Nonce:        0, // To be solved.
			ExtraData:    [32]byte{},
			StakeVersion: 0,
		},
		Transactions:  regularTxns,
		STransactions: stakeTxns,
	}
	block.Header.Size = uint32(block.SerializeSize())

	// Perform any block munging just before solving.  Only recalculate the
	// merkle roots and block size if they weren't manually changed by a
	// munge function.
	curMerkleRoot := block.Header.MerkleRoot
	curStakeRoot := block.Header.StakeRoot
	curSize := block.Header.Size
	curNonce := block.Header.Nonce
	for _, f := range mungers {
		f(&block)
	}
	if block.Header.MerkleRoot == curMerkleRoot {
		block.Header.MerkleRoot = calcMerkleRoot(block.Transactions)
	}
	if block.Header.StakeRoot == curStakeRoot {
		block.Header.StakeRoot = calcMerkleRoot(block.STransactions)
	}
	if block.Header.Size == curSize {
		block.Header.Size = uint32(block.SerializeSize())
	}

	// Only solve the block if the nonce wasn't manually changed by a munge
	// function.
	if block.Header.Nonce == curNonce && !solveBlock(&block.Header) {
		panic(fmt.Sprintf("Unable to solve block at height %d",
			block.Header.Height))
	}

	// Update generator state and return the block.
	blockHash := block.BlockSha()
	g.blocks[blockHash] = &block
	g.blocksByName[blockName] = &block
	g.tip = &block
	g.tipName = blockName
	return &block
}

// createPremineBlock generates the first block of the chain with the required
// premine payouts.  The additional amount parameter can be used to create a
// block that is otherwise a completely valid premine block except it adds the
// extra amount to each payout and thus create a block that violates consensus.
func (g *testGenerator) createPremineBlock(blockName string, additionalAmount dcrutil.Amount) *wire.MsgBlock {
	coinbaseTx := wire.NewMsgTx()
	coinbaseTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, dcrutil.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         0, // Updated below.
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: coinbaseSigScript,
	})

	// Add each required output and tally the total payouts for the coinbase
	// in order to set the input value appropriately.
	var totalSubsidy dcrutil.Amount
	for _, payout := range g.params.BlockOneLedger {
		payoutAddr, err := dcrutil.DecodeAddress(payout.Address, g.params)
		if err != nil {
			panic(err)
		}
		pkScript, err := txscript.PayToAddrScript(payoutAddr)
		if err != nil {
			panic(err)
		}
		coinbaseTx.AddTxOut(&wire.TxOut{
			Value:    payout.Amount + int64(additionalAmount),
			Version:  0,
			PkScript: pkScript,
		})

		totalSubsidy += dcrutil.Amount(payout.Amount)
	}
	coinbaseTx.TxIn[0].ValueIn = int64(totalSubsidy)

	// Generate the block with the specially created regular transactions.
	return g.nextBlock(blockName, nil, nil, func(b *wire.MsgBlock) {
		b.Transactions = []*wire.MsgTx{coinbaseTx}
	})
}

// updateBlockState manually updates the generator state to remove all internal
// map references to a block via its old hash and insert new ones for the new
// block hash.  This is useful if the test code has to manually change a block
// after 'nextBlock' has returned.
func (g *testGenerator) updateBlockState(oldBlockName string, oldBlockHash chainhash.Hash, newBlockName string, newBlock *wire.MsgBlock) {
	// Remove existing entries.
	delete(g.blocks, oldBlockHash)
	delete(g.blocksByName, oldBlockName)

	// Add new entries.
	newBlockHash := newBlock.BlockSha()
	g.blocks[newBlockHash] = newBlock
	g.blocksByName[newBlockName] = newBlock
}

// setTip changes the tip of the instance to the block with the provided name.
// This is useful since the tip is used for things such as generating subsequent
// blocks.
func (g *testGenerator) setTip(blockName string) {
	g.tip = g.blocksByName[blockName]
	if g.tip == nil {
		panic(fmt.Sprintf("tip block name %s does not exist", blockName))
	}
	g.tipName = blockName
}

// oldestCoinbaseOuts removes the oldest set of coinbase proof-of-work outputs
// that was previously saved to the generator and returns the set as a slice.
func (g *testGenerator) oldestCoinbaseOuts() []spendableOut {
	outs := g.spendableOuts[0]
	g.spendableOuts = g.spendableOuts[1:]
	return outs
}

// saveTipCoinbaseOuts adds the proof-of-work outputs of the coinbase tx in the
// current tip block to the list of spendable outputs.
func (g *testGenerator) saveTipCoinbaseOuts() {
	g.spendableOuts = append(g.spendableOuts, []spendableOut{
		makeSpendableOut(g.tip, 0, 2),
		makeSpendableOut(g.tip, 0, 3),
		makeSpendableOut(g.tip, 0, 4),
		makeSpendableOut(g.tip, 0, 5),
		makeSpendableOut(g.tip, 0, 6),
		makeSpendableOut(g.tip, 0, 7),
	})
	g.prevCollectedHash = g.tip.BlockSha()
}

// saveSpendableCoinbaseOuts adds all proof-of-work coinbase outputs starting
// from the block after the last block that had its coinbase outputs collected
// and ending at the current tip.  This is useful to batch the collection of the
// outputs once the tests reach a stable point so they don't have to manually
// add them for the right tests which will ultimately end up being the best
// chain.
func (g *testGenerator) saveSpendableCoinbaseOuts() {
	// Ensure tip is reset to the current one when done.
	curTipName := g.tipName
	defer g.setTip(curTipName)

	// Loop through the ancestors of the current tip until the
	// reaching the block that has already had the coinbase outputs
	// collected.
	var collectBlocks []*wire.MsgBlock
	for b := g.tip; b != nil; b = g.blocks[b.Header.PrevBlock] {
		if b.BlockSha() == g.prevCollectedHash {
			break
		}
		collectBlocks = append(collectBlocks, b)
	}
	for i := range collectBlocks {
		g.tip = collectBlocks[len(collectBlocks)-1-i]
		g.saveTipCoinbaseOuts()
	}
}

// assertTipHeight panics if the current tip block associated with the generator
// does not have the specified height.
func (g *testGenerator) assertTipHeight(expected uint32) {
	height := g.tip.Header.Height
	if height != expected {
		panic(fmt.Sprintf("height for block %q is %d instead of "+
			"expected %d", g.tipName, height, expected))
	}
}

// cloneBlock returns a deep copy of the provided block.
func cloneBlock(b *wire.MsgBlock) wire.MsgBlock {
	var blockCopy wire.MsgBlock
	blockCopy.Header = b.Header
	for _, tx := range b.Transactions {
		blockCopy.AddTransaction(tx.Copy())
	}
	for _, stx := range b.Transactions {
		blockCopy.AddSTransaction(stx.Copy())
	}
	return blockCopy
}

// repeatOpcode returns a byte slice with the provided opcode repeated the
// specified number of times.
func repeatOpcode(opcode uint8, numRepeats int) []byte {
	return bytes.Repeat([]byte{opcode}, numRepeats)
}

// countBlockSigOps returns the number of legacy signature operations in the
// scripts in the passed block.
func countBlockSigOps(block *wire.MsgBlock) int {
	totalSigOps := 0
	for _, tx := range block.Transactions {
		for _, txIn := range tx.TxIn {
			numSigOps := txscript.GetSigOpCount(txIn.SignatureScript)
			totalSigOps += numSigOps
		}
		for _, txOut := range tx.TxOut {
			numSigOps := txscript.GetSigOpCount(txOut.PkScript)
			totalSigOps += numSigOps
		}
	}

	return totalSigOps
}

// assertTipBlockSigOpsCount panics if the current tip block associated with the
// generator does not have the specified number of signature operations.
func (g *testGenerator) assertTipBlockSigOpsCount(expected int) {
	numSigOps := countBlockSigOps(g.tip)
	if numSigOps != expected {
		panic(fmt.Sprintf("generated number of sigops for block %q "+
			"(height %d) is %d instead of expected %d", g.tipName,
			g.tip.Header.Height, numSigOps, expected))
	}
}

// assertTipBlockSize panics if the if the current tip block associated with the
// generator does not have the specified size when serialized.
func (g *testGenerator) assertTipBlockSize(expected int) {
	serializeSize := g.tip.SerializeSize()
	if serializeSize != expected {
		panic(fmt.Sprintf("block size of block %q (height %d) is %d "+
			"instead of expected %d", g.tipName,
			g.tip.Header.Height, serializeSize, expected))
	}
}

// assertTipBlockNumTxns panics if the number of transactions in the current tip
// block associated with the generator does not match the specified value.
func (g *testGenerator) assertTipBlockNumTxns(expected int) {
	numTxns := len(g.tip.Transactions)
	if numTxns != expected {
		panic(fmt.Sprintf("number of txns in block %q (height %d) is "+
			"%d instead of expected %d", g.tipName,
			g.tip.Header.Height, numTxns, expected))
	}
}

// assertTipBlockHash panics if the current tip block associated with the
// generator does not match the specified hash.
func (g *testGenerator) assertTipBlockHash(expected chainhash.Hash) {
	hash := g.tip.BlockSha()
	if hash != expected {
		panic(fmt.Sprintf("block hash of block %q (height %d) is %v "+
			"instead of expected %v", g.tipName,
			g.tip.Header.Height, hash, expected))
	}
}

// assertTipBlockMerkleRoot panics if the merkle root in header of the current
// tip block associated with the generator does not match the specified hash.
func (g *testGenerator) assertTipBlockMerkleRoot(expected chainhash.Hash) {
	hash := g.tip.Header.MerkleRoot
	if hash != expected {
		panic(fmt.Sprintf("merkle root of block %q (height %d) is %v "+
			"instead of expected %v", g.tipName,
			g.tip.Header.Height, hash, expected))
	}
}

// Generate returns a slice of tests that can be used to exercise the consensus
// validation rules.  The tests are intended to be flexible enough to allow both
// unit-style tests directly against the blockchain code as well as
// integration-style tests over the peer-to-peer network.  To achieve that goal,
// each test contains additional information about the expected result, however
// that information can be ignored when doing comparison tests between to
// independent versions over the peer-to-peer network.
func Generate() (tests [][]TestInstance, err error) {
	// In order to simplify the generation code which really should never
	// fail unless the test code itself is broken, panics are used
	// internally.  This deferred func ensures any panics don't escape the
	// generator by replacing the named error return with the underlying
	// panic error.
	defer func() {
		if r := recover(); r != nil {
			tests = nil

			switch rt := r.(type) {
			case string:
				err = errors.New(rt)
			case error:
				err = rt
			default:
				err = errors.New("Unknown panic")
			}
		}
	}()

	// Create a test generator instance initialized with the genesis block
	// as the tip as well as some cached payment scripts to be used
	// throughout the tests.
	g, err := makeTestGenerator(simNetParams)
	if err != nil {
		return nil, err
	}

	// Define some convenience helper functions to return an individual test
	// instance that has the described characteristics.
	//
	// acceptBlock creates a test instance that expects the provided block
	// to be accepted by the consensus rules.
	//
	// rejectBlock creates a test instance that expects the provided block
	// to be rejected by the consensus rules.
	//
	// orphanOrRejectBlock creates a test instance that expected the
	// provided block to either by accepted as an orphan or rejected by the
	// consensus rules.
	//
	// expectTipBlock creates a test instance that expects the provided
	// block to be the current tip of the block chain.
	acceptBlock := func(blockName string, block *wire.MsgBlock, isMainChain, isOrphan bool) TestInstance {
		return AcceptedBlock{blockName, block, isMainChain, isOrphan}
	}
	rejectBlock := func(blockName string, block *wire.MsgBlock, code blockchain.ErrorCode) TestInstance {
		return RejectedBlock{blockName, block, code}
	}
	orphanOrRejectBlock := func(blockName string, block *wire.MsgBlock) TestInstance {
		return OrphanOrRejectedBlock{blockName, block}
	}
	expectTipBlock := func(blockName string, block *wire.MsgBlock) TestInstance {
		return ExpectedTip{blockName, block}
	}

	// Define some convenience helper functions to populate the tests slice
	// with test instances that have the described characteristics.
	//
	// accepted creates and appends a single acceptBlock test instance for
	// the current tip which expects the block to be accepted to the main
	// chain.
	//
	// acceptedToSideChainWithExpectedTip creates an appends a two-instance
	// test.  The first instance is an acceptBlock test instance for the
	// current tip which expects the block to be accepted to a side chain.
	// The second instance is an expectBlockTip test instance for provided
	// values.
	//
	// orphaned creates and appends a single acceptBlock test instance for
	// the current tip which expects the block to be accepted as an orphan.
	//
	// orphanedOrRejected creates and appends a single orphanOrRejectBlock
	// test instance for the current tip.
	//
	// rejected creates and appends a single rejectBlock test instance for
	// the current tip.
	accepted := func() {
		tests = append(tests, []TestInstance{
			acceptBlock(g.tipName, g.tip, true, false),
		})
	}
	acceptedToSideChainWithExpectedTip := func(tipName string) {
		tests = append(tests, []TestInstance{
			acceptBlock(g.tipName, g.tip, false, false),
			expectTipBlock(tipName, g.blocksByName[tipName]),
		})
	}
	orphaned := func() {
		tests = append(tests, []TestInstance{
			acceptBlock(g.tipName, g.tip, false, true),
		})
	}
	orphanedOrRejected := func() {
		tests = append(tests, []TestInstance{
			orphanOrRejectBlock(g.tipName, g.tip),
		})
	}
	rejected := func(code blockchain.ErrorCode) {
		tests = append(tests, []TestInstance{
			rejectBlock(g.tipName, g.tip, code),
		})
	}

	// Shorter versions of useful params for convenience.
	coinbaseMaturity := g.params.CoinbaseMaturity
	stakeEnabledHeight := g.params.StakeEnabledHeight
	stakeValidationHeight := g.params.StakeValidationHeight
	maxBlockSize := g.params.MaximumBlockSize
	ticketsPerBlock := g.params.TicketsPerBlock

	// ---------------------------------------------------------------------
	// Premine tests.
	// ---------------------------------------------------------------------

	// Attempt to insert an initial premine block that does not conform to
	// the required premine payouts by adding one atom too many to each
	// payout.
	//
	//   genesis
	//          \-> bpbad0
	g.createPremineBlock("bpbad0", 1)
	rejected(blockchain.ErrBadCoinbaseValue)

	// Add the required premine block.
	//
	//   genesis -> bp
	g.setTip("genesis")
	g.createPremineBlock("bp", 0)
	g.assertTipHeight(1)
	accepted()

	// TODO:
	// - Try to spend premine output before maturity in the regular tree
	// - Try to spend premine output before maturity in the stake tree by
	//   creating a ticket purchase

	// ---------------------------------------------------------------------
	// Generate enough blocks to have mature coinbase outputs to work with.
	//
	//   genesis -> bp -> bm0 -> bm1 -> ... -> bm#
	// ---------------------------------------------------------------------

	var testInstances []TestInstance
	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bm%d", i)
		g.nextBlock(blockName, nil, nil)
		g.saveTipCoinbaseOuts()
		testInstances = append(testInstances, acceptBlock(g.tipName,
			g.tip, true, false))
	}
	tests = append(tests, testInstances)
	g.assertTipHeight(uint32(coinbaseMaturity) + 1)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake enabled height while
	// creating ticket purchases that spend from the coinbases matured
	// above.  This will also populate the pool of immature tickets.
	//
	//   ... -> bm# ... -> bse0 -> bse1 -> ... -> bse#
	// ---------------------------------------------------------------------

	testInstances = nil
	for i := int64(0); int64(g.tip.Header.Height) < stakeEnabledHeight; i++ {
		outs := g.oldestCoinbaseOuts()
		blockName := fmt.Sprintf("bse%d", i)
		g.nextBlock(blockName, nil, outs[1:])
		g.saveTipCoinbaseOuts()
		testInstances = append(testInstances, acceptBlock(g.tipName,
			g.tip, true, false))
	}
	tests = append(tests, testInstances)
	g.assertTipHeight(uint32(stakeEnabledHeight))

	// TODO: Modify the above to generate a few less so this section can
	// test negative validation failures such as the following items and
	// then finish generating the rest after to reach the stake enabled
	// height.
	//
	// - Ticket purchase with various malformed transactions such as
	//   incorrectly ordered txouts, scripts that do not involve p2pkh or
	//   p2sh addresses, etc
	// - Try to vote with an immature ticket
	// - Try to vote before stake validation height

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake validation height while
	// continuing to purchase tickets using the coinbases matured above and
	// allowing the immature tickets to mature and thus become live.
	//
	// While doing so, also generate blocks that have the required early
	// vote unset to ensure they are properly rejected.
	//
	//   ... -> bse# -> bsv0 -> bsv1 -> ... -> bsv#
	//             \       \       \        \-> bevbad#
	//              |       |       \-> bevbad2
	//              |       \-> bevbad1
	//              \-> bevbad0
	// ---------------------------------------------------------------------

	testInstances = nil
	for i := int64(0); int64(g.tip.Header.Height) < stakeValidationHeight; i++ {
		// Until stake validation height is reached, test that any
		// blocks without the early vote bits set are rejected.
		if int64(g.tip.Header.Height) < stakeValidationHeight-1 {
			prevTip := g.tipName
			blockName := fmt.Sprintf("bevbad%d", i)
			g.nextBlock(blockName, nil, nil, func(b *wire.MsgBlock) {
				b.Header.VoteBits = 0
			})
			testInstances = append(testInstances, rejectBlock(g.tipName,
				g.tip, blockchain.ErrInvalidEarlyVoteBits))
			g.setTip(prevTip)
		}

		outs := g.oldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsv%d", i)
		g.nextBlock(blockName, nil, outs[1:])
		g.saveTipCoinbaseOuts()
		testInstances = append(testInstances, acceptBlock(g.tipName,
			g.tip, true, false))
	}
	tests = append(tests, testInstances)
	g.assertTipHeight(uint32(stakeValidationHeight))

	// ---------------------------------------------------------------------
	// Generate enough blocks to have a known distance to the first mature
	// coinbase outputs for all tests that follow.  These blocks continue
	// to purchase tickets to avoid running out of votes.
	//
	//   ... -> bsv# -> bbm0 -> bbm1 -> ... -> bbm#
	// ---------------------------------------------------------------------

	testInstances = nil
	for i := uint16(0); i < coinbaseMaturity; i++ {
		outs := g.oldestCoinbaseOuts()
		blockName := fmt.Sprintf("bbm%d", i)
		g.nextBlock(blockName, nil, outs[1:])
		g.saveTipCoinbaseOuts()
		testInstances = append(testInstances, acceptBlock(g.tipName,
			g.tip, true, false))
	}
	tests = append(tests, testInstances)

	// Collect spendable outputs into two different slices.  The outs slice
	// is intended to be used for regular transactions that spend from the
	// output, while the ticketOuts slice is intended to be used for stake
	// ticket purchases.
	var outs []*spendableOut
	var ticketOuts [][]spendableOut
	for i := uint16(0); i < coinbaseMaturity; i++ {
		coinbaseOuts := g.oldestCoinbaseOuts()
		outs = append(outs, &coinbaseOuts[0])
		ticketOuts = append(ticketOuts, coinbaseOuts[1:])
	}

	// ---------------------------------------------------------------------
	// Basic forking and reorg tests.
	// ---------------------------------------------------------------------

	// ---------------------------------------------------------------------
	// The comments below identify the structure of the chain being built.
	//
	// The values in parenthesis repesent which outputs are being spent.
	//
	// For example, b1(0) indicates the first collected spendable output
	// which, due to the code above to create the correct number of blocks,
	// is the first output that can be spent at the current block height due
	// to the coinbase maturity requirement.
	// ---------------------------------------------------------------------

	// Start by building a couple of blocks at current tip (value in parens
	// is which output is spent):
	//
	//   ... -> b1(0) -> b2(1)
	g.nextBlock("b1", outs[0], ticketOuts[0])
	accepted()

	g.nextBlock("b2", outs[1], ticketOuts[1])
	accepted()

	// Ensure duplicate blocks are rejected.
	//
	//   ... -> b1(0) -> b2(1)
	//               \-> b2(1)
	rejected(blockchain.ErrDuplicateBlock)

	// Create a fork from b1.  There should not be a reorg since b2 was seen
	// first.
	//
	//   ... -> b1(0) -> b2(1)
	//               \-> b3(1)
	g.setTip("b1")
	g.nextBlock("b3", outs[1], ticketOuts[1])
	b3Tx1Out := makeSpendableOut(g.tip, 1, 0)
	acceptedToSideChainWithExpectedTip("b2")

	// Extend b3 fork to make the alternative chain longer and force reorg.
	//
	//   ... -> b1(0) -> b2(1)
	//               \-> b3(1) -> b4(2)
	g.nextBlock("b4", outs[2], ticketOuts[2])
	accepted()

	// Extend b2 fork twice to make first chain longer and force reorg.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//               \-> b3(1) -> b4(2)
	g.setTip("b2")
	g.nextBlock("b5", outs[2], ticketOuts[2])
	acceptedToSideChainWithExpectedTip("b4")

	g.nextBlock("b6", outs[3], ticketOuts[3])
	accepted()

	// ---------------------------------------------------------------------
	// Double spend tests.
	// ---------------------------------------------------------------------

	// Create a fork that double spends.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//                                 \-> b7(2) -> b8(4)
	//               \-> b3(1) -> b4(2)
	g.setTip("b5")
	g.nextBlock("b7", outs[2], ticketOuts[3])
	acceptedToSideChainWithExpectedTip("b6")

	g.nextBlock("b8", outs[4], ticketOuts[4])
	rejected(blockchain.ErrMissingTx)

	// ---------------------------------------------------------------------
	// Too much proof-of-work coinbase tests.
	// ---------------------------------------------------------------------

	// Create a block that generates too much proof-of-work coinbase.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//                                         \-> b9(4)
	//               \-> b3(1) -> b4(2)
	g.setTip("b6")
	g.nextBlock("b9", outs[4], ticketOuts[4], additionalCoinbasePoW(1))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create a fork that ends with block that generates too much
	// proof-of-work coinbase.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//                                 \-> b10(3) -> b11(4)
	//               \-> b3(1) -> b4(2)
	g.setTip("b5")
	g.nextBlock("b10", outs[3], ticketOuts[3])
	acceptedToSideChainWithExpectedTip("b6")

	g.nextBlock("b11", outs[4], ticketOuts[4], additionalCoinbasePoW(1))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create a fork that ends with block that generates too much
	// proof-of-work coinbase as before, but with a valid fork first.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//              |                  \-> b12(3) -> b13(4) -> b14(5)
	//              |                      (b12 added last)
	//               \-> b3(1) -> b4(2)
	g.setTip("b5")
	b12 := g.nextBlock("b12", outs[3], ticketOuts[3])
	b13 := g.nextBlock("b13", outs[4], ticketOuts[4])
	b14 := g.nextBlock("b14", outs[5], ticketOuts[5], additionalCoinbasePoW(1))
	tests = append(tests, []TestInstance{
		acceptBlock("b13", b13, false, true),
		acceptBlock("b14", b14, false, true),
		rejectBlock("b12", b12, blockchain.ErrBadCoinbaseValue),
		expectTipBlock("b13", b13),
	})

	// ---------------------------------------------------------------------
	// Bad dev org output tests.
	// ---------------------------------------------------------------------

	// Create a block that does not generate a payment to the dev-org P2SH
	// address.  Test this by trying to pay to a secp256k1 P2PKH address
	// using the same HASH160.
	//
	//   ... -> b5(2) -> b12(3) -> b13(4)
	//   \                               \-> bbadtaxscript(5)
	//    \-> b3(1) -> b4(2)
	g.setTip("b13")
	g.nextBlock("bbadtaxscript", outs[5], ticketOuts[5], func(b *wire.MsgBlock) {
		taxOutput := b.Transactions[0].TxOut[0]
		_, addrs, _, _ := txscript.ExtractPkScriptAddrs(
			g.params.OrganizationPkScriptVersion, taxOutput.PkScript,
			g.params)
		p2shTaxAddr := addrs[0].(*dcrutil.AddressScriptHash)
		p2pkhTaxAddr, err := dcrutil.NewAddressPubKeyHash(
			p2shTaxAddr.Hash160()[:], g.params,
			chainec.ECTypeSecp256k1)
		if err != nil {
			panic(err)
		}
		p2pkhScript, err := txscript.PayToAddrScript(p2pkhTaxAddr)
		if err != nil {
			panic(err)
		}
		taxOutput.PkScript = p2pkhScript
	})
	rejected(blockchain.ErrNoTax)

	// Create a block that uses a newer output script version than is
	// supported for the dev-org tax output.
	//
	//   ... -> b5(2) -> b12(3) -> b13(4)
	//   \                               \-> bbadtaxscriptversion(5)
	//    \-> b3(1) -> b4(2)
	g.setTip("b13")
	g.nextBlock("bbadtaxscriptversion", outs[5], ticketOuts[5], func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut[0].Version = 1
	})
	rejected(blockchain.ErrNoTax)

	// ---------------------------------------------------------------------
	// Too much dev-org coinbase tests.
	// ---------------------------------------------------------------------

	// Create a block that generates too much dev-org coinbase.
	//
	//   ... -> b5(2) -> b12(3) -> b13(4)
	//   \                               \-> b15(5)
	//    \-> b3(1) -> b4(2)
	g.setTip("b13")
	g.nextBlock("b15", outs[5], ticketOuts[5], additionalCoinbaseDev(1))
	rejected(blockchain.ErrNoTax)

	// Create a fork that ends with block that generates too much dev-org
	// coinbase.
	//
	//   ... -> b5(2) -> b12(3) -> b13(4)
	//   \                     \-> b16(4) -> b17(5)
	//    \-> b3(1) -> b4(2)
	g.setTip("b12")
	g.nextBlock("b16", outs[4], ticketOuts[4], additionalCoinbaseDev(1))
	acceptedToSideChainWithExpectedTip("b13")

	g.nextBlock("b17", outs[5], ticketOuts[5], additionalCoinbaseDev(1))
	rejected(blockchain.ErrNoTax)

	// Create a fork that ends with block that generates too much dev-org
	// coinbase as before, but with a valid fork first.
	//
	//   ... -> b5(2) -> b12(3) -> b13(4)
	//   \                     \-> b18(4) -> b19(5) -> b20(6)
	//   |                         (b18 added last)
	//    \-> b3(1) -> b4(2)
	//
	g.setTip("b12")
	b18 := g.nextBlock("b18", outs[4], ticketOuts[4])
	b19 := g.nextBlock("b19", outs[5], ticketOuts[5])
	b20 := g.nextBlock("b20", outs[6], ticketOuts[6], additionalCoinbaseDev(1))
	tests = append(tests, []TestInstance{
		acceptBlock("b19", b19, false, true),
		acceptBlock("b20", b20, false, true),
		rejectBlock("b18", b18, blockchain.ErrNoTax),
		expectTipBlock("b19", b19),
	})

	// ---------------------------------------------------------------------
	// Checksig signature operation count tests.
	// ---------------------------------------------------------------------

	// Add a block with max allowed signature operations.
	//
	//   ... -> b5(2) -> b12(3) -> b18(4) -> b19(5) -> b21(6)
	//   \-> b3(1) -> b4(2)
	g.setTip("b19")
	manySigOps := bytes.Repeat([]byte{txscript.OP_CHECKSIG}, maxBlockSigOps)
	g.nextBlock("b21", outs[6], ticketOuts[6], replaceSpendScript(manySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Attempt to add block with more than max allowed signature operations.
	//
	//   ... -> b5(2) -> b12(3) -> b18(4) -> b19(5) -> b21(6)
	//   \                                                   \-> b22(7)
	//    \-> b3(1) -> b4(2)
	tooManySigOps := bytes.Repeat([]byte{txscript.OP_CHECKSIG}, maxBlockSigOps+1)
	g.nextBlock("b22", outs[7], ticketOuts[7], replaceSpendScript(tooManySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// ---------------------------------------------------------------------
	// Cross-fork spend tests.
	// ---------------------------------------------------------------------

	// Create block that spends a tx created on a different fork.
	//
	//   ... -> b5(2) -> b12(3) -> b18(4) -> b19(5) -> b21(6)
	//   \                                                   \-> b23(b3.tx[1])
	//    \-> b3(1) -> b4(2)
	g.setTip("b21")
	g.nextBlock("b23", &b3Tx1Out, nil)
	rejected(blockchain.ErrMissingTx)

	// Create block that forks and spends a tx created on a third fork.
	//
	//   ... -> b5(2) -> b12(3) -> b18(4) -> b19(5) -> b21(6)
	//   |                                         \-> b24(b3.tx[1]) -> b25(6)
	//    \-> b3(1) -> b4(2)
	g.setTip("b19")
	g.nextBlock("b24", &b3Tx1Out, nil)
	acceptedToSideChainWithExpectedTip("b21")

	g.nextBlock("b25", outs[6], ticketOuts[6])
	rejected(blockchain.ErrMissingTx)

	// ---------------------------------------------------------------------
	// Immature coinbase tests.
	// ---------------------------------------------------------------------

	// Create block that spends immature coinbase.
	//
	//   ... -> b19(5) -> b21(6)
	//                          \-> b26(8)
	g.setTip("b21")
	g.nextBlock("b26", outs[8], ticketOuts[7])
	rejected(blockchain.ErrImmatureSpend)

	// Create block that spends immature coinbase on a fork.
	//
	//   ... -> b19(5) -> b21(6)
	//                \-> b27(6) -> b28(8)
	g.setTip("b19")
	g.nextBlock("b27", outs[6], ticketOuts[6])
	acceptedToSideChainWithExpectedTip("b21")

	g.nextBlock("b28", outs[8], ticketOuts[7])
	rejected(blockchain.ErrImmatureSpend)

	// ---------------------------------------------------------------------
	// Max block size tests.
	// ---------------------------------------------------------------------

	// Create block that is the max allowed size.
	//
	//   ... -> b21(6) -> b29(7)
	g.setTip("b21")
	g.nextBlock("b29", outs[7], ticketOuts[7], func(b *wire.MsgBlock) {
		curScriptLen := len(g.p2shOpTrueScript)
		bytesToMaxSize := maxBlockSize - b.SerializeSize() +
			(curScriptLen - 4)
		sizePadScript := repeatOpcode(0x00, bytesToMaxSize)
		replaceSpendScript(sizePadScript)(b)
	})
	g.assertTipBlockSize(maxBlockSize)
	accepted()

	// Create block that is the one byte larger than max allowed size.  This
	// is done on a fork and should be rejected regardless.
	//
	//   ... -> b21(6) -> b29(7)
	//                \-> b30(7) -> b31(8)
	g.setTip("b21")
	g.nextBlock("b30", outs[7], ticketOuts[7], func(b *wire.MsgBlock) {
		curScriptLen := len(g.p2shOpTrueScript)
		bytesToMaxSize := maxBlockSize - b.SerializeSize() +
			(curScriptLen - 4)
		sizePadScript := repeatOpcode(0x00, bytesToMaxSize+1)
		replaceSpendScript(sizePadScript)(b)
	})
	g.assertTipBlockSize(maxBlockSize + 1)
	rejected(blockchain.ErrBlockTooBig)

	// Parent was rejected, so this block must either be an orphan or
	// outright rejected due to an invalid parent.
	g.nextBlock("b31", outs[8], ticketOuts[8])
	orphanedOrRejected()

	// ---------------------------------------------------------------------
	// Orphan tests.
	// ---------------------------------------------------------------------

	// Create valid orphan block with zero prev hash.
	//
	//   No previous block
	//                    \-> borphan0(7)
	g.setTip("b21")
	g.nextBlock("borphan0", outs[7], ticketOuts[7], func(b *wire.MsgBlock) {
		b.Header.PrevBlock = chainhash.Hash{}
	})
	orphaned()

	// Create valid orphan block.
	//
	//   ... -> b21(6)
	//                \-> borphanbase(7) -> borphan1(8)
	g.setTip("b21")
	g.nextBlock("borphanbase", outs[7], ticketOuts[7])
	g.nextBlock("borphan1", outs[8], ticketOuts[8])
	orphaned()

	// Ensure duplicate orphan blocks are rejected.
	rejected(blockchain.ErrDuplicateBlock)

	// ---------------------------------------------------------------------
	// Coinbase script length limits tests.
	// ---------------------------------------------------------------------

	// Create block that has a coinbase script that is smaller than the
	// required length.  This is done on a fork and should be rejected
	// regardless.  Also, create a block that builds on the rejected block.
	//
	//   ... -> b21(6) -> b29(7)
	//                \-> b32(7) -> b33(8)
	g.setTip("b21")
	tooSmallCbScript := repeatOpcode(0x00, minCoinbaseScriptLen-1)
	g.nextBlock("b32", outs[7], ticketOuts[7], replaceCoinbaseSigScript(tooSmallCbScript))
	rejected(blockchain.ErrBadCoinbaseScriptLen)

	// Parent was rejected, so this block must either be an orphan or
	// outright rejected due to an invalid parent.
	g.nextBlock("b33", outs[8], ticketOuts[8])
	orphanedOrRejected()

	// Create block that has a coinbase script that is larger than the
	// allowed length.  This is done on a fork and should be rejected
	// regardless.  Also, create a block that builds on the rejected block.
	//
	//   ... -> b21(6) -> b29(7)
	//                \-> b34(7) -> b35(8)
	g.setTip("b21")
	tooLargeCbScript := repeatOpcode(0x00, maxCoinbaseScriptLen+1)
	g.nextBlock("b34", outs[7], ticketOuts[7], replaceCoinbaseSigScript(tooLargeCbScript))
	rejected(blockchain.ErrBadCoinbaseScriptLen)

	// Parent was rejected, so this block must either be an orphan or
	// outright rejected due to an invalid parent.
	g.nextBlock("b35", outs[8], ticketOuts[8])
	orphanedOrRejected()

	// Create block that has a max length coinbase script.
	//
	//   ... -> b29(7) -> b36(8)
	g.setTip("b29")
	maxSizeCbScript := repeatOpcode(0x00, maxCoinbaseScriptLen)
	g.nextBlock("b36", outs[8], ticketOuts[8], replaceCoinbaseSigScript(maxSizeCbScript))
	accepted()

	// ---------------------------------------------------------------------
	// Vote tests.
	// ---------------------------------------------------------------------

	// Attempt to add block where ssgen has a null ticket reference hash.
	//
	//   ... -> b36(8)
	//                \-> bv1(9)
	g.setTip("b36")
	g.nextBlock("bv1", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.STransactions[0].TxIn[1].PreviousOutPoint = wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: math.MaxUint32,
			Tree:  dcrutil.TxTreeRegular,
		}
	})
	rejected(blockchain.ErrBadTxInput)

	// Attempt to add block with a regular tx in the stake tree.
	//
	//   ... -> b36(8)
	//                \-> bv2(9)
	g.setTip("b36")
	g.nextBlock("bv2", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.STransactions[0] = b.Transactions[0]
	})
	rejected(blockchain.ErrRegTxInStakeTree)

	// Attempt to add block with too many votes.
	//
	//   ... -> b36(8)
	//                \-> bv3(9)
	g.setTip("b36")
	g.nextBlock("bv3", outs[9], ticketOuts[9], g.replaceWithNVotes(ticketsPerBlock+1))
	rejected(blockchain.ErrTooManyVotes)

	// Attempt to add block with too few votes.
	//
	//   ... -> b36(8)
	//                \-> bv4(9)
	g.setTip("b36")
	g.nextBlock("bv4", outs[9], ticketOuts[9], g.replaceWithNVotes(ticketsPerBlock-3))
	rejected(blockchain.ErrNotEnoughVotes)

	// Attempt to add block with different number of votes in stake tree and
	// header.
	//
	//   ... -> b36(8)
	//                \-> bv5(9)
	g.setTip("b36")
	g.nextBlock("bv5", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.Header.FreshStake = 4
	})
	rejected(blockchain.ErrFreshStakeMismatch)

	// ---------------------------------------------------------------------
	// Stake ticket difficulty tests.
	// ---------------------------------------------------------------------

	// Create block with ticket purchase below required ticket price.
	//
	//   ... -> b36(8)
	//                \-> bsd0(9)
	g.setTip("b36")
	g.nextBlock("bsd0", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.STransactions[5].TxOut[0].Value -= 1
	})
	rejected(blockchain.ErrNotEnoughStake)

	// Create block with stake transaction below pos limit.
	//
	//   ... -> b36(8)
	//                \-> bsd1(9)
	g.setTip("b36")
	g.nextBlock("bsd1", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		minStakeDiff := g.params.MinimumStakeDiff
		b.Header.SBits = minStakeDiff - 2
		// TODO: This should not be necessary.  The code is checking
		// the ticket commit value against sbits before checking if
		// sbits is under the minimum.  It should be reversed.
		b.STransactions[5].TxOut[0].Value = minStakeDiff - 1
	})
	rejected(blockchain.ErrStakeBelowMinimum)

	// ---------------------------------------------------------------------
	// Stakebase script tests.
	// ---------------------------------------------------------------------

	// Create block that has a stakebase script that is smaller than the
	// minimum allowed length.
	//
	//   ... -> b36(8)
	//                \-> bss0(9)
	g.setTip("b36")
	tooSmallCbScript = repeatOpcode(0x00, minCoinbaseScriptLen-1)
	g.nextBlock("bss0", outs[9], ticketOuts[9], replaceStakeSigScript(tooSmallCbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Create block that has a stakebase script that is larger than the
	// maximum allowed length.
	//
	//   ... -> b36(8)
	//                \-> bss1(9)
	g.setTip("b36")
	tooLargeCbScript = repeatOpcode(0x00, maxCoinbaseScriptLen+1)
	g.nextBlock("bss1", outs[9], ticketOuts[9], replaceStakeSigScript(tooLargeCbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Add a block with a stake transaction with a signature script that is
	// not the required script, but is otherwise
	// script.
	//
	//   ... -> b36(8)
	//                \-> bss2(9)
	g.setTip("b36")
	badScript := append(g.params.StakeBaseSigScript, 0x00)
	g.nextBlock("bss2", outs[9], ticketOuts[9], replaceStakeSigScript(badScript))
	rejected(blockchain.ErrBadStakevaseScrVal)

	// ---------------------------------------------------------------------
	// Multisig[Verify]/ChecksigVerifiy signature operation count tests.
	// ---------------------------------------------------------------------

	// Create block with max signature operations as OP_CHECKMULTISIG.
	//
	//   ... -> b36(8) -> b37(9)
	//
	// OP_CHECKMULTISIG counts for 20 sigops.
	g.setTip("b36")
	manySigOps = repeatOpcode(txscript.OP_CHECKMULTISIG, maxBlockSigOps/20)
	g.nextBlock("b37", outs[9], ticketOuts[9], replaceSpendScript(manySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with more than max allowed signature operations using
	// OP_CHECKMULTISIG.
	//
	//   ... -> b37(9)
	//                \-> b38(10)
	//
	// OP_CHECKMULTISIG counts for 20 sigops.
	tooManySigOps = repeatOpcode(txscript.OP_CHECKMULTISIG, maxBlockSigOps/20)
	tooManySigOps = append(manySigOps, txscript.OP_CHECKSIG)
	g.nextBlock("b38", outs[10], ticketOuts[10], replaceSpendScript(tooManySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// Create block with max signature operations as OP_CHECKMULTISIGVERIFY.
	//
	//   ... -> b37(9) -> b39(10)
	//
	g.setTip("b37")
	manySigOps = repeatOpcode(txscript.OP_CHECKMULTISIGVERIFY, maxBlockSigOps/20)
	g.nextBlock("b39", outs[10], ticketOuts[10], replaceSpendScript(manySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with more than max allowed signature operations using
	// OP_CHECKMULTISIGVERIFY.
	//
	//   ... -> b39(10)
	//                \-> b40(11)
	//
	tooManySigOps = repeatOpcode(txscript.OP_CHECKMULTISIGVERIFY, maxBlockSigOps/20)
	tooManySigOps = append(manySigOps, txscript.OP_CHECKSIG)
	g.nextBlock("b40", outs[11], ticketOuts[11], replaceSpendScript(tooManySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// Create block with max signature operations as OP_CHECKSIGVERIFY.
	//
	//   ... -> b39(10) -> b41(11)
	//
	g.setTip("b39")
	manySigOps = repeatOpcode(txscript.OP_CHECKSIGVERIFY, maxBlockSigOps)
	g.nextBlock("b41", outs[11], ticketOuts[11], replaceSpendScript(manySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with more than max allowed signature operations using
	// OP_CHECKSIGVERIFY.
	//
	//   ... -> b41(11)
	//                 \-> b42(12)
	//
	tooManySigOps = repeatOpcode(txscript.OP_CHECKSIGVERIFY, maxBlockSigOps+1)
	g.nextBlock("b42", outs[12], ticketOuts[12], replaceSpendScript(tooManySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// ---------------------------------------------------------------------
	// Spending of tx outputs in block that failed to connect tests.
	// ---------------------------------------------------------------------

	// Create block that spends a transaction from a block that failed to
	// connect (due to containing a double spend).
	//
	//   ... -> b41(11)
	//                 \-> b42(12)
	//                 \-> b43(b42.tx[1])
	//
	g.setTip("b41")
	doubleSpendTx := g.createSpendTx(outs[12], lowFee)
	g.nextBlock("b42", outs[12], ticketOuts[12], additionalPoWTx(doubleSpendTx))
	b42Tx1Out := makeSpendableOut(g.tip, 1, 0)
	// TODO: This really shoud be ErrDoubleSpend
	rejected(blockchain.ErrMissingTx)

	g.setTip("b41")
	g.nextBlock("b43", &b42Tx1Out, ticketOuts[12])
	rejected(blockchain.ErrMissingTx)

	// ---------------------------------------------------------------------
	// Various malformed block tests.
	// ---------------------------------------------------------------------

	// Create block with an otherwise valid transaction in place of where
	// the coinbase must be.
	//
	//   ... -> b41(11)
	//                 \-> b44(12)
	g.nextBlock("b44", nil, ticketOuts[12], func(b *wire.MsgBlock) {
		nonCoinbaseTx := g.createSpendTx(outs[12], lowFee)
		b.Transactions[0] = nonCoinbaseTx
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with no transactions.
	//
	//   ... -> b41(11)
	//                 \-> b45(_)
	g.setTip("b41")
	g.nextBlock("b45", nil, nil, func(b *wire.MsgBlock) {
		b.Transactions = nil
	})
	rejected(blockchain.ErrNoTransactions)

	// Create block with invalid proof of work.
	//
	//   ... -> b41(11)
	//                 \-> b46(12)
	g.setTip("b41")
	b46 := g.nextBlock("b46", outs[12], ticketOuts[12])
	// This can't be done inside a munge function passed to nextBlock
	// because the block is solved after the function returns and this test
	// requires an unsolved block.
	{
		origHash := b46.BlockSha()
		for {
			// Keep incrementing the nonce until the hash treated as
			// a uint256 is higher than the limit.
			b46.Header.Nonce += 1
			hash := b46.BlockSha()
			hashNum := blockchain.ShaHashToBig(&hash)
			if hashNum.Cmp(g.params.PowLimit) >= 0 {
				break
			}
		}
		g.updateBlockState("b46", origHash, "b46", b46)
	}
	rejected(blockchain.ErrHighHash)

	// Create block with a timestamp too far in the future.
	//
	//   ... -> b41(11)
	//                 \-> b47(12)
	g.setTip("b41")
	g.nextBlock("b47", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		// 3 hours in the future clamped to 1 second precision.
		nowPlus3Hours := time.Now().Add(time.Hour * 3)
		b.Header.Timestamp = time.Unix(nowPlus3Hours.Unix(), 0)
	})
	rejected(blockchain.ErrTimeTooNew)

	// Create block with an invalid merkle root.
	//
	//   ... -> b41(11)
	//                 \-> b48(12)
	g.setTip("b41")
	g.nextBlock("b48", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		b.Header.MerkleRoot = chainhash.Hash{}
	})
	g.assertTipBlockMerkleRoot(chainhash.Hash{})
	rejected(blockchain.ErrBadMerkleRoot)

	// Create block with an invalid proof-of-work limit.
	//
	//   ... -> b41(11)
	//                 \-> b49(12)
	g.setTip("b41")
	g.nextBlock("b49", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		b.Header.Bits -= 1
	})
	rejected(blockchain.ErrUnexpectedDifficulty)

	// Create block with two coinbase transactions.
	//
	//   ... -> b41(11)
	//                 \-> b50(12)
	g.setTip("b41")
	coinbaseTx := g.createCoinbaseTx(g.tip.Header.Height+1, ticketsPerBlock)
	g.nextBlock("b50", outs[12], ticketOuts[12], additionalPoWTx(coinbaseTx))
	rejected(blockchain.ErrMultipleCoinbases)

	// Create block with duplicate transactions in the regular transaction
	// tree.
	//
	// This test relies on the shape of the shape of the merkle tree to test
	// the intended condition.  That is the reason for the assertion.
	//
	//   ... -> b41(11)
	//                 \-> b51(12)
	g.setTip("b41")
	g.nextBlock("b51", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		b.AddTransaction(b.Transactions[1])
	})
	g.assertTipBlockNumTxns(3)
	rejected(blockchain.ErrDuplicateTx)

	// Create block with state tx in regular tx tree.
	//
	//   ... -> b41(11)
	//                 \-> bmf0(12)
	g.setTip("b41")
	g.nextBlock("bmf0", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		b.AddTransaction(b.STransactions[1])
	})
	rejected(blockchain.ErrStakeTxInRegularTree)

	// ---------------------------------------------------------------------
	// Block header median time tests.
	// ---------------------------------------------------------------------

	// Create a block with a timestamp that is exactly the median time.
	//
	//   ... b21(6) -> b29(7) -> b36(8) -> b37(9) -> b39(10) -> b41(11)
	//                                                                 \-> b52(12)
	g.setTip("b41")
	g.nextBlock("b52", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		medianBlock := g.blocks[b.Header.PrevBlock]
		for i := 0; i < (medianTimeBlocks / 2); i++ {
			medianBlock = g.blocks[medianBlock.Header.PrevBlock]
		}
		b.Header.Timestamp = medianBlock.Header.Timestamp
	})
	rejected(blockchain.ErrTimeTooOld)

	// Create a block with a timestamp that is one second after the median
	// time.
	//
	//   ... b21(6) -> b29(7) -> b36(8) -> b37(9) -> b39(10) -> b41(11) -> b53(12)
	g.setTip("b41")
	g.nextBlock("b53", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		medianBlock := g.blocks[b.Header.PrevBlock]
		for i := 0; i < (medianTimeBlocks / 2); i++ {
			medianBlock = g.blocks[medianBlock.Header.PrevBlock]
		}
		medianBlockTime := medianBlock.Header.Timestamp
		b.Header.Timestamp = medianBlockTime.Add(time.Second)
	})
	accepted()

	// ---------------------------------------------------------------------
	// Invalid transaction type tests.
	// ---------------------------------------------------------------------

	// Create block with a transaction that tries to spend from an index
	// that is out of range from an otherwise valid and existing tx.
	//
	//   ... -> b53(12)
	//                 \-> b54(13)
	g.setTip("b53")
	g.nextBlock("b54", outs[13], ticketOuts[13], func(b *wire.MsgBlock) {
		b.Transactions[1].TxIn[0].PreviousOutPoint.Index = 12345
	})
	rejected(blockchain.ErrMissingTx)

	// Create block with transaction that pays more than its inputs.
	//
	//   ... -> b53(12)
	//                 \-> b54(13)
	g.setTip("b53")
	g.nextBlock("b54", outs[13], ticketOuts[13], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = int64(outs[13].amount) + 1
	})
	rejected(blockchain.ErrSpendTooHigh)

	// ---------------------------------------------------------------------
	// Blocks with non-final transaction tests.
	// ---------------------------------------------------------------------

	// Create block that contains a non-final non-coinbase transaction.
	//
	//   ... -> b53(12)
	//                 \-> b55(13)
	g.setTip("b53")
	g.nextBlock("b55", outs[13], ticketOuts[13], func(b *wire.MsgBlock) {
		// A non-final transaction must have at least one input with a
		// non-final sequence number in addition to a non-final lock
		// time.
		b.Transactions[1].LockTime = 0xffffffff
		b.Transactions[1].TxIn[0].Sequence = 0
	})
	rejected(blockchain.ErrUnfinalizedTx)

	// Create block that contains a non-final coinbase transaction.
	//
	//   ... -> b53(12)
	//                 \-> b56(13)
	g.setTip("b53")
	g.nextBlock("b56", outs[13], ticketOuts[13], func(b *wire.MsgBlock) {
		// A non-final transaction must have at least one input with a
		// non-final sequence number in addition to a non-final lock
		// time.
		b.Transactions[0].LockTime = 0xffffffff
		b.Transactions[0].TxIn[0].Sequence = 0
	})
	rejected(blockchain.ErrUnfinalizedTx)

	// ---------------------------------------------------------------------
	// Same block transaction spend tests.
	// ---------------------------------------------------------------------

	// Create block that spends an output created earlier in the same block.
	//
	//   ... -> b53(12) -> b57(13)
	g.setTip("b53")
	g.nextBlock("b57", outs[13], ticketOuts[13], func(b *wire.MsgBlock) {
		spendTx3 := makeSpendableOut(b, 1, 0)
		tx3 := g.createSpendTx(&spendTx3, lowFee)
		b.AddTransaction(tx3)
	})
	accepted()

	// Create block that spends an output created later in the same block.
	//
	//   ... -> b57(13)
	//                 \-> b58(14)
	g.nextBlock("b58", nil, ticketOuts[14], func(b *wire.MsgBlock) {
		tx2 := g.createSpendTx(outs[14], lowFee)
		tx3 := g.createSpendTxForTx(tx2, b.Header.Height, 2, lowFee)
		b.AddTransaction(tx3)
		b.AddTransaction(tx2)
	})
	rejected(blockchain.ErrMissingTx)

	// Create block that double spends a transaction created in the same
	// block.
	//
	//   ... -> b57(13)
	//                 \-> b59(14)
	g.setTip("b57")
	g.nextBlock("b59", outs[14], ticketOuts[14], func(b *wire.MsgBlock) {
		tx2 := b.Transactions[1]
		tx3 := g.createSpendTxForTx(tx2, b.Header.Height, 1, lowFee)
		tx4 := g.createSpendTxForTx(tx2, b.Header.Height, 1, lowFee)
		b.AddTransaction(tx3)
		b.AddTransaction(tx4)
	})
	// TODO: This really shoud be ErrDoubleSpend
	rejected(blockchain.ErrMissingTx)

	// ---------------------------------------------------------------------
	// Extra subsidy tests.
	// ---------------------------------------------------------------------

	// Create block that pays 10 extra to the coinbase and a tx that only
	// pays 9 fee.
	//
	//   ... -> b57(13)
	//                 \-> b60(14)
	g.setTip("b57")
	g.nextBlock("b60", outs[14], ticketOuts[14], additionalCoinbasePoW(10),
		additionalSpendFee(9))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create block that pays 10 extra to the coinbase and a tx that pays
	// the extra 10 fee.
	//
	//   ... -> b57(13) -> b61(14)
	g.setTip("b57")
	g.nextBlock("b61", outs[14], ticketOuts[14], additionalCoinbasePoW(10),
		additionalSpendFee(10))
	accepted()

	// ---------------------------------------------------------------------
	// Non-existent transaction spend tests.
	// ---------------------------------------------------------------------

	// Create a block that spends a transaction that does not exist.
	//
	//   ... -> b61(14)
	//                 \-> b62(15)
	g.setTip("b61")
	g.nextBlock("b62", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		hash := newHashFromStr("00000000000000000000000000000000" +
			"00000000000000000123456789abcdef")
		b.Transactions[1].TxIn[0].PreviousOutPoint.Hash = *hash
		b.Transactions[1].TxIn[0].PreviousOutPoint.Index = 0
	})
	rejected(blockchain.ErrMissingTx)

	// ---------------------------------------------------------------------
	// Malformed coinbase tests
	// ---------------------------------------------------------------------

	// Create block with no proof-of-work subsidy output in the coinbase.
	//
	//   ... -> b61(14)
	//                 \-> b63(15)
	g.setTip("b61")
	g.nextBlock("b63", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut = b.Transactions[0].TxOut[0:1]
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with an invalid script type in the coinbase block
	// commitment output.
	//
	//   ... -> b61(14)
	//                 \-> b64(15)
	g.setTip("b61")
	g.nextBlock("b64", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut[1].PkScript = nil
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with too few bytes for the coinbase height commitment.
	//
	//   ... -> b61(14)
	//                 \-> b65(15)
	g.setTip("b61")
	g.nextBlock("b65", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		script := opReturnScript(repeatOpcode(0x00, 3))
		b.Transactions[0].TxOut[1].PkScript = script
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with invalid block height in the coinbase commitment.
	//
	//   ... -> b61(14)
	//                 \-> b66(15)
	g.setTip("b61")
	g.nextBlock("b66", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		script := standardCoinbaseOpReturnScript(b.Header.Height - 1)
		b.Transactions[0].TxOut[1].PkScript = script
	})
	rejected(blockchain.ErrCoinbaseHeight)

	// Create block with a fraudulent transaction (invalid index).
	//
	//   ... -> b61(14)
	//                 \-> b67(15)
	g.setTip("b61")
	g.nextBlock("b67", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[0].TxIn[0].BlockIndex = wire.NullBlockIndex - 1
	})
	rejected(blockchain.ErrBadCoinbaseFraudProof)

	// Create block containing a transaction with no inputs.
	//
	//   ... -> b61(14)
	//                 \-> b68(15)
	g.setTip("b61")
	g.nextBlock("b68", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[1].TxIn = nil
	})
	rejected(blockchain.ErrNoTxInputs)

	// Create block containing a transaction with no outputs.
	//
	//   ... -> b61(14)
	//                 \-> b69(15)
	g.setTip("b61")
	g.nextBlock("b69", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut = nil
	})
	rejected(blockchain.ErrNoTxOutputs)

	// Create block containing a transaction output with negative value.
	//
	//   ... -> b61(14)
	//                 \-> b70(15)
	g.setTip("b61")
	g.nextBlock("b70", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = -1
	})
	rejected(blockchain.ErrBadTxOutValue)

	// Create block containing a transaction output with an exceedingly
	// large (and therefore invalid) value.
	//
	//   ... -> b61(14)
	//                 \-> b71(15)
	g.setTip("b61")
	g.nextBlock("b71", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = dcrutil.MaxAmount + 1
	})
	rejected(blockchain.ErrBadTxOutValue)

	// Create block containing a transaction whose outputs have an
	// exceedingly large (and therefore invalid) total value.
	//
	//   ... -> b61(14)
	//                 \-> b72(15)
	g.setTip("b61")
	g.nextBlock("b72", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = dcrutil.MaxAmount
		b.Transactions[1].TxOut[1].Value = 1
	})
	rejected(blockchain.ErrBadTxOutValue)

	// Create block containing a stakebase tx with a small signature script.
	//
	//   ... -> b61(14)
	//                 \-> b73(15)
	g.setTip("b61")
	tooSmallSbScript := repeatOpcode(0x00, minCoinbaseScriptLen-1)
	g.nextBlock("b73", outs[15], ticketOuts[15], replaceStakeSigScript(tooSmallSbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Create block containing a base stake tx with a large signature script.
	//
	//   ... -> b61(14)
	//                 \-> b74(15)
	g.setTip("b61")
	tooLargeSbScript := repeatOpcode(0x00, maxCoinbaseScriptLen+1)
	g.nextBlock("b74", outs[15], ticketOuts[15], replaceStakeSigScript(tooLargeSbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Create block containing an input transaction with a null outpoint.
	//
	//   ... -> b61(14)
	//                 \-> b75(15)
	g.setTip("b61")
	g.nextBlock("b75", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		tx := b.Transactions[1]
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
				wire.MaxPrevOutIndex, dcrutil.TxTreeRegular)})
	})
	rejected(blockchain.ErrBadTxInput)

	// Create block containing duplicate tx inputs.
	//
	//   ... -> b61(14)
	//                 \-> b76(15)
	g.setTip("b61")
	g.nextBlock("b76", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		tx := b.Transactions[1]
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: b.Transactions[1].TxIn[0].PreviousOutPoint})
	})
	rejected(blockchain.ErrDuplicateTxInputs)

	// Create block with nanosecond precision timestamp.
	//
	//   ... -> b61(14)
	//                 \-> b77(15)
	g.setTip("b61")
	g.nextBlock("b77", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Header.Timestamp = b.Header.Timestamp.Add(1 * time.Nanosecond)
	})
	rejected(blockchain.ErrInvalidTime)

	// Create block with target difficulty that is too low (0 or below).
	//
	//   ... -> b61(14)
	//                 \-> b78(15)
	g.setTip("b61")
	b78 := g.nextBlock("b78", outs[15], ticketOuts[15])
	{
		// This can't be done inside a munge function passed to nextBlock
		// because the block is solved after the function returns and this test
		// involves an unsolvable block.
		b78Hash := b78.BlockSha()
		b78.Header.Bits = 0x01810000 // -1 in compact form.
		g.updateBlockState("b78", b78Hash, "b78", b78)
	}
	rejected(blockchain.ErrUnexpectedDifficulty)

	// Create block with target difficulty that is greater than max allowed.
	//
	//   ... -> b61(14)
	//                 \-> b79(15)
	g.setTip("b61")
	b79 := g.nextBlock("b79", outs[15], ticketOuts[15])
	{
		// This can't be done inside a munge function passed to nextBlock
		// because the block is solved after the function returns and this test
		// involves an improperly solved block.
		b79Hash := b79.BlockSha()
		b79.Header.Bits = g.params.PowLimitBits + 1
		g.updateBlockState("b79", b79Hash, "b79", b79)
	}
	rejected(blockchain.ErrUnexpectedDifficulty)

	// ---------------------------------------------------------------------
	// More signature operation counting tests.
	// ---------------------------------------------------------------------

	// Create block with more than max allowed signature operations such
	// that the signature operation that pushes it over the limit is after
	// a push data with a script element size that is larger than the max
	// allowed size when executed.
	//
	// The script generated consists of the following form:
	//
	//  Comment assumptions:
	//    maxBlockSigOps = 5000
	//    maxScriptElementSize = 2048
	//
	//  [0-4999]   : OP_CHECKSIG
	//  [5000]     : OP_PUSHDATA4
	//  [5001-5004]: 2049 (little-endian encoded maxScriptElementSize+1)
	//  [5005-7053]: too large script element
	//  [7054]     : OP_CHECKSIG (goes over the limit)
	//
	//   ... -> b61(14)
	//                 \-> b80(15)
	g.setTip("b61")
	scriptSize := maxBlockSigOps + 5 + (maxScriptElementSize + 1) + 1
	tooManySigOps = repeatOpcode(txscript.OP_CHECKSIG, scriptSize)
	tooManySigOps[maxBlockSigOps] = txscript.OP_PUSHDATA4
	binary.LittleEndian.PutUint32(tooManySigOps[maxBlockSigOps+1:],
		maxScriptElementSize+1)
	g.nextBlock("b80", outs[15], ticketOuts[15], replaceSpendScript(tooManySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	return tests, nil
}
