// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"errors"
	"fmt"
	"math"
	"math/big"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

const (
	// MaxSigOpsPerBlock is the maximum number of signature operations
	// allowed for a block.  It is a fraction of the max block payload size.
	MaxSigOpsPerBlock = wire.MaxBlockPayload / 200

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
)

var (
	// zeroHash is the zero value for a wire.ShaHash and is defined as
	// a package level variable to avoid the need to create a new instance
	// every time a check is needed.
	zeroHash = &chainhash.Hash{}
)

// isNullOutpoint determines whether or not a previous transaction output point
// is set.
func isNullOutpoint(outpoint *wire.OutPoint) bool {
	if outpoint.Index == math.MaxUint32 && outpoint.Hash.IsEqual(zeroHash) &&
		outpoint.Tree == dcrutil.TxTreeRegular {
		return true
	}
	return false
}

// isNullFraudProof determines whether or not a previous transaction fraud proof
// is set.
func isNullFraudProof(txIn *wire.TxIn) bool {
	switch {
	case txIn.BlockHeight != wire.NullBlockHeight:
		return false
	case txIn.BlockIndex != wire.NullBlockIndex:
		return false
	}

	return true
}

// IsCoinBaseTx determines whether or not a transaction is a coinbase.  A coinbase
// is a special transaction created by miners that has no inputs.  This is
// represented in the block chain by a transaction with a single input that has
// a previous output transaction index set to the maximum value along with a
// zero hash.
//
// This function only differs from IsCoinBase in that it works with a raw wire
// transaction as opposed to a higher level util transaction.
func IsCoinBaseTx(msgTx *wire.MsgTx) bool {
	// A coin base must only have one transaction input.
	if len(msgTx.TxIn) != 1 {
		return false
	}

	// The previous output of a coin base must have a max value index and
	// a zero hash.
	prevOut := &msgTx.TxIn[0].PreviousOutPoint
	if prevOut.Index != math.MaxUint32 || !prevOut.Hash.IsEqual(zeroHash) {
		return false
	}

	return true
}

// IsCoinBase determines whether or not a transaction is a coinbase.  A coinbase
// is a special transaction created by miners that has no inputs.  This is
// represented in the block chain by a transaction with a single input that has
// a previous output transaction index set to the maximum value along with a
// zero hash.
//
// This function only differs from IsCoinBaseTx in that it works with a higher
// level util transaction as opposed to a raw wire transaction.
func IsCoinBase(tx *dcrutil.Tx) bool {
	return IsCoinBaseTx(tx.MsgTx())
}

// CheckTransactionSanity performs some preliminary checks on a transaction to
// ensure it is sane.  These checks are context free.
func CheckTransactionSanity(tx *dcrutil.Tx, params *chaincfg.Params) error {
	// A transaction must have at least one input.
	msgTx := tx.MsgTx()
	if len(msgTx.TxIn) == 0 {
		return ruleError(ErrNoTxInputs, "transaction has no inputs")
	}

	// A transaction must have at least one output.
	if len(msgTx.TxOut) == 0 {
		return ruleError(ErrNoTxOutputs, "transaction has no outputs")
	}

	// A transaction must not exceed the maximum allowed block payload when
	// serialized.
	serializedTxSize := tx.MsgTx().SerializeSize()
	if serializedTxSize > params.MaximumBlockSize {
		str := fmt.Sprintf("serialized transaction is too big - got "+
			"%d, max %d", serializedTxSize, params.MaximumBlockSize)
		return ruleError(ErrTxTooBig, str)
	}

	// Ensure the transaction amounts are in range.  Each transaction
	// output must not be negative or more than the max allowed per
	// transaction.  Also, the total of all outputs must abide by the same
	// restrictions.  All amounts in a transaction are in a unit value known
	// as a atom.  One decred is a quantity of atoms as defined by the
	// AtomsPerCoin constant.
	var totalAtom int64
	for _, txOut := range msgTx.TxOut {
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
		// is detected and reported.  This is impossible for Decred, but
		// perhaps possible if an alt increases the total money supply.
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

	// Check for duplicate transaction inputs.
	existingTxOut := make(map[wire.OutPoint]struct{})
	for _, txIn := range msgTx.TxIn {
		if _, exists := existingTxOut[txIn.PreviousOutPoint]; exists {
			return ruleError(ErrDuplicateTxInputs, "transaction "+
				"contains duplicate inputs")
		}
		existingTxOut[txIn.PreviousOutPoint] = struct{}{}
	}

	isSSGen, _ := stake.IsSSGen(tx)

	// Coinbase script length must be between min and max length.
	if IsCoinBase(tx) {
		// The referenced outpoint should be null.
		if !isNullOutpoint(&msgTx.TxIn[0].PreviousOutPoint) {
			str := fmt.Sprintf("coinbase transaction did not use a " +
				"null outpoint")
			return ruleError(ErrBadCoinbaseOutpoint, str)
		}

		// The fraud proof should also be null.
		if !isNullFraudProof(msgTx.TxIn[0]) {
			str := fmt.Sprintf("coinbase transaction fraud proof was " +
				"non-null")
			return ruleError(ErrBadCoinbaseFraudProof, str)
		}

		slen := len(msgTx.TxIn[0].SignatureScript)
		if slen < MinCoinbaseScriptLen || slen > MaxCoinbaseScriptLen {
			str := fmt.Sprintf("coinbase transaction script length "+
				"of %d is out of range (min: %d, max: %d)",
				slen, MinCoinbaseScriptLen, MaxCoinbaseScriptLen)
			return ruleError(ErrBadCoinbaseScriptLen, str)
		}
	} else if isSSGen {
		// Check script length of stake base signature.
		slen := len(msgTx.TxIn[0].SignatureScript)
		if slen < MinCoinbaseScriptLen || slen > MaxCoinbaseScriptLen {
			str := fmt.Sprintf("stakebase transaction script length "+
				"of %d is out of range (min: %d, max: %d)",
				slen, MinCoinbaseScriptLen, MaxCoinbaseScriptLen)
			return ruleError(ErrBadStakebaseScriptLen, str)
		}

		// The script must be set to the one specified by the network.
		// Check script length of stake base signature.
		if !bytes.Equal(msgTx.TxIn[0].SignatureScript,
			params.StakeBaseSigScript) {
			str := fmt.Sprintf("stakebase transaction signature script "+
				"was set to disallowed value (got %x, want %x)",
				msgTx.TxIn[0].SignatureScript,
				params.StakeBaseSigScript)
			return ruleError(ErrBadStakevaseScrVal, str)
		}

		// The ticket reference hash in an SSGen tx must not be null.
		ticketHash := &msgTx.TxIn[1].PreviousOutPoint
		if isNullOutpoint(ticketHash) {
			return ruleError(ErrBadTxInput, "ssgen tx "+
				"ticket input refers to previous output that "+
				"is null")
		}
	} else {
		// Previous transaction outputs referenced by the inputs to this
		// transaction must not be null except in the case of stake bases
		// for SSGen tx.
		for _, txIn := range msgTx.TxIn {
			prevOut := &txIn.PreviousOutPoint
			if isNullOutpoint(prevOut) {
				return ruleError(ErrBadTxInput, "transaction "+
					"input refers to previous output that "+
					"is null")
			}
		}
	}

	return nil
}

// checkProofOfStake checks to see that all new SStx tx in a block are actually at
// the network stake target.
func checkProofOfStake(block *dcrutil.Block, posLimit int64) error {
	msgBlock := block.MsgBlock()
	for _, staketx := range block.STransactions() {
		if is, _ := stake.IsSStx(staketx); is {
			commitValue := staketx.MsgTx().TxOut[0].Value

			// Check for underflow block sbits.
			if commitValue < msgBlock.Header.SBits {
				errStr := fmt.Sprintf("Stake tx %v has a commitment value "+
					"less than the minimum stake difficulty specified in "+
					"the block (%v)",
					staketx.Sha(), msgBlock.Header.SBits)
				return ruleError(ErrNotEnoughStake, errStr)
			}

			// Check if it's above the PoS limit.
			if commitValue < posLimit {
				errStr := fmt.Sprintf("Stake tx %v has a commitment value "+
					"less than the minimum stake difficulty for the "+
					"network (%v)",
					staketx.Sha(), posLimit)
				return ruleError(ErrStakeBelowMinimum, errStr)
			}
		}
	}

	return nil
}

// CheckProofOfStake exports the above func.
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
func checkProofOfWork(header *wire.BlockHeader, powLimit *big.Int,
	flags BehaviorFlags) error {
	// The target difficulty must be larger than zero.
	target := CompactToBig(header.Bits)
	if target.Sign() <= 0 {
		str := fmt.Sprintf("block target difficulty of %064x is too low",
			target)
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
		hash := header.BlockSha()
		hashNum := ShaHashToBig(&hash)
		if hashNum.Cmp(target) > 0 {
			str := fmt.Sprintf("block hash of %064x is higher than "+
				"expected max of %064x", hashNum, target)
			return ruleError(ErrHighHash, str)
		}
	}

	return nil
}

// CheckProofOfWork ensures the block header bits which indicate the target
// difficulty is in min/max range and that the block hash is less than the
// target difficulty as claimed.
func CheckProofOfWork(block *dcrutil.Block, powLimit *big.Int) error {
	return checkProofOfWork(&block.MsgBlock().Header, powLimit, BFNone)
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
func CountP2SHSigOps(tx *dcrutil.Tx, isCoinBaseTx bool, isStakeBaseTx bool,
	txStore TxStore) (int, error) {
	// Coinbase transactions have no interesting inputs.
	if isCoinBaseTx {
		return 0, nil
	}

	// Stakebase (SSGen) transactions have no P2SH inputs. Same with SSRtx,
	// but they will still pass the checks below.
	if isStakeBaseTx {
		return 0, nil
	}

	// Accumulate the number of signature operations in all transaction
	// inputs.
	msgTx := tx.MsgTx()
	totalSigOps := 0
	for _, txIn := range msgTx.TxIn {
		// Ensure the referenced input transaction is available.
		txInHash := &txIn.PreviousOutPoint.Hash
		originTx, exists := txStore[*txInHash]
		if !exists || originTx.Err != nil || originTx.Tx == nil {
			str := fmt.Sprintf("unable to find input transaction "+
				"%v referenced from transaction %v", txInHash,
				tx.Sha())
			return 0, ruleError(ErrMissingTx, str)
		}
		originMsgTx := originTx.Tx.MsgTx()

		// Ensure the output index in the referenced transaction is
		// available.
		originTxIndex := txIn.PreviousOutPoint.Index
		if originTxIndex >= uint32(len(originMsgTx.TxOut)) {
			str := fmt.Sprintf("out of bounds input index %d in "+
				"transaction %v referenced from transaction %v",
				originTxIndex, txInHash, tx.Sha())
			return 0, ruleError(ErrBadTxInput, str)
		}

		// We're only interested in pay-to-script-hash types, so skip
		// this input if it's not one.
		pkScript := originMsgTx.TxOut[originTxIndex].PkScript
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
			str := fmt.Sprintf("the public key script from "+
				"output index %d in transaction %v contains "+
				"too many signature operations - overflow",
				originTxIndex, txInHash)
			return 0, ruleError(ErrTooManySigOps, str)
		}
	}

	return totalSigOps, nil
}

// checkBlockHeaderSanity performs some preliminary checks on a block header to
// ensure it is sane before continuing with processing.  These checks are
// context free.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkProofOfWork.
func checkBlockHeaderSanity(block *dcrutil.Block, timeSource MedianTimeSource,
	flags BehaviorFlags, chainParams *chaincfg.Params) error {
	powLimit := chainParams.PowLimit
	posLimit := chainParams.MinimumStakeDiff
	header := &block.MsgBlock().Header

	// Ensure the proof of work bits in the block header is in min/max range
	// and the block hash is less than the target value described by the
	// bits.
	err := checkProofOfWork(header, powLimit, flags)
	if err != nil {
		return err
	}

	// Check to make sure that all newly purchased tickets meet the difficulty
	// specified in the block.
	err = checkProofOfStake(block, posLimit)
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
	maxTimestamp := time.Now().Add(time.Second * MaxTimeOffsetSeconds)
	if header.Timestamp.After(maxTimestamp) {
		str := fmt.Sprintf("block timestamp of %v is too far in the "+
			"future", header.Timestamp)
		return ruleError(ErrTimeTooNew, str)
	}

	return nil
}

// checkBlockSanity performs some preliminary checks on a block to ensure it is
// sane before continuing with block processing.  These checks are context free.
//
// The flags do not modify the behavior of this function directly, however they
// are needed to pass along to checkBlockHeaderSanity.
func checkBlockSanity(block *dcrutil.Block, timeSource MedianTimeSource,
	flags BehaviorFlags, chainParams *chaincfg.Params) error {

	msgBlock := block.MsgBlock()
	header := &msgBlock.Header
	err := checkBlockHeaderSanity(block, timeSource, flags, chainParams)
	if err != nil {
		return err
	}

	// A block must have at least one regular transaction.
	numTx := len(msgBlock.Transactions)
	if numTx == 0 {
		return ruleError(ErrNoTransactions, "block does not contain "+
			"any transactions")
	}

	// A block must not have more transactions than the max block payload.
	if numTx > chainParams.MaximumBlockSize {
		str := fmt.Sprintf("block contains too many transactions - "+
			"got %d, max %d", numTx, chainParams.MaximumBlockSize)
		return ruleError(ErrTooManyTransactions, str)
	}

	// A block must not have more stake transactions than the max block payload.
	numStakeTx := len(msgBlock.STransactions)
	if numStakeTx > chainParams.MaximumBlockSize {
		str := fmt.Sprintf("block contains too many stake transactions - "+
			"got %d, max %d", numStakeTx, chainParams.MaximumBlockSize)
		return ruleError(ErrTooManyTransactions, str)
	}

	// A block must not exceed the maximum allowed block payload when
	// serialized.
	serializedSize := msgBlock.SerializeSize()
	if serializedSize > chainParams.MaximumBlockSize {
		str := fmt.Sprintf("serialized block is too big - got %d, "+
			"max %d", serializedSize, chainParams.MaximumBlockSize)
		return ruleError(ErrBlockTooBig, str)
	}
	if msgBlock.Header.Size != uint32(serializedSize) {
		str := fmt.Sprintf("serialized block is not size indicated in "+
			"header - got %d, expected %d", msgBlock.Header.Size,
			serializedSize)
		return ruleError(ErrWrongBlockSize, str)
	}

	// The first transaction in a block's txtreeregular must be a coinbase.
	transactions := block.Transactions()
	if !IsCoinBase(transactions[0]) {
		return ruleError(ErrFirstTxNotCoinbase, "first transaction in "+
			"block is not a coinbase")
	}

	// A block must not have more than one coinbase.
	for i, tx := range transactions[1:] {
		if IsCoinBase(tx) {
			str := fmt.Sprintf("block contains second coinbase at "+
				"index %d", i)
			return ruleError(ErrMultipleCoinbases, str)
		}
	}

	// Do some preliminary checks on each transaction to ensure they are
	// sane before continuing.
	for _, tx := range transactions {
		txType := stake.DetermineTxType(tx)
		if txType != stake.TxTypeRegular {
			errStr := fmt.Sprintf("found stake tx in regular tx tree")
			return ruleError(ErrStakeTxInRegularTree, errStr)
		}
		err := CheckTransactionSanity(tx, chainParams)
		if err != nil {
			return err
		}
	}
	for _, stx := range block.STransactions() {
		txType := stake.DetermineTxType(stx)
		if txType == stake.TxTypeRegular {
			errStr := fmt.Sprintf("found regular tx in stake tx tree")
			return ruleError(ErrRegTxInStakeTree, errStr)
		}
		err := CheckTransactionSanity(stx, chainParams)
		if err != nil {
			return err
		}
	}

	// Check that the coinbase pays the tax, if applicable.
	err = CoinbasePaysTax(block.Transactions()[0], header.Height, header.Voters,
		chainParams)
	if err != nil {
		return err
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
		str := fmt.Sprintf("block stake merkle root is invalid - block "+
			"header indicates %v, but calculated value is %v",
			header.StakeRoot, calculatedStakeMerkleRoot)
		return ruleError(ErrBadMerkleRoot, str)
	}

	// Check for duplicate transactions.  This check will be fairly quick
	// since the transaction hashes are already cached due to building the
	// merkle tree above.
	existingTxHashes := make(map[chainhash.Hash]struct{})
	stakeTransactions := block.STransactions()
	allTransactions := append(transactions, stakeTransactions...)

	for _, tx := range allTransactions {
		hash := tx.Sha()
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

		isSSGen, _ := stake.IsSSGen(tx)
		isCoinBase := IsCoinBase(tx)

		totalSigOps += CountSigOps(tx, isCoinBase, isSSGen)
		if totalSigOps < lastSigOps || totalSigOps > MaxSigOpsPerBlock {
			str := fmt.Sprintf("block contains too many signature "+
				"operations - got %v, max %v", totalSigOps,
				MaxSigOpsPerBlock)
			return ruleError(ErrTooManySigOps, str)
		}
	}

	// Blocks before stake validation height may only have 0x0001
	// as their VoteBits in the header.
	if int64(header.Height) < chainParams.StakeValidationHeight {
		if header.VoteBits != earlyVoteBitsValue {
			str := fmt.Sprintf("pre stake validation height block %v "+
				"contained an invalid votebits value (expected %v, "+
				"got %v)", block.Sha(), earlyVoteBitsValue,
				header.VoteBits)
			return ruleError(ErrInvalidEarlyVoteBits, str)
		}
	}

	return nil
}

// CheckBlockSanity performs some preliminary checks on a block to ensure it is
// sane before continuing with block processing.  These checks are context free.
func CheckBlockSanity(block *dcrutil.Block, timeSource MedianTimeSource,
	chainParams *chaincfg.Params) error {
	return checkBlockSanity(block, timeSource, BFNone, chainParams)
}

// CheckWorklessBlockSanity performs some preliminary checks on a block to
// ensure it is sane before continuing with block processing.  These checks are
// context free.
func CheckWorklessBlockSanity(block *dcrutil.Block, timeSource MedianTimeSource,
	chainParams *chaincfg.Params) error {
	return checkBlockSanity(block, timeSource, BFNoPoWCheck, chainParams)
}

// checkBlockHeaderContext peforms several validation checks on the block header
// which depend on its position within the block chain.
//
// The flags modify the behavior of this function as follows:
//  - BFFastAdd: All checks except those involving comparing the header against
//    the checkpoints are not performed.
func (b *BlockChain) checkBlockHeaderContext(header *wire.BlockHeader,
	prevNode *blockNode, flags BehaviorFlags) error {
	// The genesis block is valid by definition.
	if prevNode == nil {
		return nil
	}

	fastAdd := flags&BFFastAdd == BFFastAdd
	if !fastAdd {
		// Ensure the difficulty specified in the block header matches
		// the calculated difficulty based on the previous block and
		// difficulty retarget rules.
		expectedDifficulty, err := b.calcNextRequiredDifficulty(prevNode,
			header.Timestamp)
		if err != nil {
			return err
		}
		blockDifficulty := header.Bits
		if blockDifficulty != expectedDifficulty {
			str := "block difficulty of %d is not the expected value of %d"
			str = fmt.Sprintf(str, blockDifficulty, expectedDifficulty)
			return ruleError(ErrUnexpectedDifficulty, str)
		}

		// Ensure the timestamp for the block header is after the
		// median time of the last several blocks (medianTimeBlocks).
		medianTime, err := b.calcPastMedianTime(prevNode)
		if err != nil {
			log.Errorf("calcPastMedianTime: %v", err)
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

	// Ensure chain matches up to predetermined checkpoints.
	blockHash := header.BlockSha()
	if !b.verifyCheckpoint(blockHeight, &blockHash) {
		str := fmt.Sprintf("block at height %d does not match "+
			"checkpoint hash", blockHeight)
		return ruleError(ErrBadCheckpoint, str)
	}

	// Find the previous checkpoint and prevent blocks which fork the main
	// chain before it.  This prevents storage of new, otherwise valid,
	// blocks which build off of old blocks that are likely at a much easier
	// difficulty and therefore could be used to waste cache and disk space.
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
		// Reject old version blocks once a majority of the network has
		// upgraded.
		mv := b.chainParams.CurrentBlockVersion
		if header.Version < mv &&
			b.isMajorityVersion(mv,
				prevNode,
				b.chainParams.CurrentBlockVersion) {
			str := "new blocks with version %d are no longer valid"
			str = fmt.Sprintf(str, header.Version)
			return ruleError(ErrBlockVersionTooOld, str)
		}
	}

	return nil
}

// isTransactionSpent returns whether or not the provided transaction data
// describes a fully spent transaction.  A fully spent transaction is one where
// all outputs have been spent.
func isTransactionSpent(txD *TxData) bool {
	for _, isOutputSpent := range txD.Spent {
		if !isOutputSpent {
			return false
		}
	}
	return true
}

// checkDupTxsMain ensures blocks do not contain duplicate
// transactions which 'overwrite' older transactions that are not fully
// spent.  This prevents an attack where a coinbase and all of its
// dependent transactions could be duplicated to effectively revert the
// overwritten transactions to a single confirmation thereby making
// them vulnerable to a double spend.
//
// For more details, see https://en.bitcoin.it/wiki/BIP_0030 and
// http://r6.ca/blog/20120206T005236Z.html.
//
// Decred: Check the stake transactions to make sure they don't have this txid
// too.
func (b *BlockChain) checkDupTxsMain(txResults TxStore) error {
	// Examine the resulting data about the requested transactions.
	for _, txD := range txResults {
		switch txD.Err {
		// A duplicate transaction was not found.  This is the most
		// common case.
		case database.ErrTxShaMissing:
			continue

		// A duplicate transaction was found.  This is only allowed if
		// the duplicate transaction is fully spent.
		case nil:
			if !isTransactionSpent(txD) {
				str := fmt.Sprintf("tried to overwrite "+
					"transaction %v at block height %d "+
					"that is not fully spent", txD.Hash,
					txD.BlockHeight)
				return ruleError(ErrOverwriteTx, str)
			}

		// Some other unexpected error occurred.  Return it now.
		default:
			return txD.Err
		}
	}

	return nil
}

// checkDupTxs is a local function to check for duplicate tx hashes in
// blocks that spend from tx hashes that are not yet totally spent.
func (b *BlockChain) checkDupTxs(node *blockNode, parentNode *blockNode,
	block *dcrutil.Block, parentBlock *dcrutil.Block) error {
	// Attempt to fetch duplicate transactions for all of the transactions
	// in this block from the point of view of the parent node and the
	// sequential addition of different TxTrees.

	// Genesis block.
	if parentNode == nil {
		return nil
	}

	// Parent TxTreeRegular (if applicable).
	regularTxTreeValid := dcrutil.IsFlagSet16(node.header.VoteBits,
		dcrutil.BlockValid)
	thisNodeStakeViewpoint := ViewpointPrevInvalidStake
	thisNodeRegularViewpoint := ViewpointPrevInvalidRegular

	if regularTxTreeValid {
		fetchSet := make(map[chainhash.Hash]struct{})

		for _, tx := range parentBlock.Transactions() {
			fetchSet[*tx.Sha()] = struct{}{}
		}

		txResults, err := b.fetchTxStore(parentNode, block, fetchSet,
			ViewpointPrevValidInitial)
		if err != nil {
			log.Tracef("Failed to fetch TxTreeRegular viewpoint of prev "+
				"block: %v", err.Error())
			return err
		}

		err = b.checkDupTxsMain(txResults)
		if err != nil {
			str := fmt.Sprintf("Failed dup tx check of TxTreeRegular of prev "+
				"block: %v", err.Error())
			return ruleError(ErrDuplicateTx, str)
		}

		// TxTreeRegular of previous block is valid, so change the viewpoint
		// below.
		thisNodeStakeViewpoint = ViewpointPrevValidStake
		thisNodeRegularViewpoint = ViewpointPrevValidRegular
	}

	fetchSetStake := make(map[chainhash.Hash]struct{})

	for _, tx := range block.STransactions() {
		fetchSetStake[*tx.Sha()] = struct{}{}
	}

	txResults, err := b.fetchTxStore(node, block, fetchSetStake,
		thisNodeStakeViewpoint)
	if err != nil {
		log.Tracef("Failed to fetch TxTreeStake viewpoint of cur "+
			"block: %v", err.Error())
		return err
	}

	err = b.checkDupTxsMain(txResults)
	if err != nil {
		str := fmt.Sprintf("Failed dup tx check of TxTreeStake of cur "+
			"block: %v", err.Error())
		return ruleError(ErrDuplicateTx, str)
	}

	fetchSetRegular := make(map[chainhash.Hash]struct{})

	for _, tx := range block.Transactions() {
		fetchSetRegular[*tx.Sha()] = struct{}{}
	}

	txResults, err = b.fetchTxStore(node, block, fetchSetRegular,
		thisNodeRegularViewpoint)
	if err != nil {
		log.Tracef("Failed to fetch TxTreeRegular viewpoint of cur "+
			"block: %v", err.Error())
		return err
	}

	err = b.checkDupTxsMain(txResults)
	if err != nil {
		str := fmt.Sprintf("Failed dup tx check of TxTreeRegular of cur "+
			"block: %v", err.Error())
		return ruleError(ErrDuplicateTx, str)
	}

	return nil
}

// CheckBlockStakeSanity performs a series of checks on a block to ensure that the
// information from the block's header about stake is sane. For instance, the
// number of SSGen tx must be equal to voters.
// TODO: We can consider breaking this into two functions and making some of these
// checks go through in processBlock, however if a block has demonstrable PoW it
// seems unlikely that it will have stake errors (because the miner is then just
// wasting hash power).
func (b *BlockChain) CheckBlockStakeSanity(tixStore TicketStore,
	stakeValidationHeight int64, node *blockNode, block *dcrutil.Block,
	parent *dcrutil.Block, chainParams *chaincfg.Params) error {

	// Setup variables.
	stakeTransactions := block.STransactions()
	msgBlock := block.MsgBlock()
	voters := msgBlock.Header.Voters
	freshstake := msgBlock.Header.FreshStake
	revocations := msgBlock.Header.Revocations
	sbits := msgBlock.Header.SBits
	blockSha := block.Sha()
	prevBlockHash := &msgBlock.Header.PrevBlock
	poolSize := int(msgBlock.Header.PoolSize)
	finalState := node.header.FinalState

	ticketsPerBlock := int(b.chainParams.TicketsPerBlock)

	txTreeRegularValid := dcrutil.IsFlagSet16(msgBlock.Header.VoteBits,
		dcrutil.BlockValid)

	stakeEnabledHeight := chainParams.StakeEnabledHeight

	// Do some preliminary checks on each stake transaction to ensure they
	// are sane before continuing.
	ssGens := 0 // Votes
	ssRtxs := 0 // Revocations
	for i, tx := range stakeTransactions {
		isSSGen, _ := stake.IsSSGen(tx)
		isSSRtx, _ := stake.IsSSRtx(tx)

		if isSSGen {
			ssGens++
		}

		if isSSRtx {
			ssRtxs++
		}

		// If we haven't reached the point in which staking is enabled, there
		// should be absolutely no SSGen or SSRtx transactions.
		if (isSSGen && (block.Height() < stakeEnabledHeight)) ||
			(isSSRtx && (block.Height() < stakeEnabledHeight)) {
			errStr := fmt.Sprintf("block contained SSGen or SSRtx "+
				"transaction at idx %v, which was before stake voting"+
				" was enabled; block height %v, stake enabled height "+
				"%v", i, block.Height(), stakeEnabledHeight)
			return ruleError(ErrInvalidEarlyStakeTx, errStr)
		}
	}

	// Make sure we have no votes or revocations if stake validation is
	// not enabled.
	containsVotes := ssGens > 0
	containsRevocations := ssRtxs > 0
	if node.height < chainParams.StakeValidationHeight &&
		(containsVotes || containsRevocations) {
		errStr := fmt.Sprintf("block contained votes or revocations " +
			"before the stake validation height")
		return ruleError(ErrInvalidEarlyStakeTx, errStr)
	}
	// Check the number of voters if stake validation is enabled.
	if node.height >= chainParams.StakeValidationHeight {
		// Too many voters on this block.
		if ssGens > int(chainParams.TicketsPerBlock) {
			errStr := fmt.Sprintf("block contained too many votes! "+
				"%v votes but only %v max allowed", ssGens,
				chainParams.TicketsPerBlock)
			return ruleError(ErrTooManyVotes, errStr)
		}

		// Not enough voters on this block.
		if block.Height() >= stakeValidationHeight &&
			ssGens <= int(chainParams.TicketsPerBlock)/2 {
			errStr := fmt.Sprintf("block contained too few votes! "+
				"%v votes but %v or more required", ssGens,
				(int(chainParams.TicketsPerBlock)/2)+1)
			return ruleError(ErrNotEnoughVotes, errStr)
		}
	}

	// ----------------------------------------------------------------------------
	// SStx Tx Handling
	// ----------------------------------------------------------------------------
	// PER SSTX
	// 1. Check to make sure that the amount committed with the SStx is equal to
	//     the target of the last block (sBits).
	// 2. Ensure the the number of SStx tx in the block is the same as FreshStake
	//     in the header.
	// PER BLOCK
	// 3. Check to make sure we haven't exceeded max number of new SStx.

	numSStxTx := 0

	for _, staketx := range stakeTransactions {
		if is, _ := stake.IsSStx(staketx); is {
			numSStxTx++

			// 1. Make sure that we're committing enough coins. Checked already
			// when we check stake difficulty, so may not be needed.
			if staketx.MsgTx().TxOut[0].Value < sbits {
				txSha := staketx.Sha()
				errStr := fmt.Sprintf("Error in stake consensus: the amount "+
					"committed in SStx %v was less than the sBits value %v",
					txSha, sbits)
				return ruleError(ErrNotEnoughStake, errStr)
			}
		}
	}

	// 2. Ensure the the number of SStx tx in the block is the same as FreshStake
	//     in the header.
	if uint8(numSStxTx) != freshstake {
		errStr := fmt.Sprintf("Error in stake consensus: the number of SStx tx "+
			"in block %v was %v, however in the header freshstake is %v", blockSha,
			numSStxTx, freshstake)
		return ruleError(ErrFreshStakeMismatch, errStr)
	}

	// 3. Check to make sure we haven't exceeded max number of new SStx. May not
	// need this check, as the above one should fail if you overflow uint8.
	if numSStxTx > int(chainParams.MaxFreshStakePerBlock) {
		errStr := fmt.Sprintf("Error in stake consensus: the number of SStx tx "+
			"in block %v was %v, overflowing the maximum allowed (255)", blockSha,
			numSStxTx)
		return ruleError(ErrTooManySStxs, errStr)
	}

	// Break if the stake system is otherwise disabled ----------------------------
	if block.Height() < stakeValidationHeight {
		stakeTxSum := numSStxTx

		// Check and make sure we're only including SStx in the stake tx tree.
		if stakeTxSum != len(stakeTransactions) {
			errStr := fmt.Sprintf("Error in stake consensus: the number of "+
				"stake tx in block %v was %v, however we expected %v",
				block.Sha(), stakeTxSum, len(stakeTransactions))
			return ruleError(ErrInvalidEarlyStakeTx, errStr)
		}

		// Check the ticket pool size.
		_, calcPoolSize, _, err := b.getWinningTicketsInclStore(node, tixStore)
		if err != nil {
			log.Tracef("failed to retrieve poolsize for stake "+
				"consensus: %v", err.Error())
			return err
		}

		if calcPoolSize != poolSize {
			errStr := fmt.Sprintf("Error in stake consensus: the poolsize "+
				"in block %v was %v, however we expected %v",
				node.hash,
				poolSize,
				calcPoolSize)
			return ruleError(ErrPoolSize, errStr)
		}

		return nil
	}

	// ----------------------------------------------------------------------------
	// General Purpose Checks
	// ----------------------------------------------------------------------------
	// 1. Check that we have a majority vote of potential voters.

	// 1. Check to make sure we have a majority of the potential voters voting.
	if msgBlock.Header.Voters == 0 {
		errStr := fmt.Sprintf("Error: no voters in block %v",
			blockSha)
		return ruleError(ErrNotEnoughVotes, errStr)
	}

	majority := (chainParams.TicketsPerBlock / 2) + 1
	if msgBlock.Header.Voters < majority {
		errStr := fmt.Sprintf("Error in stake consensus: the number of voters is "+
			"not in the majority as compared to potential votes for block %v",
			blockSha)
		return ruleError(ErrNotEnoughVotes, errStr)
	}

	// ----------------------------------------------------------------------------
	// SSGen Tx Handling
	// ----------------------------------------------------------------------------
	// PER SSGEN
	// 1. Retrieve an emulated ticket database of SStxMemMaps from both the
	//     ticket database and the ticket store.
	// 2. Check to ensure that the tickets included in the block are the ones
	//     that indeed should have been included according to the emulated
	//     ticket database.
	// 3. Check to make sure that the SSGen votes on the correct block/height.

	// PER BLOCK
	// 4. Check and make sure that we have the same number of SSGen tx as we do
	//     votes.
	// 5. Check for voters overflows (highly unlikely, but check anyway).
	// 6. Ensure that the block votes on tx tree regular of the previous block in
	//     the way of the majority of the voters.
	// 7. Check final state and ensure that it matches.

	// Store the number of SSGen tx and votes to check later.
	numSSGenTx := 0
	voteYea := 0
	voteNay := 0

	// 1. Retrieve an emulated ticket database of SStxMemMaps from both the
	//     ticket database and the ticket store.
	ticketsWhichCouldBeUsed := make(map[chainhash.Hash]struct{}, ticketsPerBlock)
	ticketSlice, calcPoolSize, finalStateCalc, err :=
		b.getWinningTicketsInclStore(node, tixStore)
	if err != nil {
		errStr := fmt.Sprintf("unexpected getWinningTicketsInclStore error: %v",
			err.Error())
		return errors.New(errStr)
	}

	// 2. Obtain the tickets which could have been used on the block for votes
	//     and then check below to make sure that these were indeed the tickets
	//     used.
	for _, ticketHash := range ticketSlice {
		ticketsWhichCouldBeUsed[ticketHash] = struct{}{}
	}

	for _, staketx := range stakeTransactions {
		if is, _ := stake.IsSSGen(staketx); is {
			numSSGenTx++

			// Check and store the vote for TxTreeRegular.
			ssGenVoteBits := stake.GetSSGenVoteBits(staketx)
			if dcrutil.IsFlagSet16(ssGenVoteBits, dcrutil.BlockValid) {
				voteYea++
			} else {
				voteNay++
			}

			// Grab the input SStx hash from the inputs of the transaction.
			msgTx := staketx.MsgTx()
			sstxIn := msgTx.TxIn[1] // sstx input
			sstxHash := sstxIn.PreviousOutPoint.Hash

			// Check to make sure this was actually a ticket we were allowed to
			// use.
			_, ticketAvailable := ticketsWhichCouldBeUsed[sstxHash]
			if !ticketAvailable {
				errStr := fmt.Sprintf("Error in stake consensus: Ticket %v was "+
					"not found to be available in the stake patch or database, "+
					"yet block %v spends it!", sstxHash, blockSha)
				return ruleError(ErrTicketUnavailable, errStr)
			}

			// 3. Check to make sure that the SSGen tx votes on the parent block of
			// the block in which it is included.
			votedOnSha, votedOnHeight, err := stake.GetSSGenBlockVotedOn(staketx)
			if err != nil {
				errStr := fmt.Sprintf("unexpected vote tx decode error: %v",
					err.Error())
				return ruleError(ErrUnparseableSSGen, errStr)
			}

			if !(votedOnSha.IsEqual(prevBlockHash)) ||
				(votedOnHeight != uint32(block.Height())-1) {
				txSha := msgTx.TxSha()
				errStr := fmt.Sprintf("Error in stake consensus: SSGen %v voted "+
					"on block %v at height %v, however it was found inside "+
					"block %v at height %v!", txSha, votedOnSha,
					votedOnHeight, prevBlockHash, block.Height()-1)
				return ruleError(ErrVotesOnWrongBlock, errStr)
			}
		}
	}

	// 4. Check and make sure that we have the same number of SSGen tx as we do
	// votes.
	if uint16(numSSGenTx) != voters {
		errStr := fmt.Sprintf("Error in stake consensus: The number of SSGen tx"+
			" in block %v was not the %v voters expected from the header!",
			blockSha, voters)
		return ruleError(ErrVotesMismatch, errStr)
	}

	// 5. Check for too many voters (already checked in block sanity, but check
	// again).
	if numSSGenTx > int(chainParams.TicketsPerBlock) {
		errStr := fmt.Sprintf("Error in stake consensus: the number of SSGen tx "+
			"in block %v was %v, overflowing the maximum allowed (%v)",
			blockSha, numSSGenTx, int(chainParams.TicketsPerBlock))
		return ruleError(ErrTooManyVotes, errStr)
	}

	// 6. Determine if TxTreeRegular should be valid or not, and then check it
	//     against what is provided in the block header.
	if (voteYea <= voteNay) && txTreeRegularValid {
		errStr := fmt.Sprintf("Error in stake consensus: the voters voted "+
			"against parent TxTreeRegular inclusion in block %v, but the "+
			"block header indicates it was voted for", blockSha)
		return ruleError(ErrIncongruentVotebit, errStr)
	}
	if (voteYea > voteNay) && !txTreeRegularValid {
		errStr := fmt.Sprintf("Error in stake consensus: the voters voted "+
			"for parent TxTreeRegular inclusion in block %v, but the "+
			"block header indicates it was voted against", blockSha)
		return ruleError(ErrIncongruentVotebit, errStr)
	}

	// 7. Check the final state of the lottery PRNG and ensure that it matches.
	if finalStateCalc != finalState {
		errStr := fmt.Sprintf("Error in stake consensus: the final state of "+
			"the lottery PRNG was calculated to be %x, but %x was found in "+
			"the block", finalStateCalc, finalState)
		return ruleError(ErrInvalidFinalState, errStr)
	}

	// ----------------------------------------------------------------------------
	// SSRtx Tx Handling
	// ----------------------------------------------------------------------------
	// PER SSRTX
	// 1. Ensure that the SSRtx has been marked missed in the ticket patch data
	//     and, if not, ensure it has been marked missed in the ticket database.
	// 2. Ensure that at least ticketMaturity many blocks has passed since the
	//     SStx it references was included in the blockchain.
	// PER BLOCK
	// 3. Check and make sure that we have the same number of SSRtx tx as we do
	//     revocations.
	// 4. Check for revocation overflows.
	numSSRtxTx := 0

	missedTickets, err := b.GenerateMissedTickets(tixStore)
	if err != nil {
		h := block.Sha()
		str := fmt.Sprintf("Failed to generate missed tickets data "+
			"for block %v, height %v! Error given: %v",
			h,
			block.Height(),
			err.Error())
		return errors.New(str)
	}

	for _, staketx := range stakeTransactions {
		if is, _ := stake.IsSSRtx(staketx); is {
			numSSRtxTx++

			// Grab the input SStx hash from the inputs of the transaction.
			msgTx := staketx.MsgTx()
			sstxIn := msgTx.TxIn[0] // sstx input
			sstxHash := sstxIn.PreviousOutPoint.Hash

			ticketMissed := false

			if _, exists := missedTickets[sstxHash]; exists {
				ticketMissed = true
			}

			if !ticketMissed {
				errStr := fmt.Sprintf("Error in stake consensus: Ticket %v was "+
					"not found to be missed in the stake patch or database, "+
					"yet block %v spends it!", sstxHash, blockSha)
				return ruleError(ErrInvalidSSRtx, errStr)
			}
		}
	}

	// 3. Check and make sure that we have the same number of SSRtx tx as we do
	// votes.
	if uint8(numSSRtxTx) != revocations {
		errStr := fmt.Sprintf("Error in stake consensus: The number of SSRtx tx"+
			" in block %v was not the %v revocations expected from the header! "+
			"(%v found)", blockSha, revocations, numSSRtxTx)
		return ruleError(ErrInvalidRevNum, errStr)
	}

	// 4. Check for revocation overflows. Should be impossible given the above
	// check, but check anyway.
	if numSSRtxTx > math.MaxUint8 {
		errStr := fmt.Sprintf("Error in stake consensus: the number of SSRtx tx "+
			"in block %v was %v, overflowing the maximum allowed (255)", blockSha,
			numSSRtxTx)
		return ruleError(ErrTooManyRevocations, errStr)
	}

	// ----------------------------------------------------------------------------
	// Final Checks
	// ----------------------------------------------------------------------------
	// 1. Make sure that all the tx in the stake tx tree are either SStx, SSGen,
	//     or SSRtx.
	// 2. Check and make sure that the ticketpool size is calculated correctly
	//     after account for spent, missed, and expired tickets.

	// 1. Ensure that all stake transactions are accounted for. If not, this
	//     indicates that there was some sort of non-standard stake tx present
	//     in the block. This is already checked before, but check again here.
	stakeTxSum := numSStxTx + numSSGenTx + numSSRtxTx

	if stakeTxSum != len(stakeTransactions) {
		errStr := fmt.Sprintf("Error in stake consensus: the number of stake tx "+
			"in block %v was %v, however we expected %v", block.Sha(), stakeTxSum,
			len(stakeTransactions))
		return ruleError(ErrNonstandardStakeTx, errStr)
	}

	// 2. Check the ticket pool size.
	if calcPoolSize != poolSize {
		errStr := fmt.Sprintf("Error in stake consensus: the poolsize "+
			"in block %v was %v, however we expected %v",
			node.hash,
			poolSize,
			calcPoolSize)
		return ruleError(ErrPoolSize, errStr)
	}

	return nil
}

// CheckTransactionInputs performs a series of checks on the inputs to a
// transaction to ensure they are valid.  An example of some of the checks
// include verifying all inputs exist, ensuring the coinbase seasoning
// requirements are met, detecting double spends, validating all values and fees
// are in the legal range and the total output amount doesn't exceed the input
// amount, and verifying the signatures to prove the spender was the owner of
// the decred and therefore allowed to spend them.  As it checks the inputs,
// it also calculates the total fees for the transaction and returns that value.
func CheckTransactionInputs(tx *dcrutil.Tx, txHeight int64, txStore TxStore,
	checkFraudProof bool, chainParams *chaincfg.Params) (int64, error) {
	// Expired transactions are not allowed.
	if tx.MsgTx().Expiry != wire.NoExpiryValue {
		if txHeight >= int64(tx.MsgTx().Expiry) {
			errStr := fmt.Sprintf("Transaction indicated an expiry of %v"+
				" while the current height is %v", tx.MsgTx().Expiry, txHeight)
			return 0, ruleError(ErrExpiredTx, errStr)
		}
	}

	ticketMaturity := int64(chainParams.TicketMaturity)
	stakeEnabledHeight := chainParams.StakeEnabledHeight
	txHash := tx.Sha()
	var totalAtomIn int64

	// Coinbase transactions have no inputs.
	if IsCoinBase(tx) {
		return 0, nil
	}

	// ----------------------------------------------------------------------------
	// Decred stake transaction testing.
	// ----------------------------------------------------------------------------

	// SSTX -----------------------------------------------------------------------
	// 1. Check and make sure that the output amounts in the commitments to the
	//     ticket are correctly calculated.

	// 1. Check and make sure that the output amounts in the commitments to the
	// ticket are correctly calculated.
	isSStx, _ := stake.IsSStx(tx)
	if isSStx {
		msgTx := tx.MsgTx()

		sstxInAmts := make([]int64, len(msgTx.TxIn))

		for idx, txIn := range msgTx.TxIn {
			// Ensure the input is available.
			txInHash := &txIn.PreviousOutPoint.Hash
			originTx, exists := txStore[*txInHash]

			if !exists || originTx.Err != nil || originTx.Tx == nil {
				str := fmt.Sprintf("unable to find input transaction "+
					"%v for transaction %v", txInHash, txHash)
				return 0, ruleError(ErrMissingTx, str)
			}

			// Ensure the transaction is not double spending coins.
			originTxIndex := txIn.PreviousOutPoint.Index
			originTxMsgTx := originTx.Tx.MsgTx()

			if int(originTxIndex) >= len(originTxMsgTx.TxOut) {
				errStr := fmt.Sprintf("SStx input using tx %x, txout %v "+
					"referenced a txout that was out of range",
					originTx.Tx.Sha(),
					originTxIndex)
				return 0, ruleError(ErrBadTxInput, errStr)
			}

			// Check and make sure that the input is P2PKH or P2SH.
			thisPkVersion := originTxMsgTx.TxOut[originTxIndex].Version
			thisPkScript := originTxMsgTx.TxOut[originTxIndex].PkScript
			class := txscript.GetScriptClass(thisPkVersion, thisPkScript)
			if txscript.IsStakeOutput(thisPkScript) {
				class, _ = txscript.GetStakeOutSubclass(thisPkScript)
			}

			if !(class == txscript.PubKeyHashTy ||
				class == txscript.ScriptHashTy) {
				errStr := fmt.Sprintf("SStx input using tx %x, txout %v "+
					"referenced a txout that was not a PubKeyHashTy or "+
					"ScriptHashTy pkScript (class: %v)",
					originTx.Tx.Sha(),
					originTxIndex,
					class)
				return 0, ruleError(ErrSStxInScrType, errStr)
			}

			// Get the value of the input.
			sstxInAmts[idx] = originTxMsgTx.TxOut[originTxIndex].Value
		}

		_, _, sstxOutAmts, sstxChangeAmts, _, _ :=
			stake.GetSStxStakeOutputInfo(tx)
		_, sstxOutAmtsCalc, err := stake.GetSStxNullOutputAmounts(sstxInAmts,
			sstxChangeAmts,
			msgTx.TxOut[0].Value)
		if err != nil {
			return 0, err
		}

		err = stake.VerifySStxAmounts(sstxOutAmts, sstxOutAmtsCalc)
		if err != nil {
			errStr := fmt.Sprintf("SStx output commitment amounts were not the "+
				"same as calculated amounts; Error returned %v", err)
			return 0, ruleError(ErrSStxCommitment, errStr)
		}
	}

	// SSGEN ----------------------------------------------------------------------
	// 1. Check SSGen output + rewards to make sure they're in line with the
	//     consensus code and what the outputs are in the original SStx. Also
	//     check to ensure that there is congruency for output PKH from SStx to
	//     SSGen outputs.
	//     Check also that the input transaction was an SStx.
	// 2. Make sure the second input is an SStx tagged output.
	// 3. Check to make sure that the difference in height between the current
	//     block and the block the SStx was included in is > ticketMaturity.

	// Save whether or not this is an SSGen tx; if it is, we need to skip the
	//     input check of the stakebase later, and another input check for OP_SSTX
	//     tagged output uses.
	isSSGen, _ := stake.IsSSGen(tx)
	if isSSGen {
		// Cursory check to see if we've even reached stake-enabled height.
		if txHeight < stakeEnabledHeight {
			errStr := fmt.Sprintf("SSGen tx appeared in block height %v before "+
				"stake enabled height %v", txHeight, stakeEnabledHeight)
			return 0, ruleError(ErrInvalidEarlyStakeTx, errStr)
		}

		// Grab the input SStx hash from the inputs of the transaction.
		msgTx := tx.MsgTx()
		nullIn := msgTx.TxIn[0]
		sstxIn := msgTx.TxIn[1] // sstx input
		sstxHash := sstxIn.PreviousOutPoint.Hash

		// Calculate the theoretical stake vote subsidy by extracting the vote
		// height. Should be impossible because IsSSGen requires this byte string
		// to be a certain number of bytes.
		_, heightVotingOn, err := stake.GetSSGenBlockVotedOn(tx)
		if err != nil {
			errStr := fmt.Sprintf("Could not parse SSGen block vote information "+
				"from SSGen %v; Error returned %v",
				txHash, err)
			return 0, ruleError(ErrUnparseableSSGen, errStr)
		}

		stakeVoteSubsidy := CalcStakeVoteSubsidy(int64(heightVotingOn),
			chainParams)

		// AmountIn for the input should be equal to the stake subsidy.
		if nullIn.ValueIn != stakeVoteSubsidy {
			errStr := fmt.Sprintf("bad stake vote subsidy; got %v, expect %v",
				nullIn.ValueIn, stakeVoteSubsidy)
			return 0, ruleError(ErrBadStakebaseAmountIn, errStr)
		}

		// 1. Fetch the input sstx transaction from the txstore and then check
		// to make sure that the reward has been calculated correctly from the
		// subsidy and the inputs.
		// We also need to make sure that the SSGen outputs that are P2PKH go
		// to the addresses specified in the original SSTx. Check that too.
		originTx, exists := txStore[sstxHash]
		if !exists || originTx.Err != nil || originTx.Tx == nil {
			errStr := fmt.Sprintf("Unable to find input sstx transaction "+
				"%v for transaction %v", sstxHash, txHash)
			return 0, ruleError(ErrMissingTx, errStr)
		}

		// While we're here, double check to make sure that the input is from an
		// SStx. By doing so, you also ensure the first output is OP_SSTX tagged.
		if isSStx, _ := stake.IsSStx(originTx.Tx); !isSStx {
			errStr := fmt.Sprintf("Input transaction %v for SSGen was not"+
				"an SStx tx (given input: %v)", txHash, sstxHash)
			return 0, ruleError(ErrInvalidSSGenInput, errStr)
		}

		// Make sure it's using the 0th output.
		if sstxIn.PreviousOutPoint.Index != 0 {
			errStr := fmt.Sprintf("Input transaction %v for SSGen did not"+
				"reference the first output (given idx %v)", txHash,
				sstxIn.PreviousOutPoint.Index)
			return 0, ruleError(ErrInvalidSSGenInput, errStr)
		}

		sstxMsgTx := originTx.Tx.MsgTx()

		sstxPayTypes, sstxPkhs, sstxAmts, _, sstxRules, sstxLimits :=
			stake.GetSStxStakeOutputInfo(originTx.Tx)

		ssgenPayTypes, ssgenPkhs, ssgenAmts, err :=
			stake.GetSSGenStakeOutputInfo(tx, chainParams)
		if err != nil {
			errStr := fmt.Sprintf("Could not decode outputs for SSgen %v: %v",
				txHash, err.Error())
			return 0, ruleError(ErrSSGenPayeeOuts, errStr)
		}

		// Quick check to make sure the number of SStx outputs is equal to
		// the number of SSGen outputs.
		if (len(sstxPayTypes) != len(ssgenPayTypes)) ||
			(len(sstxPkhs) != len(ssgenPkhs)) ||
			(len(sstxAmts) != len(ssgenAmts)) {
			errStr := fmt.Sprintf("Incongruent payee number for SSGen "+
				"%v and input SStx %v", txHash, sstxHash)
			return 0, ruleError(ErrSSGenPayeeNum, errStr)
		}

		// Get what the stake payouts should be after appending the reward
		// to each output.
		ssgenCalcAmts := stake.GetStakeRewards(sstxAmts,
			sstxMsgTx.TxOut[0].Value,
			stakeVoteSubsidy)

		/*
			err = stake.VerifyStakingPkhsAndAmounts(sstxPayTypes,
				sstxPkhs,
				ssrtxAmts,
				ssrtxPayTypes,
				ssrtxPkhs,
				ssrtxCalcAmts,
				false, // Revocation
				sstxRules,
				sstxLimits)
		*/

		// Check that the generated slices for pkhs and amounts are congruent.
		err = stake.VerifyStakingPkhsAndAmounts(sstxPayTypes,
			sstxPkhs,
			ssgenAmts,
			ssgenPayTypes,
			ssgenPkhs,
			ssgenCalcAmts,
			true, // Vote
			sstxRules,
			sstxLimits)

		if err != nil {
			errStr := fmt.Sprintf("Stake reward consensus violation for "+
				"SStx input %v and SSGen output %v: %v", sstxHash, txHash, err)
			return 0, ruleError(ErrSSGenPayeeOuts, errStr)
		}

		// 2. Check to make sure that the second input was an OP_SSTX tagged
		// output from the referenced SStx.
		if txscript.GetScriptClass(sstxMsgTx.TxOut[0].Version,
			sstxMsgTx.TxOut[0].PkScript) != txscript.StakeSubmissionTy {
			errStr := fmt.Sprintf("First SStx output in SStx %v referenced "+
				"by SSGen %v should have been OP_SSTX tagged, but it was "+
				"not", sstxHash, txHash)
			return 0, ruleError(ErrInvalidSSGenInput, errStr)
		}

		// 3. Check to ensure that ticket maturity number of blocks have passed
		// between the block the SSGen plans to go into and the block in which
		// the SStx was originally found in.
		originHeight := originTx.BlockHeight
		blocksSincePrev := txHeight - originHeight

		// NOTE: You can only spend an OP_SSTX tagged output on the block AFTER
		// the entire range of ticketMaturity has passed, hence <= instead of <.
		if blocksSincePrev <= ticketMaturity {
			errStr := fmt.Sprintf("tried to spend sstx output from "+
				"transaction %v from height %v at height %v before "+
				"required ticket maturity of %v+1 blocks", sstxHash, originHeight,
				txHeight, ticketMaturity)
			return 0, ruleError(ErrSStxInImmature, errStr)
		}
	}

	// SSRTX ----------------------------------------------------------------------
	// 1. Ensure the only input present is an OP_SSTX tagged output, and that the
	//     input transaction is actually an SStx.
	// 2. Ensure that payouts are to the original SStx NullDataTy outputs in the
	//     amounts given there, to the public key hashes given then.
	// 3. Check to make sure that the difference in height between the current
	//     block and the block the SStx was included in is > ticketMaturity.

	// Save whether or not this is an SSRtx tx; if it is, we need to know this
	// later input check for OP_SSTX outs.
	isSSRtx, _ := stake.IsSSRtx(tx)

	if isSSRtx {
		// Cursory check to see if we've even reach stake-enabled height.
		// Note for an SSRtx to be valid a vote must be missed, so for SSRtx the
		// height of allowance is +1.
		if txHeight < stakeEnabledHeight+1 {
			errStr := fmt.Sprintf("SSRtx tx appeared in block height %v before "+
				"stake enabled height+1 %v", txHeight, stakeEnabledHeight+1)
			return 0, ruleError(ErrInvalidEarlyStakeTx, errStr)
		}

		// Grab the input SStx hash from the inputs of the transaction.
		msgTx := tx.MsgTx()
		sstxIn := msgTx.TxIn[0] // sstx input
		sstxHash := sstxIn.PreviousOutPoint.Hash

		// 1. Fetch the input sstx transaction from the txstore and then check
		// to make sure that the reward has been calculated correctly from the
		// subsidy and the inputs.
		// We also need to make sure that the SSGen outputs that are P2PKH go
		// to the addresses specified in the original SSTx. Check that too.
		originTx, exists := txStore[sstxHash]
		if !exists || originTx.Err != nil || originTx.Tx == nil {
			errStr := fmt.Sprintf("Unable to find input sstx transaction "+
				"%v for transaction %v", sstxHash, txHash)
			return 0, ruleError(ErrMissingTx, errStr)
		}

		// While we're here, double check to make sure that the input is from an
		// SStx. By doing so, you also ensure the first output is OP_SSTX tagged.
		if isSStx, _ := stake.IsSStx(originTx.Tx); !isSStx {
			errStr := fmt.Sprintf("Input transaction %v for SSRtx %v was not"+
				"an SStx tx", txHash, sstxHash)
			return 0, ruleError(ErrInvalidSSRtxInput, errStr)
		}

		sstxMsgTx := originTx.Tx.MsgTx()

		sstxPayTypes, sstxPkhs, sstxAmts, _, sstxRules, sstxLimits :=
			stake.GetSStxStakeOutputInfo(originTx.Tx)

		// This should be impossible to hit given the strict bytecode
		// size restrictions for components of SSRtxs already checked
		// for in IsSSRtx.
		ssrtxPayTypes, ssrtxPkhs, ssrtxAmts, err :=
			stake.GetSSRtxStakeOutputInfo(tx, chainParams)
		if err != nil {
			errStr := fmt.Sprintf("Could not decode outputs for SSRtx %v: %v",
				txHash, err.Error())
			return 0, ruleError(ErrSSRtxPayees, errStr)
		}

		// Quick check to make sure the number of SStx outputs is equal to
		// the number of SSGen outputs.
		if (len(sstxPkhs) != len(ssrtxPkhs)) ||
			(len(sstxAmts) != len(ssrtxAmts)) {
			errStr := fmt.Sprintf("Incongruent payee number for SSRtx "+
				"%v and input SStx %v", txHash, sstxHash)
			return 0, ruleError(ErrSSRtxPayeesMismatch, errStr)
		}

		// Get what the stake payouts should be after appending the reward
		// to each output.
		ssrtxCalcAmts := stake.GetStakeRewards(sstxAmts,
			sstxMsgTx.TxOut[0].Value,
			int64(0)) // SSRtx has no subsidy

		// Check that the generated slices for pkhs and amounts are congruent.
		err = stake.VerifyStakingPkhsAndAmounts(sstxPayTypes,
			sstxPkhs,
			ssrtxAmts,
			ssrtxPayTypes,
			ssrtxPkhs,
			ssrtxCalcAmts,
			false, // Revocation
			sstxRules,
			sstxLimits)

		if err != nil {
			errStr := fmt.Sprintf("Stake consensus violation for SStx input"+
				" %v and SSRtx output %v: %v", sstxHash, txHash, err)
			return 0, ruleError(ErrSSRtxPayees, errStr)
		}

		// 2. Check to make sure that the second input was an OP_SSTX tagged
		// output from the referenced SStx.
		if txscript.GetScriptClass(sstxMsgTx.TxOut[0].Version,
			sstxMsgTx.TxOut[0].PkScript) != txscript.StakeSubmissionTy {
			errStr := fmt.Sprintf("First SStx output in SStx %v referenced "+
				"by SSGen %v should have been OP_SSTX tagged, but it was "+
				"not", sstxHash, txHash)
			return 0, ruleError(ErrInvalidSSRtxInput, errStr)
		}

		// 3. Check to ensure that ticket maturity number of blocks have passed
		// between the block the SSRtx plans to go into and the block in which
		// the SStx was originally found in.
		originHeight := originTx.BlockHeight
		blocksSincePrev := txHeight - originHeight

		// NOTE: You can only spend an OP_SSTX tagged output on the block AFTER
		// the entire range of ticketMaturity has passed, hence <= instead of <.
		// Also note that for OP_SSRTX spending, the ticket needs to have been
		// missed, and this can't possibly happen until reaching ticketMaturity +
		// 2.
		if blocksSincePrev <= ticketMaturity+1 {
			errStr := fmt.Sprintf("tried to spend sstx output from "+
				"transaction %v from height %v at height %v before "+
				"required ticket maturity of %v+1 blocks", sstxHash, originHeight,
				txHeight, ticketMaturity)
			return 0, ruleError(ErrSStxInImmature, errStr)
		}
	}

	// ----------------------------------------------------------------------------
	// Decred general transaction testing (and a few stake exceptions).
	// ----------------------------------------------------------------------------

	for idx, txIn := range tx.MsgTx().TxIn {
		// Ensure the input is available.
		txInHash := &txIn.PreviousOutPoint.Hash
		originTx, exists := txStore[*txInHash]

		// Inputs won't exist for stakebase tx, so ignore them.
		if isSSGen && idx == 0 {
			// However, do add the reward amount.
			_, heightVotingOn, _ := stake.GetSSGenBlockVotedOn(tx)
			stakeVoteSubsidy := CalcStakeVoteSubsidy(int64(heightVotingOn),
				chainParams)
			totalAtomIn += stakeVoteSubsidy
			continue
		}

		if !exists || originTx.Err != nil || originTx.Tx == nil {
			str := fmt.Sprintf("unable to find input transaction "+
				"%v for transaction %v", txInHash, txHash)
			return 0, ruleError(ErrMissingTx, str)
		}

		// Check fraud proof witness data.
		originTxIndex := txIn.PreviousOutPoint.Index

		// Using zero value outputs as inputs is banned.
		if originTx.Tx.MsgTx().TxOut[originTxIndex].Value == 0 {
			str := fmt.Sprintf("tried to spend zero value output from input %v,"+
				" idx %v",
				originTx.Tx.Sha(),
				originTxIndex)
			return 0, ruleError(ErrZeroValueOutputSpend, str)
		}

		if checkFraudProof {
			if txIn.ValueIn !=
				originTx.Tx.MsgTx().TxOut[originTxIndex].Value {
				str := fmt.Sprintf("bad fraud check value in (expected %v, "+
					"given %v) for txIn %v",
					originTx.Tx.MsgTx().TxOut[originTxIndex].Value,
					txIn.ValueIn, idx)
				return 0, ruleError(ErrFraudAmountIn, str)
			}

			if int64(txIn.BlockHeight) != originTx.BlockHeight {
				str := fmt.Sprintf("bad fraud check block height (expected %v, "+
					"given %v) for txIn %v", originTx.BlockHeight,
					txIn.BlockHeight, idx)
				return 0, ruleError(ErrFraudBlockHeight, str)
			}

			if txIn.BlockIndex != originTx.BlockIndex {
				str := fmt.Sprintf("bad fraud check block index (expected %v, "+
					"given %v) for txIn %v", originTx.BlockIndex, txIn.BlockIndex,
					idx)
				return 0, ruleError(ErrFraudBlockIndex, str)
			}
		}

		// Ensure the transaction is not spending coins which have not
		// yet reached the required coinbase maturity.
		coinbaseMaturity := int64(chainParams.CoinbaseMaturity)
		if IsCoinBase(originTx.Tx) {
			originHeight := originTx.BlockHeight
			blocksSincePrev := txHeight - originHeight
			if blocksSincePrev < coinbaseMaturity {
				str := fmt.Sprintf("tx %v tried to spend coinbase "+
					"transaction %v from height %v at "+
					"height %v before required maturity "+
					"of %v blocks", txHash, txInHash, originHeight,
					txHeight, coinbaseMaturity)
				return 0, ruleError(ErrImmatureSpend, str)
			}
		}

		// Ensure that the transaction is not spending coins from a
		// transaction that included an expiry but which has not yet
		// reached coinbase maturity many blocks.
		if originTx.Tx.MsgTx().Expiry != wire.NoExpiryValue {
			originHeight := originTx.BlockHeight
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

		// Ensure the transaction is not double spending coins.
		if originTxIndex >= uint32(len(originTx.Spent)) {
			str := fmt.Sprintf("out of bounds input index %d in "+
				"transaction %v referenced from transaction %v",
				originTxIndex, txInHash, txHash)
			return 0, ruleError(ErrBadTxInput, str)
		}
		if originTx.Spent[originTxIndex] && !(isSSGen || isSSRtx) {
			str := fmt.Sprintf("transaction %v tried to double "+
				"spend coins from transaction %v", txHash,
				txInHash)
			return 0, ruleError(ErrDoubleSpend, str)
		}

		// Ensure that the outpoint's tx tree makes sense.
		originTxOPTree := txIn.PreviousOutPoint.Tree
		originTxType := stake.DetermineTxType(originTx.Tx)
		indicatedTree := dcrutil.TxTreeRegular
		if originTxType != stake.TxTypeRegular {
			indicatedTree = dcrutil.TxTreeStake
		}
		if indicatedTree != originTxOPTree {
			errStr := fmt.Sprintf("Tx %v attempted to spend from a %v "+
				"tx tree, yet the outpoint specified a %v tx tree "+
				"instead",
				txHash,
				indicatedTree,
				originTxOPTree)
			return 0, ruleError(ErrDiscordantTxTree, errStr)
		}

		// The only transaction types that are allowed to spend from OP_SSTX
		// tagged outputs are SSGen or SSRtx tx.
		// So, check all the inputs from non SSGen or SSRtx and make sure that
		// they spend no OP_SSTX tagged outputs.
		originTxMsgTx := originTx.Tx.MsgTx()

		if !(isSSGen || isSSRtx) {
			if txscript.GetScriptClass(originTxMsgTx.TxOut[originTxIndex].Version,
				originTxMsgTx.TxOut[originTxIndex].PkScript) ==
				txscript.StakeSubmissionTy {
				_, errIsSSGen := stake.IsSSGen(tx)
				_, errIsSSRtx := stake.IsSSRtx(tx)
				errStr := fmt.Sprintf("Tx %v attempted to spend an OP_SSTX "+
					"tagged output, however it was not an SSGen or SSRtx tx"+
					"; IsSSGen err: %v, isSSRtx err: %v",
					txHash,
					errIsSSGen.Error(),
					errIsSSRtx.Error())
				return 0, ruleError(ErrTxSStxOutSpend, errStr)
			}
		}

		// OP_SSGEN and OP_SSRTX tagged outputs can only be spent after
		// coinbase maturity many blocks.
		scriptClass := txscript.GetScriptClass(
			originTxMsgTx.TxOut[originTxIndex].Version,
			originTxMsgTx.TxOut[originTxIndex].PkScript)
		if scriptClass == txscript.OP_SSGEN ||
			scriptClass == txscript.OP_SSRTX {

			originHeight := originTx.BlockHeight
			blocksSincePrev := txHeight - originHeight
			if blocksSincePrev < int64(chainParams.SStxChangeMaturity) {
				str := fmt.Sprintf("tried to spend OP_SSGEN or "+
					"OP_SSRTX output from tx %v from height %v at "+
					"height %v before required maturity "+
					"of %v blocks", txInHash, originHeight,
					txHeight, coinbaseMaturity)
				return 0, ruleError(ErrImmatureSpend, str)
			}
		}

		// SStx change outputs may only be spent after sstx change maturity many
		// blocks.
		if scriptClass == txscript.StakeSubChangeTy {
			originHeight := originTx.BlockHeight
			blocksSincePrev := txHeight - originHeight
			if blocksSincePrev < int64(chainParams.SStxChangeMaturity) {
				str := fmt.Sprintf("tried to spend SStx change "+
					"output from tx %v from height %v at "+
					"height %v before required maturity "+
					"of %v blocks", txInHash, originHeight,
					txHeight, chainParams.SStxChangeMaturity)
				return 0, ruleError(ErrImmatureSpend, str)
			}
		}

		// Ensure the transaction amounts are in range.  Each of the
		// output values of the input transactions must not be negative
		// or more than the max allowed per transaction.  All amounts in
		// a transaction are in a unit value known as a atom.  One
		// decred is a quantity of atoms as defined by the
		// AtomPerCoin constant.
		originTxAtom := originTx.Tx.MsgTx().TxOut[originTxIndex].Value
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
		// allowed per transaction.  Also, we could potentially overflow
		// the accumulator so check for overflow.
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

		// Mark the referenced output as spent.
		originTx.Spent[originTxIndex] = true
	}

	// Calculate the total output amount for this transaction.  It is safe
	// to ignore overflow and out of range errors here because those error
	// conditions would have already been caught by checkTransactionSanity.
	var totalAtomOut int64
	for i, txOut := range tx.MsgTx().TxOut {
		totalAtomOut += txOut.Value

		// Double check and make sure that, if this is not a stake transaction,
		// that no outputs have OP code tags OP_SSTX, OP_SSRTX, OP_SSGEN, or
		// OP_SSTX_CHANGE.
		if !isSStx && !isSSGen && !isSSRtx {
			scriptClass := txscript.GetScriptClass(txOut.Version, txOut.PkScript)
			if (scriptClass == txscript.StakeSubmissionTy) ||
				(scriptClass == txscript.StakeGenTy) ||
				(scriptClass == txscript.StakeRevocationTy) ||
				(scriptClass == txscript.StakeSubChangeTy) {
				errStr := fmt.Sprintf("Non-stake tx %v included stake output "+
					"type %v at in txout at position %v", txHash, scriptClass, i)
				return 0, ruleError(ErrRegTxSpendStakeOut, errStr)
			}

			// Check to make sure that non-stake transactions also are not
			// using stake tagging OP codes anywhere else in their output
			// pkScripts.
			hasStakeOpCodes, err := txscript.ContainsStakeOpCodes(txOut.PkScript)
			if err != nil {
				return 0, ruleError(ErrScriptMalformed, err.Error())
			}
			if hasStakeOpCodes {
				errStr := fmt.Sprintf("Non-stake tx %v included stake OP code "+
					"in txout at position %v", txHash, i)
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

// checkP2SHNumSigOps Checks the number of P2SH signature operations to make
// sure they don't overflow the limits.
// TxTree true == Regular, false == Stake
func checkP2SHNumSigOps(txs []*dcrutil.Tx, txInputStore TxStore,
	txTree bool) error {
	totalSigOps := 0
	for i, tx := range txs {
		isSSGen, _ := stake.IsSSGen(tx)
		numsigOps := CountSigOps(tx, (i == 0) && txTree, isSSGen)

		// Since the first (and only the first) transaction has
		// already been verified to be a coinbase transaction,
		// use (i == 0) && TxTree as an optimization for the
		// flag to countP2SHSigOps for whether or not the
		// transaction is a coinbase transaction rather than
		// having to do a full coinbase check again.
		numP2SHSigOps, err := CountP2SHSigOps(tx, (i == 0) && txTree, isSSGen,
			txInputStore)
		if err != nil {
			log.Tracef("CountP2SHSigOps failed; error "+
				"returned %v", err.Error())
			return err
		}
		numsigOps += numP2SHSigOps

		// Check for overflow or going over the limits.  We have to do
		// this on every loop iteration to avoid overflow.
		lastSigops := totalSigOps
		totalSigOps += numsigOps
		if totalSigOps < lastSigops || totalSigOps > MaxSigOpsPerBlock {
			str := fmt.Sprintf("block contains too many "+
				"signature operations - got %v, max %v",
				totalSigOps, MaxSigOpsPerBlock)
			return ruleError(ErrTooManySigOps, str)
		}
	}

	return nil
}

// checkStakeBaseAmounts calculates the total amount given as subsidy from
// single stakebase transactions (votes) within a block. This function skips a
// ton of checks already performed by CheckTransactionInputs.
func checkStakeBaseAmounts(height int64, params *chaincfg.Params,
	txs []*dcrutil.Tx, txStore TxStore) error {
	for _, tx := range txs {
		if is, _ := stake.IsSSGen(tx); is {
			// Ensure the input is available.
			txInHash := &tx.MsgTx().TxIn[1].PreviousOutPoint.Hash
			originTx, exists := txStore[*txInHash]
			if !exists || originTx.Err != nil || originTx.Tx == nil {
				str := fmt.Sprintf("couldn't find input tx %v for stakebase "+
					"amounts check", txInHash)
				return ruleError(ErrTicketUnavailable, str)
			}

			originTxIndex := tx.MsgTx().TxIn[1].PreviousOutPoint.Index
			originTxAtom := originTx.Tx.MsgTx().TxOut[originTxIndex].Value

			totalOutputs := int64(0)
			// Sum up the outputs.
			for _, out := range tx.MsgTx().TxOut {
				totalOutputs += out.Value
			}

			difference := totalOutputs - originTxAtom

			// Subsidy aligns with the height we're voting on, not with the
			// height of the current block.
			calcSubsidy := CalcStakeVoteSubsidy(height-1, params)

			if difference > calcSubsidy {
				str := fmt.Sprintf("ssgen tx %v spent more than allowed "+
					"(spent %v, allowed %v)", tx.Sha(), difference, calcSubsidy)
				return ruleError(ErrSSGenSubsidy, str)
			}
		}
	}

	return nil
}

// getStakeBaseAmounts calculates the total amount given as subsidy from
// the collective stakebase transactions (votes) within a block. This
// function skips a ton of checks already performed by
// CheckTransactionInputs.
func getStakeBaseAmounts(txs []*dcrutil.Tx, txStore TxStore) (int64, error) {
	totalInputs := int64(0)
	totalOutputs := int64(0)
	for _, tx := range txs {
		if is, _ := stake.IsSSGen(tx); is {
			// Ensure the input is available.
			txInHash := &tx.MsgTx().TxIn[1].PreviousOutPoint.Hash
			originTx, exists := txStore[*txInHash]
			if !exists || originTx.Err != nil || originTx.Tx == nil {
				str := fmt.Sprintf("couldn't find input tx %v for stakebase "+
					"amounts get",
					txInHash)
				return 0, ruleError(ErrTicketUnavailable, str)
			}

			originTxIndex := tx.MsgTx().TxIn[1].PreviousOutPoint.Index
			originTxAtom := originTx.Tx.MsgTx().TxOut[originTxIndex].Value

			totalInputs += originTxAtom

			// Sum up the outputs.
			for _, out := range tx.MsgTx().TxOut {
				totalOutputs += out.Value
			}
		}
	}

	return totalOutputs - totalInputs, nil
}

// getStakeTreeFees determines the amount of fees for in the stake tx tree
// of some node given a transaction store.
func getStakeTreeFees(height int64, params *chaincfg.Params,
	txs []*dcrutil.Tx, txStore TxStore) (dcrutil.Amount, error) {
	totalInputs := int64(0)
	totalOutputs := int64(0)
	for _, tx := range txs {
		isSSGen, _ := stake.IsSSGen(tx)

		for i, in := range tx.MsgTx().TxIn {
			// Ignore stakebases.
			if isSSGen && i == 0 {
				continue
			}

			txInHash := &in.PreviousOutPoint.Hash
			originTx, exists := txStore[*txInHash]
			if !exists || originTx.Err != nil || originTx.Tx == nil {
				str := fmt.Sprintf("couldn't find input tx %v for stake "+
					"tree fee calculation", txInHash)
				return 0, ruleError(ErrTicketUnavailable, str)
			}

			originTxIndex := in.PreviousOutPoint.Index
			originTxAtom := originTx.Tx.MsgTx().TxOut[originTxIndex].Value

			totalInputs += originTxAtom
		}

		for _, out := range tx.MsgTx().TxOut {
			totalOutputs += out.Value
		}

		// For votes, subtract the subsidy to determine actual
		// fees.
		if isSSGen {
			// Subsidy aligns with the height we're voting on, not with the
			// height of the current block.
			totalOutputs -= CalcStakeVoteSubsidy(height-1, params)
		}
	}

	if totalInputs < totalOutputs {
		str := fmt.Sprintf("negative cumulative fees found in stake tx tree")
		return 0, ruleError(ErrStakeFees, str)
	}

	return dcrutil.Amount(totalInputs - totalOutputs), nil
}

// checkTransactionInputs is the local function used to check the transaction
// inputs for a transaction list given a predetermined TxStore.
// TxTree true == Regular, false == Stake
func (b *BlockChain) checkTransactionInputs(
	inputFees dcrutil.Amount,
	node *blockNode,
	txs []*dcrutil.Tx,
	txStore TxStore,
	txTree bool) error {
	// Perform several checks on the inputs for each transaction.  Also
	// accumulate the total fees.  This could technically be combined with
	// the loop above instead of running another loop over the transactions,
	// but by separating it we can avoid running the more expensive (though
	// still relatively cheap as compared to running the scripts) checks
	// against all the inputs when the signature operations are out of
	// bounds.
	totalFees := int64(inputFees) // Stake tx tree carry forward
	for _, tx := range txs {
		// Check double spending for some stake types, because
		// checkTransactionInputs doesn't do this for SSGens or
		// SSRtxs.
		isSSGen, _ := stake.IsSSGen(tx)
		isSSRtx, _ := stake.IsSSRtx(tx)
		if isSSGen || isSSRtx {
			for i, txIn := range tx.MsgTx().TxIn {
				// Stakebase handling.
				if isSSGen && i == 0 {
					continue
				}

				// Ensure the input is available.
				txInHash := &txIn.PreviousOutPoint.Hash
				originTx, exists := txStore[*txInHash]
				originTxIndex := txIn.PreviousOutPoint.Index
				if !exists {
					str := fmt.Sprintf("missing input tx for index %d in "+
						"transaction %v referenced from stake transaction %v",
						originTxIndex, txInHash, tx.Sha())
					return ruleError(ErrBadTxInput, str)
				}

				// Ensure the transaction is not double spending coins.
				if originTxIndex >= uint32(len(originTx.Spent)) {
					str := fmt.Sprintf("out of bounds input index %d in "+
						"transaction %v referenced from stake transaction %v",
						originTxIndex, txInHash, tx.Sha())
					return ruleError(ErrBadTxInput, str)
				}
				if originTx.Spent[originTxIndex] {
					str := fmt.Sprintf("stake transaction %v tried to double "+
						"spend coins from transaction %v", tx.Sha(),
						txInHash)
					return ruleError(ErrDoubleSpend, str)
				}
			}
		}

		// This step modifies the txStore and marks the tx outs used
		// spent, so be aware of this.
		txFee, err := CheckTransactionInputs(tx,
			node.height,
			txStore,
			true, // Check fraud proofs
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
	}

	// The total output values of the coinbase transaction must not exceed
	// the expected subsidy value plus total transaction fees gained from
	// mining the block.  It is safe to ignore overflow and out of range
	// errors here because those error conditions would have already been
	// caught by checkTransactionSanity.
	if txTree { //TxTreeRegular
		// Apply penalty to fees if we're at stake validation height.
		if node.height >= b.chainParams.StakeValidationHeight {
			totalFees *= int64(node.header.Voters)
			totalFees /= int64(b.chainParams.TicketsPerBlock)
		}

		var totalAtomOutRegular int64

		for _, txOut := range txs[0].MsgTx().TxOut {
			totalAtomOutRegular += txOut.Value
		}

		var expectedAtomOut int64
		if node.height == 1 {
			expectedAtomOut = calcBlockSubsidy(node.height, b.chainParams)
		} else {
			subsidyWork := CalcBlockWorkSubsidy(node.height, node.header.Voters,
				b.chainParams)
			subsidyTax := CalcBlockTaxSubsidy(node.height, node.header.Voters,
				b.chainParams)
			expectedAtomOut = subsidyWork + subsidyTax + totalFees
		}

		// AmountIn for the input should be equal to the subsidy.
		coinbaseIn := txs[0].MsgTx().TxIn[0]
		subsidyWithoutFees := expectedAtomOut - totalFees
		if (coinbaseIn.ValueIn != subsidyWithoutFees) && (node.height > 0) {
			errStr := fmt.Sprintf("bad coinbase subsidy in input; got %v, "+
				"expect %v", coinbaseIn.ValueIn, subsidyWithoutFees)
			return ruleError(ErrBadCoinbaseAmountIn, errStr)
		}

		if totalAtomOutRegular > expectedAtomOut {
			str := fmt.Sprintf("coinbase transaction for block %v pays %v "+
				"which is more than expected value of %v",
				node.hash, totalAtomOutRegular, expectedAtomOut)
			return ruleError(ErrBadCoinbaseValue, str)
		}
	} else { // TxTreeStake
		if len(txs) == 0 && node.height < b.chainParams.StakeValidationHeight {
			return nil
		}
		if len(txs) == 0 && node.height >= b.chainParams.StakeValidationHeight {
			str := fmt.Sprintf("empty tx tree stake in block after " +
				"stake validation height")
			return ruleError(ErrNoStakeTx, str)
		}

		err := checkStakeBaseAmounts(node.height, b.chainParams, txs, txStore)
		if err != nil {
			return err
		}

		totalAtomOutStake, err := getStakeBaseAmounts(txs, txStore)
		if err != nil {
			return err
		}

		expectedAtomOut := int64(0)
		if node.height >= b.chainParams.StakeValidationHeight {
			// Subsidy aligns with the height we're voting on, not with the
			// height of the current block.
			expectedAtomOut = CalcStakeVoteSubsidy(node.height-1,
				b.chainParams) * int64(node.header.Voters)
		} else {
			expectedAtomOut = totalFees
		}

		if totalAtomOutStake > expectedAtomOut {
			str := fmt.Sprintf("stakebase transactions for block pays %v "+
				"which is more than expected value of %v",
				totalAtomOutStake, expectedAtomOut)
			return ruleError(ErrBadStakebaseValue, str)
		}
	}

	return nil
}

// checkConnectBlock performs several checks to confirm connecting the passed
// block to the main chain (including whatever reorganization might be necessary
// to get this node to the main chain) does not violate any rules.
//
// The CheckConnectBlock function makes use of this function to perform the
// bulk of its work.  The only difference is this function accepts a node which
// may or may not require reorganization to connect it to the main chain whereas
// CheckConnectBlock creates a new node which specifically connects to the end
// of the current main chain and then calls this function with that node.
//
// See the comments for CheckConnectBlock for some examples of the type of
// checks performed by this function.
func (b *BlockChain) checkConnectBlock(node *blockNode,
	block *dcrutil.Block) error {
	// If the side chain blocks end up in the database, a call to
	// CheckBlockSanity should be done here in case a previous version
	// allowed a block that is no longer valid.  However, since the
	// implementation only currently uses memory for the side chain blocks,
	// it isn't currently necessary.
	parent, err := b.getPrevNodeFromNode(node)
	if err != nil {
		return err
	}
	parentBlock, err := b.getBlockFromHash(node.parentHash)
	if err != nil {
		return ruleError(ErrMissingParent, err.Error())
	}

	// The coinbase for the Genesis block is not spendable, so just return
	// now.
	if node.hash.IsEqual(b.chainParams.GenesisHash) && b.bestChain == nil {
		return nil
	}

	err = b.checkDupTxs(node, parent, block, parentBlock)
	if err != nil {
		errStr := fmt.Sprintf("checkDupTxs failed for incoming "+
			"node %v; error given: %v", node.hash, err)
		return ruleError(ErrBIP0030, errStr)
	}

	// Request a map that contains all input transactions for the following
	// TxTrees:
	// 1) Parent TxTreeRegular (if validated)
	// 2) Current TxTreeStake
	// 3) Current TxTreeRegular
	// These transactions are needed for verification of things such as
	// transaction inputs, counting pay-to-script-hashes, and scripts.
	//
	// TODO This is very slow. Ideally, we should fetch once, check
	// what we need to for this state, then update to the next state
	// with the connect block function for a single tx store.
	// Additionally, txStores for sidechains being evaluated should
	// be cached so that if another block is added on top of the side
	// chain, we don't have to recalculate the entire ticket store
	// again resulting in O(n) behaviour per block check connect
	// that can easily be O(1).
	// The same going for ticket lookup, and would offer a dramatic
	// improvement there as well. Blocks on extremely long side
	// chains may take a very long time to validate with the current
	// code, with hundreds of blocks taking hours.
	regularTxTreeValid := dcrutil.IsFlagSet16(node.header.VoteBits,
		dcrutil.BlockValid)
	thisNodeStakeViewpoint := ViewpointPrevInvalidStake
	thisNodeRegularViewpoint := ViewpointPrevInvalidRegular
	var txInputStoreInitial TxStore

	// TxStore at blockchain HEAD.
	if regularTxTreeValid {
		txInputStoreInitial, err = b.fetchInputTransactions(node, block,
			ViewpointPrevValidInitial)
		if err != nil {
			log.Tracef("fetchInputTransactions failed for incoming "+
				"node %v; error given: %v", node.hash, err)
			return err
		}

		thisNodeStakeViewpoint = ViewpointPrevValidStake
		thisNodeRegularViewpoint = ViewpointPrevValidRegular
	}

	// TxStore at blockchain HEAD + TxTreeRegular of prevBlock (if
	// validated).
	txInputStoreStake, err := b.fetchInputTransactions(node, block,
		thisNodeStakeViewpoint)
	if err != nil {
		log.Tracef("fetchInputTransactions failed for incoming "+
			"node %v; error given: %v", node.hash, err)
		return err
	}

	// TxStore at blockchain HEAD + TxTreeRegular of prevBlock (if
	// validated) + TxTreeStake of current block.
	txInputStoreRegular, err := b.fetchInputTransactions(node, block,
		thisNodeRegularViewpoint)
	if err != nil {
		log.Tracef("fetchInputTransactions failed for incoming "+
			"node %v; error given: %v", node.hash, err)
		return err
	}

	// Check to ensure consensus via the PoS ticketing system versus the
	// informations stored in the header.
	ticketStore, err := b.fetchTicketStore(node)
	if err != nil {
		log.Tracef("Failed to generate ticket store for incoming "+
			"node %v; error given: %v", node.hash, err)
		return err
	}

	err = b.CheckBlockStakeSanity(ticketStore,
		b.stakeValidationHeight,
		node,
		block,
		parentBlock,
		b.chainParams)
	if err != nil {
		log.Tracef("CheckBlockStakeSanity failed for incoming "+
			"node %v; error given: %v", node.hash, err)
		return err
	}

	// The number of signature operations must be less than the maximum
	// allowed per block.  Note that the preliminary sanity checks on a
	// block also include a check similar to this one, but this check
	// expands the count to include a precise count of pay-to-script-hash
	// signature operations in each of the input transaction public key
	// scripts.
	// Do this for all TxTrees.
	if regularTxTreeValid {
		err = checkP2SHNumSigOps(parentBlock.Transactions(),
			txInputStoreInitial, true)
		if err != nil {
			return err
		}
	}

	err = checkP2SHNumSigOps(block.STransactions(),
		txInputStoreStake, false)
	if err != nil {
		return err
	}

	err = checkP2SHNumSigOps(block.Transactions(),
		txInputStoreRegular, true)
	if err != nil {
		return err
	}

	// Perform several checks on the inputs for each transaction.  Also
	// accumulate the total fees.  This could technically be combined with
	// the loop above instead of running another loop over the transactions,
	// but by separating it we can avoid running the more expensive (though
	// still relatively cheap as compared to running the scripts) checks
	// against all the inputs when the signature operations are out of
	// bounds.
	// TxTreeRegular of previous block.
	if regularTxTreeValid {
		// TODO when validating the previous block, cache the stake
		// fees in a node so you don't have to redo these expensive
		// lookups.
		parentTxTreeValid := dcrutil.IsFlagSet16(parent.header.VoteBits,
			dcrutil.BlockValid)
		thisParentNodeStakeViewpoint := ViewpointPrevInvalidStake
		beforeSVH := node.height < b.chainParams.StakeValidationHeight
		firstBlock := node.height == 1
		if parentTxTreeValid || (beforeSVH && !firstBlock) {
			thisParentNodeStakeViewpoint = ViewpointPrevValidStake
		}

		parentTxInputStoreStake, err := b.fetchInputTransactions(parent,
			parentBlock, thisParentNodeStakeViewpoint)
		if err != nil {
			log.Tracef("fetchInputTransactions failed for incoming "+
				"parent node %v stake tree; error given: %v", node.hash, err)
			return err
		}

		stakeTreeFees, err := getStakeTreeFees(parent.height,
			b.chainParams,
			parentBlock.STransactions(),
			parentTxInputStoreStake)
		if err != nil {
			log.Tracef("getStakeTreeFees failed for prev "+
				"TxTreeStake: %v", err.Error())
			return err
		}

		err = b.checkTransactionInputs(stakeTreeFees, parent,
			parentBlock.Transactions(), txInputStoreInitial, true)
		if err != nil {
			log.Tracef("checkTransactionInputs failed for prev "+
				"TxTreeRegular: %v", err.Error())
			return err
		}
	}

	// TxTreeStake of current block.
	err = b.checkTransactionInputs(0, node, block.STransactions(),
		txInputStoreStake, false)
	if err != nil {
		log.Tracef("checkTransactionInputs failed for cur "+
			"TxTreeStake: %v", err.Error())
		return err
	}

	stakeTreeFees, err := getStakeTreeFees(node.height,
		b.chainParams,
		block.STransactions(),
		txInputStoreStake)
	if err != nil {
		log.Tracef("getStakeTreeFees failed for cur "+
			"TxTreeStake: %v", err.Error())
		return err
	}

	// TxTreeRegular of current block.
	err = b.checkTransactionInputs(stakeTreeFees, node, block.Transactions(),
		txInputStoreRegular, true)
	if err != nil {
		log.Tracef("checkTransactionInputs failed for cur "+
			"TxTreeRegular: %v", err.Error())
		return err
	}

	// Don't run scripts if this node is before the latest known good
	// checkpoint since the validity is verified via the checkpoints (all
	// transactions are included in the merkle root hash and any changes
	// will therefore be detected by the next checkpoint).  This is a huge
	// optimization because running the scripts is the most time consuming
	// portion of block handling.
	checkpoint := b.LatestCheckpoint()
	runScripts := !b.noVerify
	if checkpoint != nil && node.height <= checkpoint.Height {
		runScripts = false
	}

	// Now that the inexpensive checks are done and have passed, verify the
	// transactions are actually allowed to spend the coins by running the
	// expensive ECDSA signature check scripts.  Doing this last helps
	// prevent CPU exhaustion attacks.
	if runScripts {
		var scriptFlags txscript.ScriptFlags
		scriptFlags |= txscript.ScriptBip16
		scriptFlags |= txscript.ScriptVerifyDERSignatures
		scriptFlags |= txscript.ScriptVerifyStrictEncoding
		scriptFlags |= txscript.ScriptVerifyMinimalData
		scriptFlags |= txscript.ScriptVerifyCleanStack
		scriptFlags |= txscript.ScriptVerifyCheckLockTimeVerify

		if regularTxTreeValid {
			// TxTreeRegular of previous block.
			err = checkBlockScripts(parentBlock, txInputStoreInitial,
				true, scriptFlags, b.sigCache)
			if err != nil {
				log.Tracef("checkBlockScripts failed; error "+
					"returned on txtreeregular of prev block: %v",
					err.Error())
				return err
			}
		}

		// TxTreeStake of current block.
		err = checkBlockScripts(block, txInputStoreStake, false,
			scriptFlags, b.sigCache)
		if err != nil {
			log.Tracef("checkBlockScripts failed; error "+
				"returned on txtreestake of cur block: %v", err.Error())
			return err
		}

		// TxTreeRegular of current block.
		err = checkBlockScripts(block, txInputStoreRegular, true,
			scriptFlags, b.sigCache)
		if err != nil {
			log.Tracef("checkBlockScripts failed; error "+
				"returned on txtreeregular of cur block: %v", err.Error())
			return err
		}
	}

	// First block has special rules concerning the ledger.
	if node.height == 1 {
		err := BlockOneCoinbasePaysTokens(block.Transactions()[0],
			b.chainParams)
		if err != nil {
			return err
		}
	}

	return nil
}

// CheckConnectBlock performs several checks to confirm connecting the passed
// block to the main chain does not violate any rules.  An example of some of
// the checks performed are ensuring connecting the block would not cause any
// duplicate transaction hashes for old transactions that aren't already fully
// spent, double spends, exceeding the maximum allowed signature operations
// per block, invalid values in relation to the expected block subsidy, or fail
// transaction script validation.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) CheckConnectBlock(block *dcrutil.Block) error {
	parentHash := block.MsgBlock().Header.PrevBlock
	prevNode, err := b.findNode(&parentHash)
	if err != nil {
		return ruleError(ErrMissingParent, err.Error())
	}

	var voteBitsStake []uint16
	for _, stx := range block.STransactions() {
		if is, _ := stake.IsSSGen(stx); is {
			vb := stake.GetSSGenVoteBits(stx)
			voteBitsStake = append(voteBitsStake, vb)
		}
	}

	newNode := newBlockNode(&block.MsgBlock().Header, block.Sha(),
		block.Height(), voteBitsStake)
	if prevNode != nil {
		newNode.parent = prevNode
		newNode.workSum.Add(prevNode.workSum, newNode.workSum)
	}

	return b.checkConnectBlock(newNode, block)
}
