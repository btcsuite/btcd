// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

const (
	// MaxSigOpsPerBlock is the maximum number of signature operations
	// allowed for a block.  This really should be based upon the max
	// allowed block size for a network and any votes that might change it,
	// however, since it was not updated to be based upon it before
	// release, it will require a hard fork and associated vote agenda to
	// change it.  The original max block size for the protocol was 1MiB,
	// so that is what this is based on.
	MaxSigOpsPerBlock = 1000000 / 200

	// MaxTimeOffsetSeconds is the maximum number of seconds a block time
	// is allowed to be ahead of the current time.  This is currently 2
	// hours.
	MaxTimeOffsetSeconds = 2 * 60 * 60

	// MinCoinbaseScriptLen is the minimum length a coinbase script can be.
	MinCoinbaseScriptLen = 2

	// MaxCoinbaseScriptLen is the maximum length a coinbase script can be.
	MaxCoinbaseScriptLen = 100

	// medianTimeBlocks is the number of previous blocks which should be
	// used to calculate the median time used to validate block timestamps.
	medianTimeBlocks = 11

	// earlyVoteBitsValue is the only value of VoteBits allowed in a block
	// header before stake validation height.
	earlyVoteBitsValue = 0x0001

	// maxRevocationsPerBlock is the maximum number of revocations that are
	// allowed per block.
	maxRevocationsPerBlock = 255
)

var (
	// zeroHash is the zero value for a chainhash.Hash and is defined as a
	// package level variable to avoid the need to create a new instance
	// every time a check is needed.
	zeroHash = &chainhash.Hash{}

	// earlyFinalState is the only value of the final state allowed in a
	// block header before stake validation height.
	earlyFinalState = [6]byte{0x00}
)

// voteBitsApproveParent returns whether or not the passed vote bits indicate
// the regular transaction tree of the parent block should be considered valid.
func voteBitsApproveParent(voteBits uint16) bool {
	return dcrutil.IsFlagSet16(voteBits, dcrutil.BlockValid)
}

// approvesParent returns whether or not the vote bits in the passed header
// indicate the regular transaction tree of the parent block should be
// considered valid.
func headerApprovesParent(header *wire.BlockHeader) bool {
	return voteBitsApproveParent(header.VoteBits)
}

// isNullOutpoint determines whether or not a previous transaction output point
// is set.
func isNullOutpoint(outpoint *wire.OutPoint) bool {
	if outpoint.Index == math.MaxUint32 &&
		outpoint.Hash.IsEqual(zeroHash) &&
		outpoint.Tree == wire.TxTreeRegular {
		return true
	}
	return false
}

// isNullFraudProof determines whether or not a previous transaction fraud
// proof is set.
func isNullFraudProof(txIn *wire.TxIn) bool {
	switch {
	case txIn.BlockHeight != wire.NullBlockHeight:
		return false
	case txIn.BlockIndex != wire.NullBlockIndex:
		return false
	}

	return true
}

// IsCoinBaseTx determines whether or not a transaction is a coinbase.  A
// coinbase is a special transaction created by miners that has no inputs.
// This is represented in the block chain by a transaction with a single input
// that has a previous output transaction index set to the maximum value along
// with a zero hash.
//
// This function only differs from IsCoinBase in that it works with a raw wire
// transaction as opposed to a higher level util transaction.
func IsCoinBaseTx(msgTx *wire.MsgTx) bool {
	// A coin base must only have one transaction input.
	if len(msgTx.TxIn) != 1 {
		return false
	}

	// The previous output of a coin base must have a max value index and a
	// zero hash.
	prevOut := &msgTx.TxIn[0].PreviousOutPoint
	if prevOut.Index != math.MaxUint32 || !prevOut.Hash.IsEqual(zeroHash) {
		return false
	}

	return true
}

// IsCoinBase determines whether or not a transaction is a coinbase.  A
// coinbase is a special transaction created by miners that has no inputs.
// This is represented in the block chain by a transaction with a single input
// that has a previous output transaction index set to the maximum value along
// with a zero hash.
//
// This function only differs from IsCoinBaseTx in that it works with a higher
// level util transaction as opposed to a raw wire transaction.
func IsCoinBase(tx *dcrutil.Tx) bool {
	return IsCoinBaseTx(tx.MsgTx())
}

// SequenceLockActive determines if all of the inputs to a given transaction
// have achieved a relative age that surpasses the requirements specified by
// their respective sequence locks as calculated by CalcSequenceLock.  A single
// sequence lock is sufficient because the calculated lock selects the minimum
// required time and block height from all of the non-disabled inputs after
// which the transaction can be included.
func SequenceLockActive(lock *SequenceLock, blockHeight int64, medianTime time.Time) bool {
	// The transaction is not yet mature if it has not yet reached the
	// required minimum time and block height according to its sequence
	// locks.
	if blockHeight <= lock.MinHeight || medianTime.Unix() <= lock.MinTime {
		return false
	}

	return true
}

// CheckTransactionSanity performs some preliminary checks on a transaction to
// ensure it is sane.  These checks are context free.
func CheckTransactionSanity(tx *wire.MsgTx, params *chaincfg.Params) error {
	// A transaction must have at least one input.
	if len(tx.TxIn) == 0 {
		return ruleError(ErrNoTxInputs, "transaction has no inputs")
	}

	// A transaction must have at least one output.
	if len(tx.TxOut) == 0 {
		return ruleError(ErrNoTxOutputs, "transaction has no outputs")
	}

	// A transaction must not exceed the maximum allowed size when
	// serialized.
	serializedTxSize := tx.SerializeSize()
	if serializedTxSize > params.MaxTxSize {
		str := fmt.Sprintf("serialized transaction is too big - got "+
			"%d, max %d", serializedTxSize, params.MaxTxSize)
		return ruleError(ErrTxTooBig, str)
	}

	// Ensure the transaction amounts are in range.  Each transaction
	// output must not be negative or more than the max allowed per
	// transaction.  Also, the total of all outputs must abide by the same
	// restrictions.  All amounts in a transaction are in a unit value
	// known as an atom.  One decred is a quantity of atoms as defined by
	// the AtomsPerCoin constant.
	var totalAtom int64
	for _, txOut := range tx.TxOut {
		atom := txOut.Value
		if atom < 0 {
			str := fmt.Sprintf("transaction output has negative "+
				"value of %v", atom)
			return ruleError(ErrBadTxOutValue, str)
		}
		if atom > dcrutil.MaxAmount {
			str := fmt.Sprintf("transaction output value of %v is "+
				"higher than max allowed value of %v", atom,
				dcrutil.MaxAmount)
			return ruleError(ErrBadTxOutValue, str)
		}

		// Two's complement int64 overflow guarantees that any overflow
		// is detected and reported.  This is impossible for Decred,
		// but perhaps possible if an alt increases the total money
		// supply.
		totalAtom += atom
		if totalAtom < 0 {
			str := fmt.Sprintf("total value of all transaction "+
				"outputs exceeds max allowed value of %v",
				dcrutil.MaxAmount)
			return ruleError(ErrBadTxOutValue, str)
		}
		if totalAtom > dcrutil.MaxAmount {
			str := fmt.Sprintf("total value of all transaction "+
				"outputs is %v which is higher than max "+
				"allowed value of %v", totalAtom,
				dcrutil.MaxAmount)
			return ruleError(ErrBadTxOutValue, str)
		}
	}

	// Coinbase script length must be between min and max length.
	if IsCoinBaseTx(tx) {
		// The referenced outpoint should be null.
		if !isNullOutpoint(&tx.TxIn[0].PreviousOutPoint) {
			str := fmt.Sprintf("coinbase transaction did not use " +
				"a null outpoint")
			return ruleError(ErrBadCoinbaseOutpoint, str)
		}

		// The fraud proof should also be null.
		if !isNullFraudProof(tx.TxIn[0]) {
			str := fmt.Sprintf("coinbase transaction fraud proof " +
				"was non-null")
			return ruleError(ErrBadCoinbaseFraudProof, str)
		}

		slen := len(tx.TxIn[0].SignatureScript)
		if slen < MinCoinbaseScriptLen || slen > MaxCoinbaseScriptLen {
			str := fmt.Sprintf("coinbase transaction script "+
				"length of %d is out of range (min: %d, max: "+
				"%d)", slen, MinCoinbaseScriptLen,
				MaxCoinbaseScriptLen)
			return ruleError(ErrBadCoinbaseScriptLen, str)
		}
	} else if stake.IsSSGen(tx) {
		// Check script length of stake base signature.
		slen := len(tx.TxIn[0].SignatureScript)
		if slen < MinCoinbaseScriptLen || slen > MaxCoinbaseScriptLen {
			str := fmt.Sprintf("stakebase transaction script "+
				"length of %d is out of range (min: %d, max: "+
				"%d)", slen, MinCoinbaseScriptLen,
				MaxCoinbaseScriptLen)
			return ruleError(ErrBadStakebaseScriptLen, str)
		}

		// The script must be set to the one specified by the network.
		// Check script length of stake base signature.
		if !bytes.Equal(tx.TxIn[0].SignatureScript,
			params.StakeBaseSigScript) {
			str := fmt.Sprintf("stakebase transaction signature "+
				"script was set to disallowed value (got %x, "+
				"want %x)", tx.TxIn[0].SignatureScript,
				params.StakeBaseSigScript)
			return ruleError(ErrBadStakebaseScrVal, str)
		}

		// The ticket reference hash in an SSGen tx must not be null.
		ticketHash := &tx.TxIn[1].PreviousOutPoint
		if isNullOutpoint(ticketHash) {
			return ruleError(ErrBadTxInput, "ssgen tx ticket input"+
				" refers to previous output that is null")
		}
	} else {
		// Previous transaction outputs referenced by the inputs to
		// this transaction must not be null except in the case of
		// stake bases for SSGen tx.
		for _, txIn := range tx.TxIn {
			prevOut := &txIn.PreviousOutPoint
			if isNullOutpoint(prevOut) {
				return ruleError(ErrBadTxInput, "transaction "+
					"input refers to previous output that "+
					"is null")
			}
		}
	}

	// Check for duplicate transaction inputs.
	existingTxOut := make(map[wire.OutPoint]struct{})
	for _, txIn := range tx.TxIn {
		if _, exists := existingTxOut[txIn.PreviousOutPoint]; exists {
			return ruleError(ErrDuplicateTxInputs, "transaction "+
				"contains duplicate inputs")
		}
		existingTxOut[txIn.PreviousOutPoint] = struct{}{}
	}

	return nil
}

// checkProofOfStake ensures that all ticket purchases in the block pay at least
// the amount required by the block header stake bits which indicate the target
// stake difficulty (aka ticket price) as claimed.
func checkProofOfStake(block *dcrutil.Block, posLimit int64) error {
	msgBlock := block.MsgBlock()
	for _, staketx := range block.STransactions() {
		msgTx := staketx.MsgTx()
		if stake.IsSStx(msgTx) {
			commitValue := msgTx.TxOut[0].Value

			// Check for underflow block sbits.
			if commitValue < msgBlock.Header.SBits {
				errStr := fmt.Sprintf("Stake tx %v has a "+
					"commitment value less than the "+
					"minimum stake difficulty specified in"+
					" the block (%v)", staketx.Hash(),
					msgBlock.Header.SBits)
				return ruleError(ErrNotEnoughStake, errStr)
			}

			// Check if it's above the PoS limit.
			if commitValue < posLimit {
				errStr := fmt.Sprintf("Stake tx %v has a "+
					"commitment value less than the "+
					"minimum stake difficulty for the "+
					"network (%v)", staketx.Hash(),
					posLimit)
				return ruleError(ErrStakeBelowMinimum, errStr)
			}
		}
	}

	return nil
}

// CheckProofOfStake ensures that all ticket purchases in the block pay at least
// the amount required by the block header stake bits which indicate the target
// stake difficulty (aka ticket price) as claimed.
func CheckProofOfStake(block *dcrutil.Block, posLimit int64) error {
	return checkProofOfStake(block, posLimit)
}

// checkProofOfWork ensures the block header bits which indicate the target
// difficulty is in min/max range and that the block hash is less than the
// target difficulty as claimed.
//
// The flags modify the behavior of this function as follows:
//  - BFNoPoWCheck: The check to ensure the block hash is less than the target
//    difficulty is not performed.
func checkProofOfWork(header *wire.BlockHeader, powLimit *big.Int, flags BehaviorFlags) error {
	// The target difficulty must be larger than zero.
	target := CompactToBig(header.Bits)
	if target.Sign() <= 0 {
		str := fmt.Sprintf("block target difficulty of %064x is too "+
			"low", target)
		return ruleError(ErrUnexpectedDifficulty, str)
	}

	// The target difficulty must be less than the maximum allowed.
	if target.Cmp(powLimit) > 0 {
		str := fmt.Sprintf("block target difficulty of %064x is "+
			"higher than max of %064x", target, powLimit)
		return ruleError(ErrUnexpectedDifficulty, str)
	}

	// The block hash must be less than the claimed target unless the flag
	// to avoid proof of work checks is set.
	if flags&BFNoPoWCheck != BFNoPoWCheck {
		// The block hash must be less than the claimed target.
		hash := header.BlockHash()
		hashNum := HashToBig(&hash)
		if hashNum.Cmp(target) > 0 {
			str := fmt.Sprintf("block hash of %064x is higher than"+
				" expected max of %064x", hashNum, target)
			return ruleError(ErrHighHash, str)
		}
	}

	return nil
}

// CheckProofOfWork ensures the block header bits which indicate the target
// difficulty is in min/max range and that the block hash is less than the
// target difficulty as claimed.
func CheckProofOfWork(header *wire.BlockHeader, powLimit *big.Int) error {
	return checkProofOfWork(header, powLimit, BFNone)
}

// checkBlockHeaderSanity performs some preliminary checks on a block header to
// ensure it is sane before continuing with processing.  These checks are
// context free.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkProofOfWork.
func checkBlockHeaderSanity(header *wire.BlockHeader, timeSource MedianTimeSource, flags BehaviorFlags, chainParams *chaincfg.Params) error {
	// The stake validation height should always be at least stake enabled
	// height, so assert it because the code below relies on that assumption.
	stakeValidationHeight := uint32(chainParams.StakeValidationHeight)
	stakeEnabledHeight := uint32(chainParams.StakeEnabledHeight)
	if stakeEnabledHeight > stakeValidationHeight {
		return AssertError(fmt.Sprintf("checkBlockHeaderSanity called "+
			"with stake enabled height %d after stake validation "+
			"height %d", stakeEnabledHeight, stakeValidationHeight))
	}

	// Ensure the proof of work bits in the block header is in min/max
	// range and the block hash is less than the target value described by
	// the bits.
	err := checkProofOfWork(header, chainParams.PowLimit, flags)
	if err != nil {
		return err
	}

	// A block timestamp must not have a greater precision than one second.
	// This check is necessary because Go time.Time values support
	// nanosecond precision whereas the consensus rules only apply to
	// seconds and it's much nicer to deal with standard Go time values
	// instead of converting to seconds everywhere.
	if !header.Timestamp.Equal(time.Unix(header.Timestamp.Unix(), 0)) {
		str := fmt.Sprintf("block timestamp of %v has a higher "+
			"precision than one second", header.Timestamp)
		return ruleError(ErrInvalidTime, str)
	}

	// Ensure the block time is not too far in the future.
	maxTimestamp := timeSource.AdjustedTime().Add(time.Second *
		MaxTimeOffsetSeconds)
	if header.Timestamp.After(maxTimestamp) {
		str := fmt.Sprintf("block timestamp of %v is too far in the "+
			"future", header.Timestamp)
		return ruleError(ErrTimeTooNew, str)
	}

	// A block must not contain any votes or revocations, its vote bits
	// must be 0x0001, and its final state must be all zeroes before
	// stake validation begins.
	if header.Height < stakeValidationHeight {
		if header.Voters > 0 {
			errStr := fmt.Sprintf("block at height %d commits to "+
				"%d votes before stake validation height %d",
				header.Height, header.Voters,
				stakeValidationHeight)
			return ruleError(ErrInvalidEarlyStakeTx, errStr)
		}

		if header.Revocations > 0 {
			errStr := fmt.Sprintf("block at height %d commits to "+
				"%d revocations before stake validation height %d",
				header.Height, header.Revocations,
				stakeValidationHeight)
			return ruleError(ErrInvalidEarlyStakeTx, errStr)
		}

		if header.VoteBits != earlyVoteBitsValue {
			errStr := fmt.Sprintf("block at height %d commits to "+
				"invalid vote bits before stake validation "+
				"height %d (expected %x, got %x)",
				header.Height, stakeValidationHeight,
				earlyVoteBitsValue, header.VoteBits)
			return ruleError(ErrInvalidEarlyVoteBits, errStr)
		}

		if header.FinalState != earlyFinalState {
			errStr := fmt.Sprintf("block at height %d commits to "+
				"invalid final state before stake validation "+
				"height %d (expected %x, got %x)",
				header.Height, stakeValidationHeight,
				earlyFinalState, header.FinalState)
			return ruleError(ErrInvalidEarlyFinalState, errStr)
		}
	}

	// A block must not contain more votes than the minimum required to
	// reach majority once stake validation height has been reached.
	if header.Height >= stakeValidationHeight {
		majority := (chainParams.TicketsPerBlock / 2) + 1
		if header.Voters < majority {
			errStr := fmt.Sprintf("block does not commit to enough "+
				"votes (min: %d, got %d)", majority,
				header.Voters)
			return ruleError(ErrNotEnoughVotes, errStr)
		}
	}

	// The block header must not claim to contain more votes than the
	// maximum allowed.
	if header.Voters > chainParams.TicketsPerBlock {
		errStr := fmt.Sprintf("block commits to too many votes (max: "+
			"%d, got %d)", chainParams.TicketsPerBlock, header.Voters)
		return ruleError(ErrTooManyVotes, errStr)
	}

	// The block must not contain more ticket purchases than the maximum
	// allowed.
	if header.FreshStake > chainParams.MaxFreshStakePerBlock {
		errStr := fmt.Sprintf("block commits to too many ticket "+
			"purchases (max: %d, got %d)",
			chainParams.MaxFreshStakePerBlock, header.FreshStake)
		return ruleError(ErrTooManySStxs, errStr)
	}

	return nil
}

// checkBlockSanity performs some preliminary checks on a block to ensure it is
// sane before continuing with block processing.  These checks are context
// free.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkBlockHeaderSanity.
func checkBlockSanity(block *dcrutil.Block, timeSource MedianTimeSource, flags BehaviorFlags, chainParams *chaincfg.Params) error {
	msgBlock := block.MsgBlock()
	header := &msgBlock.Header
	err := checkBlockHeaderSanity(header, timeSource, flags, chainParams)
	if err != nil {
		return err
	}

	// All ticket purchases must meet the difficulty specified by the block
	// header.
	err = checkProofOfStake(block, chainParams.MinimumStakeDiff)
	if err != nil {
		return err
	}

	// A block must have at least one regular transaction.
	numTx := len(msgBlock.Transactions)
	if numTx == 0 {
		return ruleError(ErrNoTransactions, "block does not contain "+
			"any transactions")
	}

	// A block must not exceed the maximum allowed block payload when
	// serialized.
	//
	// This is a quick and context-free sanity check of the maximum block
	// size according to the wire protocol.  Even though the wire protocol
	// already prevents blocks bigger than this limit, there are other
	// methods of receiving a block that might not have been checked
	// already.  A separate block size is enforced later that takes into
	// account the network-specific block size and the results of block
	// size votes.  Typically that block size is more restrictive than this
	// one.
	serializedSize := msgBlock.SerializeSize()
	if serializedSize > wire.MaxBlockPayload {
		str := fmt.Sprintf("serialized block is too big - got %d, "+
			"max %d", serializedSize, wire.MaxBlockPayload)
		return ruleError(ErrBlockTooBig, str)
	}
	if header.Size != uint32(serializedSize) {
		str := fmt.Sprintf("serialized block is not size indicated in "+
			"header - got %d, expected %d", header.Size,
			serializedSize)
		return ruleError(ErrWrongBlockSize, str)
	}

	// The first transaction in a block's regular tree must be a coinbase.
	transactions := block.Transactions()
	if !IsCoinBaseTx(transactions[0].MsgTx()) {
		return ruleError(ErrFirstTxNotCoinbase, "first transaction in "+
			"block is not a coinbase")
	}

	// A block must not have more than one coinbase.
	for i, tx := range transactions[1:] {
		if IsCoinBaseTx(tx.MsgTx()) {
			str := fmt.Sprintf("block contains second coinbase at "+
				"index %d", i+1)
			return ruleError(ErrMultipleCoinbases, str)
		}
	}

	// Do some preliminary checks on each regular transaction to ensure they
	// are sane before continuing.
	for i, tx := range transactions {
		// A block must not have stake transactions in the regular
		// transaction tree.
		msgTx := tx.MsgTx()
		txType := stake.DetermineTxType(msgTx)
		if txType != stake.TxTypeRegular {
			errStr := fmt.Sprintf("block contains a stake "+
				"transaction in the regular transaction tree at "+
				"index %d", i)
			return ruleError(ErrStakeTxInRegularTree, errStr)
		}

		err := CheckTransactionSanity(msgTx, chainParams)
		if err != nil {
			return err
		}
	}

	// Do some preliminary checks on each stake transaction to ensure they
	// are sane while tallying each type before continuing.
	stakeValidationHeight := uint32(chainParams.StakeValidationHeight)
	var totalTickets, totalVotes, totalRevocations int64
	var totalYesVotes int64
	for txIdx, stx := range msgBlock.STransactions {
		err := CheckTransactionSanity(stx, chainParams)
		if err != nil {
			return err
		}

		// A block must not have regular transactions in the stake
		// transaction tree.
		txType := stake.DetermineTxType(stx)
		if txType == stake.TxTypeRegular {
			errStr := fmt.Sprintf("block contains regular "+
				"transaction in stake transaction tree at "+
				"index %d", txIdx)
			return ruleError(ErrRegTxInStakeTree, errStr)
		}

		switch txType {
		case stake.TxTypeSStx:
			totalTickets++

		case stake.TxTypeSSGen:
			totalVotes++

			// All votes in a block must commit to the parent of the
			// block once stake validation height has been reached.
			if header.Height >= stakeValidationHeight {
				votedHash, votedHeight := stake.SSGenBlockVotedOn(stx)
				if (votedHash != header.PrevBlock) || (votedHeight !=
					header.Height-1) {

					errStr := fmt.Sprintf("vote %s at index %d is "+
						"for parent block %s (height %d) versus "+
						"expected parent block %s (height %d)",
						stx.TxHash(), txIdx, votedHash,
						votedHeight, header.PrevBlock,
						header.Height-1)
					return ruleError(ErrVotesOnWrongBlock, errStr)
				}

				// Tally how many votes approve the previous block for use
				// when validating the header commitment.
				if voteBitsApproveParent(stake.SSGenVoteBits(stx)) {
					totalYesVotes++
				}
			}

		case stake.TxTypeSSRtx:
			totalRevocations++
		}
	}

	// A block must not contain more than the maximum allowed number of
	// revocations.
	if totalRevocations > maxRevocationsPerBlock {
		errStr := fmt.Sprintf("block contains %d revocations which "+
			"exceeds the maximum allowed amount of %d",
			totalRevocations, maxRevocationsPerBlock)
		return ruleError(ErrTooManyRevocations, errStr)
	}

	// A block must only contain stake transactions of the the allowed
	// types.
	//
	// NOTE: This is not possible to hit at the time this comment was
	// written because all transactions which are not specifically one of
	// the recognized stake transaction forms are considered regular
	// transactions and those are rejected above.  However, if a new stake
	// transaction type is added, that implicit condition would no longer
	// hold and therefore an explicit check is performed here.
	numStakeTx := int64(len(msgBlock.STransactions))
	calcStakeTx := totalTickets + totalVotes + totalRevocations
	if numStakeTx != calcStakeTx {
		errStr := fmt.Sprintf("block contains an unexpected number "+
			"of stake transactions (contains %d, expected %d)",
			numStakeTx, calcStakeTx)
		return ruleError(ErrNonstandardStakeTx, errStr)
	}

	// A block header must commit to the actual number of tickets purchases that
	// are in the block.
	if int64(header.FreshStake) != totalTickets {
		errStr := fmt.Sprintf("block header commitment to %d ticket "+
			"purchases does not match %d contained in the block",
			header.FreshStake, totalTickets)
		return ruleError(ErrFreshStakeMismatch, errStr)
	}

	// A block header must commit to the the actual number of votes that are
	// in the block.
	if int64(header.Voters) != totalVotes {
		errStr := fmt.Sprintf("block header commitment to %d votes "+
			"does not match %d contained in the block",
			header.Voters, totalVotes)
		return ruleError(ErrVotesMismatch, errStr)
	}

	// A block header must commit to the actual number of revocations that
	// are in the block.
	if int64(header.Revocations) != totalRevocations {
		errStr := fmt.Sprintf("block header commitment to %d revocations "+
			"does not match %d contained in the block",
			header.Revocations, totalRevocations)
		return ruleError(ErrRevocationsMismatch, errStr)
	}

	// A block header must commit to the same previous block acceptance
	// semantics expressed by the votes once stake validation height has
	// been reached.
	if header.Height >= stakeValidationHeight {
		totalNoVotes := totalVotes - totalYesVotes
		headerApproves := headerApprovesParent(header)
		votesApprove := totalYesVotes > totalNoVotes
		if headerApproves != votesApprove {
			errStr := fmt.Sprintf("block header commitment to previous "+
				"block approval does not match votes (header claims: %v, "+
				"votes: %v)", headerApproves, votesApprove)
			return ruleError(ErrIncongruentVotebit, errStr)
		}
	}

	// A block must not contain anything other than ticket purchases prior to
	// stake validation height.
	//
	// NOTE: This case is impossible to hit at this point at the time this
	// comment was written since the votes and revocations have already been
	// proven to be zero before stake validation height and the only other
	// type at the current time is ticket purchases, however, if another
	// stake type is ever added, consensus would break without this check.
	// It's better to be safe and it's a cheap check.
	if header.Height < stakeValidationHeight {
		if int64(len(msgBlock.STransactions)) != totalTickets {
			errStr := fmt.Sprintf("block contains stake "+
				"transactions other than ticket purchases before "+
				"stake validation height %d (total: %d, expected %d)",
				uint32(chainParams.StakeValidationHeight),
				len(msgBlock.STransactions), header.FreshStake)
			return ruleError(ErrInvalidEarlyStakeTx, errStr)
		}
	}

	// Build merkle tree and ensure the calculated merkle root matches the
	// entry in the block header.  This also has the effect of caching all
	// of the transaction hashes in the block to speed up future hash
	// checks.  Bitcoind builds the tree here and checks the merkle root
	// after the following checks, but there is no reason not to check the
	// merkle root matches here.
	merkles := BuildMerkleTreeStore(block.Transactions())
	calculatedMerkleRoot := merkles[len(merkles)-1]
	if !header.MerkleRoot.IsEqual(calculatedMerkleRoot) {
		str := fmt.Sprintf("block merkle root is invalid - block "+
			"header indicates %v, but calculated value is %v",
			header.MerkleRoot, calculatedMerkleRoot)
		return ruleError(ErrBadMerkleRoot, str)
	}

	// Build the stake tx tree merkle root too and check it.
	merkleStake := BuildMerkleTreeStore(block.STransactions())
	calculatedStakeMerkleRoot := merkleStake[len(merkleStake)-1]
	if !header.StakeRoot.IsEqual(calculatedStakeMerkleRoot) {
		str := fmt.Sprintf("block stake merkle root is invalid - block"+
			" header indicates %v, but calculated value is %v",
			header.StakeRoot, calculatedStakeMerkleRoot)
		return ruleError(ErrBadMerkleRoot, str)
	}

	// Check for duplicate transactions.  This check will be fairly quick
	// since the transaction hashes are already cached due to building the
	// merkle trees above.
	existingTxHashes := make(map[chainhash.Hash]struct{})
	stakeTransactions := block.STransactions()
	allTransactions := append(transactions, stakeTransactions...)

	for _, tx := range allTransactions {
		hash := tx.Hash()
		if _, exists := existingTxHashes[*hash]; exists {
			str := fmt.Sprintf("block contains duplicate "+
				"transaction %v", hash)
			return ruleError(ErrDuplicateTx, str)
		}
		existingTxHashes[*hash] = struct{}{}
	}

	// The number of signature operations must be less than the maximum
	// allowed per block.
	totalSigOps := 0
	for _, tx := range allTransactions {
		// We could potentially overflow the accumulator so check for
		// overflow.
		lastSigOps := totalSigOps

		msgTx := tx.MsgTx()
		isCoinBase := IsCoinBaseTx(msgTx)
		isSSGen := stake.IsSSGen(msgTx)
		totalSigOps += CountSigOps(tx, isCoinBase, isSSGen)
		if totalSigOps < lastSigOps || totalSigOps > MaxSigOpsPerBlock {
			str := fmt.Sprintf("block contains too many signature "+
				"operations - got %v, max %v", totalSigOps,
				MaxSigOpsPerBlock)
			return ruleError(ErrTooManySigOps, str)
		}
	}

	return nil
}

// CheckBlockSanity performs some preliminary checks on a block to ensure it is
// sane before continuing with block processing.  These checks are context
// free.
func CheckBlockSanity(block *dcrutil.Block, timeSource MedianTimeSource, chainParams *chaincfg.Params) error {
	return checkBlockSanity(block, timeSource, BFNone, chainParams)
}

// CheckWorklessBlockSanity performs some preliminary checks on a block to
// ensure it is sane before continuing with block processing.  These checks are
// context free.
func CheckWorklessBlockSanity(block *dcrutil.Block, timeSource MedianTimeSource, chainParams *chaincfg.Params) error {
	return checkBlockSanity(block, timeSource, BFNoPoWCheck, chainParams)
}

// checkBlockHeaderContext peforms several validation checks on the block
// header which depend on its position within the block chain.
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: All checks except those involving comparing the header against
//    the checkpoints are not performed.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) checkBlockHeaderContext(header *wire.BlockHeader, prevNode *blockNode, flags BehaviorFlags) error {
	// The genesis block is valid by definition.
	if prevNode == nil {
		return nil
	}

	fastAdd := flags&BFFastAdd == BFFastAdd
	if !fastAdd {
		// Ensure the difficulty specified in the block header matches
		// the calculated difficulty based on the previous block and
		// difficulty retarget rules.
		expDiff, err := b.calcNextRequiredDifficulty(prevNode,
			header.Timestamp)
		if err != nil {
			return err
		}
		blockDifficulty := header.Bits
		if blockDifficulty != expDiff {
			str := fmt.Sprintf("block difficulty of %d is not the"+
				" expected value of %d", blockDifficulty,
				expDiff)
			return ruleError(ErrUnexpectedDifficulty, str)
		}

		// Ensure the stake difficulty specified in the block header
		// matches the calculated difficulty based on the previous block
		// and difficulty retarget rules.
		expSDiff, err := b.calcNextRequiredStakeDifficulty(prevNode)
		if err != nil {
			return err
		}
		if header.SBits != expSDiff {
			errStr := fmt.Sprintf("block stake difficulty of %d "+
				"is not the expected value of %d", header.SBits,
				expSDiff)
			return ruleError(ErrUnexpectedDifficulty, errStr)
		}

		// Ensure the timestamp for the block header is after the
		// median time of the last several blocks (medianTimeBlocks).
		medianTime, err := b.index.CalcPastMedianTime(prevNode)
		if err != nil {
			log.Errorf("CalcPastMedianTime: %v", err)
			return err
		}
		if !header.Timestamp.After(medianTime) {
			str := "block timestamp of %v is not after expected %v"
			str = fmt.Sprintf(str, header.Timestamp, medianTime)
			return ruleError(ErrTimeTooOld, str)
		}
	}

	// The height of this block is one more than the referenced previous
	// block.
	blockHeight := prevNode.height + 1

	// Ensure the header commits to the correct height based on the height it
	// actually connects in the blockchain.
	if int64(header.Height) != blockHeight {
		errStr := fmt.Sprintf("block header commitment to height %d "+
			"does not match chain height %d", header.Height,
			blockHeight)
		return ruleError(ErrBadBlockHeight, errStr)
	}

	// Ensure chain matches up to predetermined checkpoints.
	blockHash := header.BlockHash()
	if !b.verifyCheckpoint(blockHeight, &blockHash) {
		str := fmt.Sprintf("block at height %d does not match "+
			"checkpoint hash", blockHeight)
		return ruleError(ErrBadCheckpoint, str)
	}

	// Find the previous checkpoint and prevent blocks which fork the main
	// chain before it.  This prevents storage of new, otherwise valid,
	// blocks which build off of old blocks that are likely at a much
	// easier difficulty and therefore could be used to waste cache and
	// disk space.
	checkpointBlock, err := b.findPreviousCheckpoint()
	if err != nil {
		return err
	}
	if checkpointBlock != nil && blockHeight < checkpointBlock.Height() {
		str := fmt.Sprintf("block at height %d forks the main chain "+
			"before the previous checkpoint at height %d",
			blockHeight, checkpointBlock.Height())
		return ruleError(ErrForkTooOld, str)
	}

	if !fastAdd {
		// Reject version 5 blocks for networks other than the main
		// network once a majority of the network has upgraded.
		if b.chainParams.Net != wire.MainNet && header.Version < 6 &&
			b.isMajorityVersion(6, prevNode,
				b.chainParams.BlockRejectNumRequired) {

			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, header.Version)
			return ruleError(ErrBlockVersionTooOld, str)
		}

		// Reject version 4 blocks once a majority of the network has
		// upgraded.
		if header.Version < 5 && b.isMajorityVersion(5, prevNode,
			b.chainParams.BlockRejectNumRequired) {

			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, header.Version)
			return ruleError(ErrBlockVersionTooOld, str)
		}

		// Reject version 3 blocks once a majority of the network has
		// upgraded.
		if header.Version < 4 && b.isMajorityVersion(4, prevNode,
			b.chainParams.BlockRejectNumRequired) {

			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, header.Version)
			return ruleError(ErrBlockVersionTooOld, str)
		}

		// Reject version 2 blocks once a majority of the network has
		// upgraded.
		if header.Version < 3 && b.isMajorityVersion(3, prevNode,
			b.chainParams.BlockRejectNumRequired) {

			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, header.Version)
			return ruleError(ErrBlockVersionTooOld, str)
		}

		// Reject version 1 blocks once a majority of the network has
		// upgraded.
		if header.Version < 2 && b.isMajorityVersion(2, prevNode,
			b.chainParams.BlockRejectNumRequired) {

			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, header.Version)
			return ruleError(ErrBlockVersionTooOld, str)
		}

		// Enforce the stake version in the header once a majority of
		// the network has upgraded to version 3 blocks.
		if header.Version >= 3 && b.isMajorityVersion(3, prevNode,
			b.chainParams.BlockEnforceNumRequired) {

			expectedStakeVer := b.calcStakeVersion(prevNode)
			if header.StakeVersion != expectedStakeVer {
				str := fmt.Sprintf("block stake version of %d "+
					"is not the expected version of %d",
					header.StakeVersion, expectedStakeVer)
				return ruleError(ErrBadStakeVersion, str)
			}
		}

		// Ensure the header commits to the correct pool size based on
		// its position within the chain.
		parentStakeNode, err := b.fetchStakeNode(prevNode)
		if err != nil {
			return err
		}
		calcPoolSize := uint32(parentStakeNode.PoolSize())
		if header.PoolSize != calcPoolSize {
			errStr := fmt.Sprintf("block header commitment to "+
				"pool size %d does not match expected size %d",
				header.PoolSize, calcPoolSize)
			return ruleError(ErrPoolSize, errStr)
		}

		// Ensure the header commits to the correct final state of the
		// ticket lottery.
		calcFinalState := parentStakeNode.FinalState()
		if header.FinalState != calcFinalState {
			errStr := fmt.Sprintf("block header commitment to "+
				"final state of the ticket lottery %x does not "+
				"match expected value %x", header.FinalState,
				calcFinalState)
			return ruleError(ErrInvalidFinalState, errStr)
		}
	}

	return nil
}

// checkAllowedVotes performs validation of all votes in the block to ensure
// they spend tickets that are actually allowed to vote per the lottery.
//
// This function is safe for concurrent access.
func (b *BlockChain) checkAllowedVotes(parentStakeNode *stake.Node, block *wire.MsgBlock) error {
	// Determine the winning ticket hashes and create a map for faster lookup.
	ticketsPerBlock := int(b.chainParams.TicketsPerBlock)
	winningHashes := make(map[chainhash.Hash]struct{}, ticketsPerBlock)
	for _, ticketHash := range parentStakeNode.Winners() {
		winningHashes[ticketHash] = struct{}{}
	}

	for _, stx := range block.STransactions {
		// Ignore non-vote stake transactions.
		if !stake.IsSSGen(stx) {
			continue
		}

		// Ensure the ticket being spent is actually eligible to vote in
		// this block.
		ticketHash := stx.TxIn[1].PreviousOutPoint.Hash
		if _, ok := winningHashes[ticketHash]; !ok {
			errStr := fmt.Sprintf("block contains vote for "+
				"ineligible ticket %s (eligible tickets: %s)",
				ticketHash, winningHashes)
			return ruleError(ErrTicketUnavailable, errStr)
		}
	}

	return nil
}

// checkAllowedRevocations performs validation of all revocations in the block
// to ensure they spend tickets that are actually allowed to be revoked per the
// lottery.  Tickets are only eligible to be revoked if they were missed or have
// expired.
//
// This function is safe for concurrent access.
func (b *BlockChain) checkAllowedRevocations(parentStakeNode *stake.Node, block *wire.MsgBlock) error {
	for _, stx := range block.STransactions {
		// Ignore non-revocation stake transactions.
		if !stake.IsSSRtx(stx) {
			continue
		}

		// Ensure the ticket being spent is actually eligible to be
		// revoked in this block.
		ticketHash := stx.TxIn[0].PreviousOutPoint.Hash
		if !parentStakeNode.ExistsMissedTicket(ticketHash) {
			errStr := fmt.Sprintf("block contains revocation of "+
				"ineligible ticket %s", ticketHash)
			return ruleError(ErrInvalidSSRtx, errStr)
		}
	}

	return nil
}

// checkBlockContext peforms several validation checks on the block which depend
// on its position within the block chain.
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: The transactions are not checked to see if they are finalized
//    and the somewhat expensive duplication transaction check is not performed.
//
// The flags are also passed to checkBlockHeaderContext.  See its documentation
// for how the flags modify its behavior.
func (b *BlockChain) checkBlockContext(block *dcrutil.Block, prevNode *blockNode, flags BehaviorFlags) error {
	// The genesis block is valid by definition.
	if prevNode == nil {
		return nil
	}

	// Perform all block header related validation checks.
	header := &block.MsgBlock().Header
	err := b.checkBlockHeaderContext(header, prevNode, flags)
	if err != nil {
		return err
	}

	fastAdd := flags&BFFastAdd == BFFastAdd
	if !fastAdd {
		// A block must not exceed the maximum allowed size as defined
		// by the network parameters and the current status of any hard
		// fork votes to change it when serialized.
		maxBlockSize, err := b.maxBlockSize(prevNode)
		if err != nil {
			return err
		}
		serializedSize := int64(block.MsgBlock().Header.Size)
		if serializedSize > maxBlockSize {
			str := fmt.Sprintf("serialized block is too big - "+
				"got %d, max %d", serializedSize,
				maxBlockSize)
			return ruleError(ErrBlockTooBig, str)
		}

		// Switch to using the past median time of the block prior to
		// the block being checked for all checks related to lock times
		// once the stake vote for the agenda is active.
		blockTime := header.Timestamp
		lnFeaturesActive, err := b.isLNFeaturesAgendaActive(prevNode)
		if err != nil {
			return err
		}
		if lnFeaturesActive {
			medianTime, err := b.index.CalcPastMedianTime(prevNode)
			if err != nil {
				return err
			}

			blockTime = medianTime
		}

		// The height of this block is one more than the referenced
		// previous block.
		blockHeight := prevNode.height + 1

		// Ensure all transactions in the block are finalized.
		for _, tx := range block.Transactions() {
			if !IsFinalizedTransaction(tx, blockHeight, blockTime) {
				str := fmt.Sprintf("block contains unfinalized regular "+
					"transaction %v", tx.Hash())
				return ruleError(ErrUnfinalizedTx, str)
			}
		}
		for _, stx := range block.STransactions() {
			if !IsFinalizedTransaction(stx, blockHeight, blockTime) {
				str := fmt.Sprintf("block contains unfinalized stake "+
					"transaction %v", stx.Hash())
				return ruleError(ErrUnfinalizedTx, str)
			}
		}

		// Check that the coinbase contains at minimum the block
		// height in output 1.
		if blockHeight > 1 {
			err := checkCoinbaseUniqueHeight(blockHeight, block)
			if err != nil {
				return err
			}
		}

		// Ensure that all votes are only for winning tickets and all
		// revocations are actually eligible to be revoked once stake
		// validation height has been reached.
		if blockHeight >= b.chainParams.StakeValidationHeight {
			parentStakeNode, err := b.fetchStakeNode(prevNode)
			if err != nil {
				return err
			}
			err = b.checkAllowedVotes(parentStakeNode, block.MsgBlock())
			if err != nil {
				return err
			}

			err = b.checkAllowedRevocations(parentStakeNode,
				block.MsgBlock())
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// checkDupTxs ensures blocks do not contain duplicate transactions which
// 'overwrite' older transactions that are not fully spent.  This prevents an
// attack where a coinbase and all of its dependent transactions could be
// duplicated to effectively revert the overwritten transactions to a single
// confirmation thereby making them vulnerable to a double spend.
//
// For more details, see https://en.bitcoin.it/wiki/BIP_0030 and
// http://r6.ca/blog/20120206T005236Z.html.
//
// Decred: Check the stake transactions to make sure they don't have this txid
// too.
func (b *BlockChain) checkDupTxs(txSet []*dcrutil.Tx, view *UtxoViewpoint) error {
	if !chaincfg.CheckForDuplicateHashes {
		return nil
	}

	// Fetch utxo details for all of the transactions in this block.
	// Typically, there will not be any utxos for any of the transactions.
	fetchSet := make(map[chainhash.Hash]struct{})
	for _, tx := range txSet {
		fetchSet[*tx.Hash()] = struct{}{}
	}
	err := view.fetchUtxos(b.db, fetchSet)
	if err != nil {
		return err
	}

	// Duplicate transactions are only allowed if the previous transaction
	// is fully spent.
	for _, tx := range txSet {
		txEntry := view.LookupEntry(tx.Hash())
		if txEntry != nil && !txEntry.IsFullySpent() {
			str := fmt.Sprintf("tried to overwrite transaction %v "+
				"at block height %d that is not fully spent",
				tx.Hash(), txEntry.BlockHeight())
			return ruleError(ErrOverwriteTx, str)
		}
	}

	return nil
}

// CheckTransactionInputs performs a series of checks on the inputs to a
// transaction to ensure they are valid.  An example of some of the checks
// include verifying all inputs exist, ensuring the coinbase seasoning
// requirements are met, detecting double spends, validating all values and
// fees are in the legal range and the total output amount doesn't exceed the
// input amount, and verifying the signatures to prove the spender was the
// owner of the decred and therefore allowed to spend them.  As it checks the
// inputs, it also calculates the total fees for the transaction and returns
// that value.
//
// NOTE: The transaction MUST have already been sanity checked with the
// CheckTransactionSanity function prior to calling this function.
func CheckTransactionInputs(subsidyCache *SubsidyCache, tx *dcrutil.Tx, txHeight int64, utxoView *UtxoViewpoint, checkFraudProof bool, chainParams *chaincfg.Params) (int64, error) {
	msgTx := tx.MsgTx()

	// Expired transactions are not allowed.
	if msgTx.Expiry != wire.NoExpiryValue {
		if txHeight >= int64(msgTx.Expiry) {
			errStr := fmt.Sprintf("Transaction indicated an "+
				"expiry of %v while the current height is %v",
				tx.MsgTx().Expiry, txHeight)
			return 0, ruleError(ErrExpiredTx, errStr)
		}
	}

	ticketMaturity := int64(chainParams.TicketMaturity)
	stakeEnabledHeight := chainParams.StakeEnabledHeight
	txHash := tx.Hash()
	var totalAtomIn int64

	// Coinbase transactions have no inputs.
	if IsCoinBaseTx(msgTx) {
		return 0, nil
	}

	// -------------------------------------------------------------------
	// Decred stake transaction testing.
	// -------------------------------------------------------------------

	// SSTX --------------------------------------------------------------
	// 1. Check and make sure that the output amounts in the commitments to
	//    the ticket are correctly calculated.

	// 1. Check and make sure that the output amounts in the commitments to
	//    the ticket are correctly calculated.
	isSStx := stake.IsSStx(msgTx)
	if isSStx {
		sstxInAmts := make([]int64, len(msgTx.TxIn))

		for idx, txIn := range msgTx.TxIn {
			// Ensure the input is available.
			originTxHash := &txIn.PreviousOutPoint.Hash
			originTxIndex := txIn.PreviousOutPoint.Index
			utxoEntry := utxoView.LookupEntry(originTxHash)
			if utxoEntry == nil || utxoEntry.IsOutputSpent(originTxIndex) {
				str := fmt.Sprintf("output %v referenced from "+
					"transaction %s:%d either does not exist or "+
					"has already been spent", txIn.PreviousOutPoint,
					txHash, idx)
				return 0, ruleError(ErrMissingTxOut, str)
			}

			// Check and make sure that the input is P2PKH or P2SH.
			pkVer := utxoEntry.ScriptVersionByIndex(originTxIndex)
			pkScrpt := utxoEntry.PkScriptByIndex(originTxIndex)
			class := txscript.GetScriptClass(pkVer, pkScrpt)
			if txscript.IsStakeOutput(pkScrpt) {
				class, _ = txscript.GetStakeOutSubclass(pkScrpt)
			}

			if !(class == txscript.PubKeyHashTy ||
				class == txscript.ScriptHashTy) {
				errStr := fmt.Sprintf("SStx input using tx %v"+
					", txout %v referenced a txout that "+
					"was not a PubKeyHashTy or "+
					"ScriptHashTy pkScrpt (class: %v, "+
					"version %v, script %x)", originTxHash,
					originTxIndex, class, pkVer, pkScrpt)
				return 0, ruleError(ErrSStxInScrType, errStr)
			}

			// Get the value of the input.
			sstxInAmts[idx] = utxoEntry.AmountByIndex(originTxIndex)
		}

		_, _, outAmt, chgAmt, _, _ := stake.TxSStxStakeOutputInfo(msgTx)
		_, outAmtCalc, err := stake.SStxNullOutputAmounts(sstxInAmts,
			chgAmt, msgTx.TxOut[0].Value)
		if err != nil {
			return 0, err
		}

		err = stake.VerifySStxAmounts(outAmt, outAmtCalc)
		if err != nil {
			errStr := fmt.Sprintf("SStx output commitment amounts"+
				" were not the same as calculated amounts: %v",
				err)
			return 0, ruleError(ErrSStxCommitment, errStr)
		}
	}

	// SSGEN -------------------------------------------------------------
	// 1. Check SSGen output + rewards to make sure they're in line with
	//    the consensus code and what the outputs are in the original SStx.
	//    Also check to ensure that there is congruency for output PKH from
	//    SStx to SSGen outputs.  Check also that the input transaction was
	//    an SStx.
	// 2. Make sure the second input is an SStx tagged output.
	// 3. Check to make sure that the difference in height between the
	//    current block and the block the SStx was included in is >
	//    ticketMaturity.

	// Save whether or not this is an SSGen tx; if it is, we need to skip
	// the input check of the stakebase later, and another input check for
	// OP_SSTX tagged output uses.
	isSSGen := stake.IsSSGen(msgTx)
	if isSSGen {
		// Cursory check to see if we've even reached stake-enabled
		// height.
		if txHeight < stakeEnabledHeight {
			errStr := fmt.Sprintf("SSGen tx appeared in block "+
				"height %v before stake enabled height %v",
				txHeight, stakeEnabledHeight)
			return 0, ruleError(ErrInvalidEarlyStakeTx, errStr)
		}

		// Grab the input SStx hash from the inputs of the transaction.
		nullIn := msgTx.TxIn[0]
		sstxIn := msgTx.TxIn[1] // sstx input
		sstxHash := sstxIn.PreviousOutPoint.Hash

		// Calculate the theoretical stake vote subsidy by extracting
		// the vote height.  Should be impossible because IsSSGen
		// requires this byte string to be a certain number of bytes.
		_, heightVotingOn := stake.SSGenBlockVotedOn(msgTx)
		stakeVoteSubsidy := CalcStakeVoteSubsidy(subsidyCache,
			int64(heightVotingOn), chainParams)

		// AmountIn for the input should be equal to the stake subsidy.
		if nullIn.ValueIn != stakeVoteSubsidy {
			errStr := fmt.Sprintf("bad stake vote subsidy; got %v"+
				", expect %v", nullIn.ValueIn, stakeVoteSubsidy)
			return 0, ruleError(ErrBadStakebaseAmountIn, errStr)
		}

		// 1. Fetch the input sstx transaction from the txstore and
		//    then check to make sure that the reward has been
		//    calculated correctly from the subsidy and the inputs.
		//
		// We also need to make sure that the SSGen outputs that are
		// P2PKH go to the addresses specified in the original SSTx.
		// Check that too.
		utxoEntrySstx := utxoView.LookupEntry(&sstxHash)
		if utxoEntrySstx == nil {
			str := fmt.Sprintf("ticket output %v referenced from "+
				"transaction %s:%d either does not exist or "+
				"has already been spent", sstxIn.PreviousOutPoint,
				txHash, 1)
			return 0, ruleError(ErrMissingTxOut, str)
		}

		// While we're here, double check to make sure that the input
		// is from an SStx.  By doing so, you also ensure the first
		// output is OP_SSTX tagged.
		if utxoEntrySstx.TransactionType() != stake.TxTypeSStx {
			errStr := fmt.Sprintf("Input transaction %v for SSGen"+
				" was not an SStx tx (given input: %v)", txHash,
				sstxHash)
			return 0, ruleError(ErrInvalidSSGenInput, errStr)
		}

		// Make sure it's using the 0th output.
		if sstxIn.PreviousOutPoint.Index != 0 {
			errStr := fmt.Sprintf("Input transaction %v for SSGen"+
				" did not reference the first output (given "+
				"idx %v)", txHash,
				sstxIn.PreviousOutPoint.Index)
			return 0, ruleError(ErrInvalidSSGenInput, errStr)
		}

		minOutsSStx := ConvertUtxosToMinimalOutputs(utxoEntrySstx)
		if len(minOutsSStx) == 0 {
			return 0, AssertError("missing stake extra data for " +
				"ticket used as input for vote")
		}
		sstxPayTypes, sstxPkhs, sstxAmts, _, sstxRules, sstxLimits :=
			stake.SStxStakeOutputInfo(minOutsSStx)

		ssgenPayTypes, ssgenPkhs, ssgenAmts, err :=
			stake.TxSSGenStakeOutputInfo(msgTx, chainParams)
		if err != nil {
			errStr := fmt.Sprintf("Could not decode outputs for "+
				"SSgen %v: %v", txHash, err)
			return 0, ruleError(ErrSSGenPayeeOuts, errStr)
		}

		// Quick check to make sure the number of SStx outputs is equal
		// to the number of SSGen outputs.
		if (len(sstxPayTypes) != len(ssgenPayTypes)) ||
			(len(sstxPkhs) != len(ssgenPkhs)) ||
			(len(sstxAmts) != len(ssgenAmts)) {
			errStr := fmt.Sprintf("Incongruent payee number for "+
				"SSGen %v and input SStx %v", txHash, sstxHash)
			return 0, ruleError(ErrSSGenPayeeNum, errStr)
		}

		// Get what the stake payouts should be after appending the
		// reward to each output.
		ssgenCalcAmts := stake.CalculateRewards(sstxAmts,
			utxoEntrySstx.AmountByIndex(0), stakeVoteSubsidy)

		// Check that the generated slices for pkhs and amounts are
		// congruent.
		err = stake.VerifyStakingPkhsAndAmounts(sstxPayTypes, sstxPkhs,
			ssgenAmts, ssgenPayTypes, ssgenPkhs, ssgenCalcAmts,
			true /* vote */, sstxRules, sstxLimits)

		if err != nil {
			errStr := fmt.Sprintf("Stake reward consensus "+
				"violation for SStx input %v and SSGen "+
				"output %v: %v", sstxHash, txHash, err)
			return 0, ruleError(ErrSSGenPayeeOuts, errStr)
		}

		// 2. Check to make sure that the second input was an OP_SSTX
		//    tagged output from the referenced SStx.
		if txscript.GetScriptClass(utxoEntrySstx.ScriptVersionByIndex(0),
			utxoEntrySstx.PkScriptByIndex(0)) !=
			txscript.StakeSubmissionTy {
			errStr := fmt.Sprintf("First SStx output in SStx %v "+
				"referenced by SSGen %v should have been "+
				"OP_SSTX tagged, but it was not", sstxHash,
				txHash)
			return 0, ruleError(ErrInvalidSSGenInput, errStr)
		}

		// 3. Check to ensure that ticket maturity number of blocks
		//    have passed between the block the SSGen plans to go into
		//    and the block in which the SStx was originally found in.
		originHeight := utxoEntrySstx.BlockHeight()
		blocksSincePrev := txHeight - originHeight

		// NOTE: You can only spend an OP_SSTX tagged output on the
		// block AFTER the entire range of ticketMaturity has passed,
		// hence <= instead of <.
		if blocksSincePrev <= ticketMaturity {
			errStr := fmt.Sprintf("tried to spend sstx output "+
				"from transaction %v from height %v at height"+
				" %v before required ticket maturity of %v+1 "+
				"blocks", sstxHash, originHeight, txHeight,
				ticketMaturity)
			return 0, ruleError(ErrSStxInImmature, errStr)
		}
	}

	// SSRTX -------------------------------------------------------------
	// 1. Ensure the only input present is an OP_SSTX tagged output, and
	//    that the input transaction is actually an SStx.
	// 2. Ensure that payouts are to the original SStx NullDataTy outputs
	//    in the amounts given there, to the public key hashes given then.
	// 3. Check to make sure that the difference in height between the
	//    current block and the block the SStx was included in is >
	//    ticketMaturity.

	// Save whether or not this is an SSRtx tx; if it is, we need to know
	// this later input check for OP_SSTX outs.
	isSSRtx := stake.IsSSRtx(msgTx)

	if isSSRtx {
		// Cursory check to see if we've even reach stake-enabled
		// height.  Note for an SSRtx to be valid a vote must be
		// missed, so for SSRtx the height of allowance is +1.
		if txHeight < stakeEnabledHeight+1 {
			errStr := fmt.Sprintf("SSRtx tx appeared in block "+
				"height %v before stake enabled height+1 %v",
				txHeight, stakeEnabledHeight+1)
			return 0, ruleError(ErrInvalidEarlyStakeTx, errStr)
		}

		// Grab the input SStx hash from the inputs of the transaction.
		sstxIn := msgTx.TxIn[0] // sstx input
		sstxHash := sstxIn.PreviousOutPoint.Hash

		// 1. Fetch the input sstx transaction from the txstore and
		//    then check to make sure that the reward has been
		//    calculated correctly from the subsidy and the inputs.
		//
		// We also need to make sure that the SSGen outputs that are
		// P2PKH go to the addresses specified in the original SSTx.
		// Check that too.
		utxoEntrySstx := utxoView.LookupEntry(&sstxHash)
		if utxoEntrySstx == nil {
			str := fmt.Sprintf("ticket output %v referenced from "+
				"transaction %s:%d either does not exist or "+
				"has already been spent", sstxIn.PreviousOutPoint,
				txHash, 0)
			return 0, ruleError(ErrMissingTxOut, str)
		}

		// While we're here, double check to make sure that the input
		// is from an SStx.  By doing so, you also ensure the first
		// output is OP_SSTX tagged.
		if utxoEntrySstx.TransactionType() != stake.TxTypeSStx {
			errStr := fmt.Sprintf("Input transaction %v for SSRtx"+
				" %v was not an SStx tx", txHash, sstxHash)
			return 0, ruleError(ErrInvalidSSRtxInput, errStr)
		}

		minOutsSStx := ConvertUtxosToMinimalOutputs(utxoEntrySstx)
		sstxPayTypes, sstxPkhs, sstxAmts, _, sstxRules, sstxLimits :=
			stake.SStxStakeOutputInfo(minOutsSStx)

		// This should be impossible to hit given the strict bytecode
		// size restrictions for components of SSRtxs already checked
		// for in IsSSRtx.
		ssrtxPayTypes, ssrtxPkhs, ssrtxAmts, err :=
			stake.TxSSRtxStakeOutputInfo(msgTx, chainParams)
		if err != nil {
			errStr := fmt.Sprintf("Could not decode outputs for "+
				"SSRtx %v: %v", txHash, err)
			return 0, ruleError(ErrSSRtxPayees, errStr)
		}

		// Quick check to make sure the number of SStx outputs is equal
		// to the number of SSGen outputs.
		if (len(sstxPkhs) != len(ssrtxPkhs)) ||
			(len(sstxAmts) != len(ssrtxAmts)) {
			errStr := fmt.Sprintf("Incongruent payee number for "+
				"SSRtx %v and input SStx %v", txHash, sstxHash)
			return 0, ruleError(ErrSSRtxPayeesMismatch, errStr)
		}

		// Get what the stake payouts should be after appending the
		// reward to each output.
		ssrtxCalcAmts := stake.CalculateRewards(sstxAmts,
			utxoEntrySstx.AmountByIndex(0),
			int64(0)) // SSRtx has no subsidy

		// Check that the generated slices for pkhs and amounts are
		// congruent.
		err = stake.VerifyStakingPkhsAndAmounts(sstxPayTypes, sstxPkhs,
			ssrtxAmts, ssrtxPayTypes, ssrtxPkhs, ssrtxCalcAmts,
			false /* revocation */, sstxRules, sstxLimits)

		if err != nil {
			errStr := fmt.Sprintf("Stake consensus violation for "+
				"SStx input %v and SSRtx output %v: %v",
				sstxHash, txHash, err)
			return 0, ruleError(ErrSSRtxPayees, errStr)
		}

		// 2. Check to make sure that the second input was an OP_SSTX
		//    tagged output from the referenced SStx.
		if txscript.GetScriptClass(utxoEntrySstx.ScriptVersionByIndex(0),
			utxoEntrySstx.PkScriptByIndex(0)) !=
			txscript.StakeSubmissionTy {
			errStr := fmt.Sprintf("First SStx output in SStx %v "+
				"referenced by SSGen %v should have been "+
				"OP_SSTX tagged, but it was not", sstxHash,
				txHash)
			return 0, ruleError(ErrInvalidSSRtxInput, errStr)
		}

		// 3. Check to ensure that ticket maturity number of blocks
		//    have passed between the block the SSRtx plans to go into
		//    and the block in which the SStx was originally found in.
		originHeight := utxoEntrySstx.BlockHeight()
		blocksSincePrev := txHeight - originHeight

		// NOTE: You can only spend an OP_SSTX tagged output on the
		// block AFTER the entire range of ticketMaturity has passed,
		// hence <= instead of <.  Also note that for OP_SSRTX
		// spending, the ticket needs to have been missed, and this
		// can't possibly happen until reaching ticketMaturity + 2.
		if blocksSincePrev <= ticketMaturity+1 {
			errStr := fmt.Sprintf("tried to spend sstx output "+
				"from transaction %v from height %v at height"+
				" %v before required ticket maturity of %v+1 "+
				"blocks", sstxHash, originHeight, txHeight,
				ticketMaturity)
			return 0, ruleError(ErrSStxInImmature, errStr)
		}
	}

	// -------------------------------------------------------------------
	// Decred general transaction testing (and a few stake exceptions).
	// -------------------------------------------------------------------
	for idx, txIn := range msgTx.TxIn {
		// Inputs won't exist for stakebase tx, so ignore them.
		if isSSGen && idx == 0 {
			// However, do add the reward amount.
			_, heightVotingOn := stake.SSGenBlockVotedOn(msgTx)
			stakeVoteSubsidy := CalcStakeVoteSubsidy(subsidyCache,
				int64(heightVotingOn), chainParams)
			totalAtomIn += stakeVoteSubsidy
			continue
		}

		txInHash := &txIn.PreviousOutPoint.Hash
		originTxIndex := txIn.PreviousOutPoint.Index
		utxoEntry := utxoView.LookupEntry(txInHash)
		if utxoEntry == nil || utxoEntry.IsOutputSpent(originTxIndex) {
			str := fmt.Sprintf("output %v referenced from "+
				"transaction %s:%d either does not exist or "+
				"has already been spent", txIn.PreviousOutPoint,
				txHash, idx)
			return 0, ruleError(ErrMissingTxOut, str)
		}

		// Check fraud proof witness data.

		// Using zero value outputs as inputs is banned.
		if utxoEntry.AmountByIndex(originTxIndex) == 0 {
			str := fmt.Sprintf("tried to spend zero value output "+
				"from input %v, idx %v", txInHash,
				originTxIndex)
			return 0, ruleError(ErrZeroValueOutputSpend, str)
		}

		if checkFraudProof {
			if txIn.ValueIn !=
				utxoEntry.AmountByIndex(originTxIndex) {
				str := fmt.Sprintf("bad fraud check value in "+
					"(expected %v, given %v) for txIn %v",
					utxoEntry.AmountByIndex(originTxIndex),
					txIn.ValueIn, idx)
				return 0, ruleError(ErrFraudAmountIn, str)
			}

			if int64(txIn.BlockHeight) != utxoEntry.BlockHeight() {
				str := fmt.Sprintf("bad fraud check block "+
					"height (expected %v, given %v) for "+
					"txIn %v", utxoEntry.BlockHeight(),
					txIn.BlockHeight, idx)
				return 0, ruleError(ErrFraudBlockHeight, str)
			}

			if txIn.BlockIndex != utxoEntry.BlockIndex() {
				str := fmt.Sprintf("bad fraud check block "+
					"index (expected %v, given %v) for "+
					"txIn %v", utxoEntry.BlockIndex(),
					txIn.BlockIndex, idx)
				return 0, ruleError(ErrFraudBlockIndex, str)
			}
		}

		// Ensure the transaction is not spending coins which have not
		// yet reached the required coinbase maturity.
		coinbaseMaturity := int64(chainParams.CoinbaseMaturity)
		originHeight := utxoEntry.BlockHeight()
		if utxoEntry.IsCoinBase() {
			blocksSincePrev := txHeight - originHeight
			if blocksSincePrev < coinbaseMaturity {
				str := fmt.Sprintf("tx %v tried to spend "+
					"coinbase transaction %v from height "+
					"%v at height %v before required "+
					"maturity of %v blocks", txHash,
					txInHash, originHeight, txHeight,
					coinbaseMaturity)
				return 0, ruleError(ErrImmatureSpend, str)
			}
		}

		// Ensure that the transaction is not spending coins from a
		// transaction that included an expiry but which has not yet
		// reached coinbase maturity many blocks.
		if utxoEntry.HasExpiry() {
			originHeight := utxoEntry.BlockHeight()
			blocksSincePrev := txHeight - originHeight
			if blocksSincePrev < coinbaseMaturity {
				str := fmt.Sprintf("tx %v tried to spend "+
					"transaction %v including an expiry "+
					"from height %v at height %v before "+
					"required maturity of %v blocks",
					txHash, txInHash, originHeight,
					txHeight, coinbaseMaturity)
				return 0, ruleError(ErrExpiryTxSpentEarly, str)
			}
		}

		// Ensure that the outpoint's tx tree makes sense.
		originTxOPTree := txIn.PreviousOutPoint.Tree
		originTxType := utxoEntry.TransactionType()
		indicatedTree := wire.TxTreeRegular
		if originTxType != stake.TxTypeRegular {
			indicatedTree = wire.TxTreeStake
		}
		if indicatedTree != originTxOPTree {
			errStr := fmt.Sprintf("tx %v attempted to spend from "+
				"a %v tx tree (hash %v), yet the outpoint "+
				"specified a %v tx tree instead", txHash,
				indicatedTree, txIn.PreviousOutPoint.Hash,
				originTxOPTree)
			return 0, ruleError(ErrDiscordantTxTree, errStr)
		}

		// The only transaction types that are allowed to spend from
		// OP_SSTX tagged outputs are SSGen or SSRtx tx.  So, check all
		// the inputs from non SSGen or SSRtx and make sure that they
		// spend no OP_SSTX tagged outputs.
		if !(isSSGen || isSSRtx) {
			if txscript.GetScriptClass(
				utxoEntry.ScriptVersionByIndex(originTxIndex),
				utxoEntry.PkScriptByIndex(originTxIndex)) ==
				txscript.StakeSubmissionTy {
				errSSGen := stake.CheckSSGen(msgTx)
				errSSRtx := stake.CheckSSRtx(msgTx)
				errStr := fmt.Sprintf("Tx %v attempted to "+
					"spend an OP_SSTX tagged output, "+
					"however it was not an SSGen or SSRtx"+
					" tx; SSGen err: %v, SSRtx err: %v",
					txHash, errSSGen.Error(),
					errSSRtx.Error())
				return 0, ruleError(ErrTxSStxOutSpend, errStr)
			}
		}

		// OP_SSGEN and OP_SSRTX tagged outputs can only be spent after
		// coinbase maturity many blocks.
		scriptClass := txscript.GetScriptClass(
			utxoEntry.ScriptVersionByIndex(originTxIndex),
			utxoEntry.PkScriptByIndex(originTxIndex))
		if scriptClass == txscript.StakeGenTy ||
			scriptClass == txscript.StakeRevocationTy {
			originHeight := utxoEntry.BlockHeight()
			blocksSincePrev := txHeight - originHeight
			if blocksSincePrev <
				int64(chainParams.SStxChangeMaturity) {
				str := fmt.Sprintf("tried to spend OP_SSGEN or"+
					" OP_SSRTX output from tx %v from "+
					"height %v at height %v before "+
					"required maturity of %v blocks",
					txInHash, originHeight, txHeight,
					coinbaseMaturity)
				return 0, ruleError(ErrImmatureSpend, str)
			}
		}

		// SStx change outputs may only be spent after sstx change
		// maturity many blocks.
		if scriptClass == txscript.StakeSubChangeTy {
			originHeight := utxoEntry.BlockHeight()
			blocksSincePrev := txHeight - originHeight
			if blocksSincePrev <
				int64(chainParams.SStxChangeMaturity) {
				str := fmt.Sprintf("tried to spend SStx change"+
					" output from tx %v from height %v at "+
					"height %v before required maturity "+
					"of %v blocks", txInHash, originHeight,
					txHeight, chainParams.SStxChangeMaturity)
				return 0, ruleError(ErrImmatureSpend, str)
			}
		}

		// Ensure the transaction amounts are in range.  Each of the
		// output values of the input transactions must not be negative
		// or more than the max allowed per transaction.  All amounts
		// in a transaction are in a unit value known as an atom.  One
		// decred is a quantity of atoms as defined by the AtomPerCoin
		// constant.
		originTxAtom := utxoEntry.AmountByIndex(originTxIndex)
		if originTxAtom < 0 {
			str := fmt.Sprintf("transaction output has negative "+
				"value of %v", originTxAtom)
			return 0, ruleError(ErrBadTxOutValue, str)
		}
		if originTxAtom > dcrutil.MaxAmount {
			str := fmt.Sprintf("transaction output value of %v is "+
				"higher than max allowed value of %v",
				originTxAtom, dcrutil.MaxAmount)
			return 0, ruleError(ErrBadTxOutValue, str)
		}

		// The total of all outputs must not be more than the max
		// allowed per transaction.  Also, we could potentially
		// overflow the accumulator so check for overflow.
		lastAtomIn := totalAtomIn
		totalAtomIn += originTxAtom
		if totalAtomIn < lastAtomIn ||
			totalAtomIn > dcrutil.MaxAmount {
			str := fmt.Sprintf("total value of all transaction "+
				"inputs is %v which is higher than max "+
				"allowed value of %v", totalAtomIn,
				dcrutil.MaxAmount)
			return 0, ruleError(ErrBadTxOutValue, str)
		}
	}

	// Calculate the total output amount for this transaction.  It is safe
	// to ignore overflow and out of range errors here because those error
	// conditions would have already been caught by checkTransactionSanity.
	var totalAtomOut int64
	for i, txOut := range tx.MsgTx().TxOut {
		totalAtomOut += txOut.Value

		// Double check and make sure that, if this is not a stake
		// transaction, that no outputs have OP code tags OP_SSTX,
		// OP_SSRTX, OP_SSGEN, or OP_SSTX_CHANGE.
		if !isSStx && !isSSGen && !isSSRtx {
			scriptClass := txscript.GetScriptClass(txOut.Version, txOut.PkScript)
			if (scriptClass == txscript.StakeSubmissionTy) ||
				(scriptClass == txscript.StakeGenTy) ||
				(scriptClass == txscript.StakeRevocationTy) ||
				(scriptClass == txscript.StakeSubChangeTy) {
				errStr := fmt.Sprintf("Non-stake tx %v "+
					"included stake output type %v at in "+
					"txout at position %v", txHash,
					scriptClass, i)
				return 0, ruleError(ErrRegTxSpendStakeOut, errStr)
			}

			// Check to make sure that non-stake transactions also
			// are not using stake tagging OP codes anywhere else
			// in their output pkScripts.
			op, err := txscript.ContainsStakeOpCodes(txOut.PkScript)
			if err != nil {
				return 0, ruleError(ErrScriptMalformed,
					err.Error())
			}
			if op {
				errStr := fmt.Sprintf("Non-stake tx %v "+
					"included stake OP code in txout at "+
					"position %v", txHash, i)
				return 0, ruleError(ErrScriptMalformed, errStr)
			}
		}
	}

	// Ensure the transaction does not spend more than its inputs.
	if totalAtomIn < totalAtomOut {
		str := fmt.Sprintf("total value of all transaction inputs for "+
			"transaction %v is %v which is less than the amount "+
			"spent of %v", txHash, totalAtomIn, totalAtomOut)
		return 0, ruleError(ErrSpendTooHigh, str)
	}

	// NOTE: bitcoind checks if the transaction fees are < 0 here, but that
	// is an impossible condition because of the check above that ensures
	// the inputs are >= the outputs.
	txFeeInAtom := totalAtomIn - totalAtomOut

	return txFeeInAtom, nil
}

// CountSigOps returns the number of signature operations for all transaction
// input and output scripts in the provided transaction.  This uses the
// quicker, but imprecise, signature operation counting mechanism from
// txscript.
func CountSigOps(tx *dcrutil.Tx, isCoinBaseTx bool, isSSGen bool) int {
	msgTx := tx.MsgTx()

	// Accumulate the number of signature operations in all transaction
	// inputs.
	totalSigOps := 0
	for i, txIn := range msgTx.TxIn {
		// Skip coinbase inputs.
		if isCoinBaseTx {
			continue
		}
		// Skip stakebase inputs.
		if isSSGen && i == 0 {
			continue
		}

		numSigOps := txscript.GetSigOpCount(txIn.SignatureScript)
		totalSigOps += numSigOps
	}

	// Accumulate the number of signature operations in all transaction
	// outputs.
	for _, txOut := range msgTx.TxOut {
		numSigOps := txscript.GetSigOpCount(txOut.PkScript)
		totalSigOps += numSigOps
	}

	return totalSigOps
}

// CountP2SHSigOps returns the number of signature operations for all input
// transactions which are of the pay-to-script-hash type.  This uses the
// precise, signature operation counting mechanism from the script engine which
// requires access to the input transaction scripts.
func CountP2SHSigOps(tx *dcrutil.Tx, isCoinBaseTx bool, isStakeBaseTx bool, utxoView *UtxoViewpoint) (int, error) {
	// Coinbase transactions have no interesting inputs.
	if isCoinBaseTx {
		return 0, nil
	}

	// Stakebase (SSGen) transactions have no P2SH inputs.  Same with SSRtx,
	// but they will still pass the checks below.
	if isStakeBaseTx {
		return 0, nil
	}

	// Accumulate the number of signature operations in all transaction
	// inputs.
	msgTx := tx.MsgTx()
	totalSigOps := 0
	for txInIndex, txIn := range msgTx.TxIn {
		// Ensure the referenced input transaction is available.
		originTxHash := &txIn.PreviousOutPoint.Hash
		originTxIndex := txIn.PreviousOutPoint.Index
		utxoEntry := utxoView.LookupEntry(originTxHash)
		if utxoEntry == nil || utxoEntry.IsOutputSpent(originTxIndex) {
			str := fmt.Sprintf("output %v referenced from "+
				"transaction %s:%d either does not exist or "+
				"has already been spent", txIn.PreviousOutPoint,
				tx.Hash(), txInIndex)
			return 0, ruleError(ErrMissingTxOut, str)
		}

		// We're only interested in pay-to-script-hash types, so skip
		// this input if it's not one.
		pkScript := utxoEntry.PkScriptByIndex(originTxIndex)
		if !txscript.IsPayToScriptHash(pkScript) {
			continue
		}

		// Count the precise number of signature operations in the
		// referenced public key script.
		sigScript := txIn.SignatureScript
		numSigOps := txscript.GetPreciseSigOpCount(sigScript, pkScript,
			true)

		// We could potentially overflow the accumulator so check for
		// overflow.
		lastSigOps := totalSigOps
		totalSigOps += numSigOps
		if totalSigOps < lastSigOps {
			str := fmt.Sprintf("the public key script from output "+
				"%v contains too many signature operations - "+
				"overflow", txIn.PreviousOutPoint)
			return 0, ruleError(ErrTooManySigOps, str)
		}
	}

	return totalSigOps, nil
}

// checkNumSigOps Checks the number of P2SH signature operations to make
// sure they don't overflow the limits.  It takes a cumulative number of sig
// ops as an argument and increments will each call.
// TxTree true == Regular, false == Stake
func checkNumSigOps(tx *dcrutil.Tx, utxoView *UtxoViewpoint, index int, txTree bool, cumulativeSigOps int) (int, error) {
	msgTx := tx.MsgTx()
	isSSGen := stake.IsSSGen(msgTx)
	numsigOps := CountSigOps(tx, (index == 0) && txTree, isSSGen)

	// Since the first (and only the first) transaction has already been
	// verified to be a coinbase transaction, use (i == 0) && TxTree as an
	// optimization for the flag to countP2SHSigOps for whether or not the
	// transaction is a coinbase transaction rather than having to do a
	// full coinbase check again.
	numP2SHSigOps, err := CountP2SHSigOps(tx, (index == 0) && txTree,
		isSSGen, utxoView)
	if err != nil {
		log.Tracef("CountP2SHSigOps failed; error returned %v", err)
		return 0, err
	}

	startCumSigOps := cumulativeSigOps
	cumulativeSigOps += numsigOps
	cumulativeSigOps += numP2SHSigOps

	// Check for overflow or going over the limits.  We have to do
	// this on every loop iteration to avoid overflow.
	if cumulativeSigOps < startCumSigOps ||
		cumulativeSigOps > MaxSigOpsPerBlock {
		str := fmt.Sprintf("block contains too many signature "+
			"operations - got %v, max %v", cumulativeSigOps,
			MaxSigOpsPerBlock)
		return 0, ruleError(ErrTooManySigOps, str)
	}

	return cumulativeSigOps, nil
}

// checkStakeBaseAmounts calculates the total amount given as subsidy from
// single stakebase transactions (votes) within a block.  This function skips a
// ton of checks already performed by CheckTransactionInputs.
func checkStakeBaseAmounts(subsidyCache *SubsidyCache, height int64, params *chaincfg.Params, txs []*dcrutil.Tx, utxoView *UtxoViewpoint) error {
	for _, tx := range txs {
		msgTx := tx.MsgTx()
		if stake.IsSSGen(msgTx) {
			// Ensure the input is available.
			txInHash := &msgTx.TxIn[1].PreviousOutPoint.Hash
			utxoEntry, exists := utxoView.entries[*txInHash]
			if !exists || utxoEntry == nil {
				str := fmt.Sprintf("couldn't find input tx %v "+
					"for stakebase amounts check", txInHash)
				return ruleError(ErrTicketUnavailable, str)
			}

			originTxIndex := msgTx.TxIn[1].PreviousOutPoint.Index
			originTxAtom := utxoEntry.AmountByIndex(originTxIndex)

			totalOutputs := int64(0)
			// Sum up the outputs.
			for _, out := range msgTx.TxOut {
				totalOutputs += out.Value
			}

			difference := totalOutputs - originTxAtom

			// Subsidy aligns with the height we're voting on, not
			// with the height of the current block.
			calcSubsidy := CalcStakeVoteSubsidy(subsidyCache,
				height-1, params)

			if difference > calcSubsidy {
				str := fmt.Sprintf("ssgen tx %v spent more "+
					"than allowed (spent %v, allowed %v)",
					tx.Hash(), difference, calcSubsidy)
				return ruleError(ErrSSGenSubsidy, str)
			}
		}
	}

	return nil
}

// getStakeBaseAmounts calculates the total amount given as subsidy from the
// collective stakebase transactions (votes) within a block.  This function
// skips a ton of checks already performed by CheckTransactionInputs.
func getStakeBaseAmounts(txs []*dcrutil.Tx, utxoView *UtxoViewpoint) (int64, error) {
	totalInputs := int64(0)
	totalOutputs := int64(0)
	for _, tx := range txs {
		msgTx := tx.MsgTx()
		if stake.IsSSGen(msgTx) {
			// Ensure the input is available.
			txInHash := &msgTx.TxIn[1].PreviousOutPoint.Hash
			utxoEntry, exists := utxoView.entries[*txInHash]
			if !exists || utxoEntry == nil {
				str := fmt.Sprintf("couldn't find input tx %v "+
					"for stakebase amounts get", txInHash)
				return 0, ruleError(ErrTicketUnavailable, str)
			}

			originTxIndex := msgTx.TxIn[1].PreviousOutPoint.Index
			originTxAtom := utxoEntry.AmountByIndex(originTxIndex)

			totalInputs += originTxAtom

			// Sum up the outputs.
			for _, out := range msgTx.TxOut {
				totalOutputs += out.Value
			}
		}
	}

	return totalOutputs - totalInputs, nil
}

// getStakeTreeFees determines the amount of fees for in the stake tx tree of
// some node given a transaction store.
func getStakeTreeFees(subsidyCache *SubsidyCache, height int64, params *chaincfg.Params, txs []*dcrutil.Tx, utxoView *UtxoViewpoint) (dcrutil.Amount, error) {
	totalInputs := int64(0)
	totalOutputs := int64(0)
	for _, tx := range txs {
		msgTx := tx.MsgTx()
		isSSGen := stake.IsSSGen(msgTx)

		for i, in := range msgTx.TxIn {
			// Ignore stakebases.
			if isSSGen && i == 0 {
				continue
			}

			txInHash := &in.PreviousOutPoint.Hash
			utxoEntry, exists := utxoView.entries[*txInHash]
			if !exists || utxoEntry == nil {
				str := fmt.Sprintf("couldn't find input tx "+
					"%v for stake tree fee calculation",
					txInHash)
				return 0, ruleError(ErrTicketUnavailable, str)
			}

			originTxIndex := in.PreviousOutPoint.Index
			originTxAtom := utxoEntry.AmountByIndex(originTxIndex)

			totalInputs += originTxAtom
		}

		for _, out := range msgTx.TxOut {
			totalOutputs += out.Value
		}

		// For votes, subtract the subsidy to determine actual fees.
		if isSSGen {
			// Subsidy aligns with the height we're voting on, not
			// with the height of the current block.
			totalOutputs -= CalcStakeVoteSubsidy(subsidyCache,
				height-1, params)
		}
	}

	if totalInputs < totalOutputs {
		str := fmt.Sprintf("negative cumulative fees found in stake " +
			"tx tree")
		return 0, ruleError(ErrStakeFees, str)
	}

	return dcrutil.Amount(totalInputs - totalOutputs), nil
}

// checkTransactionsAndConnect is the local function used to check the
// transaction inputs for a transaction list given a predetermined TxStore.
// After ensuring the transaction is valid, the transaction is connected to the
// UTXO viewpoint.  TxTree true == Regular, false == Stake
func (b *BlockChain) checkTransactionsAndConnect(subsidyCache *SubsidyCache, inputFees dcrutil.Amount, node *blockNode, txs []*dcrutil.Tx, utxoView *UtxoViewpoint, stxos *[]spentTxOut, txTree bool) error {
	// Perform several checks on the inputs for each transaction.  Also
	// accumulate the total fees.  This could technically be combined with
	// the loop above instead of running another loop over the
	// transactions, but by separating it we can avoid running the more
	// expensive (though still relatively cheap as compared to running the
	// scripts) checks against all the inputs when the signature operations
	// are out of bounds.
	totalFees := int64(inputFees) // Stake tx tree carry forward
	var cumulativeSigOps int
	for idx, tx := range txs {
		// Ensure that the number of signature operations is not beyond
		// the consensus limit.
		var err error
		cumulativeSigOps, err = checkNumSigOps(tx, utxoView, idx,
			txTree, cumulativeSigOps)
		if err != nil {
			return err
		}

		// This step modifies the txStore and marks the tx outs used
		// spent, so be aware of this.
		txFee, err := CheckTransactionInputs(b.subsidyCache, tx,
			node.height, utxoView, true, /* check fraud proofs */
			b.chainParams)
		if err != nil {
			log.Tracef("CheckTransactionInputs failed; error "+
				"returned: %v", err)
			return err
		}

		// Sum the total fees and ensure we don't overflow the
		// accumulator.
		lastTotalFees := totalFees
		totalFees += txFee
		if totalFees < lastTotalFees {
			return ruleError(ErrBadFees, "total fees for block "+
				"overflows accumulator")
		}

		// Connect the transaction to the UTXO viewpoint, so that in
		// flight transactions may correctly validate.
		err = utxoView.connectTransaction(tx, node.height, uint32(idx),
			stxos)
		if err != nil {
			return err
		}
	}

	// The total output values of the coinbase transaction must not exceed
	// the expected subsidy value plus total transaction fees gained from
	// mining the block.  It is safe to ignore overflow and out of range
	// errors here because those error conditions would have already been
	// caught by checkTransactionSanity.
	if txTree { //TxTreeRegular
		// Apply penalty to fees if we're at stake validation height.
		if node.height >= b.chainParams.StakeValidationHeight {
			totalFees *= int64(node.voters)
			totalFees /= int64(b.chainParams.TicketsPerBlock)
		}

		var totalAtomOutRegular int64

		for _, txOut := range txs[0].MsgTx().TxOut {
			totalAtomOutRegular += txOut.Value
		}

		var expAtomOut int64
		if node.height == 1 {
			expAtomOut = subsidyCache.CalcBlockSubsidy(node.height)
		} else {
			subsidyWork := CalcBlockWorkSubsidy(subsidyCache,
				node.height, node.voters, b.chainParams)
			subsidyTax := CalcBlockTaxSubsidy(subsidyCache,
				node.height, node.voters, b.chainParams)
			expAtomOut = subsidyWork + subsidyTax + totalFees
		}

		// AmountIn for the input should be equal to the subsidy.
		coinbaseIn := txs[0].MsgTx().TxIn[0]
		subsidyWithoutFees := expAtomOut - totalFees
		if (coinbaseIn.ValueIn != subsidyWithoutFees) &&
			(node.height > 0) {
			errStr := fmt.Sprintf("bad coinbase subsidy in input;"+
				" got %v, expected %v", coinbaseIn.ValueIn,
				subsidyWithoutFees)
			return ruleError(ErrBadCoinbaseAmountIn, errStr)
		}

		if totalAtomOutRegular > expAtomOut {
			str := fmt.Sprintf("coinbase transaction for block %v"+
				" pays %v which is more than expected value "+
				"of %v", node.hash, totalAtomOutRegular,
				expAtomOut)
			return ruleError(ErrBadCoinbaseValue, str)
		}
	} else { // TxTreeStake
		if len(txs) == 0 &&
			node.height < b.chainParams.StakeValidationHeight {
			return nil
		}
		if len(txs) == 0 &&
			node.height >= b.chainParams.StakeValidationHeight {
			str := fmt.Sprintf("empty tx tree stake in block " +
				"after stake validation height")
			return ruleError(ErrNoStakeTx, str)
		}

		err := checkStakeBaseAmounts(subsidyCache, node.height,
			b.chainParams, txs, utxoView)
		if err != nil {
			return err
		}

		totalAtomOutStake, err := getStakeBaseAmounts(txs, utxoView)
		if err != nil {
			return err
		}

		var expAtomOut int64
		if node.height >= b.chainParams.StakeValidationHeight {
			// Subsidy aligns with the height we're voting on, not
			// with the height of the current block.
			expAtomOut = CalcStakeVoteSubsidy(subsidyCache,
				node.height-1, b.chainParams) *
				int64(node.voters)
		} else {
			expAtomOut = totalFees
		}

		if totalAtomOutStake > expAtomOut {
			str := fmt.Sprintf("stakebase transactions for block "+
				"pays %v which is more than expected value "+
				"of %v", totalAtomOutStake, expAtomOut)
			return ruleError(ErrBadStakebaseValue, str)
		}
	}

	return nil
}

// consensusScriptVerifyFlags returns the script flags that must be used when
// executing transaction scripts to enforce the consensus rules. This includes
// any flags required as the result of any agendas that have passed and become
// active.
func (b *BlockChain) consensusScriptVerifyFlags(node *blockNode) (txscript.ScriptFlags, error) {
	scriptFlags := txscript.ScriptBip16 |
		txscript.ScriptVerifyDERSignatures |
		txscript.ScriptVerifyStrictEncoding |
		txscript.ScriptVerifyMinimalData |
		txscript.ScriptVerifyCleanStack |
		txscript.ScriptVerifyCheckLockTimeVerify

	// Enable enforcement of OP_CSV and OP_SHA256 if the stake vote
	// for the agenda is active.
	lnFeaturesActive, err := b.isLNFeaturesAgendaActive(node.parent)
	if err != nil {
		return 0, err
	}
	if lnFeaturesActive {
		scriptFlags |= txscript.ScriptVerifyCheckSequenceVerify
		scriptFlags |= txscript.ScriptVerifySHA256
	}
	return scriptFlags, err
}

// checkConnectBlock performs several checks to confirm connecting the passed
// block to the chain represented by the passed view does not violate any
// rules.  In addition, the passed view is updated to spend all of the
// referenced outputs and add all of the new utxos created by block.  Thus, the
// view will represent the state of the chain as if the block were actually
// connected and consequently the best hash for the view is also updated to
// passed block.
//
// The CheckConnectBlock function makes use of this function to perform the
// bulk of its work.  The only difference is this function accepts a node which
// may or may not require reorganization to connect it to the main chain
// whereas CheckConnectBlock creates a new node which specifically connects to
// the end of the current main chain and then calls this function with that
// node.
//
// See the comments for CheckConnectBlock for some examples of the type of
// checks performed by this function.
//
// This function MUST be called with the chain state lock held (for writes).
func (b *BlockChain) checkConnectBlock(node *blockNode, block, parent *dcrutil.Block, utxoView *UtxoViewpoint, stxos *[]spentTxOut) error {
	// If the side chain blocks end up in the database, a call to
	// CheckBlockSanity should be done here in case a previous version
	// allowed a block that is no longer valid.  However, since the
	// implementation only currently uses memory for the side chain blocks,
	// it isn't currently necessary.

	// The coinbase for the Genesis block is not spendable, so just return
	// an error now.
	if node.hash.IsEqual(b.chainParams.GenesisHash) {
		str := "the coinbase for the genesis block is not spendable"
		return ruleError(ErrMissingTxOut, str)
	}

	// Ensure the view is for the node being checked.
	if !utxoView.BestHash().IsEqual(&node.parentHash) {
		return AssertError(fmt.Sprintf("inconsistent view when "+
			"checking block connection: best hash is %v instead "+
			"of expected %v", utxoView.BestHash(),
			node.parentHash))
	}

	// Check that the coinbase pays the tax, if applicable.
	err := CoinbasePaysTax(b.subsidyCache, block.Transactions()[0], node.height,
		node.voters, b.chainParams)
	if err != nil {
		return err
	}

	// Don't run scripts if this node is before the latest known good
	// checkpoint since the validity is verified via the checkpoints (all
	// transactions are included in the merkle root hash and any changes
	// will therefore be detected by the next checkpoint).  This is a huge
	// optimization because running the scripts is the most time consuming
	// portion of block handling.
	checkpoint := b.latestCheckpoint()
	runScripts := !b.noVerify
	if checkpoint != nil && node.height <= checkpoint.Height {
		runScripts = false
	}
	var scriptFlags txscript.ScriptFlags
	if runScripts {
		var err error
		scriptFlags, err = b.consensusScriptVerifyFlags(node)
		if err != nil {
			return err
		}
	}

	// The number of signature operations must be less than the maximum
	// allowed per block.  Note that the preliminary sanity checks on a
	// block also include a check similar to this one, but this check
	// expands the count to include a precise count of pay-to-script-hash
	// signature operations in each of the input transaction public key
	// scripts.
	// Do this for all TxTrees.
	regularTxTreeValid := voteBitsApproveParent(node.voteBits)
	thisNodeStakeViewpoint := ViewpointPrevInvalidStake
	thisNodeRegularViewpoint := ViewpointPrevInvalidRegular
	if regularTxTreeValid {
		thisNodeStakeViewpoint = ViewpointPrevValidStake
		thisNodeRegularViewpoint = ViewpointPrevValidRegular

		utxoView.SetStakeViewpoint(ViewpointPrevValidInitial)
		err = utxoView.fetchInputUtxos(b.db, block, parent)
		if err != nil {
			return err
		}

		for i, tx := range parent.Transactions() {
			err := utxoView.connectTransaction(tx,
				node.parent.height, uint32(i), stxos)
			if err != nil {
				return err
			}
		}
	}

	// TxTreeStake of current block.
	utxoView.SetStakeViewpoint(thisNodeStakeViewpoint)
	err = b.checkDupTxs(block.STransactions(), utxoView)
	if err != nil {
		log.Tracef("checkDupTxs failed for cur TxTreeStake: %v", err)
		return err
	}

	err = utxoView.fetchInputUtxos(b.db, block, parent)
	if err != nil {
		return err
	}

	err = b.checkTransactionsAndConnect(b.subsidyCache, 0, node,
		block.STransactions(), utxoView, stxos, false)
	if err != nil {
		log.Tracef("checkTransactionsAndConnect failed for "+
			"TxTreeStake: %v", err)
		return err
	}

	stakeTreeFees, err := getStakeTreeFees(b.subsidyCache, node.height,
		b.chainParams, block.STransactions(), utxoView)
	if err != nil {
		log.Tracef("getStakeTreeFees failed for TxTreeStake: %v", err)
		return err
	}

	// Enforce all relative lock times via sequence numbers for the regular
	// transaction tree once the stake vote for the agenda is active.
	var prevMedianTime time.Time
	lnFeaturesActive, err := b.isLNFeaturesAgendaActive(node.parent)
	if err != nil {
		return err
	}
	if lnFeaturesActive {
		// Use the past median time of the *previous* block in order
		// to determine if the transactions in the current block are
		// final.
		prevMedianTime, err = b.index.CalcPastMedianTime(node.parent)
		if err != nil {
			return err
		}

		// Skip the coinbase since it does not have any inputs and thus
		// lock times do not apply.
		for _, tx := range block.Transactions()[1:] {
			sequenceLock, err := b.calcSequenceLock(node, tx,
				utxoView, true)
			if err != nil {
				return err
			}
			if !SequenceLockActive(sequenceLock, node.height,
				prevMedianTime) {

				str := fmt.Sprintf("block contains " +
					"transaction whose input sequence " +
					"locks are not met")
				return ruleError(ErrUnfinalizedTx, str)
			}
		}
	}

	if runScripts {
		err = checkBlockScripts(block, utxoView, false, scriptFlags,
			b.sigCache)
		if err != nil {
			log.Tracef("checkBlockScripts failed; error returned "+
				"on txtreestake of cur block: %v", err)
			return err
		}
	}

	// TxTreeRegular of current block. At this point, the stake
	// transactions have already added, so set this to the correct stake
	// viewpoint and disable automatic connection.
	utxoView.SetStakeViewpoint(thisNodeRegularViewpoint)
	err = b.checkDupTxs(block.Transactions(), utxoView)
	if err != nil {
		log.Tracef("checkDupTxs failed for cur TxTreeRegular: %v", err)
		return err
	}

	err = utxoView.fetchInputUtxos(b.db, block, parent)
	if err != nil {
		return err
	}

	err = b.checkTransactionsAndConnect(b.subsidyCache, stakeTreeFees, node,
		block.Transactions(), utxoView, stxos, true)
	if err != nil {
		log.Tracef("checkTransactionsAndConnect failed for cur "+
			"TxTreeRegular: %v", err)
		return err
	}

	// Enforce all relative lock times via sequence numbers for the stake
	// transaction tree once the stake vote for the agenda is active.
	if lnFeaturesActive {
		for _, stx := range block.STransactions() {
			sequenceLock, err := b.calcSequenceLock(node, stx,
				utxoView, true)
			if err != nil {
				return err
			}
			if !SequenceLockActive(sequenceLock, node.height,
				prevMedianTime) {

				str := fmt.Sprintf("block contains " +
					"stake transaction whose input " +
					"sequence locks are not met")
				return ruleError(ErrUnfinalizedTx, str)
			}
		}
	}

	if runScripts {
		err = checkBlockScripts(block, utxoView, true,
			scriptFlags, b.sigCache)
		if err != nil {
			log.Tracef("checkBlockScripts failed; error returned "+
				"on txtreeregular of cur block: %v", err)
			return err
		}
	}

	// Rollback the final tx tree regular so that we don't write it to
	// database.
	if node.height > 1 && stxos != nil {
		idx, err := utxoView.disconnectTransactionSlice(block.Transactions(),
			node.height, stxos)
		if err != nil {
			return err
		}
		stxosDeref := *stxos
		*stxos = stxosDeref[0:idx]
	}

	// First block has special rules concerning the ledger.
	if node.height == 1 {
		err := BlockOneCoinbasePaysTokens(block.Transactions()[0],
			b.chainParams)
		if err != nil {
			return err
		}
	}

	// Update the best hash for view to include this block since all of its
	// transactions have been connected.
	utxoView.SetBestHash(&node.hash)

	return nil
}

// CheckConnectBlock performs several checks to confirm connecting the passed
// block to the main chain does not violate any rules.  An example of some of
// the checks performed are ensuring connecting the block would not cause any
// duplicate transaction hashes for old transactions that aren't already fully
// spent, double spends, exceeding the maximum allowed signature operations per
// block, invalid values in relation to the expected block subsidy, or fail
// transaction script validation.
//
// The flags modify the behavior of this function as follows:
//  - BFNoPoWCheck: The check to ensure the block hash is less than the target
//    difficulty is not performed.
//  - BFFastAdd: The transactions are not checked to see if they are finalized
//    and the somewhat expensive duplication transaction check is not performed.
//
// This function is safe for concurrent access.
func (b *BlockChain) CheckConnectBlock(block *dcrutil.Block, flags BehaviorFlags) error {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()

	parentHash := block.MsgBlock().Header.PrevBlock
	prevNode, err := b.findNode(&parentHash, maxSearchDepth)
	if err != nil {
		return ruleError(ErrMissingParent, err.Error())
	}

	// Perform context-free sanity checks on the block and its transactions.
	err = checkBlockSanity(block, b.timeSource, flags, b.chainParams)
	if err != nil {
		return err
	}

	// The block must pass all of the validation rules which depend on the
	// position of the block within the block chain.
	err = b.checkBlockContext(block, prevNode, flags)
	if err != nil {
		return err
	}

	newNode := newBlockNode(&block.MsgBlock().Header, prevNode)
	newNode.populateTicketInfo(stake.FindSpentTicketsInBlock(block.MsgBlock()))

	// If we are extending the main (best) chain with a new block, just use
	// the ticket database we already have.
	if b.bestNode == nil || (prevNode != nil &&
		prevNode.hash == b.bestNode.hash) {

		// Grab the parent block since it is required throughout the block
		// connection process.
		parent, err := b.fetchMainChainBlockByHash(&parentHash)
		if err != nil {
			return ruleError(ErrMissingParent, err.Error())
		}

		view := NewUtxoViewpoint()
		view.SetBestHash(&prevNode.hash)
		return b.checkConnectBlock(newNode, block, parent, view, nil)
	}

	// The requested node is either on a side chain or is a node on the
	// main chain before the end of it.  In either case, we need to undo
	// the transactions and spend information for the blocks which would be
	// disconnected during a reorganize to the point of view of the node
	// just before the requested node.
	detachNodes, attachNodes, err := b.getReorganizeNodes(prevNode)
	if err != nil {
		return err
	}

	view := NewUtxoViewpoint()
	view.SetBestHash(&b.bestNode.hash)
	view.SetStakeViewpoint(ViewpointPrevValidInitial)
	var stxos []spentTxOut
	var nextBlockToDetach *dcrutil.Block
	for e := detachNodes.Front(); e != nil; e = e.Next() {
		// Grab the block to detach based on the node.  Use the fact that the
		// parent of the block is already required, and the next block to detach
		// will also be the parent to optimize.
		n := e.Value.(*blockNode)
		block := nextBlockToDetach
		if block == nil {
			var err error
			block, err = b.fetchMainChainBlockByHash(&n.hash)
			if err != nil {
				return err
			}
		}
		if n.hash != *block.Hash() {
			return AssertError(fmt.Sprintf("detach block node hash %v (height "+
				"%v) does not match previous parent block hash %v", &n.hash,
				n.height, block.Hash()))
		}

		parent, err := b.fetchMainChainBlockByHash(&n.parentHash)
		if err != nil {
			return err
		}
		nextBlockToDetach = parent

		// Load all of the spent txos for the block from the spend journal.
		err = b.db.View(func(dbTx database.Tx) error {
			stxos, err = dbFetchSpendJournalEntry(dbTx, block, parent)
			return err
		})
		if err != nil {
			return err
		}

		err = b.disconnectTransactions(view, block, parent, stxos)
		if err != nil {
			return err
		}
	}

	// The UTXO viewpoint is now accurate to either the node where the
	// requested node forks off the main chain (in the case where the
	// requested node is on a side chain), or the requested node itself if
	// the requested node is an old node on the main chain.  Entries in the
	// attachNodes list indicate the requested node is on a side chain, so
	// if there are no nodes to attach, we're done.
	if attachNodes.Len() == 0 {
		// Grab the parent block since it is required throughout the block
		// connection process.
		parent, err := b.fetchMainChainBlockByHash(&parentHash)
		if err != nil {
			return ruleError(ErrMissingParent, err.Error())
		}

		view.SetBestHash(&parentHash)
		return b.checkConnectBlock(newNode, block, parent, view, nil)
	}

	// The requested node is on a side chain, so we need to apply the
	// transactions and spend information from each of the nodes to attach.
	var prevAttachBlock *dcrutil.Block
	for e := attachNodes.Front(); e != nil; e = e.Next() {
		// Grab the block to attach based on the node.  Use the fact that the
		// parent of the block is either the fork point for the first node being
		// attached or the previous one that was attached for subsequent blocks
		// to optimize.
		n := e.Value.(*blockNode)
		block, err := b.fetchBlockByHash(&n.hash)
		if err != nil {
			return err
		}
		parent := prevAttachBlock
		if parent == nil {
			var err error
			parent, err = b.fetchMainChainBlockByHash(&n.parentHash)
			if err != nil {
				return err
			}
		}
		if n.parentHash != *parent.Hash() {
			return AssertError(fmt.Sprintf("attach block node hash %v (height "+
				"%v) parent hash %v does not match previous parent block "+
				"hash %v", &n.hash, n.height, &n.parentHash, parent.Hash()))
		}

		// Store the loaded block for the next iteration.
		prevAttachBlock = block

		err = b.connectTransactions(view, block, parent, &stxos)
		if err != nil {
			return err
		}
	}

	// Grab the parent block since it is required throughout the block
	// connection process.
	parent, err := b.fetchBlockByHash(&parentHash)
	if err != nil {
		return ruleError(ErrMissingParent, err.Error())
	}

	view.SetBestHash(&parentHash)
	return b.checkConnectBlock(newNode, block, parent, view, &stxos)
}
