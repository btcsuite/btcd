// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

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

	merkles := BuildMerkleTreeStore(tempBlock.Transactions())
	merklesStake := BuildMerkleTreeStore(tempBlock.STransactions())

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
	timeSource := NewMedianTime()

	// ----------------------------------------------------------------------------
	// ErrBlockOneOutputs 1
	// No coinbase outputs check can't trigger because it throws an error
	// elsewhere.

	// ----------------------------------------------------------------------------
	// Add the rest of the blocks up to the stake early test block.
	stakeEarlyTest := 142
	for i := 1; i < stakeEarlyTest; i++ {
		bl, err := dcrutil.NewBlockFromBytes(blockChain[int64(i)])
		if err != nil {
			t.Errorf("NewBlockFromBytes error: %v", err.Error())
		}

		_, _, err = chain.ProcessBlock(bl, BFNoPoWCheck)
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

	err = CheckWorklessBlockSanity(b142test, timeSource, params)
	if err == nil || err.(RuleError).ErrorCode != ErrInvalidEarlyStakeTx {
		t.Errorf("Got unexpected no error or wrong error for "+
			"ErrInvalidEarlyStakeTx test: %v", err)
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

		_, _, err = chain.ProcessBlock(bl, BFNoPoWCheck)
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
	err = chain.CheckConnectBlock(block153, BFNoPoWCheck)
	if err != nil {
		t.Errorf("CheckConnectBlock error: %v", err.Error())
	}
	block153Bytes := blockChain[int64(testsIdx1)]

	// ----------------------------------------------------------------------------
	// ErrMissingParent
	missingParent153 := new(wire.MsgBlock)
	missingParent153.FromBytes(block153Bytes)
	missingParent153.Header.PrevBlock[8] ^= 0x01
	updateVoteCommitments(missingParent153)
	recalculateMsgBlockMerkleRootsSize(missingParent153)
	b153test := dcrutil.NewBlock(missingParent153)

	err = CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected sanity error for ErrMissingParent test: %v",
			err)
	}

	err = chain.CheckConnectBlock(b153test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrMissingParent {
		t.Errorf("Got no or unexpected error for ErrMissingParent test %v", err)
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

	err = CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrSSRtxPayeesMismatch sanity  "+
			"check: %v", err)
	}

	// Fails and hits ErrSSRtxPayeesMismatch.
	err = chain.CheckConnectBlock(b153test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrSSRtxPayeesMismatch {
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

	err = CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrSSRtxPayees sanity  "+
			"check 1: %v", err)
	}

	// Fails and hits ErrSSRtxPayees.
	err = chain.CheckConnectBlock(b153test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrSSRtxPayees {
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

	err = CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrSSRtxPayees sanity "+
			"check 2: %v", err)
	}

	// Fails and hits ErrSSRtxPayees.
	err = chain.CheckConnectBlock(b153test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrSSRtxPayees {
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

	err = CheckWorklessBlockSanity(b153test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrInvalidSSRtx sanity check: %v",
			err)
	}

	// Fails and hits ErrInvalidSSRtx.
	err = chain.CheckConnectBlock(b153test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrInvalidSSRtx {
		t.Errorf("Unexpected no or wrong error for ErrInvalidSSRtx test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// Insert block 154 and continue testing
	block153MsgBlock := new(wire.MsgBlock)
	block153MsgBlock.FromBytes(block153Bytes)
	b153test = dcrutil.NewBlock(block153MsgBlock)
	_, _, err = chain.ProcessBlock(b153test, BFNoPoWCheck)
	if err != nil {
		t.Errorf("Got unexpected error processing block 153 %v", err)
	}
	block154Bytes := blockChain[int64(testsIdx2)]
	block154MsgBlock := new(wire.MsgBlock)
	block154MsgBlock.FromBytes(block154Bytes)
	b154test := dcrutil.NewBlock(block154MsgBlock)

	// The incoming block should pass fine.
	err = CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("Unexpected error for check block 154 sanity: %v", err.Error())
	}

	err = chain.CheckConnectBlock(b154test, BFNoPoWCheck)
	if err != nil {
		t.Errorf("Unexpected error for check block 154 connect: %v", err.Error())
	}

	// ----------------------------------------------------------------------------
	// ErrStakeBelowMinimum still needs to be tested, can't on this blockchain
	// because it's above minimum and it'll always trigger failure on that
	// condition first.

	// ----------------------------------------------------------------------------
	// ErrTicketUnavailable
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

	nonChosenTicket154 := new(wire.MsgBlock)
	nonChosenTicket154.FromBytes(block154Bytes)
	nonChosenTicket154.STransactions[4] = mtxFromB
	updateVoteCommitments(nonChosenTicket154)
	recalculateMsgBlockMerkleRootsSize(nonChosenTicket154)
	b154test = dcrutil.NewBlock(nonChosenTicket154)

	err = CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrTicketUnavailable sanity check"+
			": %v",
			err)
	}

	// Fails and hits ErrTooManyVotes.
	err = chain.CheckConnectBlock(b154test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrTicketUnavailable {
		t.Errorf("Unexpected no or wrong error for ErrTicketUnavailable test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrInvalidSSGenInput
	// It doesn't look like this one can actually be hit since checking if
	// IsSSGen should fail first.

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

	err = CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrTxSStxOutSpend sanity check: %v",
			err)
	}

	// Fails and hits ErrTxSStxOutSpend.
	err = chain.CheckConnectBlock(b154test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrTxSStxOutSpend {
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

	err = CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrRegTxSpendStakeOut sanity check: %v",
			err)
	}

	// Fails and hits ErrRegTxSpendStakeOut.
	err = chain.CheckConnectBlock(b154test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrRegTxSpendStakeOut {
		t.Errorf("Unexpected no or wrong error for ErrRegTxSpendStakeOut test: %v",
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

	err = CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrDiscordantTxTree sanity check: %v",
			err)
	}

	// Fails and hits ErrDiscordantTxTree.
	err = chain.CheckConnectBlock(b154test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrDiscordantTxTree {
		t.Errorf("Unexpected no or wrong error for ErrDiscordantTxTree test: %v",
			err)
	}

	// ----------------------------------------------------------------------------
	// ErrStakeFees
	// It should be impossible for this to ever be triggered because of the
	// paranoid around transaction inflation, but leave it in anyway just
	// in case there is database corruption etc.

	// ----------------------------------------------------------------------------
	// ErrNoTax 1
	// Tax output missing
	taxMissing154 := new(wire.MsgBlock)
	taxMissing154.FromBytes(block154Bytes)
	taxMissing154.Transactions[0].TxOut[0] = taxMissing154.Transactions[0].TxOut[1]

	recalculateMsgBlockMerkleRootsSize(taxMissing154)
	b154test = dcrutil.NewBlock(taxMissing154)

	err = CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected error for ErrNoTax "+
			"test 1: %v", err)
	}

	err = chain.CheckConnectBlock(b154test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrNoTax {
		t.Errorf("Got no error or unexpected error for ErrNoTax "+
			"test 1: %v", err)
	}

	// ErrNoTax 3
	// Wrong amount paid
	taxMissing154 = new(wire.MsgBlock)
	taxMissing154.FromBytes(block154Bytes)
	taxMissing154.Transactions[0].TxOut[0].Value--

	recalculateMsgBlockMerkleRootsSize(taxMissing154)
	b154test = dcrutil.NewBlock(taxMissing154)

	err = CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("Got unexpected error for ErrNoTax test 3: %v", err)
	}

	err = chain.CheckConnectBlock(b154test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrNoTax {
		t.Errorf("Got no error or unexpected error for ErrNoTax "+
			"test 3: %v", err)
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

	err = CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrScriptValidation sanity check: %v",
			err)
	}

	// Fails and hits ErrScriptValidation.
	err = chain.CheckConnectBlock(b154test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrScriptValidation {
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

	err = CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrScriptValidation sanity check: %v",
			err)
	}

	// Fails and hits ErrScriptValidation.
	err = chain.CheckConnectBlock(b154test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrScriptValidation {
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

	err = CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for invalMissingInsS154 sanity check: %v",
			err)
	}

	// Fails and hits ErrMissingTxOut.
	err = chain.CheckConnectBlock(b154test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrMissingTxOut {
		t.Errorf("Unexpected no or wrong error for invalMissingInsS154 test: %v",
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

	err = CheckWorklessBlockSanity(b154test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrZeroValueOutputSpend sanity "+
			"check: %v", err)
	}

	// Fails and hits ErrZeroValueOutputSpend.
	err = chain.CheckConnectBlock(b154test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrMissingTxOut {
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

		_, _, err = chain.ProcessBlock(bl, BFNoPoWCheck)
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

	err = CheckWorklessBlockSanity(b166test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrMissingTxOut test 1 sanity "+
			"check: %v", err)
	}

	// Fails and hits ErrMissingTxOut.
	err = chain.CheckConnectBlock(b166test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrMissingTxOut {
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

	err = CheckWorklessBlockSanity(b166test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for ErrMissingTxOut test 2 sanity "+
			"check: %v", err)
	}

	// Fails and hits ErrMissingTxOut.
	err = chain.CheckConnectBlock(b166test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrMissingTxOut {
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

	err = CheckWorklessBlockSanity(b166test, timeSource, params)
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
	err = chain.CheckConnectBlock(b166test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrImmatureSpend {
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

	err = CheckWorklessBlockSanity(b166test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for double spend test 1 sanity "+
			"check: %v", err)
	}

	// Fails and hits ErrMissingTxOut.
	err = chain.CheckConnectBlock(b166test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrMissingTxOut {
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

	err = CheckWorklessBlockSanity(b166test, timeSource, params)
	if err != nil {
		t.Errorf("got unexpected error for deouble spend test 2 sanity "+
			"check: %v", err)
	}

	// Fails and hits ErrMissingTxOut.
	err = chain.CheckConnectBlock(b166test, BFNoPoWCheck)
	if err == nil || err.(RuleError).ErrorCode != ErrMissingTxOut {
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

		_, _, err = chain.ProcessBlock(bl, BFNone)
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
		seqLock := SequenceLock{
			MinHeight: test.seqLockHeight,
			MinTime:   test.seqLockTime,
		}
		got := SequenceLockActive(&seqLock, test.blockHeight,
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
	timeSource := NewMedianTime()
	block := dcrutil.NewBlock(&badBlock)
	err := CheckBlockSanity(block, timeSource, params)
	if err == nil {
		t.Fatalf("block should fail.\n")
	}
}

// TestCheckWorklessBlockSanity tests the context free workless block sanity
// checks with blocks not on a chain.
func TestCheckWorklessBlockSanity(t *testing.T) {
	params := &chaincfg.SimNetParams
	timeSource := NewMedianTime()
	block := dcrutil.NewBlock(&badBlock)
	err := CheckWorklessBlockSanity(block, timeSource, params)
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
	chain, err := New(&Config{
		DB:          db,
		ChainParams: params,
		TimeSource:  NewMedianTime(),
	})
	if err != nil {
		t.Fatalf("Failed to create chain instance: %v\n", err)
		return
	}

	err = chain.checkBlockHeaderContext(&params.GenesisBlock.Header, nil, BFNone)
	if err != nil {
		t.Fatalf("genesisblock should pass just by definition: %v\n", err)
		return
	}

	// Test failing checkBlockHeaderContext when calcNextRequiredDifficulty
	// fails.
	block := dcrutil.NewBlock(&badBlock)
	newNode := newBlockNode(&block.MsgBlock().Header, nil)
	err = chain.checkBlockHeaderContext(&block.MsgBlock().Header, newNode, BFNone)
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
	err := CheckTransactionSanity(tx, &chaincfg.MainNetParams)
	rerr, ok := err.(RuleError)
	if !ok {
		t.Fatalf("CheckTransactionSanity: unexpected error type for "+
			"transaction that is too large -- got %T", err)
	}
	if rerr.ErrorCode != ErrTxTooBig {
		t.Fatalf("CheckTransactionSanity: unexpected error code for "+
			"transaction that is too large -- got %v, want %v",
			rerr.ErrorCode, ErrTxTooBig)
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
