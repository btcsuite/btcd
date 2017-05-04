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

// additionalPoWTx returns a function that itself takes a block and modifies it
// by adding the the provided transaction to the regular transaction tree.
func additionalPoWTx(tx *wire.MsgTx) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.AddTransaction(tx)
	}
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
			acceptBlock(g.TipName(), g.Tip(), true, false),
		})
	}
	acceptedToSideChainWithExpectedTip := func(tipName string) {
		tests = append(tests, []TestInstance{
			acceptBlock(g.TipName(), g.Tip(), false, false),
			expectTipBlock(tipName, g.BlockByName(tipName)),
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
	rejected := func(code blockchain.ErrorCode) {
		tests = append(tests, []TestInstance{
			rejectBlock(g.TipName(), g.Tip(), code),
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
	// vote unset to ensure they are properly rejected.
	//
	//   ... -> bse# -> bsv0 -> bsv1 -> ... -> bsv#
	//             \       \       \        \-> bevbad#
	//              |       |       \-> bevbad2
	//              |       \-> bevbad1
	//              \-> bevbad0
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
	rejected(blockchain.ErrMissingTx)

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
	manySigOps := bytes.Repeat([]byte{txscript.OP_CHECKSIG}, maxBlockSigOps)
	g.NextBlock("b21", outs[6], ticketOuts[6], replaceSpendScript(manySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Attempt to add block with more than max allowed signature operations.
	//
	//   ... -> b5(2) -> b12(3) -> b18(4) -> b19(5) -> b21(6)
	//   \                                                   \-> b22(7)
	//    \-> b3(1) -> b4(2)
	tooManySigOps := bytes.Repeat([]byte{txscript.OP_CHECKSIG}, maxBlockSigOps+1)
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
	rejected(blockchain.ErrMissingTx)

	// Create block that forks and spends a tx created on a third fork.
	//
	//   ... -> b5(2) -> b12(3) -> b18(4) -> b19(5) -> b21(6)
	//   |                                         \-> b24(b3.tx[1]) -> b25(6)
	//    \-> b3(1) -> b4(2)
	g.SetTip("b19")
	g.NextBlock("b24", &b3Tx1Out, nil)
	acceptedToSideChainWithExpectedTip("b21")

	g.NextBlock("b25", outs[6], ticketOuts[6])
	rejected(blockchain.ErrMissingTx)

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
	//   ... -> b21(6)
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

	// Attempt to add block where ssgen has a null ticket reference hash.
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
	g.NextBlock("bv4", outs[9], ticketOuts[9], g.ReplaceWithNVotes(ticketsPerBlock-3))
	rejected(blockchain.ErrNotEnoughVotes)

	// Attempt to add block with different number of votes in stake tree and
	// header.
	//
	//   ... -> b36(8)
	//                \-> bv5(9)
	g.SetTip("b36")
	g.NextBlock("bv5", outs[9], ticketOuts[9], func(b *wire.MsgBlock) {
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
	// not the required script, but is otherwise
	// script.
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
	// TODO: This really shoud be ErrDoubleSpend
	rejected(blockchain.ErrMissingTx)

	g.SetTip("b41")
	g.NextBlock("b43", &b42Tx1Out, ticketOuts[12])
	rejected(blockchain.ErrMissingTx)

	// ---------------------------------------------------------------------
	// Various malformed block tests.
	// ---------------------------------------------------------------------

	// Create block with an otherwise valid transaction in place of where
	// the coinbase must be.
	//
	//   ... -> b41(11)
	//                 \-> b44(12)
	g.NextBlock("b44", nil, ticketOuts[12], func(b *wire.MsgBlock) {
		nonCoinbaseTx := g.CreateSpendTx(outs[12], lowFee)
		b.Transactions[0] = nonCoinbaseTx
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with no transactions.
	//
	//   ... -> b41(11)
	//                 \-> b45(_)
	g.SetTip("b41")
	g.NextBlock("b45", nil, nil, func(b *wire.MsgBlock) {
		b.Transactions = nil
	})
	rejected(blockchain.ErrNoTransactions)

	// Create block with invalid proof of work.
	//
	//   ... -> b41(11)
	//                 \-> b46(12)
	g.SetTip("b41")
	b46 := g.NextBlock("b46", outs[12], ticketOuts[12])
	// This can't be done inside a munge function passed to NextBlock
	// because the block is solved after the function returns and this test
	// requires an unsolved block.
	{
		origHash := b46.BlockHash()
		for {
			// Keep incrementing the nonce until the hash treated as
			// a uint256 is higher than the limit.
			b46.Header.Nonce += 1
			hash := b46.BlockHash()
			hashNum := blockchain.HashToBig(&hash)
			if hashNum.Cmp(g.Params().PowLimit) >= 0 {
				break
			}
		}
		g.UpdateBlockState("b46", origHash, "b46", b46)
	}
	rejected(blockchain.ErrHighHash)

	// Create block with a timestamp too far in the future.
	//
	//   ... -> b41(11)
	//                 \-> b47(12)
	g.SetTip("b41")
	g.NextBlock("b47", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		// 3 hours in the future clamped to 1 second precision.
		nowPlus3Hours := time.Now().Add(time.Hour * 3)
		b.Header.Timestamp = time.Unix(nowPlus3Hours.Unix(), 0)
	})
	rejected(blockchain.ErrTimeTooNew)

	// Create block with an invalid merkle root.
	//
	//   ... -> b41(11)
	//                 \-> b48(12)
	g.SetTip("b41")
	g.NextBlock("b48", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		b.Header.MerkleRoot = chainhash.Hash{}
	})
	g.AssertTipBlockMerkleRoot(chainhash.Hash{})
	rejected(blockchain.ErrBadMerkleRoot)

	// Create block with an invalid proof-of-work limit.
	//
	//   ... -> b41(11)
	//                 \-> b49(12)
	g.SetTip("b41")
	g.NextBlock("b49", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		b.Header.Bits -= 1
	})
	rejected(blockchain.ErrUnexpectedDifficulty)

	// Create block with two coinbase transactions.
	//
	//   ... -> b41(11)
	//                 \-> b50(12)
	g.SetTip("b41")
	coinbaseTx := g.CreateCoinbaseTx(g.Tip().Header.Height+1, ticketsPerBlock)
	g.NextBlock("b50", outs[12], ticketOuts[12], additionalPoWTx(coinbaseTx))
	rejected(blockchain.ErrMultipleCoinbases)

	// Create block with duplicate transactions in the regular transaction
	// tree.
	//
	// This test relies on the shape of the shape of the merkle tree to test
	// the intended condition.  That is the reason for the assertion.
	//
	//   ... -> b41(11)
	//                 \-> b51(12)
	g.SetTip("b41")
	g.NextBlock("b51", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		b.AddTransaction(b.Transactions[1])
	})
	g.AssertTipBlockNumTxns(3)
	rejected(blockchain.ErrDuplicateTx)

	// Create block with state tx in regular tx tree.
	//
	//   ... -> b41(11)
	//                 \-> bmf0(12)
	g.SetTip("b41")
	g.NextBlock("bmf0", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
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
	g.SetTip("b41")
	g.NextBlock("b52", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		medianBlk := g.BlockByHash(&b.Header.PrevBlock)
		for i := 0; i < (medianTimeBlocks / 2); i++ {
			medianBlk = g.BlockByHash(&medianBlk.Header.PrevBlock)
		}
		b.Header.Timestamp = medianBlk.Header.Timestamp
	})
	rejected(blockchain.ErrTimeTooOld)

	// Create a block with a timestamp that is one second after the median
	// time.
	//
	//   ... b21(6) -> b29(7) -> b36(8) -> b37(9) -> b39(10) -> b41(11) -> b53(12)
	g.SetTip("b41")
	g.NextBlock("b53", outs[12], ticketOuts[12], func(b *wire.MsgBlock) {
		medianBlk := g.BlockByHash(&b.Header.PrevBlock)
		for i := 0; i < (medianTimeBlocks / 2); i++ {
			medianBlk = g.BlockByHash(&medianBlk.Header.PrevBlock)
		}
		medianBlockTime := medianBlk.Header.Timestamp
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
	g.SetTip("b53")
	g.NextBlock("b54", outs[13], ticketOuts[13], func(b *wire.MsgBlock) {
		b.Transactions[1].TxIn[0].PreviousOutPoint.Index = 12345
	})
	rejected(blockchain.ErrMissingTx)

	// Create block with transaction that pays more than its inputs.
	//
	//   ... -> b53(12)
	//                 \-> b54(13)
	g.SetTip("b53")
	g.NextBlock("b54", outs[13], ticketOuts[13], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = int64(outs[13].Amount() + 2)
	})
	rejected(blockchain.ErrSpendTooHigh)

	// ---------------------------------------------------------------------
	// Blocks with non-final transaction tests.
	// ---------------------------------------------------------------------

	// Create block that contains a non-final non-coinbase transaction.
	//
	//   ... -> b53(12)
	//                 \-> b55(13)
	g.SetTip("b53")
	g.NextBlock("b55", outs[13], ticketOuts[13], func(b *wire.MsgBlock) {
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
	g.SetTip("b53")
	g.NextBlock("b56", outs[13], ticketOuts[13], func(b *wire.MsgBlock) {
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
	g.SetTip("b53")
	g.NextBlock("b57", outs[13], ticketOuts[13], func(b *wire.MsgBlock) {
		spendTx3 := chaingen.MakeSpendableOut(b, 1, 0)
		tx3 := g.CreateSpendTx(&spendTx3, lowFee)
		b.AddTransaction(tx3)
	})
	accepted()

	// Create block that spends an output created later in the same block.
	//
	//   ... -> b57(13)
	//                 \-> b58(14)
	g.NextBlock("b58", nil, ticketOuts[14], func(b *wire.MsgBlock) {
		tx2 := g.CreateSpendTx(outs[14], lowFee)
		tx3 := g.CreateSpendTxForTx(tx2, b.Header.Height, 2, lowFee)
		b.AddTransaction(tx3)
		b.AddTransaction(tx2)
	})
	rejected(blockchain.ErrMissingTx)

	// Create block that double spends a transaction created in the same
	// block.
	//
	//   ... -> b57(13)
	//                 \-> b59(14)
	g.SetTip("b57")
	g.NextBlock("b59", outs[14], ticketOuts[14], func(b *wire.MsgBlock) {
		tx2 := b.Transactions[1]
		tx3 := g.CreateSpendTxForTx(tx2, b.Header.Height, 1, lowFee)
		tx4 := g.CreateSpendTxForTx(tx2, b.Header.Height, 1, lowFee)
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
	g.SetTip("b57")
	g.NextBlock("b60", outs[14], ticketOuts[14], additionalCoinbasePoW(10),
		additionalSpendFee(9))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create block that pays 10 extra to the coinbase and a tx that pays
	// the extra 10 fee.
	//
	//   ... -> b57(13) -> b61(14)
	g.SetTip("b57")
	g.NextBlock("b61", outs[14], ticketOuts[14], additionalCoinbasePoW(10),
		additionalSpendFee(10))
	accepted()

	// ---------------------------------------------------------------------
	// Non-existent transaction spend tests.
	// ---------------------------------------------------------------------

	// Create a block that spends a transaction that does not exist.
	//
	//   ... -> b61(14)
	//                 \-> b62(15)
	g.SetTip("b61")
	g.NextBlock("b62", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
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
	g.SetTip("b61")
	g.NextBlock("b63", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut = b.Transactions[0].TxOut[0:1]
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with an invalid script type in the coinbase block
	// commitment output.
	//
	//   ... -> b61(14)
	//                 \-> b64(15)
	g.SetTip("b61")
	g.NextBlock("b64", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[0].TxOut[1].PkScript = nil
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with too few bytes for the coinbase height commitment.
	//
	//   ... -> b61(14)
	//                 \-> b65(15)
	g.SetTip("b61")
	g.NextBlock("b65", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		script := opReturnScript(repeatOpcode(0x00, 3))
		b.Transactions[0].TxOut[1].PkScript = script
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with invalid block height in the coinbase commitment.
	//
	//   ... -> b61(14)
	//                 \-> b66(15)
	g.SetTip("b61")
	g.NextBlock("b66", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		script := standardCoinbaseOpReturnScript(b.Header.Height - 1)
		b.Transactions[0].TxOut[1].PkScript = script
	})
	rejected(blockchain.ErrCoinbaseHeight)

	// Create block with a fraudulent transaction (invalid index).
	//
	//   ... -> b61(14)
	//                 \-> b67(15)
	g.SetTip("b61")
	g.NextBlock("b67", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[0].TxIn[0].BlockIndex = wire.NullBlockIndex - 1
	})
	rejected(blockchain.ErrBadCoinbaseFraudProof)

	// Create block containing a transaction with no inputs.
	//
	//   ... -> b61(14)
	//                 \-> b68(15)
	g.SetTip("b61")
	g.NextBlock("b68", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[1].TxIn = nil
	})
	rejected(blockchain.ErrNoTxInputs)

	// Create block containing a transaction with no outputs.
	//
	//   ... -> b61(14)
	//                 \-> b69(15)
	g.SetTip("b61")
	g.NextBlock("b69", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut = nil
	})
	rejected(blockchain.ErrNoTxOutputs)

	// Create block containing a transaction output with negative value.
	//
	//   ... -> b61(14)
	//                 \-> b70(15)
	g.SetTip("b61")
	g.NextBlock("b70", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = -1
	})
	rejected(blockchain.ErrBadTxOutValue)

	// Create block containing a transaction output with an exceedingly
	// large (and therefore invalid) value.
	//
	//   ... -> b61(14)
	//                 \-> b71(15)
	g.SetTip("b61")
	g.NextBlock("b71", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = dcrutil.MaxAmount + 1
	})
	rejected(blockchain.ErrBadTxOutValue)

	// Create block containing a transaction whose outputs have an
	// exceedingly large (and therefore invalid) total value.
	//
	//   ... -> b61(14)
	//                 \-> b72(15)
	g.SetTip("b61")
	g.NextBlock("b72", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = dcrutil.MaxAmount
		b.Transactions[1].TxOut[1].Value = 1
	})
	rejected(blockchain.ErrBadTxOutValue)

	// Create block containing a stakebase tx with a small signature script.
	//
	//   ... -> b61(14)
	//                 \-> b73(15)
	g.SetTip("b61")
	tooSmallSbScript := repeatOpcode(0x00, minCoinbaseScriptLen-1)
	g.NextBlock("b73", outs[15], ticketOuts[15], replaceStakeSigScript(tooSmallSbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Create block containing a base stake tx with a large signature script.
	//
	//   ... -> b61(14)
	//                 \-> b74(15)
	g.SetTip("b61")
	tooLargeSbScript := repeatOpcode(0x00, maxCoinbaseScriptLen+1)
	g.NextBlock("b74", outs[15], ticketOuts[15], replaceStakeSigScript(tooLargeSbScript))
	rejected(blockchain.ErrBadStakebaseScriptLen)

	// Create block containing an input transaction with a null outpoint.
	//
	//   ... -> b61(14)
	//                 \-> b75(15)
	g.SetTip("b61")
	g.NextBlock("b75", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		tx := b.Transactions[1]
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
				wire.MaxPrevOutIndex, wire.TxTreeRegular)})
	})
	rejected(blockchain.ErrBadTxInput)

	// Create block containing duplicate tx inputs.
	//
	//   ... -> b61(14)
	//                 \-> b76(15)
	g.SetTip("b61")
	g.NextBlock("b76", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		tx := b.Transactions[1]
		tx.AddTxIn(&wire.TxIn{
			PreviousOutPoint: b.Transactions[1].TxIn[0].PreviousOutPoint})
	})
	rejected(blockchain.ErrDuplicateTxInputs)

	// Create block with nanosecond precision timestamp.
	//
	//   ... -> b61(14)
	//                 \-> b77(15)
	g.SetTip("b61")
	g.NextBlock("b77", outs[15], ticketOuts[15], func(b *wire.MsgBlock) {
		b.Header.Timestamp = b.Header.Timestamp.Add(1 * time.Nanosecond)
	})
	rejected(blockchain.ErrInvalidTime)

	// Create block with target difficulty that is too low (0 or below).
	//
	//   ... -> b61(14)
	//                 \-> b78(15)
	g.SetTip("b61")
	b78 := g.NextBlock("b78", outs[15], ticketOuts[15])
	{
		// This can't be done inside a munge function passed to NextBlock
		// because the block is solved after the function returns and this test
		// involves an unsolvable block.
		b78Hash := b78.BlockHash()
		b78.Header.Bits = 0x01810000 // -1 in compact form.
		g.UpdateBlockState("b78", b78Hash, "b78", b78)
	}
	rejected(blockchain.ErrUnexpectedDifficulty)

	// Create block with target difficulty that is greater than max allowed.
	//
	//   ... -> b61(14)
	//                 \-> b79(15)
	g.SetTip("b61")
	b79 := g.NextBlock("b79", outs[15], ticketOuts[15])
	{
		// This can't be done inside a munge function passed to NextBlock
		// because the block is solved after the function returns and this test
		// involves an improperly solved block.
		b79Hash := b79.BlockHash()
		b79.Header.Bits = g.Params().PowLimitBits + 1
		g.UpdateBlockState("b79", b79Hash, "b79", b79)
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
	g.SetTip("b61")
	scriptSize := maxBlockSigOps + 5 + (maxScriptElementSize + 1) + 1
	tooManySigOps = repeatOpcode(txscript.OP_CHECKSIG, scriptSize)
	tooManySigOps[maxBlockSigOps] = txscript.OP_PUSHDATA4
	binary.LittleEndian.PutUint32(tooManySigOps[maxBlockSigOps+1:],
		maxScriptElementSize+1)
	g.NextBlock("b80", outs[15], ticketOuts[15], replaceSpendScript(tooManySigOps))
	g.AssertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	return tests, nil
}
