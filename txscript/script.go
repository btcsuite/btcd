// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
)

const (
	// maxDataCarrierSize is the maximum number of bytes allowed in pushed
	// data to be considered a nulldata transaction
	maxDataCarrierSize = 80
)

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

	// sigHashMask defines the number of bits of the hash type which is used
	// to identify which outputs are signed.
	sigHashMask = 0x1f
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

// scriptClassToName houses the human-readable strings which describe each
// script class.
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

// isSmallInt returns whether or not the opcode is considered a small integer,
// which is an OP_0, or OP_1 through OP_16.
func isSmallInt(op *opcode) bool {
	if op.value == OP_0 || (op.value >= OP_1 && op.value <= OP_16) {
		return true
	}
	return false
}

// isPubkey returns true if the script passed is a pay-to-pubkey transaction,
// false otherwise.
func isPubkey(pops []parsedOpcode) bool {
	// Valid pubkeys are either 33 or 65 bytes.
	return len(pops) == 2 &&
		(len(pops[0].data) == 33 || len(pops[0].data) == 65) &&
		pops[1].opcode.value == OP_CHECKSIG
}

// isPubkeyHash returns true if the script passed is a pay-to-pubkey-hash
// transaction, false otherwise.
func isPubkeyHash(pops []parsedOpcode) bool {
	return len(pops) == 5 &&
		pops[0].opcode.value == OP_DUP &&
		pops[1].opcode.value == OP_HASH160 &&
		pops[2].opcode.value == OP_DATA_20 &&
		pops[3].opcode.value == OP_EQUALVERIFY &&
		pops[4].opcode.value == OP_CHECKSIG

}

// isScriptHash returns true if the script passed is a pay-to-script-hash
// transaction, false otherwise.
func isScriptHash(pops []parsedOpcode) bool {
	return len(pops) == 3 &&
		pops[0].opcode.value == OP_HASH160 &&
		pops[1].opcode.value == OP_DATA_20 &&
		pops[2].opcode.value == OP_EQUAL
}

// IsPayToScriptHash returns true if the script is in the standard
// pay-to-script-hash (P2SH) format, false otherwise.
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
	// The absolute minimum is 1 pubkey:
	// OP_0/OP_1-16 <pubkey> OP_1 OP_CHECKMULTISIG
	l := len(pops)
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
		// Valid pubkeys are either 33 or 65 bytes.
		if len(pop.data) != 33 && len(pop.data) != 65 {
			return false
		}
	}
	return true
}

// isNullData returns true if the passed script is a null data transaction,
// false otherwise.
func isNullData(pops []parsedOpcode) bool {
	// A nulldata transaction is either a single OP_RETURN or an
	// OP_RETURN SMALLDATA (where SMALLDATA is a data push up to
	// maxDataCarrierSize bytes).
	l := len(pops)
	if l == 1 && pops[0].opcode.value == OP_RETURN {
		return true
	}

	return l == 2 &&
		pops[0].opcode.value == OP_RETURN &&
		pops[1].opcode.value <= OP_PUSHDATA4 &&
		len(pops[1].data) <= maxDataCarrierSize
}

// isPushOnly returns true if the script only pushes data, false otherwise.
func isPushOnly(pops []parsedOpcode) bool {
	// NOTE: This function does NOT verify opcodes directly since it is
	// internal and is only called with parsed opcodes for scripts that did
	// not have any parse errors.  Thus, consensus is properly maintained.

	for _, pop := range pops {
		// All opcodes up to OP_16 are data push instructions.
		// NOTE: This does consider OP_RESERVED to be a data push
		// instruction, but execution of OP_RESERVED will fail anyways
		// and matches the behavior required by consensus.
		if pop.opcode.value > OP_16 {
			return false
		}
	}
	return true
}

// IsPushOnlyScript returns whether or not the passed script only pushes data.
//
// False will be returned when the script does not parse.
func IsPushOnlyScript(script []byte) bool {
	pops, err := parseScript(script)
	if err != nil {
		return false
	}
	return isPushOnly(pops)
}

// scriptType returns the type of the script being inspected from the known
// standard types.
func typeOfScript(pops []parsedOpcode) ScriptClass {
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

// GetScriptClass returns the class of the script passed.
//
// NonStandardTy will be returned when the script does not parse.
func GetScriptClass(script []byte) ScriptClass {
	pops, err := parseScript(script)
	if err != nil {
		return NonStandardTy
	}
	return typeOfScript(pops)
}

// parseScriptTemplate is the same as parseScript but allows the passing of the
// template list for testing purposes.  When there are parse errors, it returns
// the list of parsed opcodes up to the point of failure along with the error.
func parseScriptTemplate(script []byte, opcodes *[256]opcode) ([]parsedOpcode, error) {
	retScript := make([]parsedOpcode, 0, len(script))
	for i := 0; i < len(script); {
		instr := script[i]
		op := opcodes[instr]
		pop := parsedOpcode{opcode: &op}

		// Parse data out of instruction.
		switch {
		// No additional data.  Note that some of the opcodes, notably
		// OP_1NEGATE, OP_0, and OP_[1-16] represent the data
		// themselves.
		case op.length == 1:
			i++

		// Data pushes of specific lengths -- OP_DATA_[1-75].
		case op.length > 1:
			if len(script[i:]) < op.length {
				return retScript, ErrStackShortScript
			}

			// Slice out the data.
			pop.data = script[i+1 : i+op.length]
			i += op.length

		// Data pushes with parsed lengths -- OP_PUSHDATAP{1,2,4}.
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

			// Move offset to beginning of the data.
			off += -op.length

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

// parseScript preparses the script in bytes into a list of parsedOpcodes while
// applying a number of sanity checks.
func parseScript(script []byte) ([]parsedOpcode, error) {
	return parseScriptTemplate(script, &opcodeArray)
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

// removeOpcodeByData will return the script minus any opcodes that would push
// the passed data to the stack.
func removeOpcodeByData(pkscript []parsedOpcode, data []byte) []parsedOpcode {
	retScript := make([]parsedOpcode, 0, len(pkscript))
	for _, pop := range pkscript {
		if !canonicalPush(pop) || !bytes.Contains(pop.data, data) {
			retScript = append(retScript, pop)
		}
	}
	return retScript

}

// calcSignatureHash will, given a script and hash type for the current script
// engine instance, calculate the signature hash to be used for signing and
// verification.
func calcSignatureHash(script []parsedOpcode, hashType SigHashType, tx *wire.MsgTx, idx int) []byte {
	// The SigHashSingle signature type signs only the corresponding input
	// and output (the output with the same index number as the input).
	//
	// Since transactions can have more inputs than outputs, this means it
	// is improper to use SigHashSingle on input indices that don't have a
	// corresponding output.
	//
	// A bug in the original Satoshi client implementation means specifying
	// an index that is out of range results in a signature hash of 1 (as a
	// uint256 little endian).  The original intent appeared to be to
	// indicate failure, but unfortunately, it was never checked and thus is
	// treated as the actual signature hash.  This buggy behavior is now
	// part of the consensus and a hard fork would be required to fix it.
	//
	// Due to this, care must be taken by software that creates transactions
	// which make use of SigHashSingle because it can lead to an extremely
	// dangerous situation where the invalid inputs will end up signing a
	// hash of 1.  This in turn presents an opportunity for attackers to
	// cleverly construct transactions which can steal those coins provided
	// they can reuse signatures.
	if hashType&sigHashMask == SigHashSingle && idx >= len(tx.TxOut) {
		var hash wire.ShaHash
		hash[0] = 0x01
		return hash[:]
	}

	// Remove all instances of OP_CODESEPARATOR from the script.
	script = removeOpcode(script, OP_CODESEPARATOR)

	// Make a deep copy of the transaction, zeroing out the script for all
	// inputs that are not currently being processed.
	txCopy := tx.Copy()
	for i := range txCopy.TxIn {
		var txIn wire.TxIn
		txIn = *txCopy.TxIn[i]
		txCopy.TxIn[i] = &txIn
		if i == idx {
			// UnparseScript cannot fail here because removeOpcode
			// above only returns a valid script.
			sigScript, _ := unparseScript(script)
			txCopy.TxIn[idx].SignatureScript = sigScript
		} else {
			txCopy.TxIn[i].SignatureScript = nil
		}
	}

	// Default behavior has all outputs set up.
	for i := range txCopy.TxOut {
		var txOut wire.TxOut
		txOut = *txCopy.TxOut[i]
		txCopy.TxOut[i] = &txOut
	}

	switch hashType & sigHashMask {
	case SigHashNone:
		txCopy.TxOut = txCopy.TxOut[0:0] // Empty slice.
		for i := range txCopy.TxIn {
			if i != idx {
				txCopy.TxIn[i].Sequence = 0
			}
		}

	case SigHashSingle:
		// Resize output array to up to and including requested index.
		txCopy.TxOut = txCopy.TxOut[:idx+1]

		// All but current output get zeroed out.
		for i := 0; i < idx; i++ {
			txCopy.TxOut[i].Value = -1
			txCopy.TxOut[i].PkScript = nil
		}

		// Sequence on all other inputs is 0, too.
		for i := range txCopy.TxIn {
			if i != idx {
				txCopy.TxIn[i].Sequence = 0
			}
		}

	default:
		// Consensus treats undefined hashtypes like normal SigHashAll
		// for purposes of hash generation.
		fallthrough
	case SigHashOld:
		fallthrough
	case SigHashAll:
		// Nothing special here.
	}
	if hashType&SigHashAnyOneCanPay != 0 {
		txCopy.TxIn = txCopy.TxIn[idx : idx+1]
		idx = 0
	}

	// The final hash is the double sha256 of both the serialized modified
	// transaction and the hash type (encoded as a 4-byte little-endian
	// value) appended.
	var wbuf bytes.Buffer
	txCopy.Serialize(&wbuf)
	binary.Write(&wbuf, binary.LittleEndian, uint32(hashType))
	return wire.DoubleSha256(wbuf.Bytes())
}

// asSmallInt returns the passed opcode, which must be true according to
// isSmallInt(), as an integer.
func asSmallInt(op *opcode) int {
	if op.value == OP_0 {
		return 0
	}

	return int(op.value - (OP_1 - 1))
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
				nSigs += asSmallInt(pops[i-1].opcode)
			} else {
				nSigs += MaxPubKeysPerMultiSig
			}
		default:
			// Not a sigop.
		}
	}

	return nSigs
}

// GetSigOpCount provides a quick count of the number of signature operations
// in a script. a CHECKSIG operations counts for 1, and a CHECK_MULTISIG for 20.
// If the script fails to parse, then the count up to the point of failure is
// returned.
func GetSigOpCount(script []byte) int {
	// Don't check error since parseScript returns the parsed-up-to-error
	// list of pops.
	pops, _ := parseScript(script)
	return getSigOpCount(pops, false)
}

// GetPreciseSigOpCount returns the number of signature operations in
// scriptPubKey.  If bip16 is true then scriptSig may be searched for the
// Pay-To-Script-Hash script in order to find the precise number of signature
// operations in the transaction.  If the script fails to parse, then the count
// up to the point of failure is returned.
func GetPreciseSigOpCount(scriptSig, scriptPubKey []byte, bip16 bool) int {
	// Don't check error since parseScript returns the parsed-up-to-error
	// list of pops.
	pops, _ := parseScript(scriptPubKey)

	// Treat non P2SH transactions as normal.
	if !(bip16 && isScriptHash(pops)) {
		return getSigOpCount(pops, true)
	}

	// The public key script is a pay-to-script-hash, so parse the signature
	// script to get the final item.  Scripts that fail to fully parse count
	// as 0 signature operations.
	sigPops, err := parseScript(scriptSig)
	if err != nil {
		return 0
	}

	// The signature script must only push data to the stack for P2SH to be
	// a valid pair, so the signature operation count is 0 when that is not
	// the case.
	if !isPushOnly(sigPops) || len(sigPops) == 0 {
		return 0
	}

	// The P2SH script is the last item the signature script pushes to the
	// stack.  When the script is empty, there are no signature operations.
	shScript := sigPops[len(sigPops)-1].data
	if len(shScript) == 0 {
		return 0
	}

	// Parse the P2SH script and don't check the error since parseScript
	// returns the parsed-up-to-error list of pops and the consensus rules
	// dictate signature operations are counted up to the first parse
	// failure.
	shPops, _ := parseScript(shScript)
	return getSigOpCount(shPops, true)
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

// MultiSigScript returns a valid script for a multisignature redemption where
// nrequired of the keys in pubkeys are required to have signed the transaction
// for success.  An ErrBadNumRequired will be returned if nrequired is larger
// than the number of keys provided.
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

// expectedInputs returns the number of arguments required by a script.
// If the script is of unknown type such that the number can not be determined
// then -1 is returned. We are an internal function and thus assume that class
// is the real class of pops (and we can thus assume things that were determined
// while finding out the type).
func expectedInputs(pops []parsedOpcode, class ScriptClass) int {
	switch class {
	case PubKeyTy:
		return 1

	case PubKeyHashTy:
		return 2

	case ScriptHashTy:
		// Not including script.  That is handled by the caller.
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
	// PkScriptClass is the class of the public key script and is equivalent
	// to calling GetScriptClass on it.
	PkScriptClass ScriptClass

	// NumInputs is the number of inputs provided by the public key script.
	NumInputs int

	// ExpectedInputs is the number of outputs required by the signature
	// script and any pay-to-script-hash scripts. The number will be -1 if
	// unknown.
	ExpectedInputs int

	// SigOps is the number of signature operations in the script pair.
	SigOps int
}

// CalcScriptInfo returns a structure providing data about the provided script
// pair.  It will error if the pair is in someway invalid such that they can not
// be analysed, i.e. if they do not parse or the pkScript is not a push-only
// script
func CalcScriptInfo(sigScript, pkScript []byte, bip16 bool) (*ScriptInfo, error) {
	sigPops, err := parseScript(sigScript)
	if err != nil {
		return nil, err
	}

	pkPops, err := parseScript(pkScript)
	if err != nil {
		return nil, err
	}

	// Push only sigScript makes little sense.
	si := new(ScriptInfo)
	si.PkScriptClass = typeOfScript(pkPops)

	// Can't have a pkScript that doesn't just push data.
	if !isPushOnly(sigPops) {
		return nil, ErrStackNonPushOnly
	}

	si.ExpectedInputs = expectedInputs(pkPops, si.PkScriptClass)

	// All entries pushed to stack (or are OP_RESERVED and exec will fail).
	si.NumInputs = len(sigPops)

	// Count sigops taking into account pay-to-script-hash.
	if si.PkScriptClass == ScriptHashTy && bip16 {
		// The pay-to-hash-script is the final data push of the
		// signature script.
		script := sigPops[len(sigPops)-1].data
		shPops, err := parseScript(script)
		if err != nil {
			return nil, err
		}

		shInputs := expectedInputs(shPops, typeOfScript(shPops))
		if shInputs == -1 {
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
