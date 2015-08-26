// Copyright (c) 2014 Conformal Systems LLC.
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package schnorr

import (
	"fmt"
)

// ErrorCode identifies a kind of error.
type ErrorCode int

// These constants are used to identify a specific RuleError.
const (
	// ErrBadInputSize indicates that input to a signature was of the wrong size.
	ErrBadInputSize = iota

	// ErrInputValue indicates that the value of an input was wrong (e.g. zero).
	ErrInputValue

	// ErrSchnorrHashValue indicates that the hash of (R || m) was too large
	// and so a new k value (nonce) should be used.
	ErrSchnorrHashValue

	// ErrPointNotOnCurve indicates that a point was not on the given
	// elliptic curve.
	ErrPointNotOnCurve

	// ErrBadSigRYValue indicates that the calculated Y value of R was odd,
	// which is not allowed.
	ErrBadSigRYValue

	// ErrBadSigRNotOnCurve indicates that the calculated or given point R for some
	// signature was not on the curve.
	ErrBadSigRNotOnCurve

	// ErrUnequalRValues indicates that the calculated point R for some
	// signature was not the same as the given R value for the signature.
	ErrUnequalRValues

	// ErrRegenerateRPoint indicates that a point could not be regenerated
	// from r.
	ErrRegenerateRPoint

	// ErrPubKeyOffCurve indicates that a regenerated pubkey was off the curve.
	ErrPubKeyOffCurve

	// ErrRegenSig indicates that a regenerated pubkey could not be validated
	// against the signature.
	ErrRegenSig

	// ErrBadNonce indicates that a generated nonce from some algorithm was
	// unusable.
	ErrBadNonce

	// ErrZeroSigS indates a zero signature S value, which is invalid.
	ErrZeroSigS

	// ErrNonmatchingR indicates that all signatures to be combined in a
	// threshold signature failed to have a matching R value.
	ErrNonmatchingR
)

// Map of ErrorCode values back to their constant names for pretty printing.
var errorCodeStrings = map[ErrorCode]string{
	ErrBadInputSize:      "BadInputSize",
	ErrInputValue:        "ErrInputValue",
	ErrSchnorrHashValue:  "ErrSchnorrHashValue",
	ErrPointNotOnCurve:   "ErrPointNotOnCurve",
	ErrBadSigRYValue:     "ErrBadSigRYValue",
	ErrBadSigRNotOnCurve: "ErrBadSigRNotOnCurve",
	ErrRegenerateRPoint:  "ErrRegenerateRPoint",
	ErrPubKeyOffCurve:    "ErrPubKeyOffCurve",
	ErrRegenSig:          "ErrRegenSig",
	ErrBadNonce:          "ErrBadNonce",
	ErrZeroSigS:          "ErrZeroSigS",
	ErrNonmatchingR:      "ErrNonmatchingR",
}

// String returns the ErrorCode as a human-readable name.
func (e ErrorCode) String() string {
	if s := errorCodeStrings[e]; s != "" {
		return s
	}
	return fmt.Sprintf("Unknown ErrorCode (%d)", int(e))
}

// Error identifies a violation.
type Error struct {
	ErrorCode   ErrorCode // Describes the kind of error
	Description string    // Human readable description of the issue
}

// Error satisfies the error interface and prints human-readable errors.
func (e Error) Error() string {
	return e.Description
}

// GetCode satisfies the error interface and prints human-readable errors.
func (e Error) GetCode() ErrorCode {
	return e.ErrorCode
}

// schnorrError creates a Error given a set of arguments.
func schnorrError(c ErrorCode, desc string) Error {
	return Error{ErrorCode: c, Description: desc}
}
