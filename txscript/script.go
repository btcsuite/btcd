// Copyright (c) 2013-2015 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/btcec"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

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
	ErrStackOpDisabled = errors.New("Disabled Opcode")

	// ErrStackVerifyFailed is returned when one of the OP_VERIFY or
	// OP_*VERIFY instructions is executed and the conditions fails.
	ErrStackVerifyFailed = errors.New("Verify failed")

	// ErrStackNumberTooBig is returned when the argument for an opcode that
	// should be an offset is obviously far too large.
	ErrStackNumberTooBig = errors.New("number too big")

	// ErrStackInvalidOpcode is returned when an opcode marked as invalid or
	// a completely undefined opcode is encountered.
	ErrStackInvalidOpcode = errors.New("Invalid Opcode")

	// ErrStackReservedOpcode is returned when an opcode marked as reserved
	// is encountered.
	ErrStackReservedOpcode = errors.New("Reserved Opcode")

	// ErrStackEarlyReturn is returned when OP_RETURN is executed in the
	// script.
	ErrStackEarlyReturn = errors.New("Script returned early")

	// ErrStackNoIf is returned if an OP_ELSE or OP_ENDIF is encountered
	// without first having an OP_IF or OP_NOTIF in the script.
	ErrStackNoIf = errors.New("OP_ELSE or OP_ENDIF with no matching OP_IF")

	// ErrStackMissingEndif is returned if the end of a script is reached
	// without and OP_ENDIF to correspond to a conditional expression.
	ErrStackMissingEndif = fmt.Errorf("execute fail, in conditional execution")

	// ErrStackTooManyPubkeys is returned if an OP_CHECKMULTISIG is
	// encountered with more than MaxPubKeysPerMultiSig pubkeys present.
	ErrStackTooManyPubkeys = errors.New("Invalid pubkey count in OP_CHECKMULTISIG")

	// ErrStackTooManyOperations is returned if a script has more than
	// MaxOpsPerScript opcodes that do not push data.
	ErrStackTooManyOperations = errors.New("Too many operations in script")

	// ErrStackElementTooBig is returned if the size of an element to be
	// pushed to the stack is over MaxScriptElementSize.
	ErrStackElementTooBig = errors.New("Element in script too large")

	// ErrStackUnknownAddress is returned when ScriptToAddrHash does not
	// recognise the pattern of the script and thus can not find the address
	// for payment.
	ErrStackUnknownAddress = errors.New("non-recognised address")

	// ErrStackScriptFailed is returned when at the end of a script the
	// boolean on top of the stack is false signifying that the script has
	// failed.
	ErrStackScriptFailed = errors.New("execute fail, fail on stack")

	// ErrStackScriptUnfinished is returned when CheckErrorCondition is
	// called on a script that has not finished executing.
	ErrStackScriptUnfinished = errors.New("Error check when script unfinished")

	// ErrStackEmptyStack is returned when the stack is empty at the end of
	// execution. Normal operation requires that a boolean is on top of the
	// stack when the scripts have finished executing.
	ErrStackEmptyStack = errors.New("Stack empty at end of execution")

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
	ErrStackInvalidIndex = errors.New("Invalid script index")

	// ErrStackNonPushOnly is returned when ScriptInfo is called with a
	// pkScript that peforms operations other that pushing data to the stack.
	ErrStackNonPushOnly = errors.New("SigScript is non pushonly")

	// ErrStackOverflow is returned when stack and altstack combined depth
	// is over the limit.
	ErrStackOverflow = errors.New("Stacks overflowed")

	// ErrStackInvalidPubKey is returned when the ScriptVerifyScriptEncoding
	// flag is set and the script contains invalid pubkeys.
	ErrStackInvalidPubKey = errors.New("invalid strict pubkey")

	// ErrStackMinimalData is returned when the ScriptVerifyMinimalData flag
	// is set and the script contains push operations that do not use
	// the minimal opcode required.
	ErrStackMinimalData = errors.New("non-minimally encoded script number")
)

const (
	// maxStackSize is the maximum combined height of stack and alt stack
	// during execution.
	maxStackSize = 1000

	// maxScriptSize is the maximum allowed length of a raw script.
	maxScriptSize = 10000
)

// ErrUnsupportedAddress is returned when a concrete type that implements
// a btcutil.Address is not a supported type.
var ErrUnsupportedAddress = errors.New("unsupported address type")

// Bip16Activation is the timestamp where BIP0016 is valid to use in the
// blockchain.  To be used to determine if BIP0016 should be called for or not.
// This timestamp corresponds to Sun Apr 1 00:00:00 UTC 2012.
var Bip16Activation = time.Unix(1333238400, 0)

// SigHashType represents hash type bits at the end of a signature.
type SigHashType byte

// Hash type bits from the end of a signature.
const (
	SigHashOld          SigHashType = 0x0
	SigHashAll          SigHashType = 0x1
	SigHashNone         SigHashType = 0x2
	SigHashSingle       SigHashType = 0x3
	SigHashAnyOneCanPay SigHashType = 0x80
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

// Script is the virtual machine that executes scripts.
type Script struct {
	scripts                  [][]parsedOpcode
	scriptidx                int
	scriptoff                int
	lastcodesep              int
	dstack                   Stack // data stack
	astack                   Stack // alt stack
	tx                       wire.MsgTx
	txidx                    int
	condStack                []int
	numOps                   int
	bip16                    bool     // treat execution as pay-to-script-hash
	strictMultiSig           bool     // verify multisig stack item is zero length
	discourageUpgradableNops bool     // NOP1 to NOP10 are reserved for future soft-fork upgrades
	verifyStrictEncoding     bool     // verify strict encoding of signatures
	verifyDERSignatures      bool     // verify signatures compily with the DER
	savedFirstStack          [][]byte // stack from first script for bip16 scripts
}

// isSmallInt returns whether or not the opcode is considered a small integer,
// which is an OP_0, or OP_1 through OP_16.
func isSmallInt(op *opcode) bool {
	if op.value == OP_0 || (op.value >= OP_1 && op.value <= OP_16) {
		return true
	}
	return false
}

// isPubkey returns true if the script passed is a pubkey transaction, false
// otherwise.
func isPubkey(pops []parsedOpcode) bool {
	// valid pubkeys are either 33 or 65 bytes
	return len(pops) == 2 &&
		(len(pops[0].data) == 33 || len(pops[0].data) == 65) &&
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
	// OP_0/OP_1-16, pubkey, OP_1, OP_CHECKMULTISIG
	if l < 4 {
		return false
	}
	if !isSmallInt(pops[0].opcode) {
		return false
	}
	if !isSmallInt(pops[l-2].opcode) {
		return false
	}
	if pops[l-1].opcode.value != OP_CHECKMULTISIG {
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
	// OP_RETURN SMALLDATA (where SMALLDATA is a push data up to 40 bytes).
	l := len(pops)
	if l == 1 && pops[0].opcode.value == OP_RETURN {
		return true
	}

	return l == 2 &&
		pops[0].opcode.value == OP_RETURN &&
		pops[1].opcode.value <= OP_PUSHDATA4 &&
		len(pops[1].data) <= 40
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

// checkHashTypeEncoding returns whether or not the passed hashtype adheres to
// the strict encoding requirements if enabled.
func (s *Script) checkHashTypeEncoding(hashType SigHashType) error {
	if !s.verifyStrictEncoding {
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
func (s *Script) checkPubKeyEncoding(pubKey []byte) error {
	if !s.verifyStrictEncoding {
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
func (s *Script) checkSignatureEncoding(sig []byte) error {
	if !s.verifyStrictEncoding && !s.verifyDERSignatures {
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

	return nil
}

// canonicalPush returns true if the object is either not a push instruction
// or the push instruction contained wherein is matches the canonical form
// or using the smallest instruction to do the job. False otherwise.
func canonicalPush(pop parsedOpcode) bool {
	opcode := pop.opcode.value
	data := pop.data
	dataLen := len(pop.data)
	if opcode > OP_16 {
		return true
	}

	if opcode < OP_PUSHDATA1 && opcode > OP_0 && (dataLen == 1 && data[0] <= 16) {
		return false
	}
	if opcode == OP_PUSHDATA1 && dataLen < OP_PUSHDATA1 {
		return false
	}
	if opcode == OP_PUSHDATA2 && dataLen <= 0xff {
		return false
	}
	if opcode == OP_PUSHDATA4 && dataLen <= 0xffff {
		return false
	}
	return true
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
			return retScript, ErrStackInvalidOpcode
		}
		pop := parsedOpcode{opcode: op}
		// parse data out of instruction.
		switch {
		case op.length == 1:
			// no data, done here
			i++
		case op.length > 1:
			if len(script[i:]) < op.length {
				return retScript, ErrStackShortScript
			}
			// slice out the data.
			pop.data = script[i+1 : i+op.length]
			i += op.length
		case op.length < 0:
			var l uint
			off := i + 1

			if len(script[off:]) < -op.length {
				return retScript, ErrStackShortScript
			}

			// Next -length bytes are little endian length of data.
			switch op.length {
			case -1:
				l = uint(script[off])
			case -2:
				l = ((uint(script[off+1]) << 8) |
					uint(script[off]))
			case -4:
				l = ((uint(script[off+3]) << 24) |
					(uint(script[off+2]) << 16) |
					(uint(script[off+1]) << 8) |
					uint(script[off]))
			default:
				return retScript,
					fmt.Errorf("invalid opcode length %d",
						op.length)
			}

			off += -op.length // beginning of data
			// Disallow entries that do not fit script or were
			// sign extended.
			if int(l) > len(script[off:]) || int(l) < 0 {
				return retScript, ErrStackShortScript
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

	// ScriptVerifyDERSignatures defines that signatures are required
	// to compily with the DER format.
	ScriptVerifyDERSignatures

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
	// TODO: These flags do not belong here.  These flags belong in a
	// policy package.
	StandardVerifyFlags = ScriptBip16 |
		ScriptVerifyDERSignatures |
		ScriptVerifyStrictEncoding |
		ScriptVerifyMinimalData |
		ScriptStrictMultiSig |
		ScriptDiscourageUpgradableNops
)

// NewScript returns a new script engine for the provided tx and input idx with
// a signature script scriptSig and a pubkeyscript scriptPubKey. If bip16 is
// true then it will be treated as if the bip16 threshhold has passed and thus
// pay-to-script hash transactions will be fully validated.
func NewScript(scriptSig []byte, scriptPubKey []byte, txidx int, tx *wire.MsgTx, flags ScriptFlags) (*Script, error) {
	var m Script
	if flags&ScriptVerifySigPushOnly == ScriptVerifySigPushOnly && !IsPushOnlyScript(scriptSig) {
		return nil, ErrStackNonPushOnly
	}

	scripts := [][]byte{scriptSig, scriptPubKey}
	m.scripts = make([][]parsedOpcode, len(scripts))
	for i, scr := range scripts {
		if len(scr) > maxScriptSize {
			return nil, ErrStackLongScript
		}
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
			return nil, ErrStackP2SHNonPushOnly
		}
		m.bip16 = true
	}
	if flags&ScriptStrictMultiSig == ScriptStrictMultiSig {
		m.strictMultiSig = true
	}
	if flags&ScriptDiscourageUpgradableNops == ScriptDiscourageUpgradableNops {
		m.discourageUpgradableNops = true
	}
	if flags&ScriptVerifyStrictEncoding == ScriptVerifyStrictEncoding {
		m.verifyStrictEncoding = true
	}
	if flags&ScriptVerifyDERSignatures == ScriptVerifyDERSignatures {
		m.verifyDERSignatures = true
	}
	if flags&ScriptVerifyMinimalData == ScriptVerifyMinimalData {
		m.dstack.verifyMinimalData = true
		m.astack.verifyMinimalData = true
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
				dstr = "Stack:\n" + s.dstack.String()
			}
			if s.astack.Depth() != 0 {
				astr = "AltStack:\n" + s.astack.String()
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
		return ErrStackScriptUnfinished
	}
	if s.dstack.Depth() < 1 {
		return ErrStackEmptyStack
	}
	v, err := s.dstack.PopBool()
	if err == nil && v == false {
		// log interesting data.
		log.Tracef("%v", newLogClosure(func() string {
			dis0, _ := s.DisasmScript(0)
			dis1, _ := s.DisasmScript(1)
			return fmt.Sprintf("scripts failed: script0: %s\n"+
				"script1: %s", dis0, dis1)
		}))
		err = ErrStackScriptFailed
	}
	return err
}

// Step will execute the next instruction and move the program counter to the
// next opcode in the script, or the next script if the curent has ended. Step
// will return true in the case that the last opcode was successfully executed.
// if an error is returned then the result of calling Step or any other method
// is undefined.
func (s *Script) Step() (done bool, err error) {
	// verify that it is pointing to a valid script address
	err = s.validPC()
	if err != nil {
		return true, err
	}
	opcode := s.scripts[s.scriptidx][s.scriptoff]

	err = opcode.exec(s)
	if err != nil {
		return true, err
	}

	if s.dstack.Depth()+s.astack.Depth() > maxStackSize {
		return false, ErrStackOverflow
	}

	// prepare for next instruction
	s.scriptoff++
	if s.scriptoff >= len(s.scripts[s.scriptidx]) {
		// Illegal to have an `if' that straddles two scripts.
		if err == nil && len(s.condStack) != 1 {
			return false, ErrStackMissingEndif
		}

		// alt stack doesn't persist.
		_ = s.astack.DropN(s.astack.Depth())

		s.numOps = 0 // number of ops is per script.
		s.scriptoff = 0
		if s.scriptidx == 0 && s.bip16 {
			s.scriptidx++
			s.savedFirstStack = s.GetStack()
		} else if s.scriptidx == 1 && s.bip16 {
			// Put us past the end for CheckErrorCondition()
			s.scriptidx++
			// We check script ran ok, if so then we pull
			// the script out of the first stack and executre that.
			err := s.CheckErrorCondition()
			if err != nil {
				return false, err
			}

			script := s.savedFirstStack[len(s.savedFirstStack)-1]
			pops, err := parseScript(script)
			if err != nil {
				return false, err
			}
			s.scripts = append(s.scripts, pops)
			// Set stack to be the stack from first script
			// minus the script itself
			s.SetStack(s.savedFirstStack[:len(s.savedFirstStack)-1])
		} else {
			s.scriptidx++
		}
		// there are zero length scripts in the wild
		if s.scriptidx < len(s.scripts) && s.scriptoff >= len(s.scripts[s.scriptidx]) {
			s.scriptidx++
		}
		s.lastcodesep = 0
		if s.scriptidx >= len(s.scripts) {
			return true, nil
		}
	}
	return false, nil
}

// curPC returns either the current script and offset, or an error if the
// position isn't valid.
func (s *Script) curPC() (script int, off int, err error) {
	err = s.validPC()
	if err != nil {
		return 0, 0, err
	}
	return s.scriptidx, s.scriptoff, nil
}

// validPC returns an error if the current script position is valid for
// execution, nil otherwise.
func (s *Script) validPC() error {
	if s.scriptidx >= len(s.scripts) {
		return fmt.Errorf("Past input scripts %v:%v %v:xxxx", s.scriptidx, s.scriptoff, len(s.scripts))
	}
	if s.scriptoff >= len(s.scripts[s.scriptidx]) {
		return fmt.Errorf("Past input scripts %v:%v %v:%04d", s.scriptidx, s.scriptoff, s.scriptidx, len(s.scripts[s.scriptidx]))
	}
	return nil
}

// DisasmScript returns the disassembly string for the script at offset
// ``idx''.  Where 0 is the scriptSig and 1 is the scriptPubKey.
func (s *Script) DisasmScript(idx int) (disstr string, err error) {
	if idx >= len(s.scripts) {
		return "", ErrStackInvalidIndex
	}
	for i := range s.scripts[idx] {
		disstr = disstr + s.disasm(idx, i) + "\n"
	}
	return disstr, nil
}

// DisasmPC returns the string for the disassembly of the opcode that will be
// next to execute when Step() is called.
func (s *Script) DisasmPC() (disstr string, err error) {
	scriptidx, scriptoff, err := s.curPC()
	if err != nil {
		return "", err
	}
	return s.disasm(scriptidx, scriptoff), nil
}

// disasm is a helper member to produce the output for DisasmPC and
// DisasmScript. It produces the opcode prefixed by the program counter at the
// provided position in the script. it does no error checking and leaves that
// to the caller to provide a valid offse.
func (s *Script) disasm(scriptidx int, scriptoff int) string {
	return fmt.Sprintf("%02x:%04x: %s", scriptidx, scriptoff,
		s.scripts[scriptidx][scriptoff].print(false))
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
		if !canonicalPush(pop) || !bytes.Contains(pop.data, data) {
			retScript = append(retScript, pop)
		}
	}
	return retScript

}

// DisasmString formats a disassembled script for one line printing.  When the
// script fails to parse, the returned string will contain the disassembled
// script up to the point the failure occurred along with the string '[error]'
// appended.  In addition, the reason the script failed to parse is returned
// if the caller wants more information about the failure.
func DisasmString(buf []byte) (string, error) {
	disbuf := ""
	opcodes, err := parseScript(buf)
	for _, pop := range opcodes {
		disbuf += pop.print(true) + " "
	}
	if disbuf != "" {
		disbuf = disbuf[:len(disbuf)-1]
	}
	if err != nil {
		disbuf += "[error]"
	}
	return disbuf, err
}

// calcScriptHash will, given the a script and hashtype for the current
// scriptmachine, calculate the doubleSha256 hash of the transaction and
// script to be used for signature signing and verification.
func calcScriptHash(script []parsedOpcode, hashType SigHashType, tx *wire.MsgTx, idx int) []byte {

	// remove all instances of OP_CODESEPARATOR still left in the script
	script = removeOpcode(script, OP_CODESEPARATOR)

	// Make a deep copy of the transaction, zeroing out the script
	// for all inputs that are not currently being processed.
	txCopy := tx.Copy()
	for i := range txCopy.TxIn {
		var txIn wire.TxIn
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
		var txOut wire.TxOut
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

	return wire.DoubleSha256(wbuf.Bytes())
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
		case OP_CHECKMULTISIG:
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

// payToPubKeyHashScript creates a new script to pay a transaction
// output to a 20-byte pubkey hash. It is expected that the input is a valid
// hash.
func payToPubKeyHashScript(pubKeyHash []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_DUP).AddOp(OP_HASH160).
		AddData(pubKeyHash).AddOp(OP_EQUALVERIFY).AddOp(OP_CHECKSIG).
		Script()
}

// payToScriptHashScript creates a new script to pay a transaction output to a
// script hash. It is expected that the input is a valid hash.
func payToScriptHashScript(scriptHash []byte) ([]byte, error) {
	return NewScriptBuilder().AddOp(OP_HASH160).AddData(scriptHash).
		AddOp(OP_EQUAL).Script()
}

// payToPubkeyScript creates a new script to pay a transaction output to a
// public key. It is expected that the input is a valid pubkey.
func payToPubKeyScript(serializedPubKey []byte) ([]byte, error) {
	return NewScriptBuilder().AddData(serializedPubKey).
		AddOp(OP_CHECKSIG).Script()
}

// PayToAddrScript creates a new script to pay a transaction output to a the
// specified address.
func PayToAddrScript(addr btcutil.Address) ([]byte, error) {
	switch addr := addr.(type) {
	case *btcutil.AddressPubKeyHash:
		if addr == nil {
			return nil, ErrUnsupportedAddress
		}
		return payToPubKeyHashScript(addr.ScriptAddress())

	case *btcutil.AddressScriptHash:
		if addr == nil {
			return nil, ErrUnsupportedAddress
		}
		return payToScriptHashScript(addr.ScriptAddress())

	case *btcutil.AddressPubKey:
		if addr == nil {
			return nil, ErrUnsupportedAddress
		}
		return payToPubKeyScript(addr.ScriptAddress())
	}

	return nil, ErrUnsupportedAddress
}

// ErrBadNumRequired is returned from MultiSigScript when nrequired is larger
// than the number of provided public keys.
var ErrBadNumRequired = errors.New("more signatures required than keys present")

// MultiSigScript returns a valid script for a multisignature redemption where
// nrequired of the keys in pubkeys are required to have signed the transaction
// for success. An ErrBadNumRequired will be returned if nrequired is larger than
// the number of keys provided.
func MultiSigScript(pubkeys []*btcutil.AddressPubKey, nrequired int) ([]byte, error) {
	if len(pubkeys) < nrequired {
		return nil, ErrBadNumRequired
	}

	builder := NewScriptBuilder().AddInt64(int64(nrequired))
	for _, key := range pubkeys {
		builder.AddData(key.ScriptAddress())
	}
	builder.AddInt64(int64(len(pubkeys)))
	builder.AddOp(OP_CHECKMULTISIG)

	return builder.Script()
}

// SignatureScript creates an input signature script for tx to spend
// BTC sent from a previous output to the owner of privKey. tx must
// include all transaction inputs and outputs, however txin scripts are
// allowed to be filled or empty. The returned script is calculated to
// be used as the idx'th txin sigscript for tx. subscript is the PkScript
// of the previous output being used as the idx'th input. privKey is
// serialized in either a compressed or uncompressed format based on
// compress. This format must match the same format used to generate
// the payment address, or the script validation will fail.
func SignatureScript(tx *wire.MsgTx, idx int, subscript []byte, hashType SigHashType, privKey *btcec.PrivateKey, compress bool) ([]byte, error) {
	sig, err := RawTxInSignature(tx, idx, subscript, hashType, privKey)
	if err != nil {
		return nil, err
	}

	pk := (*btcec.PublicKey)(&privKey.PublicKey)
	var pkData []byte
	if compress {
		pkData = pk.SerializeCompressed()
	} else {
		pkData = pk.SerializeUncompressed()
	}

	return NewScriptBuilder().AddData(sig).AddData(pkData).Script()
}

// RawTxInSignature returns the serialized ECDSA signature for the input
// idx of the given transaction, with hashType appended to it.
func RawTxInSignature(tx *wire.MsgTx, idx int, subScript []byte,
	hashType SigHashType, key *btcec.PrivateKey) ([]byte, error) {
	parsedScript, err := parseScript(subScript)
	if err != nil {
		return nil, fmt.Errorf("cannot parse output script: %v", err)
	}
	hash := calcScriptHash(parsedScript, hashType, tx, idx)
	signature, err := key.Sign(hash)
	if err != nil {
		return nil, fmt.Errorf("cannot sign tx input: %s", err)
	}

	return append(signature.Serialize(), byte(hashType)), nil
}

func p2pkSignatureScript(tx *wire.MsgTx, idx int, subScript []byte, hashType SigHashType, privKey *btcec.PrivateKey) ([]byte, error) {
	sig, err := RawTxInSignature(tx, idx, subScript, hashType, privKey)
	if err != nil {
		return nil, err
	}

	return NewScriptBuilder().AddData(sig).Script()
}

// signMultiSig signs as many of the outputs in the provided multisig script as
// possible. It returns the generated script and a boolean if the script fulfils
// the contract (i.e. nrequired signatures are provided).  Since it is arguably
// legal to not be able to sign any of the outputs, no error is returned.
func signMultiSig(tx *wire.MsgTx, idx int, subScript []byte, hashType SigHashType,
	addresses []btcutil.Address, nRequired int, kdb KeyDB) ([]byte, bool) {
	// We start with a single OP_FALSE to work around the (now standard)
	// but in the reference implementation that causes a spurious pop at
	// the end of OP_CHECKMULTISIG.
	builder := NewScriptBuilder().AddOp(OP_FALSE)
	signed := 0
	for _, addr := range addresses {
		key, _, err := kdb.GetKey(addr)
		if err != nil {
			continue
		}
		sig, err := RawTxInSignature(tx, idx, subScript, hashType, key)
		if err != nil {
			continue
		}

		builder.AddData(sig)
		signed++
		if signed == nRequired {
			break
		}

	}

	script, _ := builder.Script()
	return script, signed == nRequired
}

func sign(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int,
	subScript []byte, hashType SigHashType, kdb KeyDB, sdb ScriptDB) ([]byte,
	ScriptClass, []btcutil.Address, int, error) {

	class, addresses, nrequired, err := ExtractPkScriptAddrs(subScript,
		chainParams)
	if err != nil {
		return nil, NonStandardTy, nil, 0, err
	}

	switch class {
	case PubKeyTy:
		// look up key for address
		key, _, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		script, err := p2pkSignatureScript(tx, idx, subScript, hashType,
			key)
		if err != nil {
			return nil, class, nil, 0, err
		}

		return script, class, addresses, nrequired, nil
	case PubKeyHashTy:
		// look up key for address
		key, compressed, err := kdb.GetKey(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		script, err := SignatureScript(tx, idx, subScript, hashType,
			key, compressed)
		if err != nil {
			return nil, class, nil, 0, err
		}

		return script, class, addresses, nrequired, nil
	case ScriptHashTy:
		script, err := sdb.GetScript(addresses[0])
		if err != nil {
			return nil, class, nil, 0, err
		}

		return script, class, addresses, nrequired, nil
	case MultiSigTy:
		script, _ := signMultiSig(tx, idx, subScript, hashType,
			addresses, nrequired, kdb)
		return script, class, addresses, nrequired, nil
	case NullDataTy:
		return nil, class, nil, 0,
			errors.New("can't sign NULLDATA transactions")
	default:
		return nil, class, nil, 0,
			errors.New("can't sign unknown transactions")
	}
}

// mergeScripts merges sigScript and prevScript assuming they are both
// partial solutions for pkScript spending output idx of tx. class, addresses
// and nrequired are the result of extracting the addresses from pkscript.
// The return value is the best effort merging of the two scripts. Calling this
// function with addresses, class and nrequired that do not match pkScript is
// an error and results in undefined behaviour.
func mergeScripts(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int,
	pkScript []byte, class ScriptClass, addresses []btcutil.Address,
	nRequired int, sigScript, prevScript []byte) []byte {

	// TODO(oga) the scripthash and multisig paths here are overly
	// inefficient in that they will recompute already known data.
	// some internal refactoring could probably make this avoid needless
	// extra calculations.
	switch class {
	case ScriptHashTy:
		// Remove the last push in the script and then recurse.
		// this could be a lot less inefficient.
		sigPops, err := parseScript(sigScript)
		if err != nil || len(sigPops) == 0 {
			return prevScript
		}
		prevPops, err := parseScript(prevScript)
		if err != nil || len(prevPops) == 0 {
			return sigScript
		}

		// assume that script in sigPops is the correct one, we just
		// made it.
		script := sigPops[len(sigPops)-1].data

		// We already know this information somewhere up the stack.
		class, addresses, nrequired, err :=
			ExtractPkScriptAddrs(script, chainParams)

		// regenerate scripts.
		sigScript, _ := unparseScript(sigPops)
		prevScript, _ := unparseScript(prevPops)

		// Merge
		mergedScript := mergeScripts(chainParams, tx, idx, script,
			class, addresses, nrequired, sigScript, prevScript)

		// Reappend the script and return the result.
		builder := NewScriptBuilder()
		builder.script = mergedScript
		builder.AddData(script)
		finalScript, _ := builder.Script()
		return finalScript
	case MultiSigTy:
		return mergeMultiSig(tx, idx, addresses, nRequired, pkScript,
			sigScript, prevScript)

	// It doesn't actualy make sense to merge anything other than multiig
	// and scripthash (because it could contain multisig). Everything else
	// has either zero signature, can't be spent, or has a single signature
	// which is either present or not. The other two cases are handled
	// above. In the conflict case here we just assume the longest is
	// correct (this matches behaviour of the reference implementation).
	default:
		if len(sigScript) > len(prevScript) {
			return sigScript
		}
		return prevScript
	}
}

// mergeMultiSig combines the two signature scripts sigScript and prevScript
// that both provide signatures for pkScript in output idx of tx. addresses
// and nRequired should be the results from extracting the addresses from
// pkScript. Since this function is internal only we assume that the arguments
// have come from other functions internally and thus are all consistent with
// each other, behaviour is undefined if this contract is broken.
func mergeMultiSig(tx *wire.MsgTx, idx int, addresses []btcutil.Address,
	nRequired int, pkScript, sigScript, prevScript []byte) []byte {

	// This is an internal only function and we already parsed this script
	// as ok for multisig (this is how we got here), so if this fails then
	// all assumptions are broken and who knows which way is up?
	pkPops, _ := parseScript(pkScript)

	sigPops, err := parseScript(sigScript)
	if err != nil || len(sigPops) == 0 {
		return prevScript
	}

	prevPops, err := parseScript(prevScript)
	if err != nil || len(prevPops) == 0 {
		return sigScript
	}

	// Convenience function to avoid duplication.
	extractSigs := func(pops []parsedOpcode, sigs [][]byte) [][]byte {
		for _, pop := range pops {
			if len(pop.data) != 0 {
				sigs = append(sigs, pop.data)
			}
		}
		return sigs
	}

	possibleSigs := make([][]byte, 0, len(sigPops)+len(prevPops))
	possibleSigs = extractSigs(sigPops, possibleSigs)
	possibleSigs = extractSigs(prevPops, possibleSigs)

	// Now we need to match the signatures to pubkeys, the only real way to
	// do that is to try to verify them all and match it to the pubkey
	// that verifies it. we then can go through the addresses in order
	// to build our script. Anything that doesn't parse or doesn't verify we
	// throw away.
	addrToSig := make(map[string][]byte)
sigLoop:
	for _, sig := range possibleSigs {

		// can't have a valid signature that doesn't at least have a
		// hashtype, in practise it is even longer than this. but
		// that'll be checked next.
		if len(sig) < 1 {
			continue
		}
		tSig := sig[:len(sig)-1]
		hashType := SigHashType(sig[len(sig)-1])

		pSig, err := btcec.ParseDERSignature(tSig, btcec.S256())
		if err != nil {
			continue
		}

		// We have to do this each round since hash types may vary
		// between signatures and so the hash will vary. We can,
		// however, assume no sigs etc are in the script since that
		// would make the transaction nonstandard and thus not
		// MultiSigTy, so we just need to hash the full thing.
		hash := calcScriptHash(pkPops, hashType, tx, idx)

		for _, addr := range addresses {
			// All multisig addresses should be pubkey addreses
			// it is an error to call this internal function with
			// bad input.
			pkaddr := addr.(*btcutil.AddressPubKey)

			pubKey := pkaddr.PubKey()

			// If it matches we put it in the map. We only
			// can take one signature per public key so if we
			// already have one, we can throw this away.
			if pSig.Verify(hash, pubKey) {
				aStr := addr.EncodeAddress()
				if _, ok := addrToSig[aStr]; !ok {
					addrToSig[aStr] = sig
				}
				continue sigLoop
			}
		}
	}

	// Extra opcode to handle the extra arg consumed (due to previous bugs
	// in the reference implementation).
	builder := NewScriptBuilder().AddOp(OP_FALSE)
	doneSigs := 0
	// This assumes that addresses are in the same order as in the script.
	for _, addr := range addresses {
		sig, ok := addrToSig[addr.EncodeAddress()]
		if !ok {
			continue
		}
		builder.AddData(sig)
		doneSigs++
		if doneSigs == nRequired {
			break
		}
	}

	// padding for missing ones.
	for i := doneSigs; i < nRequired; i++ {
		builder.AddOp(OP_0)
	}

	script, _ := builder.Script()
	return script
}

// KeyDB is an interface type provided to SignTxOutput, it encapsulates
// any user state required to get the private keys for an address.
type KeyDB interface {
	GetKey(btcutil.Address) (*btcec.PrivateKey, bool, error)
}

// KeyClosure implements ScriptDB with a closure
type KeyClosure func(btcutil.Address) (*btcec.PrivateKey, bool, error)

// GetKey implements KeyDB by returning the result of calling the closure
func (kc KeyClosure) GetKey(address btcutil.Address) (*btcec.PrivateKey,
	bool, error) {
	return kc(address)
}

// ScriptDB is an interface type provided to SignTxOutput, it encapsulates
// any user state required to get the scripts for an pay-to-script-hash address.
type ScriptDB interface {
	GetScript(btcutil.Address) ([]byte, error)
}

// ScriptClosure implements ScriptDB with a closure
type ScriptClosure func(btcutil.Address) ([]byte, error)

// GetScript implements ScriptDB by returning the result of calling the closure
func (sc ScriptClosure) GetScript(address btcutil.Address) ([]byte, error) {
	return sc(address)
}

// SignTxOutput signs output idx of the given tx to resolve the script given in
// pkScript with a signature type of hashType. Any keys required will be
// looked up by calling getKey() with the string of the given address.
// Any pay-to-script-hash signatures will be similarly looked up by calling
// getScript. If previousScript is provided then the results in previousScript
// will be merged in a type-dependant manner with the newly generated.
// signature script.
func SignTxOutput(chainParams *chaincfg.Params, tx *wire.MsgTx, idx int,
	pkScript []byte, hashType SigHashType, kdb KeyDB, sdb ScriptDB,
	previousScript []byte) ([]byte, error) {

	sigScript, class, addresses, nrequired, err := sign(chainParams, tx,
		idx, pkScript, hashType, kdb, sdb)
	if err != nil {
		return nil, err
	}

	if class == ScriptHashTy {
		// TODO keep the sub addressed and pass down to merge.
		realSigScript, _, _, _, err := sign(chainParams, tx, idx,
			sigScript, hashType, kdb, sdb)
		if err != nil {
			return nil, err
		}

		// This is a bad thing. Append the p2sh script as the last
		// push in the script.
		builder := NewScriptBuilder()
		builder.script = realSigScript
		builder.AddData(sigScript)

		sigScript, _ = builder.Script()
		// TODO keep a copy of the script for merging.
	}

	// Merge scripts. with any previous data, if any.
	mergedScript := mergeScripts(chainParams, tx, idx, pkScript, class,
		addresses, nrequired, sigScript, previousScript)
	return mergedScript, nil
}

// expectedInputs returns the number of arguments required by a script.
// If the script is of unnown type such that the number can not be determined
// then -1 is returned. We are an internal function and thus assume that class
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
		// of sigs and number of keys.  Check the first push instruction
		// to see how many arguments are expected. typeOfScript already
		// checked this so we know it'll be a small int.  Also, due to
		// the original bitcoind bug where OP_CHECKMULTISIG pops an
		// additional item from the stack, add an extra expected input
		// for the extra push that is required to compensate.
		return asSmallInt(pops[0].opcode) + 1
	case NullDataTy:
		fallthrough
	default:
		return -1
	}
}

// ScriptInfo houses information about a script pair that is determined by
// CalcScriptInfo.
type ScriptInfo struct {
	// The class of the sigscript, equivalent to calling GetScriptClass
	// on the sigScript.
	PkScriptClass ScriptClass

	// NumInputs is the number of inputs provided by the pkScript.
	NumInputs int

	// ExpectedInputs is the number of outputs required by sigScript and any
	// pay-to-script-hash scripts. The number will be -1 if unknown.
	ExpectedInputs int

	// SigOps is the nubmer of signature operations in the script pair.
	SigOps int
}

// CalcScriptInfo returns a structure providing data about the scriptpair that
// are provided as arguments. It will error if the pair is in someway invalid
// such that they can not be analysed, i.e. if they do not parse or the
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
		return nil, ErrStackNonPushOnly
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

// asSmallInt returns the passed opcode, which must be true according to
// isSmallInt(), as an integer.
func asSmallInt(op *opcode) int {
	if op.value == OP_0 {
		return 0
	}

	return int(op.value - (OP_1 - 1))
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
		return 0, 0, ErrStackUnderflow
	}

	numSigs := asSmallInt(pops[0].opcode)
	numPubKeys := asSmallInt(pops[len(pops)-2].opcode)
	return numPubKeys, numSigs, nil
}

// PushedData returns an array of byte slices containing any pushed data found
// in the passed script.  This includes OP_0, but not OP_1 - OP_16.
func PushedData(script []byte) ([][]byte, error) {
	pops, err := parseScript(script)
	if err != nil {
		return nil, err
	}
	var data [][]byte
	for _, pop := range pops {
		if pop.data != nil {
			data = append(data, pop.data)
		} else if pop.opcode.value == OP_0 {
			data = append(data, []byte{})
		}
	}
	return data, nil
}
