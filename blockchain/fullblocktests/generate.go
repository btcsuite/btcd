// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// The vast majority of the rules tested in this package were ported from the
// the original Java-based 'official' block acceptance tests at
// https://github.com/TheBlueMatt/test-scripts as well as some additional tests
// available in the Core python port of the same.

package fullblocktests

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"runtime"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

const (
	// Intentionally defined here rather than using constants from codebase
	// to ensure consensus changes are detected.
	maxBlockSigOps       = 20000
	maxBlockSize         = 1000000
	minCoinbaseScriptLen = 2
	maxCoinbaseScriptLen = 100
	medianTimeBlocks     = 11
	maxScriptElementSize = 520

	// numLargeReorgBlocks is the number of blocks to use in the large block
	// reorg test (when enabled).  This is the equivalent of 1 week's worth
	// of blocks.
	numLargeReorgBlocks = 1088
)

var (
	// opTrueScript is simply a public key script that contains the OP_TRUE
	// opcode.  It is defined here to reduce garbage creation.
	opTrueScript = []byte{txscript.OP_TRUE}

	// lowFee is a single satoshi and exists to make the test code more
	// readable.
	lowFee = btcutil.Amount(1)
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
	Height      int32
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
	Height     int32
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
	Name   string
	Block  *wire.MsgBlock
	Height int32
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
	Name   string
	Block  *wire.MsgBlock
	Height int32
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

// FullBlockTestInstance only exists to allow RejectedNonCanonicalBlock to be treated as
// a TestInstance.
//
// This implements the TestInstance interface.
func (b RejectedNonCanonicalBlock) FullBlockTestInstance() {}

// spendableOut represents a transaction output that is spendable along with
// additional metadata such as the block its in and how much it pays.
type spendableOut struct {
	prevOut wire.OutPoint
	amount  btcutil.Amount
}

// makeSpendableOutForTx returns a spendable output for the given transaction
// and transaction output index within the transaction.
func makeSpendableOutForTx(tx *wire.MsgTx, txOutIndex uint32) spendableOut {
	return spendableOut{
		prevOut: wire.OutPoint{
			Hash:  tx.TxHash(),
			Index: txOutIndex,
		},
		amount: btcutil.Amount(tx.TxOut[txOutIndex].Value),
	}
}

// makeSpendableOut returns a spendable output for the given block, transaction
// index within the block, and transaction output index within the transaction.
func makeSpendableOut(block *wire.MsgBlock, txIndex, txOutIndex uint32) spendableOut {
	return makeSpendableOutForTx(block.Transactions[txIndex], txOutIndex)
}

// testGenerator houses state used to easy the process of generating test blocks
// that build from one another along with housing other useful things such as
// available spendable outputs used throughout the tests.
type testGenerator struct {
	params       *chaincfg.Params
	tip          *wire.MsgBlock
	tipName      string
	tipHeight    int32
	blocks       map[chainhash.Hash]*wire.MsgBlock
	blocksByName map[string]*wire.MsgBlock
	blockHeights map[string]int32

	// Used for tracking spendable coinbase outputs.
	spendableOuts     []spendableOut
	prevCollectedHash chainhash.Hash

	// Common key for any tests which require signed transactions.
	privKey *btcec.PrivateKey
}

// makeTestGenerator returns a test generator instance initialized with the
// genesis block as the tip.
func makeTestGenerator(params *chaincfg.Params) (testGenerator, error) {
	privKey, _ := btcec.PrivKeyFromBytes([]byte{0x01})
	genesis := params.GenesisBlock
	genesisHash := genesis.BlockHash()
	return testGenerator{
		params:       params,
		blocks:       map[chainhash.Hash]*wire.MsgBlock{genesisHash: genesis},
		blocksByName: map[string]*wire.MsgBlock{"genesis": genesis},
		blockHeights: map[string]int32{"genesis": 0},
		tip:          genesis,
		tipName:      "genesis",
		tipHeight:    0,
		privKey:      privKey,
	}, nil
}

// payToScriptHashScript returns a standard pay-to-script-hash for the provided
// redeem script.
func payToScriptHashScript(redeemScript []byte) []byte {
	redeemScriptHash := btcutil.Hash160(redeemScript)
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

// standardCoinbaseScript returns a standard script suitable for use as the
// signature script of the coinbase transaction of a new block.  In particular,
// it starts with the block height that is required by version 2 blocks.
func standardCoinbaseScript(blockHeight int32, extraNonce uint64) ([]byte, error) {
	return txscript.NewScriptBuilder().AddInt64(int64(blockHeight)).
		AddInt64(int64(extraNonce)).Script()
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

	data := make([]byte, 8)
	binary.LittleEndian.PutUint64(data[0:8], rand)
	return opReturnScript(data)
}

// createCoinbaseTx returns a coinbase transaction paying an appropriate
// subsidy based on the passed block height.  The coinbase signature script
// conforms to the requirements of version 2 blocks.
func (g *testGenerator) createCoinbaseTx(blockHeight int32) *wire.MsgTx {
	extraNonce := uint64(0)
	coinbaseScript, err := standardCoinbaseScript(blockHeight, extraNonce)
	if err != nil {
		panic(err)
	}

	tx := wire.NewMsgTx(1)
	tx.AddTxIn(&wire.TxIn{
		// Coinbase transactions have no inputs, so previous outpoint is
		// zero hash and max index.
		PreviousOutPoint: *wire.NewOutPoint(&chainhash.Hash{},
			wire.MaxPrevOutIndex),
		Sequence:        wire.MaxTxInSequenceNum,
		SignatureScript: coinbaseScript,
	})
	tx.AddTxOut(&wire.TxOut{
		Value:    blockchain.CalcBlockSubsidy(blockHeight, g.params),
		PkScript: opTrueScript,
	})
	return tx
}

// calcMerkleRoot creates a merkle tree from the slice of transactions and
// returns the root of the tree.
func calcMerkleRoot(txns []*wire.MsgTx) chainhash.Hash {
	if len(txns) == 0 {
		return chainhash.Hash{}
	}

	utilTxns := make([]*btcutil.Tx, 0, len(txns))
	for _, tx := range txns {
		utilTxns = append(utilTxns, btcutil.NewTx(tx))
	}
	return blockchain.CalcMerkleRoot(utilTxns, false)
}

// solveBlock attempts to find a nonce which makes the passed block header hash
// to a value less than the target difficulty.  When a successful solution is
// found true is returned and the nonce field of the passed header is updated
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

// additionalCoinbase returns a function that itself takes a block and
// modifies it by adding the provided amount to coinbase subsidy.
func additionalCoinbase(amount btcutil.Amount) func(*wire.MsgBlock) {
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
// 'additionalCoinbase' for that purpose.
func additionalSpendFee(fee btcutil.Amount) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		// Increase the fee of the spending transaction by reducing the
		// amount paid.
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

// additionalTx returns a function that itself takes a block and modifies it by
// adding the provided transaction.
func additionalTx(tx *wire.MsgTx) func(*wire.MsgBlock) {
	return func(b *wire.MsgBlock) {
		b.AddTransaction(tx)
	}
}

// createSpendTx creates a transaction that spends from the provided spendable
// output and includes an additional unique OP_RETURN output to ensure the
// transaction ends up with a unique hash.  The script is a simple OP_TRUE
// script which avoids the need to track addresses and signature scripts in the
// tests.
func createSpendTx(spend *spendableOut, fee btcutil.Amount) *wire.MsgTx {
	spendTx := wire.NewMsgTx(1)
	spendTx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: spend.prevOut,
		Sequence:         wire.MaxTxInSequenceNum,
		SignatureScript:  nil,
	})
	spendTx.AddTxOut(wire.NewTxOut(int64(spend.amount-fee),
		opTrueScript))
	spendTx.AddTxOut(wire.NewTxOut(0, uniqueOpReturnScript()))

	return spendTx
}

// createSpendTxForTx creates a transaction that spends from the first output of
// the provided transaction and includes an additional unique OP_RETURN output
// to ensure the transaction ends up with a unique hash.  The public key script
// is a simple OP_TRUE script which avoids the need to track addresses and
// signature scripts in the tests.  The signature script is nil.
func createSpendTxForTx(tx *wire.MsgTx, fee btcutil.Amount) *wire.MsgTx {
	spend := makeSpendableOutForTx(tx, 0)
	return createSpendTx(&spend, fee)
}

// nextBlock builds a new block that extends the current tip associated with the
// generator and updates the generator's tip to the newly generated block.
//
// The block will include the following:
// - A coinbase that pays the required subsidy to an OP_TRUE script
// - When a spendable output is provided:
//   - A transaction that spends from the provided output the following outputs:
//   - One that pays the inputs amount minus 1 atom to an OP_TRUE script
//   - One that contains an OP_RETURN output with a random uint64 in order to
//     ensure the transaction has a unique hash
//
// Additionally, if one or more munge functions are specified, they will be
// invoked with the block prior to solving it.  This provides callers with the
// opportunity to modify the block which is especially useful for testing.
//
// In order to simply the logic in the munge functions, the following rules are
// applied after all munge functions have been invoked:
// - The merkle root will be recalculated unless it was manually changed
// - The block will be solved unless the nonce was changed
func (g *testGenerator) nextBlock(blockName string, spend *spendableOut, mungers ...func(*wire.MsgBlock)) *wire.MsgBlock {
	// Create coinbase transaction for the block using any additional
	// subsidy if specified.
	nextHeight := g.tipHeight + 1
	coinbaseTx := g.createCoinbaseTx(nextHeight)
	txns := []*wire.MsgTx{coinbaseTx}
	if spend != nil {
		// Create the transaction with a fee of 1 atom for the
		// miner and increase the coinbase subsidy accordingly.
		fee := btcutil.Amount(1)
		coinbaseTx.TxOut[0].Value += int64(fee)

		// Create a transaction that spends from the provided spendable
		// output and includes an additional unique OP_RETURN output to
		// ensure the transaction ends up with a unique hash, then add
		// add it to the list of transactions to include in the block.
		// The script is a simple OP_TRUE script in order to avoid the
		// need to track addresses and signature scripts in the tests.
		txns = append(txns, createSpendTx(spend, fee))
	}

	// Use a timestamp that is one second after the previous block unless
	// this is the first block in which case the current time is used.
	var ts time.Time
	if nextHeight == 1 {
		ts = time.Unix(time.Now().Unix(), 0)
	} else {
		ts = g.tip.Header.Timestamp.Add(time.Second)
	}

	block := wire.MsgBlock{
		Header: wire.BlockHeader{
			Version:    1,
			PrevBlock:  g.tip.BlockHash(),
			MerkleRoot: calcMerkleRoot(txns),
			Bits:       g.params.PowLimitBits,
			Timestamp:  ts,
			Nonce:      0, // To be solved.
		},
		Transactions: txns,
	}

	// Perform any block munging just before solving.  Only recalculate the
	// merkle root if it wasn't manually changed by a munge function.
	curMerkleRoot := block.Header.MerkleRoot
	curNonce := block.Header.Nonce
	for _, f := range mungers {
		f(&block)
	}
	if block.Header.MerkleRoot == curMerkleRoot {
		block.Header.MerkleRoot = calcMerkleRoot(block.Transactions)
	}

	// Only solve the block if the nonce wasn't manually changed by a munge
	// function.
	if block.Header.Nonce == curNonce && !solveBlock(&block.Header) {
		panic(fmt.Sprintf("Unable to solve block at height %d",
			nextHeight))
	}

	// Update generator state and return the block.
	blockHash := block.BlockHash()
	g.blocks[blockHash] = &block
	g.blocksByName[blockName] = &block
	g.blockHeights[blockName] = nextHeight
	g.tip = &block
	g.tipName = blockName
	g.tipHeight = nextHeight
	return &block
}

// updateBlockState manually updates the generator state to remove all internal
// map references to a block via its old hash and insert new ones for the new
// block hash.  This is useful if the test code has to manually change a block
// after 'nextBlock' has returned.
func (g *testGenerator) updateBlockState(oldBlockName string, oldBlockHash chainhash.Hash, newBlockName string, newBlock *wire.MsgBlock) {
	// Look up the height from the existing entries.
	blockHeight := g.blockHeights[oldBlockName]

	// Remove existing entries.
	delete(g.blocks, oldBlockHash)
	delete(g.blocksByName, oldBlockName)
	delete(g.blockHeights, oldBlockName)

	// Add new entries.
	newBlockHash := newBlock.BlockHash()
	g.blocks[newBlockHash] = newBlock
	g.blocksByName[newBlockName] = newBlock
	g.blockHeights[newBlockName] = blockHeight
}

// setTip changes the tip of the instance to the block with the provided name.
// This is useful since the tip is used for things such as generating subsequent
// blocks.
func (g *testGenerator) setTip(blockName string) {
	g.tip = g.blocksByName[blockName]
	g.tipName = blockName
	g.tipHeight = g.blockHeights[blockName]
}

// oldestCoinbaseOuts removes the oldest coinbase output that was previously
// saved to the generator and returns the set as a slice.
func (g *testGenerator) oldestCoinbaseOut() spendableOut {
	op := g.spendableOuts[0]
	g.spendableOuts = g.spendableOuts[1:]
	return op
}

// saveTipCoinbaseOut adds the coinbase tx output in the current tip block to
// the list of spendable outputs.
func (g *testGenerator) saveTipCoinbaseOut() {
	g.spendableOuts = append(g.spendableOuts, makeSpendableOut(g.tip, 0, 0))
	g.prevCollectedHash = g.tip.BlockHash()
}

// saveSpendableCoinbaseOuts adds all coinbase outputs from the last block that
// had its coinbase tx output colleted to the current tip.  This is useful to
// batch the collection of coinbase outputs once the tests reach a stable point
// so they don't have to manually add them for the right tests which will
// ultimately end up being the best chain.
func (g *testGenerator) saveSpendableCoinbaseOuts() {
	// Ensure tip is reset to the current one when done.
	curTipName := g.tipName
	defer g.setTip(curTipName)

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
		g.tip = collectBlocks[len(collectBlocks)-1-i]
		g.saveTipCoinbaseOut()
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
	b.Header.BtcEncode(&buf, 0, wire.BaseEncoding)
	buf.Write(nonCanonicalVarInt(uint32(len(b.Transactions))))
	for _, tx := range b.Transactions {
		tx.BtcEncode(&buf, 0, wire.BaseEncoding)
	}
	return buf.Bytes()
}

// cloneBlock returns a deep copy of the provided block.
func cloneBlock(b *wire.MsgBlock) wire.MsgBlock {
	var blockCopy wire.MsgBlock
	blockCopy.Header = b.Header
	for _, tx := range b.Transactions {
		blockCopy.AddTransaction(tx.Copy())
	}
	return blockCopy
}

// repeatOpcode returns a byte slice with the provided opcode repeated the
// specified number of times.
func repeatOpcode(opcode uint8, numRepeats int) []byte {
	return bytes.Repeat([]byte{opcode}, numRepeats)
}

// assertScriptSigOpsCount panics if the provided script does not have the
// specified number of signature operations.
func assertScriptSigOpsCount(script []byte, expected int) {
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

// assertTipBlockSigOpsCount panics if the current tip block associated with the
// generator does not have the specified number of signature operations.
func (g *testGenerator) assertTipBlockSigOpsCount(expected int) {
	numSigOps := countBlockSigOps(g.tip)
	if numSigOps != expected {
		panic(fmt.Sprintf("generated number of sigops for block %q "+
			"(height %d) is %d instead of expected %d", g.tipName,
			g.tipHeight, numSigOps, expected))
	}
}

// assertTipBlockSize panics if the if the current tip block associated with the
// generator does not have the specified size when serialized.
func (g *testGenerator) assertTipBlockSize(expected int) {
	serializeSize := g.tip.SerializeSize()
	if serializeSize != expected {
		panic(fmt.Sprintf("block size of block %q (height %d) is %d "+
			"instead of expected %d", g.tipName, g.tipHeight,
			serializeSize, expected))
	}
}

// assertTipNonCanonicalBlockSize panics if the if the current tip block
// associated with the generator does not have the specified non-canonical size
// when serialized.
func (g *testGenerator) assertTipNonCanonicalBlockSize(expected int) {
	serializeSize := len(encodeNonCanonicalBlock(g.tip))
	if serializeSize != expected {
		panic(fmt.Sprintf("block size of block %q (height %d) is %d "+
			"instead of expected %d", g.tipName, g.tipHeight,
			serializeSize, expected))
	}
}

// assertTipBlockNumTxns panics if the number of transactions in the current tip
// block associated with the generator does not match the specified value.
func (g *testGenerator) assertTipBlockNumTxns(expected int) {
	numTxns := len(g.tip.Transactions)
	if numTxns != expected {
		panic(fmt.Sprintf("number of txns in block %q (height %d) is "+
			"%d instead of expected %d", g.tipName, g.tipHeight,
			numTxns, expected))
	}
}

// assertTipBlockHash panics if the current tip block associated with the
// generator does not match the specified hash.
func (g *testGenerator) assertTipBlockHash(expected chainhash.Hash) {
	hash := g.tip.BlockHash()
	if hash != expected {
		panic(fmt.Sprintf("block hash of block %q (height %d) is %v "+
			"instead of expected %v", g.tipName, g.tipHeight, hash,
			expected))
	}
}

// assertTipBlockMerkleRoot panics if the merkle root in header of the current
// tip block associated with the generator does not match the specified hash.
func (g *testGenerator) assertTipBlockMerkleRoot(expected chainhash.Hash) {
	hash := g.tip.Header.MerkleRoot
	if hash != expected {
		panic(fmt.Sprintf("merkle root of block %q (height %d) is %v "+
			"instead of expected %v", g.tipName, g.tipHeight, hash,
			expected))
	}
}

// assertTipBlockTxOutOpReturn panics if the current tip block associated with
// the generator does not have an OP_RETURN script for the transaction output at
// the provided tx index and output index.
func (g *testGenerator) assertTipBlockTxOutOpReturn(txIndex, txOutIndex uint32) {
	if txIndex >= uint32(len(g.tip.Transactions)) {
		panic(fmt.Sprintf("Transaction index %d in block %q "+
			"(height %d) does not exist", txIndex, g.tipName,
			g.tipHeight))
	}

	tx := g.tip.Transactions[txIndex]
	if txOutIndex >= uint32(len(tx.TxOut)) {
		panic(fmt.Sprintf("transaction index %d output %d in block %q "+
			"(height %d) does not exist", txIndex, txOutIndex,
			g.tipName, g.tipHeight))
	}

	txOut := tx.TxOut[txOutIndex]
	if txOut.PkScript[0] != txscript.OP_RETURN {
		panic(fmt.Sprintf("transaction index %d output %d in block %q "+
			"(height %d) is not an OP_RETURN", txIndex, txOutIndex,
			g.tipName, g.tipHeight))
	}
}

// Generate returns a slice of tests that can be used to exercise the consensus
// validation rules.  The tests are intended to be flexible enough to allow both
// unit-style tests directly against the blockchain code as well as integration
// style tests over the peer-to-peer network.  To achieve that goal, each test
// contains additional information about the expected result, however that
// information can be ignored when doing comparison tests between two
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
				err = errors.New("Unknown panic")
			}
		}
	}()

	// Create a test generator instance initialized with the genesis block
	// as the tip.
	g, err := makeTestGenerator(regressionNetParams)
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
		blockHeight := g.blockHeights[blockName]
		return AcceptedBlock{blockName, block, blockHeight, isMainChain,
			isOrphan}
	}
	rejectBlock := func(blockName string, block *wire.MsgBlock, code blockchain.ErrorCode) TestInstance {
		blockHeight := g.blockHeights[blockName]
		return RejectedBlock{blockName, block, blockHeight, code}
	}
	rejectNonCanonicalBlock := func(blockName string, block *wire.MsgBlock) TestInstance {
		blockHeight := g.blockHeights[blockName]
		encoded := encodeNonCanonicalBlock(block)
		return RejectedNonCanonicalBlock{blockName, encoded, blockHeight}
	}
	orphanOrRejectBlock := func(blockName string, block *wire.MsgBlock) TestInstance {
		blockHeight := g.blockHeights[blockName]
		return OrphanOrRejectedBlock{blockName, block, blockHeight}
	}
	expectTipBlock := func(blockName string, block *wire.MsgBlock) TestInstance {
		blockHeight := g.blockHeights[blockName]
		return ExpectedTip{blockName, block, blockHeight}
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
	// orphanedOrRejected creates and appends a single orphanOrRejectBlock
	// test instance for the current tip.
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
	rejected := func(code blockchain.ErrorCode) {
		tests = append(tests, []TestInstance{
			rejectBlock(g.tipName, g.tip, code),
		})
	}
	rejectedNonCanonical := func() {
		tests = append(tests, []TestInstance{
			rejectNonCanonicalBlock(g.tipName, g.tip),
		})
	}
	orphanedOrRejected := func() {
		tests = append(tests, []TestInstance{
			orphanOrRejectBlock(g.tipName, g.tip),
		})
	}

	// ---------------------------------------------------------------------
	// Generate enough blocks to have mature coinbase outputs to work with.
	//
	//   genesis -> bm0 -> bm1 -> ... -> bm99
	// ---------------------------------------------------------------------

	coinbaseMaturity := g.params.CoinbaseMaturity
	var testInstances []TestInstance
	for i := uint16(0); i < coinbaseMaturity; i++ {
		blockName := fmt.Sprintf("bm%d", i)
		g.nextBlock(blockName, nil)
		g.saveTipCoinbaseOut()
		testInstances = append(testInstances, acceptBlock(g.tipName,
			g.tip, true, false))
	}
	tests = append(tests, testInstances)

	// Collect spendable outputs.  This simplifies the code below.
	var outs []*spendableOut
	for i := uint16(0); i < coinbaseMaturity; i++ {
		op := g.oldestCoinbaseOut()
		outs = append(outs, &op)
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
	g.nextBlock("b1", outs[0])
	accepted()

	g.nextBlock("b2", outs[1])
	accepted()

	// Create a fork from b1.  There should not be a reorg since b2 was seen
	// first.
	//
	//   ... -> b1(0) -> b2(1)
	//               \-> b3(1)
	g.setTip("b1")
	g.nextBlock("b3", outs[1])
	b3Tx1Out := makeSpendableOut(g.tip, 1, 0)
	acceptedToSideChainWithExpectedTip("b2")

	// Extend b3 fork to make the alternative chain longer and force reorg.
	//
	//   ... -> b1(0) -> b2(1)
	//               \-> b3(1) -> b4(2)
	g.nextBlock("b4", outs[2])
	accepted()

	// Extend b2 fork twice to make first chain longer and force reorg.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//               \-> b3(1) -> b4(2)
	g.setTip("b2")
	g.nextBlock("b5", outs[2])
	acceptedToSideChainWithExpectedTip("b4")

	g.nextBlock("b6", outs[3])
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
	g.nextBlock("b7", outs[2])
	acceptedToSideChainWithExpectedTip("b6")

	g.nextBlock("b8", outs[4])
	rejected(blockchain.ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Too much proof-of-work coinbase tests.
	// ---------------------------------------------------------------------

	// Create a block that generates too coinbase.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//                                         \-> b9(4)
	//               \-> b3(1) -> b4(2)
	g.setTip("b6")
	g.nextBlock("b9", outs[4], additionalCoinbase(1))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create a fork that ends with block that generates too much coinbase.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//                                 \-> b10(3) -> b11(4)
	//               \-> b3(1) -> b4(2)
	g.setTip("b5")
	g.nextBlock("b10", outs[3])
	acceptedToSideChainWithExpectedTip("b6")

	g.nextBlock("b11", outs[4], additionalCoinbase(1))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create a fork that ends with block that generates too much coinbase
	// as before, but with a valid fork first.
	//
	//   ... -> b1(0) -> b2(1) -> b5(2) -> b6(3)
	//              |                  \-> b12(3) -> b13(4) -> b14(5)
	//              |                      (b12 added last)
	//               \-> b3(1) -> b4(2)
	g.setTip("b5")
	b12 := g.nextBlock("b12", outs[3])
	b13 := g.nextBlock("b13", outs[4])
	b14 := g.nextBlock("b14", outs[5], additionalCoinbase(1))
	tests = append(tests, []TestInstance{
		acceptBlock("b13", b13, false, true),
		acceptBlock("b14", b14, false, true),
		rejectBlock("b12", b12, blockchain.ErrBadCoinbaseValue),
		expectTipBlock("b13", b13),
	})

	// ---------------------------------------------------------------------
	// Checksig signature operation count tests.
	// ---------------------------------------------------------------------

	// Add a block with max allowed signature operations using OP_CHECKSIG.
	//
	//   ... -> b5(2) -> b12(3) -> b13(4) -> b15(5)
	//   \-> b3(1) -> b4(2)
	g.setTip("b13")
	manySigOps := repeatOpcode(txscript.OP_CHECKSIG, maxBlockSigOps)
	g.nextBlock("b15", outs[5], replaceSpendScript(manySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Attempt to add block with more than max allowed signature operations
	// using OP_CHECKSIG.
	//
	//   ... -> b5(2) -> b12(3) -> b13(4) -> b15(5)
	//   \                                         \-> b16(7)
	//    \-> b3(1) -> b4(2)
	tooManySigOps := repeatOpcode(txscript.OP_CHECKSIG, maxBlockSigOps+1)
	g.nextBlock("b16", outs[6], replaceSpendScript(tooManySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// ---------------------------------------------------------------------
	// Cross-fork spend tests.
	// ---------------------------------------------------------------------

	// Create block that spends a tx created on a different fork.
	//
	//   ... -> b5(2) -> b12(3) -> b13(4) -> b15(5)
	//   \                                         \-> b17(b3.tx[1])
	//    \-> b3(1) -> b4(2)
	g.setTip("b15")
	g.nextBlock("b17", &b3Tx1Out)
	rejected(blockchain.ErrMissingTxOut)

	// Create block that forks and spends a tx created on a third fork.
	//
	//   ... -> b5(2) -> b12(3) -> b13(4) -> b15(5)
	//   |                               \-> b18(b3.tx[1]) -> b19(6)
	//    \-> b3(1) -> b4(2)
	g.setTip("b13")
	g.nextBlock("b18", &b3Tx1Out)
	acceptedToSideChainWithExpectedTip("b15")

	g.nextBlock("b19", outs[6])
	rejected(blockchain.ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Immature coinbase tests.
	// ---------------------------------------------------------------------

	// Create block that spends immature coinbase.
	//
	//   ... -> b13(4) -> b15(5)
	//                          \-> b20(7)
	g.setTip("b15")
	g.nextBlock("b20", outs[7])
	rejected(blockchain.ErrImmatureSpend)

	// Create block that spends immature coinbase on a fork.
	//
	//   ... -> b13(4) -> b15(5)
	//                \-> b21(5) -> b22(7)
	g.setTip("b13")
	g.nextBlock("b21", outs[5])
	acceptedToSideChainWithExpectedTip("b15")

	g.nextBlock("b22", outs[7])
	rejected(blockchain.ErrImmatureSpend)

	// ---------------------------------------------------------------------
	// Max block size tests.
	// ---------------------------------------------------------------------

	// Create block that is the max allowed size.
	//
	//   ... -> b15(5) -> b23(6)
	g.setTip("b15")
	g.nextBlock("b23", outs[6], func(b *wire.MsgBlock) {
		bytesToMaxSize := maxBlockSize - b.SerializeSize() - 3
		sizePadScript := repeatOpcode(0x00, bytesToMaxSize)
		replaceSpendScript(sizePadScript)(b)
	})
	g.assertTipBlockSize(maxBlockSize)
	accepted()

	// Create block that is the one byte larger than max allowed size.  This
	// is done on a fork and should be rejected regardless.
	//
	//   ... -> b15(5) -> b23(6)
	//                \-> b24(6) -> b25(7)
	g.setTip("b15")
	g.nextBlock("b24", outs[6], func(b *wire.MsgBlock) {
		bytesToMaxSize := maxBlockSize - b.SerializeSize() - 3
		sizePadScript := repeatOpcode(0x00, bytesToMaxSize+1)
		replaceSpendScript(sizePadScript)(b)
	})
	g.assertTipBlockSize(maxBlockSize + 1)
	rejected(blockchain.ErrBlockTooBig)

	// Parent was rejected, so this block must either be an orphan or
	// outright rejected due to an invalid parent.
	g.nextBlock("b25", outs[7])
	orphanedOrRejected()

	// ---------------------------------------------------------------------
	// Coinbase script length limits tests.
	// ---------------------------------------------------------------------

	// Create block that has a coinbase script that is smaller than the
	// required length.  This is done on a fork and should be rejected
	// regardless.  Also, create a block that builds on the rejected block.
	//
	//   ... -> b15(5) -> b23(6)
	//                \-> b26(6) -> b27(7)
	g.setTip("b15")
	tooSmallCbScript := repeatOpcode(0x00, minCoinbaseScriptLen-1)
	g.nextBlock("b26", outs[6], replaceCoinbaseSigScript(tooSmallCbScript))
	rejected(blockchain.ErrBadCoinbaseScriptLen)

	// Parent was rejected, so this block must either be an orphan or
	// outright rejected due to an invalid parent.
	g.nextBlock("b27", outs[7])
	orphanedOrRejected()

	// Create block that has a coinbase script that is larger than the
	// allowed length.  This is done on a fork and should be rejected
	// regardless.  Also, create a block that builds on the rejected block.
	//
	//   ... -> b15(5) -> b23(6)
	//                \-> b28(6) -> b29(7)
	g.setTip("b15")
	tooLargeCbScript := repeatOpcode(0x00, maxCoinbaseScriptLen+1)
	g.nextBlock("b28", outs[6], replaceCoinbaseSigScript(tooLargeCbScript))
	rejected(blockchain.ErrBadCoinbaseScriptLen)

	// Parent was rejected, so this block must either be an orphan or
	// outright rejected due to an invalid parent.
	g.nextBlock("b29", outs[7])
	orphanedOrRejected()

	// Create block that has a max length coinbase script.
	//
	//   ... -> b23(6) -> b30(7)
	g.setTip("b23")
	maxSizeCbScript := repeatOpcode(0x00, maxCoinbaseScriptLen)
	g.nextBlock("b30", outs[7], replaceCoinbaseSigScript(maxSizeCbScript))
	accepted()

	// ---------------------------------------------------------------------
	// Multisig[Verify]/ChecksigVerifiy signature operation count tests.
	// ---------------------------------------------------------------------

	// Create block with max signature operations as OP_CHECKMULTISIG.
	//
	//   ... -> b30(7) -> b31(8)
	//
	// OP_CHECKMULTISIG counts for 20 sigops.
	manySigOps = repeatOpcode(txscript.OP_CHECKMULTISIG, maxBlockSigOps/20)
	g.nextBlock("b31", outs[8], replaceSpendScript(manySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with more than max allowed signature operations using
	// OP_CHECKMULTISIG.
	//
	//   ... -> b31(8)
	//                \-> b32(9)
	//
	// OP_CHECKMULTISIG counts for 20 sigops.
	tooManySigOps = repeatOpcode(txscript.OP_CHECKMULTISIG, maxBlockSigOps/20)
	tooManySigOps = append(manySigOps, txscript.OP_CHECKSIG)
	g.nextBlock("b32", outs[9], replaceSpendScript(tooManySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// Create block with max signature operations as OP_CHECKMULTISIGVERIFY.
	//
	//   ... -> b31(8) -> b33(9)
	g.setTip("b31")
	manySigOps = repeatOpcode(txscript.OP_CHECKMULTISIGVERIFY, maxBlockSigOps/20)
	g.nextBlock("b33", outs[9], replaceSpendScript(manySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with more than max allowed signature operations using
	// OP_CHECKMULTISIGVERIFY.
	//
	//   ... -> b33(9)
	//                \-> b34(10)
	//
	tooManySigOps = repeatOpcode(txscript.OP_CHECKMULTISIGVERIFY, maxBlockSigOps/20)
	tooManySigOps = append(manySigOps, txscript.OP_CHECKSIG)
	g.nextBlock("b34", outs[10], replaceSpendScript(tooManySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// Create block with max signature operations as OP_CHECKSIGVERIFY.
	//
	//   ... -> b33(9) -> b35(10)
	//
	g.setTip("b33")
	manySigOps = repeatOpcode(txscript.OP_CHECKSIGVERIFY, maxBlockSigOps)
	g.nextBlock("b35", outs[10], replaceSpendScript(manySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with more than max allowed signature operations using
	// OP_CHECKSIGVERIFY.
	//
	//   ... -> b35(10)
	//                 \-> b36(11)
	//
	tooManySigOps = repeatOpcode(txscript.OP_CHECKSIGVERIFY, maxBlockSigOps+1)
	g.nextBlock("b36", outs[11], replaceSpendScript(tooManySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// ---------------------------------------------------------------------
	// Spending of tx outputs in block that failed to connect tests.
	// ---------------------------------------------------------------------

	// Create block that spends a transaction from a block that failed to
	// connect (due to containing a double spend).
	//
	//   ... -> b35(10)
	//                 \-> b37(11)
	//                 \-> b38(b37.tx[1])
	//
	g.setTip("b35")
	doubleSpendTx := createSpendTx(outs[11], lowFee)
	g.nextBlock("b37", outs[11], additionalTx(doubleSpendTx))
	b37Tx1Out := makeSpendableOut(g.tip, 1, 0)
	rejected(blockchain.ErrMissingTxOut)

	g.setTip("b35")
	g.nextBlock("b38", &b37Tx1Out)
	rejected(blockchain.ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Pay-to-script-hash signature operation count tests.
	// ---------------------------------------------------------------------

	// Create a pay-to-script-hash redeem script that consists of 9
	// signature operations to be used in the next three blocks.
	const redeemScriptSigOps = 9
	redeemScript := pushDataScript(g.privKey.PubKey().SerializeCompressed())
	redeemScript = append(redeemScript, bytes.Repeat([]byte{txscript.OP_2DUP,
		txscript.OP_CHECKSIGVERIFY}, redeemScriptSigOps-1)...)
	redeemScript = append(redeemScript, txscript.OP_CHECKSIG)
	assertScriptSigOpsCount(redeemScript, redeemScriptSigOps)

	// Create a block that has enough pay-to-script-hash outputs such that
	// another block can be created that consumes them all and exceeds the
	// max allowed signature operations per block.
	//
	//   ... -> b35(10) -> b39(11)
	g.setTip("b35")
	b39 := g.nextBlock("b39", outs[11], func(b *wire.MsgBlock) {
		// Create a chain of transactions each spending from the
		// previous one such that each contains an output that pays to
		// the redeem script and the total number of signature
		// operations in those redeem scripts will be more than the
		// max allowed per block.
		p2shScript := payToScriptHashScript(redeemScript)
		txnsNeeded := (maxBlockSigOps / redeemScriptSigOps) + 1
		prevTx := b.Transactions[1]
		for i := 0; i < txnsNeeded; i++ {
			prevTx = createSpendTxForTx(prevTx, lowFee)
			prevTx.TxOut[0].Value -= 2
			prevTx.AddTxOut(wire.NewTxOut(2, p2shScript))
			b.AddTransaction(prevTx)
		}
	})
	g.assertTipBlockNumTxns((maxBlockSigOps / redeemScriptSigOps) + 3)
	accepted()

	// Create a block with more than max allowed signature operations where
	// the majority of them are in pay-to-script-hash scripts.
	//
	//   ... -> b35(10) -> b39(11)
	//                            \-> b40(12)
	g.setTip("b39")
	g.nextBlock("b40", outs[12], func(b *wire.MsgBlock) {
		txnsNeeded := (maxBlockSigOps / redeemScriptSigOps)
		for i := 0; i < txnsNeeded; i++ {
			// Create a signed transaction that spends from the
			// associated p2sh output in b39.
			spend := makeSpendableOutForTx(b39.Transactions[i+2], 2)
			tx := createSpendTx(&spend, lowFee)
			sig, err := txscript.RawTxInSignature(tx, 0,
				redeemScript, txscript.SigHashAll, g.privKey)
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
		finalTx := b.Transactions[len(b.Transactions)-1]
		tx := createSpendTxForTx(finalTx, lowFee)
		tx.TxOut[0].PkScript = repeatOpcode(txscript.OP_CHECKSIG, fill)
		b.AddTransaction(tx)
	})
	rejected(blockchain.ErrTooManySigOps)

	// Create a block with the max allowed signature operations where the
	// majority of them are in pay-to-script-hash scripts.
	//
	//   ... -> b35(10) -> b39(11) -> b41(12)
	g.setTip("b39")
	g.nextBlock("b41", outs[12], func(b *wire.MsgBlock) {
		txnsNeeded := (maxBlockSigOps / redeemScriptSigOps)
		for i := 0; i < txnsNeeded; i++ {
			spend := makeSpendableOutForTx(b39.Transactions[i+2], 2)
			tx := createSpendTx(&spend, lowFee)
			sig, err := txscript.RawTxInSignature(tx, 0,
				redeemScript, txscript.SigHashAll, g.privKey)
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
		finalTx := b.Transactions[len(b.Transactions)-1]
		tx := createSpendTxForTx(finalTx, lowFee)
		tx.TxOut[0].PkScript = repeatOpcode(txscript.OP_CHECKSIG, fill)
		b.AddTransaction(tx)
	})
	accepted()

	// ---------------------------------------------------------------------
	// Reset the chain to a stable base.
	//
	//   ... -> b35(10) -> b39(11) -> b42(12) -> b43(13)
	//                            \-> b41(12)
	// ---------------------------------------------------------------------

	g.setTip("b39")
	g.nextBlock("b42", outs[12])
	acceptedToSideChainWithExpectedTip("b41")

	g.nextBlock("b43", outs[13])
	accepted()

	// ---------------------------------------------------------------------
	// Various malformed block tests.
	// ---------------------------------------------------------------------

	// Create block with an otherwise valid transaction in place of where
	// the coinbase must be.
	//
	//   ... -> b43(13)
	//                 \-> b44(14)
	g.nextBlock("b44", nil, func(b *wire.MsgBlock) {
		nonCoinbaseTx := createSpendTx(outs[14], lowFee)
		b.Transactions[0] = nonCoinbaseTx
	})
	rejected(blockchain.ErrFirstTxNotCoinbase)

	// Create block with no transactions.
	//
	//   ... -> b43(13)
	//                 \-> b45(_)
	g.setTip("b43")
	g.nextBlock("b45", nil, func(b *wire.MsgBlock) {
		b.Transactions = nil
	})
	rejected(blockchain.ErrNoTransactions)

	// Create block with invalid proof of work.
	//
	//   ... -> b43(13)
	//                 \-> b46(14)
	g.setTip("b43")
	b46 := g.nextBlock("b46", outs[14])
	// This can't be done inside a munge function passed to nextBlock
	// because the block is solved after the function returns and this test
	// requires an unsolved block.
	{
		origHash := b46.BlockHash()
		for {
			// Keep incrementing the nonce until the hash treated as
			// a uint256 is higher than the limit.
			b46.Header.Nonce++
			blockHash := b46.BlockHash()
			hashNum := blockchain.HashToBig(&blockHash)
			if hashNum.Cmp(g.params.PowLimit) >= 0 {
				break
			}
		}
		g.updateBlockState("b46", origHash, "b46", b46)
	}
	rejected(blockchain.ErrHighHash)

	// Create block with a timestamp too far in the future.
	//
	//   ... -> b43(13)
	//                 \-> b47(14)
	g.setTip("b43")
	g.nextBlock("b47", outs[14], func(b *wire.MsgBlock) {
		// 3 hours in the future clamped to 1 second precision.
		nowPlus3Hours := time.Now().Add(time.Hour * 3)
		b.Header.Timestamp = time.Unix(nowPlus3Hours.Unix(), 0)
	})
	rejected(blockchain.ErrTimeTooNew)

	// Create block with an invalid merkle root.
	//
	//   ... -> b43(13)
	//                 \-> b48(14)
	g.setTip("b43")
	g.nextBlock("b48", outs[14], func(b *wire.MsgBlock) {
		b.Header.MerkleRoot = chainhash.Hash{}
	})
	rejected(blockchain.ErrBadMerkleRoot)

	// Create block with an invalid proof-of-work limit.
	//
	//   ... -> b43(13)
	//                 \-> b49(14)
	g.setTip("b43")
	g.nextBlock("b49", outs[14], func(b *wire.MsgBlock) {
		b.Header.Bits--
	})
	rejected(blockchain.ErrUnexpectedDifficulty)

	// Create block with an invalid negative proof-of-work limit.
	//
	//   ... -> b43(13)
	//                 \-> b49a(14)
	g.setTip("b43")
	b49a := g.nextBlock("b49a", outs[14])
	// This can't be done inside a munge function passed to nextBlock
	// because the block is solved after the function returns and this test
	// involves an unsolvable block.
	{
		origHash := b49a.BlockHash()
		b49a.Header.Bits = 0x01810000 // -1 in compact form.
		g.updateBlockState("b49a", origHash, "b49a", b49a)
	}
	rejected(blockchain.ErrUnexpectedDifficulty)

	// Create block with two coinbase transactions.
	//
	//   ... -> b43(13)
	//                 \-> b50(14)
	g.setTip("b43")
	coinbaseTx := g.createCoinbaseTx(g.tipHeight + 1)
	g.nextBlock("b50", outs[14], additionalTx(coinbaseTx))
	rejected(blockchain.ErrMultipleCoinbases)

	// Create block with duplicate transactions.
	//
	// This test relies on the shape of the shape of the merkle tree to test
	// the intended condition and thus is asserted below.
	//
	//   ... -> b43(13)
	//                 \-> b51(14)
	g.setTip("b43")
	g.nextBlock("b51", outs[14], func(b *wire.MsgBlock) {
		b.AddTransaction(b.Transactions[1])
	})
	g.assertTipBlockNumTxns(3)
	rejected(blockchain.ErrDuplicateTx)

	// Create a block that spends a transaction that does not exist.
	//
	//   ... -> b43(13)
	//                 \-> b52(14)
	g.setTip("b43")
	g.nextBlock("b52", outs[14], func(b *wire.MsgBlock) {
		hash := newHashFromStr("00000000000000000000000000000000" +
			"00000000000000000123456789abcdef")
		b.Transactions[1].TxIn[0].PreviousOutPoint.Hash = *hash
		b.Transactions[1].TxIn[0].PreviousOutPoint.Index = 0
	})
	rejected(blockchain.ErrMissingTxOut)

	// ---------------------------------------------------------------------
	// Block header median time tests.
	// ---------------------------------------------------------------------

	// Reset the chain to a stable base.
	//
	//   ... -> b33(9) -> b35(10) -> b39(11) -> b42(12) -> b43(13) -> b53(14)
	g.setTip("b43")
	g.nextBlock("b53", outs[14])
	accepted()

	// Create a block with a timestamp that is exactly the median time.  The
	// block must be rejected.
	//
	//   ... -> b33(9) -> b35(10) -> b39(11) -> b42(12) -> b43(13) -> b53(14)
	//                                                                       \-> b54(15)
	g.nextBlock("b54", outs[15], func(b *wire.MsgBlock) {
		medianBlock := g.blocks[b.Header.PrevBlock]
		for i := 0; i < medianTimeBlocks/2; i++ {
			medianBlock = g.blocks[medianBlock.Header.PrevBlock]
		}
		b.Header.Timestamp = medianBlock.Header.Timestamp
	})
	rejected(blockchain.ErrTimeTooOld)

	// Create a block with a timestamp that is one second after the median
	// time.  The block must be accepted.
	//
	//   ... -> b33(9) -> b35(10) -> b39(11) -> b42(12) -> b43(13) -> b53(14) -> b55(15)
	g.setTip("b53")
	g.nextBlock("b55", outs[15], func(b *wire.MsgBlock) {
		medianBlock := g.blocks[b.Header.PrevBlock]
		for i := 0; i < medianTimeBlocks/2; i++ {
			medianBlock = g.blocks[medianBlock.Header.PrevBlock]
		}
		medianBlockTime := medianBlock.Header.Timestamp
		b.Header.Timestamp = medianBlockTime.Add(time.Second)
	})
	accepted()

	// ---------------------------------------------------------------------
	// CVE-2012-2459 (block hash collision due to merkle tree algo) tests.
	// ---------------------------------------------------------------------

	// Create two blocks that have the same hash via merkle tree tricks to
	// ensure that the valid block is accepted even though it has the same
	// hash as the invalid block that was rejected first.
	//
	// This is accomplished by building the blocks as follows:
	//
	// b57 (valid block):
	//
	//                root = h1234 = h(h12 || h34)
	//	        //                           \\
	//	  h12 = h(h(cb) || h(tx2))  h34 = h(h(tx3) || h(tx3))
	//	   //                  \\             //           \\
	//	 coinbase              tx2           tx3           nil
	//
	//   transactions: coinbase, tx2, tx3
	//   merkle tree level 1: h12 = h(h(cb) || h(tx2))
	//                        h34 = h(h(tx3) || h(tx3)) // Algo reuses tx3
	//   merkle tree root: h(h12 || h34)
	//
	// b56 (invalid block with the same hash):
	//
	//                root = h1234 = h(h12 || h34)
	//	        //                          \\
	//	  h12 = h(h(cb) || h(tx2))  h34 = h(h(tx3) || h(tx3))
	//	   //                  \\             //           \\
	//	 coinbase              tx2           tx3           tx3
	//
	//   transactions: coinbase, tx2, tx3, tx3
	//   merkle tree level 1: h12 = h(h(cb) || h(tx2))
	//                        h34 = h(h(tx3) || h(tx3)) // real tx3 dup
	//   merkle tree root: h(h12 || h34)
	//
	//   ... -> b55(15) -> b57(16)
	//                 \-> b56(16)
	g.setTip("b55")
	b57 := g.nextBlock("b57", outs[16], func(b *wire.MsgBlock) {
		tx2 := b.Transactions[1]
		tx3 := createSpendTxForTx(tx2, lowFee)
		b.AddTransaction(tx3)
	})
	g.assertTipBlockNumTxns(3)

	g.setTip("b55")
	b56 := g.nextBlock("b56", nil, func(b *wire.MsgBlock) {
		*b = cloneBlock(b57)
		b.AddTransaction(b.Transactions[2])
	})
	g.assertTipBlockNumTxns(4)
	g.assertTipBlockHash(b57.BlockHash())
	g.assertTipBlockMerkleRoot(b57.Header.MerkleRoot)
	rejected(blockchain.ErrDuplicateTx)

	// Since the two blocks have the same hash and the generator state now
	// has b56 associated with the hash, manually remove b56, replace it
	// with b57, and then reset the tip to it.
	g.updateBlockState("b56", b56.BlockHash(), "b57", b57)
	g.setTip("b57")
	accepted()

	// Create a block that contains two duplicate txns that are not in a
	// consecutive position within the merkle tree.
	//
	// This is accomplished by building the block as follows:
	//
	//   transactions: coinbase, tx2, tx3, tx4, tx5, tx6, tx3, tx4
	//   merkle tree level 2: h12 = h(h(cb) || h(tx2))
	//                        h34 = h(h(tx3) || h(tx4))
	//                        h56 = h(h(tx5) || h(tx6))
	//                        h78 = h(h(tx3) || h(tx4)) // Same as h34
	//   merkle tree level 1: h1234 = h(h12 || h34)
	//                        h5678 = h(h56 || h78)
	//   merkle tree root: h(h1234 || h5678)
	//
	//
	//   ... -> b55(15) -> b57(16)
	//                 \-> b56p2(16)
	g.setTip("b55")
	g.nextBlock("b56p2", outs[16], func(b *wire.MsgBlock) {
		// Create 4 transactions that each spend from the previous tx
		// in the block.
		spendTx := b.Transactions[1]
		for i := 0; i < 4; i++ {
			spendTx = createSpendTxForTx(spendTx, lowFee)
			b.AddTransaction(spendTx)
		}

		// Add the duplicate transactions (3rd and 4th).
		b.AddTransaction(b.Transactions[2])
		b.AddTransaction(b.Transactions[3])
	})
	g.assertTipBlockNumTxns(8)
	rejected(blockchain.ErrDuplicateTx)

	// ---------------------------------------------------------------------
	// Invalid transaction type tests.
	// ---------------------------------------------------------------------

	// Create block with a transaction that tries to spend from an index
	// that is out of range from an otherwise valid and existing tx.
	//
	//   ... -> b57(16)
	//                 \-> b58(17)
	g.setTip("b57")
	g.nextBlock("b58", outs[17], func(b *wire.MsgBlock) {
		b.Transactions[1].TxIn[0].PreviousOutPoint.Index = 42
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block with transaction that pays more than its inputs.
	//
	//   ... -> b57(16)
	//                 \-> b59(17)
	g.setTip("b57")
	g.nextBlock("b59", outs[17], func(b *wire.MsgBlock) {
		b.Transactions[1].TxOut[0].Value = int64(outs[17].amount) + 1
	})
	rejected(blockchain.ErrSpendTooHigh)

	// ---------------------------------------------------------------------
	// BIP0030 tests.
	// ---------------------------------------------------------------------

	// Create a good block to reset the chain to a stable base.
	//
	//   ... -> b57(16) -> b60(17)
	g.setTip("b57")
	g.nextBlock("b60", outs[17])
	accepted()

	// Create block that has a tx with the same hash as an existing tx that
	// has not been fully spent.
	//
	//   ... -> b60(17)
	//                 \-> b61(18)
	g.nextBlock("b61", outs[18], func(b *wire.MsgBlock) {
		// Duplicate the coinbase of the parent block to force the
		// condition.
		parent := g.blocks[b.Header.PrevBlock]
		b.Transactions[0] = parent.Transactions[0]
	})
	rejected(blockchain.ErrOverwriteTx)

	// ---------------------------------------------------------------------
	// Blocks with non-final transaction tests.
	// ---------------------------------------------------------------------

	// Create block that contains a non-final non-coinbase transaction.
	//
	//   ... -> b60(17)
	//                 \-> b62(18)
	g.setTip("b60")
	g.nextBlock("b62", outs[18], func(b *wire.MsgBlock) {
		// A non-final transaction must have at least one input with a
		// non-final sequence number in addition to a non-final lock
		// time.
		b.Transactions[1].LockTime = 0xffffffff
		b.Transactions[1].TxIn[0].Sequence = 0
	})
	rejected(blockchain.ErrUnfinalizedTx)

	// Create block that contains a non-final coinbase transaction.
	//
	//   ... -> b60(17)
	//                 \-> b63(18)
	g.setTip("b60")
	g.nextBlock("b63", outs[18], func(b *wire.MsgBlock) {
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
	//   ... -> b60(17) -> b64(18)
	//                 \-> b64a(18)
	g.setTip("b60")
	b64a := g.nextBlock("b64a", outs[18], func(b *wire.MsgBlock) {
		bytesToMaxSize := maxBlockSize - b.SerializeSize() - 3
		sizePadScript := repeatOpcode(0x00, bytesToMaxSize)
		replaceSpendScript(sizePadScript)(b)
	})
	g.assertTipNonCanonicalBlockSize(maxBlockSize + 8)
	rejectedNonCanonical()

	g.setTip("b60")
	b64 := g.nextBlock("b64", outs[18], func(b *wire.MsgBlock) {
		*b = cloneBlock(b64a)
	})
	// Since the two blocks have the same hash and the generator state now
	// has b64a associated with the hash, manually remove b64a, replace it
	// with b64, and then reset the tip to it.
	g.updateBlockState("b64a", b64a.BlockHash(), "b64", b64)
	g.setTip("b64")
	g.assertTipBlockHash(b64a.BlockHash())
	g.assertTipBlockSize(maxBlockSize)
	accepted()

	// ---------------------------------------------------------------------
	// Same block transaction spend tests.
	// ---------------------------------------------------------------------

	// Create block that spends an output created earlier in the same block.
	//
	//   ... b64(18) -> b65(19)
	g.setTip("b64")
	g.nextBlock("b65", outs[19], func(b *wire.MsgBlock) {
		tx3 := createSpendTxForTx(b.Transactions[1], lowFee)
		b.AddTransaction(tx3)
	})
	accepted()

	// Create block that spends an output created later in the same block.
	//
	//   ... -> b65(19)
	//                 \-> b66(20)
	g.nextBlock("b66", nil, func(b *wire.MsgBlock) {
		tx2 := createSpendTx(outs[20], lowFee)
		tx3 := createSpendTxForTx(tx2, lowFee)
		b.AddTransaction(tx3)
		b.AddTransaction(tx2)
	})
	rejected(blockchain.ErrMissingTxOut)

	// Create block that double spends a transaction created in the same
	// block.
	//
	//   ... -> b65(19)
	//                 \-> b67(20)
	g.setTip("b65")
	g.nextBlock("b67", outs[20], func(b *wire.MsgBlock) {
		tx2 := b.Transactions[1]
		tx3 := createSpendTxForTx(tx2, lowFee)
		tx4 := createSpendTxForTx(tx2, lowFee)
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
	//   ... -> b65(19)
	//                 \-> b68(20)
	g.setTip("b65")
	g.nextBlock("b68", outs[20], additionalCoinbase(10), additionalSpendFee(9))
	rejected(blockchain.ErrBadCoinbaseValue)

	// Create block that pays 10 extra to the coinbase and a tx that pays
	// the extra 10 fee.
	//
	//   ... -> b65(19) -> b69(20)
	g.setTip("b65")
	g.nextBlock("b69", outs[20], additionalCoinbase(10), additionalSpendFee(10))
	accepted()

	// ---------------------------------------------------------------------
	// More signature operations counting tests.
	//
	// The next several tests ensure signature operations are counted before
	// script elements that cause parse failure while those after are
	// ignored and that signature operations after script elements that
	// successfully parse even if that element will fail at run-time are
	// counted.
	// ---------------------------------------------------------------------

	// Create block with more than max allowed signature operations such
	// that the signature operation that pushes it over the limit is after
	// a push data with a script element size that is larger than the max
	// allowed size when executed.  The block must be rejected because the
	// signature operation after the script element must be counted since
	// the script parses validly.
	//
	// The script generated consists of the following form:
	//
	//  Comment assumptions:
	//    maxBlockSigOps = 20000
	//    maxScriptElementSize = 520
	//
	//  [0-19999]    : OP_CHECKSIG
	//  [20000]      : OP_PUSHDATA4
	//  [20001-20004]: 521 (little-endian encoded maxScriptElementSize+1)
	//  [20005-20525]: too large script element
	//  [20526]      : OP_CHECKSIG (goes over the limit)
	//
	//   ... -> b69(20)
	//                 \-> b70(21)
	scriptSize := maxBlockSigOps + 5 + (maxScriptElementSize + 1) + 1
	tooManySigOps = repeatOpcode(txscript.OP_CHECKSIG, scriptSize)
	tooManySigOps[maxBlockSigOps] = txscript.OP_PUSHDATA4
	binary.LittleEndian.PutUint32(tooManySigOps[maxBlockSigOps+1:],
		maxScriptElementSize+1)
	g.nextBlock("b70", outs[21], replaceSpendScript(tooManySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// Create block with more than max allowed signature operations such
	// that the signature operation that pushes it over the limit is before
	// an invalid push data that claims a large amount of data even though
	// that much data is not provided.
	//
	//   ... -> b69(20)
	//                 \-> b71(21)
	g.setTip("b69")
	scriptSize = maxBlockSigOps + 5 + maxScriptElementSize + 1
	tooManySigOps = repeatOpcode(txscript.OP_CHECKSIG, scriptSize)
	tooManySigOps[maxBlockSigOps+1] = txscript.OP_PUSHDATA4
	binary.LittleEndian.PutUint32(tooManySigOps[maxBlockSigOps+2:], 0xffffffff)
	g.nextBlock("b71", outs[21], replaceSpendScript(tooManySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps + 1)
	rejected(blockchain.ErrTooManySigOps)

	// Create block with the max allowed signature operations such that all
	// counted signature operations are before an invalid push data that
	// claims a large amount of data even though that much data is not
	// provided.  The pushed data itself consists of OP_CHECKSIG so the
	// block would be rejected if any of them were counted.
	//
	//   ... -> b69(20) -> b72(21)
	g.setTip("b69")
	scriptSize = maxBlockSigOps + 5 + maxScriptElementSize
	manySigOps = repeatOpcode(txscript.OP_CHECKSIG, scriptSize)
	manySigOps[maxBlockSigOps] = txscript.OP_PUSHDATA4
	binary.LittleEndian.PutUint32(manySigOps[maxBlockSigOps+1:], 0xffffffff)
	g.nextBlock("b72", outs[21], replaceSpendScript(manySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// Create block with the max allowed signature operations such that all
	// counted signature operations are before an invalid push data that
	// contains OP_CHECKSIG in the number of bytes to push.  The block would
	// be rejected if any of them were counted.
	//
	//   ... -> b72(21) -> b73(22)
	scriptSize = maxBlockSigOps + 5 + (maxScriptElementSize + 1)
	manySigOps = repeatOpcode(txscript.OP_CHECKSIG, scriptSize)
	manySigOps[maxBlockSigOps] = txscript.OP_PUSHDATA4
	g.nextBlock("b73", outs[22], replaceSpendScript(manySigOps))
	g.assertTipBlockSigOpsCount(maxBlockSigOps)
	accepted()

	// ---------------------------------------------------------------------
	// Dead execution path tests.
	// ---------------------------------------------------------------------

	// Create block with an invalid opcode in a dead execution path.
	//
	//   ... -> b73(22) -> b74(23)
	script := []byte{txscript.OP_IF, txscript.OP_INVALIDOPCODE,
		txscript.OP_ELSE, txscript.OP_TRUE, txscript.OP_ENDIF}
	g.nextBlock("b74", outs[23], replaceSpendScript(script), func(b *wire.MsgBlock) {
		tx2 := b.Transactions[1]
		tx3 := createSpendTxForTx(tx2, lowFee)
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
	//   ... -> b74(23) -> b75(24)
	g.nextBlock("b75", outs[24], func(b *wire.MsgBlock) {
		// Add 4 outputs to the spending transaction that are spent
		// below.
		const numAdditionalOutputs = 4
		const zeroCoin = int64(0)
		spendTx := b.Transactions[1]
		for i := 0; i < numAdditionalOutputs; i++ {
			spendTx.AddTxOut(wire.NewTxOut(zeroCoin, opTrueScript))
		}

		// Add transactions spending from the outputs added above that
		// each contain an OP_RETURN output.
		//
		// NOTE: The createSpendTx func adds the OP_RETURN output.
		zeroFee := btcutil.Amount(0)
		for i := uint32(0); i < numAdditionalOutputs; i++ {
			spend := makeSpendableOut(b, 1, i+2)
			tx := createSpendTx(&spend, zeroFee)
			b.AddTransaction(tx)
		}
	})
	g.assertTipBlockNumTxns(6)
	g.assertTipBlockTxOutOpReturn(5, 1)
	b75OpReturnOut := makeSpendableOut(g.tip, 5, 1)
	accepted()

	// Reorg to a side chain that does not contain the OP_RETURNs.
	//
	//   ... -> b74(23) -> b75(24)
	//                 \-> b76(24) -> b77(25)
	g.setTip("b74")
	g.nextBlock("b76", outs[24])
	acceptedToSideChainWithExpectedTip("b75")

	g.nextBlock("b77", outs[25])
	accepted()

	// Reorg back to the original chain that contains the OP_RETURNs.
	//
	//   ... -> b74(23) -> b75(24) -> b78(25) -> b79(26)
	//                 \-> b76(24) -> b77(25)
	g.setTip("b75")
	g.nextBlock("b78", outs[25])
	acceptedToSideChainWithExpectedTip("b77")

	g.nextBlock("b79", outs[26])
	accepted()

	// Create a block that spends an OP_RETURN.
	//
	//   ... -> b74(23) -> b75(24) -> b78(25) -> b79(26)
	//                 \-> b76(24) -> b77(25)           \-> b80(b75.tx[5].out[1])
	//
	// An OP_RETURN output doesn't have any value and the default behavior
	// of nextBlock is to assign a fee of one, so increment the amount here
	// to effective negate that behavior.
	b75OpReturnOut.amount++
	g.nextBlock("b80", &b75OpReturnOut)
	rejected(blockchain.ErrMissingTxOut)

	// Create a block that has a transaction with multiple OP_RETURNs.  Even
	// though it's not considered a standard transaction, it is still valid
	// by the consensus rules.
	//
	//   ... -> b79(26) -> b81(27)
	//
	g.setTip("b79")
	g.nextBlock("b81", outs[27], func(b *wire.MsgBlock) {
		const numAdditionalOutputs = 4
		const zeroCoin = int64(0)
		spendTx := b.Transactions[1]
		for i := 0; i < numAdditionalOutputs; i++ {
			opRetScript := uniqueOpReturnScript()
			spendTx.AddTxOut(wire.NewTxOut(zeroCoin, opRetScript))
		}
	})
	for i := uint32(2); i < 6; i++ {
		g.assertTipBlockTxOutOpReturn(1, i)
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
	//   ... -> b81(27) -> ...
	g.setTip("b81")

	// Collect all of the spendable coinbase outputs from the previous
	// collection point up to the current tip.
	g.saveSpendableCoinbaseOuts()
	spendableOutOffset := g.tipHeight - int32(coinbaseMaturity)

	// Extend the main chain by a large number of max size blocks.
	//
	//   ... -> br0 -> br1 -> ... -> br#
	testInstances = nil
	reorgSpend := *outs[spendableOutOffset]
	reorgStartBlockName := g.tipName
	chain1TipName := g.tipName
	for i := int32(0); i < numLargeReorgBlocks; i++ {
		chain1TipName = fmt.Sprintf("br%d", i)
		g.nextBlock(chain1TipName, &reorgSpend, func(b *wire.MsgBlock) {
			bytesToMaxSize := maxBlockSize - b.SerializeSize() - 3
			sizePadScript := repeatOpcode(0x00, bytesToMaxSize)
			replaceSpendScript(sizePadScript)(b)
		})
		g.assertTipBlockSize(maxBlockSize)
		g.saveTipCoinbaseOut()
		testInstances = append(testInstances, acceptBlock(g.tipName,
			g.tip, true, false))

		// Use the next available spendable output.  First use up any
		// remaining spendable outputs that were already popped into the
		// outs slice, then just pop them from the stack.
		if spendableOutOffset+1+i < int32(len(outs)) {
			reorgSpend = *outs[spendableOutOffset+1+i]
		} else {
			reorgSpend = g.oldestCoinbaseOut()
		}
	}
	tests = append(tests, testInstances)

	// Create a side chain that has the same length.
	//
	//   ... -> br0    -> ... -> br#
	//      \-> bralt0 -> ... -> bralt#
	g.setTip(reorgStartBlockName)
	testInstances = nil
	chain2TipName := g.tipName
	for i := uint16(0); i < numLargeReorgBlocks; i++ {
		chain2TipName = fmt.Sprintf("bralt%d", i)
		g.nextBlock(chain2TipName, nil)
		testInstances = append(testInstances, acceptBlock(g.tipName,
			g.tip, false, false))
	}
	testInstances = append(testInstances, expectTipBlock(chain1TipName,
		g.blocksByName[chain1TipName]))
	tests = append(tests, testInstances)

	// Extend the side chain by one to force the large reorg.
	//
	//   ... -> bralt0 -> ... -> bralt# -> bralt#+1
	//      \-> br0    -> ... -> br#
	g.nextBlock(fmt.Sprintf("bralt%d", g.tipHeight+1), nil)
	chain2TipName = g.tipName
	accepted()

	// Extend the first chain by two to force a large reorg back to it.
	//
	//   ... -> br0    -> ... -> br#    -> br#+1    -> br#+2
	//      \-> bralt0 -> ... -> bralt# -> bralt#+1
	g.setTip(chain1TipName)
	g.nextBlock(fmt.Sprintf("br%d", g.tipHeight+1), nil)
	chain1TipName = g.tipName
	acceptedToSideChainWithExpectedTip(chain2TipName)

	g.nextBlock(fmt.Sprintf("br%d", g.tipHeight+2), nil)
	chain1TipName = g.tipName
	accepted()

	return tests, nil
}
