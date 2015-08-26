// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
//
// Contains a collection of functions that determine what type of stake tx a
// given tx is and does a cursory check for sanity.

package stake

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/big"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainec"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

// TxType indicates the type of tx (regular or stake type).
type TxType int

// Possible TxTypes. Statically declare these so that they might be used in
// consensus code.
const (
	TxTypeRegular = 0
	TxTypeSStx    = 1
	TxTypeSSGen   = 2
	TxTypeSSRtx   = 3
)

const (
	// MaxInputsPerSStx is the maximum number of inputs allowed in an SStx.
	MaxInputsPerSStx = 64

	// MaxOutputsPerSStx is the maximum number of outputs allowed in an SStx;
	// you need +1 for the tagged SStx output.
	MaxOutputsPerSStx = MaxInputsPerSStx*2 + 1

	// NumInputsPerSSGen is the exact number of inputs for an SSGen
	// (stakebase) tx. Inputs are a tagged SStx output and a stakebase (null)
	// input.
	NumInputsPerSSGen = 2 // SStx and stakebase

	// MaxOutputsPerSSGen is the maximum number of outputs in an SSGen tx,
	// which are all outputs to the addresses specified in the OP_RETURNs of
	// the original SStx referenced as input plus reference and vote
	// OP_RETURN outputs in the zeroeth and first position.
	MaxOutputsPerSSGen = MaxInputsPerSStx + 2

	// NumInputsPerSSRtx is the exact number of inputs for an SSRtx (stake
	// revocation tx); the only input should be the SStx output.
	NumInputsPerSSRtx = 1

	// MaxOutputsPerSSRtx is the maximum number of outputs in an SSRtx, which
	// are all outputs to the addresses specified in the OP_RETURNs of the
	// original SStx referenced as input plus a reference to the block header
	// hash of the block in which voting was missed.
	MaxOutputsPerSSRtx = MaxInputsPerSStx

	// SStxPKHMinOutSize is the minimum size of of an OP_RETURN commitment output
	// for an SStx tx.
	// 20 bytes P2SH/P2PKH + 8 byte amount + 4 byte fee range limits
	SStxPKHMinOutSize = 32

	// SStxPKHMaxOutSize is the maximum size of of an OP_RETURN commitment output
	// for an SStx tx.
	SStxPKHMaxOutSize = 77

	// SSGenBlockReferenceOutSize is the size of a block reference OP_RETURN
	// output for an SSGen tx.
	SSGenBlockReferenceOutSize = 38

	// SSGenVoteBitsOutputMinSize is the minimum size for a VoteBits push
	// in an SSGen.
	SSGenVoteBitsOutputMinSize = 4

	// SSGenVoteBitsOutputMaxSize is the maximum size for a VoteBits push
	// in an SSGen.
	SSGenVoteBitsOutputMaxSize = 77

	// MaxSingleBytePushLength is the largest maximum push for an
	// SStx commitment or VoteBits push.
	MaxSingleBytePushLength = 75

	// SStxVoteReturnFractionMask extracts the return fraction from a
	// commitment output version.
	// If after applying this mask &0x003f is given, the entire amount of
	// the output is allowed to be spent as fees if the flag to allow fees
	// is set.
	SStxVoteReturnFractionMask = 0x003f

	// SStxRevReturnFractionMask extracts the return fraction from a
	// commitment output version.
	// If after applying this mask &0x3f00 is given, the entire amount of
	// the output is allowed to be spent as fees if the flag to allow fees
	// is set.
	SStxRevReturnFractionMask = 0x3f00

	// SStxVoteFractionFlag is a bitflag mask specifying whether or not to
	// apply a fractional limit to the amount used for fees in a vote.
	// 00000000 00000000 = No fees allowed
	// 00000000 01000000 = Apply fees rule
	SStxVoteFractionFlag = 0x0040

	// SStxRevFractionFlag is a bitflag mask specifying whether or not to
	// apply a fractional limit to the amount used for fees in a vote.
	// 00000000 00000000 = No fees allowed
	// 01000000 00000000 = Apply fees rule
	SStxRevFractionFlag = 0x4000

	// fractioningAmount is the amount to divide by after multiplying
	// to obtain the limit for the output's amount.
	fractioningAmount = 64
)

var (
	// validSStxAddressOutPrefix is the valid prefix for a 30-byte
	// minimum OP_RETURN push for a commitment for an SStx.
	// Example SStx address out:
	// 0x6a (OP_RETURN)
	// 0x1e (OP_DATA_30, push length: 30 bytes)
	//
	// 0x?? 0x?? 0x?? 0x?? (20 byte public key hash)
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x??
	//
	// 0x?? 0x?? 0x?? 0x?? (8 byte amount)
	// 0x?? 0x?? 0x?? 0x??
	//
	// 0x?? 0x??           (2 byte range limits)
	validSStxAddressOutMinPrefix = []byte{0x6a, 0x1e}

	// validSSGenReferenceOutPrefix is the valid prefix for a block
	// reference output for an SSGen tx.
	// Example SStx address out:
	// 0x6a (OP_RETURN)
	// 0x28 (OP_DATA_40, push length: 40 bytes)
	//
	// 0x?? 0x?? 0x?? 0x?? (32 byte block header hash for the block
	// 0x?? 0x?? 0x?? 0x??   you wish to vote on)
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	// 0x?? 0x?? 0x?? 0x??
	//
	// 0x?? 0x?? 0x?? 0x?? (4 byte uint32 for the height of the block
	//                      that you wish to vote on)
	validSSGenReferenceOutPrefix = []byte{0x6a, 0x24}

	// validSSGenVoteOutMinPrefix is the valid prefix for a vote output for an
	// SSGen tx.
	// 0x6a (OP_RETURN)
	// 0x02 (OP_DATA_2 to OP_DATA_75, push length: 2-75 bytes)
	//
	// 0x?? 0x?? (VoteBits) ... 0x??
	validSSGenVoteOutMinPrefix = []byte{0x6a, 0x02}

	// zeroHash is the zero value for a wire.ShaHash and is defined as
	// a package level variable to avoid the need to create a new instance
	// every time a check is needed.
	zeroHash = &chainhash.Hash{}

	// rangeLimitMax is the maximum bitshift for a fees limit on an
	// sstx commitment output.
	rangeLimitMax = uint16(63)
)

// --------------------------------------------------------------------------------
// Accessory Stake Functions
// --------------------------------------------------------------------------------

// isNullOutpoint determines whether or not a previous transaction output point
// is set.
func isNullOutpoint(tx *dcrutil.Tx) bool {
	nullInOP := tx.MsgTx().TxIn[0].PreviousOutPoint
	if nullInOP.Index == math.MaxUint32 && nullInOP.Hash.IsEqual(zeroHash) &&
		nullInOP.Tree == dcrutil.TxTreeRegular {
		return true
	}
	return false
}

// isNullFraudProof determines whether or not a previous transaction fraud proof
// is set.
func isNullFraudProof(tx *dcrutil.Tx) bool {
	txIn := tx.MsgTx().TxIn[0]
	switch {
	case txIn.BlockHeight != wire.NullBlockHeight:
		return false
	case txIn.BlockIndex != wire.NullBlockIndex:
		return false
	}

	return true
}

// IsStakeBase returns whether or not a tx could be considered as having a
// topically valid stake base present.
func IsStakeBase(tx *dcrutil.Tx) bool {
	msgTx := tx.MsgTx()

	// A stake base (SSGen) must only have two transaction inputs.
	if len(msgTx.TxIn) != 2 {
		return false
	}

	// The previous output of a coin base must have a max value index and
	// a zero hash, as well as null fraud proofs.
	if !isNullOutpoint(tx) {
		return false
	}
	if !isNullFraudProof(tx) {
		return false
	}

	return true
}

// MinimalOutput is a struct encoding a minimally sized output for use in parsing
// stake related information.
type MinimalOutput struct {
	PkScript []byte
	Value    int64
	Version  uint16
}

// ConvertToMinimalOutputs converts a transaction to its minimal outputs
// derivative.
func ConvertToMinimalOutputs(tx *dcrutil.Tx) []*MinimalOutput {
	minOuts := make([]*MinimalOutput, len(tx.MsgTx().TxOut))
	for i, txOut := range tx.MsgTx().TxOut {
		minOuts[i] = &MinimalOutput{
			PkScript: txOut.PkScript,
			Value:    txOut.Value,
			Version:  txOut.Version,
		}
	}

	return minOuts
}

// SStxStakeOutputInfo takes an SStx as input and scans through its outputs,
// returning the pubkeyhashs and amounts for any NullDataTy's (future
// commitments to stake generation rewards).
func SStxStakeOutputInfo(outs []*MinimalOutput) ([]bool, [][]byte, []int64,
	[]int64, [][]bool, [][]uint16) {
	expectedInLen := len(outs) / 2
	isP2SH := make([]bool, expectedInLen)
	addresses := make([][]byte, expectedInLen)
	amounts := make([]int64, expectedInLen)
	changeAmounts := make([]int64, expectedInLen)
	allSpendRules := make([][]bool, expectedInLen)
	allSpendLimits := make([][]uint16, expectedInLen)

	// Cycle through the inputs and pull the proportional amounts
	// and commit to PKHs/SHs.
	for idx, out := range outs {
		// We only care about the outputs where we get proportional
		// amounts and the PKHs/SHs to send rewards to, which is all
		// the odd numbered output indexes.
		if (idx > 0) && (idx%2 != 0) {
			// The MSB (sign), not used ever normally, encodes whether
			// or not it is a P2PKH or P2SH for the input.
			amtEncoded := make([]byte, 8, 8)
			copy(amtEncoded, out.PkScript[22:30])
			isP2SH[idx/2] = !(amtEncoded[7]&(1<<7) == 0) // MSB set?
			amtEncoded[7] &= ^uint8(1 << 7)              // Clear bit

			addresses[idx/2] = out.PkScript[2:22]
			amounts[idx/2] = int64(binary.LittleEndian.Uint64(amtEncoded))

			// Get flags and restrictions for the outputs to be
			// make in either a vote or revocation.
			spendRules := make([]bool, 2, 2)
			spendLimits := make([]uint16, 2, 2)

			// This bitflag is true/false.
			feeLimitUint16 := binary.LittleEndian.Uint16(out.PkScript[30:32])
			spendRules[0] = (feeLimitUint16 & SStxVoteFractionFlag) ==
				SStxVoteFractionFlag
			spendRules[1] = (feeLimitUint16 & SStxRevFractionFlag) ==
				SStxRevFractionFlag
			allSpendRules[idx/2] = spendRules

			// This is the fraction to use out of 64.
			spendLimits[0] = feeLimitUint16 & SStxVoteReturnFractionMask
			spendLimits[1] = feeLimitUint16 & SStxRevReturnFractionMask
			spendLimits[1] >>= 8
			allSpendLimits[idx/2] = spendLimits
		}

		// Here we only care about the change amounts, so scan
		// the change outputs (even indices) and save their
		// amounts.
		if (idx > 0) && (idx%2 == 0) {
			changeAmounts[(idx/2)-1] = out.Value
		}
	}

	return isP2SH, addresses, amounts, changeAmounts, allSpendRules,
		allSpendLimits
}

// TxSStxStakeOutputInfo takes an SStx as input and scans through its outputs,
// returning the pubkeyhashs and amounts for any NullDataTy's (future
// commitments to stake generation rewards).
func TxSStxStakeOutputInfo(tx *dcrutil.Tx) ([]bool, [][]byte, []int64, []int64,
	[][]bool, [][]uint16) {
	return SStxStakeOutputInfo(ConvertToMinimalOutputs(tx))
}

// AddrFromSStxPkScrCommitment extracts a P2SH or P2PKH address from a
// ticket commitment pkScript.
func AddrFromSStxPkScrCommitment(pkScript []byte,
	params *chaincfg.Params) (dcrutil.Address, error) {
	if len(pkScript) < SStxPKHMinOutSize {
		return nil, stakeRuleError(ErrSStxBadCommitAmount, "short read "+
			"of sstx commit pkscript")
	}

	// The MSB (sign), not used ever normally, encodes whether
	// or not it is a P2PKH or P2SH for the input.
	amtEncoded := make([]byte, 8, 8)
	copy(amtEncoded, pkScript[22:30])
	isP2SH := !(amtEncoded[7]&(1<<7) == 0) // MSB set?

	// The 20 byte PKH or SH.
	hashBytes := pkScript[2:22]

	var err error
	var addr dcrutil.Address
	if isP2SH {
		addr, err = dcrutil.NewAddressScriptHashFromHash(hashBytes, params)
	} else {
		addr, err = dcrutil.NewAddressPubKeyHash(hashBytes, params,
			chainec.ECTypeSecp256k1)
	}

	return addr, err
}

// AmountFromSStxPkScrCommitment extracts a commitment amount from a
// ticket commitment pkScript.
func AmountFromSStxPkScrCommitment(pkScript []byte) (dcrutil.Amount, error) {
	if len(pkScript) < SStxPKHMinOutSize {
		return 0, stakeRuleError(ErrSStxBadCommitAmount, "short read "+
			"of sstx commit pkscript")
	}

	// The MSB (sign), not used ever normally, encodes whether
	// or not it is a P2PKH or P2SH for the input.
	amtEncoded := make([]byte, 8, 8)
	copy(amtEncoded, pkScript[22:30])
	amtEncoded[7] &= ^uint8(1 << 7) // Clear bit for P2SH flag

	return dcrutil.Amount(binary.LittleEndian.Uint64(amtEncoded)), nil
}

// TxSSGenStakeOutputInfo takes an SSGen tx as input and scans through its
// outputs, returning the amount of the output and the PKH or SH that it was
// sent to.
func TxSSGenStakeOutputInfo(tx *dcrutil.Tx, params *chaincfg.Params) ([]bool,
	[][]byte, []int64, error) {
	msgTx := tx.MsgTx()
	numOutputsInSSGen := len(msgTx.TxOut)

	isP2SH := make([]bool, numOutputsInSSGen-2)
	addresses := make([][]byte, numOutputsInSSGen-2)
	amounts := make([]int64, numOutputsInSSGen-2)

	// Cycle through the inputs and generate
	for idx, out := range msgTx.TxOut {
		// We only care about the outputs where we get proportional
		// amounts and the PKHs they were sent to.
		if (idx > 1) && (idx < numOutputsInSSGen) {
			// Get the PKH or SH it's going to, and what type of
			// script it is.
			class, addr, _, err :=
				txscript.ExtractPkScriptAddrs(out.Version, out.PkScript, params)
			if err != nil {
				return nil, nil, nil, err
			}
			if class != txscript.StakeGenTy {
				return nil, nil, nil, fmt.Errorf("ssgen output included non "+
					"ssgen tagged output in idx %v", idx)
			}
			subClass, err := txscript.GetStakeOutSubclass(out.PkScript)
			if !(subClass == txscript.PubKeyHashTy ||
				subClass == txscript.ScriptHashTy) {
				return nil, nil, nil, fmt.Errorf("bad script type")
			}
			isP2SH[idx-2] = false
			if subClass == txscript.ScriptHashTy {
				isP2SH[idx-2] = true
			}

			// Get the amount that was sent.
			amt := out.Value
			addresses[idx-2] = addr[0].ScriptAddress()
			amounts[idx-2] = amt
		}
	}

	return isP2SH, addresses, amounts, nil
}

// SSGenBlockVotedOn takes an SSGen tx and returns the block voted on in the
// first OP_RETURN by hash and height.
func SSGenBlockVotedOn(tx *dcrutil.Tx) (chainhash.Hash, uint32, error) {
	msgTx := tx.MsgTx()

	// Get the block header hash.
	blockSha, err := chainhash.NewHash(msgTx.TxOut[0].PkScript[2:34])
	if err != nil {
		return chainhash.Hash{}, 0, err
	}

	// Get the block height.
	height := binary.LittleEndian.Uint32(msgTx.TxOut[0].PkScript[34:38])

	return *blockSha, height, nil
}

// SSGenVoteBits takes an SSGen tx as input and scans through its
// outputs, returning the VoteBits of the index 1 output.
func SSGenVoteBits(tx *dcrutil.Tx) uint16 {
	msgTx := tx.MsgTx()

	votebits := binary.LittleEndian.Uint16(msgTx.TxOut[1].PkScript[2:4])

	return votebits
}

// TxSSRtxStakeOutputInfo takes an SSRtx tx as input and scans through its
// outputs, returning the amount of the output and the pkh that it was sent to.
func TxSSRtxStakeOutputInfo(tx *dcrutil.Tx, params *chaincfg.Params) ([]bool,
	[][]byte, []int64, error) {
	msgTx := tx.MsgTx()
	numOutputsInSSRtx := len(msgTx.TxOut)

	isP2SH := make([]bool, numOutputsInSSRtx)
	addresses := make([][]byte, numOutputsInSSRtx)
	amounts := make([]int64, numOutputsInSSRtx)

	// Cycle through the inputs and generate
	for idx, out := range msgTx.TxOut {
		// Get the PKH or SH it's going to, and what type of
		// script it is.
		class, addr, _, err :=
			txscript.ExtractPkScriptAddrs(out.Version, out.PkScript, params)
		if err != nil {
			return nil, nil, nil, err
		}
		if class != txscript.StakeRevocationTy {
			return nil, nil, nil, fmt.Errorf("ssrtx output included non "+
				"ssrtx tagged output in idx %v", idx)
		}
		subClass, err := txscript.GetStakeOutSubclass(out.PkScript)
		if !(subClass == txscript.PubKeyHashTy ||
			subClass == txscript.ScriptHashTy) {
			return nil, nil, nil, fmt.Errorf("bad script type")
		}
		isP2SH[idx] = false
		if subClass == txscript.ScriptHashTy {
			isP2SH[idx] = true
		}

		// Get the amount that was sent.
		amt := out.Value

		addresses[idx] = addr[0].ScriptAddress()
		amounts[idx] = amt
	}

	return isP2SH, addresses, amounts, nil
}

// SStxNullOutputAmounts takes an array of input amounts, change amounts, and a
// ticket purchase amount, calculates the adjusted proportion from the purchase
// amount, stores it in an array, then returns the array. That is, for any given
// SStx, this function calculates the proportional outputs that any single user
// should receive.
// Returns: (1) Fees (2) Output Amounts (3) Error
func SStxNullOutputAmounts(amounts []int64,
	changeAmounts []int64,
	amountTicket int64) (int64, []int64, error) {
	lengthAmounts := len(amounts)

	if lengthAmounts != len(changeAmounts) {
		errStr := fmt.Sprintf("amounts was not equal in length " +
			"to change amounts!")
		return 0, nil, errors.New(errStr)
	}

	if amountTicket <= 0 {
		errStr := fmt.Sprintf("committed amount was too small!")
		return 0, nil, stakeRuleError(ErrSStxBadCommitAmount, errStr)
	}

	contribAmounts := make([]int64, lengthAmounts)
	sum := int64(0)

	// Now we want to get the adjusted amounts. The algorithm is like this:
	// 1 foreach amount
	// 2     subtract change from input, store
	// 3     add this amount to sum
	// 4 check sum against the total committed amount
	for i := 0; i < lengthAmounts; i++ {
		contribAmounts[i] = amounts[i] - changeAmounts[i]
		if contribAmounts[i] < 0 {
			errStr := fmt.Sprintf("change at idx %v spent more coins than "+
				"allowed (have: %v, spent: %v)", i, amounts[i], changeAmounts[i])
			return 0, nil, stakeRuleError(ErrSStxBadChangeAmts, errStr)
		}

		sum += contribAmounts[i]
	}

	fees := sum - amountTicket

	return fees, contribAmounts, nil
}

// CalculateRewards takes a list of SStx adjusted output amounts, the amount used
// to purchase that ticket, and the reward for an SSGen tx and subsequently
// generates what the outputs should be in the SSGen tx. If used for calculating
// the outputs for an SSRtx, pass 0 for subsidy.
func CalculateRewards(amounts []int64, amountTicket int64,
	subsidy int64) []int64 {
	outputsAmounts := make([]int64, len(amounts))

	// SSGen handling
	amountWithStakebase := amountTicket + subsidy

	// Get the sum of the amounts contributed between both fees
	// and contributions to the ticket.
	totalContrib := int64(0)
	for _, amount := range amounts {
		totalContrib += amount
	}

	// Now we want to get the adjusted amounts including the reward.
	// The algorithm is like this:
	// 1 foreach amount
	// 2     amount *= 2^32
	// 3     amount /= amountTicket
	// 4     amount *= amountWithStakebase
	// 5     amount /= 2^32
	amountWithStakebaseBig := big.NewInt(amountWithStakebase)
	totalContribBig := big.NewInt(totalContrib)

	for idx, amount := range amounts {
		amountBig := big.NewInt(amount) // We need > 64 bits

		// mul amountWithStakebase
		amountBig.Mul(amountBig, amountWithStakebaseBig)

		// mul 2^32
		amountBig.Lsh(amountBig, 32)

		// div totalContrib
		amountBig.Div(amountBig, totalContribBig)

		// div 2^32
		amountBig.Rsh(amountBig, 32)

		// make int64
		amountFinal := int64(amountBig.Uint64())

		outputsAmounts[idx] = amountFinal
	}

	return outputsAmounts
}

// VerifySStxAmounts compares a list of calculated amounts for ticket commitments
// to the list of commitment amounts from the actual SStx.
func VerifySStxAmounts(sstxAmts []int64, sstxCalcAmts []int64) error {
	if len(sstxCalcAmts) != len(sstxAmts) {
		errStr := fmt.Sprintf("SStx verify error: number of calculated " +
			"sstx output values was not equivalent to the number of sstx " +
			"commitment outputs")
		return stakeRuleError(ErrVerSStxAmts, errStr)
	}

	for idx, amt := range sstxCalcAmts {
		if !(amt == sstxAmts[idx]) {
			errStr := fmt.Sprintf("SStx verify error: at index %v incongruent "+
				"amt %v in SStx calculated reward and amt %v in "+
				"SStx", idx, amt, sstxAmts[idx])
			return stakeRuleError(ErrVerSStxAmts, errStr)
		}
	}

	return nil
}

// VerifyStakingPkhsAndAmounts takes the following:
// 1. sstxTypes: A list of types for what the output should be (P2PK or P2SH).
// 2. sstxPkhs: A list of payee PKHs from NullDataTy outputs of an input SStx.
// 3. ssSpendAmts: What the payouts in an SSGen/SSRtx tx actually were.
// 4. ssSpendTypes: A list of types for what the outputs actually were.
// 5. ssSpendPkhs: A list of payee PKHs from OP_SSGEN tagged outputs of the SSGen
//     or SSRtx.
// 6. ssSpendCalcAmts: A list of payee amounts that was calculated based on
//     the input SStx. These are the maximum possible amounts that can be
//     transacted from this output.
// 7. isVote: Whether this is a vote (true) or revocation (false).
// 8. spendRules: Spending rules for each output in terms of fees allowable
//     as extracted from the origin output Version.
// 9. spendLimits: Spending limits for each output in terms of fees allowable
//     as extracted from the origin output Version.
//
// and determines if the two pairs of slices are congruent or not.
func VerifyStakingPkhsAndAmounts(
	sstxTypes []bool,
	sstxPkhs [][]byte,
	ssSpendAmts []int64,
	ssSpendTypes []bool,
	ssSpendPkhs [][]byte,
	ssSpendCalcAmts []int64,
	isVote bool,
	spendRules [][]bool,
	spendLimits [][]uint16) error {

	if len(sstxTypes) != len(ssSpendTypes) {
		errStr := fmt.Sprintf("Staking verify error: number of " +
			"sstx type values was not equivalent to the number " +
			"of ss*** type values")
		return stakeRuleError(ErrVerifyInput, errStr)
	}

	if len(ssSpendAmts) != len(ssSpendCalcAmts) {
		errStr := fmt.Sprintf("Staking verify error: number of " +
			"sstx output values was not equivalent to the number " +
			"of ss*** output values")
		return stakeRuleError(ErrVerifyInput, errStr)
	}

	if len(sstxPkhs) != len(ssSpendPkhs) {
		errStr := fmt.Sprintf("Staking verify error: number of " +
			"sstx output pks was not equivalent to the number " +
			"of ss*** output pkhs")
		return stakeRuleError(ErrVerifyInput, errStr)
	}

	for idx, typ := range sstxTypes {
		if typ != ssSpendTypes[idx] {
			errStr := fmt.Sprintf("SStx in/SS*** out verify error: at index %v "+
				"non-equivalent type %v in SStx and type %v in SS***", idx, typ,
				ssSpendTypes[idx])
			return stakeRuleError(ErrVerifyOutType, errStr)
		}
	}

	for idx, amt := range ssSpendAmts {
		rule := false
		limit := uint16(0)
		if isVote {
			// Vote.
			rule = spendRules[idx][0]
			limit = spendLimits[idx][0]
		} else {
			// Revocation.
			rule = spendRules[idx][1]
			limit = spendLimits[idx][1]
		}

		// Apply the spending rules and see if the transaction is within
		// the specified limits if it asks us to.
		if rule {
			// If 63 is given, the entire amount may be used as a fee.
			// Obviously we can't allow shifting 1 63 places because
			// we'd get a negative number.
			feeAllowance := ssSpendCalcAmts[idx]
			if limit < rangeLimitMax {
				if int64(1<<uint64(limit)) < ssSpendCalcAmts[idx] {
					feeAllowance = int64(1 << uint64(limit))
				}
			}

			amtLimitLow := ssSpendCalcAmts[idx] - feeAllowance
			amtLimitHigh := ssSpendCalcAmts[idx]

			// Our new output is not allowed to be below this minimum
			// amount.
			if amt < amtLimitLow {
				errStr := fmt.Sprintf("SStx in/SS*** out verify error: "+
					"at index %v amt min limit was calculated to be %v yet "+
					"the actual amount output was %v", idx, amtLimitLow,
					amt)
				return stakeRuleError(ErrVerifyTooMuchFees, errStr)
			}
			// It also isn't allowed to spend more than the maximum
			// amount.
			if amt > amtLimitHigh {
				errStr := fmt.Sprintf("at index %v amt max limit was "+
					"calculated to be %v yet the actual amount output "+
					"was %v", idx, amtLimitHigh, amt)
				return stakeRuleError(ErrVerifySpendTooMuch, errStr)
			}
		} else {
			// Fees are disabled.
			if amt != ssSpendCalcAmts[idx] {
				errStr := fmt.Sprintf("SStx in/SS*** out verify error: "+
					"at index %v non-equivalent amt %v in SStx and amt "+
					"%v in SS***", idx, amt, ssSpendCalcAmts[idx])
				return stakeRuleError(ErrVerifyOutputAmt, errStr)
			}
		}
	}

	for idx, pkh := range sstxPkhs {
		if !bytes.Equal(pkh, ssSpendPkhs[idx]) {
			errStr := fmt.Sprintf("SStx in/SS*** out verify error: at index %v "+
				"non-equivalent pkh %x in SStx and pkh %x in SS***", idx, pkh,
				ssSpendPkhs[idx])
			return stakeRuleError(ErrVerifyOutPkhs, errStr)
		}
	}

	return nil
}

// --------------------------------------------------------------------------------
// Stake Transaction Identification Functions
// --------------------------------------------------------------------------------

// IsSStx returns whether or not a transaction is an SStx. It does some
// simple validation steps to make sure the number of inputs, number of
// outputs, and the input/output scripts are valid.
//
// SStx transactions are specified as below.
// Inputs:
// untagged output 1 [index 0]
// untagged output 2 [index 1]
// ...
// untagged output MaxInputsPerSStx [index MaxInputsPerSStx-1]
//
// Outputs:
// OP_SSTX tagged output [index 0]
// OP_RETURN push of input 1's address for reward receiving [index 1]
// OP_SSTXCHANGE tagged output for input 1 [index 2]
// OP_RETURN push of input 2's address for reward receiving [index 3]
// OP_SSTXCHANGE tagged output for input 2 [index 4]
// ...
// OP_RETURN push of input MaxInputsPerSStx's address for reward receiving
//     [index (MaxInputsPerSStx*2)-2]
// OP_SSTXCHANGE tagged output [index (MaxInputsPerSStx*2)-1]
//
// The output OP_RETURN pushes should be of size 20 bytes (standard address).
//
// The errors in this function can be ignored if you want to use it in to
// identify SStx from a list of stake tx.
func IsSStx(tx *dcrutil.Tx) (bool, error) {
	msgTx := tx.MsgTx()

	// Check to make sure there aren't too many inputs.
	// CheckTransactionSanity already makes sure that number of inputs is
	// greater than 0, so no need to check that.
	if len(msgTx.TxIn) > MaxInputsPerSStx {
		return false, stakeRuleError(ErrSStxTooManyInputs, "SStx has too many "+
			"inputs")
	}

	// Check to make sure there aren't too many outputs.
	if len(msgTx.TxOut) > MaxOutputsPerSStx {
		return false, stakeRuleError(ErrSStxTooManyOutputs, "SStx has too many "+
			"outputs")
	}

	// Check to make sure there are some outputs.
	if len(msgTx.TxOut) == 0 {
		return false, stakeRuleError(ErrSStxNoOutputs, "SStx has no "+
			"outputs")
	}

	// Check to make sure that all output scripts are the default version.
	for idx, txOut := range msgTx.TxOut {
		if txOut.Version != txscript.DefaultScriptVersion {
			errStr := fmt.Sprintf("invalid script version found in "+
				"txOut idx %v", idx)
			return false, stakeRuleError(ErrSStxInvalidOutputs, errStr)
		}
	}

	// Ensure that the first output is tagged OP_SSTX.
	if txscript.GetScriptClass(msgTx.TxOut[0].Version, msgTx.TxOut[0].PkScript) !=
		txscript.StakeSubmissionTy {
		return false, stakeRuleError(ErrSStxInvalidOutputs, "First SStx output "+
			"should have been OP_SSTX tagged, but it was not")
	}

	// Ensure that the number of outputs is equal to the number of inputs
	// + 1.
	if (len(msgTx.TxIn)*2 + 1) != len(msgTx.TxOut) {
		return false, stakeRuleError(ErrSStxInOutProportions, "The number of "+
			"inputs in the SStx tx was not the number of outputs/2 - 1")
	}

	// Ensure that the rest of the odd outputs are 28-byte OP_RETURN pushes that
	// contain putative pubkeyhashes, and that the rest of the odd outputs are
	// OP_SSTXCHANGE tagged.
	for outTxIndex := 1; outTxIndex < len(msgTx.TxOut); outTxIndex++ {
		scrVersion := msgTx.TxOut[outTxIndex].Version
		rawScript := msgTx.TxOut[outTxIndex].PkScript

		// Check change outputs.
		if outTxIndex%2 == 0 {
			if txscript.GetScriptClass(scrVersion, rawScript) !=
				txscript.StakeSubChangeTy {
				str := fmt.Sprintf("SStx output at output index %d was not "+
					"an sstx change output", outTxIndex)
				return false, stakeRuleError(ErrSStxInvalidOutputs, str)
			}
			continue
		}

		// Else (odd) check commitment outputs.  The script should be a
		// NullDataTy output.
		if txscript.GetScriptClass(scrVersion, rawScript) !=
			txscript.NullDataTy {
			str := fmt.Sprintf("SStx output at output index %d was not "+
				"a NullData (OP_RETURN) push", outTxIndex)
			return false, stakeRuleError(ErrSStxInvalidOutputs, str)
		}

		// The length of the output script should be between 32 and 77 bytes long.
		if len(rawScript) < SStxPKHMinOutSize ||
			len(rawScript) > SStxPKHMaxOutSize {
			str := fmt.Sprintf("SStx output at output index %d was a "+
				"NullData (OP_RETURN) push of the wrong size", outTxIndex)
			return false, stakeRuleError(ErrSStxInvalidOutputs, str)
		}

		// The OP_RETURN output script prefix should conform to the standard.
		outputScriptBuffer := bytes.NewBuffer(rawScript)
		outputScriptPrefix := outputScriptBuffer.Next(2)

		minPush := uint8(validSStxAddressOutMinPrefix[1])
		maxPush := uint8(validSStxAddressOutMinPrefix[1]) +
			(MaxSingleBytePushLength - minPush)
		pushLen := uint8(outputScriptPrefix[1])
		pushLengthValid := (pushLen >= minPush) && (pushLen <= maxPush)
		// The first byte should be OP_RETURN, while the second byte should be a
		// valid push length.
		if !(outputScriptPrefix[0] == validSStxAddressOutMinPrefix[0]) ||
			!pushLengthValid {
			errStr := fmt.Sprintf("sstx commitment at output idx %v had "+
				"an invalid prefix", outTxIndex)
			return false, stakeRuleError(ErrSStxInvalidOutputs,
				errStr)
		}
	}

	return true, nil
}

// IsSSGen returns whether or not a transaction is an SSGen tx. It does some
// simple validation steps to make sure the number of inputs, number of
// outputs, and the input/output scripts are valid.
//
// This does NOT check to see if the subsidy is valid or whether or not the
// value of input[0] + subsidy = value of the outputs.
//
// SSGen transactions are specified as below.
// Inputs:
// Stakebase null input [index 0]
// SStx-tagged output [index 1]
//
// Outputs:
// OP_RETURN push of 40 bytes containing: [index 0]
//     i. 32-byte block header of block being voted on.
//     ii. 8-byte int of this block's height.
// OP_RETURN push of 2 bytes containing votebits [index 1]
// SSGen-tagged output to address from SStx-tagged output's tx index output 1
//     [index 2]
// SSGen-tagged output to address from SStx-tagged output's tx index output 2
//     [index 3]
// ...
// SSGen-tagged output to address from SStx-tagged output's tx index output
//     MaxInputsPerSStx [index MaxOutputsPerSSgen - 1]
//
// The errors in this function can be ignored if you want to use it in to
// identify SSGen from a list of stake tx.
func IsSSGen(tx *dcrutil.Tx) (bool, error) {
	msgTx := tx.MsgTx()

	// Check to make sure there aren't too many inputs.
	// CheckTransactionSanity already makes sure that number of inputs is
	// greater than 0, so no need to check that.
	if len(msgTx.TxIn) != NumInputsPerSSGen {
		return false, stakeRuleError(ErrSSGenWrongNumInputs, "SSgen tx has an "+
			"invalid number of inputs")
	}

	// Check to make sure there aren't too many outputs.
	if len(msgTx.TxOut) > MaxOutputsPerSSGen {
		return false, stakeRuleError(ErrSSGenTooManyOutputs, "SSgen tx has too "+
			"many outputs")
	}

	// Check to make sure there are some outputs.
	if len(msgTx.TxOut) == 0 {
		return false, stakeRuleError(ErrSSGenNoOutputs, "SSgen tx no "+
			"many outputs")
	}

	// Ensure that the first input is a stake base null input.
	// Also checks to make sure that there aren't too many or too few inputs.
	if !IsStakeBase(tx) {
		return false, stakeRuleError(ErrSSGenNoStakebase, "SSGen tx did not "+
			"include a stakebase in the zeroeth input position")
	}

	// Check to make sure that the output used as input came from TxTreeStake.
	for i, txin := range msgTx.TxIn {
		// Skip the stakebase
		if i == 0 {
			continue
		}

		if txin.PreviousOutPoint.Index != 0 {
			errStr := fmt.Sprintf("SSGen used an invalid input idx (got %v, "+
				"want 0)", txin.PreviousOutPoint.Index)
			return false, stakeRuleError(ErrSSGenWrongIndex, errStr)
		}

		if txin.PreviousOutPoint.Tree != dcrutil.TxTreeStake {
			return false, stakeRuleError(ErrSSGenWrongTxTree, "SSGen used "+
				"a non-stake input")
		}
	}

	// Check to make sure that all output scripts are the default version.
	for _, txOut := range msgTx.TxOut {
		if txOut.Version != txscript.DefaultScriptVersion {
			return false, stakeRuleError(ErrSSGenBadGenOuts, "invalid "+
				"script version found in txOut")
		}
	}

	// Ensure the number of outputs is equal to the number of inputs found in
	// the original SStx + 2.
	// TODO: Do this in validate, requires DB and valid chain.

	// Ensure that the second input is an SStx tagged output.
	// TODO: Do this in validate, as we don't want to actually lookup
	// old tx here. This function is for more general sorting.

	// Ensure that the first output is an OP_RETURN push.
	zeroethOutputVersion := msgTx.TxOut[0].Version
	zeroethOutputScript := msgTx.TxOut[0].PkScript
	if txscript.GetScriptClass(zeroethOutputVersion, zeroethOutputScript) !=
		txscript.NullDataTy {
		return false, stakeRuleError(ErrSSGenNoReference, "First SSGen output "+
			"should have been an OP_RETURN data push, but was not")
	}

	// Ensure that the first output is the correct size.
	if len(zeroethOutputScript) != SSGenBlockReferenceOutSize {
		return false, stakeRuleError(ErrSSGenBadReference, "First SSGen output "+
			"should have been 43 bytes long, but was not")
	}

	// The OP_RETURN output script prefix for block referencing should
	// conform to the standard.
	zeroethOutputScriptBuffer := bytes.NewBuffer(zeroethOutputScript)

	zeroethOutputScriptPrefix := zeroethOutputScriptBuffer.Next(2)
	if !bytes.Equal(zeroethOutputScriptPrefix,
		validSSGenReferenceOutPrefix) {
		return false, stakeRuleError(ErrSSGenBadReference, "First SSGen output "+
			"had an invalid prefix")
	}

	// Ensure that the block header hash given in the first 32 bytes of the
	// OP_RETURN push is a valid block header and found in the main chain.
	// TODO: This is validate level stuff, do this there.

	// Ensure that the second output is an OP_RETURN push.
	firstOutputVersion := msgTx.TxOut[1].Version
	firstOutputScript := msgTx.TxOut[1].PkScript
	if txscript.GetScriptClass(firstOutputVersion, firstOutputScript) !=
		txscript.NullDataTy {
		return false, stakeRuleError(ErrSSGenNoVotePush, "Second SSGen output "+
			"should have been an OP_RETURN data push, but was not")
	}

	// The length of the output script should be between 4 and 77 bytes long.
	if len(firstOutputScript) < SSGenVoteBitsOutputMinSize ||
		len(firstOutputScript) > SSGenVoteBitsOutputMaxSize {
		str := fmt.Sprintf("SSGen votebits output at output index 1 was a " +
			"NullData (OP_RETURN) push of the wrong size")
		return false, stakeRuleError(ErrSSGenBadVotePush, str)
	}

	// The OP_RETURN output script prefix for voting should conform to the
	// standard.
	firstOutputScriptBuffer := bytes.NewBuffer(firstOutputScript)
	firstOutputScriptPrefix := firstOutputScriptBuffer.Next(2)

	minPush := uint8(validSSGenVoteOutMinPrefix[1])
	maxPush := uint8(validSSGenVoteOutMinPrefix[1]) +
		(MaxSingleBytePushLength - minPush)
	pushLen := uint8(firstOutputScriptPrefix[1])
	pushLengthValid := (pushLen >= minPush) && (pushLen <= maxPush)
	// The first byte should be OP_RETURN, while the second byte should be a
	// valid push length.
	if !(firstOutputScriptPrefix[0] == validSSGenVoteOutMinPrefix[0]) ||
		!pushLengthValid {
		return false, stakeRuleError(ErrSSGenBadVotePush, "Second SSGen output "+
			"had an invalid prefix")
	}

	// Ensure that the tx height given in the last 8 bytes is StakeMaturity
	// many blocks ahead of the block in which that SStx appears, otherwise
	// this ticket has failed to mature and the SStx must be invalid.
	// TODO: This is validate level stuff, do this there.

	// Ensure that the remaining outputs are OP_SSGEN tagged.
	for outTxIndex := 2; outTxIndex < len(msgTx.TxOut); outTxIndex++ {
		scrVersion := msgTx.TxOut[outTxIndex].Version
		rawScript := msgTx.TxOut[outTxIndex].PkScript

		// The script should be a OP_SSGEN tagged output.
		if txscript.GetScriptClass(scrVersion, rawScript) !=
			txscript.StakeGenTy {
			str := fmt.Sprintf("SSGen tx output at output index %d was not "+
				"an OP_SSGEN tagged output", outTxIndex)
			return false, stakeRuleError(ErrSSGenBadGenOuts, str)
		}
	}

	return true, nil
}

// IsSSRtx returns whether or not a transaction is an SSRtx. It does some
// simple validation steps to make sure the number of inputs, number of
// outputs, and the input/output scripts are valid.
//
// SSRtx transactions are specified as below.
// Inputs:
// SStx-tagged output [index 0]
//
// Outputs:
// SSGen-tagged output to address from SStx-tagged output's tx index output 1
//     [index 0]
// SSGen-tagged output to address from SStx-tagged output's tx index output 2
//     [index 1]
// ...
// SSGen-tagged output to address from SStx-tagged output's tx index output
//     MaxInputsPerSStx [index MaxOutputsPerSSRtx - 1]
//
// The errors in this function can be ignored if you want to use it in to
// identify SSRtx from a list of stake tx.
func IsSSRtx(tx *dcrutil.Tx) (bool, error) {
	msgTx := tx.MsgTx()

	// Check to make sure there is the correct number of inputs.
	// CheckTransactionSanity already makes sure that number of inputs is
	// greater than 0, so no need to check that.
	if len(msgTx.TxIn) != NumInputsPerSSRtx {
		return false, stakeRuleError(ErrSSRtxWrongNumInputs, "SSRtx has an "+
			" invalid number of inputs")
	}

	// Check to make sure there aren't too many outputs.
	if len(msgTx.TxOut) > MaxOutputsPerSSRtx {
		return false, stakeRuleError(ErrSSRtxTooManyOutputs, "SSRtx has too "+
			"many outputs")
	}

	// Check to make sure there are some outputs.
	if len(msgTx.TxOut) == 0 {
		return false, stakeRuleError(ErrSSRtxNoOutputs, "SSRtx has no "+
			"outputs")
	}

	// Check to make sure that all output scripts are the default version.
	for _, txOut := range msgTx.TxOut {
		if txOut.Version != txscript.DefaultScriptVersion {
			return false, stakeRuleError(ErrSSRtxBadOuts, "invalid "+
				"script version found in txOut")
		}
	}

	// Check to make sure that the output used as input came from TxTreeStake.
	for _, txin := range msgTx.TxIn {
		if txin.PreviousOutPoint.Tree != dcrutil.TxTreeStake {
			return false, stakeRuleError(ErrSSRtxWrongTxTree, "SSRtx used "+
				"a non-stake input")
		}
	}

	// Ensure that the first input is an SStx tagged output.
	// TODO: Do this in validate, needs a DB and chain.

	// Ensure that the tx height given in the last 8 bytes is StakeMaturity
	// many blocks ahead of the block in which that SStx appear, otherwise
	// this ticket has failed to mature and the SStx must be invalid.
	// TODO: Do this in validate, needs a DB and chain.

	// Ensure that the outputs are OP_SSRTX tagged.
	// Ensure that the tx height given in the last 8 bytes is StakeMaturity
	// many blocks ahead of the block in which that SStx appear, otherwise
	// this ticket has failed to mature and the SStx must be invalid.
	// TODO: This is validate level stuff, do this there.

	// Ensure that the outputs are OP_SSRTX tagged.
	for outTxIndex := 0; outTxIndex < len(msgTx.TxOut); outTxIndex++ {
		scrVersion := msgTx.TxOut[outTxIndex].Version
		rawScript := msgTx.TxOut[outTxIndex].PkScript

		// The script should be a OP_SSRTX tagged output.
		if txscript.GetScriptClass(scrVersion, rawScript) !=
			txscript.StakeRevocationTy {
			str := fmt.Sprintf("SSRtx output at output index %d was not "+
				"an OP_SSRTX tagged output", outTxIndex)
			return false, stakeRuleError(ErrSSRtxBadOuts, str)
		}
	}

	// Ensure the number of outputs is equal to the number of inputs found in
	// the original SStx.
	// TODO: Do this in validate, needs a DB and chain.

	return true, nil
}

// DetermineTxType determines the type of stake transaction a transaction is; if
// none, it returns that it is an assumed regular tx.
func DetermineTxType(tx *dcrutil.Tx) TxType {
	if is, _ := IsSStx(tx); is {
		return TxTypeSStx
	}
	if is, _ := IsSSGen(tx); is {
		return TxTypeSSGen
	}
	if is, _ := IsSSRtx(tx); is {
		return TxTypeSSRtx
	}
	return TxTypeRegular
}

// SetTxTree analyzes the embedded MsgTx and sets the transaction tree
// accordingly.
func SetTxTree(tx *dcrutil.Tx) {
	txType := DetermineTxType(tx)

	indicatedTree := dcrutil.TxTreeRegular
	if txType != TxTypeRegular {
		indicatedTree = dcrutil.TxTreeStake
	}

	tx.SetTree(indicatedTree)
}
