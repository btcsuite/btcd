// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package fullblocktests

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/chaingen"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/dcrec"
	"github.com/decred/dcrd/dcrec/secp256k1"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

const (
	// Intentionally defined here rather than using constants from codebase
	// to ensure consensus changes are detected.
	maxBlockSigOps       = 5000
	minCoinbaseScriptLen = 2
	maxCoinbaseScriptLen = 100
	medianTimeBlocks     = 11
	maxScriptElementSize = 2048

	// numLargeReorgBlocks is the number of blocks to use in the large block
	// reorg test (when enabled).  This is the equivalent of 2 day's worth
	// of blocks.
	numLargeReorgBlocks = 576

	// voteBitNo and voteBitYes represent no and yes votes, respectively, on
	// whether or not to approve the previous block.
	voteBitNo  = 0x0000
	voteBitYes = 0x0001
)

var (
	// opTrueScript is a simple public key script that contains the OP_TRUE
	// opcode.  It is defined here to reduce garbage creation.
	opTrueScript = []byte{txscript.OP_TRUE}

	// invalidP2SHRedeemScript is a script that evaluates to false. This makes
	// it an invalid P2SH redeem script.
	invalidP2SHRedeemScript = []byte{0x01, txscript.OP_FALSE}

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

// FullBlockTestInstance only exists to allow AcceptedBlock to be treated as a
// TestInstance.
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

// FullBlockTestInstance only exists to allow RejectedBlock to be treated as a
// TestInstance.
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

// FullBlockTestInstance only exists to allow ExpectedTip to be treated as a
// TestInstance.
//
// This implements the TestInstance interface.
func (b ExpectedTip) FullBlockTestInstance() {}

// RejectedNonCanonicalBlock defines a test instance that expects a serialized
// block that is not canonical and therefore should be rejected.
type RejectedNonCanonicalBlock struct {
	Name     string
	RawBlock []byte
	Height   int32
}

// FullBlockTestInstance only exists to allow RejectedNonCanonicalBlock to be
// treated as a TestInstance.
//
// This implements the TestInstance interface.
func (b RejectedNonCanonicalBlock) FullBlockTestInstance() {}

// payToScriptHashScript returns a standard pay-to-script-hash for the provided
// redeem script.
func payToScriptHashScript(redeemScript []byte) []byte {
	redeemScriptHash := dcrutil.Hash160(redeemScript)
	script, err := txscript.NewScriptBuilder().
		AddOp(txscript.OP_HASH160).AddData(redeemScriptHash).
		AddOp(txscript.OP_EQUAL).Script()
	if err != nil {
		panic(err)
	}
	return script
}

// pushDataScript returns a script with the provided items individually pushed
// to the stack.
func pushDataScript(items ...[]byte) []byte {
	builder := txscript.NewScriptBuilder()
	for _, item := range items {
		builder.AddData(item)
	}
	script, err := builder.Script()
	if err != nil {
		panic(err)
	}
	return script
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

// additionalPoWTx returns a function that itself takes a block and modifies it
// by adding the the provided transaction to the regular transaction tree.
func additionalPoWTx(tx *wire.MsgTx) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.AddTransaction(tx)
	}
}

// nonCanonicalVarInt return a variable-length encoded integer that is encoded
// with 9 bytes even though it could be encoded with a minimal canonical
// encoding.
func nonCanonicalVarInt(val uint32) []byte {
	var rv [9]byte
	rv[0] = 0xff
	binary.LittleEndian.PutUint64(rv[1:], uint64(val))
	return rv[:]
}

// encodeNonCanonicalBlock serializes the block in a non-canonical way by
// encoding the number of transactions using a variable-length encoded integer
// with 9 bytes even though it should be encoded with a minimal canonical
// encoding.
func encodeNonCanonicalBlock(b *wire.MsgBlock) []byte {
	var buf bytes.Buffer
	b.Header.BtcEncode(&buf, 0)
	buf.Write(nonCanonicalVarInt(uint32(len(b.Transactions))))
	for _, tx := range b.Transactions {
		tx.BtcEncode(&buf, 0)
	}
	wire.WriteVarInt(&buf, 0, uint64(len(b.STransactions)))
	for _, stx := range b.STransactions {
		stx.BtcEncode(&buf, 0)
	}
	return buf.Bytes()
}

// assertTipsNonCanonicalBlockSize panics if the if the current tip block
// associated with the generator does not have the specified non-canonical size
// when serialized.
func assertTipNonCanonicalBlockSize(g *chaingen.Generator, expected int) {
	tip := g.Tip()
	serializeSize := len(encodeNonCanonicalBlock(tip))
	if serializeSize != expected {
		panic(fmt.Sprintf("block size of block %q (height %d) is %d "+
			"instead of expected %d", g.TipName(), tip.Header.Height,
			serializeSize, expected))
	}
}

// cloneBlock returns a deep copy of the provided block.
func cloneBlock(b *wire.MsgBlock) wire.MsgBlock {
	var blockCopy wire.MsgBlock
	blockCopy.Header = b.Header
	for _, tx := range b.Transactions {
		blockCopy.AddTransaction(tx.Copy())
	}
	for _, stx := range b.STransactions {
		blockCopy.AddSTransaction(stx)
	}
	return blockCopy
}

// repeatOpcode returns a byte slice with the provided opcode repeated the
// specified number of times.
func repeatOpcode(opcode uint8, numRepeats int) []byte {
	return bytes.Repeat([]byte{opcode}, numRepeats)
}

// Generate returns a slice of tests that can be used to exercise the consensus
// validation rules.  The tests are intended to be flexible enough to allow both
// unit-style tests directly against the blockchain code as well as
// integration-style tests over the peer-to-peer network.  To achieve that goal,
// each test contains additional information about the expected result, however
// that information can be ignored when doing comparison tests between to
// independent versions over the peer-to-peer network.
func Generate(includeLargeReorg bool) (tests [][]TestInstance, err error) {
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
				err = errors.New("unknown panic")
			}
		}
	}()

	// Create a generator instance initialized with the genesis block as the
	// tip as well as some cached payment scripts to be used throughout the
	// tests.
	g, err := chaingen.MakeGenerator(simNetParams)
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
	// rejectNonCanonicalBlock creates a test instance that encodes the
	// provided block using a non-canonical encoded as described by the
	// encodeNonCanonicalBlock function and expects it to be rejected.
	//
	// orphanOrRejectBlock creates a test instance that expects the provided
	// block to either be accepted as an orphan or rejected by the consensus
	// rules.
	//
	// expectTipBlock creates a test instance that expects the provided
	// block to be the current tip of the block chain.
	acceptBlock := func(blockName string, block *wire.MsgBlock, isMainChain, isOrphan bool) TestInstance {
		return AcceptedBlock{blockName, block, isMainChain, isOrphan}
	}
	rejectBlock := func(blockName string, block *wire.MsgBlock, code blockchain.ErrorCode) TestInstance {
		return RejectedBlock{blockName, block, code}
	}
	rejectNonCanonicalBlock := func(blockName string, block *wire.MsgBlock) TestInstance {
		blockHeight := int32(block.Header.Height)
		encoded := encodeNonCanonicalBlock(block)
		return RejectedNonCanonicalBlock{blockName, encoded, blockHeight}
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
	// rejected creates and appends a single rejectBlock test instance for
	// the current tip.
	//
	// rejectedNonCanonical creates and appends a single
	// rejectNonCanonicalBlock test instance for the current tip.
	//
	// orphaned creates and appends a single acceptBlock test instance for
	// the current tip which expects the block to be accepted as an orphan.
	//
	// orphanedOrRejected creates and appends a single orphanOrRejectBlock
	// test instance for the current tip.
	accepted := func() {
		tests = append(tests, []TestInstance{
			acceptBlock(g.TipName(), g.Tip(), true, false),
		})
	}
	acceptedToSideChainWithExpectedTip := func(tipName string) {
		tests = append(tests, []TestInstance{
			acceptBlock(g.TipName(), g.Tip(), false, false),
			expectTipBlock(tipName, g.BlockByName(tipName)),
		})
	}
	rejected := func(code blockchain.ErrorCode) {
		tests = append(tests, []TestInstance{
			rejectBlock(g.TipName(), g.Tip(), code),
		})
	}
	rejectedNonCanonical := func() {
		tests = append(tests, []TestInstance{
			rejectNonCanonicalBlock(g.TipName(), g.Tip()),
		})
	}
	orphaned := func() {
		tests = append(tests, []TestInstance{
			acceptBlock(g.TipName(), g.Tip(), false, true),
		})
	}
	orphanedOrRejected := func() {
		tests = append(tests, []TestInstance{
			orphanOrRejectBlock(g.TipName(), g.Tip()),
		})
	}

	// Shorter versions of useful params for convenience.
	coinbaseMaturity := g.Params().CoinbaseMaturity
	stakeEnabledHeight := g.Params().StakeEnabledHeight
	stakeValidationHeight := g.Params().StakeValidationHeight
	maxBlockSize := g.Params().MaximumBlockSizes[0]
	ticketsPerBlock := g.Params().TicketsPerBlock

	// ---------------------------------------------------------------------
	// Premine tests.
	// ---------------------------------------------------------------------

	// Attempt to insert an initial premine block that does not conform to
	// the required premine payouts by adding one atom too many to each
	// payout.
	//
	//   genesis
	//          \-> bpbad0
	g.CreatePremineBlock("bpbad0", 1)
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create a premine block with one premine output removed.
	//
	//   genesis -> bpbad1
	g.SetTip("genesis")
	g.CreatePremineBlock("bpbad1", 0, func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut = b.Transactions[0].TxOut[:2]
	})
	rejected(blockchain.ErrBlockOneOutputs)

	// Create a premine block with a bad spend script.
	//
	//   genesis -> bpbad2
	g.SetTip("genesis")
	g.CreatePremineBlock("bpbad2", 0, func(b *wire.MsgBlock) {
		scriptSize := len(b.Transactions[0].TxOut[0].PkScript)
		badScript := repeatOpcode(txscript.OP_0, scriptSize)
		b.Transactions[0].TxOut[0].PkScript = badScript
	})
	rejected(blockchain.ErrBlockOneOutputs)

	// Create a premine block with an incorrect pay to amount.
	//
	//   genesis -> bpbad3
	g.SetTip("genesis")
	g.CreatePremineBlock("bpbad3", 0, func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut[0].Value--
	})
	rejected(blockchain.ErrBlockOneOutputs)

	// Add the required premine block.
	//
	//   genesis -> bp
	g.SetTip("genesis")
	g.CreatePremineBlock("bp", 0)
	g.AssertTipHeight(1)
	accepted()

	// Create block that tries to spend premine output before
	// maturity in the regular tree
	//
	// genesis -> bp
	//              \-> bpi0
	g.NextBlock("bpi0", nil, nil, func(b *wire.MsgBlock) {
		spendOut := chaingen.MakeSpendableOut(g.Tip(), 0, 0)
		tx := g.CreateSpendTx(&spendOut, lowFee)
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrImmatureSpend)

	// Create block that tries to spend premine output before
	// maturity in the stake tree by creating a ticket purchase
	//
	// genesis -> bp
	//              \-> bpi1
	g.SetTip("bp")
	g.NextBlock("bpi1", nil, nil, func(b *wire.MsgBlock) {
		spendOut := chaingen.MakeSpendableOut(g.Tip(), 0, 0)
		ticketPrice := dcrutil.Amount(g.CalcNextRequiredStakeDifficulty())
		tx := g.CreateTicketPurchaseTx(&spendOut, ticketPrice, lowFee)
		b.AddSTransaction(tx)
		b.Header.FreshStake++
	})
	rejected(blockchain.ErrImmatureSpend)

	// ---------------------------------------------------------------------
	// Generate enough blocks to have mature coinbase outputs to work with.
	//
	//   genesis -> bp -> bm0 -> bm1 -> ... -> bm#
	// ---------------------------------------------------------------------

	g.SetTip("bp")
	var testInstances []TestInstance
	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bm%d", i)
		g.NextBlock(blockName, nil, nil)
		g.SaveTipCoinbaseOuts()
		testInstances = append(testInstances, acceptBlock(g.TipName(),
			g.Tip(), true, false))
	}
	tests = append(tests, testInstances)
	g.AssertTipHeight(uint32(coinbaseMaturity) + 1)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake enabled height as well as the
	// first set of live tickets while creating ticket purchases that spend
	// from the coinbases matured above. This will also populate the
	// pool of immature tickets.
	//
	//   ... -> bm# ... -> bse0 -> bse1 -> ... -> bse#
	// ---------------------------------------------------------------------

	testInstances = nil
	var ticketsPurchased int
	beforeLiveTickets := stakeEnabledHeight + 2
	for i := int64(0); int64(g.Tip().Header.Height) < beforeLiveTickets; i++ {
		outs := g.OldestCoinbaseOuts()
		ticketOuts := outs[1:]
		ticketsPurchased += len(ticketOuts)
		blockName := fmt.Sprintf("bse%d", i)
		g.NextBlock(blockName, nil, ticketOuts)
		g.SaveTipCoinbaseOuts()
		testInstances = append(testInstances, acceptBlock(g.TipName(),
			g.Tip(), true, false))
	}
	tests = append(tests, testInstances)
	g.AssertTipHeight(uint32(beforeLiveTickets))
	bseTipName := g.TipName()

	// Create block that tries to vote before stake validation height.
	//   bse#
	//       \ -> bis1
	g.NextBlock("bis1", nil, nil, func(b *wire.MsgBlock) {
		// block bse0 being the first block to purchase tickets will be the
		// first block with mature tickets.
		ticketBlock := g.BlockByName("bse0")
		voteBlock := g.BlockByName(bseTipName)
		ticketIdx := uint32(1)
		ticketTx := ticketBlock.STransactions[ticketIdx]
		voteTx := g.CreateVoteTx(voteBlock, ticketTx,
			ticketBlock.Header.Height, ticketIdx)
		b.AddSTransaction(voteTx)
		b.Header.Voters++
	})
	g.AssertPoolSize(5)
	rejected(blockchain.ErrInvalidEarlyStakeTx)
	g.SetTip(bseTipName)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake validation height while
	// continuing to purchase tickets using the coinbases matured above and
	// allowing the immature tickets to mature and thus become live.
	//
	// While doing so, also generate blocks that have the required early
	// vote unset and other that have a non-zero early final state to ensure
	// they are properly rejected.
	//
	//   ... -> bse# -> bsv0 -> bsv1 -> ... -> bsv#
	//             \       \       \        \-> bevbad#
	//              |       |       \-> bevbad2
	//              |       \-> bevbad1     \-> befsbad#
	//              \-> bevbad0     \-> befsbad2
	//              |       \-> befsbad1
	//              \-> befsbad0
	// ---------------------------------------------------------------------

	testInstances = nil
	targetPoolSize := g.Params().TicketPoolSize * g.Params().TicketsPerBlock
	for i := int64(0); int64(g.Tip().Header.Height) < stakeValidationHeight; i++ {
		// Until stake validation height is reached, test that any
		// blocks without the early vote bits set are rejected.
		if int64(g.Tip().Header.Height) < stakeValidationHeight-1 {
			prevTip := g.TipName()
			blockName := fmt.Sprintf("bevbad%d", i)
			g.NextBlock(blockName, nil, nil, func(b *wire.MsgBlock) {
				b.Header.VoteBits = 0
			})
			testInstances = append(testInstances, rejectBlock(
				g.TipName(), g.Tip(),
				blockchain.ErrInvalidEarlyVoteBits))
			g.SetTip(prevTip)
		}

		// Until stake validation height is reached, test that any
		// blocks without a zero final state are rejected.
		if int64(g.Tip().Header.Height) < stakeValidationHeight-1 {
			prevTip := g.TipName()
			blockName := fmt.Sprintf("befsbad%d", i)
			g.NextBlock(blockName, nil, nil, func(b *wire.MsgBlock) {
				b.Header.FinalState = [6]byte{0x01}
			})
			testInstances = append(testInstances, rejectBlock(
				g.TipName(), g.Tip(),
				blockchain.ErrInvalidEarlyFinalState))
			g.SetTip(prevTip)
		}

		// Only purchase tickets until the target ticket pool size is
		// reached.
		outs := g.OldestCoinbaseOuts()
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
		g.NextBlock(blockName, nil, ticketOuts)
		g.SaveTipCoinbaseOuts()
		testInstances = append(testInstances, acceptBlock(g.TipName(),
			g.Tip(), true, false))
	}
	tests = append(tests, testInstances)
	g.AssertTipHeight(uint32(stakeValidationHeight))

	// ---------------------------------------------------------------------
	// Generate enough blocks to have a known distance to the first mature
	// coinbase outputs for all tests that follow.  These blocks continue
	// to purchase tickets to avoid running out of votes.
	//
	//   ... -> bsv# -> bbm0 -> bbm1 -> ... -> bbm#
	// ---------------------------------------------------------------------

	testInstances = nil
	for i := uint16(0); i < coinbaseMaturity; i++ {
		outs := g.OldestCoinbaseOuts()
		blockName := fmt.Sprintf("bbm%d", i)
		g.NextBlock(blockName, nil, outs[1:])
		g.SaveTipCoinbaseOuts()
		testInstances = append(testInstances, acceptBlock(g.TipName(),
			g.Tip(), true, false))
	}
	tests = append(tests, testInstances)

	// Collect spendable outputs into two different slices.  The outs slice
	// is intended to be used for regular transactions that spend from the
	// output, while the ticketOuts slice is intended to be used for stake
	// ticket purchases.
	var outs []*chaingen.SpendableOut
	var ticketOuts [][]chaingen.SpendableOut
	for i := uint16(0); i < coinbaseMaturity; i++ {
		coinbaseOuts := g.OldestCoinbaseOuts()
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
	//   ... -> bf1(0) -> bf2(1)
	g.NextBlock("bf1", outs[0], ticketOuts[0])
	accepted()

	g.NextBlock("bf2", outs[1], ticketOuts[1])
	accepted()

	// Ensure duplicate blocks are rejected.
	//
	//   ... -> bf1(0) -> bf2(1)
	//                \-> bf2(1)
	rejected(blockchain.ErrDuplicateBlock)

	// Create a fork from bf1.  There should not be a reorg since bf2 was seen
	// first.
	//
	//   ... -> bf1(0) -> bf2(1)
	//                \-> bf3(1)
	g.SetTip("bf1")
	g.NextBlock("bf3", outs[1], ticketOuts[1])
	bf3Tx1Out := chaingen.MakeSpendableOut(g.Tip(), 1, 0)
	acceptedToSideChainWithExpectedTip("bf2")

	// Extend bf3 fork to make the alternative chain longer and force reorg.
	//
	//   ... -> bf1(0) -> bf2(1)
	//                \-> bf3(1) -> bf4(2)
	g.NextBlock("bf4", outs[2], ticketOuts[2])
	accepted()

	// Extend bf2 fork twice to make first chain longer and force reorg.
	//
	//   ... -> bf1(0) -> bf2(1) -> bf5(2) -> bf6(3)
	//                \-> bf3(1) -> bf4(2)
	g.SetTip("bf2")
	g.NextBlock("bf5", outs[2], ticketOuts[2])
	acceptedToSideChainWithExpectedTip("bf4")

	g.NextBlock("bf6", outs[3], ticketOuts[3])
	accepted()

	// ---------------------------------------------------------------------
	// Double spend tests.
	// ---------------------------------------------------------------------

	// Create a fork that double spends.
	//
	//   ... -> bf1(0) -> bf2(1) -> bf5(2) -> bf6(3)
	//                                 \-> bf7(2) -> bf8(4)
	//                \-> bf3(1) -> bf4(2)
	g.SetTip("bf5")
	g.NextBlock("bf7", outs[2], ticketOuts[3])
	acceptedToSideChainWithExpectedTip("bf6")

	g.NextBlock("bf8", outs[4], ticketOuts[4])
	rejected(blockchain.ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Too much proof-of-work coinbase tests.
	// ---------------------------------------------------------------------

	// Create a block that generates too much proof-of-work coinbase.
	//
	//   ... -> bf1(0) -> bf2(1) -> bf5(2) -> bf6(3)
	//                                              \-> bpw1(4)
	//                \-> bf3(1) -> bf4(2)
	g.SetTip("bf6")
	g.NextBlock("bpw1", outs[4], ticketOuts[4], additionalCoinbasePoW(1))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create a fork that ends with block that generates too much
	// proof-of-work coinbase.
	//
	//   ... -> bf1(0) -> bf2(1) -> bf5(2) -> bf6(3)
	//                                    \-> bpw2(3) -> bpw3(4)
	//                \-> bf3(1) -> bf4(2)
	g.SetTip("bf5")
	g.NextBlock("bpw2", outs[3], ticketOuts[3])
	acceptedToSideChainWithExpectedTip("bf6")

	g.NextBlock("bpw3", outs[4], ticketOuts[4], additionalCoinbasePoW(1))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create a fork that ends with block that generates too much
	// proof-of-work coinbase as before, but with a valid fork first.
	//
	//   ... -> bf1(0) -> bf2(1) -> bf5(2) -> bf6(3)
	//               |                    \-> bpw4(3) -> bpw5(4) -> bpw6(5)
	//               |                       (bpw4 added last)
	//                \-> bf3(1) -> bf4(2)
	g.SetTip("bf5")
	bpw4 := g.NextBlock("bpw4", outs[3], ticketOuts[3])
	bpw5 := g.NextBlock("bpw5", outs[4], ticketOuts[4])
	bpw6 := g.NextBlock("bpw6", outs[5], ticketOuts[5], additionalCoinbasePoW(1))
	tests = append(tests, []TestInstance{
		acceptBlock("bpw5", bpw5, false, true),
		acceptBlock("bpw6", bpw6, false, true),
		rejectBlock("bpw4", bpw4, blockchain.ErrBadCoinbaseValue),
		expectTipBlock("bpw5", bpw5),
	})

	// ---------------------------------------------------------------------
	// Bad dev org output tests.
	// ---------------------------------------------------------------------

	// Create a block that does not generate a payment to the dev-org P2SH
	// address.  Test this by trying to pay to a secp256k1 P2PKH address
	// using the same HASH160.
	//
	//   ... -> bf5(2) -> bpw4(3) -> bpw5(4)
	//   \                                  \-> bbadtaxscript(5)
	//    \-> bf3(1) -> bf4(2)
	g.SetTip("bpw5")
	g.NextBlock("bbadtaxscript", outs[5], ticketOuts[5],
		func(b *wire.MsgBlock) {
			taxOutput := b.Transactions[0].TxOut[0]
			_, addrs, _, _ := txscript.ExtractPkScriptAddrs(
				g.Params().OrganizationPkScriptVersion,
				taxOutput.PkScript, g.Params())
			p2shTaxAddr := addrs[0].(*dcrutil.AddressScriptHash)
			p2pkhTaxAddr, err := dcrutil.NewAddressPubKeyHash(
				p2shTaxAddr.Hash160()[:], g.Params(),
				dcrec.STEcdsaSecp256k1)
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
	//   ... -> bf5(2) -> bpw4(3) -> bpw5(4)
	//   \                                  \-> bbadtaxscriptversion(5)
	//    \-> bf3(1) -> bf4(2)
	g.SetTip("bpw5")
	g.NextBlock("bbadtaxscriptversion", outs[5], ticketOuts[5],
		func(b *wire.MsgBlock) {
			b.Transactions[0].TxOut[0].Version = 1
		})
	rejected(blockchain.ErrNoTax)

	// ---------------------------------------------------------------------
	// Too much dev-org coinbase tests.
	// ---------------------------------------------------------------------

	// Create a block that generates too much dev-org coinbase.
	//
	//   ... -> bf5(2) -> bpw4(3) -> bpw5(4)
	//   \                                  \-> bdc1(5)
	//    \-> bf3(1) -> bf4(2)
	g.SetTip("bpw5")
	g.NextBlock("bdc1", outs[5], ticketOuts[5], additionalCoinbaseDev(1))
	rejected(blockchain.ErrNoTax)

	// Create a fork that ends with block that generates too much dev-org
	// coinbase.
	//
	//   ... -> bf5(2) -> bpw4(3) -> bpw5(4)
	//   \                       \-> bdc2(4) -> bdc3(5)
	//    \-> bf3(1) -> bf4(2)
	g.SetTip("bpw4")
	g.NextBlock("bdc2", outs[4], ticketOuts[4], additionalCoinbaseDev(1))
	acceptedToSideChainWithExpectedTip("bpw5")

	g.NextBlock("bdc3", outs[5], ticketOuts[5], additionalCoinbaseDev(1))
	rejected(blockchain.ErrNoTax)

	// Create a fork that ends with block that generates too much dev-org
	// coinbase as before, but with a valid fork first.
	//
	//   ... -> bf5(2) -> bpw4(3) -> bpw5(4)
	//   \                       \-> bdc4(4) -> bdc5(5) -> bdc6(6)
	//   |                           (bdc4 added last)
	//    \-> bf3(1) -> bf4(2)
	//
	g.SetTip("bpw4")
	bdc4 := g.NextBlock("bdc4", outs[4], ticketOuts[4])
	bdc5 := g.NextBlock("bdc5", outs[5], ticketOuts[5])
	bdc6 := g.NextBlock("bdc6", outs[6], ticketOuts[6], additionalCoinbaseDev(1))
	tests = append(tests, []TestInstance{
		acceptBlock("bdc5", bdc5, false, true),
		acceptBlock("bdc6", bdc6, false, true),
		rejectBlock("bdc4", bdc4, blockchain.ErrNoTax),
		expectTipBlock("bdc5", bdc5),
	})

	// ---------------------------------------------------------------------
	// Checksig signature operation count tests.
	// ---------------------------------------------------------------------

	// Add a block with max allowed signature operations.
	//
	//   ... -> bf5(2) -> bpw4(3) -> bdc4(4) -> bdc5(5) -> bcs1(6)
	//   \-> bf3(1) -> bf4(2)
	g.SetTip("bdc5")
	manySigOps := repeatOpcode(txscript.OP_CHECKSIG, maxBlockSigOps)
	g.NextBlock("bcs1", outs[6], ticketOuts[6], replaceSpendScript(manySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Attempt to add block with more than max allowed signature operations.
	//
	//   ... -> bf5(2) -> bpw4(3) -> bdc4(4) -> bdc5(5) -> bcs1(6)
	//   \                                                        \-> bcs2(7)
	//    \-> bf3(1) -> bf4(2)
	tooManySigOps := repeatOpcode(txscript.OP_CHECKSIG, maxBlockSigOps+1)
	g.NextBlock("bcs2", outs[7], ticketOuts[7],
		replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// ---------------------------------------------------------------------
	// Cross-fork spend tests.
	// ---------------------------------------------------------------------

	// Create block that spends a tx created on a different fork.
	//
	//   ... -> bf5(2) -> bpw4(3) -> bdc4(4) -> bdc5(5) -> bcs1(6)
	//   \                                                        \-> bcf1(b3.tx[1])
	//    \-> bf3(1) -> bf4(2)
	g.SetTip("bcs1")
	g.NextBlock("bcf1", &bf3Tx1Out, nil)
	rejected(blockchain.ErrMissingTxOut)

	// Create block that forks and spends a tx created on a third fork.
	//
	//   ... -> bf5(2) -> bpw4(3) -> bdc4(4) -> bdc5(5) -> bcs1(6)
	//   |                                             \-> bcf1(bf3.tx[1]) -> bcf2(6)
	//    \-> bf3(1) -> bf4(2)
	g.SetTip("bdc5")
	g.NextBlock("bcf2", &bf3Tx1Out, nil)
	acceptedToSideChainWithExpectedTip("bcs1")

	g.NextBlock("bcf3", outs[6], ticketOuts[6])
	rejected(blockchain.ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Immature coinbase tests.
	// ---------------------------------------------------------------------

	// Create block that spends immature coinbase.
	//
	//   ... -> bdc5(5) -> bcs1(6)
	//                            \-> bic1(8)
	g.SetTip("bcs1")
	g.NextBlock("bic1", outs[8], ticketOuts[7])
	rejected(blockchain.ErrImmatureSpend)

	// Create block that spends immature coinbase on a fork.
	//
	//   ... -> bdc5(5) -> bcs1(6)
	//                 \-> bic2(6) -> bic3(8)
	g.SetTip("bdc5")
	g.NextBlock("bic2", outs[6], ticketOuts[6])
	acceptedToSideChainWithExpectedTip("bcs1")

	g.NextBlock("bic3", outs[8], ticketOuts[7])
	rejected(blockchain.ErrImmatureSpend)

	// ---------------------------------------------------------------------
	// Max block size tests.
	// ---------------------------------------------------------------------

	// Create block that is the max allowed size.
	//
	//   ... -> bcs1(6) -> bms1(7)
	g.SetTip("bcs1")
	g.NextBlock("bms1", outs[7], ticketOuts[7], func(b *wire.MsgBlock) {
		curScriptLen := len(b.Transactions[1].TxOut[0].PkScript)
		bytesToMaxSize := maxBlockSize - b.SerializeSize() +
			(curScriptLen - 4)
		sizePadScript := repeatOpcode(0x00, bytesToMaxSize)
		replaceSpendScript(sizePadScript)(b)
	})
	g.AssertTipBlockSize(maxBlockSize)
	accepted()

	// Create block that is the one byte larger than max allowed size.  This
	// is done on a fork and should be rejected regardless.
	//
	//   ... -> bcs1(6) -> bms1(7)
	//                 \-> bms2(7) -> bms3(8)
	g.SetTip("bcs1")
	g.NextBlock("bms2", outs[7], ticketOuts[7], func(b *wire.MsgBlock) {
		curScriptLen := len(b.Transactions[1].TxOut[0].PkScript)
		bytesToMaxSize := maxBlockSize - b.SerializeSize() +
			(curScriptLen - 4)
		sizePadScript := repeatOpcode(0x00, bytesToMaxSize+1)
		replaceSpendScript(sizePadScript)(b)
	})
	g.AssertTipBlockSize(maxBlockSize + 1)
	rejected(blockchain.ErrBlockTooBig)

	// Parent was rejected, so this block must either be an orphan or
	// outright rejected due to an invalid parent.
	g.NextBlock("bms3", outs[8], ticketOuts[8])
	orphanedOrRejected()

	// ---------------------------------------------------------------------
	// Orphan tests.
	// ---------------------------------------------------------------------

	// Create valid orphan block with zero prev hash.
	//
	//   No previous block
	//                    \-> borphan0(7)
	g.SetTip("bcs1")
	g.NextBlock("borphan0", outs[7], ticketOuts[7], func(b *wire.MsgBlock) {
		b.Header.PrevBlock = chainhash.Hash{}
	})
	orphaned()

	// Create valid orphan block.
	//
	//   ... -> bcs1(6) -> bms1(7)
	//                 \-> borphanbase(7) -> borphan1(8)
	g.SetTip("bcs1")
	g.NextBlock("borphanbase", outs[7], ticketOuts[7])
	g.NextBlock("borphan1", outs[8], ticketOuts[8])
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
	//   ... -> bcs1(6) -> bms1(7)
	//                 \-> bsl1(7) -> bsl2(8)
	g.SetTip("bcs1")
	tooSmallCbScript := repeatOpcode(0x00, minCoinbaseScriptLen-1)
	g.NextBlock("bsl1", outs[7], ticketOuts[7],
		replaceCoinbaseSigScript(tooSmallCbScript))
	rejected(blockchain.ErrBadCoinbaseScriptLen)

	// Parent was rejected, so this block must either be an orphan or
	// outright rejected due to an invalid parent.
	g.NextBlock("bsl2", outs[8], ticketOuts[8])
	orphanedOrRejected()

	// Create block that has a coinbase script that is larger than the
	// allowed length.  This is done on a fork and should be rejected
	// regardless.  Also, create a block that builds on the rejected block.
	//
	//   ... -> bcs1(6) -> bms1(7)
	//                 \-> bsl3(7) -> bsl4(8)
	g.SetTip("bcs1")
	tooLargeCbScript := repeatOpcode(0x00, maxCoinbaseScriptLen+1)
	g.NextBlock("bsl3", outs[7], ticketOuts[7],
		replaceCoinbaseSigScript(tooLargeCbScript))
	rejected(blockchain.ErrBadCoinbaseScriptLen)

	// Parent was rejected, so this block must either be an orphan or
	// outright rejected due to an invalid parent.
	g.NextBlock("bsl4", outs[8], ticketOuts[8])
	orphanedOrRejected()

	// Create block that has a max length coinbase script.
	//
	//   ... -> bms1(7) -> bsl5(8)
	g.SetTip("bms1")
	maxSizeCbScript := repeatOpcode(0x00, maxCoinbaseScriptLen)
	g.NextBlock("bsl5", outs[8], ticketOuts[8],
		replaceCoinbaseSigScript(maxSizeCbScript))
	accepted()

	// ---------------------------------------------------------------------
	// Vote tests.
	// ---------------------------------------------------------------------

	// Attempt to add block where vote has a null ticket reference hash.
	//
	//   ... -> bsl5(8)
	//                 \-> bv1(9)
	g.SetTip("bsl5")
	g.NextBlock("bv1", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.STransactions[0].TxIn[1].PreviousOutPoint = wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: math.MaxUint32,
			Tree:  wire.TxTreeRegular,
		}
	})
	rejected(blockchain.ErrBadTxInput)

	// Attempt to add block with a regular tx in the stake tree.
	//
	//   ... -> bsl5(8)
	//                 \-> bv2(9)
	g.SetTip("bsl5")
	g.NextBlock("bv2", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.STransactions[0] = b.Transactions[0]
	})
	rejected(blockchain.ErrRegTxInStakeTree)

	// Attempt to add block with too many votes.
	//
	//   ... -> bsl5(8)
	//                 \-> bv3(9)
	g.SetTip("bsl5")
	g.NextBlock("bv3", outs[9], ticketOuts[9],
		g.ReplaceWithNVotes(ticketsPerBlock+1))
	rejected(blockchain.ErrTooManyVotes)

	// Attempt to add block with too few votes.
	//
	//   ... -> bsl5(8)
	//                 \-> bv4(9)
	g.SetTip("bsl5")
	g.NextBlock("bv4", outs[9], ticketOuts[9],
		g.ReplaceWithNVotes(ticketsPerBlock/2))
	rejected(blockchain.ErrNotEnoughVotes)

	// Attempt to add block with different number of votes in stake tree and
	// header.
	//
	//   ... -> bsl5(8)
	//                 \-> bv5(9)
	g.SetTip("bsl5")
	g.NextBlock("bv5", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.Header.FreshStake--
	})
	rejected(blockchain.ErrFreshStakeMismatch)

	// Attempt to add block with a ticket voting on the parent of the actual
	// block it should be voting for.
	//
	//   ... -> bsl5(8)
	//                 \-> bv6(9)
	g.SetTip("bsl5")
	g.NextBlock("bv6", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		parent := g.BlockByHash(&b.Header.PrevBlock)
		voteBlock := g.BlockByHash(&parent.Header.PrevBlock)
		script := chaingen.VoteCommitmentScript(voteBlock.BlockHash(),
			voteBlock.Header.Height)
		b.STransactions[0].TxOut[0].PkScript = script
	})
	rejected(blockchain.ErrVotesOnWrongBlock)

	// Attempt to add block with a ticket voting on the correct block hash,
	// but the wrong block height.
	//
	//   ... -> bsl5(8)
	//                \-> bv6a(9)
	g.SetTip("bsl5")
	g.NextBlock("bv6a", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		parent := g.BlockByHash(&b.Header.PrevBlock)
		script := chaingen.VoteCommitmentScript(parent.BlockHash(),
			b.Header.Height)
		b.STransactions[1].TxOut[0].PkScript = script
	})
	rejected(blockchain.ErrVotesOnWrongBlock)

	// Attempt to add block with a ticket voting on the correct block height,
	// but the wrong block hash.
	//
	//   ... -> bsl5(8)
	//                \-> bv6b(9)
	g.SetTip("bsl5")
	g.NextBlock("bv6b", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		parent := g.BlockByHash(&b.Header.PrevBlock)
		voteBlock := g.BlockByHash(&parent.Header.PrevBlock)
		script := chaingen.VoteCommitmentScript(voteBlock.BlockHash(),
			parent.Header.Height)
		b.STransactions[2].TxOut[0].PkScript = script
	})
	rejected(blockchain.ErrVotesOnWrongBlock)

	// Attempt to add block with incorrect votebits set.
	// Everyone votes Yes, but block header says No.
	//
	//   ... -> bsl5(8)
	//                 \-> bv7(9)
	g.SetTip("bsl5")
	g.NextBlock("bv7", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.Header.VoteBits &^= voteBitYes

		// Leaving vote bits as is since all blocks from the generator have
		// votes set to Yes by default
	})
	rejected(blockchain.ErrIncongruentVotebit)

	// Attempt to add block with incorrect votebits set.
	// Everyone votes No, but block header says Yes.
	//
	//   ... -> bsl5(8)
	//                 \-> bv8(9)
	g.SetTip("bsl5")
	g.NextBlock("bv8", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.Header.VoteBits |= voteBitYes

		for i := 0; i < 5; i++ {
			g.ReplaceVoteBitsN(i, voteBitNo)(b)
		}
	})
	rejected(blockchain.ErrIncongruentVotebit)

	// Attempt to add block with incorrect votebits set.
	// 3x No 2x Yes, but block header says Yes.
	//
	//   ... -> bsl5(8)
	//                 \-> bv9(9)
	g.SetTip("bsl5")
	g.NextBlock("bv9", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.Header.VoteBits |= voteBitYes

		for i := 0; i < 3; i++ {
			g.ReplaceVoteBitsN(i, voteBitNo)(b)
		}
	})
	rejected(blockchain.ErrIncongruentVotebit)

	// Attempt to add block with incorrect votebits set.
	// 2x No 3x Yes, but block header says No.
	//
	//   ... -> bsl5(8)
	//                 \-> bv10(9)
	g.SetTip("bsl5")
	g.NextBlock("bv10", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.Header.VoteBits &^= voteBitYes

		for i := 0; i < 2; i++ {
			g.ReplaceVoteBitsN(i, voteBitNo)(b)
		}
	})
	rejected(blockchain.ErrIncongruentVotebit)

	// Create block with a header that commits to less votes
	// than the block actually contains.
	//
	//   ... -> bsl5(8)
	//                 \-> bv11(9)
	g.SetTip("bsl5")
	g.NextBlock("bv11", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.Header.Voters--
	})
	rejected(blockchain.ErrVotesMismatch)

	// Attempt to add block with incorrect votebits set.
	// 4x Voters
	// 2x No 2x Yes, but block header says Yes
	//   ... -> bsl5(8)
	//                 \-> bv12(9)
	g.SetTip("bsl5")
	g.NextBlock("bv12", outs[9], ticketOuts[9], g.ReplaceWithNVotes(4),
		func(b *wire.MsgBlock) {
			b.Header.VoteBits |= voteBitYes

			for i := 0; i < 2; i++ {
				g.ReplaceVoteBitsN(i, voteBitNo)(b)
			}
		})
	rejected(blockchain.ErrIncongruentVotebit)

	// Attempt to add block with incorrect votebits set.
	// 3x Voters
	// 2x No 1x Yes, but block header says Yes
	//   ... -> bsl5(8)
	//                 \-> bv12(9)
	g.SetTip("bsl5")
	g.NextBlock("bv13", outs[9], ticketOuts[9], g.ReplaceWithNVotes(3),
		func(b *wire.MsgBlock) {
			b.Header.VoteBits |= voteBitYes

			for i := 0; i < 2; i++ {
				g.ReplaceVoteBitsN(i, voteBitNo)(b)
			}
		})
	rejected(blockchain.ErrIncongruentVotebit)

	// Attempt to add block with incorrect votebits set.
	// 3x Voters
	// 1x No 2x Yes, but block header says No
	//   ... -> bsl5(8)
	//                 \-> bv14(9)
	g.SetTip("bsl5")
	g.NextBlock("bv14", outs[9], ticketOuts[9], g.ReplaceWithNVotes(3),
		func(b *wire.MsgBlock) {
			b.Header.VoteBits &^= voteBitYes

			for i := 0; i < 1; i++ {
				g.ReplaceVoteBitsN(i, voteBitNo)(b)
			}
		})
	rejected(blockchain.ErrIncongruentVotebit)

	// Attempt to add block with a bad ticket purchase commitment.
	//
	//   ... -> bsl5(8)
	//                 \-> bv15(9)
	g.SetTip("bsl5")
	g.NextBlock("bv15", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		ticketFee := dcrutil.Amount(2)
		ticketPrice := dcrutil.Amount(g.CalcNextReqStakeDifficulty(g.Tip()))
		ticketPrice--
		b.STransactions[5].TxOut[1].PkScript =
			chaingen.PurchaseCommitmentScript(g.P2shOpTrueAddr(),
				ticketPrice+ticketFee, 0, ticketPrice)
	})
	rejected(blockchain.ErrSStxCommitment)

	// Attempt to add block with a ticket purchase using output from
	// disapproved block.
	//
	//   ... -> bsl5(8)
	//                 \-> bv16(9)
	g.SetTip("bsl5")
	g.NextBlock("bv16", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.Header.VoteBits &^= voteBitYes
		for i := 0; i < 5; i++ {
			g.ReplaceVoteBitsN(i, voteBitNo)(b)
		}

		prevBlock := g.Tip()
		spend := chaingen.MakeSpendableOut(prevBlock, 1, 0)
		ticketPrice := dcrutil.Amount(g.CalcNextReqStakeDifficulty(g.Tip()))
		ticket := g.CreateTicketPurchaseTx(&spend, ticketPrice, lowFee)
		b.AddSTransaction(ticket)
		b.Header.FreshStake++
	})
	rejected(blockchain.ErrMissingTxOut)

	// Attempt to add block with a regular transaction using output
	// from disapproved block.
	//
	//   ... -> bsl5(8)
	//                 \-> bv17(9)
	g.SetTip("bsl5")
	g.NextBlock("bv17", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.Header.VoteBits &^= voteBitYes
		for i := 0; i < 5; i++ {
			g.ReplaceVoteBitsN(i, voteBitNo)(b)
		}

		prevBlock := g.Tip()
		spend := chaingen.MakeSpendableOut(prevBlock, 1, 0)
		tx := g.CreateSpendTx(&spend, lowFee)
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Stake ticket difficulty tests.
	// ---------------------------------------------------------------------

	// Create block with ticket purchase below required ticket price.
	//
	//   ... -> bsl5(8)
	//                 \-> bsd0(9)
	g.SetTip("bsl5")
	g.NextBlock("bsd0", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.STransactions[5].TxOut[0].Value--
	})
	rejected(blockchain.ErrNotEnoughStake)

	// Create block with stake transaction below pos limit.
	//
	//   ... -> bsl5(8)
	//                 \-> bsd1(9)
	g.SetTip("bsl5")
	g.NextBlock("bsd1", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		minStakeDiff := g.Params().MinimumStakeDiff
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
	//   ... -> bsl5(8)
	//                 \-> bss0(9)
	g.SetTip("bsl5")
	tooSmallCbScript = repeatOpcode(0x00, minCoinbaseScriptLen-1)
	g.NextBlock("bss0", outs[9], ticketOuts[9],
		replaceStakeSigScript(tooSmallCbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Create block that has a stakebase script that is larger than the
	// maximum allowed length.
	//
	//   ... -> bsl5(8)
	//                 \-> bss1(9)
	g.SetTip("bsl5")
	tooLargeCbScript = repeatOpcode(0x00, maxCoinbaseScriptLen+1)
	g.NextBlock("bss1", outs[9], ticketOuts[9],
		replaceStakeSigScript(tooLargeCbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Add a block with a stake transaction with a signature script that is
	// not the required script, but is otherwise a valid script.
	//
	//   ... -> bsl5(8)
	//                 \-> bss2(9)
	g.SetTip("bsl5")
	badScript := append(g.Params().StakeBaseSigScript, 0x00)
	g.NextBlock("bss2", outs[9], ticketOuts[9], replaceStakeSigScript(badScript))
	rejected(blockchain.ErrBadStakebaseScrVal)

	// Attempt to add a block with a bad vote payee output.
	//
	//   ... -> bsl5(8)
	//                 \-> bss3(9)
	g.SetTip("bsl5")
	g.NextBlock("bss3", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.STransactions[0].TxOut[2].PkScript[8] ^= 0x55
	})
	rejected(blockchain.ErrSSGenPayeeOuts)

	// Attempt to add a block with an incorrect vote payee output amount.
	//
	//   ... -> bsl5(8)
	//                 \-> bss4(9)
	g.SetTip("bsl5")
	g.NextBlock("bss4", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.STransactions[0].TxOut[2].Value++
	})
	rejected(blockchain.ErrSSGenPayeeOuts)

	// ---------------------------------------------------------------------
	// Multisig[Verify]/ChecksigVerifiy signature operation count tests.
	// ---------------------------------------------------------------------

	// Create block with max signature operations as OP_CHECKMULTISIG.
	//
	//   ... -> bsl5(8) -> bmo1(9)
	//
	// OP_CHECKMULTISIG counts for 20 sigops.
	g.SetTip("bsl5")
	manySigOps = repeatOpcode(txscript.OP_CHECKMULTISIG, maxBlockSigOps/20)
	g.NextBlock("bmo1", outs[9], ticketOuts[9], replaceSpendScript(manySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with more than max allowed signature operations using
	// OP_CHECKMULTISIG.
	//
	//   ... -> bmo1(9)
	//                 \-> bmo2(10)
	//
	// OP_CHECKMULTISIG counts for 20 sigops.
	tooManySigOps = repeatOpcode(txscript.OP_CHECKMULTISIG, maxBlockSigOps/20)
	tooManySigOps = append(tooManySigOps, txscript.OP_CHECKSIG)
	g.NextBlock("bmo2", outs[10], ticketOuts[10],
		replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// Create block with max signature operations as OP_CHECKMULTISIGVERIFY.
	//
	//   ... -> bmo1(9) -> bmo3(10)
	//
	g.SetTip("bmo1")
	manySigOps = repeatOpcode(txscript.OP_CHECKMULTISIGVERIFY, maxBlockSigOps/20)
	g.NextBlock("bmo3", outs[10], ticketOuts[10], replaceSpendScript(manySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with more than max allowed signature operations using
	// OP_CHECKMULTISIGVERIFY.
	//
	//   ... -> bmo3(10)
	//                  \-> bmo4(11)
	//
	tooManySigOps =
		repeatOpcode(txscript.OP_CHECKMULTISIGVERIFY, maxBlockSigOps/20)
	tooManySigOps = append(tooManySigOps, txscript.OP_CHECKSIG)
	g.NextBlock("bmo4", outs[11], ticketOuts[11],
		replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// Create block with max signature operations as OP_CHECKSIGVERIFY.
	//
	//   ... -> bmo3(10) -> bmo5(11)
	//
	g.SetTip("bmo3")
	manySigOps = repeatOpcode(txscript.OP_CHECKSIGVERIFY, maxBlockSigOps)
	g.NextBlock("bmo5", outs[11], ticketOuts[11], replaceSpendScript(manySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with more than max allowed signature operations using
	// OP_CHECKSIGVERIFY.
	//
	//   ... -> bmo5(11)
	//                  \-> bmo6(12)
	//
	tooManySigOps = repeatOpcode(txscript.OP_CHECKSIGVERIFY, maxBlockSigOps+1)
	g.NextBlock("bmo6", outs[12], ticketOuts[12],
		replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// ---------------------------------------------------------------------
	// Spending of tx outputs in block that failed to connect tests.
	// ---------------------------------------------------------------------

	// Create block that spends a transaction from a block that failed to
	// connect (due to containing a double spend).
	//
	//   ... -> bmo5(11)
	//                  \-> bsp1(12)
	//                  \-> bsp2(bsp1.tx[1])
	//                  \-> bsp3(bsp1.tx[1])
	//
	g.SetTip("bmo5")
	doubleSpendTx := g.CreateSpendTx(outs[12], lowFee)
	g.NextBlock("bsp1", outs[12], ticketOuts[12], additionalPoWTx(doubleSpendTx))
	bsp1Tx1Out := chaingen.MakeSpendableOut(g.Tip(), 1, 0)
	rejected(blockchain.ErrMissingTxOut)

	g.SetTip("bmo5")
	g.NextBlock("bsp2", &bsp1Tx1Out, ticketOuts[12])
	rejected(blockchain.ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Pay-to-script-hash signature operation count tests.
	// ---------------------------------------------------------------------

	// Create a private/public key pair for signing transactions.
	privKey, err := secp256k1.GeneratePrivateKey()
	if err != nil {
		panic(err)
	}
	pubKey := secp256k1.PublicKey(privKey.PublicKey)

	// Create a pay-to-script-hash redeem script that consists of 9
	// signature operations to be used in the next three blocks.
	const redeemScriptSigOps = 9
	redeemScript := pushDataScript(pubKey.SerializeCompressed())
	redeemScript = append(redeemScript, bytes.Repeat([]byte{txscript.OP_2DUP,
		txscript.OP_CHECKSIGVERIFY}, redeemScriptSigOps-1)...)
	redeemScript = append(redeemScript, txscript.OP_CHECKSIG)
	g.AssertScriptSigOpsCount(redeemScript, redeemScriptSigOps)

	// Create a block that has enough pay-to-script-hash outputs such that
	// another block can be created that consumes them all and exceeds the
	// max allowed signature operations per block.
	//
	//   ... -> bmo5(11) -> bshso0 (12)
	g.SetTip("bmo5")
	bshso0 := g.NextBlock("bshso0", outs[12], ticketOuts[12],
		func(b *wire.MsgBlock) {
			// Create a chain of transactions each spending from the
			// previous one such that each contains an output that pays to
			// the redeem script and the total number of signature
			// operations in those redeem scripts will be more than the
			// max allowed per block.
			p2shScript := payToScriptHashScript(redeemScript)
			txnsNeeded := (maxBlockSigOps / redeemScriptSigOps) + 1
			prevTx := b.Transactions[1]
			for i := 0; i < txnsNeeded; i++ {
				prevTx = g.CreateSpendTxForTx(prevTx, b.Header.Height,
					uint32(i)+1, lowFee)
				prevTx.TxOut[0].Value -= 2
				prevTx.AddTxOut(wire.NewTxOut(2, p2shScript))
				b.AddTransaction(prevTx)
			}
		})
	g.AssertTipBlockNumTxns((maxBlockSigOps / redeemScriptSigOps) + 3)
	accepted()

	// Create a block with more than max allowed signature operations where
	// the majority of them are in pay-to-script-hash scripts.
	//
	//   ... -> bmo5(11) -> bshso0(12)
	//                                \-> bshso1(13)
	g.NextBlock("bshso1", outs[13], ticketOuts[13], func(b *wire.MsgBlock) {
		txnsNeeded := (maxBlockSigOps / redeemScriptSigOps)
		for i := 0; i < txnsNeeded; i++ {
			// Create a signed transaction that spends from the
			// associated p2sh output in bshso0.
			spend := chaingen.MakeSpendableOut(bshso0, uint32(i+2), 2)
			tx := g.CreateSpendTx(&spend, lowFee)
			sig, err := txscript.RawTxInSignature(tx, 0,
				redeemScript, txscript.SigHashAll, privKey)
			if err != nil {
				panic(err)
			}
			tx.TxIn[0].SignatureScript = pushDataScript(sig,
				redeemScript)
			b.AddTransaction(tx)
		}

		// Create a final tx that includes a non-pay-to-script-hash
		// output with the number of signature operations needed to push
		// the block one over the max allowed.
		fill := maxBlockSigOps - (txnsNeeded * redeemScriptSigOps) + 1
		finalTxIndex := uint32(len(b.Transactions)) - 1
		finalTx := b.Transactions[finalTxIndex]
		tx := g.CreateSpendTxForTx(finalTx, b.Header.Height,
			finalTxIndex, lowFee)
		tx.TxOut[0].PkScript = repeatOpcode(txscript.OP_CHECKSIG, fill)
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrTooManySigOps)

	// Create a block with the max allowed signature operations where the
	// majority of them are in pay-to-script-hash scripts.
	//
	//   ... -> bmo5(11) -> bshso0(12) -> bshso2(13)
	g.SetTip("bshso0")
	g.NextBlock("bshso2", outs[13], ticketOuts[13], func(b *wire.MsgBlock) {
		txnsNeeded := (maxBlockSigOps / redeemScriptSigOps)
		for i := 0; i < txnsNeeded; i++ {
			// Create a signed transaction that spends from the
			// associated p2sh output in bshso0.
			spend := chaingen.MakeSpendableOut(bshso0, uint32(i+2), 2)
			tx := g.CreateSpendTx(&spend, lowFee)
			sig, err := txscript.RawTxInSignature(tx, 0,
				redeemScript, txscript.SigHashAll, privKey)
			if err != nil {
				panic(err)
			}
			tx.TxIn[0].SignatureScript = pushDataScript(sig,
				redeemScript)
			b.AddTransaction(tx)
		}

		// Create a final tx that includes a non-pay-to-script-hash
		// output with the number of signature operations needed to push
		// the block to exactly the max allowed.
		fill := maxBlockSigOps - (txnsNeeded * redeemScriptSigOps)
		if fill == 0 {
			return
		}
		finalTxIndex := uint32(len(b.Transactions)) - 1
		finalTx := b.Transactions[finalTxIndex]
		tx := g.CreateSpendTxForTx(finalTx, b.Header.Height,
			finalTxIndex, lowFee)
		tx.TxOut[0].PkScript = repeatOpcode(txscript.OP_CHECKSIG, fill)
		b.AddTransaction(tx)
	})
	accepted()

	// ---------------------------------------------------------------------
	// Reset the chain to a stable base.
	//
	//   ... -> bmo5(11) -> brs1(12)   -> brs2(13)  -> brs3(14)
	//                  \-> bshso0(12) -> bshso2(13)
	// ---------------------------------------------------------------------

	g.SetTip("bmo5")
	g.NextBlock("brs1", outs[12], ticketOuts[12])
	acceptedToSideChainWithExpectedTip("bshso2")

	g.NextBlock("brs2", outs[13], ticketOuts[13])
	acceptedToSideChainWithExpectedTip("bshso2")

	g.NextBlock("brs3", outs[14], ticketOuts[14])
	accepted()

	// Collect all of the spendable coinbase outputs from the previous
	// collection point up to the current tip and add them to the slices.
	// This is necessary since the coinbase maturity is small enough such
	// that there are no more spendable outputs without using the ones
	// created in the previous tests.
	g.SaveSpendableCoinbaseOuts()
	for g.NumSpendableCoinbaseOuts() > 0 {
		coinbaseOuts := g.OldestCoinbaseOuts()
		outs = append(outs, &coinbaseOuts[0])
		ticketOuts = append(ticketOuts, coinbaseOuts[1:])
	}

	// ---------------------------------------------------------------------
	// Various malformed block tests.
	// ---------------------------------------------------------------------

	// Create block with an otherwise valid transaction in place of where
	// the coinbase must be.
	//
	//   ... -> brs3(14)
	//                  \-> bmf1(15)
	g.NextBlock("bmf1", nil, ticketOuts[15], func(b *wire.MsgBlock) {
		nonCoinbaseTx := g.CreateSpendTx(outs[15], lowFee)
		b.Transactions[0] = nonCoinbaseTx
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with no transactions.
	//
	//   ... -> brs3(14)
	//                  \-> bmf2(_)
	g.SetTip("brs3")
	g.NextBlock("bmf2", nil, nil, func(b *wire.MsgBlock) {
		b.Transactions = nil
	})
	rejected(blockchain.ErrNoTransactions)

	// Create block with invalid proof of work.
	//
	//   ... -> brs3(14)
	//                  \-> bmf3(15)
	g.SetTip("brs3")
	bmf3 := g.NextBlock("bmf3", outs[15], ticketOuts[15])
	// This can't be done inside a munge function passed to NextBlock
	// because the block is solved after the function returns and this test
	// requires an unsolved block.  Thus, just increment the nonce until
	// it's not solved and then replace it in the generator's state.
	{
		origHash := bmf3.BlockHash()
		for chaingen.IsSolved(&bmf3.Header) {
			bmf3.Header.Nonce++
		}
		g.UpdateBlockState("bmf3", origHash, "bmf3", bmf3)
	}
	rejected(blockchain.ErrHighHash)

	// Create block with a timestamp too far in the future.
	//
	//   ... -> brs3(14)
	//                  \-> bmf4(15)
	g.SetTip("brs3")
	g.NextBlock("bmf4", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		// 3 hours in the future clamped to 1 second precision.
		nowPlus3Hours := time.Now().Add(time.Hour * 3)
		b.Header.Timestamp = time.Unix(nowPlus3Hours.Unix(), 0)
	})
	rejected(blockchain.ErrTimeTooNew)

	// Create block with an invalid merkle root.
	//
	//   ... -> brs3(14)
	//                  \-> bmf5(15)
	g.SetTip("brs3")
	g.NextBlock("bmf5", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		// Set the merkle root to an invalid hash.
		b.Header.MerkleRoot = chainhash.Hash{}
	})
	g.AssertTipBlockMerkleRoot(chainhash.Hash{})
	rejected(blockchain.ErrBadMerkleRoot)

	// Create block with an invalid stake root.
	//
	//   ... -> brs3(14)
	//                  \-> bmf6(15)
	g.SetTip("brs3")
	g.NextBlock("bmf6", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		// Set the stake root to an invalid hash.
		b.Header.StakeRoot = chainhash.Hash{}
	})
	g.AssertTipBlockStakeRoot(chainhash.Hash{})
	rejected(blockchain.ErrBadMerkleRoot)

	// Create block with an invalid block size.
	//
	//   ... -> brs3(14)
	//                  \-> bmf7(15)
	g.SetTip("brs3")
	g.NextBlock("bmf7", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Header.Size++
	})
	rejected(blockchain.ErrWrongBlockSize)

	// Create block with an invalid subsidy for a coinbase input.
	//
	//   ... -> brs3(14)
	//                  \-> bmf8(15)
	g.SetTip("brs3")
	g.NextBlock("bmf8", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[0].TxIn[0].ValueIn++
	})
	rejected(blockchain.ErrBadCoinbaseAmountIn)

	// Create block with an invalid subsidy for a stakebase input.
	//
	//   ... -> brs3(14)
	//                  \-> bmf9(15)
	g.SetTip("brs3")
	g.NextBlock("bmf9", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.STransactions[0].TxIn[0].ValueIn++
	})
	rejected(blockchain.ErrBadStakebaseAmountIn)

	// Create block with a header that commits to more revocations
	// than the block actually contains.
	//
	//   ... -> brs3(14)
	//                  \-> bmf10(15)
	g.SetTip("brs3")
	g.NextBlock("bmf10", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Header.Revocations++
	})
	rejected(blockchain.ErrRevocationsMismatch)

	// Create block with an invalid proof-of-work limit.
	//
	//   ... -> brs3(14)
	//                  \-> bmf11(15)
	g.SetTip("brs3")
	g.NextBlock("bmf11", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		// Set an invalid POW limit.
		b.Header.Bits--
	})
	rejected(blockchain.ErrUnexpectedDifficulty)

	// Create block with an invalid negative proof-of-work limit.
	//
	//   ... -> brs3(14)
	//                  \-> bmf12(15)
	g.SetTip("brs3")
	bmf12 := g.NextBlock("bmf12", outs[15], ticketOuts[15])
	// This can't be done inside a munge function passed to nextBlock
	// because the block is solved after the function returns and this test
	// involves an unsolvable block.
	{
		origHash := bmf12.BlockHash()
		bmf12.Header.Bits = 0x01810000 // -1 in compact form.
		g.UpdateBlockState("bmf12", origHash, "bmf12", bmf12)
	}
	rejected(blockchain.ErrUnexpectedDifficulty)

	// Create block with two coinbase transactions.
	//
	//   ... -> brs3(14)
	//                  \-> bmf13(15)
	g.SetTip("brs3")
	coinbaseTx := g.CreateCoinbaseTx(g.Tip().Header.Height+1, ticketsPerBlock)
	g.NextBlock("bmf13", outs[15], ticketOuts[15], additionalPoWTx(coinbaseTx))
	rejected(blockchain.ErrMultipleCoinbases)

	// Create block with duplicate transactions in the regular transaction
	// tree.
	//
	// This test relies on the shape of the shape of the merkle tree to test
	// the intended condition.  That is the reason for the assertion.
	//
	//   ... -> brs3(14)
	//                  \-> bmf14(15)
	g.SetTip("brs3")
	g.NextBlock("bmf14", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.AddTransaction(b.Transactions[1])
	})
	g.AssertTipBlockNumTxns(3)
	rejected(blockchain.ErrDuplicateTx)

	// Create a block that spends a transaction that does not exist.
	//
	//   ... -> brs3(14)
	//                  \-> bmf15(15)
	g.SetTip("brs3")
	g.NextBlock("bmf15", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		hash := newHashFromStr("00000000000000000000000000000000" +
			"00000000000000000123456789abcdef")
		b.Transactions[1].TxIn[0].PreviousOutPoint.Hash = *hash
		b.Transactions[1].TxIn[0].PreviousOutPoint.Index = 0
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block with stake tx in regular tx tree.
	//
	//   ... -> brs3(14)
	//                  \-> bmf16(15)
	g.SetTip("brs3")
	g.NextBlock("bmf16", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.AddTransaction(b.STransactions[1])
	})
	rejected(blockchain.ErrStakeTxInRegularTree)

	// Create block with a regular transaction that commits to an
	// invalid block index.
	//
	//   ... -> brs3(14)
	//                  \-> bmf17(15)
	g.SetTip("brs3")
	g.NextBlock("bmf17", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		txOut := chaingen.MakeSpendableOut(b, 1, 0)
		tx := g.CreateSpendTx(&txOut, lowFee)
		tx.TxIn[0].BlockIndex++
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrFraudBlockIndex)

	// Create block with a regular transaction that commits to an
	// invalid block height.
	//
	//   ... -> brs3(14)
	//                  \-> bmf18(15)
	g.SetTip("brs3")
	g.NextBlock("bmf18", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		txOut := chaingen.MakeSpendableOut(b, 1, 0)
		tx := g.CreateSpendTx(&txOut, lowFee)
		tx.TxIn[0].BlockHeight++
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrFraudBlockHeight)

	// Create block with a regular transaction that commits to an
	// invalid input amount.
	//   ... -> brs3(14)
	//                  \-> bmf19(15)
	g.SetTip("brs3")
	g.NextBlock("bmf19", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		txOut := chaingen.MakeSpendableOut(b, 1, 0)
		tx := g.CreateSpendTx(&txOut, lowFee)
		tx.TxIn[0].ValueIn--
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrFraudAmountIn)

	// Create block with an expired transaction in the regular tx tree.
	//
	//   ... -> brs3(14)
	//                  \-> bmf20(15)
	g.SetTip("brs3")
	g.NextBlock("bmf20", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		txOut := chaingen.MakeSpendableOut(b, 1, 0)
		tx := g.CreateSpendTx(&txOut, lowFee)
		tx.Expiry = b.Header.Height
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrExpiredTx)

	// Create block with an expired transaction in the stake tx tree.
	//
	//   ... -> brs3(14)
	//                  \-> bmf20b(15)
	g.SetTip("brs3")
	g.NextBlock("bmf20b", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.STransactions[5].Expiry = b.Header.Height
	})
	rejected(blockchain.ErrExpiredTx)

	// Create block that commits to an invalid height.
	//
	//   ... -> brs3(14)
	//                  \-> bmf21(15)
	g.SetTip("brs3")
	g.NextBlock("bmf21", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Header.Height++
	})
	rejected(blockchain.ErrBadBlockHeight)

	// Create block that commits to an invalid ticket pool size.
	//
	//   ... -> brs3(14)
	//                  \-> bmf22(15)
	g.SetTip("brs3")
	g.NextBlock("bmf22", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Header.PoolSize++
	})
	rejected(blockchain.ErrPoolSize)

	// Create block that commits to an invalid final state.
	//
	//   ... -> brs3(14)
	//                  \-> bmf23(15)
	g.SetTip("brs3")
	g.NextBlock("bmf23", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Header.FinalState[0] ^= 0x55
	})
	rejected(blockchain.ErrInvalidFinalState)

	// Create block with a regular transaction that commits to a
	// malformed spend script.
	//
	//   ... -> brs3(14)
	//                  \-> bmf24(15)
	g.SetTip("brs3")
	g.NextBlock("bmf24", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		spendTx := chaingen.MakeSpendableOut(b, 1, 0)
		tx := g.CreateSpendTx(&spendTx, lowFee)
		tx.TxOut[0].PkScript = []byte{0x01, 0x02, 0x03, 0x04}
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrScriptMalformed)

	// Create block that spends immature stakebase from a vote in
	// a ticket purchase.
	//
	//   ... -> brs3(14)
	//                  \-> bmf25(15)
	g.SetTip("brs3")
	g.NextBlock("bmf25", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		spendOut := chaingen.MakeSpendableStakeOut(b, 0, 2)
		ticketPrice := dcrutil.Amount(g.CalcNextReqStakeDifficulty(g.Tip()))
		ticket := g.CreateTicketPurchaseTx(&spendOut, ticketPrice, lowFee)
		b.AddSTransaction(ticket)
		b.Header.FreshStake++
	})
	rejected(blockchain.ErrImmatureSpend)

	// Create block with an invalid stake transaction signature script.
	//
	//   ... -> brs3(14)
	//                  \-> bmf26(15)
	g.SetTip("brs3")
	g.NextBlock("bmf26", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.STransactions[5].TxIn[0].SignatureScript = invalidP2SHRedeemScript
	})
	rejected(blockchain.ErrScriptValidation)

	// Create block with an invalid regular transaction signature script.
	//
	//   ... -> brs3(14)
	//                  \-> bmf27(15)
	g.SetTip("brs3")
	g.NextBlock("bmf27", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[1].TxIn[0].SignatureScript = invalidP2SHRedeemScript
	})
	rejected(blockchain.ErrScriptValidation)

	// Create block that tries to spend an input expected to be in a different
	// tx tree than the one given in the TxIn outpoint.
	//
	//   ... -> brs3(14)
	//                  \-> bmf28(15)
	g.SetTip("brs3")
	g.NextBlock("bmf28", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		tx := g.CreateSpendTxForTx(b.Transactions[1], b.Header.Height, 1, lowFee)
		tx.TxIn[0].PreviousOutPoint.Tree = wire.TxTreeStake
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrDiscordantTxTree)

	// Create block with no dev subsidy for coinbase transaction.
	//
	//   ... -> brs3(14)
	//                  \-> bmf29(15)
	g.SetTip("brs3")
	g.NextBlock("bmf29", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut[0] = b.Transactions[0].TxOut[1]
	})
	rejected(blockchain.ErrNoTax)

	// Create block with an incorrect dev subsidy output amount.
	//
	//   ... -> brs3(14)
	//                  \-> bmf30(15)
	g.SetTip("brs3")
	g.NextBlock("bmf30", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut[0].Value--
	})
	rejected(blockchain.ErrNoTax)

	// Create block that tries to buy a ticket with the block's coinbase
	// transaction.
	//
	//   ... -> brs3(14)
	//                  \-> bmf31(15)
	g.SetTip("brs3")
	g.NextBlock("bmf31", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		spend := chaingen.MakeSpendableOut(b, 0, 0)
		ticketPrice := dcrutil.Amount(g.CalcNextReqStakeDifficulty(g.Tip()))
		ticket := g.CreateTicketPurchaseTx(&spend, ticketPrice, lowFee)
		b.AddSTransaction(ticket)
		b.Header.FreshStake++
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block that attempts to spend a zero value output.
	//
	//   ... -> brs3(14)
	//                  \-> bmf32(15)
	g.SetTip("brs3")
	g.NextBlock("bmf32", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		// Create tx with zero value output.
		spend := chaingen.MakeSpendableOut(b, 1, 0)
		zeroOutputTx := g.CreateSpendTx(&spend, spend.Amount())
		b.AddTransaction(zeroOutputTx)

		// Spend from zero value output that was just created.
		zeroSpend := chaingen.MakeSpendableOut(b, 2, 0)
		zeroSpendTx := g.CreateSpendTx(&zeroSpend, 0)
		b.AddTransaction(zeroSpendTx)
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block with a vote that attempts to spend a ticket on a side chain.
	//
	//   ... -> brs3(14)
	//                  \-> bmf33(15)
	g.SetTip("brs3")
	g.NextBlock("bmf33", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		bsp1 := g.BlockByName("bsp1")
		b.STransactions[4].TxIn[1] = bsp1.STransactions[4].TxIn[1]
	})
	rejected(blockchain.ErrTicketUnavailable)

	// Create block that tries to spend a ticket purchase output as a regular
	// transaction.
	//
	//   ... -> brs3(14)
	//                  \-> bmf34(15)
	g.SetTip("brs3")
	g.NextBlock("bmf34", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		brs3 := g.BlockByName("brs3")
		spendOut := chaingen.MakeSpendableOutForSTx(brs3.STransactions[5],
			brs3.Header.Height, 5, 0)
		tx := g.CreateSpendTx(&spendOut, lowFee)
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrTxSStxOutSpend)

	// Create block that spends immature change from one ticket purchase in
	// another ticket purchase.
	//
	//   ... -> brs3(14)
	//                  \-> bmf35(15)
	g.SetTip("brs3")
	g.NextBlock("bmf35", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		spend := chaingen.MakeSpendableStakeOut(b, 5, 2)
		ticketPrice := dcrutil.Amount(g.CalcNextReqStakeDifficulty(g.Tip()))
		ticket := g.CreateTicketPurchaseTx(&spend, ticketPrice, lowFee)
		b.AddSTransaction(ticket)
		b.Header.FreshStake++
	})
	rejected(blockchain.ErrImmatureSpend)

	// Create block with a malformed outputs order for a ticket purchase.
	//
	//   ... -> brs3(14)
	//                  \-> bmf36(15)
	g.SetTip("brs3")
	g.NextBlock("bmf36", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		prevBlock := g.Tip()
		spendOut := chaingen.MakeSpendableOut(prevBlock, 1, 0)
		ticketPrice := dcrutil.Amount(g.CalcNextRequiredStakeDifficulty())
		ticket := g.CreateTicketPurchaseTx(&spendOut, ticketPrice, lowFee)
		ticket.TxOut[0], ticket.TxOut[2] = ticket.TxOut[2], ticket.TxOut[0]
		b.AddSTransaction(ticket)
		b.Header.FreshStake++
	})
	rejected(blockchain.ErrRegTxInStakeTree)

	// Create block with scripts that do not involve p2pkh or
	// p2sh addresses for a ticket purchase.
	//
	//   ... -> brs3(14)
	//                  \-> bmf37(15)
	g.SetTip("brs3")
	g.NextBlock("bmf37", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		prevBlock := g.Tip()
		spendOut := chaingen.MakeSpendableOut(prevBlock, 1, 0)
		ticketPrice := dcrutil.Amount(g.CalcNextRequiredStakeDifficulty())
		ticket := g.CreateTicketPurchaseTx(&spendOut, ticketPrice, lowFee)
		ticket.TxOut[0].PkScript = opTrueScript
		b.AddSTransaction(ticket)
		b.Header.FreshStake++
	})
	rejected(blockchain.ErrRegTxInStakeTree)

	// ---------------------------------------------------------------------
	// Block header median time tests.
	// ---------------------------------------------------------------------

	// Create a block with a timestamp that is exactly the median time.
	//
	//   ... bmo1(9) -> bmo3(10) -> bmo5(11) -> brs1(12) -> brs2(13) -> brs3(14)
	//                                                                          \-> bmt1(15)
	g.SetTip("brs3")
	g.NextBlock("bmt1", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		medianBlk := g.BlockByHash(&b.Header.PrevBlock)
		for i := 0; i < medianTimeBlocks/2; i++ {
			medianBlk = g.BlockByHash(&medianBlk.Header.PrevBlock)
		}
		b.Header.Timestamp = medianBlk.Header.Timestamp
	})
	rejected(blockchain.ErrTimeTooOld)

	// Create a block with a timestamp that is one second after the median
	// time.
	//
	//   ... bmo1(9) -> bmo3(10) -> bmo5(11) -> brs1(12) -> brs2(13) -> brs3(14) -> bmt2(15)
	g.SetTip("brs3")
	g.NextBlock("bmt2", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		medianBlk := g.BlockByHash(&b.Header.PrevBlock)
		for i := 0; i < medianTimeBlocks/2; i++ {
			medianBlk = g.BlockByHash(&medianBlk.Header.PrevBlock)
		}
		medianBlockTime := medianBlk.Header.Timestamp
		b.Header.Timestamp = medianBlockTime.Add(time.Second)
	})
	accepted()

	// ---------------------------------------------------------------------
	// CVE-2012-2459 (block hash collision due to merkle tree algo) tests.
	//
	// NOTE: This section originally came from upstream and applied to the
	// ability to create two blocks with the same hash that were not
	// identical through merkle tree tricks in Bitcoin, however, that attack
	// vector is not possible in Decred since the block size is included in
	// the header and adding an additional duplicate transaction changes the
	// size and consequently the hash.  The tests are therefore not ported
	// as they do not apply.
	// ---------------------------------------------------------------------

	// ---------------------------------------------------------------------
	// Invalid transaction type tests.
	// ---------------------------------------------------------------------

	// Create block with a transaction that tries to spend from an index
	// that is out of range from an otherwise valid and existing tx.
	//
	//   ... -> bmt2(15)
	//                  \-> bit1(16)
	g.SetTip("bmt2")
	g.NextBlock("bit1", outs[16], ticketOuts[16], func(b *wire.MsgBlock) {
		b.Transactions[1].TxIn[0].PreviousOutPoint.Index = 42
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block with transaction that pays more than its inputs.
	//
	//   ... -> bmt2(15)
	//                  \-> bit2(16)
	g.SetTip("bmt2")
	g.NextBlock("bit2", outs[16], ticketOuts[16], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = int64(outs[16].Amount()) + 1
	})
	rejected(blockchain.ErrSpendTooHigh)

	// ---------------------------------------------------------------------
	// BIP0030 tests.
	//
	// NOTE: This section originally came from upstream and applied to the
	// ability to create coinbase transactions with the same hash, however,
	// Decred enforces the coinbase includes the block height to which it
	// applies, so that condition is not possible.  The tests are therefore
	// not ported as they do not apply.
	// ---------------------------------------------------------------------

	// ---------------------------------------------------------------------
	// Blocks with non-final transaction tests.
	// ---------------------------------------------------------------------

	// Create block that contains a non-final non-coinbase transaction.
	//
	//   ... -> bmt2(15)
	//                  \-> bnt1(16)
	g.SetTip("bmt2")
	g.NextBlock("bnt1", outs[16], ticketOuts[16], func(b *wire.MsgBlock) {
		// A non-final transaction must have at least one input with a
		// non-final sequence number in addition to a non-final lock
		// time.
		b.Transactions[1].LockTime = 0xffffffff
		b.Transactions[1].TxIn[0].Sequence = 0
	})
	rejected(blockchain.ErrUnfinalizedTx)

	// Create block that contains a non-final coinbase transaction.
	//
	//   ... -> bmt2(15)
	//                  \-> bnt2(16)
	g.SetTip("bmt2")
	g.NextBlock("bnt2", outs[16], ticketOuts[16], func(b *wire.MsgBlock) {
		// A non-final transaction must have at least one input with a
		// non-final sequence number in addition to a non-final lock
		// time.
		b.Transactions[0].LockTime = 0xffffffff
		b.Transactions[0].TxIn[0].Sequence = 0
	})
	rejected(blockchain.ErrUnfinalizedTx)

	// ---------------------------------------------------------------------
	// Non-canonical variable-length integer tests.
	// ---------------------------------------------------------------------

	// Create a max size block with the variable-length integer for the
	// number of transactions replaced with a larger non-canonical version
	// that causes the block size to exceed the max allowed size.  Then,
	// create another block that is identical except with the canonical
	// encoding and ensure it is accepted.  The intent is to verify the
	// implementation does not reject the second block, which will have the
	// same hash, due to the first one already being rejected.
	//
	//   ... -> bmt2(15) -> bvl2(16)
	//                  \-> bvl1(16)
	g.SetTip("bmt2")
	bvl1 := g.NextBlock("bvl1", outs[16], ticketOuts[16],
		func(b *wire.MsgBlock) {
			curScriptLen := len(b.Transactions[1].TxOut[0].PkScript)
			bytesToMaxSize := maxBlockSize - b.SerializeSize() +
				(curScriptLen - 4)
			sizePadScript := repeatOpcode(0x00, bytesToMaxSize)
			replaceSpendScript(sizePadScript)(b)
		})
	assertTipNonCanonicalBlockSize(&g, maxBlockSize+8)
	rejectedNonCanonical()

	g.SetTip("bmt2")
	bvl2 := g.NextBlock("bvl2", outs[16], ticketOuts[16],
		func(b *wire.MsgBlock) {
			*b = cloneBlock(bvl1)
		})
	// Since the two blocks have the same hash and the generator state now
	// has bvl1 associated with the hash, manually remove bvl1, replace it
	// with bvl2, and then reset the tip to it.
	g.UpdateBlockState("bvl1", bvl1.BlockHash(), "bvl2", bvl2)
	g.SetTip("bvl2")
	g.AssertTipBlockHash(bvl1.BlockHash())
	g.AssertTipBlockSize(maxBlockSize)
	accepted()

	// ---------------------------------------------------------------------
	// Same block transaction spend tests.
	// ---------------------------------------------------------------------

	// Create block that spends an output created earlier in the same block.
	//
	//   ... -> bvl2(16) -> bts1(17)
	g.SetTip("bvl2")
	g.NextBlock("bts1", outs[17], ticketOuts[17], func(b *wire.MsgBlock) {
		spendTx3 := chaingen.MakeSpendableOut(b, 1, 0)
		tx3 := g.CreateSpendTx(&spendTx3, lowFee)
		b.AddTransaction(tx3)
	})
	accepted()

	// Create block that spends an output created later in the same block.
	//
	//   ... -> bts1(17)
	//                  \-> bts2(18)
	g.NextBlock("bts2", nil, ticketOuts[18], func(b *wire.MsgBlock) {
		tx2 := g.CreateSpendTx(outs[18], lowFee)
		tx3 := g.CreateSpendTxForTx(tx2, b.Header.Height, 2, lowFee)
		b.AddTransaction(tx3)
		b.AddTransaction(tx2)
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block that double spends a transaction created in the same
	// block.
	//
	//   ... -> bts1(17)
	//                  \-> bts3(18)
	g.SetTip("bts1")
	g.NextBlock("bts3", outs[18], ticketOuts[18], func(b *wire.MsgBlock) {
		tx2 := b.Transactions[1]
		tx3 := g.CreateSpendTxForTx(tx2, b.Header.Height, 1, lowFee)
		tx4 := g.CreateSpendTxForTx(tx2, b.Header.Height, 1, lowFee)
		b.AddTransaction(tx3)
		b.AddTransaction(tx4)
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block that spends the same output twice in stake tree.
	//
	//   ... -> bts1(17)
	//                  \-> bts4(18)
	g.SetTip("bts1")
	g.NextBlock("bts4", outs[18], ticketOuts[18], func(b *wire.MsgBlock) {
		b.STransactions[6].AddTxIn(b.STransactions[5].TxIn[0])
		b.STransactions[6].AddTxOut(b.STransactions[5].TxOut[1])
		b.STransactions[6].AddTxOut(b.STransactions[5].TxOut[2])
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block that spends an output in regular tree that is also spent
	// in stake tree.
	//
	//   ... -> bts1(17)
	//                  \-> bts5(18)
	g.SetTip("bts1")
	g.NextBlock("bts5", outs[18], ticketOuts[18], func(b *wire.MsgBlock) {
		spend := chaingen.MakeSpendableOut(b, 1, 0)
		tx := g.CreateSpendTx(&spend, lowFee)
		tx.AddTxIn(b.STransactions[5].TxIn[0])
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block that attempts to produce a stake output in a regular
	// transaction.
	//
	//   ... -> bts1(17)
	//                  \-> bts6(18)
	g.SetTip("bts1")
	g.NextBlock("bts6", outs[18], ticketOuts[18], func(b *wire.MsgBlock) {
		spend := chaingen.MakeSpendableOut(b, 1, 0)
		tx := g.CreateSpendTx(&spend, lowFee)
		tx.AddTxOut(b.STransactions[5].TxOut[0])
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrRegTxCreateStakeOut)

	// ---------------------------------------------------------------------
	// Extra subsidy tests.
	// ---------------------------------------------------------------------

	// Create block that pays 10 extra to the coinbase and a tx that only
	// pays 9 fee.
	//
	//   ... -> bts1(17)
	//                  \-> bsb1(18)
	g.SetTip("bts1")
	g.NextBlock("bsb1", outs[18], ticketOuts[18], additionalCoinbasePoW(10),
		additionalSpendFee(9))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create block that pays 10 extra to the coinbase and a tx that pays
	// the extra 10 fee.
	//
	//   ... -> bts1(17) -> bsb2(18)
	g.SetTip("bts1")
	g.NextBlock("bsb2", outs[18], ticketOuts[18], additionalCoinbasePoW(10),
		additionalSpendFee(10))
	accepted()

	// ---------------------------------------------------------------------
	// Malformed coinbase tests.
	// ---------------------------------------------------------------------

	// Create block with no proof-of-work subsidy output in the coinbase.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb1(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb1", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut = b.Transactions[0].TxOut[0:1]
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with an invalid script type in the coinbase block
	// commitment output.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb2(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb2", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut[1].PkScript = nil
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with too few bytes for the coinbase height commitment.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb2(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb3", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		script := opReturnScript(repeatOpcode(0x00, 3))
		b.Transactions[0].TxOut[1].PkScript = script
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with invalid block height in the coinbase commitment.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb4(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb4", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		script := standardCoinbaseOpReturnScript(b.Header.Height - 1)
		b.Transactions[0].TxOut[1].PkScript = script
	})
	rejected(blockchain.ErrCoinbaseHeight)

	// Create block with a fraudulent transaction (invalid index).
	//
	//   ... -> bsb2(18)
	//                  \-> bcb5(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb5", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[0].TxIn[0].BlockIndex = wire.NullBlockIndex - 1
	})
	rejected(blockchain.ErrBadCoinbaseFraudProof)

	// Create block with a fraudulent coinbase transaction (invalid height).
	//
	//   ... -> bsb2(14)
	//                  \-> bcb5a(15)
	g.SetTip("bsb2")
	g.NextBlock("bcb5a", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[0].TxIn[0].BlockHeight++
	})
	rejected(blockchain.ErrBadCoinbaseFraudProof)

	// Create block containing a transaction with no inputs.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb6(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb6", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[1].TxIn = nil
	})
	rejected(blockchain.ErrNoTxInputs)

	// Create block containing a transaction with no outputs.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb7(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb7", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut = nil
	})
	rejected(blockchain.ErrNoTxOutputs)

	// Create block containing a transaction output with negative value.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb8(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb8", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = -1
	})
	rejected(blockchain.ErrBadTxOutValue)

	// Create block containing a transaction output with an exceedingly
	// large (and therefore invalid) value.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb9(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb9", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = dcrutil.MaxAmount + 1
	})
	rejected(blockchain.ErrBadTxOutValue)

	// Create block containing a transaction whose outputs have an
	// exceedingly large (and therefore invalid) total value.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb10(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb10", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = dcrutil.MaxAmount
		b.Transactions[1].TxOut[1].Value = 1
	})
	rejected(blockchain.ErrBadTxOutValue)

	// Create block containing a stakebase tx with a small signature script.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb11(19)
	g.SetTip("bsb2")
	tooSmallSbScript := repeatOpcode(0x00, minCoinbaseScriptLen-1)
	g.NextBlock("bcb11", outs[19], ticketOuts[19],
		replaceStakeSigScript(tooSmallSbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Create block containing a base stake tx with a large signature script.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb12(19)
	g.SetTip("bsb2")
	tooLargeSbScript := repeatOpcode(0x00, maxCoinbaseScriptLen+1)
	g.NextBlock("bcb12", outs[19], ticketOuts[19],
		replaceStakeSigScript(tooLargeSbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Create block containing an input transaction with a null outpoint.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb13(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb13", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		tx := b.Transactions[1]
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
				wire.MaxPrevOutIndex, wire.TxTreeRegular)})
	})
	rejected(blockchain.ErrBadTxInput)

	// Create block containing duplicate tx inputs.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb14(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb14", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		tx := b.Transactions[1]
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: b.Transactions[1].TxIn[0].PreviousOutPoint})
	})
	rejected(blockchain.ErrDuplicateTxInputs)

	// Create block with nanosecond precision timestamp.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb15(19)
	g.SetTip("bsb2")
	g.NextBlock("bcb15", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Header.Timestamp = b.Header.Timestamp.Add(1 * time.Nanosecond)
	})
	rejected(blockchain.ErrInvalidTime)

	// Create block with target difficulty that is too low (0 or below).
	//
	//   ... -> bsb2(18)
	//                  \-> bcb16(19)
	g.SetTip("bsb2")
	bcb16 := g.NextBlock("bcb16", outs[19], ticketOuts[19])
	{
		// This can't be done inside a munge function passed to NextBlock
		// because the block is solved after the function returns and this test
		// involves an unsolvable block.
		bcb16Hash := bcb16.BlockHash()
		bcb16.Header.Bits = 0x01810000 // -1 in compact form.
		g.UpdateBlockState("bcb16", bcb16Hash, "bcb16", bcb16)
	}
	rejected(blockchain.ErrUnexpectedDifficulty)

	// Create block with target difficulty that is greater than max allowed.
	//
	//   ... -> bsb2(18)
	//                  \-> bcb17(19)
	g.SetTip("bsb2")
	bcb17 := g.NextBlock("bcb17", outs[19], ticketOuts[19])
	{
		// This can't be done inside a munge function passed to NextBlock
		// because the block is solved after the function returns and this test
		// involves an improperly solved block.
		bcb17Hash := bcb17.BlockHash()
		bcb17.Header.Bits = g.Params().PowLimitBits + 1
		g.UpdateBlockState("bcb17", bcb17Hash, "bcb17", bcb17)
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
	//   ... -> bsb2(18)
	//                  \-> bsc1(19)
	g.SetTip("bsb2")
	scriptSize := maxBlockSigOps + 5 + (maxScriptElementSize + 1) + 1
	tooManySigOps = repeatOpcode(txscript.OP_CHECKSIG, scriptSize)
	tooManySigOps[maxBlockSigOps] = txscript.OP_PUSHDATA4
	binary.LittleEndian.PutUint32(tooManySigOps[maxBlockSigOps+1:],
		maxScriptElementSize+1)
	g.NextBlock("bsc1", outs[19], ticketOuts[19],
		replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// Create block with more than max allowed signature operations such
	// that the signature operation that pushes it over the limit is before
	// an invalid push data that claims a large amount of data even though
	// that much data is not provided.
	//
	//   ... -> bsb2(18)
	//                  \-> bsc2(19)
	g.SetTip("bsb2")
	scriptSize = maxBlockSigOps + 5 + maxScriptElementSize + 1
	tooManySigOps = repeatOpcode(txscript.OP_CHECKSIG, scriptSize)
	tooManySigOps[maxBlockSigOps+1] = txscript.OP_PUSHDATA4
	binary.LittleEndian.PutUint32(tooManySigOps[maxBlockSigOps+2:], 0xffffffff)
	g.NextBlock("bsc2", outs[19], ticketOuts[19],
		replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// ---------------------------------------------------------------------
	// Dead execution path tests.
	// ---------------------------------------------------------------------

	// Create block with an invalid opcode in a dead execution path.
	//
	//   ... -> bsb2(18) -> bde1(19)
	script := []byte{txscript.OP_IF, txscript.OP_INVALIDOPCODE,
		txscript.OP_ELSE, txscript.OP_TRUE, txscript.OP_ENDIF}
	g.SetTip("bsb2")
	g.NextBlock("bde1", outs[19], ticketOuts[19], replaceSpendScript(script),
		func(b *wire.MsgBlock) {
			spendTx2 := chaingen.MakeSpendableOut(b, 1, 0)
			tx3 := g.CreateSpendTx(&spendTx2, lowFee)
			tx3.TxIn[0].SignatureScript = []byte{txscript.OP_FALSE}
			b.AddTransaction(tx3)
		})
	accepted()

	// ---------------------------------------------------------------------
	// Various OP_RETURN tests.
	// ---------------------------------------------------------------------

	// Create a block that has multiple transactions each with a single
	// OP_RETURN output.
	//
	//   ... -> bde1(19) -> bor1(20)
	g.NextBlock("bor1", outs[20], ticketOuts[20], func(b *wire.MsgBlock) {
		// Add 4 outputs to the spending transaction that are spent
		// below.
		const numAdditionalOutputs = 4
		spendTx := b.Transactions[1]
		spendTx.TxOut[0].Value -= int64(lowFee) * numAdditionalOutputs
		for i := 0; i < numAdditionalOutputs; i++ {
			spendTx.AddTxOut(wire.NewTxOut(int64(lowFee), opTrueScript))
		}

		// Add transactions spending from the outputs added above that
		// each contain an OP_RETURN output.
		//
		// NOTE: The CreateSpendTx func adds the OP_RETURN output.
		zeroFee := dcrutil.Amount(0)
		for i := uint32(0); i < numAdditionalOutputs; i++ {
			spend := chaingen.MakeSpendableOut(b, 1, i+2)
			tx := g.CreateSpendTx(&spend, zeroFee)
			tx.TxIn[0].SignatureScript = nil
			b.AddTransaction(tx)
		}
	})
	g.AssertTipBlockNumTxns(6)
	g.AssertTipBlockTxOutOpReturn(5, 1)
	bor1OpReturnOut := chaingen.MakeSpendableOut(g.Tip(), 5, 1)
	accepted()

	// Reorg to a side chain that does not contain the OP_RETURNs.
	//
	//   ... -> bde1(19) -> bor1(20)
	//                  \-> bor2(20) -> bor3(21)
	g.SetTip("bde1")
	g.NextBlock("bor2", outs[20], ticketOuts[20])
	acceptedToSideChainWithExpectedTip("bor1")

	g.NextBlock("bor3", outs[21], ticketOuts[21])
	accepted()

	// Reorg back to the original chain that contains the OP_RETURNs.
	//
	//   ... -> bde1(19) -> bor1(20) -> bor4(21) -> bor5(22)
	//                  \-> bor2(20) -> bor3(21)
	g.SetTip("bor1")
	g.NextBlock("bor4", outs[21], ticketOuts[21])
	acceptedToSideChainWithExpectedTip("bor3")

	g.NextBlock("bor5", outs[22], ticketOuts[22])
	accepted()

	// Create a block that spends an OP_RETURN.
	//
	//   ... -> bde1(19) -> bor1(20) -> bor4(21) -> bor5(22)
	//                  \-> bor2(20) -> bor3(21)           \-> bor6(bor1.tx[5].out[1])
	g.NextBlock("bor6", nil, ticketOuts[23], func(b *wire.MsgBlock) {
		// An OP_RETURN output doesn't have any value so use a fee of 0.
		zeroFee := dcrutil.Amount(0)
		tx := g.CreateSpendTx(&bor1OpReturnOut, zeroFee)
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create a block that has a transaction with multiple OP_RETURNs.  Even
	// though a transaction with a large number of OP_RETURNS is not
	// considered a standard transaction, it is still valid by the consensus
	// rules.
	//
	//   ... -> bor5(22) -> bor7(23)
	//
	g.SetTip("bor5")
	g.NextBlock("bor7", outs[23], ticketOuts[23], func(b *wire.MsgBlock) {
		const numAdditionalOutputs = 8
		const zeroCoin = int64(0)
		spendTx := b.Transactions[1]
		for i := 0; i < numAdditionalOutputs; i++ {
			opRetScript := chaingen.UniqueOpReturnScript()
			spendTx.AddTxOut(wire.NewTxOut(zeroCoin, opRetScript))
		}
	})
	for i := uint32(2); i < 10; i++ {
		g.AssertTipBlockTxOutOpReturn(1, i)
	}
	accepted()

	// ---------------------------------------------------------------------
	// Revocation tests.
	// ---------------------------------------------------------------------

	// Create valid block that misses a vote.
	//
	//   ... -> bor7(23) -> brt1(24)
	g.NextBlock("brt1", outs[24], ticketOuts[24], g.ReplaceWithNVotes(4))
	accepted()

	// Create block that contains a revocation due to previous missed vote and
	// a header that commits to more revocations than the block actually
	// contains.
	//
	//   ... -> brt1(24)
	//                  \-> brt2(25)
	g.NextBlock("brt2", outs[25], ticketOuts[25], func(b *wire.MsgBlock) {
		b.Header.Revocations++
	})
	g.AssertTipNumRevocations(2)
	rejected(blockchain.ErrRevocationsMismatch)

	// Create block that has a revocation with more payees than expected.
	//   ... -> brt1(24)
	//                  \-> brt3(25)
	g.SetTip("brt1")
	g.NextBlock("brt3", outs[25], ticketOuts[25], func(b *wire.MsgBlock) {
		g.AssertBlockRevocationTx(b, 10)
		b.STransactions[10].TxOut = append(b.STransactions[10].TxOut,
			b.STransactions[10].TxOut[0])
	})
	g.AssertTipNumRevocations(1)
	rejected(blockchain.ErrSSRtxPayeesMismatch)

	// Create block that has a revocation paying more than the original
	// amount to the committed address.
	//   ... -> brt1(24)
	//                  \-> brt4(25)
	g.SetTip("brt1")
	g.NextBlock("brt4", outs[25], ticketOuts[25], func(b *wire.MsgBlock) {
		g.AssertBlockRevocationTx(b, 10)
		b.STransactions[10].TxOut[0].Value++
	})
	g.AssertTipNumRevocations(1)
	rejected(blockchain.ErrSSRtxPayees)

	// Create block that has a revocation using a corrupted pay-to-address
	// script.
	//   ... -> brt1(24)
	//                  \-> brt5(25)
	g.SetTip("brt1")
	g.NextBlock("brt5", outs[25], ticketOuts[25], func(b *wire.MsgBlock) {
		g.AssertBlockRevocationTx(b, 10)
		b.STransactions[10].TxOut[0].PkScript[8] ^= 0x55
	})
	g.AssertTipNumRevocations(1)
	rejected(blockchain.ErrSSRtxPayees)

	// Create block that has a revocation for a voted ticket.
	//
	//   ... -> brt1(24)
	//                  \-> brt6(25)
	g.SetTip("brt1")
	g.NextBlock("brt6", outs[25], ticketOuts[25], func(b *wire.MsgBlock) {
		// Loop backwards to get the ticket transaction associated with the
		// first vote in the block.
		voteTx := b.STransactions[0]
		ticketTxIn := voteTx.TxIn[1]
		ticketBlk := g.BlockByHash(&b.Header.PrevBlock)
		for ticketBlk.Header.Height != ticketTxIn.BlockHeight {
			ticketBlk = g.BlockByHash(&ticketBlk.Header.PrevBlock)
		}
		ticketTx := ticketBlk.STransactions[ticketTxIn.BlockIndex]

		// Create new revocation for the same ticket.
		revocation := g.CreateRevocationTx(ticketTx, ticketTxIn.BlockHeight,
			ticketTxIn.BlockIndex)
		b.AddSTransaction(revocation)
		b.Header.Revocations++
	})
	g.AssertTipNumRevocations(2)
	rejected(blockchain.ErrInvalidSSRtx)

	// Create block that contains a revocation due to previous missed vote.
	//
	//   ... -> brt1(24) -> brtfinal(25)
	g.SetTip("brt1")
	g.NextBlock("brtfinal", outs[25], ticketOuts[25])
	g.AssertTipNumRevocations(1)
	accepted()

	// ---------------------------------------------------------------------
	// Large block re-org test.
	// ---------------------------------------------------------------------

	if !includeLargeReorg {
		return tests, nil
	}

	// Ensure the tip the re-org test builds on is the best chain tip.
	//
	//   ... -> brtfinal(25) -> ...
	g.SetTip("brtfinal")
	spendableOutOffset := int32(26) // Next spendable offset.

	// Collect all of the spendable coinbase outputs from the previous
	// collection point up to the current tip.
	g.SaveSpendableCoinbaseOuts()
	for g.NumSpendableCoinbaseOuts() > 0 {
		coinbaseOuts := g.OldestCoinbaseOuts()
		outs = append(outs, &coinbaseOuts[0])
		ticketOuts = append(ticketOuts, coinbaseOuts[1:])
	}

	// Extend the main chain by a large number of max size blocks.
	//
	//   ... -> br0 -> br1 -> ... -> br#
	testInstances = nil
	reorgSpend := *outs[spendableOutOffset]
	reorgTicketSpends := ticketOuts[spendableOutOffset]
	reorgStartBlockName := g.TipName()
	chain1TipName := g.TipName()
	for i := int32(0); i < numLargeReorgBlocks; i++ {
		chain1TipName = fmt.Sprintf("br%d", i)
		g.NextBlock(chain1TipName, &reorgSpend, reorgTicketSpends,
			func(b *wire.MsgBlock) {
				curScriptLen := len(b.Transactions[1].TxOut[0].PkScript)
				bytesToMaxSize := maxBlockSize - b.SerializeSize() +
					(curScriptLen - 4)
				sizePadScript := repeatOpcode(0x00, bytesToMaxSize)
				replaceSpendScript(sizePadScript)(b)
			})
		g.AssertTipBlockSize(maxBlockSize)
		g.SaveTipCoinbaseOuts()
		testInstances = append(testInstances, acceptBlock(g.TipName(),
			g.Tip(), true, false))

		// Use the next available spendable output.  First use up any
		// remaining spendable outputs that were already popped into the
		// outs slice, then just pop them from the stack.
		if spendableOutOffset+1+i < int32(len(outs)) {
			reorgSpend = *outs[spendableOutOffset+1+i]
			reorgTicketSpends = ticketOuts[spendableOutOffset+1+i]
		} else {
			reorgOuts := g.OldestCoinbaseOuts()
			reorgSpend = reorgOuts[0]
			reorgTicketSpends = reorgOuts[1:]
		}
	}
	tests = append(tests, testInstances)

	// Ensure any remaining unused spendable outs from the main chain that
	// are still associated with the generator are removed to avoid them
	// from being used on the side chain build below.
	for g.NumSpendableCoinbaseOuts() > 0 {
		g.OldestCoinbaseOuts()
	}

	// Create a side chain that has the same length.
	//
	//   ... -> br0    -> ... -> br#
	//      \-> bralt0 -> ... -> bralt#
	g.SetTip(reorgStartBlockName)
	testInstances = nil
	reorgSpend = *outs[spendableOutOffset]
	reorgTicketSpends = ticketOuts[spendableOutOffset]
	var chain2TipName string
	for i := int32(0); i < numLargeReorgBlocks; i++ {
		chain2TipName = fmt.Sprintf("bralt%d", i)
		g.NextBlock(chain2TipName, &reorgSpend, reorgTicketSpends)
		g.SaveTipCoinbaseOuts()
		testInstances = append(testInstances, acceptBlock(g.TipName(),
			g.Tip(), false, false))

		// Use the next available spendable output.  First use up any
		// remaining spendable outputs that were already popped into the
		// outs slice, then just pop them from the stack.
		if spendableOutOffset+1+i < int32(len(outs)) {
			reorgSpend = *outs[spendableOutOffset+1+i]
			reorgTicketSpends = ticketOuts[spendableOutOffset+1+i]
		} else {
			reorgOuts := g.OldestCoinbaseOuts()
			reorgSpend = reorgOuts[0]
			reorgTicketSpends = reorgOuts[1:]
		}
	}
	testInstances = append(testInstances, expectTipBlock(chain1TipName,
		g.BlockByName(chain1TipName)))
	tests = append(tests, testInstances)

	// Extend the side chain by one to force the large reorg.
	//
	//   ... -> bralt0 -> ... -> bralt# -> bralt#+1
	//      \-> br0    -> ... -> br#
	g.NextBlock(fmt.Sprintf("bralt%d", numLargeReorgBlocks), nil, nil)
	chain2TipName = g.TipName()
	accepted()

	// Extend the first chain by two to force a large reorg back to it.
	//
	//   ... -> br0    -> ... -> br#    -> br#+1    -> br#+2
	//      \-> bralt0 -> ... -> bralt# -> bralt#+1
	g.SetTip(chain1TipName)
	g.NextBlock(fmt.Sprintf("br%d", numLargeReorgBlocks), nil, nil)
	acceptedToSideChainWithExpectedTip(chain2TipName)

	g.NextBlock(fmt.Sprintf("br%d", numLargeReorgBlocks+1), nil, nil)
	accepted()

	return tests, nil
}
