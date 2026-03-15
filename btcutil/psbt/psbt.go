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
	"encoding/binary"
	"errors"
	"io"

	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/wire"
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

	Version          uint32
	FallbackLocktime uint32
	InputCount       uint32
	OutputCount      uint32
	TxVersion        uint32
	TxModifiable     uint8
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

// GetUnsignedTx returns a copy of the underlying unsigned transaction for this
// PSBT. For version 0 PSBTs, this is a copy of the parsed unsigned transaction.
// For version 2 PSBTs, it dynamically constructs the transaction from the
// individual parsing fields per BIP-0370.
func (p *Packet) GetUnsignedTx() (*wire.MsgTx, error) {
	if p.Version == 0 {
		if p.UnsignedTx == nil {
			return nil, ErrInvalidPsbtFormat
		}
		return p.UnsignedTx.Copy(), nil
	}

	if p.Version != 2 {
		return nil, ErrInvalidPsbtFormat
	}

	tx := wire.NewMsgTx(int32(p.TxVersion))

	var timeLock, heightLock uint32
	hasTime, hasHeight := false, false

	for _, pIn := range p.Inputs {
		if pIn.PreviousTxid == nil {
			return nil, ErrInvalidPsbtFormat
		}
		hash, err := chainhash.NewHash(pIn.PreviousTxid)
		if err != nil {
			return nil, err
		}

		outPoint := wire.NewOutPoint(hash, pIn.OutputIndex)
		txIn := wire.NewTxIn(outPoint, nil, nil)
		txIn.Sequence = pIn.Sequence

		tx.AddTxIn(txIn)

		if pIn.TimeLocktime != 0 {
			if pIn.TimeLocktime > timeLock {
				timeLock = pIn.TimeLocktime
			}
			hasTime = true
		}
		if pIn.HeightLocktime != 0 {
			if pIn.HeightLocktime > heightLock {
				heightLock = pIn.HeightLocktime
			}
			hasHeight = true
		}
	}

	for _, pOut := range p.Outputs {
		txOut := wire.NewTxOut(int64(pOut.Amount), pOut.Script)
		tx.AddTxOut(txOut)
	}

	if hasTime && hasHeight {
		return nil, ErrInvalidPsbtFormat
	}

	if hasTime {
		tx.LockTime = timeLock
	} else if hasHeight {
		tx.LockTime = heightLock
	} else {
		tx.LockTime = p.FallbackLocktime
	}

	return tx, nil
}

// NewFromUnsignedTx creates a new Psbt struct, without any signatures (i.e.
// only the global section is non-empty) using the passed unsigned transaction.
func NewFromUnsignedTx(tx *wire.MsgTx) (*Packet, error) {
	if !validateUnsignedTX(tx) {
		return nil, ErrInvalidRawTxSigned
	}

	inSlice := make([]PInput, len(tx.TxIn))
	outSlice := make([]POutput, len(tx.TxOut))
	for i, txin := range tx.TxIn {
		inSlice[i].PreviousTxid = txin.PreviousOutPoint.Hash[:]
		inSlice[i].OutputIndex = txin.PreviousOutPoint.Index
		inSlice[i].Sequence = txin.Sequence
	}

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
// NOTE: To create a Packet from one's own data, rather than reading in a
// serialization from a counterparty, one should use a psbt.New.
func NewFromRawBytes(r io.Reader, b64 bool) (*Packet, error) {
	// If the PSBT is encoded in bas64, then we'll create a new wrapper
	// reader that'll allow us to incrementally decode the contents of the
	// io.Reader.
	if b64 {
		based64EncodedReader := r
		r = base64.NewDecoder(base64.StdEncoding, based64EncodedReader)
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

	// Next we parse the GLOBAL section.
	var (
		xPubSlice        []XPub
		unknownSlice     []*Unknown
		msgTx            *wire.MsgTx // V0 unsigned tx
		version          uint32
		fallbackLocktime uint32
		inputCount       uint32
		outputCount      uint32
		txVersion        uint32
		txModifiable     uint8
		txVersionSeen    bool
		inputCountSeen   bool
		outputCountSeen  bool
	)

	for {
		keyCode, keyData, err := getKey(r)
		if err != nil {
			return nil, ErrInvalidPsbtFormat
		}
		if keyCode == -1 {
			break
		}

		value, err := wire.ReadVarBytes(
			r, 0, MaxPsbtValueLength, "PSBT value",
		)
		if err != nil {
			return nil, err
		}

		isUnknown := false

		switch GlobalType(keyCode) {
		case UnsignedTxType:
			if keyData != nil {
				isUnknown = true
				break
			}
			if msgTx != nil {
				return nil, ErrDuplicateKey
			}
			msgTx = wire.NewMsgTx(2)
			err = msgTx.DeserializeNoWitness(bytes.NewReader(value))
			if err != nil {
				return nil, err
			}
			if !validateUnsignedTX(msgTx) {
				return nil, ErrInvalidRawTxSigned
			}

		case XPubType:
			xPub, err := ReadXPub(keyData, value)
			if err != nil {
				return nil, err
			}
			for _, x := range xPubSlice {
				if bytes.Equal(x.ExtendedKey, keyData) {
					return nil, ErrDuplicateKey
				}
			}
			xPubSlice = append(xPubSlice, *xPub)

		case VersionType:
			if !isSaneKey(keyData, &isUnknown) {
				break
			}
			if version != 0 {
				return nil, ErrDuplicateKey
			}
			if len(value) != 4 {
				return nil, ErrInvalidKeyData
			}
			version = binary.LittleEndian.Uint32(value)

		case TxVersion:
			if !isSaneKey(keyData, &isUnknown) {
				break
			}
			if txVersionSeen {
				return nil, ErrDuplicateKey
			}
			if len(value) != 4 {
				return nil, ErrInvalidKeyData
			}
			txVersion = binary.LittleEndian.Uint32(value)
			txVersionSeen = true

		case FallbackLocktime:
			if !isSaneKey(keyData, &isUnknown) {
				break
			}
			if fallbackLocktime != 0 {
				return nil, ErrDuplicateKey
			}
			if len(value) != 4 {
				return nil, ErrInvalidKeyData
			}
			fallbackLocktime = binary.LittleEndian.Uint32(value)

		case InputCount:
			if !isSaneKey(keyData, &isUnknown) {
				break
			}
			if inputCountSeen {
				return nil, ErrDuplicateKey
			}
			if len(value) > 8 {
				return nil, ErrInvalidKeyData
			}
			num, _ := wire.ReadVarInt(bytes.NewReader(value), 0)
			inputCount = uint32(num)
			inputCountSeen = true

		case OutputCount:
			if !isSaneKey(keyData, &isUnknown) {
				break
			}
			if outputCountSeen {
				return nil, ErrDuplicateKey
			}
			if len(value) > 8 {
				return nil, ErrInvalidKeyData
			}
			num, _ := wire.ReadVarInt(bytes.NewReader(value), 0)
			outputCount = uint32(num)
			outputCountSeen = true

		case TxModifiable:
			if !isSaneKey(keyData, &isUnknown) {
				break
			}
			if txModifiable != 0 {
				return nil, ErrDuplicateKey
			}
			if len(value) != 1 {
				return nil, ErrInvalidKeyData
			}
			txModifiable = value[0]

		default:
			isUnknown = true
		}

		if isUnknown {
			keyintanddata := []byte{byte(keyCode)}
			keyintanddata = append(keyintanddata, keyData...)
			newUnknown := &Unknown{
				Key:   keyintanddata,
				Value: value,
			}
			unknownSlice = append(unknownSlice, newUnknown)
		}
	}

	// Validate version-specific constraints.
	if version == 0 && msgTx == nil {
		return nil, ErrInvalidPsbtFormat
	}
	if version == 2 {
		if msgTx != nil {
			return nil, ErrInvalidPsbtFormat
		}
		if !txVersionSeen || !inputCountSeen || !outputCountSeen {
			return nil, ErrInvalidPsbtFormat
		}
	}
	if version != 0 && version != 2 {
		return nil, ErrInvalidPsbtFormat
	}

	var inCount, outCount int
	if version == 0 {
		inCount = len(msgTx.TxIn)
		outCount = len(msgTx.TxOut)
	} else {
		inCount = int(inputCount)
		outCount = int(outputCount)
	}

	// Next we parse the INPUT section.
	inSlice := make([]PInput, inCount)
	for i := 0; i < inCount; i++ {
		input := PInput{Sequence: wire.MaxTxInSequenceNum}

		err := input.deserialize(r)
		if err != nil {
			return nil, err
		}
		inSlice[i] = input
	}

	// Next we parse the OUTPUT section.
	outSlice := make([]POutput, outCount)
	for i := 0; i < outCount; i++ {
		output := POutput{}
		err := output.deserialize(r)
		if err != nil {
			return nil, err
		}
		outSlice[i] = output
	}

	if version == 2 && txVersion == 0 {
		txVersion = 2
	}

	// Populate the new Packet object.
	newPsbt := Packet{
		UnsignedTx:       msgTx,
		Inputs:           inSlice,
		Outputs:          outSlice,
		XPubs:            xPubSlice,
		Unknowns:         unknownSlice,
		Version:          version,
		FallbackLocktime: fallbackLocktime,
		InputCount:       inputCount,
		OutputCount:      outputCount,
		TxVersion:        txVersion,
		TxModifiable:     txModifiable,
	}

	// Extended sanity checking is applied here to make sure the
	// externally-passed Packet follows all the rules.
	err := newPsbt.SanityCheck()
	if err != nil {
		return nil, err
	}

	return &newPsbt, nil
}

// Serialize creates a binary serialization of the referenced Packet struct
// with lexicographical ordering (by key) of the subsections.
func (p *Packet) Serialize(w io.Writer) error {
	// First we write out the precise set of magic bytes that identify a
	// valid PSBT transaction.
	if _, err := w.Write(psbtMagic[:]); err != nil {
		return err
	}

	switch p.Version {
	case 0:
		// Next we prep to write out the unsigned transaction by first
		// serializing it into an intermediate buffer.
		if p.UnsignedTx == nil {
			return ErrInvalidPsbtFormat
		}
		serializedTx := bytes.NewBuffer(
			make([]byte, 0, p.UnsignedTx.SerializeSize()),
		)
		if err := p.UnsignedTx.SerializeNoWitness(serializedTx); err != nil {
			return err
		}
		// Now that we have the serialized transaction, we'll write it out to
		// the proper global type.
		// Key 0x00: UnsignedTxType
		err := serializeKVPairWithType(
			w, uint8(UnsignedTxType), nil, serializedTx.Bytes(),
		)
		if err != nil {
			return err
		}

		// Serialize the global xPubs.
		// Key 0x01: XPubType
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

	case 2:
		// Serialize the global xPubs.
		// Key 0x01: XPubType
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

		var buf [4]byte

		// Key 0x02: TxVersion
		binary.LittleEndian.PutUint32(buf[:], p.TxVersion)
		if err := serializeKVPairWithType(w, uint8(TxVersion), nil, buf[:]); err != nil {
			return err
		}

		// Key 0x03: FallbackLocktime
		binary.LittleEndian.PutUint32(buf[:], p.FallbackLocktime)
		if err := serializeKVPairWithType(w, uint8(FallbackLocktime), nil, buf[:]); err != nil {
			return err
		}

		// Key 0x04: InputCount
		// Input and Output counts are compact size uints
		var countBuf bytes.Buffer
		if err := wire.WriteVarInt(&countBuf, 0, uint64(p.InputCount)); err != nil {
			return err
		}
		if err := serializeKVPairWithType(w, uint8(InputCount), nil, countBuf.Bytes()); err != nil {
			return err
		}

		// Key 0x05: OutputCount
		countBuf.Reset()
		if err := wire.WriteVarInt(&countBuf, 0, uint64(p.OutputCount)); err != nil {
			return err
		}
		if err := serializeKVPairWithType(w, uint8(OutputCount), nil, countBuf.Bytes()); err != nil {
			return err
		}

		// Key 0x06: TxModifiable
		if err := serializeKVPairWithType(w, uint8(TxModifiable), nil, []byte{p.TxModifiable}); err != nil {
			return err
		}

		// Key 0xfb: PSBT Version
		binary.LittleEndian.PutUint32(buf[:], 2)
		if err := serializeKVPairWithType(w, uint8(VersionType), nil, buf[:]); err != nil {
			return err
		}

	default:
		return ErrInvalidPsbtFormat
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
		err := pInput.serialize(w, p.Version)
		if err != nil {
			return err
		}

		if _, err := w.Write(separator); err != nil {
			return err
		}
	}

	for _, pOutput := range p.Outputs {
		err := pOutput.serialize(w, p.Version)
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
	for i := 0; i < len(p.Inputs); i++ {
		if !isFinalized(p, i) {
			return false
		}
	}
	return true
}

// SanityCheck checks conditions on a PSBT to ensure that it obeys the rules of
// BIP174 and BIP0370, and returns an error if not.
func (p *Packet) SanityCheck() error {
	if p.Version == 0 && p.UnsignedTx != nil {
		if !validateUnsignedTX(p.UnsignedTx) {
			return ErrInvalidRawTxSigned
		}
	} else if p.Version == 2 && p.UnsignedTx != nil {
		return ErrInvalidPsbtFormat
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
	if p.Version == 0 {
		for _, txOut := range p.UnsignedTx.TxOut {
			sumOutputs += txOut.Value
		}
	} else {
		for _, pOut := range p.Outputs {
			sumOutputs += int64(pOut.Amount)
		}
	}

	fee := sumInputs - sumOutputs
	return btcutil.Amount(fee), nil
}

// isSaneKey is a helper function that checks if a key that is expected to have
// no extra key data actually has some. If it does, it marks the key as unknown
// so that it can be processed as an unknown field rather than causing a
// validation error.
func isSaneKey(keyData []byte, isUnknown *bool) bool {
	if keyData != nil {
		*isUnknown = true
		return false
	}
	return true
}
