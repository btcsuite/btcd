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
	"math"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
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
	ErrInvalidMagicBytes = errors.New(
		"Invalid Psbt due to incorrect magic bytes",
	)

	// ErrInvalidRawTxSigned indicates that the raw serialized transaction
	// in the global section of the passed Psbt serialization is invalid
	// because it contains scriptSigs/witnesses (i.e. is fully or partially
	// signed), which is not allowed by BIP174.
	ErrInvalidRawTxSigned = errors.New(
		"Invalid Psbt, raw transaction must be unsigned.",
	)

	// ErrInvalidPrevOutNonWitnessTransaction indicates that the transaction
	// hash (i.e. SHA256^2) of the fully serialized previous transaction
	// provided in the NonWitnessUtxo key-value field doesn't match the
	// prevout hash in the UnsignedTx field in the PSBT itself.
	ErrInvalidPrevOutNonWitnessTransaction = errors.New(
		"Prevout hash does not match the provided non-witness " +
			"utxo serialization",
	)

	// ErrInvalidSignatureForInput indicates that the signature the user is
	// trying to append to the PSBT is invalid, either because it does
	// not correspond to the previous transaction hash, or redeem script,
	// or witness script.
	// NOTE this does not include ECDSA signature checking.
	ErrInvalidSignatureForInput = errors.New(
		"Signature does not correspond to this input",
	)

	// ErrInputAlreadyFinalized indicates that the PSBT passed to a
	// Finalizer already contains the finalized scriptSig or witness.
	ErrInputAlreadyFinalized = errors.New(
		"Cannot finalize PSBT, finalized scriptSig or " +
			"scriptWitnes already exists",
	)

	// ErrIncompletePSBT indicates that the Extractor object
	// was unable to successfully extract the passed Psbt struct because
	// it is not complete
	ErrIncompletePSBT = errors.New(
		"PSBT cannot be extracted as it is incomplete",
	)

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

	// ErrLockTimeConflict indicates that the PSBT inputs contain
	// conflicting locktime requirements which cannot be satisfied
	// simultaneously in a single transaction.
	ErrLockTimeConflict = errors.New(
		"inputs have conflicting locktime requirements",
	)

	// ErrUnsupportedDynamicAdd indicates that a PSBTv2-specific operation
	//  was attempted on a PSBT that is not version 2.
	ErrUnsupportedDynamicAdd = errors.New(
		"cannot dynamically add inputs or outputs to a non-v2 PSBT",
	)

	// ErrInputsNotModifiable indicates that the inputs of the PSBT are
	// locked per the PSBT_GLOBAL_TX_MODIFIABLE field.
	ErrInputsNotModifiable = errors.New(
		"inputs are not modifiable in this PSBT",
	)

	// ErrOutputsNotModifiable indicates that the outputs of the PSBT are
	// locked per the PSBT_GLOBAL_TX_MODIFIABLE field.
	ErrOutputsNotModifiable = errors.New(
		"outputs are not modifiable in this PSBT",
	)

	// ErrV2TxVersionBelowTwo indicates that a PSBTv2 was constructed with a
	// transaction version less than 2, which violates BIP-370.
	ErrV2TxVersionBelowTwo = errors.New(
		"PSBTv2 requires a transaction version of at least 2",
	)
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

	// Version is the PSBT packet version (0 for BIP-174, 2 for BIP-370).
	Version uint32

	// FallbackLocktime is the transaction locktime to use if no
	// input-specific locktime constraints exist (PSBTv2 only).
	FallbackLocktime *uint32

	// InputCount is the number of inputs in this PSBT (PSBTv2 only).
	InputCount uint32

	// OutputCount is the number of outputs in this PSBT (PSBTv2 only).
	OutputCount uint32

	// TxVersion is the Bitcoin transaction version for the constructed
	// transaction (PSBTv2 only).
	TxVersion int32

	// TxModifiable is a bitfield indicating which parts of the transaction
	// can be modified by subsequent updaters (PSBTv2 only).
	TxModifiable *uint8
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

// DetermineLockTime implements the BIP-370 "Determining Lock Time" algorithm.
// Per BIP-370: "the field chosen is the one which is supported by all of the
// inputs which specify a locktime in either of those fields."
//
// Algorithm:
// 1. If no inputs have locktime constraints, use PSBT_GLOBAL_FALLBACK_LOCKTIME
// 2. Find the locktime type that ALL constrained inputs can satisfy:
//   - Inputs with only TimeLocktime can ONLY be satisfied by time-based locks
//   - Inputs with only HeightLocktime can ONLY be satisfied by height-based
//     locks
//   - Inputs with both (or neither) can be satisfied by either type
//
// 3. If both types are supported, BIP-370 mandates selecting height-based
// 4. Return the maximum value of the selected type
//
// Reference:
// https://github.com/bitcoin/bips/blob/master/bip-0370.mediawiki#determining-lock-time
func (p *Packet) DetermineLockTime() (uint32, error) {
	var (
		maxTime, maxHeight  uint32
		timeSupported       = true
		heightSupported     = true
		hasAnyInputLocktime = false
	)

	for _, pIn := range p.Inputs {
		hasTimeReq := pIn.TimeLocktime != 0
		hasHeightReq := pIn.HeightLocktime != 0

		if !hasTimeReq && !hasHeightReq {
			continue
		}
		hasAnyInputLocktime = true

		// Update maximums
		if pIn.TimeLocktime > maxTime {
			maxTime = pIn.TimeLocktime
		}
		if pIn.HeightLocktime > maxHeight {
			maxHeight = pIn.HeightLocktime
		}

		// An input with only PSBT_IN_REQUIRED_TIME_LOCKTIME (0x11)
		// cannot be satisfied by a height-based lock; mark height as
		// unsupported.
		if hasTimeReq && !hasHeightReq {
			heightSupported = false
		}

		// An input with only PSBT_IN_REQUIRED_HEIGHT_LOCKTIME (0x12)
		// cannot be satisfied by a time-based lock; mark time as
		// unsupported.
		if hasHeightReq && !hasTimeReq {
			timeSupported = false
		}
	}

	// 1. Fallback Case: No inputs specified constraints
	if !hasAnyInputLocktime {
		if p.FallbackLocktime != nil {
			return *p.FallbackLocktime, nil
		}
		return 0, nil
	}

	// 2. Conflict Case: One input requires Time, another requires Height
	if !timeSupported && !heightSupported {
		return 0, ErrLockTimeConflict
	}

	// 3. Selection Case: BIP-370 tie-breaker mandates height-based when
	// both supported "If a PSBT has both types of locktimes possible...
	// then locktime determined by looking at the
	// PSBT_IN_REQUIRED_HEIGHT_LOCKTIME fields of the inputs must be chosen"
	if heightSupported {
		return maxHeight, nil
	}

	// 4. Otherwise, use Time
	return maxTime, nil
}

// GetUnsignedTx returns a copy of the underlying unsigned transaction for this
// PSBT. For version 0 PSBTs, this is a copy of the parsed unsigned transaction.
// For version 2 PSBTs, it dynamically constructs the transaction from the
// individual parsing fields per BIP-0370.
func (p *Packet) GetUnsignedTx() (*wire.MsgTx, error) {
	switch p.Version {
	case PsbtVersion0:
		if p.UnsignedTx == nil {
			return nil, ErrInvalidPsbtFormat
		}
		return p.UnsignedTx.Copy(), nil

	case PsbtVersion2:
		// Proceed to dynamically construct the transaction below.

	default:
		return nil, ErrInvalidPsbtFormat
	}

	tx := wire.NewMsgTx(p.TxVersion)

	for _, pIn := range p.Inputs {
		if pIn.PreviousTxid == nil {
			return nil, ErrInvalidPsbtFormat
		}
		hash, err := chainhash.NewHash(pIn.PreviousTxid)
		if err != nil {
			return nil, err
		}

		outPoint := wire.NewOutPoint(hash, pIn.OutputIndex)
		txIn := &wire.TxIn{
			PreviousOutPoint: *outPoint,
			Sequence:         pIn.Sequence,
		}
		tx.AddTxIn(txIn)
	}

	for _, pOut := range p.Outputs {
		txOut := wire.NewTxOut(pOut.Amount, pOut.Script)
		tx.AddTxOut(txOut)
	}

	lockTime, err := p.DetermineLockTime()
	if err != nil {
		return nil, err
	}
	tx.LockTime = lockTime

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
	for i, txout := range tx.TxOut {
		outSlice[i].Amount = txout.Value
		outSlice[i].Script = txout.PkScript
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
	if b64 {
		decoded, err := decodeBase64Strict(r)
		if err != nil {
			return nil, err
		}
		r = bytes.NewReader(decoded)
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

	// Parse the GLOBAL section into an isolated result object.
	globalData, err := parseGlobalMap(r)
	if err != nil {
		return nil, err
	}

	inCount, outCount, err := globalData.validate()
	if err != nil {
		return nil, err
	}

	newPsbt, err := globalData.toPacket(r, inCount, outCount)
	if err != nil {
		return nil, err
	}

	// Extended sanity checking is applied here to make sure the
	// externally-passed Packet follows all the rules.
	if err := newPsbt.SanityCheck(); err != nil {
		return nil, err
	}

	if err := assertFullyConsumed(r); err != nil {
		return nil, err
	}

	return newPsbt, nil
}

type globalParseResult struct {
	msgTx                *wire.MsgTx
	xPubSlice            []XPub
	unknownSlice         []*Unknown
	version              uint32
	txVersion            int32
	fallbackLocktime     *uint32
	inputCount           uint32
	outputCount          uint32
	txModifiable         *uint8
	versionSeen          bool
	txVersionSeen        bool
	inputCountSeen       bool
	outputCountSeen      bool
	txModifiableSeen     bool
	fallbackLocktimeSeen bool
}

func (r *globalParseResult) addUnknown(keyCode int, keyData, value []byte) {
	keyintanddata := []byte{byte(keyCode)}
	keyintanddata = append(keyintanddata, keyData...)
	r.unknownSlice = append(r.unknownSlice, &Unknown{
		Key:   keyintanddata,
		Value: value,
	})
}

// validate checks the parsed global fields against version-specific constraints
// (BIP-174 for v0 and BIP-370 for v2) and returns the expected input and output
// counts.
func (g *globalParseResult) validate() (int, int, error) {
	switch g.version {
	case PsbtVersion0:
		if g.msgTx == nil {
			return 0, 0, ErrInvalidPsbtFormat
		}
		// PSBT Version 0 MUST NOT contain any Version 2 global fields.
		if g.txVersionSeen || g.fallbackLocktimeSeen ||
			g.inputCountSeen ||
			g.outputCountSeen ||
			g.txModifiableSeen {

			return 0, 0, ErrInvalidPsbtFormat
		}
		return len(g.msgTx.TxIn), len(g.msgTx.TxOut), nil

	case PsbtVersion2:
		if g.msgTx != nil {
			return 0, 0, ErrInvalidPsbtFormat
		}
		if !g.txVersionSeen || !g.inputCountSeen || !g.outputCountSeen {
			return 0, 0, ErrInvalidPsbtFormat
		}
		if g.txVersion < 2 {
			return 0, 0, ErrInvalidPsbtFormat
		}
		return int(g.inputCount), int(g.outputCount), nil

	default:
		return 0, 0, ErrInvalidPsbtFormat
	}
}

// toPacket parses the inputs and outputs from the given reader according to the
// provided counts and constructs the final Packet object.
func (g *globalParseResult) toPacket(r io.Reader,
	inCount,
	outCount int) (*Packet, error) {

	// Next we parse the INPUT section.
	inSlice := make([]PInput, inCount)
	for i := range inCount {
		input := PInput{}
		switch g.version {
		case PsbtVersion0:
			input.PreviousTxid =
				g.msgTx.TxIn[i].PreviousOutPoint.Hash[:]
			input.OutputIndex =
				g.msgTx.TxIn[i].PreviousOutPoint.Index
			input.Sequence = g.msgTx.TxIn[i].Sequence

		case PsbtVersion2:
			input.Sequence = wire.MaxTxInSequenceNum
		}

		err := input.deserialize(r, g.version)
		if err != nil {
			return nil, err
		}
		inSlice[i] = input
	}

	// Next we parse the OUTPUT section.
	outSlice := make([]POutput, outCount)
	for i := 0; i < outCount; i++ {
		output := POutput{}
		switch g.version {
		case PsbtVersion0:
			output.Amount = g.msgTx.TxOut[i].Value
			output.Script = g.msgTx.TxOut[i].PkScript
		}
		err := output.deserialize(r, g.version)
		if err != nil {
			return nil, err
		}
		outSlice[i] = output
	}

	// Populate the new Packet object.
	newPsbt := Packet{
		UnsignedTx:       g.msgTx,
		Inputs:           inSlice,
		Outputs:          outSlice,
		XPubs:            g.xPubSlice,
		Unknowns:         g.unknownSlice,
		Version:          g.version,
		FallbackLocktime: g.fallbackLocktime,
		InputCount:       g.inputCount,
		OutputCount:      g.outputCount,
		TxVersion:        g.txVersion,
		TxModifiable:     g.txModifiable,
	}

	return &newPsbt, nil
}

// parseGlobalMap reads and parses the global key-value pairs of a PSBT from
// the provided io.Reader. It ensures there are no duplicate keys, validates
// the global types according to BIP 174 and BIP 370, and returns the
// extracted data in a globalParseResult struct.
func parseGlobalMap(r io.Reader) (*globalParseResult, error) {
	var res globalParseResult

	// Next we parse any unknowns that may be present, making sure that we
	// break at the separator.
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

		// Only XPubGlobalType expects key data. If any other type has
		// key data, we must treat it as an unknown key per the BIP.
		if keyData != nil && GlobalType(keyCode) != XPubType {
			res.addUnknown(keyCode, keyData, value)
			continue
		}

		switch GlobalType(keyCode) {
		case UnsignedTxType:
			if res.msgTx != nil {
				return nil, ErrDuplicateKey
			}

			res.msgTx, err = readTransaction(value, true)
			if err != nil {
				return nil, err
			}
			if !validateUnsignedTX(res.msgTx) {
				return nil, ErrInvalidRawTxSigned
			}

		case XPubType:
			xPub, err := ReadXPub(keyData, value)
			if err != nil {
				return nil, err
			}
			for _, x := range res.xPubSlice {
				if bytes.Equal(x.ExtendedKey, keyData) {
					return nil, ErrDuplicateKey
				}
			}
			res.xPubSlice = append(res.xPubSlice, *xPub)

		case VersionType:
			if res.versionSeen {
				return nil, ErrDuplicateKey
			}

			if len(value) != 4 {
				return nil, ErrInvalidKeyData
			}
			res.version = binary.LittleEndian.Uint32(value)
			res.versionSeen = true

		case TxVersionGlobalType:
			if res.txVersionSeen {
				return nil, ErrDuplicateKey
			}

			if len(value) != 4 {
				return nil, ErrInvalidKeyData
			}
			res.txVersion = int32(binary.LittleEndian.Uint32(value))
			res.txVersionSeen = true

		case FallbackLocktimeGlobalType:
			if res.fallbackLocktimeSeen {
				return nil, ErrDuplicateKey
			}

			if len(value) != 4 {
				return nil, ErrInvalidKeyData
			}
			val := binary.LittleEndian.Uint32(value)
			res.fallbackLocktime = &val
			res.fallbackLocktimeSeen = true

		case InputCountGlobalType:
			if res.inputCountSeen {
				return nil, ErrDuplicateKey
			}

			if len(value) > wire.MaxVarIntPayload {
				return nil, ErrInvalidKeyData
			}
			num, _ := wire.ReadVarInt(bytes.NewReader(value), 0)
			if num > math.MaxUint32 {
				return nil, ErrInvalidKeyData
			}
			if num > 25_000 {
				return nil, ErrInvalidKeyData
			}
			res.inputCount = uint32(num)
			res.inputCountSeen = true

		case OutputCountGlobalType:
			if res.outputCountSeen {
				return nil, ErrDuplicateKey
			}

			if len(value) > wire.MaxVarIntPayload {
				return nil, ErrInvalidKeyData
			}
			num, _ := wire.ReadVarInt(bytes.NewReader(value), 0)
			if num > math.MaxUint32 {
				return nil, ErrInvalidKeyData
			}
			if num > 100_000 {
				return nil, ErrInvalidKeyData
			}
			res.outputCount = uint32(num)
			res.outputCountSeen = true

		case TxModifiableGlobalType:
			if res.txModifiableSeen {
				return nil, ErrDuplicateKey
			}

			if len(value) != 1 {
				return nil, ErrInvalidKeyData
			}
			val := value[0]
			res.txModifiable = &val
			res.txModifiableSeen = true

		default:
			res.addUnknown(keyCode, keyData, value)
		}
	}

	return &res, nil
}

// decodeBase64Strict decodes an RFC4648 base64 stream without permitting
// whitespace and with '=' allowed only as final padding.
func decodeBase64Strict(r io.Reader) ([]byte, error) {
	encoded, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	// Go's strict base64 decoder still ignores CR/LF. Reject them before
	// decoding so base64 PSBT parsing matches the RFC4648 alphabet exactly.
	if bytes.ContainsAny(encoded, "\r\n") {
		return nil, ErrInvalidPsbtFormat
	}

	decoded := make([]byte, base64.StdEncoding.DecodedLen(len(encoded)))
	n, err := base64.StdEncoding.Strict().Decode(decoded, encoded)
	if err != nil {
		return nil, ErrInvalidPsbtFormat
	}

	return decoded[:n], nil
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
	case PsbtVersion0:

		// Next we prep to write out the unsigned transaction by first
		// serializing it into an intermediate buffer.
		if p.UnsignedTx == nil {
			return ErrInvalidPsbtFormat
		}

		serializedTx := bytes.NewBuffer(
			make([]byte, 0, p.UnsignedTx.SerializeSize()),
		)
		if err := p.UnsignedTx.SerializeNoWitness(
			serializedTx,
		); err != nil {

			return err
		}

		// Now that we have the serialized transaction, we'll write
		//  it out to the proper global type.Key 0x00: UnsignedTxType
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

	case PsbtVersion2:
		if p.UnsignedTx != nil {
			return ErrInvalidPsbtFormat
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

		var buf [4]byte

		// Key 0x02: TxVersion
		binary.LittleEndian.PutUint32(buf[:], uint32(p.TxVersion))
		err := serializeKVPairWithType(
			w, uint8(TxVersionGlobalType), nil, buf[:],
		)
		if err != nil {
			return err
		}

		// Key 0x03: FallbackLocktime
		if p.FallbackLocktime != nil {
			binary.LittleEndian.PutUint32(
				buf[:], *p.FallbackLocktime,
			)
			err = serializeKVPairWithType(
				w, uint8(FallbackLocktimeGlobalType), nil,
				buf[:],
			)
			if err != nil {
				return err
			}
		}

		// Key 0x04: InputCount
		// Input and Output counts are compact size uints
		var countBuf bytes.Buffer
		err = wire.WriteVarInt(&countBuf, 0, uint64(p.InputCount))
		if err != nil {
			return err
		}

		err = serializeKVPairWithType(
			w, uint8(InputCountGlobalType), nil, countBuf.Bytes(),
		)
		if err != nil {
			return err
		}

		// Key 0x05: OutputCount
		countBuf.Reset()
		err = wire.WriteVarInt(&countBuf, 0, uint64(p.OutputCount))
		if err != nil {
			return err
		}

		err = serializeKVPairWithType(
			w, uint8(OutputCountGlobalType), nil, countBuf.Bytes(),
		)
		if err != nil {
			return err
		}

		// Key 0x06: TxModifiable
		if p.TxModifiable != nil {
			err = serializeKVPairWithType(
				w, uint8(TxModifiableGlobalType), nil,
				[]byte{*p.TxModifiable},
			)
			if err != nil {
				return err
			}
		}

		// Key 0xfb: PSBT Version
		binary.LittleEndian.PutUint32(buf[:], 2)
		if err := serializeKVPairWithType(
			w, uint8(VersionType), nil, buf[:],
		); err != nil {

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

	if err := p.SanityCheck(); err != nil {
		return false
	}
	for i := range p.Inputs {
		if !isFinalized(p, i) {
			return false
		}
	}
	return true
}

// SanityCheck checks conditions on a PSBT to ensure that it obeys the rules of
// BIP174 and BIP0370, and returns an error if not.
func (p *Packet) SanityCheck() error {
	switch p.Version {
	case PsbtVersion0:
		if p.UnsignedTx == nil {
			return ErrInvalidPsbtFormat
		}
		if !validateUnsignedTX(p.UnsignedTx) {
			return ErrInvalidRawTxSigned
		}
		if len(p.Inputs) != len(p.UnsignedTx.TxIn) {
			return ErrInvalidPsbtFormat
		}
		if len(p.Outputs) != len(p.UnsignedTx.TxOut) {
			return ErrInvalidPsbtFormat
		}

	case PsbtVersion2:
		if p.UnsignedTx != nil {
			return ErrInvalidPsbtFormat
		}
		if p.TxVersion < 2 {
			return ErrInvalidPsbtFormat
		}
		if uint32(len(p.Inputs)) != p.InputCount {
			return ErrInvalidPsbtFormat
		}
		if uint32(len(p.Outputs)) != p.OutputCount {
			return ErrInvalidPsbtFormat
		}

		for _, tin := range p.Inputs {
			if len(tin.PreviousTxid) != 32 {
				return ErrInvalidPsbtFormat
			}
		}

	default:
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

	switch p.Version {
	case PsbtVersion0:
		for _, txOut := range p.UnsignedTx.TxOut {
			sumOutputs += txOut.Value
		}

	default:
		for _, pout := range p.Outputs {
			sumOutputs += pout.Amount
		}
	}

	fee := sumInputs - sumOutputs
	return btcutil.Amount(fee), nil
}

// outIndex returns the output index for the given input index depending on the
// PSBT version. For PSBTv0, the output index is found in the unsigned tx. For
// PSBTv2, it is found in the input.
func (p *Packet) outIndex(inIndex int) (uint32, error) {
	switch p.Version {
	case PsbtVersion2:
		return p.Inputs[inIndex].OutputIndex, nil

	default:
		if p.UnsignedTx == nil || inIndex >= len(p.UnsignedTx.TxIn) {
			return 0, ErrInvalidPsbtFormat
		}
		return p.UnsignedTx.TxIn[inIndex].PreviousOutPoint.Index, nil
	}
}
