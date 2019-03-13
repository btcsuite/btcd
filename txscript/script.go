// Copyright (c) 2013-2017 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package txscript

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"strings"
	"time"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
)

// Bip16Activation is the timestamp where BIP0016 is valid to use in the
// blockchain.  To be used to determine if BIP0016 should be called for or not.
// This timestamp corresponds to Sun Apr 1 00:00:00 UTC 2012.
var Bip16Activation = time.Unix(1333238400, 0)

// SigHashType represents hash type bits at the end of a signature.
type SigHashType uint32

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

// isSmallInt returns whether or not the opcode is considered a small integer,
// which is an OP_0, or OP_1 through OP_16.
//
// NOTE: This function is only valid for version 0 opcodes.  Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func isSmallInt(op byte) bool {
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

// IsPayToWitnessScriptHash returns true if the is in the standard
// pay-to-witness-script-hash (P2WSH) format, false otherwise.
func IsPayToWitnessScriptHash(script []byte) bool {
	return isWitnessScriptHashScript(script)
}

// IsPayToWitnessPubKeyHash returns true if the is in the standard
// pay-to-witness-pubkey-hash (P2WKH) format, false otherwise.
func IsPayToWitnessPubKeyHash(script []byte) bool {
	return isWitnessPubKeyHashScript(script)
}

// isWitnessPubKeyHash returns true if the passed script is a
// pay-to-witness-pubkey-hash, and false otherwise.
func isWitnessPubKeyHash(pops []parsedOpcode) bool {
	return len(pops) == 2 &&
		pops[0].opcode.value == OP_0 &&
		pops[1].opcode.value == OP_DATA_20
}

// IsWitnessProgram returns true if the passed script is a valid witness
// program which is encoded according to the passed witness program version. A
// witness program must be a small integer (from 0-16), followed by 2-40 bytes
// of pushed data.
func IsWitnessProgram(script []byte) bool {
	// The length of the script must be between 4 and 42 bytes. The
	// smallest program is the witness version, followed by a data push of
	// 2 bytes.  The largest allowed witness program has a data push of
	// 40-bytes.
	if len(script) < 4 || len(script) > 42 {
		return false
	}

	pops, err := parseScript(script)
	if err != nil {
		return false
	}

	return isWitnessProgram(pops)
}

// isWitnessProgram returns true if the passed script is a witness program, and
// false otherwise. A witness program MUST adhere to the following constraints:
// there must be exactly two pops (program version and the program itself), the
// first opcode MUST be a small integer (0-16), the push data MUST be
// canonical, and finally the size of the push data must be between 2 and 40
// bytes.
func isWitnessProgram(pops []parsedOpcode) bool {
	return len(pops) == 2 &&
		isSmallInt(pops[0].opcode.value) &&
		canonicalPush(pops[1]) &&
		(len(pops[1].data) >= 2 && len(pops[1].data) <= 40)
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
	pops, err := parseScript(script)
	if err != nil {
		return 0, nil, err
	}

	// If at this point, the scripts doesn't resemble a witness program,
	// then we'll exit early as there isn't a valid version or program to
	// extract.
	if !isWitnessProgram(pops) {
		return 0, nil, fmt.Errorf("script is not a witness program, " +
			"unable to extract version or witness program")
	}

	witnessVersion := asSmallInt(pops[0].opcode.value)
	witnessProgram := pops[1].data

	return witnessVersion, witnessProgram, nil
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

// parseScriptTemplate is the same as parseScript but allows the passing of the
// template list for testing purposes.  When there are parse errors, it returns
// the list of parsed opcodes up to the point of failure along with the error.
func parseScriptTemplate(script []byte, opcodes *[256]opcode) ([]parsedOpcode, error) {
	retScript := make([]parsedOpcode, 0, len(script))
	var err error
	for i := 0; i < len(script); {
		instr := script[i]
		op := &opcodes[instr]
		pop := parsedOpcode{opcode: op}
		i, err = pop.checkParseableInScript(script, i)
		if err != nil {
			return retScript, err
		}

		retScript = append(retScript, pop)
	}

	return retScript, nil
}

// checkScriptTemplateParseable is the same as parseScriptTemplate but does not
// return the list of opcodes up until the point of failure so that this can be
// used in functions which do not necessarily have a need for the failed list of
// opcodes, such as IsUnspendable.
//
// This function returns a pointer to a byte. This byte is nil if the parsing
// has an error, or if the script length is zero. If the script length is not
// zero and parsing succeeds, then the first opcode parsed will be returned.
//
// Not returning the full opcode list up until failure also has the benefit of
// reducing GC pressure, as the list would get immediately thrown away.
func checkScriptTemplateParseable(script []byte, opcodes *[256]opcode) (*byte, error) {
	var err error

	// A script of length zero is an unspendable script but it is parseable.
	var firstOpcode byte
	var numParsedInstr uint = 0

	for i := 0; i < len(script); {
		instr := script[i]
		op := &opcodes[instr]
		pop := parsedOpcode{opcode: op}
		i, err = pop.checkParseableInScript(script, i)
		if err != nil {
			return nil, err
		}

		// if this is a op_return then it is unspendable so we set the first
		// parsed instruction in case it's an op_return
		if numParsedInstr == 0 {
			firstOpcode = pop.opcode.value
		}
		numParsedInstr++
	}

	return &firstOpcode, nil
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

// calcHashPrevOuts calculates a single hash of all the previous outputs
// (txid:index) referenced within the passed transaction. This calculated hash
// can be re-used when validating all inputs spending segwit outputs, with a
// signature hash type of SigHashAll. This allows validation to re-use previous
// hashing computation, reducing the complexity of validating SigHashAll inputs
// from  O(N^2) to O(N).
func calcHashPrevOuts(tx *wire.MsgTx) chainhash.Hash {
	var b bytes.Buffer
	for _, in := range tx.TxIn {
		// First write out the 32-byte transaction ID one of whose
		// outputs are being referenced by this input.
		b.Write(in.PreviousOutPoint.Hash[:])

		// Next, we'll encode the index of the referenced output as a
		// little endian integer.
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], in.PreviousOutPoint.Index)
		b.Write(buf[:])
	}

	return chainhash.DoubleHashH(b.Bytes())
}

// calcHashSequence computes an aggregated hash of each of the sequence numbers
// within the inputs of the passed transaction. This single hash can be re-used
// when validating all inputs spending segwit outputs, which include signatures
// using the SigHashAll sighash type. This allows validation to re-use previous
// hashing computation, reducing the complexity of validating SigHashAll inputs
// from O(N^2) to O(N).
func calcHashSequence(tx *wire.MsgTx) chainhash.Hash {
	var b bytes.Buffer
	for _, in := range tx.TxIn {
		var buf [4]byte
		binary.LittleEndian.PutUint32(buf[:], in.Sequence)
		b.Write(buf[:])
	}

	return chainhash.DoubleHashH(b.Bytes())
}

// calcHashOutputs computes a hash digest of all outputs created by the
// transaction encoded using the wire format. This single hash can be re-used
// when validating all inputs spending witness programs, which include
// signatures using the SigHashAll sighash type. This allows computation to be
// cached, reducing the total hashing complexity from O(N^2) to O(N).
func calcHashOutputs(tx *wire.MsgTx) chainhash.Hash {
	var b bytes.Buffer
	for _, out := range tx.TxOut {
		wire.WriteTxOut(&b, 0, 0, out)
	}

	return chainhash.DoubleHashH(b.Bytes())
}

// calcWitnessSignatureHash computes the sighash digest of a transaction's
// segwit input using the new, optimized digest calculation algorithm defined
// in BIP0143: https://github.com/bitcoin/bips/blob/master/bip-0143.mediawiki.
// This function makes use of pre-calculated sighash fragments stored within
// the passed HashCache to eliminate duplicate hashing computations when
// calculating the final digest, reducing the complexity from O(N^2) to O(N).
// Additionally, signatures now cover the input value of the referenced unspent
// output. This allows offline, or hardware wallets to compute the exact amount
// being spent, in addition to the final transaction fee. In the case the
// wallet if fed an invalid input amount, the real sighash will differ causing
// the produced signature to be invalid.
func calcWitnessSignatureHash(subScript []parsedOpcode, sigHashes *TxSigHashes,
	hashType SigHashType, tx *wire.MsgTx, idx int, amt int64) ([]byte, error) {

	// As a sanity check, ensure the passed input index for the transaction
	// is valid.
	if idx > len(tx.TxIn)-1 {
		return nil, fmt.Errorf("idx %d but %d txins", idx, len(tx.TxIn))
	}

	// We'll utilize this buffer throughout to incrementally calculate
	// the signature hash for this transaction.
	var sigHash bytes.Buffer

	// First write out, then encode the transaction's version number.
	var bVersion [4]byte
	binary.LittleEndian.PutUint32(bVersion[:], uint32(tx.Version))
	sigHash.Write(bVersion[:])

	// Next write out the possibly pre-calculated hashes for the sequence
	// numbers of all inputs, and the hashes of the previous outs for all
	// outputs.
	var zeroHash chainhash.Hash

	// If anyone can pay isn't active, then we can use the cached
	// hashPrevOuts, otherwise we just write zeroes for the prev outs.
	if hashType&SigHashAnyOneCanPay == 0 {
		sigHash.Write(sigHashes.HashPrevOuts[:])
	} else {
		sigHash.Write(zeroHash[:])
	}

	// If the sighash isn't anyone can pay, single, or none, the use the
	// cached hash sequences, otherwise write all zeroes for the
	// hashSequence.
	if hashType&SigHashAnyOneCanPay == 0 &&
		hashType&sigHashMask != SigHashSingle &&
		hashType&sigHashMask != SigHashNone {
		sigHash.Write(sigHashes.HashSequence[:])
	} else {
		sigHash.Write(zeroHash[:])
	}

	txIn := tx.TxIn[idx]

	// Next, write the outpoint being spent.
	sigHash.Write(txIn.PreviousOutPoint.Hash[:])
	var bIndex [4]byte
	binary.LittleEndian.PutUint32(bIndex[:], txIn.PreviousOutPoint.Index)
	sigHash.Write(bIndex[:])

	if isWitnessPubKeyHash(subScript) {
		// The script code for a p2wkh is a length prefix varint for
		// the next 25 bytes, followed by a re-creation of the original
		// p2pkh pk script.
		sigHash.Write([]byte{0x19})
		sigHash.Write([]byte{OP_DUP})
		sigHash.Write([]byte{OP_HASH160})
		sigHash.Write([]byte{OP_DATA_20})
		sigHash.Write(subScript[1].data)
		sigHash.Write([]byte{OP_EQUALVERIFY})
		sigHash.Write([]byte{OP_CHECKSIG})
	} else {
		// For p2wsh outputs, and future outputs, the script code is
		// the original script, with all code separators removed,
		// serialized with a var int length prefix.
		rawScript, _ := unparseScript(subScript)
		wire.WriteVarBytes(&sigHash, 0, rawScript)
	}

	// Next, add the input amount, and sequence number of the input being
	// signed.
	var bAmount [8]byte
	binary.LittleEndian.PutUint64(bAmount[:], uint64(amt))
	sigHash.Write(bAmount[:])
	var bSequence [4]byte
	binary.LittleEndian.PutUint32(bSequence[:], txIn.Sequence)
	sigHash.Write(bSequence[:])

	// If the current signature mode isn't single, or none, then we can
	// re-use the pre-generated hashoutputs sighash fragment. Otherwise,
	// we'll serialize and add only the target output index to the signature
	// pre-image.
	if hashType&SigHashSingle != SigHashSingle &&
		hashType&SigHashNone != SigHashNone {
		sigHash.Write(sigHashes.HashOutputs[:])
	} else if hashType&sigHashMask == SigHashSingle && idx < len(tx.TxOut) {
		var b bytes.Buffer
		wire.WriteTxOut(&b, 0, 0, tx.TxOut[idx])
		sigHash.Write(chainhash.DoubleHashB(b.Bytes()))
	} else {
		sigHash.Write(zeroHash[:])
	}

	// Finally, write out the transaction's locktime, and the sig hash
	// type.
	var bLockTime [4]byte
	binary.LittleEndian.PutUint32(bLockTime[:], tx.LockTime)
	sigHash.Write(bLockTime[:])
	var bHashType [4]byte
	binary.LittleEndian.PutUint32(bHashType[:], uint32(hashType))
	sigHash.Write(bHashType[:])

	return chainhash.DoubleHashB(sigHash.Bytes()), nil
}

// CalcWitnessSigHash computes the sighash digest for the specified input of
// the target transaction observing the desired sig hash type.
func CalcWitnessSigHash(script []byte, sigHashes *TxSigHashes, hType SigHashType,
	tx *wire.MsgTx, idx int, amt int64) ([]byte, error) {

	parsedScript, err := parseScript(script)
	if err != nil {
		return nil, fmt.Errorf("cannot parse output script: %v", err)
	}

	return calcWitnessSignatureHash(parsedScript, sigHashes, hType, tx, idx,
		amt)
}

// shallowCopyTx creates a shallow copy of the transaction for use when
// calculating the signature hash.  It is used over the Copy method on the
// transaction itself since that is a deep copy and therefore does more work and
// allocates much more space than needed.
func shallowCopyTx(tx *wire.MsgTx) wire.MsgTx {
	// As an additional memory optimization, use contiguous backing arrays
	// for the copied inputs and outputs and point the final slice of
	// pointers into the contiguous arrays.  This avoids a lot of small
	// allocations.
	txCopy := wire.MsgTx{
		Version:  tx.Version,
		TxIn:     make([]*wire.TxIn, len(tx.TxIn)),
		TxOut:    make([]*wire.TxOut, len(tx.TxOut)),
		LockTime: tx.LockTime,
	}
	txIns := make([]wire.TxIn, len(tx.TxIn))
	for i, oldTxIn := range tx.TxIn {
		txIns[i] = *oldTxIn
		txCopy.TxIn[i] = &txIns[i]
	}
	txOuts := make([]wire.TxOut, len(tx.TxOut))
	for i, oldTxOut := range tx.TxOut {
		txOuts[i] = *oldTxOut
		txCopy.TxOut[i] = &txOuts[i]
	}
	return txCopy
}

// CalcSignatureHash will, given a script and hash type for the current script
// engine instance, calculate the signature hash to be used for signing and
// verification.
//
// NOTE: This function is only valid for version 0 scripts. Since the function
// does not accept a script version, the results are undefined for other script
// versions.
func CalcSignatureHash(script []byte, hashType SigHashType, tx *wire.MsgTx, idx int) ([]byte, error) {
	const scriptVersion = 0
	if err := checkScriptParses(scriptVersion, script); err != nil {
		return nil, err
	}

	return calcSignatureHashRaw(script, hashType, tx, idx), nil
}

// calcSignatureHashRaw computes the signature hash for the specified input of
// the target transaction observing the desired signature hash type.
func calcSignatureHashRaw(sigScript []byte, hashType SigHashType, tx *wire.MsgTx, idx int) []byte {
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
		var hash chainhash.Hash
		hash[0] = 0x01
		return hash[:]
	}

	// Remove all instances of OP_CODESEPARATOR from the script.
	filteredScript := make([]byte, 0, len(sigScript))
	const scriptVersion = 0
	tokenizer := MakeScriptTokenizer(scriptVersion, sigScript)
	var prevOffset int32
	for tokenizer.Next() {
		if tokenizer.Opcode() != OP_CODESEPARATOR {
			filteredScript = append(filteredScript,
				sigScript[prevOffset:tokenizer.ByteIndex()]...)
		}
		prevOffset = tokenizer.ByteIndex()
	}

	// Make a shallow copy of the transaction, zeroing out the script for
	// all inputs that are not currently being processed.
	txCopy := shallowCopyTx(tx)
	for i := range txCopy.TxIn {
		if i == idx {
			txCopy.TxIn[idx].SignatureScript = filteredScript
		} else {
			txCopy.TxIn[i].SignatureScript = nil
		}
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
	}

	// The final hash is the double sha256 of both the serialized modified
	// transaction and the hash type (encoded as a 4-byte little-endian
	// value) appended.
	wbuf := bytes.NewBuffer(make([]byte, 0, txCopy.SerializeSizeStripped()+4))
	txCopy.SerializeNoWitness(wbuf)
	binary.Write(wbuf, binary.LittleEndian, hashType)
	return chainhash.DoubleHashB(wbuf.Bytes())
}

// calcSignatureHash computes the signature hash for the specified input of the
// target transaction observing the desired signature hash type.
//
// DEPRECATED: Use calcSignatureHashRaw instead
func calcSignatureHash(prevOutScript []parsedOpcode, hashType SigHashType,
	tx *wire.MsgTx, idx int) ([]byte, error) {

	sigScript, err := unparseScript(prevOutScript)
	if err != nil {
		return nil, err
	}

	return calcSignatureHashRaw(sigScript, hashType, tx, idx), nil
}

// asSmallInt returns the passed opcode, which must be true according to
// isSmallInt(), as an integer.
func asSmallInt(op byte) int {
	if op == OP_0 {
		return 0
	}

	return int(op - (OP_1 - 1))
}

// getSigOpCount is the implementation function for counting the number of
// signature operations in the script provided by pops. If precise mode is
// requested then we attempt to count the number of operations for a multisig
// op. Otherwise we use the maximum.
//
// DEPRECATED.  Use countSigOpsV0 instead.
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
			// patterns for multisig, for now all we recognize is
			// OP_1 - OP_16 to signify the number of pubkeys.
			// Otherwise, we use the max of 20.
			if precise && i > 0 &&
				pops[i-1].opcode.value >= OP_1 &&
				pops[i-1].opcode.value <= OP_16 {
				nSigs += asSmallInt(pops[i-1].opcode.value)
			} else {
				nSigs += MaxPubKeysPerMultiSig
			}
		default:
			// Not a sigop.
		}
	}

	return nSigs
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
			// the Bitcion consensus rules, due to historical
			// reasons. This could be made more correct with a new
			// script version, however, ideally all multisignaure
			// operations in new script versions should move to
			// aggregated schemes such as Schnorr instead.
			if precise && prevOp >= OP_1 && prevOp <= OP_16 {
				numSigOps += asSmallInt(prevOp)
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
	if IsWitnessProgram(pkScript) {
		return getWitnessSigOps(pkScript, witness)
	}

	// Next, we'll check the sigScript to see if this is a nested p2sh
	// witness program. This is a case wherein the sigScript is actually a
	// datapush of a p2wsh witness program.
	if IsPayToScriptHash(pkScript) && IsPushOnlyScript(sigScript) &&
		IsWitnessProgram(sigScript[1:]) {
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
	case 0:
		switch {
		case len(witnessProgram) == payToWitnessPubKeyHashDataSize:
			return 1
		case len(witnessProgram) == payToWitnessScriptHashDataSize &&
			len(witness) > 0:

			witnessScript := witness[len(witness)-1]
			return countSigOpsV0(witnessScript, true)
		}
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
// guaranteed to fail at execution.  This allows inputs to be pruned instantly
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
