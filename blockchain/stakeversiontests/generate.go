// Copyright (c) 2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stakeversiontests

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
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
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
			Hash:  tx.TxHash(),
			Index: txOutIndex,
			Tree:  wire.TxTreeRegular,
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
	iHash := t[i].tx.CachedTxHash()[:]
	jHash := t[j].tx.CachedTxHash()[:]
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
	genesisHash := genesis.BlockHash()
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

// replaceVoteVersions returns a function that itself takes a block and
// modifies it by replacing the voter version of the stake transaction.
func replaceVoteVersions(newVersion uint32) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		for _, stx := range b.STransactions {
			if isVoteTx(stx) {
				stx.TxOut[1].PkScript = voteBitsScript(true, newVersion)
			}
		}
	}
}

// replaceStakeVersion returns a function that itself takes a block and
// modifies it by replacing the stake version of the header.
func replaceStakeVersion(newVersion uint32) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.Header.StakeVersion = newVersion
	}
}

// replaceBlockVersion returns a function that itself takes a block and
// modifies it by replacing the stake version of the header.
func replaceBlockVersion(newVersion int32) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.Header.Version = newVersion
	}
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
	iterations := int64(blockHeight) / g.params.SubsidyReductionInterval
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

// calcPoSSubsidy returns the proof-of-stake subsidy portion for a given block
// height being voted on.
//
// NOTE: This and the other subsidy calculation funcs intentionally are not
// using the blockchain code since the intent is to be able to generate known
// good tests which exercise that code, so it wouldn't make sense to use the
// same code to generate them.
func (g *testGenerator) calcPoSSubsidy(heightVotedOn uint32) dcrutil.Amount {
	if int64(heightVotedOn+1) < g.params.StakeValidationHeight {
		return 0
	}

	fullSubsidy := g.calcFullSubsidy(heightVotedOn)
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
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
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
	parentHash := parentBlock.BlockHash()
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
func voteBitsScript(voteYes bool, voteVersion uint32) []byte {
	bits := uint16(0)
	if voteYes {
		bits = 1
	}

	data := make([]byte, 2+4)
	binary.LittleEndian.PutUint16(data[:], bits)
	binary.LittleEndian.PutUint32(data[2:], voteVersion)
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
	posSubsidy := g.calcPoSSubsidy(parentBlock.Header.Height)
	voteSubsidy := posSubsidy / dcrutil.Amount(g.params.TicketsPerBlock)
	ticketPrice := dcrutil.Amount(ticket.tx.TxOut[0].Value)

	// The first output is the block (hash and height) the vote is for.
	blockScript := voteBlockScript(parentBlock)

	// The second output is the vote bits.
	voteScript := voteBitsScript(true, 3) // XXX find voterversion
	//fmt.Printf("voteScript: %x\n", voteScript)
	//spew.Dump(ticket)

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
	ticketHash := ticket.tx.TxHash()
	tx := wire.NewMsgTx()
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
		Sequence:        wire.MaxTxInSequenceNum,
		ValueIn:         int64(voteSubsidy),
		BlockHeight:     wire.NullBlockHeight,
		BlockIndex:      wire.NullBlockIndex,
		SignatureScript: g.params.StakeBaseSigScript,
	})
	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: *wire.NewOutPoint(&ticketHash, 0,
			wire.TxTreeStake),
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
	// nextPoolSizeDiff := (weightedPoolSizeSum/weightSum * curDiff) >> 32
	nextPoolSizeDiffBig := new(big.Int)
	weightedPoolSizeSumBig := new(big.Int).SetUint64(weightedPoolSizeSum)
	weightSumBig := new(big.Int).SetUint64(weightSum)
	curDiffBig := new(big.Int).SetInt64(curDiff)
	nextPoolSizeDiffBig.Div(weightedPoolSizeSumBig, weightSumBig)
	nextPoolSizeDiffBig.Mul(nextPoolSizeDiffBig, curDiffBig)
	nextPoolSizeDiffBig.Rsh(nextPoolSizeDiffBig, 32)
	nextPoolSizeDiff := nextPoolSizeDiffBig.Int64()
	if nextPoolSizeDiff < 0 {
		panic(fmt.Sprintf("nextPoolSizeDiff overflow %v", nextPoolSizeDiff))
	}
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
	// nextNewTixDiff := (weightedTicketsSum/weightSum * curDiff) >> 32
	nextNewTixDiffBig := new(big.Int)
	weightedTicketsSumBig := new(big.Int).SetUint64(weightedTicketsSum)
	nextNewTixDiffBig.Div(weightedTicketsSumBig, weightSumBig)
	nextNewTixDiffBig.Mul(nextNewTixDiffBig, curDiffBig)
	nextNewTixDiffBig.Rsh(nextNewTixDiffBig, 32)
	nextNewTixDiff := nextNewTixDiffBig.Int64()
	if nextNewTixDiff < 0 {
		panic(fmt.Sprintf("nextNewTixDiff overflow %v", nextNewTixDiff))
	}
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
	return chainhash.HashH(finalState)
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
		hp.cachedHash = chainhash.HashH(data)
		hp.idx++
		hp.hashOffset = 0
	}

	// Roll over the entire PRNG by re-hashing the seed when the hash
	// iterator index overlows a uint32.
	if hp.idx > math.MaxUint32 {
		hp.seed = chainhash.HashH(hp.seed[:])
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
			delete(allTickets, winner.tx.TxHash())
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
				allTickets[tx.TxHash()] = ticket
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
		h := winners[i].tx.TxHash()
		copy(data[chainhash.HashSize*i:], h[:])
	}
	copy(data[chainhash.HashSize*len(winners):], prngStateHash[:])
	dataHash := chainhash.HashH(data)

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
				hash := hdr.BlockHash()
				if blockchain.HashToBig(&hash).Cmp(
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
			PrevBlock:    g.tip.BlockHash(),
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
	blockHash := block.BlockHash()
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
			wire.MaxPrevOutIndex, wire.TxTreeRegular),
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
	g.prevCollectedHash = g.tip.BlockHash()
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

// assertStakeVersion panics if the current tip block associated with the
// generator does not have the specified stake version in the header.
func (g *testGenerator) assertStakeVersion(expected uint32) {
	stakeVersion := g.tip.Header.StakeVersion
	if stakeVersion != expected {
		panic(fmt.Sprintf("stake version for block %q is %d instead of "+
			"expected %d", g.tipName, stakeVersion, expected))
	}
}

// assertBlockVersion panics if the current tip block associated with the
// generator does not have the specified version.
func (g *testGenerator) assertBlockVersion(expected int32) {
	blockVersion := g.tip.Header.Version
	if blockVersion != expected {
		panic(fmt.Sprintf("block version for block %q is %d instead of "+
			"expected %d", g.tipName, blockVersion, expected))
	}
}

// Generate returns a slice of tests that can be used to exercise the consensus
// validation rules.  The tests are intended to be flexible enough to allow both
// unit-style tests directly against the blockchain code as well as
// integration-style tests over the peer-to-peer network.  To achieve that goal,
// each test contains additional information about the expected result, however
// that information can be ignored when doing comparison tests between to
// independent versions over the peer-to-peer network.
func Generate() (tests []TestInstance, err error) {
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
	acceptBlock := func(blockName string, block *wire.MsgBlock, isMainChain, isOrphan bool) TestInstance {
		return AcceptedBlock{blockName, block, isMainChain, isOrphan}
	}
	rejectBlock := func(blockName string, block *wire.MsgBlock, code blockchain.ErrorCode) TestInstance {
		return RejectedBlock{blockName, block, code}
	}

	// Define some convenience helper functions to populate the tests slice
	// with test instances that have the described characteristics.
	//
	// accepted creates and appends a single acceptBlock test instance for
	// the current tip which expects the block to be accepted to the main
	// chain.
	//
	// rejected creates and appends a single rejectBlock test instance for
	// the current tip.
	accepted := func() {
		tests = append(tests, acceptBlock(g.tipName, g.tip, true, false))
	}
	rejected := func(code blockchain.ErrorCode) {
		tests = append(tests, rejectBlock(g.tipName, g.tip, code))
	}

	// Shorter versions of useful params for convenience.
	ticketsPerBlock := g.params.TicketsPerBlock
	coinbaseMaturity := g.params.CoinbaseMaturity
	stakeEnabledHeight := g.params.StakeEnabledHeight
	stakeValidationHeight := g.params.StakeValidationHeight
	stakeVerInterval := g.params.StakeVersionInterval
	stakeMajorityMul := int64(g.params.StakeMajorityMultiplier)
	stakeMajorityDiv := int64(g.params.StakeMajorityDivisor)

	// ---------------------------------------------------------------------
	// Premine tests.
	// ---------------------------------------------------------------------

	// Add the required premine block.
	//
	//   genesis -> bp
	g.createPremineBlock("bp", 0)
	g.assertTipHeight(1)
	accepted()

	// ---------------------------------------------------------------------
	// Generate enough blocks to have mature coinbase outputs to work with.
	//
	//   genesis -> bp -> bm0 -> bm1 -> ... -> bm#
	// ---------------------------------------------------------------------

	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bm%d", i)
		g.nextBlock(blockName, nil, nil)
		g.saveTipCoinbaseOuts()
		accepted()
	}
	g.assertTipHeight(uint32(coinbaseMaturity) + 1)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake enabled height while
	// creating ticket purchases that spend from the coinbases matured
	// above.  This will also populate the pool of immature tickets.
	//
	//   ... -> bm# ... -> bse0 -> bse1 -> ... -> bse#
	// ---------------------------------------------------------------------

	var ticketsPurchased int
	for i := int64(0); int64(g.tip.Header.Height) < stakeEnabledHeight; i++ {
		outs := g.oldestCoinbaseOuts()
		ticketOuts := outs[1:]
		ticketsPurchased += len(ticketOuts)
		blockName := fmt.Sprintf("bse%d", i)
		g.nextBlock(blockName, nil, ticketOuts)
		g.saveTipCoinbaseOuts()
		accepted()
	}
	g.assertTipHeight(uint32(stakeEnabledHeight))

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake validation height while
	// continuing to purchase tickets using the coinbases matured above and
	// allowing the immature tickets to mature and thus become live.
	// ---------------------------------------------------------------------

	targetPoolSize := g.params.TicketPoolSize * ticketsPerBlock
	for i := int64(0); int64(g.tip.Header.Height) < stakeValidationHeight; i++ {
		// Only purchase tickets until the target ticket pool size is
		// reached.
		outs := g.oldestCoinbaseOuts()
		ticketOuts := outs[1:]
		if ticketsPurchased+len(ticketOuts) > int(targetPoolSize) {
			ticketsNeeded := int(targetPoolSize) - ticketsPurchased
			if ticketsNeeded > 0 {
				ticketOuts = ticketOuts[1 : ticketsNeeded+1]
			} else {
				ticketOuts = nil
			}
		}
		ticketsPurchased += len(ticketOuts)

		blockName := fmt.Sprintf("bsv%d", i)
		g.nextBlock(blockName, nil, ticketOuts)
		g.saveTipCoinbaseOuts()
		accepted()
	}
	g.assertTipHeight(uint32(stakeValidationHeight))

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 2, stake version 0, and vote
	// version 3.
	//
	// This will result in a majority of blocks with a version prior to
	// version 3 where stake version enforcement begins and thus it must not
	// be enforced.
	// ---------------------------------------------------------------------

	for i := int64(0); i < stakeVerInterval-1; i++ {
		outs := g.oldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtA%d", i)
		g.nextBlock(blockName, nil, outs[1:],
			replaceBlockVersion(2),
			replaceStakeVersion(0),
			replaceVoteVersions(3))
		g.saveTipCoinbaseOuts()
		accepted()
	}
	g.assertTipHeight(uint32(stakeValidationHeight + stakeVerInterval - 1))
	g.assertBlockVersion(2)
	g.assertStakeVersion(0)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 42, and
	// vote version 41.
	//
	// This block must be accepted because even though it is a version 3
	// block with an invalid stake version, there have not yet been a
	// majority of version 3 blocks which is required to trigger stake
	// version enforcement.
	// ---------------------------------------------------------------------

	outs := g.oldestCoinbaseOuts()
	g.nextBlock("bsvtB0", nil, outs[1:],
		replaceBlockVersion(3),
		replaceStakeVersion(42),
		replaceVoteVersions(41))
	g.saveTipCoinbaseOuts()
	accepted()
	g.assertTipHeight(uint32(stakeValidationHeight + stakeVerInterval))
	g.assertBlockVersion(3)
	g.assertStakeVersion(42) // expected bogus

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 3, stake version 0, and vote
	// version 2.
	//
	// This will result in a majority of version 3 blocks which will trigger
	// enforcement of the stake version.  It also results in a majority of
	// version 2 votes, however, since enforcement is not yet active in this
	// interval, they will not actually count toward establishing a
	// majority.
	// ---------------------------------------------------------------------

	for i := int64(0); i < stakeVerInterval-1; i++ {
		outs := g.oldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtB%d", i+1)
		g.nextBlock(blockName, nil, outs[1:],
			replaceBlockVersion(3),
			replaceStakeVersion(0),
			replaceVoteVersions(2))
		g.saveTipCoinbaseOuts()
		accepted()
	}
	g.assertTipHeight(uint32(stakeValidationHeight + 2*stakeVerInterval - 1))
	g.assertBlockVersion(3)
	g.assertStakeVersion(0)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 2, and
	// vote version 3.
	//
	// This block must be rejected because even though the majority stake
	// version per voters was 2 in the previous period, stake version
	// enforcement had not yet been achieved and thus the required stake
	// version is still 0.
	// ---------------------------------------------------------------------

	g.nextBlock("bsvtCbad0", nil, nil,
		replaceBlockVersion(3),
		replaceStakeVersion(2),
		replaceVoteVersions(3))
	g.assertTipHeight(uint32(stakeValidationHeight + 2*stakeVerInterval))
	g.assertBlockVersion(3)
	g.assertStakeVersion(2)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 1, and
	// vote version 3.
	//
	// This block must be rejected because even though the majority stake
	// version per voters was 2 in the previous period, stake version
	// enforcement had not yet been achieved and thus the required stake
	// version is still 0.
	// ---------------------------------------------------------------------

	g.setTip(fmt.Sprintf("bsvtB%d", stakeVerInterval-1))
	g.nextBlock("bsvtCbad1", nil, nil,
		replaceBlockVersion(3),
		replaceStakeVersion(1),
		replaceVoteVersions(3))
	g.assertTipHeight(uint32(stakeValidationHeight + 2*stakeVerInterval))
	g.assertBlockVersion(3)
	g.assertStakeVersion(1)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 3, stake version 0, and vote
	// version 3.
	//
	// This will result in a majority of version 3 votes which will trigger
	// enforcement of a bump in the stake version to 3.
	// ---------------------------------------------------------------------

	g.setTip(fmt.Sprintf("bsvtB%d", stakeVerInterval-1))
	for i := int64(0); i < stakeVerInterval; i++ {
		outs := g.oldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtC%d", i)
		g.nextBlock(blockName, nil, outs[1:],
			replaceBlockVersion(3),
			replaceStakeVersion(0),
			replaceVoteVersions(3))
		g.saveTipCoinbaseOuts()
		accepted()
	}
	g.assertTipHeight(uint32(stakeValidationHeight + 3*stakeVerInterval - 1))
	g.assertBlockVersion(3)
	g.assertStakeVersion(0)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 2, and
	// vote version 2.
	//
	// This block must be rejected because the majority stake version per
	// voters is now 3 and stake version enforcement has been achieved.
	// ---------------------------------------------------------------------

	g.nextBlock("bsvtDbad0", nil, nil,
		replaceBlockVersion(3),
		replaceStakeVersion(2),
		replaceVoteVersions(2))
	g.assertTipHeight(uint32(stakeValidationHeight + 3*stakeVerInterval))
	g.assertBlockVersion(3)
	g.assertStakeVersion(2)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 4, and
	// vote version 2.
	//
	// This block must be rejected because the majority stake version per
	// voters is now 3 and stake version enforcement has been achieved.
	// ---------------------------------------------------------------------

	g.setTip(fmt.Sprintf("bsvtC%d", stakeVerInterval-1))
	g.nextBlock("bsvtDbad1", nil, nil,
		replaceBlockVersion(3),
		replaceStakeVersion(4),
		replaceVoteVersions(2))
	g.assertTipHeight(uint32(stakeValidationHeight + 3*stakeVerInterval))
	g.assertBlockVersion(3)
	g.assertStakeVersion(4)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 3, stake version 3, and vote
	// version 2.
	//
	// This will result in a majority of version 2 votes, but since version
	// 3 has already been achieved, the stake version must not regress.
	// ---------------------------------------------------------------------

	g.setTip(fmt.Sprintf("bsvtC%d", stakeVerInterval-1))
	for i := int64(0); i < stakeVerInterval; i++ {
		outs := g.oldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtD%d", i)
		g.nextBlock(blockName, nil, outs[1:],
			replaceBlockVersion(3),
			replaceStakeVersion(3),
			replaceVoteVersions(2))
		g.saveTipCoinbaseOuts()
		accepted()
	}
	g.assertTipHeight(uint32(stakeValidationHeight + 4*stakeVerInterval - 1))
	g.assertBlockVersion(3)
	g.assertStakeVersion(3)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 2, and
	// vote version 2.
	//
	// This block must be rejected because even though the majority stake
	// version per voters in the previous interval was 2, the majority stake
	// version is not allowed to regress.
	// ---------------------------------------------------------------------

	g.nextBlock("bsvtEbad0", nil, nil,
		replaceBlockVersion(3),
		replaceStakeVersion(2),
		replaceVoteVersions(2))
	g.assertTipHeight(uint32(stakeValidationHeight + 4*stakeVerInterval))
	g.assertBlockVersion(3)
	g.assertStakeVersion(2)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 4, and
	// vote version 3.
	//
	// This block must be rejected because the majority stake version is
	// still 3.
	// ---------------------------------------------------------------------

	g.setTip(fmt.Sprintf("bsvtD%d", stakeVerInterval-1))
	g.nextBlock("bsvtEbad1", nil, nil,
		replaceBlockVersion(3),
		replaceStakeVersion(4),
		replaceVoteVersions(3))
	g.assertTipHeight(uint32(stakeValidationHeight + 4*stakeVerInterval))
	g.assertBlockVersion(3)
	g.assertStakeVersion(4)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 3, stake version 3, and a mix of
	// 3 and 4 for the vote version such that a super majority is *NOT*
	// achieved.
	//
	// This will result in an unchanged required stake version.
	// ---------------------------------------------------------------------

	g.setTip(fmt.Sprintf("bsvtD%d", stakeVerInterval-1))
	votesPerInterval := stakeVerInterval * int64(ticketsPerBlock)
	targetVotes := (votesPerInterval * stakeMajorityMul) / stakeMajorityDiv
	targetBlocks := targetVotes / int64(ticketsPerBlock)
	for i := int64(0); i < targetBlocks-1; i++ {
		outs := g.oldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtE%da", i)
		g.nextBlock(blockName, nil, outs[1:],
			replaceBlockVersion(3),
			replaceStakeVersion(3),
			replaceVoteVersions(4))
		g.saveTipCoinbaseOuts()
		g.assertBlockVersion(3)
		g.assertStakeVersion(3)
		accepted()
	}
	for i := int64(0); i < stakeVerInterval-(targetBlocks-1); i++ {
		outs := g.oldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtE%db", targetBlocks-1+i)
		g.nextBlock(blockName, nil, outs[1:],
			replaceBlockVersion(3),
			replaceStakeVersion(3),
			replaceVoteVersions(3))
		g.saveTipCoinbaseOuts()
		g.assertBlockVersion(3)
		g.assertStakeVersion(3)
		accepted()
	}
	g.assertTipHeight(uint32(stakeValidationHeight + 5*stakeVerInterval - 1))

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 4, and
	// vote version 3.
	//
	// This block must be rejected because the majority stake version is
	// still 3 due to failing to achieve enough votes in the previous
	// period.
	// ---------------------------------------------------------------------

	g.nextBlock("bsvtFbad0", nil, nil,
		replaceBlockVersion(3),
		replaceStakeVersion(4),
		replaceVoteVersions(3))
	g.assertTipHeight(uint32(stakeValidationHeight + 5*stakeVerInterval))
	g.assertBlockVersion(3)
	g.assertStakeVersion(4)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach one block before the next stake
	// version interval with block version 3, stake version 3, and a mix of
	// 3 and 4 for the vote version such that a super majority of version 4
	// is achieved.
	//
	// This will result in a majority stake version of 4.
	// ---------------------------------------------------------------------

	g.setTip(fmt.Sprintf("bsvtE%db", stakeVerInterval-1))
	for i := int64(0); i < targetBlocks; i++ {
		outs := g.oldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtF%da", i)
		g.nextBlock(blockName, nil, outs[1:],
			replaceBlockVersion(3),
			replaceStakeVersion(3),
			replaceVoteVersions(4))
		g.saveTipCoinbaseOuts()
		g.assertBlockVersion(3)
		g.assertStakeVersion(3)
		accepted()
	}
	for i := int64(0); i < stakeVerInterval-targetBlocks; i++ {
		outs := g.oldestCoinbaseOuts()
		blockName := fmt.Sprintf("bsvtF%db", targetBlocks+i)
		g.nextBlock(blockName, nil, outs[1:],
			replaceBlockVersion(3),
			replaceStakeVersion(3),
			replaceVoteVersions(3))
		g.saveTipCoinbaseOuts()
		g.assertBlockVersion(3)
		g.assertStakeVersion(3)
		accepted()
	}
	g.assertTipHeight(uint32(stakeValidationHeight + 6*stakeVerInterval - 1))

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 3, and
	// vote version 3.
	//
	// This block must be rejected because the majority stake version is
	// now 4 due to achieving a majority of votes in the previous period.
	// ---------------------------------------------------------------------

	g.nextBlock("bsvtGbad0", nil, nil,
		replaceBlockVersion(3),
		replaceStakeVersion(3),
		replaceVoteVersions(3))
	g.assertTipHeight(uint32(stakeValidationHeight + 6*stakeVerInterval))
	g.assertBlockVersion(3)
	g.assertStakeVersion(3)
	rejected(blockchain.ErrBadStakeVersion)

	// ---------------------------------------------------------------------
	// Generate a single block with block version 3, stake version 4, and
	// vote version 3.
	//
	// This block must be accepted because the majority stake version is
	// now 4 due to achieving a majority of votes in the previous period.
	// ---------------------------------------------------------------------

	g.setTip(fmt.Sprintf("bsvtF%db", stakeVerInterval-1))
	g.nextBlock("bsvtG0", nil, nil,
		replaceBlockVersion(3),
		replaceStakeVersion(4),
		replaceVoteVersions(3))
	g.assertTipHeight(uint32(stakeValidationHeight + 6*stakeVerInterval))
	g.assertBlockVersion(3)
	g.assertStakeVersion(4)
	accepted()

	return tests, nil
}
