// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// Package psbt is an implementation of Partially Signed Bitcoin
// Transactions (PSBT). The format is defined in BIP 174:
// https://github.com/bitcoin/bips/blob/master/bip-0174.mediawiki
package psbt

import (
	"bytes"
	"encoding/base64"
	"errors"
	"io"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// psbtMagicLength is the length of the magic bytes used to signal the start of
// a serialized PSBT packet.
const psbtMagicLength = 5

var (
	// psbtMagic is the separator.
	psbtMagic = [psbtMagicLength]byte{0x70,
		0x73, 0x62, 0x74, 0xff, // = "psbt" + 0xff sep
	}
)

// MaxPsbtValueLength is the size of the largest transaction serialization
// that could be passed in a NonWitnessUtxo field. This is definitely
// less than 4M.
const MaxPsbtValueLength = 4000000

// MaxPsbtKeyLength is the length of the largest key that we'll successfully
// deserialize from the wire. Anything more will return ErrInvalidKeyData.
const MaxPsbtKeyLength = 10000

// MaxPsbtKeyValue is the maximum value of a key type in a PSBT. This maximum
// isn't specified by the BIP but used by bitcoind in various places to limit
// the number of items processed. So we use it to validate the key type in order
// to have a consistent behavior.
const MaxPsbtKeyValue = 0x02000000

var (

	// ErrInvalidPsbtFormat is a generic error for any situation in which a
	// provided Psbt serialization does not conform to the rules of BIP174.
	ErrInvalidPsbtFormat = errors.New("Invalid PSBT serialization format")

	// ErrDuplicateKey indicates that a passed Psbt serialization is invalid
	// due to having the same key repeated in the same key-value pair.
	ErrDuplicateKey = errors.New("Invalid Psbt due to duplicate key")

	// ErrInvalidKeyData indicates that a key-value pair in the PSBT
	// serialization contains data in the key which is not valid.
	ErrInvalidKeyData = errors.New("Invalid key data")

	// ErrInvalidMagicBytes indicates that a passed Psbt serialization is
	// invalid due to having incorrect magic bytes.
	ErrInvalidMagicBytes = errors.New("Invalid Psbt due to incorrect " +
		"magic bytes")

	// ErrInvalidRawTxSigned indicates that the raw serialized transaction
	// in the global section of the passed Psbt serialization is invalid
	// because it contains scriptSigs/witnesses (i.e. is fully or partially
	// signed), which is not allowed by BIP174.
	ErrInvalidRawTxSigned = errors.New("Invalid Psbt, raw transaction " +
		"must be unsigned.")

	// ErrInvalidPrevOutNonWitnessTransaction indicates that the transaction
	// hash (i.e. SHA256^2) of the fully serialized previous transaction
	// provided in the NonWitnessUtxo key-value field doesn't match the
	// prevout hash in the UnsignedTx field in the PSBT itself.
	ErrInvalidPrevOutNonWitnessTransaction = errors.New("Prevout hash " +
		"does not match the provided non-witness utxo serialization")

	// ErrInvalidSignatureForInput indicates that the signature the user is
	// trying to append to the PSBT is invalid, either because it does
	// not correspond to the previous transaction hash, or redeem script,
	// or witness script.
	// NOTE this does not include ECDSA signature checking.
	ErrInvalidSignatureForInput = errors.New("Signature does not " +
		"correspond to this input")

	// ErrInputAlreadyFinalized indicates that the PSBT passed to a
	// Finalizer already contains the finalized scriptSig or witness.
	ErrInputAlreadyFinalized = errors.New("Cannot finalize PSBT, " +
		"finalized scriptSig or scriptWitnes already exists")

	// ErrIncompletePSBT indicates that the Extractor object
	// was unable to successfully extract the passed Psbt struct because
	// it is not complete
	ErrIncompletePSBT = errors.New("PSBT cannot be extracted as it is " +
		"incomplete")

	// ErrNotFinalizable indicates that the PSBT struct does not have
	// sufficient data (e.g. signatures) for finalization
	ErrNotFinalizable = errors.New("PSBT is not finalizable")

	// ErrInvalidSigHashFlags indicates that a signature added to the PSBT
	// uses Sighash flags that are not in accordance with the requirement
	// according to the entry in PsbtInSighashType, or otherwise not the
	// default value (SIGHASH_ALL)
	ErrInvalidSigHashFlags = errors.New("Invalid Sighash Flags")

	// ErrUnsupportedScriptType indicates that the redeem script or
	// script witness given is not supported by this codebase, or is
	// otherwise not valid.
	ErrUnsupportedScriptType = errors.New("Unsupported script type")
)

// Unknown is a struct encapsulating a key-value pair for which the key type is
// unknown by this package; these fields are allowed in both the 'Global' and
// the 'Input' section of a PSBT.
type Unknown struct {
	Key   []byte
	Value []byte
}

// Packet is the actual psbt representation. It is a set of 1 + N + M
// key-value pair lists, 1 global, defining the unsigned transaction structure
// with N inputs and M outputs.  These key-value pairs can contain scripts,
// signatures, key derivations and other transaction-defining data.
type Packet struct {
	// UnsignedTx is the decoded unsigned transaction for this PSBT.
	UnsignedTx *wire.MsgTx // Deserialization of unsigned tx

	// Inputs contains all the information needed to properly sign this
	// target input within the above transaction.
	Inputs []PInput

	// Outputs contains all information required to spend any outputs
	// produced by this PSBT.
	Outputs []POutput

	// XPubs is a list of extended public keys that can be used to derive
	// public keys used in the inputs and outputs of this transaction. It
	// should be the public key at the highest hardened derivation index so
	// that the unhardened child keys used in the transaction can be
	// derived.
	XPubs []XPub

	// Unknowns are the set of custom types (global only) within this PSBT.
	Unknowns []*Unknown
}

// validateUnsignedTx returns true if the transaction is unsigned.  Note that
// more basic sanity requirements, such as the presence of inputs and outputs,
// is implicitly checked in the call to MsgTx.Deserialize().
func validateUnsignedTX(tx *wire.MsgTx) bool {
	for _, tin := range tx.TxIn {
		if len(tin.SignatureScript) != 0 || len(tin.Witness) != 0 {
			return false
		}
	}

	return true
}

// NewFromUnsignedTx creates a new Psbt struct, without any signatures (i.e.
// only the global section is non-empty) using the passed unsigned transaction.
func NewFromUnsignedTx(tx *wire.MsgTx) (*Packet, error) {
	if !validateUnsignedTX(tx) {
		return nil, ErrInvalidRawTxSigned
	}

	inSlice := make([]PInput, len(tx.TxIn))
	outSlice := make([]POutput, len(tx.TxOut))
	xPubSlice := make([]XPub, 0)
	unknownSlice := make([]*Unknown, 0)

	return &Packet{
		UnsignedTx: tx,
		Inputs:     inSlice,
		Outputs:    outSlice,
		XPubs:      xPubSlice,
		Unknowns:   unknownSlice,
	}, nil
}

// NewFromRawBytes returns a new instance of a Packet struct created by reading
// from a byte slice. If the format is invalid, an error is returned. If the
// argument b64 is true, the passed byte slice is decoded from base64 encoding
// before processing.
//
// The parsing is strict: base64 input must not contain whitespace or any
// characters outside the RFC4648 standard alphabet, and any data after the
// packet results in ErrInvalidPsbtFormat. For raw input, trailing data is only
// detected when the reader can report its remaining length without blocking
// (such as bytes.Reader); a plain raw stream is not probed past the packet, so
// the reader is left positioned directly after it. Base64 input is decoded
// incrementally and read through its canonical end.
//
// NOTE: To create a Packet from one's own data, rather than reading in a
// serialization from a counterparty, one should use a psbt.New.
func NewFromRawBytes(r io.Reader, b64 bool) (*Packet, error) {
	if b64 {
		r = newStrictBase64Decoder(r)
	}

	// The Packet struct does not store the fixed magic bytes, but they
	// must be present or the serialization must be explicitly rejected.
	var magic [5]byte
	if _, err := io.ReadFull(r, magic[:]); err != nil {
		return nil, err
	}
	if magic != psbtMagic {
		return nil, ErrInvalidMagicBytes
	}

	// Next we parse the GLOBAL section.  There is currently only 1 known
	// key type, UnsignedTx.  We insist this exists first; unknowns are
	// allowed, but only after.
	keyCode, keyData, err := getKey(r)
	if err != nil {
		return nil, err
	}
	if GlobalType(keyCode) != UnsignedTxType || keyData != nil {
		return nil, ErrInvalidPsbtFormat
	}

	// Now that we've verified the global type is present, we'll decode it
	// into a proper unsigned transaction, and validate it.
	value, err := wire.ReadVarBytes(
		r, 0, MaxPsbtValueLength, "PSBT value",
	)
	if err != nil {
		return nil, err
	}
	// BIP-0174 states: "The transaction must be in the old serialization
	// format (without witnesses)."
	msgTx, err := readTransaction(value, true)
	if err != nil {
		return nil, err
	}
	if !validateUnsignedTX(msgTx) {
		return nil, ErrInvalidRawTxSigned
	}

	// Next we parse any unknowns that may be present, making sure that we
	// break at the separator.
	var (
		xPubSlice    []XPub
		unknownSlice []*Unknown
	)
	for {
		keyint, keydata, err := getKey(r)
		if err != nil {
			return nil, ErrInvalidPsbtFormat
		}
		if keyint == -1 {
			break
		}

		value, err := wire.ReadVarBytes(
			r, 0, MaxPsbtValueLength, "PSBT value",
		)
		if err != nil {
			return nil, err
		}

		switch GlobalType(keyint) {
		case XPubType:
			xPub, err := ReadXPub(keydata, value)
			if err != nil {
				return nil, err
			}

			// Duplicate keys are not allowed
			for _, x := range xPubSlice {
				if bytes.Equal(x.ExtendedKey, keyData) {
					return nil, ErrDuplicateKey
				}
			}

			xPubSlice = append(xPubSlice, *xPub)

		default:
			keyintanddata := []byte{byte(keyint)}
			keyintanddata = append(keyintanddata, keydata...)

			newUnknown := &Unknown{
				Key:   keyintanddata,
				Value: value,
			}
			unknownSlice = append(unknownSlice, newUnknown)
		}
	}

	// Next we parse the INPUT section.
	inSlice := make([]PInput, len(msgTx.TxIn))
	for i := range msgTx.TxIn {
		input := PInput{}
		err = input.deserialize(r)
		if err != nil {
			return nil, err
		}

		inSlice[i] = input
	}

	// Next we parse the OUTPUT section.
	outSlice := make([]POutput, len(msgTx.TxOut))
	for i := range msgTx.TxOut {
		output := POutput{}
		err = output.deserialize(r)
		if err != nil {
			return nil, err
		}

		outSlice[i] = output
	}

	// Populate the new Packet object.
	newPsbt := Packet{
		UnsignedTx: msgTx,
		Inputs:     inSlice,
		Outputs:    outSlice,
		XPubs:      xPubSlice,
		Unknowns:   unknownSlice,
	}

	// Extended sanity checking is applied here to make sure the
	// externally-passed Packet follows all the rules.
	if err = newPsbt.SanityCheck(); err != nil {
		return nil, err
	}

	if b64 {
		if err := assertBase64FullyConsumed(r); err != nil {
			return nil, err
		}
	} else {
		// Reject any trailing data after the packet when a raw reader can
		// report it without an additional read. Plain raw streams are not
		// probed, as a read for EOF could block forever on an open
		// connection that has already delivered a complete packet.
		if lr, ok := r.(interface{ Len() int }); ok && lr.Len() > 0 {
			return nil, ErrInvalidPsbtFormat
		}
	}

	return &newPsbt, nil
}

// canonicalBase64Reader rejects the CR and LF bytes that encoding/base64
// otherwise ignores while decoding.
type canonicalBase64Reader struct {
	io.Reader
}

// Read returns encoded bytes only when they use the canonical RFC4648
// alphabet.
func (r *canonicalBase64Reader) Read(p []byte) (int, error) {
	n, err := r.Reader.Read(p)
	if bytes.ContainsAny(p[:n], "\r\n") {
		return 0, ErrInvalidPsbtFormat
	}

	return n, err
}

// strictBase64Decoder maps base64 syntax and truncation errors to
// ErrInvalidPsbtFormat while preserving other errors returned by the
// caller-supplied reader.
type strictBase64Decoder struct {
	io.Reader
}

// Read returns incrementally decoded bytes.
func (d *strictBase64Decoder) Read(p []byte) (int, error) {
	n, err := d.Reader.Read(p)
	if err == nil || errors.Is(err, io.EOF) {
		return n, err
	}

	var corruptInput base64.CorruptInputError
	if errors.As(err, &corruptInput) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, ErrInvalidPsbtFormat) {

		return n, ErrInvalidPsbtFormat
	}

	return n, err
}

// newStrictBase64Decoder returns a streaming RFC4648 base64 decoder that
// rejects whitespace and only accepts '=' as final padding.
func newStrictBase64Decoder(r io.Reader) io.Reader {
	canonicalReader := &canonicalBase64Reader{Reader: r}
	decoder := base64.NewDecoder(
		base64.StdEncoding.Strict(), canonicalReader,
	)

	return &strictBase64Decoder{Reader: decoder}
}

// assertBase64FullyConsumed verifies the end of the base64 envelope and
// rejects decoded data after the PSBT packet.
func assertBase64FullyConsumed(r io.Reader) error {
	var trailing [1]byte
	_, err := io.ReadFull(r, trailing[:])
	switch {
	case err == nil:
		return ErrInvalidPsbtFormat

	case errors.Is(err, io.EOF):
		return nil

	default:
		return err
	}
}

// Serialize creates a binary serialization of the referenced Packet struct
// with lexicographical ordering (by key) of the subsections.
func (p *Packet) Serialize(w io.Writer) error {
	// First we write out the precise set of magic bytes that identify a
	// valid PSBT transaction.
	if _, err := w.Write(psbtMagic[:]); err != nil {
		return err
	}

	// Next we prep to write out the unsigned transaction by first
	// serializing it into an intermediate buffer.
	serializedTx := bytes.NewBuffer(
		make([]byte, 0, p.UnsignedTx.SerializeSize()),
	)
	if err := p.UnsignedTx.SerializeNoWitness(serializedTx); err != nil {
		return err
	}

	// Now that we have the serialized transaction, we'll write it out to
	// the proper global type.
	err := serializeKVPairWithType(
		w, uint8(UnsignedTxType), nil, serializedTx.Bytes(),
	)
	if err != nil {
		return err
	}

	// Serialize the global xPubs.
	for _, xPub := range p.XPubs {
		pathBytes := SerializeBIP32Derivation(
			xPub.MasterKeyFingerprint, xPub.Bip32Path,
		)
		err := serializeKVPairWithType(
			w, uint8(XPubType), xPub.ExtendedKey, pathBytes,
		)
		if err != nil {
			return err
		}
	}

	// Unknown is a special case; we don't have a key type, only a key and
	// a value field
	for _, kv := range p.Unknowns {
		err := serializeKVpair(w, kv.Key, kv.Value)
		if err != nil {
			return err
		}
	}

	// With that our global section is done, so we'll write out the
	// separator.
	separator := []byte{0x00}
	if _, err := w.Write(separator); err != nil {
		return err
	}

	for _, pInput := range p.Inputs {
		err := pInput.serialize(w)
		if err != nil {
			return err
		}

		if _, err := w.Write(separator); err != nil {
			return err
		}
	}

	for _, pOutput := range p.Outputs {
		err := pOutput.serialize(w)
		if err != nil {
			return err
		}

		if _, err := w.Write(separator); err != nil {
			return err
		}
	}

	return nil
}

// B64Encode returns the base64 encoding of the serialization of
// the current PSBT, or an error if the encoding fails.
func (p *Packet) B64Encode() (string, error) {
	var b bytes.Buffer
	if err := p.Serialize(&b); err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(b.Bytes()), nil
}

// IsComplete returns true only if all of the inputs are
// finalized; this is particularly important in that it decides
// whether the final extraction to a network serialized signed
// transaction will be possible.
func (p *Packet) IsComplete() bool {
	for i := 0; i < len(p.UnsignedTx.TxIn); i++ {
		if !isFinalized(p, i) {
			return false
		}
	}
	return true
}

// SanityCheck checks conditions on a PSBT to ensure that it obeys the
// rules of BIP174, and returns true if so, false if not.
func (p *Packet) SanityCheck() error {
	if !validateUnsignedTX(p.UnsignedTx) {
		return ErrInvalidRawTxSigned
	}

	for _, tin := range p.Inputs {
		if !tin.IsSane() {
			return ErrInvalidPsbtFormat
		}
	}

	return nil
}

// GetTxFee returns the transaction fee.  An error is returned if a transaction
// input does not contain any UTXO information.
func (p *Packet) GetTxFee() (btcutil.Amount, error) {
	sumInputs, err := SumUtxoInputValues(p)
	if err != nil {
		return 0, err
	}

	var sumOutputs int64
	for _, txOut := range p.UnsignedTx.TxOut {
		sumOutputs += txOut.Value
	}

	fee := sumInputs - sumOutputs
	return btcutil.Amount(fee), nil
}
