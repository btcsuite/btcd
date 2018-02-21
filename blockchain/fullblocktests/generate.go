// Copyright (c) 2016 The btcsuite developers
// Copyright (c) 2016-2017 The Decred developers
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
	"github.com/decred/dcrd/chaincfg/chainec"
	"github.com/decred/dcrd/chaincfg/chainhash"
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
)

var (
	// opTrueScript is a simple public key script that contains the OP_TRUE
	// opcode.  It is defined here to reduce garbage creation.
	opTrueScript = []byte{txscript.OP_TRUE}

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
	// encodeNonCanonicalBlock function and expected it to be rejected.
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

	// Add the required premine block.
	//
	//   genesis -> bp
	g.SetTip("genesis")
	g.CreatePremineBlock("bp", 0)
	g.AssertTipHeight(1)
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
		g.NextBlock(blockName, nil, nil)
		g.SaveTipCoinbaseOuts()
		testInstances = append(testInstances, acceptBlock(g.TipName(),
			g.Tip(), true, false))
	}
	tests = append(tests, testInstances)
	g.AssertTipHeight(uint32(coinbaseMaturity) + 1)

	// ---------------------------------------------------------------------
	// Generate enough blocks to reach the stake enabled height while
	// creating ticket purchases that spend from the coinbases matured
	// above.  This will also populate the pool of immature tickets.
	//
	//   ... -> bm# ... -> bse0 -> bse1 -> ... -> bse#
	// ---------------------------------------------------------------------

	testInstances = nil
	var ticketsPurchased int
	for i := int64(0); int64(g.Tip().Header.Height) < stakeEnabledHeight; i++ {
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
	g.AssertTipHeight(uint32(stakeEnabledHeight))

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
	//   ... -> b1(0) -> b2(1)
	g.NextBlock("b1", outs[0], ticketOuts[0])
	accepted()

	g.NextBlock("b2", outs[1], ticketOuts[1])
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
	g.SetTip("b1")
	g.NextBlock("b3", outs[1], ticketOuts[1])
	b3Tx1Out := chaingen.MakeSpendableOut(g.Tip(), 1, 0)
	acceptedToSideChainWithExpectedTip("b2")

	// Extend b3 fork to make the alternative chain longer and force reorg.
	//
	//   ... -> b1(0) -> b2(1)
	//               \-> b3(1) -> b4(2)
	g.NextBlock("b4", outs[2], ticketOuts[2])
	accepted()

	// Extend b2 fork twice to make first chain longer and force reorg.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//               \-> b3(1) -> b4(2)
	g.SetTip("b2")
	g.NextBlock("b5", outs[2], ticketOuts[2])
	acceptedToSideChainWithExpectedTip("b4")

	g.NextBlock("b6", outs[3], ticketOuts[3])
	accepted()

	// ---------------------------------------------------------------------
	// Double spend tests.
	// ---------------------------------------------------------------------

	// Create a fork that double spends.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//                                 \-> b7(2) -> b8(4)
	//               \-> b3(1) -> b4(2)
	g.SetTip("b5")
	g.NextBlock("b7", outs[2], ticketOuts[3])
	acceptedToSideChainWithExpectedTip("b6")

	g.NextBlock("b8", outs[4], ticketOuts[4])
	rejected(blockchain.ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Too much proof-of-work coinbase tests.
	// ---------------------------------------------------------------------

	// Create a block that generates too much proof-of-work coinbase.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//                                         \-> b9(4)
	//               \-> b3(1) -> b4(2)
	g.SetTip("b6")
	g.NextBlock("b9", outs[4], ticketOuts[4], additionalCoinbasePoW(1))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create a fork that ends with block that generates too much
	// proof-of-work coinbase.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//                                 \-> b10(3) -> b11(4)
	//               \-> b3(1) -> b4(2)
	g.SetTip("b5")
	g.NextBlock("b10", outs[3], ticketOuts[3])
	acceptedToSideChainWithExpectedTip("b6")

	g.NextBlock("b11", outs[4], ticketOuts[4], additionalCoinbasePoW(1))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create a fork that ends with block that generates too much
	// proof-of-work coinbase as before, but with a valid fork first.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//              |                  \-> b12(3) -> b13(4) -> b14(5)
	//              |                      (b12 added last)
	//               \-> b3(1) -> b4(2)
	g.SetTip("b5")
	b12 := g.NextBlock("b12", outs[3], ticketOuts[3])
	b13 := g.NextBlock("b13", outs[4], ticketOuts[4])
	b14 := g.NextBlock("b14", outs[5], ticketOuts[5], additionalCoinbasePoW(1))
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
	g.SetTip("b13")
	g.NextBlock("bbadtaxscript", outs[5], ticketOuts[5], func(b *wire.MsgBlock) {
		taxOutput := b.Transactions[0].TxOut[0]
		_, addrs, _, _ := txscript.ExtractPkScriptAddrs(
			g.Params().OrganizationPkScriptVersion,
			taxOutput.PkScript, g.Params())
		p2shTaxAddr := addrs[0].(*dcrutil.AddressScriptHash)
		p2pkhTaxAddr, err := dcrutil.NewAddressPubKeyHash(
			p2shTaxAddr.Hash160()[:], g.Params(),
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
	g.SetTip("b13")
	g.NextBlock("bbadtaxscriptversion", outs[5], ticketOuts[5], func(b *wire.MsgBlock) {
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
	g.SetTip("b13")
	g.NextBlock("b15", outs[5], ticketOuts[5], additionalCoinbaseDev(1))
	rejected(blockchain.ErrNoTax)

	// Create a fork that ends with block that generates too much dev-org
	// coinbase.
	//
	//   ... -> b5(2) -> b12(3) -> b13(4)
	//   \                     \-> b16(4) -> b17(5)
	//    \-> b3(1) -> b4(2)
	g.SetTip("b12")
	g.NextBlock("b16", outs[4], ticketOuts[4], additionalCoinbaseDev(1))
	acceptedToSideChainWithExpectedTip("b13")

	g.NextBlock("b17", outs[5], ticketOuts[5], additionalCoinbaseDev(1))
	rejected(blockchain.ErrNoTax)

	// Create a fork that ends with block that generates too much dev-org
	// coinbase as before, but with a valid fork first.
	//
	//   ... -> b5(2) -> b12(3) -> b13(4)
	//   \                     \-> b18(4) -> b19(5) -> b20(6)
	//   |                         (b18 added last)
	//    \-> b3(1) -> b4(2)
	//
	g.SetTip("b12")
	b18 := g.NextBlock("b18", outs[4], ticketOuts[4])
	b19 := g.NextBlock("b19", outs[5], ticketOuts[5])
	b20 := g.NextBlock("b20", outs[6], ticketOuts[6], additionalCoinbaseDev(1))
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
	g.SetTip("b19")
	manySigOps := repeatOpcode(txscript.OP_CHECKSIG, maxBlockSigOps)
	g.NextBlock("b21", outs[6], ticketOuts[6], replaceSpendScript(manySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Attempt to add block with more than max allowed signature operations.
	//
	//   ... -> b5(2) -> b12(3) -> b18(4) -> b19(5) -> b21(6)
	//   \                                                   \-> b22(7)
	//    \-> b3(1) -> b4(2)
	tooManySigOps := repeatOpcode(txscript.OP_CHECKSIG, maxBlockSigOps+1)
	g.NextBlock("b22", outs[7], ticketOuts[7], replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// ---------------------------------------------------------------------
	// Cross-fork spend tests.
	// ---------------------------------------------------------------------

	// Create block that spends a tx created on a different fork.
	//
	//   ... -> b5(2) -> b12(3) -> b18(4) -> b19(5) -> b21(6)
	//   \                                                   \-> b23(b3.tx[1])
	//    \-> b3(1) -> b4(2)
	g.SetTip("b21")
	g.NextBlock("b23", &b3Tx1Out, nil)
	rejected(blockchain.ErrMissingTxOut)

	// Create block that forks and spends a tx created on a third fork.
	//
	//   ... -> b5(2) -> b12(3) -> b18(4) -> b19(5) -> b21(6)
	//   |                                         \-> b24(b3.tx[1]) -> b25(6)
	//    \-> b3(1) -> b4(2)
	g.SetTip("b19")
	g.NextBlock("b24", &b3Tx1Out, nil)
	acceptedToSideChainWithExpectedTip("b21")

	g.NextBlock("b25", outs[6], ticketOuts[6])
	rejected(blockchain.ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Immature coinbase tests.
	// ---------------------------------------------------------------------

	// Create block that spends immature coinbase.
	//
	//   ... -> b19(5) -> b21(6)
	//                          \-> b26(8)
	g.SetTip("b21")
	g.NextBlock("b26", outs[8], ticketOuts[7])
	rejected(blockchain.ErrImmatureSpend)

	// Create block that spends immature coinbase on a fork.
	//
	//   ... -> b19(5) -> b21(6)
	//                \-> b27(6) -> b28(8)
	g.SetTip("b19")
	g.NextBlock("b27", outs[6], ticketOuts[6])
	acceptedToSideChainWithExpectedTip("b21")

	g.NextBlock("b28", outs[8], ticketOuts[7])
	rejected(blockchain.ErrImmatureSpend)

	// ---------------------------------------------------------------------
	// Max block size tests.
	// ---------------------------------------------------------------------

	// Create block that is the max allowed size.
	//
	//   ... -> b21(6) -> b29(7)
	g.SetTip("b21")
	g.NextBlock("b29", outs[7], ticketOuts[7], func(b *wire.MsgBlock) {
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
	//   ... -> b21(6) -> b29(7)
	//                \-> b30(7) -> b31(8)
	g.SetTip("b21")
	g.NextBlock("b30", outs[7], ticketOuts[7], func(b *wire.MsgBlock) {
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
	g.NextBlock("b31", outs[8], ticketOuts[8])
	orphanedOrRejected()

	// ---------------------------------------------------------------------
	// Orphan tests.
	// ---------------------------------------------------------------------

	// Create valid orphan block with zero prev hash.
	//
	//   No previous block
	//                    \-> borphan0(7)
	g.SetTip("b21")
	g.NextBlock("borphan0", outs[7], ticketOuts[7], func(b *wire.MsgBlock) {
		b.Header.PrevBlock = chainhash.Hash{}
	})
	orphaned()

	// Create valid orphan block.
	//
	//   ... -> b21(6) -> b29(7)
	//                \-> borphanbase(7) -> borphan1(8)
	g.SetTip("b21")
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
	//   ... -> b21(6) -> b29(7)
	//                \-> b32(7) -> b33(8)
	g.SetTip("b21")
	tooSmallCbScript := repeatOpcode(0x00, minCoinbaseScriptLen-1)
	g.NextBlock("b32", outs[7], ticketOuts[7], replaceCoinbaseSigScript(tooSmallCbScript))
	rejected(blockchain.ErrBadCoinbaseScriptLen)

	// Parent was rejected, so this block must either be an orphan or
	// outright rejected due to an invalid parent.
	g.NextBlock("b33", outs[8], ticketOuts[8])
	orphanedOrRejected()

	// Create block that has a coinbase script that is larger than the
	// allowed length.  This is done on a fork and should be rejected
	// regardless.  Also, create a block that builds on the rejected block.
	//
	//   ... -> b21(6) -> b29(7)
	//                \-> b34(7) -> b35(8)
	g.SetTip("b21")
	tooLargeCbScript := repeatOpcode(0x00, maxCoinbaseScriptLen+1)
	g.NextBlock("b34", outs[7], ticketOuts[7], replaceCoinbaseSigScript(tooLargeCbScript))
	rejected(blockchain.ErrBadCoinbaseScriptLen)

	// Parent was rejected, so this block must either be an orphan or
	// outright rejected due to an invalid parent.
	g.NextBlock("b35", outs[8], ticketOuts[8])
	orphanedOrRejected()

	// Create block that has a max length coinbase script.
	//
	//   ... -> b29(7) -> b36(8)
	g.SetTip("b29")
	maxSizeCbScript := repeatOpcode(0x00, maxCoinbaseScriptLen)
	g.NextBlock("b36", outs[8], ticketOuts[8], replaceCoinbaseSigScript(maxSizeCbScript))
	accepted()

	// ---------------------------------------------------------------------
	// Vote tests.
	// ---------------------------------------------------------------------

	// Attempt to add block where vote has a null ticket reference hash.
	//
	//   ... -> b36(8)
	//                \-> bv1(9)
	g.SetTip("b36")
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
	//   ... -> b36(8)
	//                \-> bv2(9)
	g.SetTip("b36")
	g.NextBlock("bv2", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.STransactions[0] = b.Transactions[0]
	})
	rejected(blockchain.ErrRegTxInStakeTree)

	// Attempt to add block with too many votes.
	//
	//   ... -> b36(8)
	//                \-> bv3(9)
	g.SetTip("b36")
	g.NextBlock("bv3", outs[9], ticketOuts[9], g.ReplaceWithNVotes(ticketsPerBlock+1))
	rejected(blockchain.ErrTooManyVotes)

	// Attempt to add block with too few votes.
	//
	//   ... -> b36(8)
	//                \-> bv4(9)
	g.SetTip("b36")
	g.NextBlock("bv4", outs[9], ticketOuts[9], g.ReplaceWithNVotes(ticketsPerBlock/2))
	rejected(blockchain.ErrNotEnoughVotes)

	// Attempt to add block with different number of votes in stake tree and
	// header.
	//
	//   ... -> b36(8)
	//                \-> bv5(9)
	g.SetTip("b36")
	g.NextBlock("bv5", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.Header.FreshStake -= 1
	})
	rejected(blockchain.ErrFreshStakeMismatch)

	// ---------------------------------------------------------------------
	// Stake ticket difficulty tests.
	// ---------------------------------------------------------------------

	// Create block with ticket purchase below required ticket price.
	//
	//   ... -> b36(8)
	//                \-> bsd0(9)
	g.SetTip("b36")
	g.NextBlock("bsd0", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
		b.STransactions[5].TxOut[0].Value -= 1
	})
	rejected(blockchain.ErrNotEnoughStake)

	// Create block with stake transaction below pos limit.
	//
	//   ... -> b36(8)
	//                \-> bsd1(9)
	g.SetTip("b36")
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
	//   ... -> b36(8)
	//                \-> bss0(9)
	g.SetTip("b36")
	tooSmallCbScript = repeatOpcode(0x00, minCoinbaseScriptLen-1)
	g.NextBlock("bss0", outs[9], ticketOuts[9], replaceStakeSigScript(tooSmallCbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Create block that has a stakebase script that is larger than the
	// maximum allowed length.
	//
	//   ... -> b36(8)
	//                \-> bss1(9)
	g.SetTip("b36")
	tooLargeCbScript = repeatOpcode(0x00, maxCoinbaseScriptLen+1)
	g.NextBlock("bss1", outs[9], ticketOuts[9], replaceStakeSigScript(tooLargeCbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Add a block with a stake transaction with a signature script that is
	// not the required script, but is otherwise a valid script.
	//
	//   ... -> b36(8)
	//                \-> bss2(9)
	g.SetTip("b36")
	badScript := append(g.Params().StakeBaseSigScript, 0x00)
	g.NextBlock("bss2", outs[9], ticketOuts[9], replaceStakeSigScript(badScript))
	rejected(blockchain.ErrBadStakebaseScrVal)

	// ---------------------------------------------------------------------
	// Multisig[Verify]/ChecksigVerifiy signature operation count tests.
	// ---------------------------------------------------------------------

	// Create block with max signature operations as OP_CHECKMULTISIG.
	//
	//   ... -> b36(8) -> b37(9)
	//
	// OP_CHECKMULTISIG counts for 20 sigops.
	g.SetTip("b36")
	manySigOps = repeatOpcode(txscript.OP_CHECKMULTISIG, maxBlockSigOps/20)
	g.NextBlock("b37", outs[9], ticketOuts[9], replaceSpendScript(manySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with more than max allowed signature operations using
	// OP_CHECKMULTISIG.
	//
	//   ... -> b37(9)
	//                \-> b38(10)
	//
	// OP_CHECKMULTISIG counts for 20 sigops.
	tooManySigOps = repeatOpcode(txscript.OP_CHECKMULTISIG, maxBlockSigOps/20)
	tooManySigOps = append(tooManySigOps, txscript.OP_CHECKSIG)
	g.NextBlock("b38", outs[10], ticketOuts[10], replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// Create block with max signature operations as OP_CHECKMULTISIGVERIFY.
	//
	//   ... -> b37(9) -> b39(10)
	//
	g.SetTip("b37")
	manySigOps = repeatOpcode(txscript.OP_CHECKMULTISIGVERIFY, maxBlockSigOps/20)
	g.NextBlock("b39", outs[10], ticketOuts[10], replaceSpendScript(manySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with more than max allowed signature operations using
	// OP_CHECKMULTISIGVERIFY.
	//
	//   ... -> b39(10)
	//                \-> b40(11)
	//
	tooManySigOps = repeatOpcode(txscript.OP_CHECKMULTISIGVERIFY, maxBlockSigOps/20)
	tooManySigOps = append(tooManySigOps, txscript.OP_CHECKSIG)
	g.NextBlock("b40", outs[11], ticketOuts[11], replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// Create block with max signature operations as OP_CHECKSIGVERIFY.
	//
	//   ... -> b39(10) -> b41(11)
	//
	g.SetTip("b39")
	manySigOps = repeatOpcode(txscript.OP_CHECKSIGVERIFY, maxBlockSigOps)
	g.NextBlock("b41", outs[11], ticketOuts[11], replaceSpendScript(manySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with more than max allowed signature operations using
	// OP_CHECKSIGVERIFY.
	//
	//   ... -> b41(11)
	//                 \-> b42(12)
	//
	tooManySigOps = repeatOpcode(txscript.OP_CHECKSIGVERIFY, maxBlockSigOps+1)
	g.NextBlock("b42", outs[12], ticketOuts[12], replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
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
	g.SetTip("b41")
	doubleSpendTx := g.CreateSpendTx(outs[12], lowFee)
	g.NextBlock("b42", outs[12], ticketOuts[12], additionalPoWTx(doubleSpendTx))
	b42Tx1Out := chaingen.MakeSpendableOut(g.Tip(), 1, 0)
	rejected(blockchain.ErrMissingTxOut)

	g.SetTip("b41")
	g.NextBlock("b43", &b42Tx1Out, ticketOuts[12])
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
	//   ... -> b41(11) -> bshso0 (12)
	g.SetTip("b41")
	bshso0 := g.NextBlock("bshso0", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
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
	//   ... -> b41(11) -> bshso0(12)
	//                               \-> bshso1(13)
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
	//   ... -> b41(11) -> bshso0(12) -> bshso2(13)
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
	//   ... -> b41(11) -> b44(12)    -> b45(13)    -> b46(14)
	//                 \-> bshso0(12) -> bshso2(13)
	// ---------------------------------------------------------------------

	g.SetTip("b41")
	g.NextBlock("b44", outs[12], ticketOuts[12])
	acceptedToSideChainWithExpectedTip("bshso2")

	g.NextBlock("b45", outs[13], ticketOuts[13])
	acceptedToSideChainWithExpectedTip("bshso2")

	g.NextBlock("b46", outs[14], ticketOuts[14])
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
	//   ... -> b46(14)
	//                 \-> b47(15)
	g.NextBlock("b47", nil, ticketOuts[15], func(b *wire.MsgBlock) {
		nonCoinbaseTx := g.CreateSpendTx(outs[15], lowFee)
		b.Transactions[0] = nonCoinbaseTx
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with no transactions.
	//
	//   ... -> b46(14)
	//                 \-> b48(_)
	g.SetTip("b46")
	g.NextBlock("b48", nil, nil, func(b *wire.MsgBlock) {
		b.Transactions = nil
	})
	rejected(blockchain.ErrNoTransactions)

	// Create block with invalid proof of work.
	//
	//   ... -> b46(14)
	//                 \-> b49(15)
	g.SetTip("b46")
	b49 := g.NextBlock("b49", outs[15], ticketOuts[15])
	// This can't be done inside a munge function passed to NextBlock
	// because the block is solved after the function returns and this test
	// requires an unsolved block.
	{
		origHash := b49.BlockHash()
		for {
			// Keep incrementing the nonce until the hash treated as
			// a uint256 is higher than the limit.
			b49.Header.Nonce += 1
			hash := b49.BlockHash()
			hashNum := blockchain.HashToBig(&hash)
			if hashNum.Cmp(g.Params().PowLimit) >= 0 {
				break
			}
		}
		g.UpdateBlockState("b49", origHash, "b49", b49)
	}
	rejected(blockchain.ErrHighHash)

	// Create block with a timestamp too far in the future.
	//
	//   ... -> b46(14)
	//                 \-> b50(15)
	g.SetTip("b46")
	g.NextBlock("b50", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		// 3 hours in the future clamped to 1 second precision.
		nowPlus3Hours := time.Now().Add(time.Hour * 3)
		b.Header.Timestamp = time.Unix(nowPlus3Hours.Unix(), 0)
	})
	rejected(blockchain.ErrTimeTooNew)

	// Create block with an invalid merkle root.
	//
	//   ... -> b46(14)
	//                 \-> b51(15)
	g.SetTip("b46")
	g.NextBlock("b51", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Header.MerkleRoot = chainhash.Hash{}
	})
	g.AssertTipBlockMerkleRoot(chainhash.Hash{})
	rejected(blockchain.ErrBadMerkleRoot)

	// Create block with an invalid proof-of-work limit.
	//
	//   ... -> b46(14)
	//                 \-> b52(15)
	g.SetTip("b46")
	g.NextBlock("b52", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Header.Bits -= 1
	})
	rejected(blockchain.ErrUnexpectedDifficulty)

	// Create block with an invalid negative proof-of-work limit.
	//
	//   ... -> b46(14)
	//                 \-> b52a(15)
	g.SetTip("b46")
	b52a := g.NextBlock("b52a", outs[15], ticketOuts[15])
	// This can't be done inside a munge function passed to nextBlock
	// because the block is solved after the function returns and this test
	// involves an unsolvable block.
	{
		origHash := b52a.BlockHash()
		b52a.Header.Bits = 0x01810000 // -1 in compact form.
		g.UpdateBlockState("b52a", origHash, "b52a", b52a)
	}
	rejected(blockchain.ErrUnexpectedDifficulty)

	// Create block with two coinbase transactions.
	//
	//   ... -> b46(14)
	//                 \-> b53(15)
	g.SetTip("b46")
	coinbaseTx := g.CreateCoinbaseTx(g.Tip().Header.Height+1, ticketsPerBlock)
	g.NextBlock("b53", outs[15], ticketOuts[15], additionalPoWTx(coinbaseTx))
	rejected(blockchain.ErrMultipleCoinbases)

	// Create block with duplicate transactions in the regular transaction
	// tree.
	//
	// This test relies on the shape of the shape of the merkle tree to test
	// the intended condition.  That is the reason for the assertion.
	//
	//   ... -> b46(14)
	//                 \-> b54(15)
	g.SetTip("b46")
	g.NextBlock("b54", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.AddTransaction(b.Transactions[1])
	})
	g.AssertTipBlockNumTxns(3)
	rejected(blockchain.ErrDuplicateTx)

	// Create a block that spends a transaction that does not exist.
	//
	//   ... -> b46(14)
	//                 \-> b55(15)
	g.SetTip("b46")
	g.NextBlock("b55", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		hash := newHashFromStr("00000000000000000000000000000000" +
			"00000000000000000123456789abcdef")
		b.Transactions[1].TxIn[0].PreviousOutPoint.Hash = *hash
		b.Transactions[1].TxIn[0].PreviousOutPoint.Index = 0
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block with stake tx in regular tx tree.
	//
	//   ... -> b46(14)
	//                 \-> bmf0(15)
	g.SetTip("b46")
	g.NextBlock("bmf0", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.AddTransaction(b.STransactions[1])
	})
	rejected(blockchain.ErrStakeTxInRegularTree)

	// ---------------------------------------------------------------------
	// Block header median time tests.
	// ---------------------------------------------------------------------

	// Create a block with a timestamp that is exactly the median time.
	//
	//   ... b37(9) -> b39(10) -> b41(11) -> b44(12) -> b45(13) -> b46(14)
	//                                                                  \-> b56(15)
	g.SetTip("b46")
	g.NextBlock("b56", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
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
	//   ... b37(9) -> b39(10) -> b41(11) -> b44(12) -> b45(13) -> b46(14) -> b57(15)
	g.SetTip("b46")
	g.NextBlock("b57", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
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
	//   ... -> b57(15)
	//                 \-> b58(16)
	g.SetTip("b57")
	g.NextBlock("b58", outs[16], ticketOuts[16], func(b *wire.MsgBlock) {
		b.Transactions[1].TxIn[0].PreviousOutPoint.Index = 42
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block with transaction that pays more than its inputs.
	//
	//   ... -> b57(15)
	//                 \-> b59(16)
	g.SetTip("b57")
	g.NextBlock("b59", outs[16], ticketOuts[16], func(b *wire.MsgBlock) {
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
	//   ... -> b57(15)
	//                 \-> b60(16)
	g.SetTip("b57")
	g.NextBlock("b60", outs[16], ticketOuts[16], func(b *wire.MsgBlock) {
		// A non-final transaction must have at least one input with a
		// non-final sequence number in addition to a non-final lock
		// time.
		b.Transactions[1].LockTime = 0xffffffff
		b.Transactions[1].TxIn[0].Sequence = 0
	})
	rejected(blockchain.ErrUnfinalizedTx)

	// Create block that contains a non-final coinbase transaction.
	//
	//   ... -> b57(15)
	//                 \-> b61(16)
	g.SetTip("b57")
	g.NextBlock("b61", outs[16], ticketOuts[16], func(b *wire.MsgBlock) {
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
	//   ... -> b57(15) -> b62(16)
	//                 \-> b62a(16)
	g.SetTip("b57")
	b62a := g.NextBlock("b62a", outs[16], ticketOuts[16], func(b *wire.MsgBlock) {
		curScriptLen := len(b.Transactions[1].TxOut[0].PkScript)
		bytesToMaxSize := maxBlockSize - b.SerializeSize() +
			(curScriptLen - 4)
		sizePadScript := repeatOpcode(0x00, bytesToMaxSize)
		replaceSpendScript(sizePadScript)(b)
	})
	assertTipNonCanonicalBlockSize(&g, maxBlockSize+8)
	rejectedNonCanonical()

	g.SetTip("b57")
	b62 := g.NextBlock("b62", outs[16], ticketOuts[16], func(b *wire.MsgBlock) {
		*b = cloneBlock(b62a)
	})
	// Since the two blocks have the same hash and the generator state now
	// has b62a associated with the hash, manually remove b62a, replace it
	// with b62, and then reset the tip to it.
	g.UpdateBlockState("b62a", b62a.BlockHash(), "b62", b62)
	g.SetTip("b62")
	g.AssertTipBlockHash(b62a.BlockHash())
	g.AssertTipBlockSize(maxBlockSize)
	accepted()

	// ---------------------------------------------------------------------
	// Same block transaction spend tests.
	// ---------------------------------------------------------------------

	// Create block that spends an output created earlier in the same block.
	//
	//   ... -> b62(16) -> b63(17)
	g.SetTip("b62")
	g.NextBlock("b63", outs[17], ticketOuts[17], func(b *wire.MsgBlock) {
		spendTx3 := chaingen.MakeSpendableOut(b, 1, 0)
		tx3 := g.CreateSpendTx(&spendTx3, lowFee)
		b.AddTransaction(tx3)
	})
	accepted()

	// Create block that spends an output created later in the same block.
	//
	//   ... -> b63(17)
	//                 \-> b64(18)
	g.NextBlock("b64", nil, ticketOuts[18], func(b *wire.MsgBlock) {
		tx2 := g.CreateSpendTx(outs[18], lowFee)
		tx3 := g.CreateSpendTxForTx(tx2, b.Header.Height, 2, lowFee)
		b.AddTransaction(tx3)
		b.AddTransaction(tx2)
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block that double spends a transaction created in the same
	// block.
	//
	//   ... -> b63(17)
	//                 \-> b65(18)
	g.SetTip("b63")
	g.NextBlock("b65", outs[18], ticketOuts[18], func(b *wire.MsgBlock) {
		tx2 := b.Transactions[1]
		tx3 := g.CreateSpendTxForTx(tx2, b.Header.Height, 1, lowFee)
		tx4 := g.CreateSpendTxForTx(tx2, b.Header.Height, 1, lowFee)
		b.AddTransaction(tx3)
		b.AddTransaction(tx4)
	})
	rejected(blockchain.ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Extra subsidy tests.
	// ---------------------------------------------------------------------

	// Create block that pays 10 extra to the coinbase and a tx that only
	// pays 9 fee.
	//
	//   ... -> b63(17)
	//                 \-> b66(18)
	g.SetTip("b63")
	g.NextBlock("b66", outs[18], ticketOuts[18], additionalCoinbasePoW(10),
		additionalSpendFee(9))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create block that pays 10 extra to the coinbase and a tx that pays
	// the extra 10 fee.
	//
	//   ... -> b63(17) -> b67(18)
	g.SetTip("b63")
	g.NextBlock("b67", outs[18], ticketOuts[18], additionalCoinbasePoW(10),
		additionalSpendFee(10))
	accepted()

	// ---------------------------------------------------------------------
	// Malformed coinbase tests.
	// ---------------------------------------------------------------------

	// Create block with no proof-of-work subsidy output in the coinbase.
	//
	//   ... -> b67(18)
	//                 \-> b68(19)
	g.SetTip("b67")
	g.NextBlock("b68", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut = b.Transactions[0].TxOut[0:1]
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with an invalid script type in the coinbase block
	// commitment output.
	//
	//   ... -> b67(18)
	//                 \-> b69(19)
	g.SetTip("b67")
	g.NextBlock("b69", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut[1].PkScript = nil
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with too few bytes for the coinbase height commitment.
	//
	//   ... -> b67(18)
	//                 \-> b70(19)
	g.SetTip("b67")
	g.NextBlock("b70", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		script := opReturnScript(repeatOpcode(0x00, 3))
		b.Transactions[0].TxOut[1].PkScript = script
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with invalid block height in the coinbase commitment.
	//
	//   ... -> b67(18)
	//                 \-> b71(19)
	g.SetTip("b67")
	g.NextBlock("b71", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		script := standardCoinbaseOpReturnScript(b.Header.Height - 1)
		b.Transactions[0].TxOut[1].PkScript = script
	})
	rejected(blockchain.ErrCoinbaseHeight)

	// Create block with a fraudulent transaction (invalid index).
	//
	//   ... -> b67(18)
	//                 \-> b72(19)
	g.SetTip("b67")
	g.NextBlock("b72", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[0].TxIn[0].BlockIndex = wire.NullBlockIndex - 1
	})
	rejected(blockchain.ErrBadCoinbaseFraudProof)

	// Create block containing a transaction with no inputs.
	//
	//   ... -> b67(18)
	//                 \-> b73(19)
	g.SetTip("b67")
	g.NextBlock("b73", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[1].TxIn = nil
	})
	rejected(blockchain.ErrNoTxInputs)

	// Create block containing a transaction with no outputs.
	//
	//   ... -> b67(18)
	//                 \-> b74(19)
	g.SetTip("b67")
	g.NextBlock("b74", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut = nil
	})
	rejected(blockchain.ErrNoTxOutputs)

	// Create block containing a transaction output with negative value.
	//
	//   ... -> b67(18)
	//                 \-> b75(19)
	g.SetTip("b67")
	g.NextBlock("b75", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = -1
	})
	rejected(blockchain.ErrBadTxOutValue)

	// Create block containing a transaction output with an exceedingly
	// large (and therefore invalid) value.
	//
	//   ... -> b67(18)
	//                 \-> b76(19)
	g.SetTip("b67")
	g.NextBlock("b76", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = dcrutil.MaxAmount + 1
	})
	rejected(blockchain.ErrBadTxOutValue)

	// Create block containing a transaction whose outputs have an
	// exceedingly large (and therefore invalid) total value.
	//
	//   ... -> b67(18)
	//                 \-> b77(19)
	g.SetTip("b67")
	g.NextBlock("b77", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = dcrutil.MaxAmount
		b.Transactions[1].TxOut[1].Value = 1
	})
	rejected(blockchain.ErrBadTxOutValue)

	// Create block containing a stakebase tx with a small signature script.
	//
	//   ... -> b67(18)
	//                 \-> b78(19)
	g.SetTip("b67")
	tooSmallSbScript := repeatOpcode(0x00, minCoinbaseScriptLen-1)
	g.NextBlock("b78", outs[19], ticketOuts[19], replaceStakeSigScript(tooSmallSbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Create block containing a base stake tx with a large signature script.
	//
	//   ... -> b67(18)
	//                 \-> b79(19)
	g.SetTip("b67")
	tooLargeSbScript := repeatOpcode(0x00, maxCoinbaseScriptLen+1)
	g.NextBlock("b79", outs[19], ticketOuts[19], replaceStakeSigScript(tooLargeSbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Create block containing an input transaction with a null outpoint.
	//
	//   ... -> b67(18)
	//                 \-> b80(19)
	g.SetTip("b67")
	g.NextBlock("b80", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		tx := b.Transactions[1]
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
				wire.MaxPrevOutIndex, wire.TxTreeRegular)})
	})
	rejected(blockchain.ErrBadTxInput)

	// Create block containing duplicate tx inputs.
	//
	//   ... -> b67(18)
	//                 \-> b81(19)
	g.SetTip("b67")
	g.NextBlock("b81", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		tx := b.Transactions[1]
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: b.Transactions[1].TxIn[0].PreviousOutPoint})
	})
	rejected(blockchain.ErrDuplicateTxInputs)

	// Create block with nanosecond precision timestamp.
	//
	//   ... -> b67(18)
	//                 \-> b82(19)
	g.SetTip("b67")
	g.NextBlock("b82", outs[19], ticketOuts[19], func(b *wire.MsgBlock) {
		b.Header.Timestamp = b.Header.Timestamp.Add(1 * time.Nanosecond)
	})
	rejected(blockchain.ErrInvalidTime)

	// Create block with target difficulty that is too low (0 or below).
	//
	//   ... -> b67(18)
	//                 \-> b83(19)
	g.SetTip("b67")
	b83 := g.NextBlock("b83", outs[19], ticketOuts[19])
	{
		// This can't be done inside a munge function passed to NextBlock
		// because the block is solved after the function returns and this test
		// involves an unsolvable block.
		b83Hash := b83.BlockHash()
		b83.Header.Bits = 0x01810000 // -1 in compact form.
		g.UpdateBlockState("b83", b83Hash, "b83", b83)
	}
	rejected(blockchain.ErrUnexpectedDifficulty)

	// Create block with target difficulty that is greater than max allowed.
	//
	//   ... -> b67(18)
	//                 \-> b84(19)
	g.SetTip("b67")
	b84 := g.NextBlock("b84", outs[19], ticketOuts[19])
	{
		// This can't be done inside a munge function passed to NextBlock
		// because the block is solved after the function returns and this test
		// involves an improperly solved block.
		b84Hash := b84.BlockHash()
		b84.Header.Bits = g.Params().PowLimitBits + 1
		g.UpdateBlockState("b84", b84Hash, "b84", b84)
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
	//   ... -> b67(18)
	//                 \-> b84(19)
	g.SetTip("b67")
	scriptSize := maxBlockSigOps + 5 + (maxScriptElementSize + 1) + 1
	tooManySigOps = repeatOpcode(txscript.OP_CHECKSIG, scriptSize)
	tooManySigOps[maxBlockSigOps] = txscript.OP_PUSHDATA4
	binary.LittleEndian.PutUint32(tooManySigOps[maxBlockSigOps+1:],
		maxScriptElementSize+1)
	g.NextBlock("b84", outs[19], ticketOuts[19], replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// Create block with more than max allowed signature operations such
	// that the signature operation that pushes it over the limit is before
	// an invalid push data that claims a large amount of data even though
	// that much data is not provided.
	//
	//   ... -> b67(18)
	//                 \-> b85(19)
	g.SetTip("b67")
	scriptSize = maxBlockSigOps + 5 + maxScriptElementSize + 1
	tooManySigOps = repeatOpcode(txscript.OP_CHECKSIG, scriptSize)
	tooManySigOps[maxBlockSigOps+1] = txscript.OP_PUSHDATA4
	binary.LittleEndian.PutUint32(tooManySigOps[maxBlockSigOps+2:], 0xffffffff)
	g.NextBlock("b85", outs[19], ticketOuts[19], replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// ---------------------------------------------------------------------
	// Dead execution path tests.
	// ---------------------------------------------------------------------

	// Create block with an invalid opcode in a dead execution path.
	//
	//   ... -> b67(18) -> b86(19)
	script := []byte{txscript.OP_IF, txscript.OP_INVALIDOPCODE,
		txscript.OP_ELSE, txscript.OP_TRUE, txscript.OP_ENDIF}
	g.SetTip("b67")
	g.NextBlock("b86", outs[19], ticketOuts[19], replaceSpendScript(script),
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
	//   ... -> b86(19) -> b87(20)
	g.NextBlock("b87", outs[20], ticketOuts[20], func(b *wire.MsgBlock) {
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
	b87OpReturnOut := chaingen.MakeSpendableOut(g.Tip(), 5, 1)
	accepted()

	// Reorg to a side chain that does not contain the OP_RETURNs.
	//
	//   ... -> b86(19) -> b87(20)
	//                 \-> b88(20) -> b89(21)
	g.SetTip("b86")
	g.NextBlock("b88", outs[20], ticketOuts[20])
	acceptedToSideChainWithExpectedTip("b87")

	g.NextBlock("b89", outs[21], ticketOuts[21])
	accepted()

	// Reorg back to the original chain that contains the OP_RETURNs.
	//
	//   ... -> b86(19) -> b87(20) -> b90(21) -> b91(22)
	//                 \-> b88(20) -> b89(21)
	g.SetTip("b87")
	g.NextBlock("b90", outs[21], ticketOuts[21])
	acceptedToSideChainWithExpectedTip("b89")

	g.NextBlock("b91", outs[22], ticketOuts[22])
	accepted()

	// Create a block that spends an OP_RETURN.
	//
	//   ... -> b86(19) -> b87(20) -> b90(21) -> b91(22)
	//                 \-> b88(20) -> b89(21)           \-> b92(b87.tx[5].out[1])
	g.NextBlock("b92", nil, ticketOuts[23], func(b *wire.MsgBlock) {
		// An OP_RETURN output doesn't have any value so use a fee of 0.
		zeroFee := dcrutil.Amount(0)
		tx := g.CreateSpendTx(&b87OpReturnOut, zeroFee)
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create a block that has a transaction with multiple OP_RETURNs.  Even
	// though a transaction with a large number of OP_RETURNS is not
	// considered a standard transaction, it is still valid by the consensus
	// rules.
	//
	//   ... -> b91(22) -> b93(23)
	//
	g.SetTip("b91")
	g.NextBlock("b93", outs[23], ticketOuts[23], func(b *wire.MsgBlock) {
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
	// Large block re-org test.
	// ---------------------------------------------------------------------

	if !includeLargeReorg {
		return tests, nil
	}

	// Ensure the tip the re-org test builds on is the best chain tip.
	//
	//   ... -> b93(23) -> ...
	//g.AssertTipHeight()
	g.SetTip("b93")
	spendableOutOffset := int32(24) // Next spendable offset.

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
		g.NextBlock(chain1TipName, &reorgSpend, reorgTicketSpends, func(b *wire.MsgBlock) {
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
