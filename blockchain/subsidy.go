// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"bytes"
	"fmt"

	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

// CalcBlockSubsidy returns the subsidy amount a block at the provided height
// should have. This is mainly used for determining how much the coinbase for
// newly generated blocks awards as well as validating the coinbase for blocks
// has the expected value.
//
// Subsidy calculation for exponential reductions:
// 0 for i in range (0, height / ReductionInterval):
// 1     subsidy *= MulSubsidy
// 2     subsidy /= DivSubsidy
//
// Safe for concurrent access.
func calcBlockSubsidy(height int64, params *chaincfg.Params) int64 {
	// Block height 1 subsidy is 'special' and used to
	// distribute initial tokens, if any.
	if height == 1 {
		return params.BlockOneSubsidy()
	}

	iterations := height / params.ReductionInterval
	subsidy := params.BaseSubsidy

	// You could stick all these values in a LUT for faster access if you
	// wanted to, but this calculation is already really fast until you
	// get very very far into the blockchain. The other method you could
	// use is storing the total subsidy in a block node and do the
	// multiplication and division when needed when adding a block.
	if iterations > 0 {
		for i := int64(0); i < iterations; i++ {
			subsidy *= params.MulSubsidy
			subsidy /= params.DivSubsidy
		}
	}

	return subsidy
}

// CalcBlockWorkSubsidy calculates the proof of work subsidy for a block as a
// proportion of the total subsidy.
func CalcBlockWorkSubsidy(height int64, voters uint16,
	params *chaincfg.Params) int64 {
	subsidy := calcBlockSubsidy(height, params)
	proportionWork := int64(params.WorkRewardProportion)
	proportions := int64(params.TotalSubsidyProportions())
	subsidy *= proportionWork
	subsidy /= proportions

	// Ignore the voters field of the header before we're at a point
	// where there are any voters.
	if height < params.StakeValidationHeight {
		return subsidy
	}

	// If there are no voters, subsidy is 0. The block will fail later anyway.
	if voters == 0 {
		return 0
	}

	// Adjust for the number of voters. This shouldn't ever overflow if you start
	// with 50 * 10^8 Atoms and voters and potentialVoters are uint16.
	potentialVoters := params.TicketsPerBlock
	actual := (int64(voters) * subsidy) / int64(potentialVoters)

	return actual
}

// CalcStakeVoteSubsidy calculates the subsidy for a stake vote based on the height
// of its input SStx.
//
// Safe for concurrent access.
func CalcStakeVoteSubsidy(height int64, params *chaincfg.Params) int64 {
	// Calculate the actual reward for this block, then further reduce reward
	// proportional to StakeRewardProportion.
	// Note that voters/potential voters is 1, so that vote reward is calculated
	// irrespective of block reward.
	subsidy := calcBlockSubsidy(height, params)
	proportionStake := int64(params.StakeRewardProportion)
	proportions := int64(params.TotalSubsidyProportions())
	subsidy *= proportionStake
	subsidy /= (proportions * int64(params.TicketsPerBlock))

	return subsidy
}

// CalcBlockTaxSubsidy calculates the subsidy for the organization address in the
// coinbase.
//
// Safe for concurrent access.
func CalcBlockTaxSubsidy(height int64, voters uint16,
	params *chaincfg.Params) int64 {
	if params.BlockTaxProportion == 0 {
		return 0
	}

	subsidy := calcBlockSubsidy(int64(height), params)
	proportionTax := int64(params.BlockTaxProportion)
	proportions := int64(params.TotalSubsidyProportions())
	subsidy *= proportionTax
	subsidy /= proportions

	// Assume all voters 'present' before stake voting is turned on.
	if height < params.StakeValidationHeight {
		voters = 5
	}

	// If there are no voters, subsidy is 0. The block will fail later anyway.
	if voters == 0 && height >= params.StakeValidationHeight {
		return 0
	}

	// Adjust for the number of voters. This shouldn't ever overflow if you start
	// with 50 * 10^8 Atoms and voters and potentialVoters are uint16.
	potentialVoters := params.TicketsPerBlock
	adjusted := (int64(voters) * subsidy) / int64(potentialVoters)

	return adjusted
}

// BlockOneCoinbasePaysTokens checks to see if the first block coinbase pays
// out to the network initial token ledger.
func BlockOneCoinbasePaysTokens(tx *dcrutil.Tx, params *chaincfg.Params) error {
	// If no ledger is specified, just return true.
	if len(params.BlockOneLedger) == 0 {
		return nil
	}

	if tx.MsgTx().LockTime != 0 {
		errStr := fmt.Sprintf("block 1 coinbase has invalid locktime")
		return ruleError(ErrBlockOneTx, errStr)
	}

	if tx.MsgTx().Expiry != wire.NoExpiryValue {
		errStr := fmt.Sprintf("block 1 coinbase has invalid expiry")
		return ruleError(ErrBlockOneTx, errStr)
	}

	if tx.MsgTx().TxIn[0].Sequence != wire.MaxTxInSequenceNum {
		errStr := fmt.Sprintf("block 1 coinbase not finalized")
		return ruleError(ErrBlockOneInputs, errStr)
	}

	if len(tx.MsgTx().TxOut) == 0 {
		errStr := fmt.Sprintf("coinbase outputs empty in block 1")
		return ruleError(ErrBlockOneOutputs, errStr)
	}

	ledger := params.BlockOneLedger
	if len(ledger) != len(tx.MsgTx().TxOut) {
		errStr := fmt.Sprintf("wrong number of outputs in block 1 coinbase; "+
			"got %v, expected %v", len(tx.MsgTx().TxOut), len(ledger))
		return ruleError(ErrBlockOneOutputs, errStr)
	}

	// Check the addresses and output amounts against those in the ledger.
	for i, txout := range tx.MsgTx().TxOut {
		if txout.Version != txscript.DefaultScriptVersion {
			errStr := fmt.Sprintf("bad block one output version; want %v, got %v",
				txscript.DefaultScriptVersion, txout.Version)
			return ruleError(ErrBlockOneOutputs, errStr)
		}

		// There should only be one address.
		_, addrs, _, err :=
			txscript.ExtractPkScriptAddrs(txout.Version, txout.PkScript, params)
		if len(addrs) != 1 {
			errStr := fmt.Sprintf("too many addresses in output")
			return ruleError(ErrBlockOneOutputs, errStr)
		}

		addrLedger, err := dcrutil.DecodeAddress(ledger[i].Address, params)
		if err != nil {
			return err
		}

		if !bytes.Equal(addrs[0].ScriptAddress(), addrLedger.ScriptAddress()) {
			errStr := fmt.Sprintf("address in output %v has non matching "+
				"address; got %v (hash160 %x), want %v (hash160 %x)",
				i,
				addrs[0].EncodeAddress(),
				addrs[0].ScriptAddress(),
				addrLedger.EncodeAddress(),
				addrLedger.ScriptAddress())
			return ruleError(ErrBlockOneOutputs, errStr)
		}

		if txout.Value != ledger[i].Amount {
			errStr := fmt.Sprintf("address in output %v has non matching "+
				"amount; got %v, want %v", i, txout.Value, ledger[i].Amount)
			return ruleError(ErrBlockOneOutputs, errStr)
		}
	}

	return nil
}

// CoinbasePaysTax checks to see if a given block's coinbase correctly pays
// tax to the developer organization.
func CoinbasePaysTax(tx *dcrutil.Tx, height uint32, voters uint16,
	params *chaincfg.Params) error {
	// Taxes only apply from block 2 onwards.
	if height <= 1 {
		return nil
	}

	// Tax is disabled.
	if params.BlockTaxProportion == 0 {
		return nil
	}

	if len(tx.MsgTx().TxOut) == 0 {
		errStr := fmt.Sprintf("invalid coinbase (no outputs)")
		return ruleError(ErrNoTxOutputs, errStr)
	}

	// Coinbase output 0 must be the subsidy to the dev organization.
	taxPkVersion := tx.MsgTx().TxOut[0].Version
	taxPkScript := tx.MsgTx().TxOut[0].PkScript
	class, addrs, _, err :=
		txscript.ExtractPkScriptAddrs(taxPkVersion, taxPkScript, params)
	// The script can't be a weird class.
	if !(class == txscript.ScriptHashTy ||
		class == txscript.PubKeyHashTy ||
		class == txscript.PubKeyTy) {
		errStr := fmt.Sprintf("wrong script class for tax output")
		return ruleError(ErrNoTax, errStr)
	}

	// There should only be one address.
	if len(addrs) != 1 {
		errStr := fmt.Sprintf("no or too many addresses in output")
		return ruleError(ErrNoTax, errStr)
	}

	// Decode the organization address.
	addrOrg, err := dcrutil.DecodeAddress(params.OrganizationAddress, params)
	if err != nil {
		return err
	}

	if !bytes.Equal(addrs[0].ScriptAddress(), addrOrg.ScriptAddress()) {
		errStr := fmt.Sprintf("address in output 0 has non matching org "+
			"address; got %v (hash160 %x), want %v (hash160 %x)",
			addrs[0].EncodeAddress(),
			addrs[0].ScriptAddress(),
			addrOrg.EncodeAddress(),
			addrOrg.ScriptAddress())
		return ruleError(ErrNoTax, errStr)
	}

	// Get the amount of subsidy that should have been paid out to
	// the organization, then check it.
	orgSubsidy := CalcBlockTaxSubsidy(int64(height), voters, params)
	amountFound := tx.MsgTx().TxOut[0].Value
	if orgSubsidy != amountFound {
		errStr := fmt.Sprintf("amount in output 0 has non matching org "+
			"calculated amount; got %v, want %v", amountFound, orgSubsidy)
		return ruleError(ErrNoTax, errStr)
	}

	return nil
}
