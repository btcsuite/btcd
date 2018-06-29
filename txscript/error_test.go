// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"testing"
)

// TestErrorCodeStringer tests the stringized output for the ErrorCode type.
func TestErrorCodeStringer(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   ErrorCode
		want string
	}{
		{ErrInternal, "ErrInternal"},
		{ErrInvalidIndex, "ErrInvalidIndex"},
		{ErrInvalidSigHashSingleIndex, "ErrInvalidSigHashSingleIndex"},
		{ErrUnsupportedAddress, "ErrUnsupportedAddress"},
		{ErrTooManyRequiredSigs, "ErrTooManyRequiredSigs"},
		{ErrMalformedCoinbaseNullData, "ErrMalformedCoinbaseNullData"},
		{ErrTooMuchNullData, "ErrTooMuchNullData"},
		{ErrNotMultisigScript, "ErrNotMultisigScript"},
		{ErrEarlyReturn, "ErrEarlyReturn"},
		{ErrEmptyStack, "ErrEmptyStack"},
		{ErrEvalFalse, "ErrEvalFalse"},
		{ErrScriptUnfinished, "ErrScriptUnfinished"},
		{ErrInvalidProgramCounter, "ErrInvalidProgramCounter"},
		{ErrScriptTooBig, "ErrScriptTooBig"},
		{ErrElementTooBig, "ErrElementTooBig"},
		{ErrTooManyOperations, "ErrTooManyOperations"},
		{ErrStackOverflow, "ErrStackOverflow"},
		{ErrInvalidPubKeyCount, "ErrInvalidPubKeyCount"},
		{ErrInvalidSignatureCount, "ErrInvalidSignatureCount"},
		{ErrNumOutOfRange, "ErrNumOutOfRange"},
		{ErrVerify, "ErrVerify"},
		{ErrEqualVerify, "ErrEqualVerify"},
		{ErrNumEqualVerify, "ErrNumEqualVerify"},
		{ErrCheckSigVerify, "ErrCheckSigVerify"},
		{ErrCheckMultiSigVerify, "ErrCheckMultiSigVerify"},
		{ErrP2SHStakeOpCodes, "ErrP2SHStakeOpCodes"},
		{ErrDisabledOpcode, "ErrDisabledOpcode"},
		{ErrReservedOpcode, "ErrReservedOpcode"},
		{ErrMalformedPush, "ErrMalformedPush"},
		{ErrInvalidStackOperation, "ErrInvalidStackOperation"},
		{ErrUnbalancedConditional, "ErrUnbalancedConditional"},
		{ErrNegativeSubstrIdx, "ErrNegativeSubstrIdx"},
		{ErrOverflowSubstrIdx, "ErrOverflowSubstrIdx"},
		{ErrNegativeRotation, "ErrNegativeRotation"},
		{ErrOverflowRotation, "ErrOverflowRotation"},
		{ErrDivideByZero, "ErrDivideByZero"},
		{ErrNegativeShift, "ErrNegativeShift"},
		{ErrOverflowShift, "ErrOverflowShift"},
		{ErrMinimalData, "ErrMinimalData"},
		{ErrInvalidSigHashType, "ErrInvalidSigHashType"},
		{ErrSigTooShort, "ErrSigTooShort"},
		{ErrSigTooLong, "ErrSigTooLong"},
		{ErrSigInvalidSeqID, "ErrSigInvalidSeqID"},
		{ErrSigInvalidDataLen, "ErrSigInvalidDataLen"},
		{ErrSigMissingSTypeID, "ErrSigMissingSTypeID"},
		{ErrSigMissingSLen, "ErrSigMissingSLen"},
		{ErrSigInvalidSLen, "ErrSigInvalidSLen"},
		{ErrSigInvalidRIntID, "ErrSigInvalidRIntID"},
		{ErrSigZeroRLen, "ErrSigZeroRLen"},
		{ErrSigNegativeR, "ErrSigNegativeR"},
		{ErrSigTooMuchRPadding, "ErrSigTooMuchRPadding"},
		{ErrSigInvalidSIntID, "ErrSigInvalidSIntID"},
		{ErrSigZeroSLen, "ErrSigZeroSLen"},
		{ErrSigNegativeS, "ErrSigNegativeS"},
		{ErrSigTooMuchSPadding, "ErrSigTooMuchSPadding"},
		{ErrSigHighS, "ErrSigHighS"},
		{ErrNotPushOnly, "ErrNotPushOnly"},
		{ErrPubKeyType, "ErrPubKeyType"},
		{ErrCleanStack, "ErrCleanStack"},
		{ErrDiscourageUpgradableNOPs, "ErrDiscourageUpgradableNOPs"},
		{ErrNegativeLockTime, "ErrNegativeLockTime"},
		{ErrUnsatisfiedLockTime, "ErrUnsatisfiedLockTime"},
		{0xffff, "Unknown ErrorCode (65535)"},
	}

	// Detect additional error codes that don't have the stringer added.
	if len(tests)-1 != int(numErrorCodes) {
		t.Errorf("It appears an error code was added without adding an " +
			"associated stringer test")
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

// TestError tests the error output for the Error type.
func TestError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		in   Error
		want string
	}{
		{
			Error{Description: "some error"},
			"some error",
		},
		{
			Error{Description: "human-readable error"},
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

// TestIsDERSigError ensures IsDERSigError returns true for all error codes
// that can be returned as a result of non-canonically-encoded DER signatures.
func TestIsDERSigError(t *testing.T) {
	tests := []struct {
		code ErrorCode
		want bool
	}{
		{ErrSigTooShort, true},
		{ErrSigTooLong, true},
		{ErrSigInvalidSeqID, true},
		{ErrSigInvalidDataLen, true},
		{ErrSigMissingSTypeID, true},
		{ErrSigMissingSLen, true},
		{ErrSigInvalidSLen, true},
		{ErrSigInvalidRIntID, true},
		{ErrSigZeroRLen, true},
		{ErrSigNegativeR, true},
		{ErrSigTooMuchRPadding, true},
		{ErrSigInvalidSIntID, true},
		{ErrSigZeroSLen, true},
		{ErrSigNegativeS, true},
		{ErrSigTooMuchSPadding, true},
		{ErrSigHighS, true},
		{ErrEvalFalse, false},
		{ErrInvalidIndex, false},
	}
	for _, test := range tests {
		result := IsDERSigError(Error{ErrorCode: test.code})
		if result != test.want {
			t.Errorf("IsDERSigError(%v): unexpected result -- got: %v want: %v",
				test.code, result, test.want)
			continue
		}
	}
}
