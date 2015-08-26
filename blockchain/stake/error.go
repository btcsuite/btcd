// Copyright (c) 2014 Conformal Systems LLC.
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"fmt"
)

// ErrorCode identifies a kind of error.
type ErrorCode int

// These constants are used to identify a specific RuleError.
const (
	// ErrSStxTooManyInputs indicates that a given SStx contains too many
	// inputs.
	ErrSStxTooManyInputs = iota

	// ErrSStxTooManyOutputs indicates that a given SStx contains too many
	// outputs.
	ErrSStxTooManyOutputs

	// ErrSStxNoOutputs indicates that a given SStx has no outputs.
	ErrSStxNoOutputs

	// ErrSStxInvalidInput indicates that an invalid output has been used as
	// an input for a SStx; only non-SStx tagged outputs may be used to
	// purchase stake tickets.
	// TODO: Add this into validate
	// Ensure that all inputs are not tagged SStx outputs of some sort,
	// along with checks to make sure they exist and are available.
	ErrSStxInvalidInputs

	// ErrSStxInvalidOutput indicates that the output for an SStx tx is
	// invalid; in particular, either the output was not tagged SStx or the
	// OP_RETURNs were missing or contained invalid addresses.
	ErrSStxInvalidOutputs

	// ErrSStxInOutProportions indicates the the number of inputs in an SStx
	// was not equal to the number of output minus one.
	ErrSStxInOutProportions

	// ErrSStxBadCommitAmount indicates that a ticket tried to commit 0 or
	// a negative value as the commitment amount.
	ErrSStxBadCommitAmount

	// ErrSStxBadChangeAmts indicates that the change amount for some SStx
	// was invalid, for instance spending more than its inputs.
	ErrSStxBadChangeAmts

	// ErrSStxVerifyCalcAmts indicates that passed calculated amounts failed
	// to conform to the amounts found in the ticket.
	ErrSStxVerifyCalcAmts

	// ErrSSGenWrongNumInputs indicates that a given SSGen tx contains an
	// invalid number of inputs.
	ErrSSGenWrongNumInputs

	// ErrSSGenTooManyOutputs indicates that a given SSGen tx contains too
	// many outputs.
	ErrSSGenTooManyOutputs

	// ErrSSGenNoOutputs indicates that a given SSGen has no outputs.
	ErrSSGenNoOutputs

	// ErrSSGenWrongIndex indicates that a given SSGen sstx input was not
	// using the correct index.
	ErrSSGenWrongIndex

	// ErrSSGenWrongTxTree indicates that a given SSGen tx input was not found in
	// the stake tx tree.
	ErrSSGenWrongTxTree

	// ErrSSGenNoStakebase indicates that the SSGen tx did not contain a
	// valid StakeBase in the zeroeth position of inputs.
	ErrSSGenNoStakebase

	// ErrSSGenNoReference indicates that there is no reference OP_RETURN
	// included as the first output.
	ErrSSGenNoReference

	// ErrSSGenNoReference indicates that the OP_RETURN included as the
	// first output was corrupted in some way.
	ErrSSGenBadReference

	// ErrSSGenNoVotePush indicates that there is no vote bits OP_RETURN
	// included as the second output.
	ErrSSGenNoVotePush

	// ErrSSGenBadVotePush indicates that the OP_RETURN included as the
	// second output was corrupted in some way.
	ErrSSGenBadVotePush

	// ErrSSGenBadGenOuts indicates that the something was wrong with the
	// stake generation outputs that were present after the first two
	// OP_RETURN pushes in an SSGen tx.
	ErrSSGenBadGenOuts

	// ErrSSRtxWrongNumInputs indicates that a given SSRtx contains an
	// invalid number of inputs.
	ErrSSRtxWrongNumInputs

	// ErrSSRtxTooManyOutputs indicates that a given SSRtx contains too many
	// outputs.
	ErrSSRtxTooManyOutputs

	// ErrSSRtxNoOutputs indicates that a given SSRtx has no outputs.
	ErrSSRtxNoOutputs

	// ErrSSRtxWrongTxTree indicates that a given SSRtx input was not found in
	// the stake tx tree.
	ErrSSRtxWrongTxTree

	// ErrSSRtxBadGenOuts indicates that there was a non-SSRtx tagged output
	// present in an SSRtx.
	ErrSSRtxBadOuts

	// ErrVerSStxAmts indicates there was an error verifying the calculated
	// SStx out amounts and the actual SStx out amounts.
	ErrVerSStxAmts

	// ErrVerifyInput indicates that there was an error in verification
	// function input.
	ErrVerifyInput

	// ErrVerifyOutType indicates that there was a non-equivalence in the
	// output type.
	ErrVerifyOutType

	// ErrVerifyTooMuchFees indicates that a transaction's output gave
	// too much in fees after taking into accounts the limits imposed
	// by the SStx output's version field.
	ErrVerifyTooMuchFees

	// ErrVerifySpendTooMuch indicates that a transaction's output spent more
	// than it was allowed to spend based on the calculated subsidy or return
	// for a vote or revocation.
	ErrVerifySpendTooMuch

	// ErrVerifyOutputAmt indicates that for a vote/revocation spend output,
	// the rule was given that it must exactly match the calculated maximum,
	// however the amount in the output did not (e.g. it gave fees).
	ErrVerifyOutputAmt

	// ErrVerifyOutPkhs indicates that the recipient of the P2PKH or P2SH
	// script was different from that indicated in the SStx input.
	ErrVerifyOutPkhs

	// ErrDatabaseCorrupt indicates a database inconsistency.
	ErrDatabaseCorrupt

	// ErrMissingDatabaseTx indicates that a node disconnection failed to
	// pass a database transaction when attempted to remove a very old
	// node.
	ErrMissingDatabaseTx

	// ErrMemoryCorruption indicates that memory has somehow become corrupt,
	// for example invalid block header serialization from an in memory
	// struct.
	ErrMemoryCorruption

	// ErrFindTicketIdxs indicates a failure to find the selected ticket
	// indexes from the block header.
	ErrFindTicketIdxs

	// ErrMissingTicket indicates that a ticket was missing in one of the
	// ticket treaps when it was attempted to be fetched.
	ErrMissingTicket

	// ErrDuplicateTicket indicates that a duplicate ticket was attempted
	// to be inserted into a ticket treap or the database.
	ErrDuplicateTicket

	// ErrUnknownTicketSpent indicates that an unknown ticket was spent by
	// the block.
	ErrUnknownTicketSpent
)

// Map of ErrorCode values back to their constant names for pretty printing.
var errorCodeStrings = map[ErrorCode]string{
	ErrSStxTooManyInputs:    "ErrSStxTooManyInputs",
	ErrSStxTooManyOutputs:   "ErrSStxTooManyOutputs",
	ErrSStxNoOutputs:        "ErrSStxNoOutputs",
	ErrSStxInvalidInputs:    "ErrSStxInvalidInputs",
	ErrSStxInvalidOutputs:   "ErrSStxInvalidOutputs",
	ErrSStxInOutProportions: "ErrSStxInOutProportions",
	ErrSStxBadCommitAmount:  "ErrSStxBadCommitAmount",
	ErrSStxBadChangeAmts:    "ErrSStxBadChangeAmts",
	ErrSStxVerifyCalcAmts:   "ErrSStxVerifyCalcAmts",
	ErrSSGenWrongNumInputs:  "ErrSSGenWrongNumInputs",
	ErrSSGenTooManyOutputs:  "ErrSSGenTooManyOutputs",
	ErrSSGenNoOutputs:       "ErrSSGenNoOutputs",
	ErrSSGenWrongIndex:      "ErrSSGenWrongIndex",
	ErrSSGenWrongTxTree:     "ErrSSGenWrongTxTree",
	ErrSSGenNoStakebase:     "ErrSSGenNoStakebase",
	ErrSSGenNoReference:     "ErrSSGenNoReference",
	ErrSSGenBadReference:    "ErrSSGenBadReference",
	ErrSSGenNoVotePush:      "ErrSSGenNoVotePush",
	ErrSSGenBadVotePush:     "ErrSSGenBadVotePush",
	ErrSSGenBadGenOuts:      "ErrSSGenBadGenOuts",
	ErrSSRtxWrongNumInputs:  "ErrSSRtxWrongNumInputs",
	ErrSSRtxTooManyOutputs:  "ErrSSRtxTooManyOutputs",
	ErrSSRtxNoOutputs:       "ErrSSRtxNoOutputs",
	ErrSSRtxWrongTxTree:     "ErrSSRtxWrongTxTree",
	ErrSSRtxBadOuts:         "ErrSSRtxBadOuts",
	ErrVerSStxAmts:          "ErrVerSStxAmts",
	ErrVerifyInput:          "ErrVerifyInput",
	ErrVerifyOutType:        "ErrVerifyOutType",
	ErrVerifyTooMuchFees:    "ErrVerifyTooMuchFees",
	ErrVerifySpendTooMuch:   "ErrVerifySpendTooMuch",
	ErrVerifyOutputAmt:      "ErrVerifyOutputAmt",
	ErrVerifyOutPkhs:        "ErrVerifyOutPkhs",
	ErrDatabaseCorrupt:      "ErrDatabaseCorrupt",
	ErrMissingDatabaseTx:    "ErrMissingDatabaseTx",
	ErrMemoryCorruption:     "ErrMemoryCorruption",
	ErrFindTicketIdxs:       "ErrFindTicketIdxs",
	ErrMissingTicket:        "ErrMissingTicket",
	ErrDuplicateTicket:      "ErrDuplicateTicket",
	ErrUnknownTicketSpent:   "ErrUnknownTicketSpent",
}

// String returns the ErrorCode as a human-readable name.
func (e ErrorCode) String() string {
	if s := errorCodeStrings[e]; s != "" {
		return s
	}
	return fmt.Sprintf("Unknown ErrorCode (%d)", int(e))
}

// RuleError identifies a rule violation.  It is used to indicate that
// processing of a block or transaction failed due to one of the many validation
// rules.  The caller can use type assertions to determine if a failure was
// specifically due to a rule violation and access the ErrorCode field to
// ascertain the specific reason for the rule violation.
type RuleError struct {
	ErrorCode   ErrorCode // Describes the kind of error
	Description string    // Human readable description of the issue
}

// Error satisfies the error interface and prints human-readable errors.
func (e RuleError) Error() string {
	return e.Description
}

// GetCode satisfies the error interface and prints human-readable errors.
func (e RuleError) GetCode() ErrorCode {
	return e.ErrorCode
}

// stakeRuleError creates an RuleError given a set of arguments.
func stakeRuleError(c ErrorCode, desc string) RuleError {
	return RuleError{ErrorCode: c, Description: desc}
}
