// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"bytes"
	"compress/bzip2"
	"encoding/binary"
	"encoding/gob"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain"
	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

// recalculateMsgBlockMerkleRootsSize recalculates the merkle roots for a msgBlock,
// then stores them in the msgBlock's header. It also updates the block size.
func recalculateMsgBlockMerkleRootsSize(msgBlock *wire.MsgBlock) {
	tempBlock := dcrutil.NewBlock(msgBlock)

	merkles := blockchain.BuildMerkleTreeStore(tempBlock.Transactions())
	merklesStake := blockchain.BuildMerkleTreeStore(tempBlock.STransactions())

	msgBlock.Header.MerkleRoot = *merkles[len(merkles)-1]
	msgBlock.Header.StakeRoot = *merklesStake[len(merklesStake)-1]
	msgBlock.Header.Size = uint32(msgBlock.SerializeSize())
}

// updateVoteCommitments updates all of the votes in the passed block to commit
// to the previous block and height specified by the header.
func updateVoteCommitments(msgBlock *wire.MsgBlock) {
	for _, stx := range msgBlock.STransactions {
		if !stake.IsSSGen(stx) {
			continue
		}

		// Generate and set the commitment.
		var commitment [36]byte
		copy(commitment[:], msgBlock.Header.PrevBlock[:])
		binary.LittleEndian.PutUint32(commitment[32:], msgBlock.Header.Height-1)
		pkScript, _ := txscript.GenerateProvablyPruneableOut(commitment[:])
		stx.TxOut[0].PkScript = pkScript
	}
}

// TestBlockValidationRules unit tests various block validation rules.
// It checks the following:
// 1. ProcessBlock
// 2. CheckWorklessBlockSanity
// 3. CheckConnectBlock (checkBlockSanity and checkBlockContext as well)
//
// The tests are done with a pregenerated simnet blockchain with two wallets
// running on it:
//
// 1: erase exodus rhythm paragraph cleanup company quiver opulent crusade
// Ohio merit recipe spheroid Pandora stairway disbelief framework component
// newborn monument tumor supportive wallet sensation standard frequency accrue
// customer stapler Burlington klaxon Medusa retouch
//
// 2: indulge hazardous bombast tobacco tunnel Pandora hockey whimsical choking
// Wilmington jawbone revival beaming Capricorn gazelle armistice beaming company
// scenic pedigree quadrant hamburger Algol Yucatan erase impetus seabird
// hemisphere drunken vacancy uncut caretaker Dupont
func TestBlockValidationRules(t *testing.T) {
	// Update simnet parameters to reflect what is expected by the legacy
	// data.
	params := cloneParams(&chaincfg.SimNetParams)
	params.GenesisBlock.Header.MerkleRoot = *mustParseHash("a216ea043f0d481a072424af646787794c32bcefd3ed181a090319bbf8a37105")
	genesisHash := params.GenesisBlock.BlockHash()
	params.GenesisHash = &genesisHash

	// Create a new database and chain instance to run tests against.
	chain, teardownFunc, err := chainSetup("validateunittests", params)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// The genesis block should fail to connect since it's already inserted.
	err = chain.CheckConnectBlock(dcrutil.NewBlock(params.GenesisBlock),
		blockchain.BFNone)
	if err == nil {
		t.Errorf("CheckConnectBlock: Did not receive expected error")
	}

	// Load up the rest of the blocks up to HEAD~1.
	filename := filepath.Join("testdata/", "blocks0to168.bz2")
	fi, err := os.Open(filename)
	if err != nil {
		t.Errorf("Failed to open %s: %v", filename, err)
	}
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder := gob.NewDecoder(bcBuf)
	blockChain := make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	// Insert blocks 1 to 142 and perform various test. Block 1 has
	// special properties, so make sure those validate correctly first.
	block1Bytes := blockChain[int64(1)]
	timeSource := blockchain.NewMedianTime()

	// ----------------------------------------------------------------------------
	// ErrBlockOneOutputs 1
	// No coinbase outputs check can't trigger because it throws an error
	// elsewhere.

	// ----------------------------------------------------------------------------
	// ErrBlockOneOutputs 2
	// Remove one of the premine outputs and make sure it fails.
	noCoinbaseOuts1 := new(wire.MsgBlock)
	noCoinbaseOuts1.FromBytes(block1Bytes)
	noCoinbaseOuts1.Transactions[0].TxOut =
		noCoinbaseOuts1.Transactions[0].TxOut[:2]

	recalculateMsgBlockMerkleRootsSize(noCoinbaseOuts1)
	b1test := dcrutil.NewBlock(noCoinbaseOuts1)

	err = blockchain.CheckWorklessBlockSanity(b1test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected error for ErrBlockOneOutputs test 2: %v", err)
	}

	err = chain.CheckConnectBlock(b1test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBlockOneOutputs {
		t.Errorf("Got no error or unexpected error for ErrBlockOneOutputs "+
			"test 2: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrBlockOneOutputs 3
	// Bad pay to hash.
	noCoinbaseOuts1 = new(wire.MsgBlock)
	noCoinbaseOuts1.FromBytes(block1Bytes)
	noCoinbaseOuts1.Transactions[0].TxOut[0].PkScript[8] ^= 0x01

	recalculateMsgBlockMerkleRootsSize(noCoinbaseOuts1)
	b1test = dcrutil.NewBlock(noCoinbaseOuts1)

	err = blockchain.CheckWorklessBlockSanity(b1test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected error for ErrBlockOneOutputs test 3: %v", err)
	}

	err = chain.CheckConnectBlock(b1test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBlockOneOutputs {
		t.Errorf("Got no error or unexpected error for ErrBlockOneOutputs "+
			"test 3: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrBlockOneOutputs 4
	// Bad pay to amount.
	noCoinbaseOuts1 = new(wire.MsgBlock)
	noCoinbaseOuts1.FromBytes(block1Bytes)
	noCoinbaseOuts1.Transactions[0].TxOut[0].Value--

	recalculateMsgBlockMerkleRootsSize(noCoinbaseOuts1)
	b1test = dcrutil.NewBlock(noCoinbaseOuts1)

	err = blockchain.CheckWorklessBlockSanity(b1test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected error for ErrBlockOneOutputs test 4: %v", err)
	}

	err = chain.CheckConnectBlock(b1test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBlockOneOutputs {
		t.Errorf("Got no error or unexpected error for ErrBlockOneOutputs "+
			"test 4: %v", err)
	}

	// ----------------------------------------------------------------------------
	// Add the rest of the blocks up to the stake early test block.
	stakeEarlyTest := 142
	for i := 1; i < stakeEarlyTest; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Errorf("NewBlockFromBytes error: %v", err.Error())
		}

		_, _, err = chain.ProcessBlock(bl, blockchain.BFNoPoWCheck)
		if err != nil {
			t.Fatalf("ProcessBlock error at height %v: %v", i, err.Error())
		}
	}

	// ----------------------------------------------------------------------------
	// ErrInvalidEarlyStakeTx
	// There are multiple paths to this error, but here we try an early SSGen.
	block142Bytes := blockChain[int64(stakeEarlyTest)]
	earlySSGen142 := new(wire.MsgBlock)
	earlySSGen142.FromBytes(block142Bytes)

	ssgenTx, _ := hex.DecodeString("01000000020000000000000000000000000000000" +
		"000000000000000000000000000000000ffffffff00ffffffff228896eaaeba3e57d" +
		"5e2b7d9e38ed1ce9918db5dd8159620f34c0f1fff860f590000000001ffffffff030" +
		"0000000000000000000266a2494f455952f88b4ff019c6673f3f01541b76e700e0c8" +
		"a2ab9da000000000000009f86010000000000000000000000046a02010066cf98a80" +
		"000000000001abb76a914efb4777e0c06c73883d6104a49739302db8058af88ac000" +
		"00000000000000221b683090000000000000000ffffffff04deadbeef4519159f000" +
		"000008e6b0100160000006a47304402202cc7a2938643d2e42e4363faa948bd19bfa" +
		"d7a4f46ad040c34af9cb5d71b23d20220268189f6fd7ed2a0be573293e8814daac53" +
		"01a01787c8d8d346c8b681ed91d4f0121031d90b4e6394841780c7292e385d7161cc" +
		"3cacc46c894cbc7b82a2fbcae12e773")
	mtxFromB := new(wire.MsgTx)
	mtxFromB.FromBytes(ssgenTx)
	earlySSGen142.AddSTransaction(mtxFromB)
	earlySSGen142.Header.Voters++
	recalculateMsgBlockMerkleRootsSize(earlySSGen142)
	b142test := dcrutil.NewBlock(earlySSGen142)

	err = blockchain.CheckWorklessBlockSanity(b142test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrInvalidEarlyStakeTx {
		t.Errorf("Got unexpected no error or wrong error for "+
			"ErrInvalidEarlyStakeTx test: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrInvalidEarlyVoteBits
	earlyBadVoteBits42 := new(wire.MsgBlock)
	earlyBadVoteBits42.FromBytes(block142Bytes)
	earlyBadVoteBits42.Header.VoteBits ^= 0x80
	b142test = dcrutil.NewBlock(earlyBadVoteBits42)

	err = blockchain.CheckWorklessBlockSanity(b142test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrInvalidEarlyVoteBits {
		t.Errorf("Got unexpected no error or wrong error for "+
			"ErrInvalidEarlyVoteBits test: %v", err)
	}

	err = chain.CheckConnectBlock(b142test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrInvalidEarlyVoteBits {
		t.Errorf("Got unexpected no error or wrong error for "+
			"ErrInvalidEarlyVoteBits test: %v", err)
	}

	// ----------------------------------------------------------------------------
	// Add blocks up to the first stage of testing.
	testsIdx1 := 153
	testsIdx2 := 154
	testsIdx3 := 166
	for i := stakeEarlyTest; i < testsIdx1; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Errorf("NewBlockFromBytes error: %v", err.Error())
		}

		_, _, err = chain.ProcessBlock(bl, blockchain.BFNoPoWCheck)
		if err != nil {
			t.Errorf("ProcessBlock error at height %v: %v", i,
				err.Error())
		}
	}

	// Make sure the last block validates.
	block153, err := dcrutil.NewBlockFromBytes(blockChain[int64(testsIdx1)])
	if err != nil {
		t.Errorf("NewBlockFromBytes error: %v", err.Error())
	}
	err = chain.CheckConnectBlock(block153, blockchain.BFNoPoWCheck)
	if err != nil {
		t.Errorf("CheckConnectBlock error: %v", err.Error())
	}
	block153Bytes := blockChain[int64(testsIdx1)]

	// ----------------------------------------------------------------------------
	// ErrBadMerkleRoot 1
	// Corrupt the merkle root in tx tree regular
	badMerkleRoot153 := new(wire.MsgBlock)
	badMerkleRoot153.FromBytes(block153Bytes)
	badMerkleRoot153.Header.MerkleRoot[0] ^= 0x01
	b153test := dcrutil.NewBlock(badMerkleRoot153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadMerkleRoot {
		t.Errorf("Failed to get error or correct error for ErrBadMerkleRoot 1"+
			"test (err: %v)", err)
	}

	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadMerkleRoot {
		t.Errorf("Failed to get error or correct error for ErrBadMerkleRoot 1"+
			"test (err: %v)", err)
	}

	// ----------------------------------------------------------------------------
	// ErrBadMerkleRoot 2
	// Corrupt the merkle root in tx tree stake
	badMerkleRoot153 = new(wire.MsgBlock)
	badMerkleRoot153.FromBytes(block153Bytes)
	badMerkleRoot153.Header.StakeRoot[0] ^= 0x01
	b153test = dcrutil.NewBlock(badMerkleRoot153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadMerkleRoot {
		t.Errorf("Failed to get error or correct error for ErrBadMerkleRoot 2"+
			"test (err: %v)", err)
	}

	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadMerkleRoot {
		t.Errorf("Failed to get error or correct error for ErrBadMerkleRoot 2"+
			"test (err: %v)", err)
	}

	// ----------------------------------------------------------------------------
	// ErrUnexpectedDifficulty
	badDifficulty153 := new(wire.MsgBlock)
	badDifficulty153.FromBytes(block153Bytes)
	badDifficulty153.Header.Bits = 0x207ffffe
	b153test = dcrutil.NewBlock(badDifficulty153)

	_, _, err = chain.ProcessBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrUnexpectedDifficulty {
		t.Errorf("Failed to get error or correct error for "+
			"ErrUnexpectedDifficulty test (err: %v)", err)
	}

	// ----------------------------------------------------------------------------
	// ErrWrongBlockSize
	badBlockSize153 := new(wire.MsgBlock)
	badBlockSize153.FromBytes(block153Bytes)
	badBlockSize153.Header.Size = 0x20ffff71
	b153test = dcrutil.NewBlock(badBlockSize153)

	_, _, err = chain.ProcessBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrWrongBlockSize {
		t.Errorf("Failed to get error or correct error for "+
			"ErrWrongBlockSize test (err: %v)", err)
	}

	// ----------------------------------------------------------------------------
	// ErrHighHash
	badHash153 := new(wire.MsgBlock)
	badHash153.FromBytes(block153Bytes)
	badHash153.Header.Size = 0x20ffff70
	b153test = dcrutil.NewBlock(badHash153)

	_, _, err = chain.ProcessBlock(b153test, blockchain.BFNone)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrHighHash {
		t.Errorf("Failed to get error or correct error for "+
			"ErrHighHash test (err: %v)", err)
	}

	// ----------------------------------------------------------------------------
	// ErrMissingParent
	missingParent153 := new(wire.MsgBlock)
	missingParent153.FromBytes(block153Bytes)
	missingParent153.Header.PrevBlock[8] ^= 0x01
	updateVoteCommitments(missingParent153)
	recalculateMsgBlockMerkleRootsSize(missingParent153)
	b153test = dcrutil.NewBlock(missingParent153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected sanity error for ErrMissingParent test: %v",
			err)
	}

	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrMissingParent {
		t.Errorf("Got no or unexpected error for ErrMissingParent test %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrBadCoinbaseValue
	badSubsidy153 := new(wire.MsgBlock)
	badSubsidy153.FromBytes(block153Bytes)
	badSubsidy153.Transactions[0].TxOut[2].Value++
	recalculateMsgBlockMerkleRootsSize(badSubsidy153)
	b153test = dcrutil.NewBlock(badSubsidy153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected sanity error for ErrBadCoinbaseValue test: %v",
			err)
	}

	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadCoinbaseValue {
		t.Errorf("Got no or unexpected error for ErrBadCoinbaseValue test %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrBadCoinbaseOutpoint/ErrFirstTxNotCoinbase
	// Seems impossible to hit this because ErrFirstTxNotCoinbase is hit first.
	badCBOutpoint153 := new(wire.MsgBlock)
	badCBOutpoint153.FromBytes(block153Bytes)
	badCBOutpoint153.Transactions[0].TxIn[0].PreviousOutPoint.Hash[0] ^= 0x01
	recalculateMsgBlockMerkleRootsSize(badCBOutpoint153)
	b153test = dcrutil.NewBlock(badCBOutpoint153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrFirstTxNotCoinbase {
		t.Errorf("Got no or unexpected sanity error for "+
			"ErrBadCoinbaseOutpoint test: %v", err)
	}

	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil {
		t.Errorf("Got unexpected no error for ErrBadCoinbaseOutpoint test")
	}

	// ----------------------------------------------------------------------------
	// ErrBadCoinbaseFraudProof
	badCBFraudProof153 := new(wire.MsgBlock)
	badCBFraudProof153.FromBytes(block153Bytes)
	badCBFraudProof153.Transactions[0].TxIn[0].BlockHeight = 0x12345678
	recalculateMsgBlockMerkleRootsSize(badCBFraudProof153)
	b153test = dcrutil.NewBlock(badCBFraudProof153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadCoinbaseFraudProof {
		t.Errorf("Got no or unexpected sanity error for "+
			"ErrBadCoinbaseFraudProof test: %v", err)
	}

	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadCoinbaseFraudProof {
		t.Errorf("Got no or unexpected sanity error for "+
			"ErrBadCoinbaseFraudProof test: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrBadCoinbaseAmountIn
	badCBAmountIn153 := new(wire.MsgBlock)
	badCBAmountIn153.FromBytes(block153Bytes)
	badCBAmountIn153.Transactions[0].TxIn[0].ValueIn = 0x1234567890123456
	recalculateMsgBlockMerkleRootsSize(badCBAmountIn153)
	b153test = dcrutil.NewBlock(badCBAmountIn153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected error for ErrBadCoinbaseFraudProof test: %v",
			err)
	}

	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadCoinbaseAmountIn {
		t.Errorf("Got no or unexpected sanity error for "+
			"ErrBadCoinbaseAmountIn test: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrBadStakebaseAmountIn
	badSBAmountIn153 := new(wire.MsgBlock)
	badSBAmountIn153.FromBytes(block153Bytes)
	badSBAmountIn153.STransactions[0].TxIn[0].ValueIn = 0x1234567890123456
	recalculateMsgBlockMerkleRootsSize(badSBAmountIn153)
	b153test = dcrutil.NewBlock(badSBAmountIn153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected error for ErrBadCoinbaseFraudProof test: %v",
			err)
	}

	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadStakebaseAmountIn {
		t.Errorf("Got no or unexpected sanity error for "+
			"ErrBadCoinbaseAmountIn test: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrRegTxInStakeTree
	// Break an SSGen by giving it a non-null outpoint.
	badStakebaseOutpoint153 := new(wire.MsgBlock)
	badStakebaseOutpoint153.FromBytes(block153Bytes)
	badOPHash, _ := chainhash.NewHash(bytes.Repeat([]byte{0x01}, 32))
	badStakebaseOutpoint153.STransactions[0].TxIn[0].PreviousOutPoint.Hash =
		*badOPHash

	recalculateMsgBlockMerkleRootsSize(badStakebaseOutpoint153)
	badStakebaseOutpoint153.Header.Voters--
	b153test = dcrutil.NewBlock(badStakebaseOutpoint153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrRegTxInStakeTree {
		t.Errorf("Failed to get error or correct error for ErrRegTxInStakeTree "+
			"test (err: %v)", err)
	}

	// It hits another error on checkConnectBlock.
	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil {
		t.Errorf("Got unexpected no error for ErrRegTxInStakeTree test")
	}

	// ----------------------------------------------------------------------------
	// ErrStakeTxInRegularTree
	// Stick an SSGen in TxTreeRegular.
	ssgenInRegular153 := new(wire.MsgBlock)
	ssgenInRegular153.FromBytes(block153Bytes)
	ssgenInRegular153.AddTransaction(ssgenInRegular153.STransactions[4])
	ssgenInRegular153.STransactions = ssgenInRegular153.STransactions[0:3]
	ssgenInRegular153.Header.Voters -= 2

	recalculateMsgBlockMerkleRootsSize(ssgenInRegular153)
	b153test = dcrutil.NewBlock(ssgenInRegular153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrStakeTxInRegularTree {
		t.Errorf("Failed to get error or correct error for ErrRegTxInStakeTree "+
			"test (err: %v)", err)
	}

	// Throws bad subsidy error too.
	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil {
		t.Errorf("Got unexpected no error for ErrStakeTxInRegularTree test")
	}

	// ----------------------------------------------------------------------------
	// ErrBadStakebaseScriptLen
	badStakebaseSS153 := new(wire.MsgBlock)
	badStakebaseSS153.FromBytes(block153Bytes)
	badStakebaseSS := bytes.Repeat([]byte{0x01}, 256)
	badStakebaseSS153.STransactions[0].TxIn[0].SignatureScript =
		badStakebaseSS
	recalculateMsgBlockMerkleRootsSize(badStakebaseSS153)
	b153test = dcrutil.NewBlock(badStakebaseSS153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadStakebaseScriptLen {
		t.Errorf("Failed to get error or correct error for bad stakebase "+
			"script len test (err: %v)", err)
	}

	// This otherwise passes the checks.
	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadStakebaseScriptLen {
		t.Errorf("Failed to get error or correct error for bad stakebase "+
			"script len test (err: %v)", err)
	}

	// ----------------------------------------------------------------------------
	// ErrBadStakebaseScrVal
	badStakebaseScr153 := new(wire.MsgBlock)
	badStakebaseScr153.FromBytes(block153Bytes)
	badStakebaseScr153.STransactions[0].TxIn[0].SignatureScript[0] ^= 0x01
	recalculateMsgBlockMerkleRootsSize(badStakebaseScr153)
	b153test = dcrutil.NewBlock(badStakebaseScr153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadStakebaseScrVal {
		t.Errorf("Failed to get error or correct error for bad stakebase "+
			"script test (err: %v)", err)
	}

	// This otherwise passes the checks.
	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadStakebaseScrVal {
		t.Errorf("Failed to get error or correct error for bad stakebase "+
			"script test (err: %v)", err)
	}

	// ----------------------------------------------------------------------------
	// ErrInvalidRevocations
	badSSRtxNum153 := new(wire.MsgBlock)
	badSSRtxNum153.FromBytes(block153Bytes)
	badSSRtxNum153.Header.Revocations = 2

	b153test = dcrutil.NewBlock(badSSRtxNum153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrRevocationsMismatch {
		t.Errorf("got unexpected no error or other error for "+
			"ErrInvalidRevocations sanity check: %v", err)
	}

	// Fails and hits ErrInvalidRevocations.
	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrRevocationsMismatch {
		t.Errorf("got unexpected no error or other error for "+
			"ErrInvalidRevocations sanity check: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrSSRtxPayeesMismatch
	// Add an extra txout to the revocation.
	ssrtxPayeesMismatch153 := new(wire.MsgBlock)
	ssrtxPayeesMismatch153.FromBytes(block153Bytes)
	ssrtxPayeesMismatch153.STransactions[5].TxOut = append(
		ssrtxPayeesMismatch153.STransactions[5].TxOut,
		ssrtxPayeesMismatch153.STransactions[5].TxOut[0])

	recalculateMsgBlockMerkleRootsSize(ssrtxPayeesMismatch153)
	b153test = dcrutil.NewBlock(ssrtxPayeesMismatch153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrSSRtxPayeesMismatch sanity  "+
			"check: %v", err)
	}

	// Fails and hits ErrSSRtxPayeesMismatch.
	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrSSRtxPayeesMismatch {
		t.Errorf("Unexpected no or wrong error for ErrSSRtxPayeesMismatch "+
			"test: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrSSRtxPayees 1
	// Corrupt the PKH it pays out to.
	badSSRtxPayee153 := new(wire.MsgBlock)
	badSSRtxPayee153.FromBytes(block153Bytes)
	badSSRtxPayee153.STransactions[5].TxOut[0].PkScript[8] ^= 0x01

	recalculateMsgBlockMerkleRootsSize(badSSRtxPayee153)
	b153test = dcrutil.NewBlock(badSSRtxPayee153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrSSRtxPayees sanity  "+
			"check 1: %v", err)
	}

	// Fails and hits ErrSSRtxPayees.
	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrSSRtxPayees {
		t.Errorf("Unexpected no or wrong error for ErrSSRtxPayees "+
			"test 1: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrSSRtxPayees 2
	// Corrupt the amount. The transaction can pay (0 ... 20000) and still be
	// valid because with the sstxOut.Version set to 0x5400 we can have fees up
	// to 2^20 for any SSRtx output.
	badSSRtxPayee153 = new(wire.MsgBlock)
	badSSRtxPayee153.FromBytes(block153Bytes)
	badSSRtxPayee153.STransactions[5].TxOut[0].Value = 20001

	recalculateMsgBlockMerkleRootsSize(badSSRtxPayee153)
	b153test = dcrutil.NewBlock(badSSRtxPayee153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrSSRtxPayees sanity "+
			"check 2: %v", err)
	}

	// Fails and hits ErrSSRtxPayees.
	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrSSRtxPayees {
		t.Errorf("Unexpected no or wrong error for ErrSSRtxPayees "+
			"test 2: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrInvalidSSRtx
	invalidSSRtxFor153, _ := hex.DecodeString("0100000001e081ca7481ed46de39e528" +
		"8a45b6a3f86c478a6ebc60a4b701c75c1bc900ea8a0000000001ffffffff01db040100" +
		"0000000000001abc76a914a495e69ddfe8b9770b823314ba66d4ca0620131088ac0000" +
		"00000000000001542c79000000000076000000010000006b483045022100d5b06e2f35" +
		"b73eeed8331a482c0b45ab3dc1bd98574ae79afbb80853bdac4735022012ea4ce6177c" +
		"76e4d7e9aca0d06978cdbcbed163a89d7fffa5297968227914e90121033147afc0d065" +
		"9798f602c92aef634aaffc0a82759b9d0654a5d04c28f3451f76")
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(invalidSSRtxFor153)

	badSSRtx153 := new(wire.MsgBlock)
	badSSRtx153.FromBytes(block153Bytes)
	badSSRtx153.AddSTransaction(mtxFromB)
	badSSRtx153.Header.Revocations = 2

	recalculateMsgBlockMerkleRootsSize(badSSRtx153)
	b153test = dcrutil.NewBlock(badSSRtx153)

	err = blockchain.CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrInvalidSSRtx sanity check: %v",
			err)
	}

	// Fails and hits ErrInvalidSSRtx.
	err = chain.CheckConnectBlock(b153test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrInvalidSSRtx {
		t.Errorf("Unexpected no or wrong error for ErrInvalidSSRtx test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// Insert block 154 and continue testing
	block153MsgBlock := new(wire.MsgBlock)
	block153MsgBlock.FromBytes(block153Bytes)
	b153test = dcrutil.NewBlock(block153MsgBlock)
	_, _, err = chain.ProcessBlock(b153test, blockchain.BFNoPoWCheck)
	if err != nil {
		t.Errorf("Got unexpected error processing block 153 %v", err)
	}
	block154Bytes := blockChain[int64(testsIdx2)]
	block154MsgBlock := new(wire.MsgBlock)
	block154MsgBlock.FromBytes(block154Bytes)
	b154test := dcrutil.NewBlock(block154MsgBlock)

	// The incoming block should pass fine.
	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("Unexpected error for check block 154 sanity: %v", err.Error())
	}

	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err != nil {
		t.Errorf("Unexpected error for check block 154 connect: %v", err.Error())
	}

	// ----------------------------------------------------------------------------
	// ErrNotEnoughStake
	notEnoughStake154 := new(wire.MsgBlock)
	notEnoughStake154.FromBytes(block154Bytes)
	notEnoughStake154.STransactions[5].TxOut[0].Value--
	notEnoughStake154.AddSTransaction(mtxFromB)
	recalculateMsgBlockMerkleRootsSize(notEnoughStake154)
	b154test = dcrutil.NewBlock(notEnoughStake154)

	// This fails both checks.
	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrNotEnoughStake {
		t.Errorf("Failed to get error or correct error for low stake amt "+
			"test (err: %v)", err)
	}

	// Throws an error in stake consensus.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil {
		t.Errorf("Unexpected error for low stake amt test: %v", err.Error())
	}

	// ----------------------------------------------------------------------------
	// ErrFreshStakeMismatch
	badFreshStake154 := new(wire.MsgBlock)
	badFreshStake154.FromBytes(block154Bytes)
	badFreshStake154.Header.FreshStake++
	recalculateMsgBlockMerkleRootsSize(badFreshStake154)
	b154test = dcrutil.NewBlock(badFreshStake154)

	// Throws an error in stake consensus.
	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrFreshStakeMismatch {
		t.Errorf("Unexpected no or wrong error for ErrFreshStakeMismatch "+
			"sanity check test: %v", err.Error())
	}

	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrFreshStakeMismatch {
		t.Errorf("Unexpected no or wrong error for ErrFreshStakeMismatch "+
			"sanity check test: %v", err.Error())
	}

	// ----------------------------------------------------------------------------
	// ErrStakeBelowMinimum still needs to be tested, can't on this blockchain
	// because it's above minimum and it'll always trigger failure on that
	// condition first.

	// ----------------------------------------------------------------------------
	// ErrNotEnoughVotes
	notEnoughVotes154 := new(wire.MsgBlock)
	notEnoughVotes154.FromBytes(block154Bytes)
	notEnoughVotes154.STransactions = notEnoughVotes154.STransactions[0:2]
	notEnoughVotes154.Header.FreshStake = 0
	notEnoughVotes154.Header.Voters = 2
	recalculateMsgBlockMerkleRootsSize(notEnoughVotes154)
	b154test = dcrutil.NewBlock(notEnoughVotes154)

	// Fails and hits ErrNotEnoughVotes.
	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrNotEnoughVotes {
		t.Errorf("Got no or unexpected block sanity err for "+
			"not enough votes (err: %v)", err)
	}

	// ----------------------------------------------------------------------------
	// ErrTooManyVotes
	invalidSSGenFor154, _ := hex.DecodeString("0100000002000000000000000000000" +
		"0000000000000000000000000000000000000000000ffffffff00ffffffff9a4fc238" +
		"0060cd86a65620f43af5d641a15c11cba8a3b41cb0f87c2e5795ef590000000001fff" +
		"fffff0300000000000000000000266a241cf1d119f9443cd651ef6ff263b561d77b27" +
		"426e6767f3a853a2370d588ccf119800000000000000000000000000046a02ffffe57" +
		"00bb10000000000001abb76a914e9c66c96902aa5ea1dae549e8bdc01ebc8ff7ae488" +
		"ac000000000000000002c5220bb10000000000000000ffffffff04deadbeef204e000" +
		"00000000037000000020000006a4730440220329517d0216a0825843e41030f40167e" +
		"1a71f7b23986eedab83ad6eaa9aec07f022029c6c808dc18ad59454985108dfeef1c1" +
		"a1f1753d07bc5041bb133d0400d294e0121032e1e80b402627c3d60789e8b52d20ae6" +
		"c05768c9c8d0a296b4ae6043a1e6a0c1")
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(invalidSSGenFor154)

	tooManyVotes154 := new(wire.MsgBlock)
	tooManyVotes154.FromBytes(block154Bytes)
	tooManyVotes154.AddSTransaction(mtxFromB)
	tooManyVotes154.Header.Voters = 6

	recalculateMsgBlockMerkleRootsSize(tooManyVotes154)
	b154test = dcrutil.NewBlock(tooManyVotes154)

	// Fails and hits ErrTooManyVotes.
	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil {
		t.Errorf("got unexpected no error for ErrTooManyVotes sanity check")
	}

	// ----------------------------------------------------------------------------
	// ErrTicketUnavailable
	nonChosenTicket154 := new(wire.MsgBlock)
	nonChosenTicket154.FromBytes(block154Bytes)
	nonChosenTicket154.STransactions[4] = mtxFromB
	updateVoteCommitments(nonChosenTicket154)
	recalculateMsgBlockMerkleRootsSize(nonChosenTicket154)
	b154test = dcrutil.NewBlock(nonChosenTicket154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrTicketUnavailable sanity check"+
			": %v",
			err)
	}

	// Fails and hits ErrTooManyVotes.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrTicketUnavailable {
		t.Errorf("Unexpected no or wrong error for ErrTicketUnavailable test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrVotesOnWrongBlock
	wrongBlockVote154 := new(wire.MsgBlock)
	wrongBlockVote154.FromBytes(block154Bytes)
	wrongBlockScript, _ := hex.DecodeString("6a24008e029f92ae880d45ae61a5366b" +
		"b81d9903c5e61045c5b17f1bc97260f8e54497000000")
	wrongBlockVote154.STransactions[0].TxOut[0].PkScript = wrongBlockScript

	recalculateMsgBlockMerkleRootsSize(wrongBlockVote154)
	b154test = dcrutil.NewBlock(wrongBlockVote154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrVotesOnWrongBlock {
		t.Errorf("Unexpected no or wrong error for ErrVotesOnWrongBlock test: %v",
			err)
	}

	// Fails and hits ErrTooManyVotes.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrVotesOnWrongBlock {
		t.Errorf("Unexpected no or wrong error for ErrVotesOnWrongBlock test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrVotesMismatch
	votesMismatch154 := new(wire.MsgBlock)
	votesMismatch154.FromBytes(block154Bytes)
	sstxsIn154 := votesMismatch154.STransactions[5:]
	votesMismatch154.STransactions = votesMismatch154.STransactions[0:4] // 4 Votes
	votesMismatch154.STransactions = append(votesMismatch154.STransactions,
		sstxsIn154...)
	recalculateMsgBlockMerkleRootsSize(votesMismatch154)
	b154test = dcrutil.NewBlock(votesMismatch154)

	// Fails and hits ErrVotesMismatch.
	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrVotesMismatch {
		t.Errorf("got unexpected no or wrong error for ErrVotesMismatch "+
			"sanity check: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrIncongruentVotebit 1
	// Everyone votes Yea, but block header says Nay
	badVoteBit154 := new(wire.MsgBlock)
	badVoteBit154.FromBytes(block154Bytes)
	badVoteBit154.Header.VoteBits &= 0xFFFE // Zero critical voteBit
	b154test = dcrutil.NewBlock(badVoteBit154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 1: %v", err)
	}

	// Fails and hits ErrIncongruentVotebit.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 1: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrIncongruentVotebit 2
	// Everyone votes Nay, but block header says Yea
	badVoteBit154 = new(wire.MsgBlock)
	badVoteBit154.FromBytes(block154Bytes)
	badVoteBit154.Header.VoteBits = 0x0001
	for i, stx := range badVoteBit154.STransactions {
		if i < 5 {
			// VoteBits is encoded little endian.
			stx.TxOut[1].PkScript[2] = 0x00
		}
	}
	recalculateMsgBlockMerkleRootsSize(badVoteBit154)
	b154test = dcrutil.NewBlock(badVoteBit154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 2: %v", err)
	}

	// Fails and hits ErrIncongruentVotebit.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 2: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrIncongruentVotebit 3
	// 3x Nay 2x Yea, but block header says Yea
	badVoteBit154 = new(wire.MsgBlock)
	badVoteBit154.FromBytes(block154Bytes)
	badVoteBit154.Header.VoteBits = 0x0001
	for i, stx := range badVoteBit154.STransactions {
		if i < 3 {
			// VoteBits is encoded little endian.
			stx.TxOut[1].PkScript[2] = 0x00
		}
	}
	recalculateMsgBlockMerkleRootsSize(badVoteBit154)
	b154test = dcrutil.NewBlock(badVoteBit154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 3: %v", err)
	}

	// Fails and hits ErrIncongruentVotebit.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 3: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrIncongruentVotebit 4
	// 2x Nay 3x Yea, but block header says Nay
	badVoteBit154 = new(wire.MsgBlock)
	badVoteBit154.FromBytes(block154Bytes)
	badVoteBit154.Header.VoteBits = 0x0000
	for i, stx := range badVoteBit154.STransactions {
		if i < 2 {
			// VoteBits is encoded little endian.
			stx.TxOut[1].PkScript[2] = 0x00
		}
	}
	recalculateMsgBlockMerkleRootsSize(badVoteBit154)
	b154test = dcrutil.NewBlock(badVoteBit154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 4: %v", err)
	}

	// Fails and hits ErrIncongruentVotebit.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 4: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrIncongruentVotebit 5
	// 4x Voters
	// 2x Nay 2x Yea, but block header says Yea
	badVoteBit154 = new(wire.MsgBlock)
	badVoteBit154.FromBytes(block154Bytes)
	badVoteBit154.STransactions = badVoteBit154.STransactions[0:4] // 4 Votes
	badVoteBit154.Header.FreshStake = 0
	badVoteBit154.Header.VoteBits = 0x0001
	badVoteBit154.Header.Voters = 4
	badVoteBit154.Transactions[0].TxOut[0].Value = 3960396039
	for i, stx := range badVoteBit154.STransactions {
		if i < 2 {
			// VoteBits is encoded little endian.
			stx.TxOut[1].PkScript[2] = 0x00
		}
	}
	recalculateMsgBlockMerkleRootsSize(badVoteBit154)
	b154test = dcrutil.NewBlock(badVoteBit154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 5: %v", err)
	}

	// Fails and hits ErrIncongruentVotebit.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 5: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrIncongruentVotebit 6
	// 3x Voters
	// 2x Nay 1x Yea, but block header says Yea
	badVoteBit154 = new(wire.MsgBlock)
	badVoteBit154.FromBytes(block154Bytes)
	badVoteBit154.STransactions = badVoteBit154.STransactions[0:3]
	badVoteBit154.Header.FreshStake = 0
	badVoteBit154.Header.VoteBits = 0x0001
	badVoteBit154.Header.Voters = 3
	badVoteBit154.Transactions[0].TxOut[0].Value = 2970297029
	for i, stx := range badVoteBit154.STransactions {
		if i < 2 {
			// VoteBits is encoded little endian.
			stx.TxOut[1].PkScript[2] = 0x00
		}
	}
	recalculateMsgBlockMerkleRootsSize(badVoteBit154)
	b154test = dcrutil.NewBlock(badVoteBit154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 6: %v", err)
	}

	// Fails and hits ErrIncongruentVotebit.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 6: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrIncongruentVotebit 7
	// 3x Voters
	// 1x Nay 2x Yea, but block header says Nay
	badVoteBit154 = new(wire.MsgBlock)
	badVoteBit154.FromBytes(block154Bytes)
	badVoteBit154.STransactions = badVoteBit154.STransactions[0:3]
	badVoteBit154.Header.FreshStake = 0
	badVoteBit154.Header.VoteBits = 0x0000
	badVoteBit154.Header.Voters = 3
	badVoteBit154.Transactions[0].TxOut[0].Value = 2970297029
	for i, stx := range badVoteBit154.STransactions {
		if i < 1 {
			// VoteBits is encoded little endian.
			stx.TxOut[1].PkScript[2] = 0x00
		}
	}
	recalculateMsgBlockMerkleRootsSize(badVoteBit154)
	b154test = dcrutil.NewBlock(badVoteBit154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 7: %v", err)
	}

	// Fails and hits ErrIncongruentVotebit.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrIncongruentVotebit {
		t.Errorf("Unexpected no or wrong error for ErrIncongruentVotebit "+
			"test 7: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrSStxCommitment
	badCommitScrB, _ := hex.DecodeString("6a1ea495e69ddfe8b9770b823314ba66d4ca0" +
		"6201310540cce08000000001234")

	badSStxCommit154 := new(wire.MsgBlock)
	badSStxCommit154.FromBytes(block154Bytes)
	badSStxCommit154.STransactions[5].TxOut[1].PkScript = badCommitScrB

	recalculateMsgBlockMerkleRootsSize(badSStxCommit154)
	b154test = dcrutil.NewBlock(badSStxCommit154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrSStxCommitment sanity check: %v",
			err)
	}

	// Fails and hits ErrSStxCommitment.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrSStxCommitment {
		t.Errorf("Unexpected no or wrong error for ErrSStxCommitment test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrInvalidSSGenInput
	// It doesn't look like this one can actually be hit since checking if
	// IsSSGen should fail first.

	// ----------------------------------------------------------------------------
	// ErrSSGenPayeeOuts 1
	// Corrupt the payee
	badSSGenPayee154 := new(wire.MsgBlock)
	badSSGenPayee154.FromBytes(block154Bytes)
	badSSGenPayee154.STransactions[0].TxOut[2].PkScript[8] ^= 0x01

	recalculateMsgBlockMerkleRootsSize(badSSGenPayee154)
	b154test = dcrutil.NewBlock(badSSGenPayee154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrSSGenPayeeOuts sanity  "+
			"check: %v", err)
	}

	// Fails and hits ErrSSGenPayeeOuts.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrSSGenPayeeOuts {
		t.Errorf("Unexpected no or wrong error for ErrSSGenPayeeOuts "+
			"test: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrSSGenPayeeOuts 2
	// Corrupt the amount
	badSSGenPayee154 = new(wire.MsgBlock)
	badSSGenPayee154.FromBytes(block154Bytes)
	badSSGenPayee154.STransactions[0].TxOut[2].Value += 1

	recalculateMsgBlockMerkleRootsSize(badSSGenPayee154)
	b154test = dcrutil.NewBlock(badSSGenPayee154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrSSGenPayeeOuts sanity  "+
			"check2 : %v", err)
	}

	// Fails and hits ErrSSGenPayeeOuts.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrSSGenPayeeOuts {
		t.Errorf("Unexpected no or wrong error for ErrSSGenPayeeOuts "+
			"test 2: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrSSGenSubsidy
	// It appears that ErrSSGenSubsidy is impossible to hit due to the
	// check above that returns ErrSSGenPayeeOuts.

	// ----------------------------------------------------------------------------
	// ErrSStxInImmature
	// This is impossible to hit from a block's perspective because the
	// ticket isn't in the ticket database. So it fails prematurely.

	// ----------------------------------------------------------------------------
	// ErrSStxInScrType
	// The testbed blockchain doesn't have any non-P2PKH or non-P2SH outputs
	// so we can't test this. Independently tested and verified, but should
	// eventually get its own unit test.

	// ----------------------------------------------------------------------------
	// ErrInvalidSSRtxInput
	// It seems impossible to hit this from a block test because it fails when
	// it can't detect the relevant tickets in the missed ticket database
	// bucket.

	// ----------------------------------------------------------------------------
	// ErrTxSStxOutSpend
	// Try to spend a ticket output as a regular transaction.
	spendTaggedIn154 := new(wire.MsgBlock)
	spendTaggedIn154.FromBytes(block154Bytes)
	regularTx154, _ := spendTaggedIn154.Transactions[11].Bytes()
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(regularTx154)
	sstxTaggedInH, _ := chainhash.NewHashFromStr("83a562e29aad50b8aacb816914da" +
		"92a3fa46bea9e8f30b69efc6e64b455f0436")
	sstxTaggedIn := new(wire.TxIn)
	sstxTaggedIn.BlockHeight = 71
	sstxTaggedIn.BlockIndex = 1
	sstxTaggedIn.ValueIn = 20000
	sstxTaggedIn.SignatureScript = []byte{0x51, 0x51}
	sstxTaggedIn.Sequence = 0xffffffff
	sstxTaggedIn.PreviousOutPoint.Hash = *sstxTaggedInH
	sstxTaggedIn.PreviousOutPoint.Index = 0
	sstxTaggedIn.PreviousOutPoint.Tree = 1
	mtxFromB.AddTxIn(sstxTaggedIn)

	spendTaggedIn154.Transactions[11] = mtxFromB
	recalculateMsgBlockMerkleRootsSize(spendTaggedIn154)
	b154test = dcrutil.NewBlock(spendTaggedIn154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrTxSStxOutSpend sanity check: %v",
			err)
	}

	// Fails and hits ErrTxSStxOutSpend.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrTxSStxOutSpend {
		t.Errorf("Unexpected no or wrong error for ErrTxSStxOutSpend test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrRegTxSpendStakeOut
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(regularTx154)
	scrWithStakeOPCode, _ := hex.DecodeString("ba76a9149fe1d1f7ed3b1d0be66c4b3c" +
		"4981ca48b810e9bb88ac")
	mtxFromB.TxOut[0].PkScript = scrWithStakeOPCode

	spendTaggedOut154 := new(wire.MsgBlock)
	spendTaggedOut154.FromBytes(block154Bytes)
	spendTaggedOut154.Transactions[11] = mtxFromB
	recalculateMsgBlockMerkleRootsSize(spendTaggedOut154)
	b154test = dcrutil.NewBlock(spendTaggedOut154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrRegTxSpendStakeOut sanity check: %v",
			err)
	}

	// Fails and hits ErrRegTxSpendStakeOut.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrRegTxSpendStakeOut {
		t.Errorf("Unexpected no or wrong error for ErrRegTxSpendStakeOut test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrInvalidFinalState
	badFinalState154 := new(wire.MsgBlock)
	badFinalState154.FromBytes(block154Bytes)
	badFinalState154.Header.FinalState[0] ^= 0x01
	b154test = dcrutil.NewBlock(badFinalState154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrInvalidFinalState sanity check: %v",
			err)
	}

	// Fails and hits ErrInvalidFinalState.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrInvalidFinalState {
		t.Errorf("Unexpected no or wrong error for ErrInvalidFinalState test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrPoolSize
	badPoolSize154 := new(wire.MsgBlock)
	badPoolSize154.FromBytes(block154Bytes)
	badPoolSize154.Header.PoolSize++
	b154test = dcrutil.NewBlock(badPoolSize154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrPoolSize sanity check: %v",
			err)
	}

	// Fails and hits ErrPoolSize.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrPoolSize {
		t.Errorf("Unexpected no or wrong error for ErrPoolSize test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrBadStakebaseValue doesn't seem be be able to be hit because
	// ErrSSGenPayeeOuts is hit first. The code should be kept in in case
	// the first check somehow fails to catch inflation.

	// ----------------------------------------------------------------------------
	// ErrDiscordantTxTree
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(regularTx154)
	mtxFromB.TxIn[0].PreviousOutPoint.Tree = wire.TxTreeStake

	errTxTreeIn154 := new(wire.MsgBlock)
	errTxTreeIn154.FromBytes(block154Bytes)
	errTxTreeIn154.Transactions[11] = mtxFromB
	recalculateMsgBlockMerkleRootsSize(errTxTreeIn154)
	b154test = dcrutil.NewBlock(errTxTreeIn154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrDiscordantTxTree sanity check: %v",
			err)
	}

	// Fails and hits ErrDiscordantTxTree.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrDiscordantTxTree {
		t.Errorf("Unexpected no or wrong error for ErrDiscordantTxTree test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrStakeFees
	// It should be impossible for this to ever be triggered because of the
	// paranoid around transaction inflation, but leave it in anyway just
	// in case there is database corruption etc.

	// ----------------------------------------------------------------------------
	// ErrBadBlockHeight
	badBlockHeight154 := new(wire.MsgBlock)
	badBlockHeight154.FromBytes(block154Bytes)
	badBlockHeight154.Header.Height++
	updateVoteCommitments(badBlockHeight154)
	recalculateMsgBlockMerkleRootsSize(badBlockHeight154)
	b154test = dcrutil.NewBlock(badBlockHeight154)

	// Throws ProcessBlock error through checkBlockContext.
	_, _, err = chain.ProcessBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrBadBlockHeight {
		t.Errorf("ProcessBlock ErrBadBlockHeight test no or unexpected "+
			"error: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrNoTax 1
	// Tax output missing
	taxMissing154 := new(wire.MsgBlock)
	taxMissing154.FromBytes(block154Bytes)
	taxMissing154.Transactions[0].TxOut[0] = taxMissing154.Transactions[0].TxOut[1]

	recalculateMsgBlockMerkleRootsSize(taxMissing154)
	b154test = dcrutil.NewBlock(taxMissing154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected error for ErrNoTax "+
			"test 1: %v", err)
	}

	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrNoTax {
		t.Errorf("Got no error or unexpected error for ErrNoTax "+
			"test 1: %v", err)
	}

	// ErrNoTax 2
	// Wrong hash paid to
	taxMissing154 = new(wire.MsgBlock)
	taxMissing154.FromBytes(block154Bytes)
	taxMissing154.Transactions[0].TxOut[0].PkScript[8] ^= 0x01

	recalculateMsgBlockMerkleRootsSize(taxMissing154)
	b154test = dcrutil.NewBlock(taxMissing154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected error for ErrNoTax test 2: %v", err)
	}

	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrNoTax {
		t.Errorf("Got no error or unexpected error for ErrNoTax "+
			"test 2: %v", err)
	}

	// ErrNoTax 3
	// Wrong amount paid
	taxMissing154 = new(wire.MsgBlock)
	taxMissing154.FromBytes(block154Bytes)
	taxMissing154.Transactions[0].TxOut[0].Value--

	recalculateMsgBlockMerkleRootsSize(taxMissing154)
	b154test = dcrutil.NewBlock(taxMissing154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected error for ErrNoTax test 3: %v", err)
	}

	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrNoTax {
		t.Errorf("Got no error or unexpected error for ErrNoTax "+
			"test 3: %v", err)
	}

	// ----------------------------------------------------------------------------
	// ErrExpiredTx
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(regularTx154)
	mtxFromB.Expiry = 154

	expiredTx154 := new(wire.MsgBlock)
	expiredTx154.FromBytes(block154Bytes)
	expiredTx154.Transactions[11] = mtxFromB
	recalculateMsgBlockMerkleRootsSize(expiredTx154)
	b154test = dcrutil.NewBlock(expiredTx154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrExpiredTx sanity check: %v",
			err)
	}

	// Fails and hits ErrExpiredTx.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrExpiredTx {
		t.Errorf("Unexpected no or wrong error for ErrExpiredTx test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrFraudAmountIn
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(regularTx154)
	mtxFromB.TxIn[0].ValueIn--

	badValueIn154 := new(wire.MsgBlock)
	badValueIn154.FromBytes(block154Bytes)
	badValueIn154.Transactions[11] = mtxFromB
	recalculateMsgBlockMerkleRootsSize(badValueIn154)
	b154test = dcrutil.NewBlock(badValueIn154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrFraudAmountIn sanity check: %v",
			err)
	}

	// Fails and hits ErrFraudAmountIn.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrFraudAmountIn {
		t.Errorf("Unexpected no or wrong error for ErrFraudAmountIn test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrFraudBlockHeight
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(regularTx154)
	mtxFromB.TxIn[0].BlockHeight++

	badHeightProof154 := new(wire.MsgBlock)
	badHeightProof154.FromBytes(block154Bytes)
	badHeightProof154.Transactions[11] = mtxFromB
	recalculateMsgBlockMerkleRootsSize(badHeightProof154)
	b154test = dcrutil.NewBlock(badHeightProof154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrFraudBlockHeight sanity check: %v",
			err)
	}

	// Fails and hits ErrFraudBlockHeight.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrFraudBlockHeight {
		t.Errorf("Unexpected no or wrong error for ErrFraudBlockHeight test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrFraudBlockIndex
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(regularTx154)
	mtxFromB.TxIn[0].BlockIndex++

	badIndexProof154 := new(wire.MsgBlock)
	badIndexProof154.FromBytes(block154Bytes)
	badIndexProof154.Transactions[11] = mtxFromB
	recalculateMsgBlockMerkleRootsSize(badIndexProof154)
	b154test = dcrutil.NewBlock(badIndexProof154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrFraudBlockIndex sanity check: %v",
			err)
	}

	// Fails and hits ErrFraudBlockIndex.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrFraudBlockIndex {
		t.Errorf("Unexpected no or wrong error for ErrFraudBlockIndex test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrScriptValidation Reg Tree
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(regularTx154)
	mtxFromB.TxOut[0].Value--

	badScrVal154 := new(wire.MsgBlock)
	badScrVal154.FromBytes(block154Bytes)
	badScrVal154.Transactions[11] = mtxFromB
	recalculateMsgBlockMerkleRootsSize(badScrVal154)
	b154test = dcrutil.NewBlock(badScrVal154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrScriptValidation sanity check: %v",
			err)
	}

	// Fails and hits ErrScriptValidation.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrScriptValidation {
		t.Errorf("Unexpected no or wrong error for ErrScriptValidation test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrScriptValidation Stake Tree
	badScrValS154 := new(wire.MsgBlock)
	badScrValS154.FromBytes(block154Bytes)
	badScrValS154.STransactions[5].TxIn[0].SignatureScript[6] ^= 0x01
	recalculateMsgBlockMerkleRootsSize(badScrValS154)
	b154test = dcrutil.NewBlock(badScrValS154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrScriptValidation sanity check: %v",
			err)
	}

	// Fails and hits ErrScriptValidation.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrScriptValidation {
		t.Errorf("Unexpected no or wrong error for ErrScriptValidation test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// Invalidate the previous block transaction tree. All the tickets in
	// this block reference the previous transaction tree regular, and so
	// all should be invalid by missing the tx if the header invalidates the
	// previous block.
	invalMissingInsS154 := new(wire.MsgBlock)
	invalMissingInsS154.FromBytes(block154Bytes)
	for i := 0; i < int(invalMissingInsS154.Header.Voters); i++ {
		invalMissingInsS154.STransactions[i].TxOut[1].PkScript[2] = 0x00
	}
	invalMissingInsS154.Header.VoteBits = 0x0000

	recalculateMsgBlockMerkleRootsSize(invalMissingInsS154)
	b154test = dcrutil.NewBlock(invalMissingInsS154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for invalMissingInsS154 sanity check: %v",
			err)
	}

	// Fails and hits ErrMissingTxOut.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrMissingTxOut {
		t.Errorf("Unexpected no or wrong error for invalMissingInsS154 test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrScriptMalformed
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(regularTx154)
	mtxFromB.TxOut[0].PkScript = []byte{0x01, 0x02, 0x03, 0x04}

	malformedScr154 := new(wire.MsgBlock)
	malformedScr154.FromBytes(block154Bytes)
	malformedScr154.Transactions[11] = mtxFromB
	recalculateMsgBlockMerkleRootsSize(malformedScr154)
	b154test = dcrutil.NewBlock(malformedScr154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrScriptValidation sanity check: %v",
			err)
	}

	// Fails and hits ErrScriptMalformed.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrScriptMalformed {
		t.Errorf("Unexpected no or wrong error for ErrScriptMalformed test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrMissingTxOut (formerly ErrZeroValueOutputSpend). In the latest version of
	// the database, zero value outputs are automatically pruned, so the output
	// is simply missing.
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(regularTx154)

	zeroValueTxH, _ := chainhash.NewHashFromStr("9432be62a2c664ad021fc3567c" +
		"700239067cfaa59be5b67b5808b158dfaed060")
	zvi := new(wire.TxIn)
	zvi.BlockHeight = 83
	zvi.BlockIndex = 0
	zvi.ValueIn = 0
	zvi.SignatureScript = []byte{0x51}
	zvi.Sequence = 0xffffffff
	zvi.PreviousOutPoint.Hash = *zeroValueTxH
	zvi.PreviousOutPoint.Index = 1
	zvi.PreviousOutPoint.Tree = 1
	mtxFromB.AddTxIn(zvi)
	spendZeroValueIn154 := new(wire.MsgBlock)
	spendZeroValueIn154.FromBytes(block154Bytes)
	spendZeroValueIn154.Transactions[11] = mtxFromB

	recalculateMsgBlockMerkleRootsSize(spendZeroValueIn154)
	b154test = dcrutil.NewBlock(spendZeroValueIn154)

	err = blockchain.CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrZeroValueOutputSpend sanity "+
			"check: %v", err)
	}

	// Fails and hits ErrZeroValueOutputSpend.
	err = chain.CheckConnectBlock(b154test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrMissingTxOut {
		t.Errorf("Unexpected no or wrong error for "+
			"ErrMissingTxOut test: %v", err)
	}

	// ----------------------------------------------------------------------------
	// DoubleSpend/TxTree invalidation edge case testing
	//
	// Load up to block 166. 165 invalidates its previous tx tree, making
	// it good for testing.
	for i := testsIdx2; i < testsIdx3; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Errorf("NewBlockFromBytes error: %v", err.Error())
		}

		// Double check and ensure there's no cross tree spending in
		// block 164.
		if i == 164 {
			for _, stx := range bl.MsgBlock().STransactions {
				for j, sTxIn := range stx.TxIn {
					for _, tx := range bl.MsgBlock().Transactions {
						h := tx.TxHash()
						if h == sTxIn.PreviousOutPoint.Hash {
							t.Errorf("Illegal cross tree reference ("+
								"stx %v references tx %v in input %v)",
								stx.TxHash(), h, j)
						}
					}
				}
			}
		}

		_, _, err = chain.ProcessBlock(bl, blockchain.BFNoPoWCheck)
		if err != nil {
			t.Errorf("ProcessBlock error: %v", err.Error())
		}
	}
	block166Bytes := blockChain[int64(testsIdx3)]

	// ----------------------------------------------------------------------------
	// Attempt to spend from TxTreeRegular of block 164, which should never
	// have existed.
	spendFrom164RegB, _ := hex.DecodeString("01000000016a7a4928f20fbdeca6c0dd534" +
		"8110d26e7abb91549d846638db6379ecae300f70500000000ffffffff01c095a9" +
		"050000000000001976a91487bd9a1466619fa8253baa37ffca87bb5b1892da88a" +
		"c000000000000000001ffffffffffffffff00000000ffffffff00")
	mtxFromB = new(wire.MsgTx)
	mtxFromB.FromBytes(spendFrom164RegB)
	spendInvalid166 := new(wire.MsgBlock)
	spendInvalid166.FromBytes(block166Bytes)
	spendInvalid166.AddTransaction(mtxFromB)

	recalculateMsgBlockMerkleRootsSize(spendInvalid166)
	b166test := dcrutil.NewBlock(spendInvalid166)

	err = blockchain.CheckWorklessBlockSanity(b166test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrMissingTxOut test 1 sanity "+
			"check: %v", err)
	}

	// Fails and hits ErrMissingTxOut.
	err = chain.CheckConnectBlock(b166test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrMissingTxOut {
		t.Errorf("Unexpected no or wrong error for "+
			"ErrMissingTxOut test 1: %v", err)
	}

	// ----------------------------------------------------------------------------
	// Try to buy a ticket with this block's coinbase transaction, which
	// should not be allowed because it doesn't yet exist.
	sstxSpendInvalid166 := new(wire.MsgBlock)
	sstxSpendInvalid166.FromBytes(block166Bytes)
	sstxToUse166 := sstxSpendInvalid166.STransactions[5]

	// Craft an otherwise valid sstx.
	coinbaseHash := spendInvalid166.Transactions[0].TxHash()
	sstxCBIn := new(wire.TxIn)
	sstxCBIn.ValueIn = 29702992297
	sstxCBIn.PreviousOutPoint.Hash = coinbaseHash
	sstxCBIn.PreviousOutPoint.Index = 2
	sstxCBIn.PreviousOutPoint.Tree = 0
	sstxCBIn.BlockHeight = 166
	sstxCBIn.BlockIndex = 0
	sstxCBIn.Sequence = 4294967295
	sstxCBIn.SignatureScript = []byte{0x51, 0x51}
	sstxToUse166.AddTxIn(sstxCBIn)

	orgAddr, _ := dcrutil.DecodeAddress("ScuQxvveKGfpG1ypt6u27F99Anf7EW3cqhq")
	pkScript, _ := txscript.GenerateSStxAddrPush(orgAddr,
		dcrutil.Amount(29702992297), 0x0000)
	txOut := wire.NewTxOut(int64(0), pkScript)
	sstxToUse166.AddTxOut(txOut)
	pkScript, _ = txscript.PayToSStxChange(orgAddr)
	txOut = wire.NewTxOut(0, pkScript)
	sstxToUse166.AddTxOut(txOut)

	recalculateMsgBlockMerkleRootsSize(sstxSpendInvalid166)
	b166test = dcrutil.NewBlock(sstxSpendInvalid166)

	err = blockchain.CheckWorklessBlockSanity(b166test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrMissingTxOut test 2 sanity "+
			"check: %v", err)
	}

	// Fails and hits ErrMissingTxOut.
	err = chain.CheckConnectBlock(b166test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrMissingTxOut {
		t.Errorf("Unexpected no or wrong error for "+
			"ErrMissingTxOut test 2: %v", err)
	}

	// ----------------------------------------------------------------------------
	// Try to spend immature change from one SStx in another SStx, hitting
	// ErrImmatureSpend.
	sstxSpend2Invalid166 := new(wire.MsgBlock)
	sstxSpend2Invalid166.FromBytes(block166Bytes)
	sstxToUse166 = sstxSpend2Invalid166.STransactions[6]
	sstxChangeHash := spendInvalid166.STransactions[5].TxHash()
	sstxChangeIn := new(wire.TxIn)
	sstxChangeIn.ValueIn = 2345438298
	sstxChangeIn.PreviousOutPoint.Hash = sstxChangeHash
	sstxChangeIn.PreviousOutPoint.Index = 2
	sstxChangeIn.PreviousOutPoint.Tree = 1
	sstxChangeIn.BlockHeight = 166
	sstxChangeIn.BlockIndex = 5
	sstxChangeIn.Sequence = 4294967295
	sstxChangeIn.SignatureScript = []byte{0x51, 0x51}
	sstxToUse166.AddTxIn(sstxChangeIn)

	pkScript, _ = txscript.GenerateSStxAddrPush(orgAddr,
		dcrutil.Amount(2345438298), 0x0000)
	txOut = wire.NewTxOut(int64(0), pkScript)
	sstxToUse166.AddTxOut(txOut)
	pkScript, _ = txscript.PayToSStxChange(orgAddr)
	txOut = wire.NewTxOut(0, pkScript)
	sstxToUse166.AddTxOut(txOut)

	recalculateMsgBlockMerkleRootsSize(sstxSpend2Invalid166)
	b166test = dcrutil.NewBlock(sstxSpend2Invalid166)

	err = blockchain.CheckWorklessBlockSanity(b166test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrImmatureSpend test sanity "+
			"check: %v", err)
	}

	// Fails and hits ErrMissingTxOut. It may not be immediately clear
	// why this happens, but in the case of the stake transaction
	// tree, because you can't spend in chains, the txlookup code
	// doesn't even bother to populate the spent list in the txlookup
	// and instead just writes the transaction hash as being missing.
	// This output doesn't become legal to spend until the next block.
	err = chain.CheckConnectBlock(b166test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrImmatureSpend {
		t.Errorf("Unexpected no or wrong error for "+
			"ErrImmatureSpend test: %v", err)
	}

	// ----------------------------------------------------------------------------
	// Try to double spend the same input in the stake transaction tree.
	sstxSpend3Invalid166 := new(wire.MsgBlock)
	sstxSpend3Invalid166.FromBytes(block166Bytes)
	sstxToUse166 = sstxSpend3Invalid166.STransactions[6]
	sstxToUse166.AddTxIn(sstxSpend3Invalid166.STransactions[5].TxIn[0])
	sstxToUse166.AddTxOut(sstxSpend3Invalid166.STransactions[5].TxOut[1])
	sstxToUse166.AddTxOut(sstxSpend3Invalid166.STransactions[5].TxOut[2])
	recalculateMsgBlockMerkleRootsSize(sstxSpend3Invalid166)
	b166test = dcrutil.NewBlock(sstxSpend3Invalid166)

	err = blockchain.CheckWorklessBlockSanity(b166test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for double spend test 1 sanity "+
			"check: %v", err)
	}

	// Fails and hits ErrMissingTxOut.
	err = chain.CheckConnectBlock(b166test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrMissingTxOut {
		t.Errorf("Unexpected no or wrong error for "+
			"double spend test 1: %v", err)
	}

	// ----------------------------------------------------------------------------
	// Try to double spend an input in the unconfirmed tx tree regular
	// that's already spent in the stake tree.
	regTxSpendStakeIn166 := new(wire.MsgBlock)
	regTxSpendStakeIn166.FromBytes(block166Bytes)
	sstxIn := regTxSpendStakeIn166.STransactions[5].TxIn[0]
	regTxSpendStakeIn166.Transactions[2].AddTxIn(sstxIn)

	recalculateMsgBlockMerkleRootsSize(regTxSpendStakeIn166)
	b166test = dcrutil.NewBlock(regTxSpendStakeIn166)

	err = blockchain.CheckWorklessBlockSanity(b166test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for deouble spend test 2 sanity "+
			"check: %v", err)
	}

	// Fails and hits ErrMissingTxOut.
	err = chain.CheckConnectBlock(b166test, blockchain.BFNoPoWCheck)
	if err == nil || err.(blockchain.RuleError).ErrorCode !=
		blockchain.ErrMissingTxOut {
		t.Errorf("Unexpected no or wrong error for "+
			"double spend test 2: %v", err)
	}
}

// TestBlockchainSpendJournal tests for whether or not the spend journal is being
// written to disk correctly on a live blockchain.
func TestBlockchainSpendJournal(t *testing.T) {
	// Create a new database and chain instance to run tests against.
	params := &chaincfg.SimNetParams
	chain, teardownFunc, err := chainSetup("spendjournalunittest", params)
	if err != nil {
		t.Errorf("Failed to setup chain instance: %v", err)
		return
	}
	defer teardownFunc()

	// The genesis block should fail to connect since it's already
	// inserted.
	err = chain.CheckConnectBlock(dcrutil.NewBlock(params.GenesisBlock),
		blockchain.BFNone)
	if err == nil {
		t.Errorf("CheckConnectBlock: Did not receive expected error")
	}

	// Load up the rest of the blocks up to HEAD.
	filename := filepath.Join("testdata/", "reorgto179.bz2")
	fi, err := os.Open(filename)
	if err != nil {
		t.Errorf("Failed to open %s: %v", filename, err)
	}
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder := gob.NewDecoder(bcBuf)
	blockChain := make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockChain); err != nil {
		t.Errorf("error decoding test blockchain: %v", err.Error())
	}

	// Load up the short chain
	finalIdx1 := 179
	for i := 1; i < finalIdx1+1; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Fatalf("NewBlockFromBytes error: %v", err.Error())
		}

		_, _, err = chain.ProcessBlock(bl, blockchain.BFNone)
		if err != nil {
			t.Fatalf("ProcessBlock error at height %v: %v", i, err.Error())
		}
	}

	err = chain.DoStxoTest()
	if err != nil {
		t.Errorf(err.Error())
	}
}

// TestSequenceLocksActive ensure the sequence locks are detected as active or
// not as expected in all possible scenarios.
func TestSequenceLocksActive(t *testing.T) {
	now := time.Now().Unix()
	tests := []struct {
		name          string
		seqLockHeight int64
		seqLockTime   int64
		blockHeight   int64
		medianTime    int64
		want          bool
	}{
		{
			// Block based sequence lock with height at min
			// required.
			name:          "min active block height",
			seqLockHeight: 1000,
			seqLockTime:   -1,
			blockHeight:   1001,
			medianTime:    now + 31,
			want:          true,
		},
		{
			// Time based sequence lock with relative time at min
			// required.
			name:          "min active median time",
			seqLockHeight: -1,
			seqLockTime:   now + 30,
			blockHeight:   1001,
			medianTime:    now + 31,
			want:          true,
		},
		{
			// Block based sequence lock at same height.
			name:          "same height",
			seqLockHeight: 1000,
			seqLockTime:   -1,
			blockHeight:   1000,
			medianTime:    now + 31,
			want:          false,
		},
		{
			// Time based sequence lock with relative time equal to
			// lock time.
			name:          "same median time",
			seqLockHeight: -1,
			seqLockTime:   now + 30,
			blockHeight:   1001,
			medianTime:    now + 30,
			want:          false,
		},
		{
			// Block based sequence lock with relative height below
			// required.
			name:          "height below required",
			seqLockHeight: 1000,
			seqLockTime:   -1,
			blockHeight:   999,
			medianTime:    now + 31,
			want:          false,
		},
		{
			// Time based sequence lock with relative time before
			// required.
			name:          "median time before required",
			seqLockHeight: -1,
			seqLockTime:   now + 30,
			blockHeight:   1001,
			medianTime:    now + 29,
			want:          false,
		},
	}

	for _, test := range tests {
		seqLock := blockchain.SequenceLock{
			MinHeight: test.seqLockHeight,
			MinTime:   test.seqLockTime,
		}
		got := blockchain.SequenceLockActive(&seqLock, test.blockHeight,
			time.Unix(test.medianTime, 0))
		if got != test.want {
			t.Errorf("%s: mismatched seqence lock status - got %v, "+
				"want %v", test.name, got, test.want)
			continue
		}
	}
}

// TestCheckBlockSanity tests the context free block sanity checks with blocks
// not on a chain.
func TestCheckBlockSanity(t *testing.T) {
	params := &chaincfg.SimNetParams
	timeSource := blockchain.NewMedianTime()
	block := dcrutil.NewBlock(&badBlock)
	err := blockchain.CheckBlockSanity(block, timeSource, params)
	if err == nil {
		t.Fatalf("block should fail.\n")
	}
}

// TestCheckWorklessBlockSanity tests the context free workless block sanity
// checks with blocks not on a chain.
func TestCheckWorklessBlockSanity(t *testing.T) {
	params := &chaincfg.SimNetParams
	timeSource := blockchain.NewMedianTime()
	block := dcrutil.NewBlock(&badBlock)
	err := blockchain.CheckWorklessBlockSanity(block, timeSource, params)
	if err == nil {
		t.Fatalf("block should fail.\n")
	}
}

// TestCheckBlockHeaderContext tests that genesis block passes context headers
// because its parent is nil.
func TestCheckBlockHeaderContext(t *testing.T) {
	// Create a new database for the blocks.
	params := &chaincfg.SimNetParams
	dbPath := filepath.Join(os.TempDir(), "examplecheckheadercontext")
	_ = os.RemoveAll(dbPath)
	db, err := database.Create("ffldb", dbPath, params.Net)
	if err != nil {
		t.Fatalf("Failed to create database: %v\n", err)
		return
	}
	defer os.RemoveAll(dbPath)
	defer db.Close()

	// Create a new BlockChain instance using the underlying database for
	// the simnet network.
	chain, err := blockchain.New(&blockchain.Config{
		DB:          db,
		ChainParams: params,
		TimeSource:  blockchain.NewMedianTime(),
	})
	if err != nil {
		t.Fatalf("Failed to create chain instance: %v\n", err)
		return
	}

	err = chain.TstCheckBlockHeaderContext(&params.GenesisBlock.Header, nil, blockchain.BFNone)
	if err != nil {
		t.Fatalf("genesisblock should pass just by definition: %v\n", err)
		return
	}

	// Test failing checkBlockHeaderContext when calcNextRequiredDifficulty
	// fails.
	block := dcrutil.NewBlock(&badBlock)
	newNode := blockchain.TstNewBlockNode(&block.MsgBlock().Header, nil)
	err = chain.TstCheckBlockHeaderContext(&block.MsgBlock().Header, newNode, blockchain.BFNone)
	if err == nil {
		t.Fatalf("Should fail due to bad diff in newNode\n")
		return
	}
}

// TestTxValidationErrors ensures certain malformed freestanding transactions
// are rejected as as expected.
func TestTxValidationErrors(t *testing.T) {
	// Create a transaction that is too large
	tx := wire.NewMsgTx()
	prevOut := wire.NewOutPoint(&chainhash.Hash{0x01}, 0, wire.TxTreeRegular)
	tx.AddTxIn(wire.NewTxIn(prevOut, nil))
	pkScript := bytes.Repeat([]byte{0x00}, wire.MaxBlockPayload)
	tx.AddTxOut(wire.NewTxOut(0, pkScript))

	// Assert the transaction is larger than the max allowed size.
	txSize := tx.SerializeSize()
	if txSize <= wire.MaxBlockPayload {
		t.Fatalf("generated transaction is not large enough -- got "+
			"%d, want > %d", txSize, wire.MaxBlockPayload)
	}

	// Ensure transaction is rejected due to being too large.
	err := blockchain.CheckTransactionSanity(tx, &chaincfg.MainNetParams)
	rerr, ok := err.(blockchain.RuleError)
	if !ok {
		t.Fatalf("CheckTransactionSanity: unexpected error type for "+
			"transaction that is too large -- got %T", err)
	}
	if rerr.ErrorCode != blockchain.ErrTxTooBig {
		t.Fatalf("CheckTransactionSanity: unexpected error code for "+
			"transaction that is too large -- got %v, want %v",
			rerr.ErrorCode, blockchain.ErrTxTooBig)
	}
}

// badBlock is an intentionally bad block that should fail the context-less
// sanity checks.
var badBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version:      1,
		MerkleRoot:   chaincfg.SimNetParams.GenesisBlock.Header.MerkleRoot,
		VoteBits:     uint16(0x0000),
		FinalState:   [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Voters:       uint16(0x0000),
		FreshStake:   uint8(0x00),
		Revocations:  uint8(0x00),
		Timestamp:    time.Unix(1401292357, 0), // 2009-01-08 20:54:25 -0600 CST
		PoolSize:     uint32(0),
		Bits:         0x207fffff, // 545259519
		SBits:        int64(0x0000000000000000),
		Nonce:        0x37580963,
		StakeVersion: uint32(0),
		Height:       uint32(0),
	},
	Transactions:  []*wire.MsgTx{},
	STransactions: []*wire.MsgTx{},
}
