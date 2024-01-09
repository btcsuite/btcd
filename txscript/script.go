// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"fmt"
	"strings"
	"time"

	"github.com/btcsuite/btcd/wire"
)

// Bip16Activation is the timestamp where BIP0016 is valid to use in the
// blockchain.  To be used to determine if BIP0016 should be called for or not.
// This timestamp corresponds to Sun Apr 1 00:00:00 UTC 2012.
var Bip16Activation = time.Unix(1333238400, 0)

const (
	// TaprootAnnexTag is the tag for an annex. This value is used to
	// identify the annex during tapscript spends. If there're at least two
	// elements in the taproot witness stack, and the first byte of the
	// last element matches this tag, then we'll extract this as a distinct
	// item.
	TaprootAnnexTag = 0x50

	// TaprootLeafMask is the mask applied to the control block to extract
	// the leaf version and parity of the y-coordinate of the output key if
	// the taproot script leaf being spent.
	TaprootLeafMask = 0xfe
)

// These are the constants specified for maximums in individual scripts.
const (
	MaxOpsPerScript       = 201 // Max number of non-push operations.
	MaxPubKeysPerMultiSig = 20  // Multisig can't have more sigs than this.
	MaxScriptElementSize  = 520 // Max bytes pushable to the stack.
)

// IsSmallInt returns whether or not the opcode is considered a small integer,
// which is an OP_0, or OP_1 through OP_16.
//
// NOTE: This function is only valid for version 0 opcodes.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func IsSmallInt(op byte) bool {
	return op == OP_0 || (op >= OP_1 && op <= OP_16)
}

// IsPayToPubKey returns true if the script is in the standard pay-to-pubkey
// (P2PK) format, false otherwise.
func IsPayToPubKey(script []byte) bool {
	return isPubKeyScript(script)
}

// IsPayToPubKeyHash returns true if the script is in the standard
// pay-to-pubkey-hash (P2PKH) format, false otherwise.
func IsPayToPubKeyHash(script []byte) bool {
	return isPubKeyHashScript(script)
}

// IsPayToScriptHash returns true if the script is in the standard
// pay-to-script-hash (P2SH) format, false otherwise.
//
// WARNING: This function always treats the passed script as version 0.  Great
// care must be taken if introducing a new script version because it is used in
// consensus which, unfortunately as of the time of this writing, does not check
// script versions before determining if the script is a P2SH which means nodes
// on existing rules will analyze new version scripts as if they were version 0.
func IsPayToScriptHash(script []byte) bool {
	return isScriptHashScript(script)
}

// IsPayToWitnessScriptHash returns true if the script is in the standard
// pay-to-witness-script-hash (P2WSH) format, false otherwise.
func IsPayToWitnessScriptHash(script []byte) bool {
	return isWitnessScriptHashScript(script)
}

// IsPayToWitnessPubKeyHash returns true if the script is in the standard
// pay-to-witness-pubkey-hash (P2WKH) format, false otherwise.
func IsPayToWitnessPubKeyHash(script []byte) bool {
	return isWitnessPubKeyHashScript(script)
}

// IsPayToTaproot returns true if the passed script is a standard
// pay-to-taproot (PTTR) scripts, and false otherwise.
func IsPayToTaproot(script []byte) bool {
	return isWitnessTaprootScript(script)
}

// IsWitnessProgram returns true if the passed script is a valid witness
// program which is encoded according to the passed witness program version. A
// witness program must be a small integer (from 0-16), followed by 2-40 bytes
// of pushed data.
func IsWitnessProgram(script []byte) bool {
	return isWitnessProgramScript(script)
}

// IsNullData returns true if the passed script is a null data script, false
// otherwise.
func IsNullData(script []byte) bool {
	const scriptVersion = 0
	return isNullDataScript(scriptVersion, script)
}

// ExtractWitnessProgramInfo attempts to extract the witness program version,
// as well as the witness program itself from the passed script.
func ExtractWitnessProgramInfo(script []byte) (int, []byte, error) {
	// If at this point, the scripts doesn't resemble a witness program,
	// then we'll exit early as there isn't a valid version or program to
	// extract.
	version, program, valid := extractWitnessProgramInfo(script)
	if !valid {
		return 0, nil, fmt.Errorf("script is not a witness program, " +
			"unable to extract version or witness program")
	}

	return version, program, nil
}

// IsPushOnlyScript returns whether or not the passed script only pushes data
// according to the consensus definition of pushing data.
//
// WARNING: This function always treats the passed script as version 0.  Great
// care must be taken if introducing a new script version because it is used in
// consensus which, unfortunately as of the time of this writing, does not check
// script versions before checking if it is a push only script which means nodes
// on existing rules will treat new version scripts as if they were version 0.
func IsPushOnlyScript(script []byte) bool {
	const scriptVersion = 0
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		// All opcodes up to OP_16 are data push instructions.
		// NOTE: This does consider OP_RESERVED to be a data push instruction,
		// but execution of OP_RESERVED will fail anyway and matches the
		// behavior required by consensus.
		if tokenizer.Opcode() > OP_16 {
			return false
		}
	}
	return tokenizer.Err() == nil
}

// DisasmString formats a disassembled script for one line printing.  When the
// script fails to parse, the returned string will contain the disassembled
// script up to the point the failure occurred along with the string '[error]'
// appended.  In addition, the reason the script failed to parse is returned
// if the caller wants more information about the failure.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func DisasmString(script []byte) (string, error) {
	const scriptVersion = 0

	var disbuf strings.Builder
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	if tokenizer.Next() {
		disasmOpcode(&disbuf, tokenizer.op, tokenizer.Data(), true)
	}
	for tokenizer.Next() {
		disbuf.WriteByte(' ')
		disasmOpcode(&disbuf, tokenizer.op, tokenizer.Data(), true)
	}
	if tokenizer.Err() != nil {
		if tokenizer.ByteIndex() != 0 {
			disbuf.WriteByte(' ')
		}
		disbuf.WriteString("[error]")
	}
	return disbuf.String(), tokenizer.Err()
}

// removeOpcodeRaw will return the script after removing any opcodes that match
// `opcode`. If the opcode does not appear in script, the original script will
// be returned unmodified. Otherwise, a new script will be allocated to contain
// the filtered script. This metehod assumes that the script parses
// successfully.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func removeOpcodeRaw(script []byte, opcode byte) []byte {
	// Avoid work when possible.
	if len(script) == 0 {
		return script
	}

	const scriptVersion = 0
	var result []byte
	var prevOffset int32

	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		if tokenizer.Opcode() == opcode {
			if result == nil {
				result = make([]byte, 0, len(script))
				result = append(result, script[:prevOffset]...)
			}
		} else if result != nil {
			result = append(result, script[prevOffset:tokenizer.ByteIndex()]...)
		}
		prevOffset = tokenizer.ByteIndex()
	}
	if result == nil {
		return script
	}
	return result
}

// isCanonicalPush returns true if the opcode is either not a push instruction
// or the data associated with the push instruction uses the smallest
// instruction to do the job.  False otherwise.
//
// For example, it is possible to push a value of 1 to the stack as "OP_1",
// "OP_DATA_1 0x01", "OP_PUSHDATA1 0x01 0x01", and others, however, the first
// only takes a single byte, while the rest take more.  Only the first is
// considered canonical.
func isCanonicalPush(opcode byte, data []byte) bool {
	dataLen := len(data)
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

// removeOpcodeByData will return the script minus any opcodes that perform a
// canonical push of data that contains the passed data to remove.  This
// function assumes it is provided a version 0 script as any future version of
// script should avoid this functionality since it is unncessary due to the
// signature scripts not being part of the witness-free transaction hash.
//
// WARNING: This will return the passed script unmodified unless a modification
// is necessary in which case the modified script is returned.  This implies
// callers may NOT rely on being able to safely mutate either the passed or
// returned script without potentially modifying the same data.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func removeOpcodeByData(script []byte, dataToRemove []byte) []byte {
	// Avoid work when possible.
	if len(script) == 0 || len(dataToRemove) == 0 {
		return script
	}

	// Parse through the script looking for a canonical data push that contains
	// the data to remove.
	const scriptVersion = 0
	var result []byte
	var prevOffset int32
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		// In practice, the script will basically never actually contain the
		// data since this function is only used during signature verification
		// to remove the signature itself which would require some incredibly
		// non-standard code to create.
		//
		// Thus, as an optimization, avoid allocating a new script unless there
		// is actually a match that needs to be removed.
		op, data := tokenizer.Opcode(), tokenizer.Data()
		if isCanonicalPush(op, data) && bytes.Contains(data, dataToRemove) {
			if result == nil {
				fullPushLen := tokenizer.ByteIndex() - prevOffset
				result = make([]byte, 0, int32(len(script))-fullPushLen)
				result = append(result, script[0:prevOffset]...)
			}
		} else if result != nil {
			result = append(result, script[prevOffset:tokenizer.ByteIndex()]...)
		}

		prevOffset = tokenizer.ByteIndex()
	}
	if result == nil {
		result = script
	}
	return result
}

// AsSmallInt returns the passed opcode, which must be true according to
// IsSmallInt(), as an integer.
func AsSmallInt(op byte) int {
	if op == OP_0 {
		return 0
	}

	return int(op - (OP_1 - 1))
}

// countSigOpsV0 returns the number of signature operations in the provided
// script up to the point of the first parse failure or the entire script when
// there are no parse failures.  The precise flag attempts to accurately count
// the number of operations for a multisig operation versus using the maximum
// allowed.
//
// WARNING: This function always treats the passed script as version 0.  Great
// care must be taken if introducing a new script version because it is used in
// consensus which, unfortunately as of the time of this writing, does not check
// script versions before counting their signature operations which means nodes
// on existing rules will count new version scripts as if they were version 0.
func countSigOpsV0(script []byte, precise bool) int {
	const scriptVersion = 0

	numSigOps := 0
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	prevOp := byte(OP_INVALIDOPCODE)
	for tokenizer.Next() {
		switch tokenizer.Opcode() {
		case OP_CHECKSIG, OP_CHECKSIGVERIFY:
			numSigOps++

		case OP_CHECKMULTISIG, OP_CHECKMULTISIGVERIFY:
			// Note that OP_0 is treated as the max number of sigops here in
			// precise mode despite it being a valid small integer in order to
			// highly discourage multisigs with zero pubkeys.
			//
			// Also, even though this is referred to as "precise" counting, it's
			// not really precise at all due to the small int opcodes only
			// covering 1 through 16 pubkeys, which means this will count any
			// more than that value (e.g. 17, 18 19) as the maximum number of
			// allowed pubkeys. This is, unfortunately, now part of
			// the Bitcoin consensus rules, due to historical
			// reasons. This could be made more correct with a new
			// script version, however, ideally all multisignaure
			// operations in new script versions should move to
			// aggregated schemes such as Schnorr instead.
			if precise && prevOp >= OP_1 && prevOp <= OP_16 {
				numSigOps += AsSmallInt(prevOp)
			} else {
				numSigOps += MaxPubKeysPerMultiSig
			}

		default:
			// Not a sigop.
		}

		prevOp = tokenizer.Opcode()
	}

	return numSigOps
}

// GetSigOpCount provides a quick count of the number of signature operations
// in a script. a CHECKSIG operations counts for 1, and a CHECK_MULTISIG for 20.
// If the script fails to parse, then the count up to the point of failure is
// returned.
//
// WARNING: This function always treats the passed script as version 0.  Great
// care must be taken if introducing a new script version because it is used in
// consensus which, unfortunately as of the time of this writing, does not check
// script versions before counting their signature operations which means nodes
// on existing rules will count new version scripts as if they were version 0.
func GetSigOpCount(script []byte) int {
	return countSigOpsV0(script, false)
}

// finalOpcodeData returns the data associated with the final opcode in the
// script.  It will return nil if the script fails to parse.
func finalOpcodeData(scriptVersion uint16, script []byte) []byte {
	// Avoid unnecessary work.
	if len(script) == 0 {
		return nil
	}

	var data []byte
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		data = tokenizer.Data()
	}
	if tokenizer.Err() != nil {
		return nil
	}
	return data
}

// GetPreciseSigOpCount returns the number of signature operations in
// scriptPubKey.  If bip16 is true then scriptSig may be searched for the
// Pay-To-Script-Hash script in order to find the precise number of signature
// operations in the transaction.  If the script fails to parse, then the count
// up to the point of failure is returned.
//
// WARNING: This function always treats the passed script as version 0.  Great
// care must be taken if introducing a new script version because it is used in
// consensus which, unfortunately as of the time of this writing, does not check
// script versions before counting their signature operations which means nodes
// on existing rules will count new version scripts as if they were version 0.
//
// The third parameter is DEPRECATED and is unused.
func GetPreciseSigOpCount(scriptSig, scriptPubKey []byte, _ bool) int {
	const scriptVersion = 0

	// Treat non P2SH transactions as normal.  Note that signature operation
	// counting includes all operations up to the first parse failure.
	if !isScriptHashScript(scriptPubKey) {
		return countSigOpsV0(scriptPubKey, true)
	}

	// The signature script must only push data to the stack for P2SH to be
	// a valid pair, so the signature operation count is 0 when that is not
	// the case.
	if len(scriptSig) == 0 || !IsPushOnlyScript(scriptSig) {
		return 0
	}

	// The P2SH script is the last item the signature script pushes to the
	// stack.  When the script is empty, there are no signature operations.
	//
	// Notice that signature scripts that fail to fully parse count as 0
	// signature operations unlike public key and redeem scripts.
	redeemScript := finalOpcodeData(scriptVersion, scriptSig)
	if len(redeemScript) == 0 {
		return 0
	}

	// Return the more precise sigops count for the redeem script.  Note that
	// signature operation counting includes all operations up to the first
	// parse failure.
	return countSigOpsV0(redeemScript, true)
}

// GetWitnessSigOpCount returns the number of signature operations generated by
// spending the passed pkScript with the specified witness, or sigScript.
// Unlike GetPreciseSigOpCount, this function is able to accurately count the
// number of signature operations generated by spending witness programs, and
// nested p2sh witness programs. If the script fails to parse, then the count
// up to the point of failure is returned.
func GetWitnessSigOpCount(sigScript, pkScript []byte, witness wire.TxWitness) int {
	// If this is a regular witness program, then we can proceed directly
	// to counting its signature operations without any further processing.
	if isWitnessProgramScript(pkScript) {
		return getWitnessSigOps(pkScript, witness)
	}

	// Next, we'll check the sigScript to see if this is a nested p2sh
	// witness program. This is a case wherein the sigScript is actually a
	// datapush of a p2wsh witness program.
	if isScriptHashScript(pkScript) && IsPushOnlyScript(sigScript) &&
		len(sigScript) > 0 && isWitnessProgramScript(sigScript[1:]) {
		return getWitnessSigOps(sigScript[1:], witness)
	}

	return 0
}

// getWitnessSigOps returns the number of signature operations generated by
// spending the passed witness program wit the passed witness. The exact
// signature counting heuristic is modified by the version of the passed
// witness program. If the version of the witness program is unable to be
// extracted, then 0 is returned for the sig op count.
func getWitnessSigOps(pkScript []byte, witness wire.TxWitness) int {
	// Attempt to extract the witness program version.
	witnessVersion, witnessProgram, err := ExtractWitnessProgramInfo(
		pkScript,
	)
	if err != nil {
		return 0
	}

	switch witnessVersion {
	case BaseSegwitWitnessVersion:
		switch {
		case len(witnessProgram) == payToWitnessPubKeyHashDataSize:
			return 1
		case len(witnessProgram) == payToWitnessScriptHashDataSize &&
			len(witness) > 0:

			witnessScript := witness[len(witness)-1]
			return countSigOpsV0(witnessScript, true)
		}

	// Taproot signature operations don't count towards the block-wide sig
	// op limit, instead a distinct weight-based accounting method is used.
	case TaprootWitnessVersion:
		return 0
	}

	return 0
}

// checkScriptParses returns an error if the provided script fails to parse.
func checkScriptParses(scriptVersion uint16, script []byte) error {
	tokenizer := MakeScriptTokenizer(scriptVersion, script)
	for tokenizer.Next() {
		// Nothing to do.
	}
	return tokenizer.Err()
}

// IsUnspendable returns whether the passed public key script is unspendable, or
// guaranteed to fail at execution.  This allows outputs to be pruned instantly
// when entering the UTXO set.
//
// NOTE: This function is only valid for version 0 scripts.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func IsUnspendable(pkScript []byte) bool {
	// The script is unspendable if starts with OP_RETURN or is guaranteed
	// to fail at execution due to being larger than the max allowed script
	// size.
	switch {
	case len(pkScript) > 0 && pkScript[0] == OP_RETURN:
		return true
	case len(pkScript) > MaxScriptSize:
		return true
	}

	// The script is unspendable if it is guaranteed to fail at execution.
	const scriptVersion = 0
	return checkScriptParses(scriptVersion, pkScript) != nil
}

// ScriptHasOpSuccess returns true if any op codes in the script contain an
// OP_SUCCESS op code.
func ScriptHasOpSuccess(witnessScript []byte) bool {
	// First, create a new script tokenizer so we can run through all the
	// elements.
	tokenizer := MakeScriptTokenizer(0, witnessScript)

	// Run through all the op codes, returning true if we find anything
	// that is marked as a new op success.
	for tokenizer.Next() {
		if _, ok := successOpcodes[tokenizer.Opcode()]; ok {
			return true
		}
	}

	return false
}
