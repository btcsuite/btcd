// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"fmt"
	"math/big"

	"github.com/decred/dcrd/chaincfg/chainec"
	"github.com/decred/dcrd/wire"
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
	// currently unused opcodes in the NOP and UNKNOWN families are reserved
	// for future upgrades.  This flag must not be used for consensus
	// critical code nor applied to blocks as this flag is only for stricter
	// standard transaction checks.  This flag is only applied when the
	// above opcodes are executed.
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
	// This flag should never be used without the ScriptBip16 flag.
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

	// ScriptVerifySigPushOnly defines that signature scripts must contain
	// only pushed data.  This is rule 2 of BIP0062.
	ScriptVerifySigPushOnly

	// ScriptVerifyStrictEncoding defines that signature scripts and
	// public keys must follow the strict encoding requirements.
	ScriptVerifyStrictEncoding

	// ScriptVerifySHA256 defines whether to treat opcode 192 (previously
	// OP_UNKNOWN192) as the OP_SHA256 opcode which consumes the top item of
	// the data stack and replaces it with the sha256 of it.
	ScriptVerifySHA256
)

const (
	// maxStackSize is the maximum combined height of stack and alt stack
	// during execution.
	maxStackSize = 1024

	// maxScriptSize is the maximum allowed length of a raw script.
	maxScriptSize = 16384

	// DefaultScriptVersion is the default scripting language version
	// representing extended Decred script.
	DefaultScriptVersion = uint16(0)
)

// halforder is used to tame ECDSA malleability (see BIP0062).
var halfOrder = new(big.Int).Rsh(chainec.Secp256k1.GetN(), 1)

// Engine is the virtual machine that executes scripts.
type Engine struct {
	scripts         [][]parsedOpcode
	savedFirstStack [][]byte // stack from first script for bip16 scripts
	sigCache        *SigCache

	scriptIdx   int
	scriptOff   int
	lastCodeSep int
	dstack      stack // data stack
	astack      stack // alt stack
	tx          wire.MsgTx
	txIdx       int
	condStack   []int
	numOps      int
	flags       ScriptFlags
	version     uint16
	bip16       bool // treat execution as pay-to-script-hash
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

// executeOpcode peforms execution on the passed opcode.  It takes into account
// whether or not it is hidden by conditionals, but some rules still must be
// tested in this case.
func (vm *Engine) executeOpcode(pop *parsedOpcode) error {
	// Disabled opcodes are fail on program counter.
	if pop.isDisabled() {
		return ErrStackOpDisabled
	}

	// Always-illegal opcodes are fail on program counter.
	if pop.alwaysIllegal() {
		return ErrStackReservedOpcode
	}

	// Note that this includes OP_RESERVED which counts as a push operation.
	if pop.opcode.value > OP_16 {
		vm.numOps++
		if vm.numOps > MaxOpsPerScript {
			return ErrStackTooManyOperations
		}

	} else if len(pop.data) > MaxScriptElementSize {
		return ErrStackElementTooBig
	}

	// Nothing left to do when this is not a conditional opcode and it is
	// not in an executing branch.
	if !vm.isBranchExecuting() && !pop.isConditional() {
		return nil
	}

	// Ensure all executed data push opcodes use the minimal encoding when
	// the minimal data verification flag is set.
	if vm.dstack.verifyMinimalData && vm.isBranchExecuting() &&
		pop.opcode.value >= 0 && pop.opcode.value <= OP_PUSHDATA4 {

		if err := pop.checkMinimalDataPush(); err != nil {
			return err
		}
	}

	return pop.opcode.opfunc(pop, vm)
}

// disasm is a helper function to produce the output for DisasmPC and
// DisasmScript.  It produces the opcode prefixed by the program counter at the
// provided position in the script.  It does no error checking and leaves that
// to the caller to provide a valid offset.
func (vm *Engine) disasm(scriptIdx int, scriptOff int) string {
	return fmt.Sprintf("%02x:%04x: %s", scriptIdx, scriptOff,
		vm.scripts[scriptIdx][scriptOff].print(false))
}

// validPC returns an error if the current script position is valid for
// execution, nil otherwise.
func (vm *Engine) validPC() error {
	if vm.scriptIdx >= len(vm.scripts) {
		return fmt.Errorf("past input scripts %v:%v %v:xxxx",
			vm.scriptIdx, vm.scriptOff, len(vm.scripts))
	}
	if vm.scriptOff >= len(vm.scripts[vm.scriptIdx]) {
		return fmt.Errorf("past input scripts %v:%v %v:%04d",
			vm.scriptIdx, vm.scriptOff, vm.scriptIdx,
			len(vm.scripts[vm.scriptIdx]))
	}
	return nil
}

// curPC returns either the current script and offset, or an error if the
// position isn't valid.
func (vm *Engine) curPC() (script int, off int, err error) {
	err = vm.validPC()
	if err != nil {
		return 0, 0, err
	}
	return vm.scriptIdx, vm.scriptOff, nil
}

// DisasmPC returns the string for the disassembly of the opcode that will be
// next to execute when Step() is called.
func (vm *Engine) DisasmPC() (string, error) {
	scriptIdx, scriptOff, err := vm.curPC()
	if err != nil {
		return "", err
	}
	return vm.disasm(scriptIdx, scriptOff), nil
}

// DisasmScript returns the disassembly string for the script at the requested
// offset index.  Index 0 is the signature script and 1 is the public key
// script.
func (vm *Engine) DisasmScript(idx int) (string, error) {
	if idx >= len(vm.scripts) {
		return "", ErrStackInvalidIndex
	}

	var disstr string
	for i := range vm.scripts[idx] {
		disstr = disstr + vm.disasm(idx, i) + "\n"
	}
	return disstr, nil
}

// CheckErrorCondition returns nil if the running script has ended and was
// successful, leaving a a true boolean on the stack.  An error otherwise,
// including if the script has not finished.
func (vm *Engine) CheckErrorCondition(finalScript bool) error {
	// Check execution is actually done.  When pc is past the end of script
	// array there are no more scripts to run.
	if vm.scriptIdx < len(vm.scripts) {
		return ErrStackScriptUnfinished
	}
	if finalScript && vm.hasFlag(ScriptVerifyCleanStack) &&
		vm.dstack.Depth() != 1 {
		return ErrStackCleanStack
	} else if vm.dstack.Depth() < 1 {
		return ErrStackEmptyStack
	}

	v, err := vm.dstack.PopBool()
	if err != nil {
		return err
	}
	if !v {
		// Log interesting data.
		log.Tracef("%v", newLogClosure(func() string {
			dis0, _ := vm.DisasmScript(0)
			dis1, _ := vm.DisasmScript(1)
			return fmt.Sprintf("scripts failed: script0: %s\n"+
				"script1: %s", dis0, dis1)
		}))
		return ErrStackScriptFailed
	}
	return nil
}

// Step will execute the next instruction and move the program counter to the
// next opcode in the script, or the next script if the current has ended.  Step
// will return true in the case that the last opcode was successfully executed.
//
// The result of calling Step or any other method is undefined if an error is
// returned.
func (vm *Engine) Step() (done bool, err error) {
	// Verify that it is pointing to a valid script address.
	err = vm.validPC()
	if err != nil {
		return true, err
	}
	opcode := &vm.scripts[vm.scriptIdx][vm.scriptOff]

	// Execute the opcode while taking into account several things such as
	// disabled opcodes, illegal opcodes, maximum allowed operations per
	// script, maximum script element sizes, and conditionals.
	err = vm.executeOpcode(opcode)
	if err != nil {
		return true, err
	}

	// The number of elements in the combination of the data and alt stacks
	// must not exceed the maximum number of stack elements allowed.
	if vm.dstack.Depth()+vm.astack.Depth() > maxStackSize {
		return false, ErrStackOverflow
	}

	// Prepare for next instruction.
	vm.scriptOff++
	if vm.scriptOff >= len(vm.scripts[vm.scriptIdx]) {
		// Illegal to have an `if' that straddles two scripts.
		if err == nil && len(vm.condStack) != 0 {
			return false, ErrStackMissingEndif
		}

		// Alt stack doesn't persist.
		_ = vm.astack.DropN(vm.astack.Depth())

		vm.numOps = 0 // number of ops is per script.
		vm.scriptOff = 0
		if vm.scriptIdx == 0 && vm.bip16 {
			vm.scriptIdx++
			vm.savedFirstStack = vm.GetStack()
		} else if vm.scriptIdx == 1 && vm.bip16 {
			// Put us past the end for CheckErrorCondition()
			vm.scriptIdx++
			// Check script ran successfully and pull the script
			// out of the first stack and execute that.
			err := vm.CheckErrorCondition(false)
			if err != nil {
				return false, err
			}

			script := vm.savedFirstStack[len(vm.savedFirstStack)-1]
			pops, err := parseScript(script)
			if err != nil {
				return false, err
			}
			vm.scripts = append(vm.scripts, pops)

			// Set stack to be the stack from first script minus the
			// script itself
			vm.SetStack(vm.savedFirstStack[:len(vm.savedFirstStack)-1])
		} else {
			vm.scriptIdx++
		}
		// there are zero length scripts in the wild
		if vm.scriptIdx < len(vm.scripts) &&
			vm.scriptOff >= len(vm.scripts[vm.scriptIdx]) {
			vm.scriptIdx++
		}
		vm.lastCodeSep = 0
		if vm.scriptIdx >= len(vm.scripts) {
			return true, nil
		}
	}
	return false, nil
}

// Execute will execute all scripts in the script engine and return either nil
// for successful validation or an error if one occurred.
func (vm *Engine) Execute() (err error) {
	// All non-default version scripts currently execute without issue,
	// making all outputs to them anyone can pay. In the future this
	// will allow for the addition of new scripting languages.
	if vm.version != DefaultScriptVersion {
		return nil
	}

	done := false
	for !done {
		log.Tracef("%v", newLogClosure(func() string {
			dis, err := vm.DisasmPC()
			if err != nil {
				return fmt.Sprintf("stepping (%v)", err)
			}
			return fmt.Sprintf("stepping %v", dis)
		}))

		done, err = vm.Step()
		if err != nil {
			return err
		}
		log.Tracef("%v", newLogClosure(func() string {
			var dstr, astr string

			// if we're tracing, dump the stacks.
			if vm.dstack.Depth() != 0 {
				dstr = "Stack:\n" + vm.dstack.String()
			}
			if vm.astack.Depth() != 0 {
				astr = "AltStack:\n" + vm.astack.String()
			}

			return dstr + astr
		}))
	}

	return vm.CheckErrorCondition(true)
}

// subScript returns the script since the last OP_CODESEPARATOR.
func (vm *Engine) subScript() []parsedOpcode {
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
		return fmt.Errorf("invalid hashtype: 0x%x\n", hashType)
	}
	return nil
}

// checkPubKeyEncoding returns whether or not the passed public key adheres to
// the strict encoding requirements if enabled.
func (vm *Engine) checkPubKeyEncoding(pubKey []byte) error {
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
	return ErrStackInvalidPubKey
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

	// Minimum length is when both numbers are 1 byte each.
	// 0x30 + <1-byte> + 0x02 + 0x01 + <byte> + 0x2 + 0x01 + <byte>
	if len(sig) < 8 {
		// Too short
		return fmt.Errorf("malformed signature: too short: %d < 8",
			len(sig))
	}

	// Maximum length is when both numbers are 33 bytes each.  It is 33
	// bytes because a 256-bit integer requires 32 bytes and an additional
	// leading null byte might required if the high bit is set in the value.
	// 0x30 + <1-byte> + 0x02 + 0x21 + <33 bytes> + 0x2 + 0x21 + <33 bytes>
	if len(sig) > 72 {
		// Too long
		return fmt.Errorf("malformed signature: too long: %d > 72",
			len(sig))
	}
	if sig[0] != 0x30 {
		// Wrong type
		return fmt.Errorf("malformed signature: format has wrong type: 0x%x",
			sig[0])
	}
	if int(sig[1]) != len(sig)-2 {
		// Invalid length
		return fmt.Errorf("malformed signature: bad length: %d != %d",
			sig[1], len(sig)-2)
	}

	rLen := int(sig[3])

	// Make sure S is inside the signature.
	if rLen+5 > len(sig) {
		return fmt.Errorf("malformed signature: S out of bounds")
	}

	sLen := int(sig[rLen+5])

	// The length of the elements does not match the length of the
	// signature.
	if rLen+sLen+6 != len(sig) {
		return fmt.Errorf("malformed signature: invalid R length")
	}

	// R elements must be integers.
	if sig[2] != 0x02 {
		return fmt.Errorf("malformed signature: missing first integer marker")
	}

	// Zero-length integers are not allowed for R.
	if rLen == 0 {
		return fmt.Errorf("malformed signature: R length is zero")
	}

	// R must not be negative.
	if sig[4]&0x80 != 0 {
		return fmt.Errorf("malformed signature: R value is negative")
	}

	// Null bytes at the start of R are not allowed, unless R would
	// otherwise be interpreted as a negative number.
	if rLen > 1 && sig[4] == 0x00 && sig[5]&0x80 == 0 {
		return fmt.Errorf("malformed signature: invalid R value")
	}

	// S elements must be integers.
	if sig[rLen+4] != 0x02 {
		return fmt.Errorf("malformed signature: missing second integer marker")
	}

	// Zero-length integers are not allowed for S.
	if sLen == 0 {
		return fmt.Errorf("malformed signature: S length is zero")
	}

	// S must not be negative.
	if sig[rLen+6]&0x80 != 0 {
		return fmt.Errorf("malformed signature: S value is negative")
	}

	// Null bytes at the start of S are not allowed, unless S would
	// otherwise be interpreted as a negative number.
	if sLen > 1 && sig[rLen+6] == 0x00 && sig[rLen+7]&0x80 == 0 {
		return fmt.Errorf("malformed signature: invalid S value")
	}

	// Verify the S value is <= half the order of the curve.  This check is
	// done because when it is higher, the complement modulo the order can
	// be used instead which is a shorter encoding by 1 byte.  Further,
	// without enforcing this, it is possible to replace a signature in a
	// valid transaction with the complement while still being a valid
	// signature that verifies.  This would result in changing the
	// transaction hash and thus is source of malleability.
	if vm.hasFlag(ScriptVerifyLowS) {
		sValue := new(big.Int).SetBytes(sig[rLen+6 : rLen+6+sLen])
		if sValue.Cmp(halfOrder) > 0 {
			return ErrStackInvalidLowSSignature
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
func NewEngine(scriptPubKey []byte, tx *wire.MsgTx, txIdx int,
	flags ScriptFlags, scriptVersion uint16, sigCache *SigCache) (*Engine, error) {

	// The provided transaction input index must refer to a valid input.
	if txIdx < 0 || txIdx >= len(tx.TxIn) {
		return nil, ErrInvalidIndex
	}
	scriptSig := tx.TxIn[txIdx].SignatureScript

	// The clean stack flag (ScriptVerifyCleanStack) is not allowed without
	// the pay-to-script-hash (P2SH) evaluation (ScriptBip16) flag.
	//
	// Recall that evaluating a P2SH script without the flag set results in
	// non-P2SH evaluation which leaves the P2SH inputs on the stack.  Thus,
	// allowing the clean stack flag without the P2SH flag would make it
	// possible to have a situation where P2SH would not be a soft fork when
	// it should be.
	vm := Engine{version: scriptVersion, flags: flags, sigCache: sigCache}
	if vm.hasFlag(ScriptVerifyCleanStack) && !vm.hasFlag(ScriptBip16) {
		return nil, ErrInvalidFlags
	}

	// The signature script must only contain data pushes when the
	// associated flag is set.
	if vm.hasFlag(ScriptVerifySigPushOnly) && !IsPushOnlyScript(scriptSig) {
		return nil, ErrStackNonPushOnly
	}

	// Subscripts for pay to script hash outputs are not allowed
	// to use any stake tag OP codes if the script version is 0.
	if scriptVersion == DefaultScriptVersion {
		err := HasP2SHScriptSigStakeOpCodes(scriptVersion, scriptSig,
			scriptPubKey)
		if err != nil {
			return nil, err
		}
	}

	// The engine stores the scripts in parsed form using a slice.  This
	// allows multiple scripts to be executed in sequence.  For example,
	// with a pay-to-script-hash transaction, there will be ultimately be
	// a third script to execute.
	scripts := [][]byte{scriptSig, scriptPubKey}
	vm.scripts = make([][]parsedOpcode, len(scripts))
	for i, scr := range scripts {
		if len(scr) > maxScriptSize {
			return nil, ErrStackLongScript
		}
		var err error
		vm.scripts[i], err = parseScript(scr)
		if err != nil {
			return nil, err
		}
	}

	// Advance the program counter to the public key script if the signature
	// script is empty since there is nothing to execute for it in that
	// case.
	if len(scripts[0]) == 0 {
		vm.scriptIdx++
	}

	if vm.hasFlag(ScriptBip16) && isAnyKindOfScriptHash(vm.scripts[1]) {
		// Only accept input scripts that push data for P2SH.
		if !isPushOnly(vm.scripts[0]) {
			return nil, ErrStackP2SHNonPushOnly
		}
		vm.bip16 = true
	}
	if vm.hasFlag(ScriptVerifyMinimalData) {
		vm.dstack.verifyMinimalData = true
		vm.astack.verifyMinimalData = true
	}

	vm.tx = *tx
	vm.txIdx = txIdx

	return &vm, nil
}
