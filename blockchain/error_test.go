// Copyright (c) 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain_test

import (
	"testing"

	"github.com/btcsuite/btcd/blockchain"
)

// TestErrorCodeStringer tests the stringized output for the ErrorCode type.
func TestErrorCodeStringer(t *testing.T) {
	tests := []struct {
		in   blockchain.ErrorCode
		want string
	}{
		{blockchain.ErrDuplicateBlock, "ErrDuplicateBlock"},
		{blockchain.ErrBlockTooBig, "ErrBlockTooBig"},
		{blockchain.ErrBlockVersionTooOld, "ErrBlockVersionTooOld"},
		{blockchain.ErrInvalidTime, "ErrInvalidTime"},
		{blockchain.ErrTimeTooOld, "ErrTimeTooOld"},
		{blockchain.ErrTimeTooNew, "ErrTimeTooNew"},
		{blockchain.ErrDifficultyTooLow, "ErrDifficultyTooLow"},
		{blockchain.ErrUnexpectedDifficulty, "ErrUnexpectedDifficulty"},
		{blockchain.ErrHighHash, "ErrHighHash"},
		{blockchain.ErrBadMerkleRoot, "ErrBadMerkleRoot"},
		{blockchain.ErrBadCheckpoint, "ErrBadCheckpoint"},
		{blockchain.ErrForkTooOld, "ErrForkTooOld"},
		{blockchain.ErrCheckpointTimeTooOld, "ErrCheckpointTimeTooOld"},
		{blockchain.ErrNoTransactions, "ErrNoTransactions"},
		{blockchain.ErrTooManyTransactions, "ErrTooManyTransactions"},
		{blockchain.ErrNoTxInputs, "ErrNoTxInputs"},
		{blockchain.ErrNoTxOutputs, "ErrNoTxOutputs"},
		{blockchain.ErrTxTooBig, "ErrTxTooBig"},
		{blockchain.ErrBadTxOutValue, "ErrBadTxOutValue"},
		{blockchain.ErrDuplicateTxInputs, "ErrDuplicateTxInputs"},
		{blockchain.ErrBadTxInput, "ErrBadTxInput"},
		{blockchain.ErrBadCheckpoint, "ErrBadCheckpoint"},
		{blockchain.ErrMissingTx, "ErrMissingTx"},
		{blockchain.ErrUnfinalizedTx, "ErrUnfinalizedTx"},
		{blockchain.ErrDuplicateTx, "ErrDuplicateTx"},
		{blockchain.ErrOverwriteTx, "ErrOverwriteTx"},
		{blockchain.ErrImmatureSpend, "ErrImmatureSpend"},
		{blockchain.ErrDoubleSpend, "ErrDoubleSpend"},
		{blockchain.ErrSpendTooHigh, "ErrSpendTooHigh"},
		{blockchain.ErrBadFees, "ErrBadFees"},
		{blockchain.ErrTooManySigOps, "ErrTooManySigOps"},
		{blockchain.ErrFirstTxNotCoinbase, "ErrFirstTxNotCoinbase"},
		{blockchain.ErrMultipleCoinbases, "ErrMultipleCoinbases"},
		{blockchain.ErrBadCoinbaseScriptLen, "ErrBadCoinbaseScriptLen"},
		{blockchain.ErrBadCoinbaseValue, "ErrBadCoinbaseValue"},
		{blockchain.ErrMissingCoinbaseHeight, "ErrMissingCoinbaseHeight"},
		{blockchain.ErrBadCoinbaseHeight, "ErrBadCoinbaseHeight"},
		{blockchain.ErrScriptMalformed, "ErrScriptMalformed"},
		{blockchain.ErrScriptValidation, "ErrScriptValidation"},
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
		in   blockchain.RuleError
		want string
	}{
		{
			blockchain.RuleError{Description: "duplicate block"},
			"duplicate block",
		},
		{
			blockchain.RuleError{Description: "human-readable error"},
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
