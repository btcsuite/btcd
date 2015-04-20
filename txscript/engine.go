// Copyright (c) 2013-2015 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"fmt"
	"math/big"

	"github.com/btcsuite/btcd/wire"
)

// ScriptFlags is a bitmask defining additional operations or
// tests that will be done when executing a Script.
type ScriptFlags uint32

const (
	// ScriptBip16 defines whether the bip16 threshhold has passed and thus
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

	// StandardVerifyFlags are the script flags which are used when
	// executing transaction scripts to enforce additional checks which
	// are required for the script to be considered standard.  These checks
	// help reduce issues related to transaction malleability as well as
	// allow pay-to-script hash transactions.  Note these flags are
	// different than what is required for the consensus rules in that they
	// are more strict.
	//
	// TODO: This definition does not belong here.  It belongs in a policy
	// package.
	StandardVerifyFlags = ScriptBip16 |
		ScriptVerifyDERSignatures |
		ScriptVerifyStrictEncoding |
		ScriptVerifyMinimalData |
		ScriptStrictMultiSig |
		ScriptDiscourageUpgradableNops |
		ScriptVerifyCleanStack
)

// Engine is the virtual machine that executes scripts.
type Engine struct {
	scripts         [][]parsedOpcode
	scriptIdx       int
	scriptOff       int
	lastcodesep     int
	dstack          Stack // data stack
	astack          Stack // alt stack
	tx              wire.MsgTx
	txIdx           int
	condStack       []int
	numOps          int
	flags           ScriptFlags
	bip16           bool     // treat execution as pay-to-script-hash
	savedFirstStack [][]byte // stack from first script for bip16 scripts
}

// hasFlag returns whether the script engine instance has the passed flag set.
func (vm *Engine) hasFlag(flag ScriptFlags) bool {
	return vm.flags&flag == flag
}

// Execute will execute all script in the script engine and return either nil
// for successful validation or an error if one occurred.
func (vm *Engine) Execute() (err error) {
	done := false
	for done != true {
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

// CheckErrorCondition returns nil if the running script has ended and was
// successful, leaving a a true boolean on the stack. An error otherwise,
// including if the script has not finished.
func (vm *Engine) CheckErrorCondition(finalScript bool) error {
	// Check we are actually done. if pc is past the end of script array
	// then we have run out of scripts to run.
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
	if v == false {
		// log interesting data.
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
// next opcode in the script, or the next script if the curent has ended. Step
// will return true in the case that the last opcode was successfully executed.
// if an error is returned then the result of calling Step or any other method
// is undefined.
func (vm *Engine) Step() (done bool, err error) {
	// verify that it is pointing to a valid script address
	err = vm.validPC()
	if err != nil {
		return true, err
	}
	opcode := vm.scripts[vm.scriptIdx][vm.scriptOff]

	err = opcode.exec(vm)
	if err != nil {
		return true, err
	}

	if vm.dstack.Depth()+vm.astack.Depth() > maxStackSize {
		return false, ErrStackOverflow
	}

	// prepare for next instruction
	vm.scriptOff++
	if vm.scriptOff >= len(vm.scripts[vm.scriptIdx]) {
		// Illegal to have an `if' that straddles two scripts.
		if err == nil && len(vm.condStack) != 1 {
			return false, ErrStackMissingEndif
		}

		// alt stack doesn't persist.
		_ = vm.astack.DropN(vm.astack.Depth())

		vm.numOps = 0 // number of ops is per script.
		vm.scriptOff = 0
		if vm.scriptIdx == 0 && vm.bip16 {
			vm.scriptIdx++
			vm.savedFirstStack = vm.GetStack()
		} else if vm.scriptIdx == 1 && vm.bip16 {
			// Put us past the end for CheckErrorCondition()
			vm.scriptIdx++
			// We check script ran ok, if so then we pull
			// the script out of the first stack and executre that.
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
			// Set stack to be the stack from first script
			// minus the script itself
			vm.SetStack(vm.savedFirstStack[:len(vm.savedFirstStack)-1])
		} else {
			vm.scriptIdx++
		}
		// there are zero length scripts in the wild
		if vm.scriptIdx < len(vm.scripts) && vm.scriptOff >= len(vm.scripts[vm.scriptIdx]) {
			vm.scriptIdx++
		}
		vm.lastcodesep = 0
		if vm.scriptIdx >= len(vm.scripts) {
			return true, nil
		}
	}
	return false, nil
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

// validPC returns an error if the current script position is valid for
// execution, nil otherwise.
func (vm *Engine) validPC() error {
	if vm.scriptIdx >= len(vm.scripts) {
		return fmt.Errorf("Past input scripts %v:%v %v:xxxx", vm.scriptIdx, vm.scriptOff, len(vm.scripts))
	}
	if vm.scriptOff >= len(vm.scripts[vm.scriptIdx]) {
		return fmt.Errorf("Past input scripts %v:%v %v:%04d", vm.scriptIdx, vm.scriptOff, vm.scriptIdx, len(vm.scripts[vm.scriptIdx]))
	}
	return nil
}

// DisasmScript returns the disassembly string for the script at offset
// ``idx''.  Where 0 is the scriptSig and 1 is the scriptPubKey.
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

// DisasmPC returns the string for the disassembly of the opcode that will be
// next to execute when Step() is called.
func (vm *Engine) DisasmPC() (string, error) {
	scriptIdx, scriptOff, err := vm.curPC()
	if err != nil {
		return "", err
	}
	return vm.disasm(scriptIdx, scriptOff), nil
}

// disasm is a helper member to produce the output for DisasmPC and
// DisasmScript. It produces the opcode prefixed by the program counter at the
// provided position in the script. it does no error checking and leaves that
// to the caller to provide a valid offse.
func (vm *Engine) disasm(scriptIdx int, scriptOff int) string {
	return fmt.Sprintf("%02x:%04x: %s", scriptIdx, scriptOff,
		vm.scripts[scriptIdx][scriptOff].print(false))
}

// subScript will return the script since the last OP_CODESEPARATOR
func (vm *Engine) subScript() []parsedOpcode {
	return vm.scripts[vm.scriptIdx][vm.lastcodesep:]
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

	if len(sig) < 8 {
		// Too short
		return fmt.Errorf("malformed signature: too short: %d < 8",
			len(sig))
	}
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

	// Make sure S is inside the signature
	if rLen+5 > len(sig) {
		return fmt.Errorf("malformed signature: S out of bounds")
	}

	sLen := int(sig[rLen+5])

	// The length of the elements does not match
	// the length of the signature
	if rLen+sLen+6 != len(sig) {
		return fmt.Errorf("malformed signature: invalid R length")
	}

	// R elements must be integers
	if sig[2] != 0x02 {
		return fmt.Errorf("malformed signature: missing first integer marker")
	}

	// Zero-length integers are not allowed for R
	if rLen == 0 {
		return fmt.Errorf("malformed signature: R length is zero")
	}

	// R must not be negative
	if sig[4]&0x80 != 0 {
		return fmt.Errorf("malformed signature: R value is negative")
	}

	// Null bytes at the start of R are not allowed, unless R would
	// otherwise be interpreted as a negative number.
	if rLen > 1 && sig[4] == 0x00 && sig[5]&0x80 == 0 {
		return fmt.Errorf("malformed signature: invalid R value")
	}

	// S elements must be integers
	if sig[rLen+4] != 0x02 {
		return fmt.Errorf("malformed signature: missing second integer marker")
	}

	// Zero-length integers are not allowed for S
	if sLen == 0 {
		return fmt.Errorf("malformed signature: S length is zero")
	}

	// S must not be negative
	if sig[rLen+6]&0x80 != 0 {
		return fmt.Errorf("malformed signature: S value is negative")
	}

	// Null bytes at the start of S are not allowed, unless S would
	// otherwise be interpreted as a negative number.
	if sLen > 1 && sig[rLen+6] == 0x00 && sig[rLen+7]&0x80 == 0 {
		return fmt.Errorf("malformed signature: invalid S value")
	}

	// Verify the S value is <= halforder.
	if vm.hasFlag(ScriptVerifyLowS) {
		sValue := new(big.Int).SetBytes(sig[rLen+6 : rLen+6+sLen])
		if sValue.Cmp(halfOrder) > 0 {
			return ErrStackInvalidLowSSignature
		}
	}

	return nil
}

// getStack returns the contents of stack as a byte array bottom up
func getStack(stack *Stack) [][]byte {
	array := make([][]byte, stack.Depth())
	for i := range array {
		// PeekByteArry can't fail due to overflow, already checked
		array[len(array)-i-1], _ = stack.PeekByteArray(i)
	}
	return array
}

// setStack sets the stack to the contents of the array where the last item in
// the array is the top item in the stack.
func setStack(stack *Stack, data [][]byte) {
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

// GetAltStack returns the contents of the primary stack as an array. where the
// last item in the array is the top of the stack.
func (vm *Engine) GetAltStack() [][]byte {
	return getStack(&vm.astack)
}

// SetAltStack sets the contents of the primary stack to the contents of the
// provided array where the last item in the array will be the top of the stack.
func (vm *Engine) SetAltStack(data [][]byte) {
	setStack(&vm.astack, data)
}

// NewEngine returns a new script engine for the provided public key script,
// transaction, and input index.  The flags modify the behavior of the script
// engine according to the description provided by each flag.
func NewEngine(scriptPubKey []byte, tx *wire.MsgTx, txIdx int, flags ScriptFlags) (*Engine, error) {
	if txIdx < 0 || txIdx >= len(tx.TxIn) {
		return nil, ErrInvalidIndex
	}
	scriptSig := tx.TxIn[txIdx].SignatureScript

	vm := Engine{flags: flags}
	if vm.hasFlag(ScriptVerifySigPushOnly) && !IsPushOnlyScript(scriptSig) {
		return nil, ErrStackNonPushOnly
	}

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

		// If the first scripts(s) are empty, must start on later ones.
		if i == 0 && len(scr) == 0 {
			// This could end up seeing an invalid initial pc if
			// all scripts were empty. However, that is an invalid
			// case and should fail.
			vm.scriptIdx = i + 1
		}
	}

	// Parse flags.
	if vm.hasFlag(ScriptBip16) && isScriptHash(vm.scripts[1]) {
		// if we are pay to scripthash then we only accept input
		// scripts that push data
		if !isPushOnly(vm.scripts[0]) {
			return nil, ErrStackP2SHNonPushOnly
		}
		vm.bip16 = true
	}
	if vm.hasFlag(ScriptVerifyMinimalData) {
		vm.dstack.verifyMinimalData = true
		vm.astack.verifyMinimalData = true
	}
	if vm.hasFlag(ScriptVerifyCleanStack) && !vm.hasFlag(ScriptBip16) {
		return nil, ErrInvalidFlags
	}

	vm.tx = *tx
	vm.txIdx = txIdx
	vm.condStack = []int{OpCondTrue}

	return &vm, nil
}
