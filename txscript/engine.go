// Copyright (c) 2013-2018 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/big"
	"strings"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// ScriptFlags is a bitmask defining additional operations or tests that will be
// done when executing a script pair.
type ScriptFlags uint32

const (
	// ScriptBip16 defines whether the bip16 threshold has passed and thus
	// pay-to-script hash transactions will be fully validated.
	ScriptBip16 ScriptFlags = 1 << iota

	// ScriptStrictMultiSig defines whether to verify the stack item
	// used by CHECKMULTISIG is zero length.
	ScriptStrictMultiSig

	// ScriptDiscourageUpgradableNops defines whether to verify that
	// NOP1 through NOP10 are reserved for future soft-fork upgrades.  This
	// flag must not be used for consensus critical code nor applied to
	// blocks as this flag is only for stricter standard transaction
	// checks.  This flag is only applied when the above opcodes are
	// executed.
	ScriptDiscourageUpgradableNops

	// ScriptVerifyCheckLockTimeVerify defines whether to verify that
	// a transaction output is spendable based on the locktime.
	// This is BIP0065.
	ScriptVerifyCheckLockTimeVerify

	// ScriptVerifyCheckSequenceVerify defines whether to allow execution
	// pathways of a script to be restricted based on the age of the output
	// being spent.  This is BIP0112.
	ScriptVerifyCheckSequenceVerify

	// ScriptVerifyCleanStack defines that the stack must contain only
	// one stack element after evaluation and that the element must be
	// true if interpreted as a boolean.  This is rule 6 of BIP0062.
	// This flag should never be used without the ScriptBip16 flag nor the
	// ScriptVerifyWitness flag.
	ScriptVerifyCleanStack

	// ScriptVerifyDERSignatures defines that signatures are required
	// to compily with the DER format.
	ScriptVerifyDERSignatures

	// ScriptVerifyLowS defines that signtures are required to comply with
	// the DER format and whose S value is <= order / 2.  This is rule 5
	// of BIP0062.
	ScriptVerifyLowS

	// ScriptVerifyMinimalData defines that signatures must use the smallest
	// push operator. This is both rules 3 and 4 of BIP0062.
	ScriptVerifyMinimalData

	// ScriptVerifyNullFail defines that signatures must be empty if
	// a CHECKSIG or CHECKMULTISIG operation fails.
	ScriptVerifyNullFail

	// ScriptVerifySigPushOnly defines that signature scripts must contain
	// only pushed data.  This is rule 2 of BIP0062.
	ScriptVerifySigPushOnly

	// ScriptVerifyStrictEncoding defines that signature scripts and
	// public keys must follow the strict encoding requirements.
	ScriptVerifyStrictEncoding

	// ScriptVerifyWitness defines whether or not to verify a transaction
	// output using a witness program template.
	ScriptVerifyWitness

	// ScriptVerifyDiscourageUpgradeableWitnessProgram makes witness
	// program with versions 2-16 non-standard.
	ScriptVerifyDiscourageUpgradeableWitnessProgram

	// ScriptVerifyMinimalIf makes a script with an OP_IF/OP_NOTIF whose
	// operand is anything other than empty vector or [0x01] non-standard.
	ScriptVerifyMinimalIf

	// ScriptVerifyWitnessPubKeyType makes a script within a check-sig
	// operation whose public key isn't serialized in a compressed format
	// non-standard.
	ScriptVerifyWitnessPubKeyType

	// ScriptVerifyTaproot defines whether or not to verify a transaction
	// output using the new taproot validation rules.
	ScriptVerifyTaproot

	// ScriptVerifyDiscourageUpgradeableWitnessProgram defines whether or
	// not to consider any new/unknown taproot leaf versions as
	// non-standard.
	ScriptVerifyDiscourageUpgradeableTaprootVersion

	// ScriptVerifyDiscourageOpSuccess defines whether or not to consider
	// usage of OP_SUCCESS op codes during tapscript execution as
	// non-standard.
	ScriptVerifyDiscourageOpSuccess

	// ScriptVerifyDiscourageUpgradeablePubkeyType defines if unknown
	// public key versions (during tapscript execution) is non-standard.
	ScriptVerifyDiscourageUpgradeablePubkeyType
)

const (
	// MaxStackSize is the maximum combined height of stack and alt stack
	// during execution.
	MaxStackSize = 1000

	// MaxScriptSize is the maximum allowed length of a raw script.
	MaxScriptSize = 10000

	// payToWitnessPubKeyHashDataSize is the size of the witness program's
	// data push for a pay-to-witness-pub-key-hash output.
	payToWitnessPubKeyHashDataSize = 20

	// payToWitnessScriptHashDataSize is the size of the witness program's
	// data push for a pay-to-witness-script-hash output.
	payToWitnessScriptHashDataSize = 32

	// payToTaprootDataSize is the size of the witness program push for
	// taproot spends. This will be the serialized x-coordinate of the
	// top-level taproot output public key.
	payToTaprootDataSize = 32
)

const (
	// BaseSegwitWitnessVersion is the original witness version that defines
	// the initial set of segwit validation logic.
	BaseSegwitWitnessVersion = 0

	// TaprootWitnessVersion is the witness version that defines the new
	// taproot verification logic.
	TaprootWitnessVersion = 1
)

// halforder is used to tame ECDSA malleability (see BIP0062).
var halfOrder = new(big.Int).Rsh(btcec.S256().N, 1)

// taprootExecutionCtx houses the special context-specific information we need
// to validate a taproot script spend. This includes the annex, the running sig
// op count tally, and other relevant information.
type taprootExecutionCtx struct {
	annex []byte

	codeSepPos uint32

	tapLeafHash chainhash.Hash

	sigOpsBudget int32

	mustSucceed bool
}

// sigOpsDelta is both the starting budget for sig ops for tapscript
// verification, as well as the decrease in the total budget when we encounter
// a signature.
const sigOpsDelta = 50

// tallysigOp attempts to decrease the current sig ops budget by sigOpsDelta.
// An error is returned if after subtracting the delta, the budget is below
// zero.
func (t *taprootExecutionCtx) tallysigOp() error {
	t.sigOpsBudget -= sigOpsDelta

	if t.sigOpsBudget < 0 {
		return scriptError(ErrTaprootMaxSigOps, "")
	}

	return nil
}

// newTaprootExecutionCtx returns a fresh instance of the taproot execution
// context.
func newTaprootExecutionCtx(inputWitnessSize int32) *taprootExecutionCtx {
	return &taprootExecutionCtx{
		codeSepPos:   blankCodeSepValue,
		sigOpsBudget: sigOpsDelta + inputWitnessSize,
	}
}

// Engine is the virtual machine that executes scripts.
type Engine struct {
	// The following fields are set when the engine is created and must not be
	// changed afterwards.  The entries of the signature cache are mutated
	// during execution, however, the cache pointer itself is not changed.
	//
	// flags specifies the additional flags which modify the execution behavior
	// of the engine.
	//
	// tx identifies the transaction that contains the input which in turn
	// contains the signature script being executed.
	//
	// txIdx identifies the input index within the transaction that contains
	// the signature script being executed.
	//
	// version specifies the version of the public key script to execute.  Since
	// signature scripts redeem public keys scripts, this means the same version
	// also extends to signature scripts and redeem scripts in the case of
	// pay-to-script-hash.
	//
	// bip16 specifies that the public key script is of a special form that
	// indicates it is a BIP16 pay-to-script-hash and therefore the
	// execution must be treated as such.
	//
	// sigCache caches the results of signature verifications.  This is useful
	// since transaction scripts are often executed more than once from various
	// contexts (e.g. new block templates, when transactions are first seen
	// prior to being mined, part of full block verification, etc).
	//
	// hashCache caches the midstate of segwit v0 and v1 sighashes to
	// optimize worst-case hashing complexity.
	//
	// prevOutFetcher is used to look up all the previous output of
	// taproot transactions, as that information is hashed into the
	// sighash digest for such inputs.
	flags          ScriptFlags
	tx             wire.MsgTx
	txIdx          int
	version        uint16
	bip16          bool
	sigCache       *SigCache
	hashCache      *TxSigHashes
	prevOutFetcher PrevOutputFetcher

	// The following fields handle keeping track of the current execution state
	// of the engine.
	//
	// scripts houses the raw scripts that are executed by the engine.  This
	// includes the signature script as well as the public key script.  It also
	// includes the redeem script in the case of pay-to-script-hash.
	//
	// scriptIdx tracks the index into the scripts array for the current program
	// counter.
	//
	// opcodeIdx tracks the number of the opcode within the current script for
	// the current program counter.  Note that it differs from the actual byte
	// index into the script and is really only used for disassembly purposes.
	//
	// lastCodeSep specifies the position within the current script of the last
	// OP_CODESEPARATOR.
	//
	// tokenizer provides the token stream of the current script being executed
	// and doubles as state tracking for the program counter within the script.
	//
	// savedFirstStack keeps a copy of the stack from the first script when
	// performing pay-to-script-hash execution.
	//
	// dstack is the primary data stack the various opcodes push and pop data
	// to and from during execution.
	//
	// astack is the alternate data stack the various opcodes push and pop data
	// to and from during execution.
	//
	// condStack tracks the conditional execution state with support for
	// multiple nested conditional execution opcodes.
	//
	// numOps tracks the total number of non-push operations in a script and is
	// primarily used to enforce maximum limits.
	scripts         [][]byte
	scriptIdx       int
	opcodeIdx       int
	lastCodeSep     int
	tokenizer       ScriptTokenizer
	savedFirstStack [][]byte
	dstack          stack
	astack          stack
	condStack       []int
	numOps          int
	witnessVersion  int
	witnessProgram  []byte
	inputAmount     int64
	taprootCtx      *taprootExecutionCtx

	// stepCallback is an optional function that will be called every time
	// a step has been performed during script execution.
	//
	// NOTE: This is only meant to be used in debugging, and SHOULD NOT BE
	// USED during regular operation.
	stepCallback func(*StepInfo) error
}

// StepInfo houses the current VM state information that is passed back to the
// stepCallback during script execution.
type StepInfo struct {
	// ScriptIndex is the index of the script currently being executed by
	// the Engine.
	ScriptIndex int

	// OpcodeIndex is the index of the next opcode that will be executed.
	// In case the execution has completed, the opcode index will be
	// incrementet beyond the number of the current script's opcodes. This
	// indicates no new script is being executed, and execution is done.
	OpcodeIndex int

	// Stack is the Engine's current content on the stack:
	Stack [][]byte

	// AltStack is the Engine's current content on the alt stack.
	AltStack [][]byte
}

// hasFlag returns whether the script engine instance has the passed flag set.
func (vm *Engine) hasFlag(flag ScriptFlags) bool {
	return vm.flags&flag == flag
}

// isBranchExecuting returns whether or not the current conditional branch is
// actively executing.  For example, when the data stack has an OP_FALSE on it
// and an OP_IF is encountered, the branch is inactive until an OP_ELSE or
// OP_ENDIF is encountered.  It properly handles nested conditionals.
func (vm *Engine) isBranchExecuting() bool {
	if len(vm.condStack) == 0 {
		return true
	}
	return vm.condStack[len(vm.condStack)-1] == OpCondTrue
}

// isOpcodeDisabled returns whether or not the opcode is disabled and thus is
// always bad to see in the instruction stream (even if turned off by a
// conditional).
func isOpcodeDisabled(opcode byte) bool {
	switch opcode {
	case OP_CAT:
		return true
	case OP_SUBSTR:
		return true
	case OP_LEFT:
		return true
	case OP_RIGHT:
		return true
	case OP_INVERT:
		return true
	case OP_AND:
		return true
	case OP_OR:
		return true
	case OP_XOR:
		return true
	case OP_2MUL:
		return true
	case OP_2DIV:
		return true
	case OP_MUL:
		return true
	case OP_DIV:
		return true
	case OP_MOD:
		return true
	case OP_LSHIFT:
		return true
	case OP_RSHIFT:
		return true
	default:
		return false
	}
}

// isOpcodeAlwaysIllegal returns whether or not the opcode is always illegal
// when passed over by the program counter even if in a non-executed branch (it
// isn't a coincidence that they are conditionals).
func isOpcodeAlwaysIllegal(opcode byte) bool {
	switch opcode {
	case OP_VERIF:
		return true
	case OP_VERNOTIF:
		return true
	default:
		return false
	}
}

// isOpcodeConditional returns whether or not the opcode is a conditional opcode
// which changes the conditional execution stack when executed.
func isOpcodeConditional(opcode byte) bool {
	switch opcode {
	case OP_IF:
		return true
	case OP_NOTIF:
		return true
	case OP_ELSE:
		return true
	case OP_ENDIF:
		return true
	default:
		return false
	}
}

// checkMinimalDataPush returns whether or not the provided opcode is the
// smallest possible way to represent the given data.  For example, the value 15
// could be pushed with OP_DATA_1 15 (among other variations); however, OP_15 is
// a single opcode that represents the same value and is only a single byte
// versus two bytes.
func checkMinimalDataPush(op *opcode, data []byte) error {
	opcodeVal := op.value
	dataLen := len(data)
	switch {
	case dataLen == 0 && opcodeVal != OP_0:
		str := fmt.Sprintf("zero length data push is encoded with opcode %s "+
			"instead of OP_0", op.name)
		return scriptError(ErrMinimalData, str)
	case dataLen == 1 && data[0] >= 1 && data[0] <= 16:
		if opcodeVal != OP_1+data[0]-1 {
			// Should have used OP_1 .. OP_16
			str := fmt.Sprintf("data push of the value %d encoded with opcode "+
				"%s instead of OP_%d", data[0], op.name, data[0])
			return scriptError(ErrMinimalData, str)
		}
	case dataLen == 1 && data[0] == 0x81:
		if opcodeVal != OP_1NEGATE {
			str := fmt.Sprintf("data push of the value -1 encoded with opcode "+
				"%s instead of OP_1NEGATE", op.name)
			return scriptError(ErrMinimalData, str)
		}
	case dataLen <= 75:
		if int(opcodeVal) != dataLen {
			// Should have used a direct push
			str := fmt.Sprintf("data push of %d bytes encoded with opcode %s "+
				"instead of OP_DATA_%d", dataLen, op.name, dataLen)
			return scriptError(ErrMinimalData, str)
		}
	case dataLen <= 255:
		if opcodeVal != OP_PUSHDATA1 {
			str := fmt.Sprintf("data push of %d bytes encoded with opcode %s "+
				"instead of OP_PUSHDATA1", dataLen, op.name)
			return scriptError(ErrMinimalData, str)
		}
	case dataLen <= 65535:
		if opcodeVal != OP_PUSHDATA2 {
			str := fmt.Sprintf("data push of %d bytes encoded with opcode %s "+
				"instead of OP_PUSHDATA2", dataLen, op.name)
			return scriptError(ErrMinimalData, str)
		}
	}
	return nil
}

// executeOpcode peforms execution on the passed opcode.  It takes into account
// whether or not it is hidden by conditionals, but some rules still must be
// tested in this case.
func (vm *Engine) executeOpcode(op *opcode, data []byte) error {
	// Disabled opcodes are fail on program counter.
	if isOpcodeDisabled(op.value) {
		str := fmt.Sprintf("attempt to execute disabled opcode %s", op.name)
		return scriptError(ErrDisabledOpcode, str)
	}

	// Always-illegal opcodes are fail on program counter.
	if isOpcodeAlwaysIllegal(op.value) {
		str := fmt.Sprintf("attempt to execute reserved opcode %s", op.name)
		return scriptError(ErrReservedOpcode, str)
	}

	// Note that this includes OP_RESERVED which counts as a push operation.
	if vm.taprootCtx == nil && op.value > OP_16 {
		vm.numOps++
		if vm.numOps > MaxOpsPerScript {
			str := fmt.Sprintf("exceeded max operation limit of %d",
				MaxOpsPerScript)
			return scriptError(ErrTooManyOperations, str)
		}

	} else if len(data) > MaxScriptElementSize {
		str := fmt.Sprintf("element size %d exceeds max allowed size %d",
			len(data), MaxScriptElementSize)
		return scriptError(ErrElementTooBig, str)
	}

	// Nothing left to do when this is not a conditional opcode and it is
	// not in an executing branch.
	if !vm.isBranchExecuting() && !isOpcodeConditional(op.value) {
		return nil
	}

	// Ensure all executed data push opcodes use the minimal encoding when
	// the minimal data verification flag is set.
	if vm.dstack.verifyMinimalData && vm.isBranchExecuting() &&
		op.value >= 0 && op.value <= OP_PUSHDATA4 {

		if err := checkMinimalDataPush(op, data); err != nil {
			return err
		}
	}

	return op.opfunc(op, data, vm)
}

// checkValidPC returns an error if the current script position is not valid for
// execution.
func (vm *Engine) checkValidPC() error {
	if vm.scriptIdx >= len(vm.scripts) {
		str := fmt.Sprintf("script index %d beyond total scripts %d",
			vm.scriptIdx, len(vm.scripts))
		return scriptError(ErrInvalidProgramCounter, str)
	}
	return nil
}

// isWitnessVersionActive returns true if a witness program was extracted
// during the initialization of the Engine, and the program's version matches
// the specified version.
func (vm *Engine) isWitnessVersionActive(version uint) bool {
	return vm.witnessProgram != nil && uint(vm.witnessVersion) == version
}

// verifyWitnessProgram validates the stored witness program using the passed
// witness as input.
func (vm *Engine) verifyWitnessProgram(witness wire.TxWitness) error {
	switch {

	// We're attempting to verify a base (witness version 0) segwit output,
	// so we'll be looking for either a p2wsh or a p2wkh spend.
	case vm.isWitnessVersionActive(BaseSegwitWitnessVersion):
		switch len(vm.witnessProgram) {
		case payToWitnessPubKeyHashDataSize: // P2WKH
			// The witness stack should consist of exactly two
			// items: the signature, and the pubkey.
			if len(witness) != 2 {
				err := fmt.Sprintf("should have exactly two "+
					"items in witness, instead have %v", len(witness))
				return scriptError(ErrWitnessProgramMismatch, err)
			}

			// Now we'll resume execution as if it were a regular
			// p2pkh transaction.
			pkScript, err := payToPubKeyHashScript(vm.witnessProgram)
			if err != nil {
				return err
			}

			const scriptVersion = 0
			err = checkScriptParses(vm.version, pkScript)
			if err != nil {
				return err
			}

			// Set the stack to the provided witness stack, then
			// append the pkScript generated above as the next
			// script to execute.
			vm.scripts = append(vm.scripts, pkScript)
			vm.SetStack(witness)

		case payToWitnessScriptHashDataSize: // P2WSH
			// Additionally, The witness stack MUST NOT be empty at
			// this point.
			if len(witness) == 0 {
				return scriptError(ErrWitnessProgramEmpty, "witness "+
					"program empty passed empty witness")
			}

			// Obtain the witness script which should be the last
			// element in the passed stack. The size of the script
			// MUST NOT exceed the max script size.
			witnessScript := witness[len(witness)-1]
			if len(witnessScript) > MaxScriptSize {
				str := fmt.Sprintf("witnessScript size %d "+
					"is larger than max allowed size %d",
					len(witnessScript), MaxScriptSize)
				return scriptError(ErrScriptTooBig, str)
			}

			// Ensure that the serialized pkScript at the end of
			// the witness stack matches the witness program.
			witnessHash := sha256.Sum256(witnessScript)
			if !bytes.Equal(witnessHash[:], vm.witnessProgram) {
				return scriptError(ErrWitnessProgramMismatch,
					"witness program hash mismatch")
			}

			// With all the validity checks passed, assert that the
			// script parses without failure.
			const scriptVersion = 0
			err := checkScriptParses(vm.version, witnessScript)
			if err != nil {
				return err
			}

			// The hash matched successfully, so use the witness as
			// the stack, and set the witnessScript to be the next
			// script executed.
			vm.scripts = append(vm.scripts, witnessScript)
			vm.SetStack(witness[:len(witness)-1])

		default:
			errStr := fmt.Sprintf("length of witness program "+
				"must either be %v or %v bytes, instead is %v bytes",
				payToWitnessPubKeyHashDataSize,
				payToWitnessScriptHashDataSize,
				len(vm.witnessProgram))
			return scriptError(ErrWitnessProgramWrongLength, errStr)
		}

	// We're attempting to to verify a taproot input, and the witness
	// program data push is of the expected size, so we'll be looking for a
	// normal key-path spend, or a merkle proof for a tapscript with
	// execution afterwards.
	case vm.isWitnessVersionActive(TaprootWitnessVersion) &&
		len(vm.witnessProgram) == payToTaprootDataSize && !vm.bip16:

		// If taproot isn't currently active, then we'll return a
		// success here in place as we don't apply the new rules unless
		// the flag flips, as governed by the version bits deployment.
		if !vm.hasFlag(ScriptVerifyTaproot) {
			return nil
		}

		// If there're no stack elements at all, then this is an
		// invalid spend.
		if len(witness) == 0 {
			return scriptError(ErrWitnessProgramEmpty, "witness "+
				"program empty passed empty witness")
		}

		// At this point, we know taproot is active, so we'll populate
		// the taproot execution context.
		vm.taprootCtx = newTaprootExecutionCtx(
			int32(witness.SerializeSize()),
		)

		// If we can detect the annex, then drop that off the stack,
		// we'll only need it to compute the sighash later.
		if isAnnexedWitness(witness) {
			vm.taprootCtx.annex, _ = extractAnnex(witness)

			// Snip the annex off the end of the witness stack.
			witness = witness[:len(witness)-1]
		}

		// From here, we'll either be validating a normal key spend, or
		// a spend from the tap script leaf using a committed leaf.
		switch {
		// If there's only a single element left on the stack (the
		// signature), then we'll apply the normal top-level schnorr
		// signature verification.
		case len(witness) == 1:
			// As we only have a single element left (after maybe
			// removing the annex), we'll do normal taproot
			// keyspend validation.
			rawSig := witness[0]
			err := VerifyTaprootKeySpend(
				vm.witnessProgram, rawSig, &vm.tx, vm.txIdx,
				vm.prevOutFetcher, vm.hashCache, vm.sigCache,
			)
			if err != nil {
				// TODO(roasbeef): proper error
				return err
			}

			// TODO(roasbeef): or remove the other items from the stack?
			vm.taprootCtx.mustSucceed = true
			return nil

		// Otherwise, we need to attempt full tapscript leaf
		// verification in place.
		default:
			// First, attempt to parse the control block, if this
			// isn't formatted properly, then we'll end execution
			// right here.
			controlBlock, err := ParseControlBlock(
				witness[len(witness)-1],
			)
			if err != nil {
				return err
			}

			// Now that we know the control block is valid, we'll
			// verify the top-level taproot commitment, which
			// proves that the specified script was committed to in
			// the merkle tree.
			witnessScript := witness[len(witness)-2]
			err = VerifyTaprootLeafCommitment(
				controlBlock, vm.witnessProgram, witnessScript,
			)
			if err != nil {
				return err
			}

			// Now that we know the commitment is valid, we'll
			// check to see if OP_SUCCESS op codes are found in the
			// script. If so, then we'll return here early as we
			// skip proper validation.
			if ScriptHasOpSuccess(witnessScript) {
				// An op success op code has been found, however if
				// the policy flag forbidding them is active, then
				// we'll return an error.
				if vm.hasFlag(ScriptVerifyDiscourageOpSuccess) {
					errStr := fmt.Sprintf("script contains " +
						"OP_SUCCESS op code")
					return scriptError(ErrDiscourageOpSuccess, errStr)
				}

				// Otherwise, the script passes scott free.
				vm.taprootCtx.mustSucceed = true
				return nil
			}

			// Before we proceed with normal execution, check the
			// leaf version of the script, as if the policy flag is
			// active, then we should only allow the base leaf
			// version.
			if controlBlock.LeafVersion != BaseLeafVersion {
				switch {
				case vm.hasFlag(ScriptVerifyDiscourageUpgradeableTaprootVersion):
					errStr := fmt.Sprintf("tapscript is attempting "+
						"to use version: %v", controlBlock.LeafVersion)
					return scriptError(
						ErrDiscourageUpgradeableTaprootVersion, errStr,
					)
				default:
					// If the policy flag isn't active,
					// then execution succeeds here as we
					// don't know the rules of the future
					// leaf versions.
					vm.taprootCtx.mustSucceed = true
					return nil
				}
			}

			// Now that we know we don't have any op success
			// fields, ensure that the script parses properly.
			//
			// TODO(roasbeef): combine w/ the above?
			err = checkScriptParses(vm.version, witnessScript)
			if err != nil {
				return err
			}

			// Now that we know the script parses, and we have a
			// valid leaf version, we'll save the tapscript hash of
			// the leaf, as we need that for signature validation
			// later.
			vm.taprootCtx.tapLeafHash = NewBaseTapLeaf(
				witnessScript,
			).TapHash()

			// Otherwise, we'll now "recurse" one level deeper, and
			// set the remaining witness (leaving off the annex and
			// the witness script) as the execution stack, and
			// enter further execution.
			vm.scripts = append(vm.scripts, witnessScript)
			vm.SetStack(witness[:len(witness)-2])
		}

	case vm.hasFlag(ScriptVerifyDiscourageUpgradeableWitnessProgram):
		errStr := fmt.Sprintf("new witness program versions "+
			"invalid: %v", vm.witnessProgram)

		return scriptError(ErrDiscourageUpgradableWitnessProgram, errStr)
	default:
		// If we encounter an unknown witness program version and we
		// aren't discouraging future unknown witness based soft-forks,
		// then we de-activate the segwit behavior within the VM for
		// the remainder of execution.
		vm.witnessProgram = nil
	}

	// TODO(roasbeef): other sanity checks here
	switch {

	// In addition to the normal script element size limits, taproot also
	// enforces a limit on the max _starting_ stack size.
	case vm.isWitnessVersionActive(TaprootWitnessVersion):
		if vm.dstack.Depth() > MaxStackSize {
			str := fmt.Sprintf("tapscript stack size %d > max allowed %d",
				vm.dstack.Depth(), MaxStackSize)
			return scriptError(ErrStackOverflow, str)
		}

		fallthrough
	case vm.isWitnessVersionActive(BaseSegwitWitnessVersion):
		// All elements within the witness stack must not be greater
		// than the maximum bytes which are allowed to be pushed onto
		// the stack.
		for _, witElement := range vm.GetStack() {
			if len(witElement) > MaxScriptElementSize {
				str := fmt.Sprintf("element size %d exceeds "+
					"max allowed size %d", len(witElement),
					MaxScriptElementSize)
				return scriptError(ErrElementTooBig, str)
			}
		}

		return nil
	}

	return nil
}

// DisasmPC returns the string for the disassembly of the opcode that will be
// next to execute when Step is called.
func (vm *Engine) DisasmPC() (string, error) {
	if err := vm.checkValidPC(); err != nil {
		return "", err
	}

	// Create a copy of the current tokenizer and parse the next opcode in the
	// copy to avoid mutating the current one.
	peekTokenizer := vm.tokenizer
	if !peekTokenizer.Next() {
		// Note that due to the fact that all scripts are checked for parse
		// failures before this code ever runs, there should never be an error
		// here, but check again to be safe in case a refactor breaks that
		// assumption or new script versions are introduced with different
		// semantics.
		if err := peekTokenizer.Err(); err != nil {
			return "", err
		}

		// Note that this should be impossible to hit in practice because the
		// only way it could happen would be for the final opcode of a script to
		// already be parsed without the script index having been updated, which
		// is not the case since stepping the script always increments the
		// script index when parsing and executing the final opcode of a script.
		//
		// However, check again to be safe in case a refactor breaks that
		// assumption or new script versions are introduced with different
		// semantics.
		str := fmt.Sprintf("program counter beyond script index %d (bytes %x)",
			vm.scriptIdx, vm.scripts[vm.scriptIdx])
		return "", scriptError(ErrInvalidProgramCounter, str)
	}

	var buf strings.Builder
	disasmOpcode(&buf, peekTokenizer.op, peekTokenizer.Data(), false)
	return fmt.Sprintf("%02x:%04x: %s", vm.scriptIdx, vm.opcodeIdx,
		buf.String()), nil
}

// DisasmScript returns the disassembly string for the script at the requested
// offset index.  Index 0 is the signature script and 1 is the public key
// script.  In the case of pay-to-script-hash, index 2 is the redeem script once
// the execution has progressed far enough to have successfully verified script
// hash and thus add the script to the scripts to execute.
func (vm *Engine) DisasmScript(idx int) (string, error) {
	if idx >= len(vm.scripts) {
		str := fmt.Sprintf("script index %d >= total scripts %d", idx,
			len(vm.scripts))
		return "", scriptError(ErrInvalidIndex, str)
	}

	var disbuf strings.Builder
	script := vm.scripts[idx]
	tokenizer := MakeScriptTokenizer(vm.version, script)
	var opcodeIdx int
	for tokenizer.Next() {
		disbuf.WriteString(fmt.Sprintf("%02x:%04x: ", idx, opcodeIdx))
		disasmOpcode(&disbuf, tokenizer.op, tokenizer.Data(), false)
		disbuf.WriteByte('\n')
		opcodeIdx++
	}
	return disbuf.String(), tokenizer.Err()
}

// CheckErrorCondition returns nil if the running script has ended and was
// successful, leaving a a true boolean on the stack.  An error otherwise,
// including if the script has not finished.
func (vm *Engine) CheckErrorCondition(finalScript bool) error {
	if vm.taprootCtx != nil && vm.taprootCtx.mustSucceed {
		return nil
	}

	// Check execution is actually done by ensuring the script index is after
	// the final script in the array script.
	if vm.scriptIdx < len(vm.scripts) {
		return scriptError(ErrScriptUnfinished,
			"error check when script unfinished")
	}

	// If we're in version zero witness execution mode, and this was the
	// final script, then the stack MUST be clean in order to maintain
	// compatibility with BIP16.
	if finalScript && vm.isWitnessVersionActive(BaseSegwitWitnessVersion) &&
		vm.dstack.Depth() != 1 {
		return scriptError(ErrEvalFalse, "witness program must "+
			"have clean stack")
	}

	// The final script must end with exactly one data stack item when the
	// verify clean stack flag is set.  Otherwise, there must be at least one
	// data stack item in order to interpret it as a boolean.
	cleanStackActive := vm.hasFlag(ScriptVerifyCleanStack) || vm.taprootCtx != nil
	if finalScript && cleanStackActive && vm.dstack.Depth() != 1 {

		str := fmt.Sprintf("stack must contain exactly one item (contains %d)",
			vm.dstack.Depth())
		return scriptError(ErrCleanStack, str)
	} else if vm.dstack.Depth() < 1 {
		return scriptError(ErrEmptyStack,
			"stack empty at end of script execution")
	}

	v, err := vm.dstack.PopBool()
	if err != nil {
		return err
	}
	if !v {
		// Log interesting data.
		log.Tracef("%v", newLogClosure(func() string {
			var buf strings.Builder
			buf.WriteString("scripts failed:\n")
			for i := range vm.scripts {
				dis, _ := vm.DisasmScript(i)
				buf.WriteString(fmt.Sprintf("script%d:\n", i))
				buf.WriteString(dis)
			}
			return buf.String()
		}))
		return scriptError(ErrEvalFalse,
			"false stack entry at end of script execution")
	}
	return nil
}

// Step executes the next instruction and moves the program counter to the next
// opcode in the script, or the next script if the current has ended.  Step will
// return true in the case that the last opcode was successfully executed.
//
// The result of calling Step or any other method is undefined if an error is
// returned.
func (vm *Engine) Step() (done bool, err error) {
	// Verify the engine is pointing to a valid program counter.
	if err := vm.checkValidPC(); err != nil {
		return true, err
	}

	// Attempt to parse the next opcode from the current script.
	if !vm.tokenizer.Next() {
		// Note that due to the fact that all scripts are checked for parse
		// failures before this code ever runs, there should never be an error
		// here, but check again to be safe in case a refactor breaks that
		// assumption or new script versions are introduced with different
		// semantics.
		if err := vm.tokenizer.Err(); err != nil {
			return false, err
		}

		str := fmt.Sprintf("attempt to step beyond script index %d (bytes %x)",
			vm.scriptIdx, vm.scripts[vm.scriptIdx])
		return true, scriptError(ErrInvalidProgramCounter, str)
	}

	// Execute the opcode while taking into account several things such as
	// disabled opcodes, illegal opcodes, maximum allowed operations per script,
	// maximum script element sizes, and conditionals.
	err = vm.executeOpcode(vm.tokenizer.op, vm.tokenizer.Data())
	if err != nil {
		return true, err
	}

	// The number of elements in the combination of the data and alt stacks
	// must not exceed the maximum number of stack elements allowed.
	combinedStackSize := vm.dstack.Depth() + vm.astack.Depth()
	if combinedStackSize > MaxStackSize {
		str := fmt.Sprintf("combined stack size %d > max allowed %d",
			combinedStackSize, MaxStackSize)
		return false, scriptError(ErrStackOverflow, str)
	}

	// Prepare for next instruction.
	vm.opcodeIdx++
	if vm.tokenizer.Done() {
		// Illegal to have a conditional that straddles two scripts.
		if len(vm.condStack) != 0 {
			return false, scriptError(ErrUnbalancedConditional,
				"end of script reached in conditional execution")
		}

		// Alt stack doesn't persist between scripts.
		_ = vm.astack.DropN(vm.astack.Depth())

		// The number of operations is per script.
		vm.numOps = 0

		// Reset the opcode index for the next script.
		vm.opcodeIdx = 0

		// Advance to the next script as needed.
		switch {
		case vm.scriptIdx == 0 && vm.bip16:
			vm.scriptIdx++
			vm.savedFirstStack = vm.GetStack()

		case vm.scriptIdx == 1 && vm.bip16:
			// Put us past the end for CheckErrorCondition()
			vm.scriptIdx++

			// Check script ran successfully.
			err := vm.CheckErrorCondition(false)
			if err != nil {
				return false, err
			}

			// Obtain the redeem script from the first stack and ensure it
			// parses.
			script := vm.savedFirstStack[len(vm.savedFirstStack)-1]
			if err := checkScriptParses(vm.version, script); err != nil {
				return false, err
			}
			vm.scripts = append(vm.scripts, script)

			// Set stack to be the stack from first script minus the redeem
			// script itself
			vm.SetStack(vm.savedFirstStack[:len(vm.savedFirstStack)-1])

		case vm.scriptIdx == 1 && vm.witnessProgram != nil,
			vm.scriptIdx == 2 && vm.witnessProgram != nil && vm.bip16: // np2sh

			vm.scriptIdx++

			witness := vm.tx.TxIn[vm.txIdx].Witness
			if err := vm.verifyWitnessProgram(witness); err != nil {
				return false, err
			}

		default:
			vm.scriptIdx++
		}

		// Skip empty scripts.
		if vm.scriptIdx < len(vm.scripts) && len(vm.scripts[vm.scriptIdx]) == 0 {
			vm.scriptIdx++
		}

		vm.lastCodeSep = 0
		if vm.scriptIdx >= len(vm.scripts) {
			return true, nil
		}

		// Finally, update the current tokenizer used to parse through scripts
		// one opcode at a time to start from the beginning of the new script
		// associated with the program counter.
		vm.tokenizer = MakeScriptTokenizer(vm.version, vm.scripts[vm.scriptIdx])
	}

	return false, nil
}

// copyStack makes a deep copy of the provided slice.
func copyStack(stk [][]byte) [][]byte {
	c := make([][]byte, len(stk))
	for i := range stk {
		c[i] = make([]byte, len(stk[i]))
		copy(c[i][:], stk[i][:])
	}

	return c
}

// Execute will execute all scripts in the script engine and return either nil
// for successful validation or an error if one occurred.
func (vm *Engine) Execute() (err error) {
	// All script versions other than 0 currently execute without issue,
	// making all outputs to them anyone can pay. In the future this
	// will allow for the addition of new scripting languages.
	if vm.version != 0 {
		return nil
	}

	// If the stepCallback is set, we start by making a call back with the
	// initial engine state.
	var stepInfo *StepInfo
	if vm.stepCallback != nil {
		stepInfo = &StepInfo{
			ScriptIndex: vm.scriptIdx,
			OpcodeIndex: vm.opcodeIdx,
			Stack:       copyStack(vm.dstack.stk),
			AltStack:    copyStack(vm.astack.stk),
		}
		err := vm.stepCallback(stepInfo)
		if err != nil {
			return err
		}
	}

	done := false
	for !done {
		log.Tracef("%v", newLogClosure(func() string {
			dis, err := vm.DisasmPC()
			if err != nil {
				return fmt.Sprintf("stepping - failed to disasm pc: %v", err)
			}
			return fmt.Sprintf("stepping %v", dis)
		}))

		done, err = vm.Step()
		if err != nil {
			return err
		}
		log.Tracef("%v", newLogClosure(func() string {
			var dstr, astr string

			// Log the non-empty stacks when tracing.
			if vm.dstack.Depth() != 0 {
				dstr = "Stack:\n" + vm.dstack.String()
			}
			if vm.astack.Depth() != 0 {
				astr = "AltStack:\n" + vm.astack.String()
			}

			return dstr + astr
		}))

		if vm.stepCallback != nil {
			scriptIdx := vm.scriptIdx
			opcodeIdx := vm.opcodeIdx

			// In case the execution has completed, we keep the
			// current script index while increasing the opcode
			// index. This is to indicate that no new script is
			// being executed.
			if done {
				scriptIdx = stepInfo.ScriptIndex
				opcodeIdx = stepInfo.OpcodeIndex + 1
			}

			stepInfo = &StepInfo{
				ScriptIndex: scriptIdx,
				OpcodeIndex: opcodeIdx,
				Stack:       copyStack(vm.dstack.stk),
				AltStack:    copyStack(vm.astack.stk),
			}
			err := vm.stepCallback(stepInfo)
			if err != nil {
				return err
			}
		}
	}

	return vm.CheckErrorCondition(true)
}

// subScript returns the script since the last OP_CODESEPARATOR.
func (vm *Engine) subScript() []byte {
	return vm.scripts[vm.scriptIdx][vm.lastCodeSep:]
}

// checkHashTypeEncoding returns whether or not the passed hashtype adheres to
// the strict encoding requirements if enabled.
func (vm *Engine) checkHashTypeEncoding(hashType SigHashType) error {
	if !vm.hasFlag(ScriptVerifyStrictEncoding) {
		return nil
	}

	sigHashType := hashType & ^SigHashAnyOneCanPay
	if sigHashType < SigHashAll || sigHashType > SigHashSingle {
		str := fmt.Sprintf("invalid hash type 0x%x", hashType)
		return scriptError(ErrInvalidSigHashType, str)
	}
	return nil
}

// isStrictPubKeyEncoding returns whether or not the passed public key adheres
// to the strict encoding requirements.
func isStrictPubKeyEncoding(pubKey []byte) bool {
	if len(pubKey) == 33 && (pubKey[0] == 0x02 || pubKey[0] == 0x03) {
		// Compressed
		return true
	}
	if len(pubKey) == 65 {
		switch pubKey[0] {
		case 0x04:
			// Uncompressed
			return true

		case 0x06, 0x07:
			// Hybrid
			return true
		}
	}
	return false
}

// checkPubKeyEncoding returns whether or not the passed public key adheres to
// the strict encoding requirements if enabled.
func (vm *Engine) checkPubKeyEncoding(pubKey []byte) error {
	if vm.hasFlag(ScriptVerifyWitnessPubKeyType) &&
		vm.isWitnessVersionActive(BaseSegwitWitnessVersion) &&
		!btcec.IsCompressedPubKey(pubKey) {

		str := "only compressed keys are accepted post-segwit"
		return scriptError(ErrWitnessPubKeyType, str)
	}

	if !vm.hasFlag(ScriptVerifyStrictEncoding) {
		return nil
	}

	if len(pubKey) == 33 && (pubKey[0] == 0x02 || pubKey[0] == 0x03) {
		// Compressed
		return nil
	}
	if len(pubKey) == 65 && pubKey[0] == 0x04 {
		// Uncompressed
		return nil
	}

	return scriptError(ErrPubKeyType, "unsupported public key type")
}

// checkSignatureEncoding returns whether or not the passed signature adheres to
// the strict encoding requirements if enabled.
func (vm *Engine) checkSignatureEncoding(sig []byte) error {
	if !vm.hasFlag(ScriptVerifyDERSignatures) &&
		!vm.hasFlag(ScriptVerifyLowS) &&
		!vm.hasFlag(ScriptVerifyStrictEncoding) {

		return nil
	}

	// The format of a DER encoded signature is as follows:
	//
	// 0x30 <total length> 0x02 <length of R> <R> 0x02 <length of S> <S>
	//   - 0x30 is the ASN.1 identifier for a sequence
	//   - Total length is 1 byte and specifies length of all remaining data
	//   - 0x02 is the ASN.1 identifier that specifies an integer follows
	//   - Length of R is 1 byte and specifies how many bytes R occupies
	//   - R is the arbitrary length big-endian encoded number which
	//     represents the R value of the signature.  DER encoding dictates
	//     that the value must be encoded using the minimum possible number
	//     of bytes.  This implies the first byte can only be null if the
	//     highest bit of the next byte is set in order to prevent it from
	//     being interpreted as a negative number.
	//   - 0x02 is once again the ASN.1 integer identifier
	//   - Length of S is 1 byte and specifies how many bytes S occupies
	//   - S is the arbitrary length big-endian encoded number which
	//     represents the S value of the signature.  The encoding rules are
	//     identical as those for R.
	const (
		asn1SequenceID = 0x30
		asn1IntegerID  = 0x02

		// minSigLen is the minimum length of a DER encoded signature and is
		// when both R and S are 1 byte each.
		//
		// 0x30 + <1-byte> + 0x02 + 0x01 + <byte> + 0x2 + 0x01 + <byte>
		minSigLen = 8

		// maxSigLen is the maximum length of a DER encoded signature and is
		// when both R and S are 33 bytes each.  It is 33 bytes because a
		// 256-bit integer requires 32 bytes and an additional leading null byte
		// might required if the high bit is set in the value.
		//
		// 0x30 + <1-byte> + 0x02 + 0x21 + <33 bytes> + 0x2 + 0x21 + <33 bytes>
		maxSigLen = 72

		// sequenceOffset is the byte offset within the signature of the
		// expected ASN.1 sequence identifier.
		sequenceOffset = 0

		// dataLenOffset is the byte offset within the signature of the expected
		// total length of all remaining data in the signature.
		dataLenOffset = 1

		// rTypeOffset is the byte offset within the signature of the ASN.1
		// identifier for R and is expected to indicate an ASN.1 integer.
		rTypeOffset = 2

		// rLenOffset is the byte offset within the signature of the length of
		// R.
		rLenOffset = 3

		// rOffset is the byte offset within the signature of R.
		rOffset = 4
	)

	// The signature must adhere to the minimum and maximum allowed length.
	sigLen := len(sig)
	if sigLen < minSigLen {
		str := fmt.Sprintf("malformed signature: too short: %d < %d", sigLen,
			minSigLen)
		return scriptError(ErrSigTooShort, str)
	}
	if sigLen > maxSigLen {
		str := fmt.Sprintf("malformed signature: too long: %d > %d", sigLen,
			maxSigLen)
		return scriptError(ErrSigTooLong, str)
	}

	// The signature must start with the ASN.1 sequence identifier.
	if sig[sequenceOffset] != asn1SequenceID {
		str := fmt.Sprintf("malformed signature: format has wrong type: %#x",
			sig[sequenceOffset])
		return scriptError(ErrSigInvalidSeqID, str)
	}

	// The signature must indicate the correct amount of data for all elements
	// related to R and S.
	if int(sig[dataLenOffset]) != sigLen-2 {
		str := fmt.Sprintf("malformed signature: bad length: %d != %d",
			sig[dataLenOffset], sigLen-2)
		return scriptError(ErrSigInvalidDataLen, str)
	}

	// Calculate the offsets of the elements related to S and ensure S is inside
	// the signature.
	//
	// rLen specifies the length of the big-endian encoded number which
	// represents the R value of the signature.
	//
	// sTypeOffset is the offset of the ASN.1 identifier for S and, like its R
	// counterpart, is expected to indicate an ASN.1 integer.
	//
	// sLenOffset and sOffset are the byte offsets within the signature of the
	// length of S and S itself, respectively.
	rLen := int(sig[rLenOffset])
	sTypeOffset := rOffset + rLen
	sLenOffset := sTypeOffset + 1
	if sTypeOffset >= sigLen {
		str := "malformed signature: S type indicator missing"
		return scriptError(ErrSigMissingSTypeID, str)
	}
	if sLenOffset >= sigLen {
		str := "malformed signature: S length missing"
		return scriptError(ErrSigMissingSLen, str)
	}

	// The lengths of R and S must match the overall length of the signature.
	//
	// sLen specifies the length of the big-endian encoded number which
	// represents the S value of the signature.
	sOffset := sLenOffset + 1
	sLen := int(sig[sLenOffset])
	if sOffset+sLen != sigLen {
		str := "malformed signature: invalid S length"
		return scriptError(ErrSigInvalidSLen, str)
	}

	// R elements must be ASN.1 integers.
	if sig[rTypeOffset] != asn1IntegerID {
		str := fmt.Sprintf("malformed signature: R integer marker: %#x != %#x",
			sig[rTypeOffset], asn1IntegerID)
		return scriptError(ErrSigInvalidRIntID, str)
	}

	// Zero-length integers are not allowed for R.
	if rLen == 0 {
		str := "malformed signature: R length is zero"
		return scriptError(ErrSigZeroRLen, str)
	}

	// R must not be negative.
	if sig[rOffset]&0x80 != 0 {
		str := "malformed signature: R is negative"
		return scriptError(ErrSigNegativeR, str)
	}

	// Null bytes at the start of R are not allowed, unless R would otherwise be
	// interpreted as a negative number.
	if rLen > 1 && sig[rOffset] == 0x00 && sig[rOffset+1]&0x80 == 0 {
		str := "malformed signature: R value has too much padding"
		return scriptError(ErrSigTooMuchRPadding, str)
	}

	// S elements must be ASN.1 integers.
	if sig[sTypeOffset] != asn1IntegerID {
		str := fmt.Sprintf("malformed signature: S integer marker: %#x != %#x",
			sig[sTypeOffset], asn1IntegerID)
		return scriptError(ErrSigInvalidSIntID, str)
	}

	// Zero-length integers are not allowed for S.
	if sLen == 0 {
		str := "malformed signature: S length is zero"
		return scriptError(ErrSigZeroSLen, str)
	}

	// S must not be negative.
	if sig[sOffset]&0x80 != 0 {
		str := "malformed signature: S is negative"
		return scriptError(ErrSigNegativeS, str)
	}

	// Null bytes at the start of S are not allowed, unless S would otherwise be
	// interpreted as a negative number.
	if sLen > 1 && sig[sOffset] == 0x00 && sig[sOffset+1]&0x80 == 0 {
		str := "malformed signature: S value has too much padding"
		return scriptError(ErrSigTooMuchSPadding, str)
	}

	// Verify the S value is <= half the order of the curve.  This check is done
	// because when it is higher, the complement modulo the order can be used
	// instead which is a shorter encoding by 1 byte.  Further, without
	// enforcing this, it is possible to replace a signature in a valid
	// transaction with the complement while still being a valid signature that
	// verifies.  This would result in changing the transaction hash and thus is
	// a source of malleability.
	if vm.hasFlag(ScriptVerifyLowS) {
		sValue := new(big.Int).SetBytes(sig[sOffset : sOffset+sLen])
		if sValue.Cmp(halfOrder) > 0 {
			return scriptError(ErrSigHighS, "signature is not canonical due "+
				"to unnecessarily high S value")
		}
	}

	return nil
}

// getStack returns the contents of stack as a byte array bottom up
func getStack(stack *stack) [][]byte {
	array := make([][]byte, stack.Depth())
	for i := range array {
		// PeekByteArry can't fail due to overflow, already checked
		array[len(array)-i-1], _ = stack.PeekByteArray(int32(i))
	}
	return array
}

// setStack sets the stack to the contents of the array where the last item in
// the array is the top item in the stack.
func setStack(stack *stack, data [][]byte) {
	// This can not error. Only errors are for invalid arguments.
	_ = stack.DropN(stack.Depth())

	for i := range data {
		stack.PushByteArray(data[i])
	}
}

// GetStack returns the contents of the primary stack as an array. where the
// last item in the array is the top of the stack.
func (vm *Engine) GetStack() [][]byte {
	return getStack(&vm.dstack)
}

// SetStack sets the contents of the primary stack to the contents of the
// provided array where the last item in the array will be the top of the stack.
func (vm *Engine) SetStack(data [][]byte) {
	setStack(&vm.dstack, data)
}

// GetAltStack returns the contents of the alternate stack as an array where the
// last item in the array is the top of the stack.
func (vm *Engine) GetAltStack() [][]byte {
	return getStack(&vm.astack)
}

// SetAltStack sets the contents of the alternate stack to the contents of the
// provided array where the last item in the array will be the top of the stack.
func (vm *Engine) SetAltStack(data [][]byte) {
	setStack(&vm.astack, data)
}

// NewEngine returns a new script engine for the provided public key script,
// transaction, and input index.  The flags modify the behavior of the script
// engine according to the description provided by each flag.
func NewEngine(scriptPubKey []byte, tx *wire.MsgTx, txIdx int, flags ScriptFlags,
	sigCache *SigCache, hashCache *TxSigHashes, inputAmount int64,
	prevOutFetcher PrevOutputFetcher) (*Engine, error) {

	const scriptVersion = 0

	// The provided transaction input index must refer to a valid input.
	if txIdx < 0 || txIdx >= len(tx.TxIn) {
		str := fmt.Sprintf("transaction input index %d is negative or "+
			">= %d", txIdx, len(tx.TxIn))
		return nil, scriptError(ErrInvalidIndex, str)
	}
	scriptSig := tx.TxIn[txIdx].SignatureScript

	// When both the signature script and public key script are empty the result
	// is necessarily an error since the stack would end up being empty which is
	// equivalent to a false top element.  Thus, just return the relevant error
	// now as an optimization.
	if len(scriptSig) == 0 && len(scriptPubKey) == 0 {
		return nil, scriptError(ErrEvalFalse,
			"false stack entry at end of script execution")
	}

	// The clean stack flag (ScriptVerifyCleanStack) is not allowed without
	// either the pay-to-script-hash (P2SH) evaluation (ScriptBip16)
	// flag or the Segregated Witness (ScriptVerifyWitness) flag.
	//
	// Recall that evaluating a P2SH script without the flag set results in
	// non-P2SH evaluation which leaves the P2SH inputs on the stack.
	// Thus, allowing the clean stack flag without the P2SH flag would make
	// it possible to have a situation where P2SH would not be a soft fork
	// when it should be. The same goes for segwit which will pull in
	// additional scripts for execution from the witness stack.
	vm := Engine{
		flags:          flags,
		sigCache:       sigCache,
		hashCache:      hashCache,
		inputAmount:    inputAmount,
		prevOutFetcher: prevOutFetcher,
	}
	if vm.hasFlag(ScriptVerifyCleanStack) && (!vm.hasFlag(ScriptBip16) &&
		!vm.hasFlag(ScriptVerifyWitness)) {
		return nil, scriptError(ErrInvalidFlags,
			"invalid flags combination")
	}

	// The signature script must only contain data pushes when the
	// associated flag is set.
	if vm.hasFlag(ScriptVerifySigPushOnly) && !IsPushOnlyScript(scriptSig) {
		return nil, scriptError(ErrNotPushOnly,
			"signature script is not push only")
	}

	// The signature script must only contain data pushes for PS2H which is
	// determined based on the form of the public key script.
	if vm.hasFlag(ScriptBip16) && isScriptHashScript(scriptPubKey) {
		// Only accept input scripts that push data for P2SH.
		// Notice that the push only checks have already been done when
		// the flag to verify signature scripts are push only is set
		// above, so avoid checking again.
		alreadyChecked := vm.hasFlag(ScriptVerifySigPushOnly)
		if !alreadyChecked && !IsPushOnlyScript(scriptSig) {
			return nil, scriptError(ErrNotPushOnly,
				"pay to script hash is not push only")
		}
		vm.bip16 = true
	}

	// The engine stores the scripts using a slice.  This allows multiple
	// scripts to be executed in sequence.  For example, with a
	// pay-to-script-hash transaction, there will be ultimately be a third
	// script to execute.
	scripts := [][]byte{scriptSig, scriptPubKey}
	for _, scr := range scripts {
		if len(scr) > MaxScriptSize {
			str := fmt.Sprintf("script size %d is larger than max allowed "+
				"size %d", len(scr), MaxScriptSize)
			return nil, scriptError(ErrScriptTooBig, str)
		}

		const scriptVersion = 0
		if err := checkScriptParses(scriptVersion, scr); err != nil {
			return nil, err
		}
	}
	vm.scripts = scripts

	// Advance the program counter to the public key script if the signature
	// script is empty since there is nothing to execute for it in that case.
	if len(scriptSig) == 0 {
		vm.scriptIdx++
	}
	if vm.hasFlag(ScriptVerifyMinimalData) {
		vm.dstack.verifyMinimalData = true
		vm.astack.verifyMinimalData = true
	}

	// Check to see if we should execute in witness verification mode
	// according to the set flags. We check both the pkScript, and sigScript
	// here since in the case of nested p2sh, the scriptSig will be a valid
	// witness program. For nested p2sh, all the bytes after the first data
	// push should *exactly* match the witness program template.
	if vm.hasFlag(ScriptVerifyWitness) {
		// If witness evaluation is enabled, then P2SH MUST also be
		// active.
		if !vm.hasFlag(ScriptBip16) {
			errStr := "P2SH must be enabled to do witness verification"
			return nil, scriptError(ErrInvalidFlags, errStr)
		}

		var witProgram []byte

		switch {
		case IsWitnessProgram(vm.scripts[1]):
			// The scriptSig must be *empty* for all native witness
			// programs, otherwise we introduce malleability.
			if len(scriptSig) != 0 {
				errStr := "native witness program cannot " +
					"also have a signature script"
				return nil, scriptError(ErrWitnessMalleated, errStr)
			}

			witProgram = scriptPubKey
		case len(tx.TxIn[txIdx].Witness) != 0 && vm.bip16:
			// The sigScript MUST be *exactly* a single canonical
			// data push of the witness program, otherwise we
			// reintroduce malleability.
			sigPops := vm.scripts[0]
			if len(sigPops) > 2 &&
				isCanonicalPush(sigPops[0], sigPops[1:]) &&
				IsWitnessProgram(sigPops[1:]) {

				witProgram = sigPops[1:]
			} else {
				errStr := "signature script for witness " +
					"nested p2sh is not canonical"
				return nil, scriptError(ErrWitnessMalleatedP2SH, errStr)
			}
		}

		if witProgram != nil {
			var err error
			vm.witnessVersion, vm.witnessProgram, err = ExtractWitnessProgramInfo(
				witProgram,
			)
			if err != nil {
				return nil, err
			}
		} else {
			// If we didn't find a witness program in either the
			// pkScript or as a datapush within the sigScript, then
			// there MUST NOT be any witness data associated with
			// the input being validated.
			if vm.witnessProgram == nil && len(tx.TxIn[txIdx].Witness) != 0 {
				errStr := "non-witness inputs cannot have a witness"
				return nil, scriptError(ErrWitnessUnexpected, errStr)
			}
		}

	}

	// Setup the current tokenizer used to parse through the script one opcode
	// at a time with the script associated with the program counter.
	vm.tokenizer = MakeScriptTokenizer(scriptVersion, scripts[vm.scriptIdx])

	vm.tx = *tx
	vm.txIdx = txIdx

	return &vm, nil
}

// NewEngine returns a new script engine with a script execution callback set.
// This is useful for debugging script execution.
func NewDebugEngine(scriptPubKey []byte, tx *wire.MsgTx, txIdx int,
	flags ScriptFlags, sigCache *SigCache, hashCache *TxSigHashes,
	inputAmount int64, prevOutFetcher PrevOutputFetcher,
	stepCallback func(*StepInfo) error) (*Engine, error) {

	vm, err := NewEngine(
		scriptPubKey, tx, txIdx, flags, sigCache, hashCache,
		inputAmount, prevOutFetcher,
	)
	if err != nil {
		return nil, err
	}

	vm.stepCallback = stepCallback
	return vm, nil
}
