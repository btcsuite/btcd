// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcchain_test

import (
	"testing"

	"github.com/conformal/btcchain"
)

// TestErrorCodeStringer tests the stringized output for the ErrorCode type.
func TestErrorCodeStringer(t *testing.T) {
	tests := []struct {
		in   btcchain.ErrorCode
		want string
	}{
		{btcchain.ErrDuplicateBlock, "ErrDuplicateBlock"},
		{btcchain.ErrBlockTooBig, "ErrBlockTooBig"},
		{btcchain.ErrBlockVersionTooOld, "ErrBlockVersionTooOld"},
		{btcchain.ErrInvalidTime, "ErrInvalidTime"},
		{btcchain.ErrTimeTooOld, "ErrTimeTooOld"},
		{btcchain.ErrTimeTooNew, "ErrTimeTooNew"},
		{btcchain.ErrDifficultyTooLow, "ErrDifficultyTooLow"},
		{btcchain.ErrUnexpectedDifficulty, "ErrUnexpectedDifficulty"},
		{btcchain.ErrHighHash, "ErrHighHash"},
		{btcchain.ErrBadMerkleRoot, "ErrBadMerkleRoot"},
		{btcchain.ErrBadCheckpoint, "ErrBadCheckpoint"},
		{btcchain.ErrForkTooOld, "ErrForkTooOld"},
		{btcchain.ErrNoTransactions, "ErrNoTransactions"},
		{btcchain.ErrTooManyTransactions, "ErrTooManyTransactions"},
		{btcchain.ErrNoTxInputs, "ErrNoTxInputs"},
		{btcchain.ErrNoTxOutputs, "ErrNoTxOutputs"},
		{btcchain.ErrTxTooBig, "ErrTxTooBig"},
		{btcchain.ErrBadTxOutValue, "ErrBadTxOutValue"},
		{btcchain.ErrDuplicateTxInputs, "ErrDuplicateTxInputs"},
		{btcchain.ErrBadTxInput, "ErrBadTxInput"},
		{btcchain.ErrBadCheckpoint, "ErrBadCheckpoint"},
		{btcchain.ErrMissingTx, "ErrMissingTx"},
		{btcchain.ErrUnfinalizedTx, "ErrUnfinalizedTx"},
		{btcchain.ErrDuplicateTx, "ErrDuplicateTx"},
		{btcchain.ErrOverwriteTx, "ErrOverwriteTx"},
		{btcchain.ErrImmatureSpend, "ErrImmatureSpend"},
		{btcchain.ErrDoubleSpend, "ErrDoubleSpend"},
		{btcchain.ErrSpendTooHigh, "ErrSpendTooHigh"},
		{btcchain.ErrBadFees, "ErrBadFees"},
		{btcchain.ErrTooManySigOps, "ErrTooManySigOps"},
		{btcchain.ErrFirstTxNotCoinbase, "ErrFirstTxNotCoinbase"},
		{btcchain.ErrMultipleCoinbases, "ErrMultipleCoinbases"},
		{btcchain.ErrBadCoinbaseScriptLen, "ErrBadCoinbaseScriptLen"},
		{btcchain.ErrBadCoinbaseValue, "ErrBadCoinbaseValue"},
		{btcchain.ErrMissingCoinbaseHeight, "ErrMissingCoinbaseHeight"},
		{btcchain.ErrBadCoinbaseHeight, "ErrBadCoinbaseHeight"},
		{btcchain.ErrScriptMalformed, "ErrScriptMalformed"},
		{btcchain.ErrScriptValidation, "ErrScriptValidation"},
		{0xffff, "Unknown ErrorCode (65535)"},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.String()
		if result != test.want {
			t.Errorf("String #%d\n got: %s want: %s", i, result,
				test.want)
			continue
		}
	}
}

// TestRuleError tests the error output for the RuleError type.
func TestRuleError(t *testing.T) {
	tests := []struct {
		in   btcchain.RuleError
		want string
	}{
		{
			btcchain.RuleError{Description: "duplicate block"},
			"duplicate block",
		},
		{
			btcchain.RuleError{Description: "human-readable error"},
			"human-readable error",
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		result := test.in.Error()
		if result != test.want {
			t.Errorf("Error #%d\n got: %s want: %s", i, result,
				test.want)
			continue
		}
	}
}
