// Copyright (c) 2014-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"testing"
)

// TestErrorCodeStringer tests the stringized output for the ErrorCode type.
func TestErrorCodeStringer(t *testing.T) {
	tests := []struct {
		in   ErrorCode
		want string
	}{
		{ErrDuplicateBlock, "ErrDuplicateBlock"},
		{ErrBlockTooBig, "ErrBlockTooBig"},
		{ErrBlockWeightTooHigh, "ErrBlockWeightTooHigh"},
		{ErrBlockVersionTooOld, "ErrBlockVersionTooOld"},
		{ErrInvalidTime, "ErrInvalidTime"},
		{ErrTimeTooOld, "ErrTimeTooOld"},
		{ErrTimeTooNew, "ErrTimeTooNew"},
		{ErrDifficultyTooLow, "ErrDifficultyTooLow"},
		{ErrUnexpectedDifficulty, "ErrUnexpectedDifficulty"},
		{ErrHighHash, "ErrHighHash"},
		{ErrBadMerkleRoot, "ErrBadMerkleRoot"},
		{ErrBadCheckpoint, "ErrBadCheckpoint"},
		{ErrForkTooOld, "ErrForkTooOld"},
		{ErrCheckpointTimeTooOld, "ErrCheckpointTimeTooOld"},
		{ErrNoTransactions, "ErrNoTransactions"},
		{ErrNoTxInputs, "ErrNoTxInputs"},
		{ErrNoTxOutputs, "ErrNoTxOutputs"},
		{ErrTxTooBig, "ErrTxTooBig"},
		{ErrBadTxOutValue, "ErrBadTxOutValue"},
		{ErrDuplicateTxInputs, "ErrDuplicateTxInputs"},
		{ErrBadTxInput, "ErrBadTxInput"},
		{ErrBadCheckpoint, "ErrBadCheckpoint"},
		{ErrMissingTxOut, "ErrMissingTxOut"},
		{ErrUnfinalizedTx, "ErrUnfinalizedTx"},
		{ErrDuplicateTx, "ErrDuplicateTx"},
		{ErrOverwriteTx, "ErrOverwriteTx"},
		{ErrImmatureSpend, "ErrImmatureSpend"},
		{ErrSpendTooHigh, "ErrSpendTooHigh"},
		{ErrBadFees, "ErrBadFees"},
		{ErrTooManySigOps, "ErrTooManySigOps"},
		{ErrFirstTxNotCoinbase, "ErrFirstTxNotCoinbase"},
		{ErrMultipleCoinbases, "ErrMultipleCoinbases"},
		{ErrBadCoinbaseScriptLen, "ErrBadCoinbaseScriptLen"},
		{ErrBadCoinbaseValue, "ErrBadCoinbaseValue"},
		{ErrMissingCoinbaseHeight, "ErrMissingCoinbaseHeight"},
		{ErrBadCoinbaseHeight, "ErrBadCoinbaseHeight"},
		{ErrScriptMalformed, "ErrScriptMalformed"},
		{ErrScriptValidation, "ErrScriptValidation"},
		{ErrUnexpectedWitness, "ErrUnexpectedWitness"},
		{ErrInvalidWitnessCommitment, "ErrInvalidWitnessCommitment"},
		{ErrWitnessCommitmentMismatch, "ErrWitnessCommitmentMismatch"},
		{ErrPreviousBlockUnknown, "ErrPreviousBlockUnknown"},
		{ErrInvalidAncestorBlock, "ErrInvalidAncestorBlock"},
		{ErrPrevBlockNotBest, "ErrPrevBlockNotBest"},
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
		in   RuleError
		want string
	}{
		{
			RuleError{Description: "duplicate block"},
			"duplicate block",
		},
		{
			RuleError{Description: "human-readable error"},
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

// TestDeploymentError tests the stringized output for the DeploymentError type.
func TestDeploymentError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   DeploymentError
		want string
	}{
		{
			DeploymentError(0),
			"deployment ID 0 does not exist",
		},
		{
			DeploymentError(10),
			"deployment ID 10 does not exist",
		},
		{
			DeploymentError(123),
			"deployment ID 123 does not exist",
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
