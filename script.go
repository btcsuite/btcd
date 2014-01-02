// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcscript

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/conformal/btcec"
	"github.com/conformal/btcwire"
	"github.com/davecgh/go-spew/spew"
	"io"
	"time"
)

// StackErrShortScript is returned if the script has an opcode that is too long
// for the length of the script.
var StackErrShortScript = errors.New("execute past end of script")

// StackErrUnderflow is returned if an opcode requires more items on the stack
// than is present.
var StackErrUnderflow = errors.New("stack underflow")

// StackErrInvalidArgs is returned if the argument for an opcode is out of
// acceptable range.
var StackErrInvalidArgs = errors.New("invalid argument")

// StackErrOpDisabled is returned when a disabled opcode is encountered in the
// script.
var StackErrOpDisabled = errors.New("Disabled Opcode")

// StackErrVerifyFailed is returned when one of the OP_VERIFY or OP_*VERIFY
// instructions is executed and the conditions fails.
var StackErrVerifyFailed = errors.New("Verify failed")

// StackErrNumberTooBig is returned when the argument for an opcode that should
// be an offset is obviously far too large.
var StackErrNumberTooBig = errors.New("number too big")

// StackErrInvalidOpcode is returned when an opcode marked as invalid or a
// completely undefined opcode is encountered.
var StackErrInvalidOpcode = errors.New("Invalid Opcode")

// StackErrReservedOpcode is returned when an opcode marked as reserved is
// encountered.
var StackErrReservedOpcode = errors.New("Reserved Opcode")

// StackErrEarlyReturn is returned when OP_RETURN is executed in the script.
var StackErrEarlyReturn = errors.New("Script returned early")

// StackErrNoIf is returned if an OP_ELSE or OP_ENDIF is encountered without
// first having an OP_IF or OP_NOTIF in the script.
var StackErrNoIf = errors.New("OP_ELSE or OP_ENDIF with no matching OP_IF")

// StackErrMissingEndif is returned if the end of a script is reached without
// and OP_ENDIF to correspond to a conditional expression.
var StackErrMissingEndif = fmt.Errorf("execute fail, in conditional execution")

// StackErrTooManyPubkeys is returned if an OP_CHECKMULTISIG is encountered
// with more than MaxPubKeysPerMultiSig pubkeys present.
var StackErrTooManyPubkeys = errors.New("Invalid pubkey count in OP_CHECKMULTISIG")

// StackErrTooManyOperations is returned if a script has more then
// MaxOpsPerScript opcodes that do not push data.
var StackErrTooManyOperations = errors.New("Too many operations in script")

// StackErrElementTooBig is returned if the size of an element to be pushed to
// the stack is over MaxScriptElementSize.
var StackErrElementTooBig = errors.New("Element in script too large")

// StackErrUnknownAddress is returned when ScriptToAddrHash does not recognise
// the pattern of the script and thus can not find the address for payment.
var StackErrUnknownAddress = errors.New("non-recognised address")

// StackErrScriptFailed is returned when at the end of a script the boolean
// on top of the stack is false signifying that the script has failed.
var StackErrScriptFailed = errors.New("execute fail, fail on stack")

// StackErrScriptUnfinished is returned when CheckErrorCondition is called
// on a script that has not finished executing.
var StackErrScriptUnfinished = errors.New("Error check when script unfinished")

// StackErrEmpyStack is returned when the stack is empty at the end of
// execution. Normal operation requires that a boolean is on top of the stack
// when the scripts have finished executing.
var StackErrEmptyStack = errors.New("Stack empty at end of execution")

// StackErrP2SHNonPushOnly is returned when a Pay-to-Script-Hash transaction
// is encountered and the ScriptSig does operations other than push data (in
// violation of bip16).
var StackErrP2SHNonPushOnly = errors.New("pay to script hash with non " +
	"pushonly input")

// StackErrInvalidParseType is an internal error returned from ScriptToAddrHash
// ony if the internal data tables are wrong.
var StackErrInvalidParseType = errors.New("internal error: invalid parsetype found")

// StackErrInvalidAddrOffset is an internal error returned from ScriptToAddrHash
// ony if the internal data tables are wrong.
var StackErrInvalidAddrOffset = errors.New("internal error: invalid offset found")

// StackErrInvalidIndex is returned when an out-of-bounds index was passed to
// a function.
var StackErrInvalidIndex = errors.New("Invalid script index")

// StackErrNonPushOnly is returned when ScriptInfo is called with a pkScript
// that peforms operations other that pushing data to the stack.
var StackErrNonPushOnly = errors.New("SigScript is non pushonly")

// Bip16Activation is the timestamp where BIP0016 is valid to use in the
// blockchain.  To be used to determine if BIP0016 should be called for or not.
// This timestamp corresponds to Sun Apr 1 00:00:00 UTC 2012.
var Bip16Activation = time.Unix(1333238400, 0)

// Hash type bits from the end of a signature.
const (
	SigHashOld          = 0x0
	SigHashAll          = 0x1
	SigHashNone         = 0x2
	SigHashSingle       = 0x3
	SigHashAnyOneCanPay = 0x80
)

// These are the constants specified for maximums in individual scripts.
const (
	MaxOpsPerScript       = 201 // Max number of non-push operations.
	MaxPubKeysPerMultiSig = 20  // Multisig can't have more sigs than this.
	MaxScriptElementSize  = 520 // Max bytes pushable to the stack.
)

// ScriptClass is an enumeration for the list of standard types of script.
type ScriptClass byte

// Classes of script payment known about in the blockchain.
const (
	NonStandardTy ScriptClass = iota // None of the recognized forms.
	PubKeyTy                         // Pay pubkey.
	PubKeyHashTy                     // Pay pubkey hash.
	ScriptHashTy                     // Pay to script hash.
	MultiSigTy                       // Multi signature.
	NullDataTy                       // Empty data-only (provably prunable).
)

var scriptClassToName = []string{
	NonStandardTy: "nonstandard",
	PubKeyTy:      "pubkey",
	PubKeyHashTy:  "pubkeyhash",
	ScriptHashTy:  "scripthash",
	MultiSigTy:    "multisig",
	NullDataTy:    "nulldata",
}

// String implements the Stringer interface by returning the name of
// the enum script class. If the enum is invalid then "Invalid" will be
// returned.
func (t ScriptClass) String() string {
	if int(t) > len(scriptClassToName) || int(t) < 0 {
		return "Invalid"
	}
	return scriptClassToName[t]
}

// Script is the virtual machine that executes btcscripts.
type Script struct {
	scripts         [][]parsedOpcode
	scriptidx       int
	scriptoff       int
	lastcodesep     int
	dstack          Stack // data stack
	astack          Stack // alt stack
	tx              btcwire.MsgTx
	txidx           int
	condStack       []int
	numOps          int
	bip16           bool     // treat execution as pay-to-script-hash
	der             bool     // enforce DER encoding
	savedFirstStack [][]byte // stack from first script for bip16 scripts
}

// isPubkey returns true if the script passed is a pubkey transaction, false
// otherwise.
func isPubkey(pops []parsedOpcode) bool {
	return len(pops) == 2 &&
		pops[0].opcode.value > OP_FALSE &&
		pops[0].opcode.value <= OP_DATA_75 &&
		pops[1].opcode.value == OP_CHECKSIG
}

// isPubkeyHash returns true if the script passed is a pubkey hash transaction,
// false otherwise.
func isPubkeyHash(pops []parsedOpcode) bool {
	return len(pops) == 5 &&
		pops[0].opcode.value == OP_DUP &&
		pops[1].opcode.value == OP_HASH160 &&
		pops[2].opcode.value == OP_DATA_20 &&
		pops[3].opcode.value == OP_EQUALVERIFY &&
		pops[4].opcode.value == OP_CHECKSIG

}

// isScriptHash returns true if the script passed is a pay-to-script-hash (P2SH)
// transction, false otherwise.
func isScriptHash(pops []parsedOpcode) bool {
	return len(pops) == 3 &&
		pops[0].opcode.value == OP_HASH160 &&
		pops[1].opcode.value == OP_DATA_20 &&
		pops[2].opcode.value == OP_EQUAL
}

// IsPayToScriptHash returns true if the script is in the standard
// Pay-To-Script-Hash format, false otherwise.
func IsPayToScriptHash(script []byte) bool {
	pops, err := parseScript(script)
	if err != nil {
		return false
	}
	return isScriptHash(pops)
}

// isMultiSig returns true if the passed script is a multisig transaction, false
// otherwise.
func isMultiSig(pops []parsedOpcode) bool {
	l := len(pops)
	// absolute minimum is 1 pubkey so
	// OP_1-16, pubkey, OP_1, OP_CHECK_MULTISIG
	if l < 4 {
		return false
	}
	if pops[0].opcode.value < OP_1 ||
		pops[0].opcode.value > OP_16 {
		return false
	}
	if pops[l-2].opcode.value < OP_1 ||
		pops[l-2].opcode.value > OP_16 {
		return false
	}
	if pops[l-1].opcode.value != OP_CHECK_MULTISIG {
		return false
	}
	for _, pop := range pops[1 : l-2] {
		// valid pubkeys are either 65 or 33 bytes
		if len(pop.data) != 33 &&
			len(pop.data) != 65 {
			return false
		}
	}
	return true
}

// isNullData returns true if the passed script is a null data transaction,
// false otherwise.
func isNullData(pops []parsedOpcode) bool {
	// A nulldata transaction is either a single OP_RETURN or an
	// OP_RETURN SMALLDATA (where SMALLDATA is a push data up to 80 bytes).
	l := len(pops)
	if l == 1 && pops[0].opcode.value == OP_RETURN {
		return true
	}

	return l == 2 &&
		pops[0].opcode.value == OP_RETURN &&
		pops[1].opcode.value <= OP_PUSHDATA4 &&
		len(pops[1].data) <= 80
}

// isPushOnly returns true if the script only pushes data, false otherwise.
func isPushOnly(pops []parsedOpcode) bool {
	// technically we cheat here, we don't look at opcodes
	for _, pop := range pops {
		// all opcodes up to OP_16 are data instructions.
		if pop.opcode.value < OP_FALSE ||
			pop.opcode.value > OP_16 {
			return false
		}
	}
	return true
}

// IsPushOnlyScript returns whether or not the passed script only pushes data.
// If the script does not parse false will be returned.
func IsPushOnlyScript(script []byte) bool {
	pops, err := parseScript(script)
	if err != nil {
		return false
	}
	return isPushOnly(pops)
}

// GetScriptClass returns the class of the script passed. If the script does not
// parse then NonStandardTy will be returned.
func GetScriptClass(script []byte) ScriptClass {
	pops, err := parseScript(script)
	if err != nil {
		return NonStandardTy
	}
	return typeOfScript(pops)
}

// scriptType returns the type of the script being inspected from the known
// standard types.
func typeOfScript(pops []parsedOpcode) ScriptClass {
	// XXX dubious optimisation: order these in order of popularity in the
	// blockchain
	if isPubkey(pops) {
		return PubKeyTy
	} else if isPubkeyHash(pops) {
		return PubKeyHashTy
	} else if isScriptHash(pops) {
		return ScriptHashTy
	} else if isMultiSig(pops) {
		return MultiSigTy
	} else if isNullData(pops) {
		return NullDataTy
	}
	return NonStandardTy

}

// parseScript preparses the script in bytes into a list of parsedOpcodes while
// applying a number of sanity checks.
func parseScript(script []byte) ([]parsedOpcode, error) {
	return parseScriptTemplate(script, opcodemap)
}

// parseScriptTemplate is the same as parseScript but allows the passing of the
// template list for testing purposes. On error we return the list of parsed
// opcodes so far.
func parseScriptTemplate(script []byte, opcodemap map[byte]*opcode) ([]parsedOpcode, error) {
	retScript := make([]parsedOpcode, 0, len(script))
	for i := 0; i < len(script); {
		instr := script[i]
		op, ok := opcodemap[instr]
		if !ok {
			return retScript, StackErrInvalidOpcode
		}
		pop := parsedOpcode{opcode: op}
		// parse data out of instruction.
		switch {
		case op.length == 1:
			// no data, done here
			i++
		case op.length > 1:
			if len(script[i:]) < op.length {
				return retScript, StackErrShortScript
			}
			// slice out the data.
			pop.data = script[i+1 : i+op.length]
			i += op.length
		case op.length < 0:
			var err error
			var l uint
			off := i + 1
			switch op.length {
			case -1:
				l, err = scriptUInt8(script[off:])
			case -2:
				l, err = scriptUInt16(script[off:])
			case -4:
				l, err = scriptUInt32(script[off:])
			default:
				return retScript,
					fmt.Errorf("invalid opcode length %d",
						op.length)
			}

			if err != nil {
				return nil, err
			}
			off = i + 1 - op.length // beginning of data
			// Disallow entries that do not fit script or were
			// sign extended.
			if int(l) > len(script[off:]) || int(l) < 0 {
				return retScript, StackErrShortScript
			}
			pop.data = script[off : off+int(l)]
			i += 1 - op.length + int(l)
		}
		retScript = append(retScript, pop)
	}
	return retScript, nil
}

// unparseScript reversed the action of parseScript and returns the
// parsedOpcodes as a list of bytes
func unparseScript(pops []parsedOpcode) ([]byte, error) {
	script := make([]byte, 0, len(pops))
	for _, pop := range pops {
		b, err := pop.bytes()
		if err != nil {
			return nil, err
		}
		script = append(script, b...)
	}
	return script, nil
}

// ScriptFlags is a bitmask defining additional operations or
// tests that will be done when executing a Script.
type ScriptFlags uint32

const (
	// ScriptBip16 defines whether the bip16 threshhold has passed and thus
	// pay-to-script hash transactions will be fully validated.
	ScriptBip16 ScriptFlags = 1 << iota

	// ScriptCanonicalSignatures defines whether additional canonical
	// signature checks are performed when parsing a signature.
	//
	// Canonical (DER) signatures are not required in the tx rules for
	// block acceptance, but are checked in recent versions of bitcoind
	// when accepting transactions to the mempool.  Non-canonical (valid
	// BER but not valid DER) transactions can potentially be changed
	// before mined into a block, either by adding extra padding or
	// flipping the sign of the R or S value in the signature, creating a
	// transaction that still validates and spends the inputs, but is not
	// recognized by creator of the transaction.  Performing a canonical
	// check enforces script signatures use a unique DER format.
	ScriptCanonicalSignatures
)

// NewScript returns a new script engine for the provided tx and input idx with
// a signature script scriptSig and a pubkeyscript scriptPubKey. If bip16 is
// true then it will be treated as if the bip16 threshhold has passed and thus
// pay-to-script hash transactions will be fully validated.
func NewScript(scriptSig []byte, scriptPubKey []byte, txidx int, tx *btcwire.MsgTx, flags ScriptFlags) (*Script, error) {
	var m Script
	scripts := [][]byte{scriptSig, scriptPubKey}
	m.scripts = make([][]parsedOpcode, len(scripts))
	for i, scr := range scripts {
		var err error
		m.scripts[i], err = parseScript(scr)
		if err != nil {
			return nil, err
		}

		// If the first scripts(s) are empty, must start on later ones.
		if i == 0 && len(scr) == 0 {
			// This could end up seeing an invalid initial pc if
			// all scripts were empty. However, that is an invalid
			// case and should fail.
			m.scriptidx = i + 1
		}
	}

	// Parse flags.
	bip16 := flags&ScriptBip16 == ScriptBip16
	if bip16 && isScriptHash(m.scripts[1]) {
		// if we are pay to scripthash then we only accept input
		// scripts that push data
		if !isPushOnly(m.scripts[0]) {
			return nil, StackErrP2SHNonPushOnly
		}
		m.bip16 = true
	}
	if flags&ScriptCanonicalSignatures == ScriptCanonicalSignatures {
		m.der = true
	}

	m.tx = *tx
	m.txidx = txidx
	m.condStack = []int{OpCondTrue}

	return &m, nil
}

// Execute will execute all script in the script engine and return either nil
// for successful validation or an error if one occurred.
func (s *Script) Execute() (err error) {
	done := false
	for done != true {
		log.Tracef("%v", newLogClosure(func() string {
			dis, err := s.DisasmPC()
			if err != nil {
				return fmt.Sprintf("stepping (%v)", err)
			}
			return fmt.Sprintf("stepping %v", dis)
		}))

		done, err = s.Step()
		if err != nil {
			return err
		}
		log.Tracef("%v", newLogClosure(func() string {
			var dstr, astr string

			// if we're tracing, dump the stacks.
			if s.dstack.Depth() != 0 {
				dstr = "Stack\n" + spew.Sdump(s.dstack)
			}
			if s.astack.Depth() != 0 {
				astr = "AltStack\n" + spew.Sdump(s.astack)
			}

			return dstr + astr
		}))
	}

	return s.CheckErrorCondition()
}

// CheckErrorCondition returns nil if the running script has ended and was
// successful, leaving a a true boolean on the stack. An error otherwise,
// including if the script has not finished.
func (s *Script) CheckErrorCondition() (err error) {
	// Check we are actually done. if pc is past the end of script array
	// then we have run out of scripts to run.
	if s.scriptidx < len(s.scripts) {
		return StackErrScriptUnfinished
	}
	if s.dstack.Depth() < 1 {
		return StackErrEmptyStack
	}
	v, err := s.dstack.PopBool()
	if err == nil && v == false {
		// log interesting data.
		log.Tracef("%v", func() string {
			dis0, _ := s.DisasmScript(0)
			dis1, _ := s.DisasmScript(1)
			return fmt.Sprintf("scripts failed: script0: %s\n"+
				"script1: %s", dis0, dis1)
		})
		err = StackErrScriptFailed
	}
	if err == nil && len(s.condStack) != 1 {
		// conditional execution stack context left active
		err = StackErrMissingEndif
	}
	return err
}

// Step will execute the next instruction and move the program counter to the
// next opcode in the script, or the next script if the curent has ended. Step
// will return true in the case that the last opcode was successfully executed.
// if an error is returned then the result of calling Step or any other method
// is undefined.
func (m *Script) Step() (done bool, err error) {
	// verify that it is pointing to a valid script address
	err = m.validPC()
	if err != nil {
		return
	}
	opcode := m.scripts[m.scriptidx][m.scriptoff]

	executeInstr := true
	if m.condStack[0] != OpCondTrue {
		// some opcodes still 'activate' if on the non-executing side
		// of conditional execution
		if opcode.conditional() {
			executeInstr = true
		} else {
			executeInstr = false
		}
	}
	if executeInstr {
		err = opcode.exec(m)
		if err != nil {
			return
		}
	}

	// prepare for next instruction
	m.scriptoff++
	if m.scriptoff >= len(m.scripts[m.scriptidx]) {
		m.numOps = 0 // number of ops is per script.
		m.scriptoff = 0
		if m.scriptidx == 0 && m.bip16 {
			m.scriptidx++
			m.savedFirstStack = m.GetStack()
		} else if m.scriptidx == 1 && m.bip16 {
			// Put us past the end for CheckErrorCondition()
			m.scriptidx++
			// We check script ran ok, if so then we pull
			// the script out of the first stack and executre that.
			err := m.CheckErrorCondition()
			if err != nil {
				return false, err
			}

			script := m.savedFirstStack[len(m.savedFirstStack)-1]
			pops, err := parseScript(script)
			if err != nil {
				return false, err
			}
			m.scripts = append(m.scripts, pops)
			// Set stack to be the stack from first script
			// minus the script itself
			m.SetStack(m.savedFirstStack[:len(m.savedFirstStack)-1])
		} else {
			m.scriptidx++
		}
		// there are zero length scripts in the wild
		if m.scriptidx < len(m.scripts) && m.scriptoff >= len(m.scripts[m.scriptidx]) {
			m.scriptidx++
		}
		m.lastcodesep = 0
		if m.scriptidx >= len(m.scripts) {
			done = true
		}
	}
	return
}

// curPC returns either the current script and offset, or an error if the
// position isn't valid.
func (m *Script) curPC() (script int, off int, err error) {
	err = m.validPC()
	if err != nil {
		return 0, 0, err
	}
	return m.scriptidx, m.scriptoff, nil
}

// validPC returns an error if the current script position is valid for
// execution, nil otherwise.
func (m *Script) validPC() error {
	if m.scriptidx >= len(m.scripts) {
		return fmt.Errorf("Past input scripts %v:%v %v:xxxx", m.scriptidx, m.scriptoff, len(m.scripts))
	}
	if m.scriptoff >= len(m.scripts[m.scriptidx]) {
		return fmt.Errorf("Past input scripts %v:%v %v:%04d", m.scriptidx, m.scriptoff, m.scriptidx, len(m.scripts[m.scriptidx]))
	}
	return nil
}

// DisasmScript returns the disassembly string for the script at offset
// ``idx''.  Where 0 is the scriptSig and 1 is the scriptPubKey.
func (m *Script) DisasmScript(idx int) (disstr string, err error) {
	if idx >= len(m.scripts) {
		return "", StackErrInvalidIndex
	}
	for i := range m.scripts[idx] {
		disstr = disstr + m.disasm(idx, i) + "\n"
	}
	return disstr, nil
}

// DisasmPC returns the string for the disassembly of the opcode that will be
// next to execute when Step() is called.
func (m *Script) DisasmPC() (disstr string, err error) {
	scriptidx, scriptoff, err := m.curPC()
	if err != nil {
		return "", err
	}
	return m.disasm(scriptidx, scriptoff), nil
}

// disasm is a helper member to produce the output for DisasmPC and
// DisasmScript. It produces the opcode prefixed by the program counter at the
// provided position in the script. it does no error checking and leaves that
// to the caller to provide a valid offse.
func (m *Script) disasm(scriptidx int, scriptoff int) string {
	return fmt.Sprintf("%02x:%04x: %s", scriptidx, scriptoff,
		m.scripts[scriptidx][scriptoff].print(false))
}

// subScript will return the script since the last OP_CODESEPARATOR
func (s *Script) subScript() []parsedOpcode {
	return s.scripts[s.scriptidx][s.lastcodesep:]
}

// removeOpcode will remove any opcode matching ``opcode'' from the opcode
// stream in pkscript
func removeOpcode(pkscript []parsedOpcode, opcode byte) []parsedOpcode {
	retScript := make([]parsedOpcode, 0, len(pkscript))
	for _, pop := range pkscript {
		if pop.opcode.value != opcode {
			retScript = append(retScript, pop)
		}
	}
	return retScript
}

// removeOpcodeByData will return the pkscript minus any opcodes that would
// push the data in ``data'' to the stack.
func removeOpcodeByData(pkscript []parsedOpcode, data []byte) []parsedOpcode {
	retScript := make([]parsedOpcode, 0, len(pkscript))
	for _, pop := range pkscript {
		if !bytes.Equal(pop.data, data) {
			retScript = append(retScript, pop)
		}
	}
	return retScript

}

// DisasmString formats a disassembled script for one line printing.
func DisasmString(buf []byte) (string, error) {
	disbuf := ""
	opcodes, err := parseScript(buf)
	if err != nil {
		return "", err
	}
	for _, pop := range opcodes {
		disbuf += pop.print(true) + " "
	}
	if disbuf != "" {
		disbuf = disbuf[:len(disbuf)-1]
	}
	return disbuf, nil
}

// calcScriptHash will, given the a script and hashtype for the current
// scriptmachine, calculate the doubleSha256 hash of the transaction and
// script to be used for signature signing and verification.
func calcScriptHash(script []parsedOpcode, hashType byte, tx *btcwire.MsgTx, idx int) []byte {

	// remove all instances of OP_CODESEPARATOR still left in the script
	script = removeOpcode(script, OP_CODESEPARATOR)

	// Make a deep copy of the transaction, zeroing out the script
	// for all inputs that are not currently being processed.
	txCopy := tx.Copy()
	for i := range txCopy.TxIn {
		var txIn btcwire.TxIn
		txIn = *txCopy.TxIn[i]
		txCopy.TxIn[i] = &txIn
		if i == idx {
			// unparseScript cannot fail here, because removeOpcode
			// above only returns a valid script.
			sigscript, _ := unparseScript(script)
			txCopy.TxIn[idx].SignatureScript = sigscript
		} else {
			txCopy.TxIn[i].SignatureScript = []byte{}
		}
	}
	// Default behaviour has all outputs set up.
	for i := range txCopy.TxOut {
		var txOut btcwire.TxOut
		txOut = *txCopy.TxOut[i]
		txCopy.TxOut[i] = &txOut
	}

	switch hashType & 31 {
	case SigHashNone:
		txCopy.TxOut = txCopy.TxOut[0:0] // empty slice
		for i := range txCopy.TxIn {
			if i != idx {
				txCopy.TxIn[i].Sequence = 0
			}
		}
	case SigHashSingle:
		if idx >= len(txCopy.TxOut) {
			// This was created by a buggy implementation.
			// In this case we do the same as bitcoind and bitcoinj
			// and return 1 (as a uint256 little endian) as an
			// error. Unfortunately this was not checked anywhere
			// and thus is treated as the actual
			// hash.
			hash := make([]byte, 32)
			hash[0] = 0x01
			return hash
		}
		// Resize output array to up to and including requested index.
		txCopy.TxOut = txCopy.TxOut[:idx+1]
		// all but  current output get zeroed out
		for i := 0; i < idx; i++ {
			txCopy.TxOut[i].Value = -1
			txCopy.TxOut[i].PkScript = []byte{}
		}
		// Sequence on all other inputs is 0, too.
		for i := range txCopy.TxIn {
			if i != idx {
				txCopy.TxIn[i].Sequence = 0
			}
		}
	default:
		// XXX bitcoind treats undefined hashtypes like normal
		// SigHashAll for purposes of hash generation.
		fallthrough
	case SigHashOld:
		fallthrough
	case SigHashAll:
		// nothing special here
	}
	if hashType&SigHashAnyOneCanPay != 0 {
		txCopy.TxIn = txCopy.TxIn[idx : idx+1]
		idx = 0
	}

	var wbuf bytes.Buffer
	txCopy.Serialize(&wbuf)
	// Append LE 4 bytes hash type
	binary.Write(&wbuf, binary.LittleEndian, uint32(hashType))

	return btcwire.DoubleSha256(wbuf.Bytes())
}

// scriptUInt8 return the number stored in the first byte of a slice.
func scriptUInt8(script []byte) (uint, error) {
	if len(script) <= 1 {
		return 0, StackErrShortScript
	}
	return uint(script[0]), nil
}

// scriptUInt16 returns the number stored in the next 2 bytes of a slice.
func scriptUInt16(script []byte) (uint, error) {
	if len(script) <= 2 {
		return 0, StackErrShortScript
	}
	// Yes this is little endian
	return ((uint(script[1]) << 8) | uint(script[0])), nil
}

// scriptUInt32 returns the number stored in the first 4 bytes of a slice.
func scriptUInt32(script []byte) (uint, error) {
	if len(script) <= 4 {
		return 0, StackErrShortScript
	}
	// Yes this is little endian
	return ((uint(script[3]) << 24) | (uint(script[2]) << 16) |
		(uint(script[1]) << 8) | uint(script[0])), nil
}

// getStack returns the contents of stack as a byte array bottom up
func getStack(stack *Stack) [][]byte {
	array := make([][]byte, stack.Depth())
	for i := range array {
		// PeekByteArry can't fail due to overflow, already checked
		array[len(array)-i-1], _ =
			stack.PeekByteArray(i)
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
func (s *Script) GetStack() [][]byte {
	return getStack(&s.dstack)
}

// SetStack sets the contents of the primary stack to the contents of the
// provided array where the last item in the array will be the top of the stack.
func (s *Script) SetStack(data [][]byte) {
	setStack(&s.dstack, data)
}

// GetAltStack returns the contents of the primary stack as an array. where the
// last item in the array is the top of the stack.
func (s *Script) GetAltStack() [][]byte {
	return getStack(&s.astack)
}

// SetAltStack sets the contents of the primary stack to the contents of the
// provided array where the last item in the array will be the top of the stack.
func (s *Script) SetAltStack(data [][]byte) {
	setStack(&s.astack, data)
}

// GetSigOpCount provides a quick count of the number of signature operations
// in a script. a CHECKSIG operations counts for 1, and a CHECK_MULTISIG for 20.
// If the script fails to parse, then the count up to the point of failure is
// returned.
func GetSigOpCount(script []byte) int {
	// We don't check error since parseScript returns the parsed-up-to-error
	// list of pops.
	pops, _ := parseScript(script)

	return getSigOpCount(pops, false)
}

// GetPreciseSigOpCount returns the number of signature operations in
// scriptPubKey. If bip16 is true then scriptSig may be searched for the
// Pay-To-Script-Hash script in order to find the precise number of signature
// operations in the transaction. If the script fails to parse, then the
// count up to the point of failure is returned.
func GetPreciseSigOpCount(scriptSig, scriptPubKey []byte, bip16 bool) int {
	// We don't check error since parseScript returns the parsed-up-to-error
	// list of pops.
	pops, _ := parseScript(scriptPubKey)
	// non P2SH transactions just treated as normal.
	if !(bip16 && isScriptHash(pops)) {
		return getSigOpCount(pops, true)
	}

	// Ok so this is P2SH, get the contained script and count it..

	sigPops, err := parseScript(scriptSig)
	if err != nil {
		return 0
	}
	if !isPushOnly(sigPops) || len(sigPops) == 0 {
		return 0
	}

	shScript := sigPops[len(sigPops)-1].data
	// Means that sigPops is jus OP_1 - OP_16, no sigops there.
	if shScript == nil {
		return 0
	}

	shPops, _ := parseScript(shScript)

	return getSigOpCount(shPops, true)
}

// getSigOpCount is the implementation function for counting the number of
// signature operations in the script provided by pops. If precise mode is
// requested then we attempt to count the number of operations for a multisig
// op. Otherwise we use the maximum.
func getSigOpCount(pops []parsedOpcode, precise bool) int {
	nSigs := 0
	for i, pop := range pops {
		switch pop.opcode.value {
		case OP_CHECKSIG:
			fallthrough
		case OP_CHECKSIGVERIFY:
			nSigs++
		case OP_CHECK_MULTISIG:
			fallthrough
		case OP_CHECKMULTISIGVERIFY:
			// If we are being precise then look for familiar
			// patterns for multisig, for now all we recognise is
			// OP_1 - OP_16 to signify the number of pubkeys.
			// Otherwise, we use the max of 20.
			if precise && i > 0 &&
				pops[i-1].opcode.value >= OP_1 &&
				pops[i-1].opcode.value <= OP_16 {
				nSigs += int(pops[i-1].opcode.value -
					(OP_1 - 1))
			} else {
				nSigs += MaxPubKeysPerMultiSig
			}
		default:
			// not a sigop.
		}
	}

	return nSigs
}

// PayToPubKeyHashScript creates a new script to pay a transaction
// output to a 20-byte pubkey hash.
func PayToPubKeyHashScript(pubKeyHash []byte) (pkScript []byte, err error) {
	pops := []parsedOpcode{
		parsedOpcode{
			opcode: opcodemap[OP_DUP],
		},
		parsedOpcode{
			opcode: opcodemap[OP_HASH160],
		},
		parsedOpcode{
			opcode: opcodemap[OP_DATA_20],
			data:   pubKeyHash,
		},
		parsedOpcode{
			opcode: opcodemap[OP_EQUALVERIFY],
		},
		parsedOpcode{
			opcode: opcodemap[OP_CHECKSIG],
		},
	}
	return unparseScript(pops)
}

// SignatureScript creates an input signature script for tx to spend
// BTC sent from a previous output to the owner of privkey.  tx must
// include all transaction inputs and outputs, however txin scripts are
// allowed to be filled or empty. The returned script is calculated to
// be used as the idx'th txin sigscript for tx. subscript is the PkScript
// of the previous output being used as the idx'th input.  privkey is
// serialized in either a compressed or uncompressed format based on
// compress.  This format must match the same format used to generate
// the payment address, or the script validation will fail.
func SignatureScript(tx *btcwire.MsgTx, idx int, subscript []byte, hashType byte, privkey *ecdsa.PrivateKey, compress bool) ([]byte, error) {

	return signatureScriptCustomReader(rand.Reader, tx, idx, subscript,
		hashType, privkey, compress)
}

// This function exists so we can test ecdsa.Sign's error for an invalid
// reader.
func signatureScriptCustomReader(reader io.Reader, tx *btcwire.MsgTx, idx int,
	subscript []byte, hashType byte, privkey *ecdsa.PrivateKey,
	compress bool) ([]byte, error) {

	parsedScript, err := parseScript(subscript)
	if err != nil {
		return nil, fmt.Errorf("cannot parse output script: %v", err)
	}
	hash := calcScriptHash(parsedScript, hashType, tx, idx)
	r, s, err := ecdsa.Sign(reader, privkey, hash)
	if err != nil {
		return nil, fmt.Errorf("cannot sign tx input: %s", err)
	}
	ecSig := btcec.Signature{R: r, S: s}
	sig := append(ecSig.Serialize(), hashType)

	pk := (*btcec.PublicKey)(&privkey.PublicKey)
	var pubkeyOpcode *parsedOpcode
	if compress {
		pubkeyOpcode = &parsedOpcode{
			opcode: opcodemap[OP_DATA_33],
			data:   pk.SerializeCompressed(),
		}
	} else {
		pubkeyOpcode = &parsedOpcode{
			opcode: opcodemap[OP_DATA_65],
			data:   pk.SerializeUncompressed(),
		}
	}
	pops := []parsedOpcode{
		parsedOpcode{
			opcode: opcodemap[byte(len(sig))],
			data:   sig,
		},
		*pubkeyOpcode,
	}
	return unparseScript(pops)
}

// expectedInputs returns the number of arguments required by a script.
// If the script is of unnown type such that the number can not be determined
// then -1 is returned. We are an interanl function and thus assume that class
// is the real class of pops (and we can thus assume things that were
// determined while finding out the type).
func expectedInputs(pops []parsedOpcode, class ScriptClass) int {
	// count needed inputs.
	switch class {
	case PubKeyTy:
		return 1
	case PubKeyHashTy:
		return 2
	case ScriptHashTy:
		// Not including script, handled below.
		return 1
	case MultiSigTy:
		// Standard multisig has a push a small number for the number
		// of sigs and number of keys.
		// Check the first push instrution to see how many arguments are
		// expected. typoeOfScript already checked this so that we know
		// it'll be one of OP_1 - OP_16.
		return int(pops[0].opcode.value - (OP_1 - 1))
	case NullDataTy:
		fallthrough
	default:
		return -1
	}
}

type ScriptInfo struct {
	// The class of the sigscript, equivalent to calling GetScriptClass
	// on the sigScript.
	PkScriptClass ScriptClass
	// the number of inputs provided by the pkScript
	NumInputs int
	// the number of outputs required by sigScript and any
	// pay-to-script-hash scripts. The number will be -1 if unknown.
	ExpectedInputs int
	// The nubmer of signature operations in the scriptpair.
	SigOps int
}

// CalcScriptInfo returns a structure providing data about the scriptpair that
// are provided as arguments. It will error if the pair is in someway invalid
// such taht they can not be analysed, i.e. if they do not parse or the
// pkScript is not a push-only script
func CalcScriptInfo(sigscript, pkscript []byte, bip16 bool) (*ScriptInfo, error) {
	si := new(ScriptInfo)
	// parse both scripts.
	sigPops, err := parseScript(sigscript)
	if err != nil {
		return nil, err
	}

	pkPops, err := parseScript(pkscript)
	if err != nil {
		return nil, err
	}

	// push only sigScript makes little sense.
	si.PkScriptClass = typeOfScript(pkPops)

	// Can't have a pkScript that doesn't just push data.
	if !isPushOnly(sigPops) {
		return nil, StackErrNonPushOnly
	}

	si.ExpectedInputs = expectedInputs(pkPops, si.PkScriptClass)
	// all entries push to stack (or are OP_RESERVED and exec will fail).
	si.NumInputs = len(sigPops)

	if si.PkScriptClass == ScriptHashTy && bip16 {
		// grab the last push instruction in the script and pull out the
		// data.
		script := sigPops[len(sigPops)-1].data
		// check for existance and error else.
		shPops, err := parseScript(script)
		if err != nil {
			return nil, err
		}

		shClass := typeOfScript(shPops)

		shInputs := expectedInputs(shPops, shClass)
		if shInputs == -1 {
			// We have no fucking clue, then.
			si.ExpectedInputs = -1
		} else {
			si.ExpectedInputs += shInputs
		}
		si.SigOps = getSigOpCount(shPops, true)
	} else {
		si.SigOps = getSigOpCount(pkPops, true)
	}

	return si, nil
}

// CalcMultiSigStats returns the number of public keys and signatures from
// a multi-signature transaction script.  The passed script MUST already be
// known to be a multi-signature script.
func CalcMultiSigStats(script []byte) (int, int, error) {
	pops, err := parseScript(script)
	if err != nil {
		return 0, 0, err
	}

	// A multi-signature script is of the pattern:
	//  NUM_SIGS PUBKEY PUBKEY PUBKEY... NUM_PUBKEYS OP_CHECKMULTISIG
	// Therefore the number of signatures is the oldest item on the stack
	// and the number of pubkeys is the 2nd to last.  Also, the absolute
	// minimum for a multi-signature script is 1 pubkey, so at least 4
	// items must be on the stack per:
	//  OP_1 PUBKEY OP_1 OP_CHECKMULTISIG
	if len(pops) < 4 {
		return 0, 0, StackErrUnderflow
	}

	numSigs := int(pops[0].opcode.value - (OP_1 - 1))
	numPubKeys := int(pops[len(pops)-2].opcode.value - (OP_1 - 1))
	return numPubKeys, numSigs, nil
}

// CalcPkScriptAddrHashes returns the type of script as well as the required
// number of signatures and the hashes of the addresses.
func CalcPkScriptAddrHashes(script []byte) (ScriptClass, int, [][]byte) {
	c := GetScriptClass(script)
	switch c {
	case ScriptHashTy:
		fallthrough
	case PubKeyTy:
		fallthrough
	case PubKeyHashTy:
		_, addrHash, err := ScriptToAddrHash(script)
		if err != nil {
			return NonStandardTy, 0, nil
		}
		return c, 1, [][]byte{addrHash}

	case MultiSigTy:
		_, reqSigs, addrHashes, err := ScriptToAddrHashes(script)
		if err != nil {
			return NonStandardTy, 0, nil
		}

		return c, reqSigs, addrHashes
	case NonStandardTy:
		fallthrough
	case NullDataTy:
		fallthrough
	default:
		return NonStandardTy, 0, nil
	}
}
