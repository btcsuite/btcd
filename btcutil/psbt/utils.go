// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// WriteTxWitness is a utility function due to non-exported witness
// serialization (writeTxWitness encodes the bitcoin protocol encoding for a
// transaction input's witness into w).
func WriteTxWitness(w io.Writer, wit [][]byte) error {
	if err := wire.WriteVarInt(w, 0, uint64(len(wit))); err != nil {
		return err
	}

	for _, item := range wit {
		err := wire.WriteVarBytes(w, 0, item)
		if err != nil {
			return err
		}
	}
	return nil
}

// writePKHWitness writes a witness for a p2wkh spending input
func writePKHWitness(sig []byte, pub []byte) ([]byte, error) {
	var (
		buf          bytes.Buffer
		witnessItems = [][]byte{sig, pub}
	)

	if err := WriteTxWitness(&buf, witnessItems); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// checkIsMultisigScript is a utility function to check whether a given
// redeemscript fits the standard multisig template used in all P2SH based
// multisig, given a set of pubkeys for redemption.
func checkIsMultiSigScript(pubKeys [][]byte, sigs [][]byte,
	script []byte) bool {

	// First insist that the script type is multisig.
	if txscript.GetScriptClass(script) != txscript.MultiSigTy {
		return false
	}

	// Inspect the script to ensure that the number of sigs and pubkeys is
	// correct
	_, numSigs, err := txscript.CalcMultiSigStats(script)
	if err != nil {
		return false
	}

	// If the number of sigs provided, doesn't match the number of required
	// pubkeys, then we can't proceed as we're not yet final.
	if numSigs != len(pubKeys) || numSigs != len(sigs) {
		return false
	}

	return true
}

// extractKeyOrderFromScript is a utility function to extract an ordered list
// of signatures, given a serialized script (redeemscript or witness script), a
// list of pubkeys and the signatures corresponding to those pubkeys. This
// function is used to ensure that the signatures will be embedded in the final
// scriptSig or scriptWitness in the correct order.
func extractKeyOrderFromScript(script []byte, expectedPubkeys [][]byte,
	sigs [][]byte) ([][]byte, error) {

	// If this isn't a proper finalized multi-sig script, then we can't
	// proceed.
	if !checkIsMultiSigScript(expectedPubkeys, sigs, script) {
		return nil, ErrUnsupportedScriptType
	}

	// Arrange the pubkeys and sigs into a slice of format:
	//   * [[pub,sig], [pub,sig],..]
	type sigWithPub struct {
		pubKey []byte
		sig    []byte
	}
	var pubsSigs []sigWithPub
	for i, pub := range expectedPubkeys {
		pubsSigs = append(pubsSigs, sigWithPub{
			pubKey: pub,
			sig:    sigs[i],
		})
	}

	// Now that we have the set of (pubkey, sig) pairs, we'll construct a
	// position map that we can use to swap the order in the slice above to
	// match how things are laid out in the script.
	type positionEntry struct {
		index int
		value sigWithPub
	}
	var positionMap []positionEntry

	// For each pubkey in our pubsSigs slice, we'll now construct a proper
	// positionMap entry, based on _where_ in the script the pubkey first
	// appears.
	for _, p := range pubsSigs {
		pos := bytes.Index(script, p.pubKey)
		if pos < 0 {
			return nil, errors.New("script does not contain pubkeys")
		}

		positionMap = append(positionMap, positionEntry{
			index: pos,
			value: p,
		})
	}

	// Now that we have the position map full populated, we'll use the
	// index data to properly sort the entries in the map based on where
	// they appear in the script.
	sort.Slice(positionMap, func(i, j int) bool {
		return positionMap[i].index < positionMap[j].index
	})

	// Finally, we can simply iterate through the position map in order to
	// extract the proper signature ordering.
	sortedSigs := make([][]byte, 0, len(positionMap))
	for _, x := range positionMap {
		sortedSigs = append(sortedSigs, x.value.sig)
	}

	return sortedSigs, nil
}

// getMultisigScriptWitness creates a full psbt serialized Witness field for
// the transaction, given the public keys and signatures to be appended. This
// function will only accept witnessScripts of the type M of N multisig. This
// is used for both p2wsh and nested p2wsh multisig cases.
func getMultisigScriptWitness(witnessScript []byte, pubKeys [][]byte,
	sigs [][]byte) ([]byte, error) {

	// First using the script as a guide, we'll properly order the sigs
	// according to how their corresponding pubkeys appear in the
	// witnessScript.
	orderedSigs, err := extractKeyOrderFromScript(
		witnessScript, pubKeys, sigs,
	)
	if err != nil {
		return nil, err
	}

	// Now that we know the proper order, we'll append each of the
	// signatures into a new witness stack, then top it off with the
	// witness script at the end, prepending the nil as we need the extra
	// pop..
	witnessElements := make(wire.TxWitness, 0, len(sigs)+2)
	witnessElements = append(witnessElements, nil)
	for _, os := range orderedSigs {
		witnessElements = append(witnessElements, os)
	}
	witnessElements = append(witnessElements, witnessScript)

	// Now that we have the full witness stack, we'll serialize it in the
	// expected format, and return the final bytes.
	var buf bytes.Buffer
	if err = WriteTxWitness(&buf, witnessElements); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// checkSigHashFlags compares the sighash flag byte on a signature with the
// value expected according to any PsbtInSighashType field in this section of
// the PSBT, and returns true if they match, false otherwise.
// If no SighashType field exists, it is assumed to be SIGHASH_ALL.
//
// TODO(waxwing): sighash type not restricted to one byte in future?
func checkSigHashFlags(sig []byte, input *PInput) bool {
	expectedSighashType := txscript.SigHashAll
	if input.SighashType != 0 {
		expectedSighashType = input.SighashType
	}

	return expectedSighashType == txscript.SigHashType(sig[len(sig)-1])
}

// serializeKVpair writes out a kv pair using a varbyte prefix for each.
func serializeKVpair(w io.Writer, key []byte, value []byte) error {
	if err := wire.WriteVarBytes(w, 0, key); err != nil {
		return err
	}

	return wire.WriteVarBytes(w, 0, value)
}

// serializeKVPairWithType writes out to the passed writer a type coupled with
// a key.
func serializeKVPairWithType(w io.Writer, kt uint8, keydata []byte,
	value []byte) error {

	// If the key has no data, then we write a blank slice.
	if keydata == nil {
		keydata = []byte{}
	}

	// The final key to be written is: {type} || {keyData}
	serializedKey := append([]byte{kt}, keydata...)
	return serializeKVpair(w, serializedKey, value)
}

// getKey retrieves a single key - both the key type and the keydata (if
// present) from the stream and returns the key type as an integer, or -1 if
// the key was of zero length. This integer is is used to indicate the presence
// of a separator byte which indicates the end of a given key-value pair list,
// and the keydata as a byte slice or nil if none is present.
func getKey(r io.Reader) (int, []byte, error) {

	// For the key, we read the varint separately, instead of using the
	// available ReadVarBytes, because we have a specific treatment of 0x00
	// here:
	count, err := wire.ReadVarInt(r, 0)
	if err != nil {
		return -1, nil, ErrInvalidPsbtFormat
	}
	if count == 0 {
		// A separator indicates end of key-value pair list.
		return -1, nil, nil
	}

	// Check that we don't attempt to decode a dangerously large key.
	if count > MaxPsbtKeyLength {
		return -1, nil, ErrInvalidKeydata
	}

	// Next, we ready out the designated number of bytes, which may include
	// a type, key, and optional data.
	keyTypeAndData := make([]byte, count)
	if _, err := io.ReadFull(r, keyTypeAndData[:]); err != nil {
		return -1, nil, err
	}

	keyType := int(string(keyTypeAndData)[0])

	// Note that the second return value will usually be empty, since most
	// keys contain no more than the key type byte.
	if len(keyTypeAndData) == 1 {
		return keyType, nil, nil
	}

	// Otherwise, we return the key, along with any data that it may
	// contain.
	return keyType, keyTypeAndData[1:], nil

}

// readTxOut is a limited version of wire.ReadTxOut, because the latter is not
// exported.
func readTxOut(txout []byte) (*wire.TxOut, error) {
	if len(txout) < 10 {
		return nil, ErrInvalidPsbtFormat
	}

	valueSer := binary.LittleEndian.Uint64(txout[:8])
	scriptPubKey := txout[9:]

	return wire.NewTxOut(int64(valueSer), scriptPubKey), nil
}

// SumUtxoInputValues tries to extract the sum of all inputs specified in the
// UTXO fields of the PSBT. An error is returned if an input is specified that
// does not contain any UTXO information.
func SumUtxoInputValues(packet *Packet) (int64, error) {
	// We take the TX ins of the unsigned TX as the truth for how many
	// inputs there should be, as the fields in the extra data part of the
	// PSBT can be empty.
	if len(packet.UnsignedTx.TxIn) != len(packet.Inputs) {
		return 0, fmt.Errorf("TX input length doesn't match PSBT " +
			"input length")
	}

	inputSum := int64(0)
	for idx, in := range packet.Inputs {
		switch {
		case in.WitnessUtxo != nil:
			// Witness UTXOs only need to reference the TxOut.
			inputSum += in.WitnessUtxo.Value

		case in.NonWitnessUtxo != nil:
			// Non-witness UTXOs reference to the whole transaction
			// the UTXO resides in.
			utxOuts := in.NonWitnessUtxo.TxOut
			txIn := packet.UnsignedTx.TxIn[idx]

			// Check that utxOuts actually has enough space to
			// contain the previous outpoint's index.
			opIdx := txIn.PreviousOutPoint.Index
			if opIdx >= uint32(len(utxOuts)) {
				return 0, fmt.Errorf("input %d has malformed "+
					"TxOut field", idx)
			}

			inputSum += utxOuts[txIn.PreviousOutPoint.Index].Value

		default:
			return 0, fmt.Errorf("input %d has no UTXO information",
				idx)
		}
	}
	return inputSum, nil
}

// TxOutsEqual returns true if two transaction outputs are equal.
func TxOutsEqual(out1, out2 *wire.TxOut) bool {
	if out1 == nil || out2 == nil {
		return out1 == out2
	}
	return out1.Value == out2.Value &&
		bytes.Equal(out1.PkScript, out2.PkScript)
}

// VerifyOutputsEqual verifies that the two slices of transaction outputs are
// deep equal to each other. We do the length check and manual loop to provide
// better error messages to the user than just returning "not equal".
func VerifyOutputsEqual(outs1, outs2 []*wire.TxOut) error {
	if len(outs1) != len(outs2) {
		return fmt.Errorf("number of outputs are different")
	}
	for idx, out := range outs1 {
		// There is a byte slice in the output so we can't use the
		// equality operator.
		if !TxOutsEqual(out, outs2[idx]) {
			return fmt.Errorf("output %d is different", idx)
		}
	}
	return nil
}

// VerifyInputPrevOutpointsEqual verifies that the previous outpoints of the
// two slices of transaction inputs are deep equal to each other. We do the
// length check and manual loop to provide better error messages to the user
// than just returning "not equal".
func VerifyInputPrevOutpointsEqual(ins1, ins2 []*wire.TxIn) error {
	if len(ins1) != len(ins2) {
		return fmt.Errorf("number of inputs are different")
	}
	for idx, in := range ins1 {
		if in.PreviousOutPoint != ins2[idx].PreviousOutPoint {
			return fmt.Errorf("previous outpoint of input %d is "+
				"different", idx)
		}
	}
	return nil
}

// VerifyInputOutputLen makes sure a packet is non-nil, contains a non-nil wire
// transaction and that the wire input/output lengths match the partial input/
// output lengths. A caller also can specify if they expect any inputs and/or
// outputs to be contained in the packet.
func VerifyInputOutputLen(packet *Packet, needInputs, needOutputs bool) error {
	if packet == nil || packet.UnsignedTx == nil {
		return fmt.Errorf("PSBT packet cannot be nil")
	}

	if len(packet.UnsignedTx.TxIn) != len(packet.Inputs) {
		return fmt.Errorf("invalid PSBT, wire inputs don't match " +
			"partial inputs")
	}
	if len(packet.UnsignedTx.TxOut) != len(packet.Outputs) {
		return fmt.Errorf("invalid PSBT, wire outputs don't match " +
			"partial outputs")
	}

	if needInputs && len(packet.UnsignedTx.TxIn) == 0 {
		return fmt.Errorf("PSBT packet must contain at least one " +
			"input")
	}
	if needOutputs && len(packet.UnsignedTx.TxOut) == 0 {
		return fmt.Errorf("PSBT packet must contain at least one " +
			"output")
	}

	return nil
}

// NewFromSignedTx is a utility function to create a packet from an
// already-signed transaction. Returned are: an unsigned transaction
// serialization, a list of scriptSigs, one per input, and a list of witnesses,
// one per input.
func NewFromSignedTx(tx *wire.MsgTx) (*Packet, [][]byte,
	[]wire.TxWitness, error) {

	scriptSigs := make([][]byte, 0, len(tx.TxIn))
	witnesses := make([]wire.TxWitness, 0, len(tx.TxIn))
	tx2 := tx.Copy()

	// Blank out signature info in inputs
	for i, tin := range tx2.TxIn {
		tin.SignatureScript = nil
		scriptSigs = append(scriptSigs, tx.TxIn[i].SignatureScript)
		tin.Witness = nil
		witnesses = append(witnesses, tx.TxIn[i].Witness)
	}

	// Outputs always contain: (value, scriptPubkey) so don't need
	// amending.  Now tx2 is tx with all signing data stripped out
	unsignedPsbt, err := NewFromUnsignedTx(tx2)
	if err != nil {
		return nil, nil, nil, err
	}
	return unsignedPsbt, scriptSigs, witnesses, nil
}
