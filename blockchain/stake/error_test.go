// Copyright (c) 2014 Conformal Systems LLC.
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake_test

import (
	"testing"

	"github.com/decred/dcrd/blockchain/stake"
)

// TestErrorCodeStringer tests the stringized output for the ErrorCode type.
func TestErrorCodeStringer(t *testing.T) {
	tests := []struct {
		in   stake.ErrorCode
		want string
	}{
		{stake.ErrSStxTooManyInputs, "ErrSStxTooManyInputs"},
		{stake.ErrSStxTooManyOutputs, "ErrSStxTooManyOutputs"},
		{stake.ErrSStxNoOutputs, "ErrSStxNoOutputs"},
		{stake.ErrSStxInvalidInputs, "ErrSStxInvalidInputs"},
		{stake.ErrSStxInvalidOutputs, "ErrSStxInvalidOutputs"},
		{stake.ErrSStxInOutProportions, "ErrSStxInOutProportions"},
		{stake.ErrSStxBadCommitAmount, "ErrSStxBadCommitAmount"},
		{stake.ErrSStxBadChangeAmts, "ErrSStxBadChangeAmts"},
		{stake.ErrSStxVerifyCalcAmts, "ErrSStxVerifyCalcAmts"},
		{stake.ErrSSGenWrongNumInputs, "ErrSSGenWrongNumInputs"},
		{stake.ErrSSGenTooManyOutputs, "ErrSSGenTooManyOutputs"},
		{stake.ErrSSGenNoOutputs, "ErrSSGenNoOutputs"},
		{stake.ErrSSGenWrongIndex, "ErrSSGenWrongIndex"},
		{stake.ErrSSGenWrongTxTree, "ErrSSGenWrongTxTree"},
		{stake.ErrSSGenNoStakebase, "ErrSSGenNoStakebase"},
		{stake.ErrSSGenNoReference, "ErrSSGenNoReference"},
		{stake.ErrSSGenBadReference, "ErrSSGenBadReference"},
		{stake.ErrSSGenNoVotePush, "ErrSSGenNoVotePush"},
		{stake.ErrSSGenBadVotePush, "ErrSSGenBadVotePush"},
		{stake.ErrSSGenBadGenOuts, "ErrSSGenBadGenOuts"},
		{stake.ErrSSRtxWrongNumInputs, "ErrSSRtxWrongNumInputs"},
		{stake.ErrSSRtxTooManyOutputs, "ErrSSRtxTooManyOutputs"},
		{stake.ErrSSRtxNoOutputs, "ErrSSRtxNoOutputs"},
		{stake.ErrSSRtxWrongTxTree, "ErrSSRtxWrongTxTree"},
		{stake.ErrSSRtxBadOuts, "ErrSSRtxBadOuts"},
		{stake.ErrVerSStxAmts, "ErrVerSStxAmts"},
		{stake.ErrVerifyInput, "ErrVerifyInput"},
		{stake.ErrVerifyOutType, "ErrVerifyOutType"},
		{stake.ErrVerifyTooMuchFees, "ErrVerifyTooMuchFees"},
		{stake.ErrVerifySpendTooMuch, "ErrVerifySpendTooMuch"},
		{stake.ErrVerifyOutputAmt, "ErrVerifyOutputAmt"},
		{stake.ErrVerifyOutPkhs, "ErrVerifyOutPkhs"},
		{stake.ErrDatabaseCorrupt, "ErrDatabaseCorrupt"},
		{stake.ErrMissingDatabaseTx, "ErrMissingDatabaseTx"},
		{stake.ErrMemoryCorruption, "ErrMemoryCorruption"},
		{stake.ErrFindTicketIdxs, "ErrFindTicketIdxs"},
		{stake.ErrMissingTicket, "ErrMissingTicket"},
		{stake.ErrDuplicateTicket, "ErrDuplicateTicket"},
		{stake.ErrUnknownTicketSpent, "ErrUnknownTicketSpent"},
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
		in   stake.RuleError
		want string
	}{
		{stake.RuleError{Description: "duplicate block"},
			"duplicate block",
		},
		{stake.RuleError{Description: "human-readable error"},
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
