// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"errors"
	"fmt"
)

// Engine execution errors.
var (
	// ErrStackShortScript is returned if the script has an opcode that is
	// too long for the length of the script.
	ErrStackShortScript = errors.New("execute past end of script")

	// ErrStackLongScript is returned if the script has an opcode that is
	// too long for the length of the script.
	ErrStackLongScript = errors.New("script is longer than maximum allowed")

	// ErrStackUnderflow is returned if an opcode requires more items on the
	// stack than is present.f
	ErrStackUnderflow = errors.New("stack underflow")

	// ErrStackInvalidArgs is returned if the argument for an opcode is out
	// of acceptable range.
	ErrStackInvalidArgs = errors.New("invalid argument")

	// ErrStackOpDisabled is returned when a disabled opcode is encountered
	// in the script.
	ErrStackOpDisabled = errors.New("disabled opcode")

	// ErrStackVerifyFailed is returned when one of the OP_VERIFY or
	// OP_*VERIFY instructions is executed and the conditions fails.
	ErrStackVerifyFailed = errors.New("verify failed")

	// ErrStackNumberTooBig is returned when the argument for an opcode that
	// should be an offset is obviously far too large.
	ErrStackNumberTooBig = errors.New("number too big")

	// ErrStackInvalidOpcode is returned when an opcode marked as invalid or
	// a completely undefined opcode is encountered.
	ErrStackInvalidOpcode = errors.New("invalid opcode")

	// ErrStackReservedOpcode is returned when an opcode marked as reserved
	// is encountered.
	ErrStackReservedOpcode = errors.New("reserved opcode")

	// ErrStackEarlyReturn is returned when OP_RETURN is executed in the
	// script.
	ErrStackEarlyReturn = errors.New("script returned early")

	// ErrStackNoIf is returned if an OP_ELSE or OP_ENDIF is encountered
	// without first having an OP_IF or OP_NOTIF in the script.
	ErrStackNoIf = errors.New("OP_ELSE or OP_ENDIF with no matching OP_IF")

	// ErrStackMissingEndif is returned if the end of a script is reached
	// without and OP_ENDIF to correspond to a conditional expression.
	ErrStackMissingEndif = fmt.Errorf("execute fail, in conditional execution")

	// ErrStackTooManyPubKeys is returned if an OP_CHECKMULTISIG is
	// encountered with more than MaxPubKeysPerMultiSig pubkeys present.
	ErrStackTooManyPubKeys = errors.New("invalid pubkey count in OP_CHECKMULTISIG")

	// ErrStackTooManyOperations is returned if a script has more than
	// MaxOpsPerScript opcodes that do not push data.
	ErrStackTooManyOperations = errors.New("too many operations in script")

	// ErrStackElementTooBig is returned if the size of an element to be
	// pushed to the stack is over MaxScriptElementSize.
	ErrStackElementTooBig = errors.New("element in script too large")

	// ErrStackUnknownAddress is returned when ScriptToAddrHash does not
	// recognize the pattern of the script and thus can not find the address
	// for payment.
	ErrStackUnknownAddress = errors.New("non-recognised address")

	// ErrStackScriptFailed is returned when at the end of a script the
	// boolean on top of the stack is false signifying that the script has
	// failed.
	ErrStackScriptFailed = errors.New("execute fail, fail on stack")

	// ErrStackScriptUnfinished is returned when CheckErrorCondition is
	// called on a script that has not finished executing.
	ErrStackScriptUnfinished = errors.New("error check when script unfinished")

	// ErrStackEmptyStack is returned when the stack is empty at the end of
	// execution. Normal operation requires that a boolean is on top of the
	// stack when the scripts have finished executing.
	ErrStackEmptyStack = errors.New("stack empty at end of execution")

	// ErrStackP2SHNonPushOnly is returned when a Pay-to-Script-Hash
	// transaction is encountered and the ScriptSig does operations other
	// than push data (in violation of bip16).
	ErrStackP2SHNonPushOnly = errors.New("pay to script hash with non " +
		"pushonly input")

	// ErrStackInvalidParseType is an internal error returned from
	// ScriptToAddrHash ony if the internal data tables are wrong.
	ErrStackInvalidParseType = errors.New("internal error: invalid parsetype found")

	// ErrStackInvalidAddrOffset is an internal error returned from
	// ScriptToAddrHash ony if the internal data tables are wrong.
	ErrStackInvalidAddrOffset = errors.New("internal error: invalid offset found")

	// ErrStackInvalidIndex is returned when an out-of-bounds index was
	// passed to a function.
	ErrStackInvalidIndex = errors.New("invalid script index")

	// ErrStackNonPushOnly is returned when ScriptInfo is called with a
	// pkScript that peforms operations other that pushing data to the stack.
	ErrStackNonPushOnly = errors.New("SigScript is non pushonly")

	// ErrStackOverflow is returned when stack and altstack combined depth
	// is over the limit.
	ErrStackOverflow = errors.New("stack overflow")

	// ErrStackInvalidLowSSignature is returned when the ScriptVerifyLowS
	// flag is set and the script contains any signatures whose S values
	// are higher than the half order.
	ErrStackInvalidLowSSignature = errors.New("invalid low s signature")

	// ErrStackInvalidPubKey is returned when the ScriptVerifyScriptEncoding
	// flag is set and the script contains invalid pubkeys.
	ErrStackInvalidPubKey = errors.New("invalid strict pubkey")

	// ErrStackCleanStack is returned when the ScriptVerifyCleanStack flag
	// is set and after evalution the stack does not contain only one element,
	// which also must be true if interpreted as a boolean.
	ErrStackCleanStack = errors.New("stack is not clean")

	// ErrStackMinimalData is returned when the ScriptVerifyMinimalData flag
	// is set and the script contains push operations that do not use
	// the minimal opcode required.
	ErrStackMinimalData = errors.New("non-minimally encoded script number")
)

// Engine script errors.
var (
	// ErrInvalidFlags is returned when the passed flags to NewScript
	// contain an invalid combination.
	ErrInvalidFlags = errors.New("invalid flags combination")

	// ErrInvalidIndex is returned when the passed input index for the
	// provided transaction is out of range.
	ErrInvalidIndex = errors.New("invalid input index")

	// ErrUnsupportedAddress is returned when a concrete type that
	// implements a dcrutil.Address is not a supported type.
	ErrUnsupportedAddress = errors.New("unsupported address type")

	// ErrBadNumRequired is returned from MultiSigScript when nrequired is
	// larger than the number of provided public keys.
	ErrBadNumRequired = errors.New("more signatures required than keys present")

	// ErrSighashSingleIdx
	ErrSighashSingleIdx = errors.New("invalid SIGHASH_SINGLE script index")

	// ErrSubstrIndexNegative indicates that the substring index was negative
	// and thus invalid.
	ErrSubstrIdxNegative = errors.New("negative number given for substring " +
		"index")

	// ErrSubstrIdxOutOfBounds indicates that the substring index was too large
	// and thus invalid.
	ErrSubstrIdxOutOfBounds = errors.New("out of bounds number given for " +
		"substring index")

	// ErrNegativeRotation indicates that too low of a rotation depth was given
	// for a uint32 bit rotation.
	ErrNegativeRotation = errors.New("rotation depth negative")

	// ErrRotationOverflow indicates that too high of a rotation depth was given
	// for a uint32 bit rotation.
	ErrRotationOverflow = errors.New("rotation depth out of bounds")

	// ErrNegativeRotation indicates that too low of a shift depth was given
	// for a uint32 bit shift.
	ErrNegativeShift = errors.New("shift depth negative")

	// ErrShiftOverflow indicates that too high of a shift depth was given
	// for a uint32 bit shift.
	ErrShiftOverflow = errors.New("shift depth out of bounds")

	// ErrDivideByZero indicates that a user attempted to divide by zero.
	ErrDivideByZero = errors.New("division by zero")

	// ErrP2SHStakeOpCodes indicates a P2SH script contained stake op codes.
	ErrP2SHStakeOpCodes = errors.New("stake opcodes were found in a p2sh script")
)
