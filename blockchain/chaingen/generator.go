// Copyright (c) 2016-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package chaingen

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"math/big"
	"runtime"
	"sort"
	"time"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

var (
	// hash256prngSeedConst is a constant derived from the hex
	// representation of pi and is used in conjunction with a caller-provided
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

const (
	// voteBitYes is the specific bit that is set in the vote bits to
	// indicate that the previous block is valid.
	voteBitYes = 0x01
)

// SpendableOut represents a transaction output that is spendable along with
// additional metadata such as the block its in and how much it pays.
type SpendableOut struct {
	prevOut     wire.OutPoint
	blockHeight uint32
	blockIndex  uint32
	amount      dcrutil.Amount
}

// PrevOut returns the outpoint associated with the spendable output.
func (s *SpendableOut) PrevOut() wire.OutPoint {
	return s.prevOut
}

// BlockHeight returns the block height of the block the spendable output is in.
func (s *SpendableOut) BlockHeight() uint32 {
	return s.blockHeight
}

// BlockIndex returns the offset into the block the spendable output is in.
func (s *SpendableOut) BlockIndex() uint32 {
	return s.blockIndex
}

// Amount returns the amount associated with the spendable output.
func (s *SpendableOut) Amount() dcrutil.Amount {
	return s.amount
}

// MakeSpendableOutForTx returns a spendable output for the given transaction
// block height, transaction index within the block, and transaction output
// index within the transaction.
func MakeSpendableOutForTx(tx *wire.MsgTx, blockHeight, txIndex, txOutIndex uint32) SpendableOut {
	return SpendableOut{
		prevOut: wire.OutPoint{
			Hash:  *tx.CachedTxHash(),
			Index: txOutIndex,
			Tree:  wire.TxTreeRegular,
		},
		blockHeight: blockHeight,
		blockIndex:  txIndex,
		amount:      dcrutil.Amount(tx.TxOut[txOutIndex].Value),
	}
}

// MakeSpendableOut returns a spendable output for the given block, transaction
// index within the block, and transaction output index within the transaction.
func MakeSpendableOut(block *wire.MsgBlock, txIndex, txOutIndex uint32) SpendableOut {
	tx := block.Transactions[txIndex]
	return MakeSpendableOutForTx(tx, block.Header.Height, txIndex, txOutIndex)
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

// Generator houses state used to ease the process of generating test blocks
// that build from one another along with housing other useful things such as
// available spendable outputs and generic payment scripts used throughout the
// tests.
type Generator struct {
	params           *chaincfg.Params
	tip              *wire.MsgBlock
	tipName          string
	blocks           map[chainhash.Hash]*wire.MsgBlock
	blocksByName     map[string]*wire.MsgBlock
	p2shOpTrueAddr   dcrutil.Address
	p2shOpTrueScript []byte

	// Used for tracking spendable coinbase outputs.
	spendableOuts     [][]SpendableOut
	prevCollectedHash chainhash.Hash

	// Used for tracking the live ticket pool.
	originalParents map[chainhash.Hash]chainhash.Hash
	immatureTickets []*stakeTicket
	liveTickets     []*stakeTicket
	wonTickets      map[chainhash.Hash][]*stakeTicket
	expiredTickets  []*stakeTicket
}

// MakeGenerator returns a generator instance initialized with the genesis block
// as the tip as well as a cached generic pay-to-script-hash script for OP_TRUE.
func MakeGenerator(params *chaincfg.Params) (Generator, error) {
	// Generate a generic pay-to-script-hash script that is a simple
	// OP_TRUE.  This allows the tests to avoid needing to generate and
	// track actual public keys and signatures.
	p2shOpTrueAddr, err := dcrutil.NewAddressScriptHash(opTrueScript, params)
	if err != nil {
		return Generator{}, err
	}
	p2shOpTrueScript, err := txscript.PayToAddrScript(p2shOpTrueAddr)
	if err != nil {
		return Generator{}, err
	}

	genesis := params.GenesisBlock
	genesisHash := genesis.BlockHash()
	return Generator{
		params:           params,
		tip:              genesis,
		tipName:          "genesis",
		blocks:           map[chainhash.Hash]*wire.MsgBlock{genesisHash: genesis},
		blocksByName:     map[string]*wire.MsgBlock{"genesis": genesis},
		p2shOpTrueAddr:   p2shOpTrueAddr,
		p2shOpTrueScript: p2shOpTrueScript,
		originalParents:  make(map[chainhash.Hash]chainhash.Hash),
		wonTickets:       make(map[chainhash.Hash][]*stakeTicket),
	}, nil
}

// Params returns the chain params associated with the generator instance.
func (g *Generator) Params() *chaincfg.Params {
	return g.params
}

// Tip returns the current tip block of the generator instance.
func (g *Generator) Tip() *wire.MsgBlock {
	return g.tip
}

// TipName returns the name of the current tip block of the generator instance.
func (g *Generator) TipName() string {
	return g.tipName
}

// BlockByName returns the block associated with the provided block name.  It
// will panic if the specified block name does not exist.
func (g *Generator) BlockByName(blockName string) *wire.MsgBlock {
	block, ok := g.blocksByName[blockName]
	if !ok {
		panic(fmt.Sprintf("block name %s does not exist", blockName))
	}
	return block
}

// BlockByHash returns the block associated with the provided block hash.  It
// will panic if the specified block hash does not exist.
func (g *Generator) BlockByHash(hash *chainhash.Hash) *wire.MsgBlock {
	block, ok := g.blocks[*hash]
	if !ok {
		panic(fmt.Sprintf("block with hash %s does not exist", hash))
	}
	return block
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

// UniqueOpReturnScript returns a standard provably-pruneable OP_RETURN script
// with a random uint64 encoded as the data.
func UniqueOpReturnScript() []byte {
	rand, err := wire.RandomUint64()
	if err != nil {
		panic(err)
	}

	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data[0:8], rand)
	return opReturnScript(data)
}

// calcFullSubsidy returns the full block subsidy for the given block height.
//
// NOTE: This and the other subsidy calculation funcs intentionally are not
// using the blockchain code since the intent is to be able to generate known
// good tests which exercise that code, so it wouldn't make sense to use the
// same code to generate them.
func (g *Generator) calcFullSubsidy(blockHeight uint32) dcrutil.Amount {
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
func (g *Generator) calcPoWSubsidy(fullSubsidy dcrutil.Amount, blockHeight uint32, numVotes uint16) dcrutil.Amount {
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
func (g *Generator) calcPoSSubsidy(heightVotedOn uint32) dcrutil.Amount {
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
func (g *Generator) calcDevSubsidy(fullSubsidy dcrutil.Amount, blockHeight uint32, numVotes uint16) dcrutil.Amount {
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
// height is not defined however this effectively mirrors the actual mining code
// at the time it was written.
func standardCoinbaseOpReturnScript(blockHeight uint32) []byte {
	rand, err := wire.RandomUint64()
	if err != nil {
		panic(err)
	}

	data := make([]byte, 36)
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
func (g *Generator) addCoinbaseTxOutputs(tx *wire.MsgTx, blockHeight uint32, devSubsidy, powSubsidy dcrutil.Amount) {
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
	// output.  These are in turn used throughout the tests as inputs to
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

// CreateCoinbaseTx returns a coinbase transaction paying an appropriate
// subsidy based on the passed block height and number of votes to the dev org
// and proof-of-work miner.
//
// See the addCoinbaseTxOutputs documentation for a breakdown of the outputs
// the transaction contains.
func (g *Generator) CreateCoinbaseTx(blockHeight uint32, numVotes uint16) *wire.MsgTx {
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
	binary.LittleEndian.PutUint16(data[28:], limits)
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
func (g *Generator) createTicketPurchaseTx(spend *SpendableOut, ticketPrice, fee dcrutil.Amount) *wire.MsgTx {
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
// suitable for use in a vote tx (ssgen) given the block hash and height to vote
// on.
func voteCommitmentScript(hash chainhash.Hash, height uint32) []byte {
	// The vote commitment consists of a 32-byte hash of the block it is
	// voting on along with its expected height as a 4-byte little-endian
	// uint32.  32-byte hash + 4-byte uint32 = 36 bytes.
	var data [36]byte
	copy(data[:], hash[:])
	binary.LittleEndian.PutUint32(data[32:], height)
	script, err := txscript.NewScriptBuilder().AddOp(txscript.OP_RETURN).
		AddData(data[:]).Script()
	if err != nil {
		panic(err)
	}
	return script
}

// voteBlockScript returns a standard provably-pruneable OP_RETURN script
// suitable for use in a vote tx (ssgen) given the block to vote on.
func voteBlockScript(parentBlock *wire.MsgBlock) []byte {
	return voteCommitmentScript(parentBlock.BlockHash(),
		parentBlock.Header.Height)
}

// voteBitsScript returns a standard provably-pruneable OP_RETURN script
// suitable for use in a vote tx (ssgen) with the appropriate vote bits set
// depending on the provided params.
func voteBitsScript(bits uint16, voteVersion uint32) []byte {
	data := make([]byte, 6)
	binary.LittleEndian.PutUint16(data[:], bits)
	binary.LittleEndian.PutUint32(data[2:], voteVersion)
	if voteVersion == 0 {
		data = data[:2]
	}
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
func (g *Generator) createVoteTx(parentBlock *wire.MsgBlock, ticket *stakeTicket) *wire.MsgTx {
	// Calculate the proof-of-stake subsidy proportion based on the block
	// height.
	posSubsidy := g.calcPoSSubsidy(parentBlock.Header.Height)
	voteSubsidy := posSubsidy / dcrutil.Amount(g.params.TicketsPerBlock)
	ticketPrice := dcrutil.Amount(ticket.tx.TxOut[0].Value)

	// The first output is the block (hash and height) the vote is for.
	blockScript := voteBlockScript(parentBlock)

	// The second output is the vote bits.
	voteScript := voteBitsScript(voteBitYes, 0)

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
	ticketHash := ticket.tx.CachedTxHash()
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
		PreviousOutPoint: *wire.NewOutPoint(ticketHash, 0,
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
func (g *Generator) ancestorBlock(block *wire.MsgBlock, height uint32, f func(*wire.MsgBlock)) *wire.MsgBlock {
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
func (g *Generator) limitRetarget(oldDiff, newDiff int64) int64 {
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
func (g *Generator) calcNextRequiredDifficulty() uint32 {
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

	// Calculate the retarget difficulty based on the exponential weighted
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
func (g *Generator) calcNextRequiredStakeDifficulty() int64 {
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
	// weighted average and shift the result back down 32 bits to account
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
		min = 1 + ^upperBound
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
	buf.Grow(wire.MaxBlockHeaderPayload)
	if err := voteBlock.Header.Serialize(&buf); err != nil {
		return nil, chainhash.Hash{}, err
	}

	// Ensure the number of live tickets is within the allowable range.
	numLiveTickets := uint32(len(liveTickets))
	if numLiveTickets > math.MaxUint32 {
		return nil, chainhash.Hash{}, fmt.Errorf("live ticket pool "+
			"has %d tickets which is more than the max allowed of "+
			"%d", len(liveTickets), uint32(math.MaxUint32))
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

// calcFinalLotteryState calculates the final lottery state for a set of winning
// tickets and the associated deterministic prng state hash after selecting the
// winners.  It is the first 6 bytes of:
//   blake256(firstTicketHash || ... || lastTicketHash || prngStateHash)
func calcFinalLotteryState(winners []*stakeTicket, prngStateHash chainhash.Hash) [6]byte {
	data := make([]byte, (len(winners)+1)*chainhash.HashSize)
	for i := 0; i < len(winners); i++ {
		h := winners[i].tx.CachedTxHash()
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
// so the 'NextBlock' function can properly detect when a nonce was modified by
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
				results <- sbResult{false, 0}
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
	var foundResult bool
	for i := uint32(0); i < numCores; i++ {
		result := <-results
		if !foundResult && result.found {
			close(quit)
			header.Nonce = result.nonce
			foundResult = true
		}
	}

	return foundResult
}

// ReplaceWithNVotes returns a function that itself takes a block and modifies
// it by replacing the votes in the stake tree with specified number of votes.
//
// NOTE: This must only be used as a munger to the 'NextBlock' function or it
// will lead to an invalid live ticket pool.  To help safeguard against improper
// usage, it will panic if called with a block that does not connect to the
// current tip block.
func (g *Generator) ReplaceWithNVotes(numVotes uint16) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		// Attempt to prevent misuse of this function by ensuring the
		// provided block connects to the current tip.
		if b.Header.PrevBlock != g.tip.BlockHash() {
			panic(fmt.Sprintf("attempt to replace number of votes "+
				"for block %s with parent %s that is not the "+
				"current tip %s", b.BlockHash(),
				b.Header.PrevBlock, g.tip.BlockHash()))
		}

		// Get the winning tickets for the specified number of votes.
		parentBlock := g.tip
		winners, _, err := winningTickets(parentBlock, g.liveTickets,
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
		stakeTxns = append(stakeTxns, b.STransactions[defaultNumVotes:]...)

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

// ReplaceBlockVersion returns a function that itself takes a block and modifies
// it by replacing the stake version of the header.
func ReplaceBlockVersion(newVersion int32) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.Header.Version = newVersion
	}
}

// ReplaceStakeVersion returns a function that itself takes a block and modifies
// it by replacing the stake version of the header.
func ReplaceStakeVersion(newVersion uint32) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.Header.StakeVersion = newVersion
	}
}

// ReplaceVoteVersions returns a function that itself takes a block and modifies
// it by replacing the voter version of the stake transactions.
//
// NOTE: This must only be used as a munger to the 'NextBlock' function or it
// will lead to an invalid live ticket pool.
func ReplaceVoteVersions(newVersion uint32) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		for _, stx := range b.STransactions {
			if isVoteTx(stx) {
				stx.TxOut[1].PkScript = voteBitsScript(
					voteBitYes, newVersion)
			}
		}
	}
}

// ReplaceVotes returns a function that itself takes a block and modifies it by
// replacing the voter version and bits of the stake transactions.
//
// NOTE: This must only be used as a munger to the 'NextBlock' function or it
// will lead to an invalid live ticket pool.
func ReplaceVotes(voteBits uint16, newVersion uint32) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		for _, stx := range b.STransactions {
			if isVoteTx(stx) {
				stx.TxOut[1].PkScript = voteBitsScript(voteBits,
					newVersion)
			}
		}
	}
}

// CreateSpendTx creates a transaction that spends from the provided spendable
// output and includes an additional unique OP_RETURN output to ensure the
// transaction ends up with a unique hash.  The public key script is a simple
// OP_TRUE p2sh script which avoids the need to track addresses and signature
// scripts in the tests.  The signature script is the opTrueRedeemScript.
func (g *Generator) CreateSpendTx(spend *SpendableOut, fee dcrutil.Amount) *wire.MsgTx {
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
	spendTx.AddTxOut(wire.NewTxOut(0, UniqueOpReturnScript()))
	return spendTx
}

// CreateSpendTxForTx creates a transaction that spends from the first output of
// the provided transaction and includes an additional unique OP_RETURN output
// to ensure the transaction ends up with a unique hash.  The public key script
// is a simple OP_TRUE p2sh script which avoids the need to track addresses and
// signature scripts in the tests.  This signature script the
// opTrueRedeemScript.
func (g *Generator) CreateSpendTxForTx(tx *wire.MsgTx, blockHeight, txIndex uint32, fee dcrutil.Amount) *wire.MsgTx {
	spend := MakeSpendableOutForTx(tx, blockHeight, txIndex, 0)
	return g.CreateSpendTx(&spend, fee)
}

// removeTicket removes the passed index from the provided slice of tickets and
// returns the resulting slice.  This is an in-place modification.
func removeTicket(tickets []*stakeTicket, index int) []*stakeTicket {
	copy(tickets[index:], tickets[index+1:])
	tickets[len(tickets)-1] = nil // Prevent memory leak
	tickets = tickets[:len(tickets)-1]
	return tickets
}

// connectLiveTickets updates the live ticket pool for a new tip block by
// removing tickets that are now expired from it, removing the passed winners
// from it, adding any immature tickets which are now mature to it, and
// resorting it.
func (g *Generator) connectLiveTickets(blockHash *chainhash.Hash, height uint32, winners, purchases []*stakeTicket) {
	// Move expired tickets from the live ticket pool to the expired ticket
	// pool.
	ticketMaturity := uint32(g.params.TicketMaturity)
	ticketExpiry := g.params.TicketExpiry
	for i := 0; i < len(g.liveTickets); i++ {
		ticket := g.liveTickets[i]
		liveHeight := ticket.blockHeight + ticketMaturity
		expireHeight := liveHeight + ticketExpiry
		if height >= expireHeight {
			g.liveTickets = removeTicket(g.liveTickets, i)
			g.expiredTickets = append(g.expiredTickets, ticket)

			// This is required because the ticket at the current
			// offset was just removed from the slice that is being
			// iterated, so adjust the offset down one accordingly.
			i--
		}
	}

	// Move winning tickets from the live ticket pool to won tickets pool.
	for i := 0; i < len(g.liveTickets); i++ {
		ticket := g.liveTickets[i]
		for _, winner := range winners {
			if ticket.tx.CachedTxHash() == winner.tx.CachedTxHash() {
				g.liveTickets = removeTicket(g.liveTickets, i)

				// This is required because the ticket at the
				// current offset was just removed from the
				// slice that is being iterated, so adjust the
				// offset down one accordingly.
				i--
				break
			}
		}
	}
	g.wonTickets[*blockHash] = winners

	// Move immature tickets which are now mature to the live ticket pool.
	for i := 0; i < len(g.immatureTickets); i++ {
		ticket := g.immatureTickets[i]
		liveHeight := ticket.blockHeight + ticketMaturity
		if height >= liveHeight {
			g.immatureTickets = removeTicket(g.immatureTickets, i)
			g.liveTickets = append(g.liveTickets, ticket)

			// This is required because the ticket at the current
			// offset was just removed from the slice that is being
			// iterated, so adjust the offset down one accordingly.
			i--
		}
	}

	// Resort the ticket pool now that all live ticket pool manipulations
	// are done.
	sort.Sort(stakeTicketSorter(g.liveTickets))

	// Add new ticket purchases to the immature ticket pool.
	g.immatureTickets = append(g.immatureTickets, purchases...)
}

// connectBlockTickets updates the live ticket pool and associated data structs
// by for the passed block.  It will panic if the specified block does not
// connect to the current tip block.
func (g *Generator) connectBlockTickets(b *wire.MsgBlock) {
	// Attempt to prevent misuse of this function by ensuring the provided
	// block connects to the current tip.
	if b.Header.PrevBlock != g.tip.BlockHash() {
		panic(fmt.Sprintf("attempt to connect block %s with parent %s "+
			"that is not the current tip %s", b.BlockHash(),
			b.Header.PrevBlock, g.tip.BlockHash()))
	}

	// Get all of the winning tickets for the block.
	numVotes := g.params.TicketsPerBlock
	winners, _, err := winningTickets(g.tip, g.liveTickets, numVotes)
	if err != nil {
		panic(err)
	}

	// Extract the ticket purchases (sstx) from the block.
	height := b.Header.Height
	var purchases []*stakeTicket
	for txIdx, tx := range b.STransactions {
		if isTicketPurchaseTx(tx) {
			ticket := &stakeTicket{tx, height, uint32(txIdx)}
			purchases = append(purchases, ticket)
		}
	}

	// Update the live ticket pool and associated data structures.
	blockHash := b.BlockHash()
	g.connectLiveTickets(&blockHash, height, winners, purchases)
}

// disconnectBlockTickets updates the live ticket pool and associated data
// structs by unwinding the passed block, which must be the current tip block.
// It will panic if the specified block is not the current tip block.
func (g *Generator) disconnectBlockTickets(b *wire.MsgBlock) {
	// Attempt to prevent misuse of this function by ensuring the provided
	// block is the current tip.
	if b != g.tip {
		panic(fmt.Sprintf("attempt to disconnect block %s that is not "+
			"the current tip %s", b.BlockHash(), g.tip.BlockHash()))
	}

	// Remove tickets created in the block from the immature ticket pool.
	blockHeight := b.Header.Height
	for i := 0; i < len(g.immatureTickets); i++ {
		ticket := g.immatureTickets[i]
		if ticket.blockHeight == blockHeight {
			g.immatureTickets = removeTicket(g.immatureTickets, i)

			// This is required because the ticket at the current
			// offset was just removed from the slice that is being
			// iterated, so adjust the offset down one accordingly.
			i--
		}
	}

	// Move tickets that are no longer mature from the live ticket pool to
	// the immature ticket pool.
	prevBlockHeight := blockHeight - 1
	ticketMaturity := uint32(g.params.TicketMaturity)
	for i := 0; i < len(g.liveTickets); i++ {
		ticket := g.liveTickets[i]
		liveHeight := ticket.blockHeight + ticketMaturity
		if prevBlockHeight < liveHeight {
			g.liveTickets = removeTicket(g.liveTickets, i)
			g.immatureTickets = append(g.immatureTickets, ticket)

			// This is required because the ticket at the current
			// offset was just removed from the slice that is being
			// iterated, so adjust the offset down one accordingly.
			i--
		}
	}

	// Move tickets that are no longer expired from the expired ticket pool
	// to the live ticket pool.
	ticketExpiry := g.params.TicketExpiry
	for i := 0; i < len(g.expiredTickets); i++ {
		ticket := g.expiredTickets[i]
		liveHeight := ticket.blockHeight + ticketMaturity
		expireHeight := liveHeight + ticketExpiry
		if prevBlockHeight < expireHeight {
			g.expiredTickets = removeTicket(g.expiredTickets, i)
			g.liveTickets = append(g.liveTickets, ticket)

			// This is required because the ticket at the current
			// offset was just removed from the slice that is being
			// iterated, so adjust the offset down one accordingly.
			i--
		}
	}

	// Add the winning tickets consumed by the block back to the live ticket
	// pool.
	blockHash := b.BlockHash()
	g.liveTickets = append(g.liveTickets, g.wonTickets[blockHash]...)
	delete(g.wonTickets, blockHash)

	// Resort the ticket pool now that all live ticket pool manipulations
	// are done.
	sort.Sort(stakeTicketSorter(g.liveTickets))
}

// originalParent returns the original block the passed block was built from.
// This is necessary because callers might change the previous block hash in a
// munger which would cause the like ticket pool to be reconstructed improperly.
func (g *Generator) originalParent(b *wire.MsgBlock) *wire.MsgBlock {
	parentHash, ok := g.originalParents[b.BlockHash()]
	if !ok {
		parentHash = b.Header.PrevBlock
	}
	return g.BlockByHash(&parentHash)
}

// SetTip changes the tip of the instance to the block with the provided name.
// This is useful since the tip is used for things such as generating subsequent
// blocks.
func (g *Generator) SetTip(blockName string) {
	// Nothing to do if already the tip.
	if blockName == g.tipName {
		return
	}

	newTip := g.blocksByName[blockName]
	if newTip == nil {
		panic(fmt.Sprintf("tip block name %s does not exist", blockName))
	}

	// Create a list of blocks to disconnect and blocks to connect in order
	// to switch to the new tip.
	var connect, disconnect []*wire.MsgBlock
	oldBranch, newBranch := g.tip, newTip
	for oldBranch != newBranch {
		// As long as the two branches are not at the same height, add
		// the tip of the longest one to the appropriate connect or
		// disconnect list and move its tip back to its previous block.
		if oldBranch.Header.Height > newBranch.Header.Height {
			disconnect = append(disconnect, oldBranch)
			oldBranch = g.originalParent(oldBranch)
			continue
		} else if newBranch.Header.Height > oldBranch.Header.Height {
			connect = append(connect, newBranch)
			newBranch = g.originalParent(newBranch)
			continue
		}

		// At this point the two branches have the same height, so add
		// each tip to the appropriate connect or disconnect list and
		// the tips to their previous block.
		disconnect = append(disconnect, oldBranch)
		oldBranch = g.originalParent(oldBranch)
		connect = append(connect, newBranch)
		newBranch = g.originalParent(newBranch)
	}

	// Update the live ticket pool and associated data structs by
	// disconnecting all blocks back to the fork point.
	for _, block := range disconnect {
		g.disconnectBlockTickets(block)
		g.tip = g.originalParent(block)
	}

	// Update the live ticket pool and associated data structs by connecting
	// all blocks after the fork point up to the new tip.  The list of
	// blocks to connect is iterated in reverse order, because it was
	// constructed in reverse, and the blocks need to be connected in the
	// order in which they build the chain.
	for i := len(connect) - 1; i >= 0; i-- {
		block := connect[i]
		g.connectBlockTickets(block)
		g.tip = block
	}

	// Ensure the tip is the expected new tip and set the associated name.
	if g.tip != newTip {
		panic(fmt.Sprintf("tip %s is not expected new tip %s",
			g.tip.BlockHash(), newTip.BlockHash()))
	}
	g.tipName = blockName
}

// updateVoteCommitments updates all of the votes in the passed block to commit
// to the previous block hash and previous height based on the values specified
// in the header.
func updateVoteCommitments(block *wire.MsgBlock) {
	for _, stx := range block.STransactions {
		if !isVoteTx(stx) {
			continue
		}

		stx.TxOut[0].PkScript = voteCommitmentScript(block.Header.PrevBlock,
			block.Header.Height-1)
	}
}

// NextBlock builds a new block that extends the current tip associated with the
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
// - All votes will have their commitments updated if the previous hash or
//   height was manually changed after stake validation height has been reached
// - The merkle root will be recalculated unless it was manually changed
// - The stake root will be recalculated unless it was manually changed
// - The size of the block will be recalculated unless it was manually changed
// - The block will be solved unless the nonce was changed
func (g *Generator) NextBlock(blockName string, spend *SpendableOut, ticketSpends []SpendableOut, mungers ...func(*wire.MsgBlock)) *wire.MsgBlock {
	// Calculate the next required stake difficulty (aka ticket price).
	ticketPrice := dcrutil.Amount(g.calcNextRequiredStakeDifficulty())

	// Generate the appropriate votes and ticket purchases based on the
	// current tip block and provided ticket spendable outputs.
	var ticketWinners []*stakeTicket
	var stakeTxns []*wire.MsgTx
	var finalState [6]byte
	nextHeight := g.tip.Header.Height + 1
	if nextHeight > uint32(g.params.CoinbaseMaturity) {
		// Generate votes once the stake validation height has been
		// reached.
		if int64(nextHeight) >= g.params.StakeValidationHeight {
			// Generate and add the vote transactions for the
			// winning tickets to the stake tree.
			numVotes := g.params.TicketsPerBlock
			winners, stateHash, err := winningTickets(g.tip,
				g.liveTickets, numVotes)
			if err != nil {
				panic(err)
			}
			ticketWinners = winners
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

	// Create stake tickets for the ticket purchases (sstx), count the
	// votes (ssgen) and ticket revocations (ssrtx),  and calculate the
	// total PoW fees generated by the stake transactions.
	var numVotes uint16
	var numTicketRevocations uint8
	var ticketPurchases []*stakeTicket
	var stakeTreeFees dcrutil.Amount
	for txIdx, tx := range stakeTxns {
		switch {
		case isVoteTx(tx):
			numVotes++
		case isTicketPurchaseTx(tx):
			ticket := &stakeTicket{tx, nextHeight, uint32(txIdx)}
			ticketPurchases = append(ticketPurchases, ticket)
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
		coinbaseTx := g.CreateCoinbaseTx(nextHeight, numVotes)
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
			spendTx := g.CreateSpendTx(spend, fee)
			regularTxns = append(regularTxns, spendTx)
		}
	}

	// Use a timestamp that is 7/8 of target timespan after the previous
	// block unless this is the first block in which case the current time
	// is used or the proof-of-work difficulty parameters have been adjusted
	// such that it's greater than the max 2 hours worth of blocks that can
	// be tested in which case one second is used.  This helps maintain the
	// retarget difficulty low as needed.  Also, ensure the timestamp is
	// limited to one second precision.
	var ts time.Time
	if nextHeight == 1 {
		ts = time.Now()
	} else {
		if g.params.WorkDiffWindowSize > 7200 {
			ts = g.tip.Header.Timestamp.Add(time.Second)
		} else {
			addDuration := g.params.TargetTimespan * 7 / 8
			ts = g.tip.Header.Timestamp.Add(addDuration)
		}
	}
	ts = time.Unix(ts.Unix(), 0)

	// Create the unsolved block.
	prevHash := g.tip.BlockHash()
	block := wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:      1,
			PrevBlock:    prevHash,
			MerkleRoot:   calcMerkleRoot(regularTxns),
			StakeRoot:    calcMerkleRoot(stakeTxns),
			VoteBits:     1,
			FinalState:   finalState,
			Voters:       numVotes,
			FreshStake:   uint8(len(ticketPurchases)),
			Revocations:  numTicketRevocations,
			PoolSize:     uint32(len(g.liveTickets)),
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

	// Perform any block munging just before solving.  Once stake validation
	// height has been reached, update the vote commitments accordingly if the
	// header height or previous hash was manually changed by a munge function.
	// Also, only recalculate the merkle roots and block size if they weren't
	// manually changed by a munge function.
	curMerkleRoot := block.Header.MerkleRoot
	curStakeRoot := block.Header.StakeRoot
	curSize := block.Header.Size
	curNonce := block.Header.Nonce
	for _, f := range mungers {
		f(&block)
	}
	if block.Header.Height != nextHeight || block.Header.PrevBlock != prevHash {
		if int64(nextHeight) >= g.params.StakeValidationHeight {
			updateVoteCommitments(&block)
		}
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
	if block.Header.PrevBlock != prevHash {
		// Save the orignal block this one was built from if it was
		// manually changed in a munger so the code which deals with
		// updating the live tickets when changing the tip has access to
		// it.
		g.originalParents[blockHash] = prevHash
	}
	g.connectLiveTickets(&blockHash, nextHeight, ticketWinners,
		ticketPurchases)
	g.blocks[blockHash] = &block
	g.blocksByName[blockName] = &block
	g.tip = &block
	g.tipName = blockName
	return &block
}

// CreatePremineBlock generates the first block of the chain with the required
// premine payouts.  The additional amount parameter can be used to create a
// block that is otherwise a completely valid premine block except it adds the
// extra amount to each payout and thus create a block that violates consensus.
func (g *Generator) CreatePremineBlock(blockName string, additionalAmount dcrutil.Amount) *wire.MsgBlock {
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
		payoutAddr, err := dcrutil.DecodeAddress(payout.Address)
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
	return g.NextBlock(blockName, nil, nil, func(b *wire.MsgBlock) {
		b.Transactions = []*wire.MsgTx{coinbaseTx}
	})
}

// UpdateBlockState manually updates the generator state to remove all internal
// map references to a block via its old hash and insert new ones for the new
// block hash.  This is useful if the test code has to manually change a block
// after 'NextBlock' has returned.
func (g *Generator) UpdateBlockState(oldBlockName string, oldBlockHash chainhash.Hash, newBlockName string, newBlock *wire.MsgBlock) {
	// Remove existing entries.
	wonTickets := g.wonTickets[oldBlockHash]
	delete(g.blocks, oldBlockHash)
	delete(g.blocksByName, oldBlockName)
	delete(g.wonTickets, oldBlockHash)

	// Add new entries.
	newBlockHash := newBlock.BlockHash()
	g.blocks[newBlockHash] = newBlock
	g.blocksByName[newBlockName] = newBlock
	g.wonTickets[newBlockHash] = wonTickets
}

// OldestCoinbaseOuts removes the oldest set of coinbase proof-of-work outputs
// that was previously saved to the generator and returns the set as a slice.
func (g *Generator) OldestCoinbaseOuts() []SpendableOut {
	outs := g.spendableOuts[0]
	g.spendableOuts = g.spendableOuts[1:]
	return outs
}

// NumSpendableCoinbaseOuts returns the number of proof-of-work outputs that
// were previously saved to the generated but have not yet been collected.
func (g *Generator) NumSpendableCoinbaseOuts() int {
	return len(g.spendableOuts)
}

// saveCoinbaseOuts adds the proof-of-work outputs of the coinbase tx in the
// passed block to the list of spendable outputs.
func (g *Generator) saveCoinbaseOuts(b *wire.MsgBlock) {
	g.spendableOuts = append(g.spendableOuts, []SpendableOut{
		MakeSpendableOut(b, 0, 2),
		MakeSpendableOut(b, 0, 3),
		MakeSpendableOut(b, 0, 4),
		MakeSpendableOut(b, 0, 5),
		MakeSpendableOut(b, 0, 6),
		MakeSpendableOut(b, 0, 7),
	})
	g.prevCollectedHash = b.BlockHash()
}

// SaveTipCoinbaseOuts adds the proof-of-work outputs of the coinbase tx in the
// current tip block to the list of spendable outputs.
func (g *Generator) SaveTipCoinbaseOuts() {
	g.saveCoinbaseOuts(g.tip)
}

// SaveSpendableCoinbaseOuts adds all proof-of-work coinbase outputs starting
// from the block after the last block that had its coinbase outputs collected
// and ending at the current tip.  This is useful to batch the collection of the
// outputs once the tests reach a stable point so they don't have to manually
// add them for the right tests which will ultimately end up being the best
// chain.
func (g *Generator) SaveSpendableCoinbaseOuts() {
	// Loop through the ancestors of the current tip until the
	// reaching the block that has already had the coinbase outputs
	// collected.
	var collectBlocks []*wire.MsgBlock
	for b := g.tip; b != nil; b = g.blocks[b.Header.PrevBlock] {
		if b.BlockHash() == g.prevCollectedHash {
			break
		}
		collectBlocks = append(collectBlocks, b)
	}
	for i := range collectBlocks {
		g.saveCoinbaseOuts(collectBlocks[len(collectBlocks)-1-i])
	}
}

// AssertTipHeight panics if the current tip block associated with the generator
// does not have the specified height.
func (g *Generator) AssertTipHeight(expected uint32) {
	height := g.tip.Header.Height
	if height != expected {
		panic(fmt.Sprintf("height for block %q is %d instead of "+
			"expected %d", g.tipName, height, expected))
	}
}

// AssertScriptSigOpsCount panics if the provided script does not have the
// specified number of signature operations.
func (g *Generator) AssertScriptSigOpsCount(script []byte, expected int) {
	numSigOps := txscript.GetSigOpCount(script)
	if numSigOps != expected {
		_, file, line, _ := runtime.Caller(1)
		panic(fmt.Sprintf("assertion failed at %s:%d: generated number "+
			"of sigops for script is %d instead of expected %d",
			file, line, numSigOps, expected))
	}
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

// AssertTipBlockSigOpsCount panics if the current tip block associated with the
// generator does not have the specified number of signature operations.
func (g *Generator) AssertTipBlockSigOpsCount(expected int) {
	numSigOps := countBlockSigOps(g.tip)
	if numSigOps != expected {
		panic(fmt.Sprintf("generated number of sigops for block %q "+
			"(height %d) is %d instead of expected %d", g.tipName,
			g.tip.Header.Height, numSigOps, expected))
	}
}

// AssertTipBlockSize panics if the if the current tip block associated with the
// generator does not have the specified size when serialized.
func (g *Generator) AssertTipBlockSize(expected int) {
	serializeSize := g.tip.SerializeSize()
	if serializeSize != expected {
		panic(fmt.Sprintf("block size of block %q (height %d) is %d "+
			"instead of expected %d", g.tipName,
			g.tip.Header.Height, serializeSize, expected))
	}
}

// AssertTipBlockNumTxns panics if the number of transactions in the current tip
// block associated with the generator does not match the specified value.
func (g *Generator) AssertTipBlockNumTxns(expected int) {
	numTxns := len(g.tip.Transactions)
	if numTxns != expected {
		panic(fmt.Sprintf("number of txns in block %q (height %d) is "+
			"%d instead of expected %d", g.tipName,
			g.tip.Header.Height, numTxns, expected))
	}
}

// AssertTipBlockHash panics if the current tip block associated with the
// generator does not match the specified hash.
func (g *Generator) AssertTipBlockHash(expected chainhash.Hash) {
	hash := g.tip.BlockHash()
	if hash != expected {
		panic(fmt.Sprintf("block hash of block %q (height %d) is %v "+
			"instead of expected %v", g.tipName,
			g.tip.Header.Height, hash, expected))
	}
}

// AssertTipBlockMerkleRoot panics if the merkle root in header of the current
// tip block associated with the generator does not match the specified hash.
func (g *Generator) AssertTipBlockMerkleRoot(expected chainhash.Hash) {
	hash := g.tip.Header.MerkleRoot
	if hash != expected {
		panic(fmt.Sprintf("merkle root of block %q (height %d) is %v "+
			"instead of expected %v", g.tipName,
			g.tip.Header.Height, hash, expected))
	}
}

// AssertTipBlockTxOutOpReturn panics if the current tip block associated with
// the generator does not have an OP_RETURN script for the transaction output at
// the provided tx index and output index.
func (g *Generator) AssertTipBlockTxOutOpReturn(txIndex, txOutIndex uint32) {
	if txIndex >= uint32(len(g.tip.Transactions)) {
		panic(fmt.Sprintf("Transaction index %d in block %q "+
			"(height %d) does not exist", txIndex, g.tipName,
			g.tip.Header.Height))
	}

	tx := g.tip.Transactions[txIndex]
	if txOutIndex >= uint32(len(tx.TxOut)) {
		panic(fmt.Sprintf("transaction index %d output %d in block %q "+
			"(height %d) does not exist", txIndex, txOutIndex,
			g.tipName, g.tip.Header.Height))
	}

	txOut := tx.TxOut[txOutIndex]
	if txOut.PkScript[0] != txscript.OP_RETURN {
		panic(fmt.Sprintf("transaction index %d output %d in block %q "+
			"(height %d) is not an OP_RETURN", txIndex, txOutIndex,
			g.tipName, g.tip.Header.Height))
	}
}

// AssertStakeVersion panics if the current tip block associated with the
// generator does not have the specified stake version in the header.
func (g *Generator) AssertStakeVersion(expected uint32) {
	stakeVersion := g.tip.Header.StakeVersion
	if stakeVersion != expected {
		panic(fmt.Sprintf("stake version for block %q is %d instead of "+
			"expected %d", g.tipName, stakeVersion, expected))
	}
}

// AssertBlockVersion panics if the current tip block associated with the
// generator does not have the specified version.
func (g *Generator) AssertBlockVersion(expected int32) {
	blockVersion := g.tip.Header.Version
	if blockVersion != expected {
		panic(fmt.Sprintf("block version for block %q is %d instead of "+
			"expected %d", g.tipName, blockVersion, expected))
	}
}
