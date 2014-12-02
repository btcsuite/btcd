// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"github.com/mably/btcchain"
	"github.com/mably/btcwire"
)

// RuleError identifies a rule violation.  It is used to indicate that
// processing of a transaction failed due to one of the many validation
// rules.  The caller can use type assertions to determine if a failure was
// specifically due to a rule violation and use the Err field to access the
// underlying error, which will be either a TxRuleError or a btcchain.RuleError.
type RuleError struct {
	Err error
}

// Error satisfies the error interface and prints human-readable errors.
func (e RuleError) Error() string {
	if e.Err == nil {
		return "<nil>"
	}
	return e.Err.Error()
}

// TxRuleError identifies a rule violation.  It is used to indicate that
// processing of a transaction failed due to one of the many validation
// rules.  The caller can use type assertions to determine if a failure was
// specifically due to a rule violation and access the ErrorCode field to
// ascertain the specific reason for the rule violation.
type TxRuleError struct {
	RejectCode  btcwire.RejectCode // The code to send with reject messages
	Description string             // Human readable description of the issue
}

// Error satisfies the error interface and prints human-readable errors.
func (e TxRuleError) Error() string {
	return e.Description
}

// txRuleError creates an underlying TxRuleError with the given a set of
// arguments and returns a RuleError that encapsulates it.
func txRuleError(c btcwire.RejectCode, desc string) RuleError {
	return RuleError{
		Err: TxRuleError{RejectCode: c, Description: desc},
	}
}

// chainRuleError returns a RuleError that encapsulates the given
// btcchain.RuleError.
func chainRuleError(chainErr btcchain.RuleError) RuleError {
	return RuleError{
		Err: chainErr,
	}
}

// extractRejectCode attempts to return a relevant reject code for a given error
// by examining the error for known types.  It will return true if a code
// was successfully extracted.
func extractRejectCode(err error) (btcwire.RejectCode, bool) {
	// Pull the underlying error out of a RuleError.
	if rerr, ok := err.(RuleError); ok {
		err = rerr.Err
	}

	switch err := err.(type) {
	case btcchain.RuleError:
		// Convert the chain error to a reject code.
		var code btcwire.RejectCode
		switch err.ErrorCode {
		// Rejected due to duplicate.
		case btcchain.ErrDuplicateBlock:
			fallthrough
		case btcchain.ErrDoubleSpend:
			code = btcwire.RejectDuplicate

		// Rejected due to obsolete version.
		case btcchain.ErrBlockVersionTooOld:
			code = btcwire.RejectObsolete

		// Rejected due to checkpoint.
		case btcchain.ErrCheckpointTimeTooOld:
			fallthrough
		case btcchain.ErrDifficultyTooLow:
			fallthrough
		case btcchain.ErrBadCheckpoint:
			fallthrough
		case btcchain.ErrForkTooOld:
			code = btcwire.RejectCheckpoint

		// Everything else is due to the block or transaction being invalid.
		default:
			code = btcwire.RejectInvalid
		}

		return code, true

	case TxRuleError:
		return err.RejectCode, true

	case nil:
		return btcwire.RejectInvalid, false
	}

	return btcwire.RejectInvalid, false
}

// errToRejectErr examines the underlying type of the error and returns a reject
// code and string appropriate to be sent in a btcwire.MsgReject message.
func errToRejectErr(err error) (btcwire.RejectCode, string) {
	// Return the reject code along with the error text if it can be
	// extracted from the error.
	rejectCode, found := extractRejectCode(err)
	if found {
		return rejectCode, err.Error()
	}

	// Return a generic rejected string if there is no error.  This really
	// should not happen unless the code elsewhere is not setting an error
	// as it should be, but it's best to be safe and simply return a generic
	// string rather than allowing the following code that derferences the
	// err to panic.
	if err == nil {
		return btcwire.RejectInvalid, "rejected"
	}

	// When the underlying error is not one of the above cases, just return
	// btcwire.RejectInvalid with a generic rejected string plus the error
	// text.
	return btcwire.RejectInvalid, "rejected: " + err.Error()
}
