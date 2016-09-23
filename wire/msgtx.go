// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strconv"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

const (
	// TxVersion is the current latest supported transaction version.
	TxVersion uint16 = 1

	// MaxTxInSequenceNum is the maximum sequence number the sequence field
	// of a transaction input can be.
	MaxTxInSequenceNum uint32 = 0xffffffff

	// MaxPrevOutIndex is the maximum index the index field of a previous
	// outpoint can be.
	MaxPrevOutIndex uint32 = 0xffffffff

	// NoExpiryValue is the value of expiry that indicates the transaction
	// has no expiry.
	NoExpiryValue uint32 = 0

	// NullValueIn is a null value for an input witness.
	NullValueIn int64 = -1

	// NullBlockHeight is the null value for an input witness. It references
	// the genesis block.
	NullBlockHeight uint32 = 0x00000000

	// NullBlockIndex is the null transaction index in a block for an input
	// witness.
	NullBlockIndex uint32 = 0xffffffff

	// DefaultPkScriptVersion is the default pkScript version, referring to
	// extended Decred script.
	DefaultPkScriptVersion uint16 = 0x0000
)

// defaultTxInOutAlloc is the default size used for the backing array for
// transaction inputs and outputs.  The array will dynamically grow as needed,
// but this figure is intended to provide enough space for the number of
// inputs and outputs in a typical transaction without needing to grow the
// backing array multiple times.
const defaultTxInOutAlloc = 15

const (
	// minTxInPayload is the minimum payload size for a transaction input.
	// PreviousOutPoint.Hash + PreviousOutPoint.Index 4 bytes +
	// PreviousOutPoint.Tree 1 byte + Varint for SignatureScript length 1
	// byte + Sequence 4 bytes.
	minTxInPayload = 11 + chainhash.HashSize

	// maxTxInPerMessage is the maximum number of transactions inputs that
	// a transaction which fits into a message could possibly have.
	maxTxInPerMessage = (MaxMessagePayload / minTxInPayload) + 1

	// minTxOutPayload is the minimum payload size for a transaction output.
	// Value 8 bytes + Varint for PkScript length 1 byte.
	minTxOutPayload = 9

	// maxTxOutPerMessage is the maximum number of transactions outputs that
	// a transaction which fits into a message could possibly have.
	maxTxOutPerMessage = (MaxMessagePayload / minTxOutPayload) + 1

	// minTxPayload is the minimum payload size for any full encoded
	// (prefix and witness transaction). Note that any realistically
	// usable transaction must have at least one input or output, but
	// that is a rule enforced at a higher layer, so it is intentionally
	// not included here.
	// Version 4 bytes + Varint number of transaction inputs 1 byte + Varint
	// number of transaction outputs 1 byte + Varint representing the number
	// of transaction signatures + LockTime 4 bytes + Expiry 4 bytes + min
	// input payload + min output payload.
	minTxPayload = 4 + 1 + 1 + 1 + 4 + 4
)

// TxSerializeType is a uint16 representing the serialized type of transaction
// this msgTx is. You can use a bitmask for this too, but Decred just splits
// the int32 version into 2x uint16s so that you have:
//  {
//    uint16 type
//    uint16 version
//  }
type TxSerializeType uint16

// The differente possible values for TxSerializeType.
const (
	TxSerializeFull = TxSerializeType(iota)
	TxSerializeNoWitness
	TxSerializeOnlyWitness
	TxSerializeWitnessSigning
	TxSerializeWitnessValueSigning
)

// TODO replace all these with predeclared int32 or [4]byte cj

// DefaultMsgTxVersion returns the default version int32 (serialize the tx
// fully, version number 1).
func DefaultMsgTxVersion() int32 {
	verBytes := make([]byte, 4, 4)
	binary.LittleEndian.PutUint16(verBytes[0:2], TxVersion)
	binary.LittleEndian.PutUint16(verBytes[2:4], uint16(TxSerializeFull))
	ver := binary.LittleEndian.Uint32(verBytes)
	return int32(ver)
}

// NoWitnessMsgTxVersion returns the witness free serializing int32 (serialize
// the tx without witness, version number 1).
func NoWitnessMsgTxVersion() int32 {
	verBytes := make([]byte, 4, 4)
	binary.LittleEndian.PutUint16(verBytes[0:2], TxVersion)
	binary.LittleEndian.PutUint16(verBytes[2:4], uint16(TxSerializeNoWitness))
	ver := binary.LittleEndian.Uint32(verBytes)
	return int32(ver)
}

// WitnessOnlyMsgTxVersion returns the witness only version int32 (serialize
// the tx witness, version number 1).
func WitnessOnlyMsgTxVersion() int32 {
	verBytes := make([]byte, 4, 4)
	binary.LittleEndian.PutUint16(verBytes[0:2], TxVersion)
	binary.LittleEndian.PutUint16(verBytes[2:4], uint16(TxSerializeOnlyWitness))
	ver := binary.LittleEndian.Uint32(verBytes)
	return int32(ver)
}

// WitnessSigningMsgTxVersion returns the witness only version int32 (serialize
// the tx witness for signing, version number 1).
func WitnessSigningMsgTxVersion() int32 {
	verBytes := make([]byte, 4, 4)
	binary.LittleEndian.PutUint16(verBytes[0:2], TxVersion)
	binary.LittleEndian.PutUint16(verBytes[2:4], uint16(TxSerializeWitnessSigning))
	ver := binary.LittleEndian.Uint32(verBytes)
	return int32(ver)
}

// WitnessValueSigningMsgTxVersion returns the witness only version int32
// (serialize the tx witness for signing with value, version number 1).
func WitnessValueSigningMsgTxVersion() int32 {
	verBytes := make([]byte, 4, 4)
	binary.LittleEndian.PutUint16(verBytes[0:2], TxVersion)
	binary.LittleEndian.PutUint16(verBytes[2:4],
		uint16(TxSerializeWitnessValueSigning))
	ver := binary.LittleEndian.Uint32(verBytes)
	return int32(ver)
}

// OutPoint defines a decred data type that is used to track previous
// transaction outputs.
type OutPoint struct {
	Hash  chainhash.Hash
	Index uint32
	Tree  int8
}

// NewOutPoint returns a new decred transaction outpoint point with the
// provided hash and index.
func NewOutPoint(hash *chainhash.Hash, index uint32, tree int8) *OutPoint {
	return &OutPoint{
		Hash:  *hash,
		Index: index,
		Tree:  tree,
	}
}

// String returns the OutPoint in the human-readable form "hash:index".
func (o OutPoint) String() string {
	// Allocate enough for hash string, colon, and 10 digits.  Although
	// at the time of writing, the number of digits can be no greater than
	// the length of the decimal representation of maxTxOutPerMessage, the
	// maximum message payload may increase in the future and this
	// optimization may go unnoticed, so allocate space for 10 decimal
	// digits, which will fit any uint32.
	buf := make([]byte, 2*chainhash.HashSize+1, 2*chainhash.HashSize+1+10)
	copy(buf, o.Hash.String())
	buf[2*chainhash.HashSize] = ':'
	buf = strconv.AppendUint(buf, uint64(o.Index), 10)
	return string(buf)
}

// TxIn defines a decred transaction input.
type TxIn struct {
	// Non-witness
	PreviousOutPoint OutPoint
	Sequence         uint32

	// Witness
	ValueIn         int64
	BlockHeight     uint32
	BlockIndex      uint32
	SignatureScript []byte
}

// SerializeSizePrefix returns the number of bytes it would take to serialize
// the transaction input for a prefix.
func (t *TxIn) SerializeSizePrefix() int {
	// Outpoint Hash 32 bytes + Outpoint Index 4 bytes + Outpoint Tree 1 byte +
	// Sequence 4 bytes.
	return 41
}

// SerializeSizeWitness returns the number of bytes it would take to serialize the
// transaction input for a witness.
func (t *TxIn) SerializeSizeWitness() int {
	// ValueIn (8 bytes) + BlockHeight (4 bytes) + BlockIndex (4 bytes) +
	// serialized varint size for the length of SignatureScript +
	// SignatureScript bytes.
	return 8 + 4 + 4 + VarIntSerializeSize(uint64(len(t.SignatureScript))) +
		len(t.SignatureScript)
}

// SerializeSizeWitnessSigning returns the number of bytes it would take to
// serialize the transaction input for a witness used in signing.
func (t *TxIn) SerializeSizeWitnessSigning() int {
	// Serialized varint size for the length of SignatureScript +
	// SignatureScript bytes.
	return VarIntSerializeSize(uint64(len(t.SignatureScript))) +
		len(t.SignatureScript)
}

// SerializeSizeWitnessValueSigning returns the number of bytes it would take to
// serialize the transaction input for a witness used in signing with value
// included.
func (t *TxIn) SerializeSizeWitnessValueSigning() int {
	// ValueIn (8 bytes) + serialized varint size for the length of
	// SignatureScript + SignatureScript bytes.
	return 8 + VarIntSerializeSize(uint64(len(t.SignatureScript))) +
		len(t.SignatureScript)
}

// LegacySerializeSize returns the number of bytes it would take to serialize the
// the transaction input.
func (t *TxIn) LegacySerializeSize() int {
	// Outpoint Hash 32 bytes + Outpoint Index 4 bytes + Sequence 4 bytes +
	// serialized varint size for the length of SignatureScript +
	// SignatureScript bytes.
	return 41 + VarIntSerializeSize(uint64(len(t.SignatureScript))) +
		len(t.SignatureScript)
}

// NewTxIn returns a new decred transaction input with the provided
// previous outpoint point and signature script with a default sequence of
// MaxTxInSequenceNum.
func NewTxIn(prevOut *OutPoint, signatureScript []byte) *TxIn {
	return &TxIn{
		PreviousOutPoint: *prevOut,
		Sequence:         MaxTxInSequenceNum,
		SignatureScript:  signatureScript,
		ValueIn:          NullValueIn,
		BlockHeight:      NullBlockHeight,
		BlockIndex:       NullBlockIndex,
	}
}

// TxOut defines a decred transaction output.
type TxOut struct {
	Value    int64
	Version  uint16
	PkScript []byte
}

// SerializeSize returns the number of bytes it would take to serialize the
// the transaction output.
func (t *TxOut) SerializeSize() int {
	// Value 8 bytes + Version 2 bytes + serialized varint size for
	// the length of PkScript + PkScript bytes.
	return 8 + 2 + VarIntSerializeSize(uint64(len(t.PkScript))) + len(t.PkScript)
}

// NewTxOut returns a new decred transaction output with the provided
// transaction value and public key script.
func NewTxOut(value int64, pkScript []byte) *TxOut {
	return &TxOut{
		Value:    value,
		Version:  DefaultPkScriptVersion,
		PkScript: pkScript,
	}
}

// MsgTx implements the Message interface and represents a decred tx message.
// It is used to deliver transaction information in response to a getdata
// message (MsgGetData) for a given transaction.
//
// Use the AddTxIn and AddTxOut functions to build up the list of transaction
// inputs and outputs.
type MsgTx struct {
	CachedHash *chainhash.Hash
	Version    int32
	TxIn       []*TxIn
	TxOut      []*TxOut
	LockTime   uint32
	Expiry     uint32
}

// AddTxIn adds a transaction input to the message.
func (msg *MsgTx) AddTxIn(ti *TxIn) {
	msg.TxIn = append(msg.TxIn, ti)
}

// AddTxOut adds a transaction output to the message.
func (msg *MsgTx) AddTxOut(to *TxOut) {
	msg.TxOut = append(msg.TxOut, to)
}

// msgTxVersionToBytes converts an int32 version into a 4 byte slice.
func msgTxVersionToBytes(version int32) []byte {
	mVerBytes := make([]byte, 4, 4)
	binary.LittleEndian.PutUint32(mVerBytes[0:4], uint32(version))
	return mVerBytes
}

// msgTxVersionDecode converts an int32 version into serialization types and
// actual version.
func msgTxVersionToVars(version int32) (uint16, TxSerializeType) {
	mVerBytes := make([]byte, 4, 4)
	binary.LittleEndian.PutUint32(mVerBytes[0:4], uint32(version))
	mVer := binary.LittleEndian.Uint16(mVerBytes[0:2])
	mType := binary.LittleEndian.Uint16(mVerBytes[2:4])
	return mVer, TxSerializeType(mType)
}

// msgTxVersionDecode converts a 4 byte slice into an int32 version.
func msgTxVersionDecode(verBytes []byte) (int32, error) {
	if len(verBytes) != 4 {
		return 0, messageError("msgTxVersionDecode", "tx version wrong size")
	}
	ver := binary.LittleEndian.Uint32(verBytes)

	return int32(ver), nil
}

// shallowCopyForSerializing make a shallow copy of a tx with a new
// version, so that it can be hashed or serialized accordingly.
func (msg *MsgTx) shallowCopyForSerializing(version int32) *MsgTx {
	return &MsgTx{
		Version:  version,
		TxIn:     msg.TxIn,
		TxOut:    msg.TxOut,
		LockTime: msg.LockTime,
		Expiry:   msg.Expiry,
	}
}

// TxSha generates the Hash name for the transaction prefix.
func (msg *MsgTx) TxSha() chainhash.Hash {
	// Encode the transaction and calculate double sha256 on the result.
	// Ignore the error returns since the only way the encode could fail
	// is being out of memory or due to nil pointers, both of which would
	// cause a run-time panic.

	// TxSha should always calculate a non-witnessed hash.
	mtxCopy := msg.shallowCopyForSerializing(NoWitnessMsgTxVersion())

	buf := bytes.NewBuffer(make([]byte, 0, mtxCopy.SerializeSize()))
	err := mtxCopy.Serialize(buf)
	if err != nil {
		panic("MsgTx failed serializing for TxSha")
	}

	return chainhash.HashFuncH(buf.Bytes())
}

// CachedTxSha generates the Hash name for the transaction prefix and stores
// it if it does not exist. The cached hash is then returned. It can be
// recalculated later with RecacheTxSha.
func (msg *MsgTx) CachedTxSha() *chainhash.Hash {
	if msg.CachedHash == nil {
		h := msg.TxSha()
		msg.CachedHash = &h
	}

	return msg.CachedHash
}

// RecacheTxSha generates the Hash name for the transaction prefix and stores
// it. The cached hash is then returned.
func (msg *MsgTx) RecacheTxSha() *chainhash.Hash {
	h := msg.TxSha()
	msg.CachedHash = &h

	return msg.CachedHash
}

// TxShaWitness generates the Hash name for the transaction witness.
func (msg *MsgTx) TxShaWitness() chainhash.Hash {
	// Encode the transaction and calculate double sha256 on the result.
	// Ignore the error returns since the only way the encode could fail
	// is being out of memory or due to nil pointers, both of which would
	// cause a run-time panic.

	// TxShaWitness should always calculate a witnessed hash.
	mtxCopy := msg.shallowCopyForSerializing(WitnessOnlyMsgTxVersion())

	buf := bytes.NewBuffer(make([]byte, 0, mtxCopy.SerializeSize()))
	err := mtxCopy.Serialize(buf)
	if err != nil {
		panic("MsgTx failed serializing for TxShaWitness")
	}

	return chainhash.HashFuncH(buf.Bytes())
}

// TxShaWitnessSigning generates the Hash name for the transaction witness with
// the malleable portions (AmountIn, BlockHeight, BlockIndex) removed. These are
// verified and set by the miner instead.
func (msg *MsgTx) TxShaWitnessSigning() chainhash.Hash {
	// Encode the transaction and calculate double sha256 on the result.
	// Ignore the error returns since the only way the encode could fail
	// is being out of memory or due to nil pointers, both of which would
	// cause a run-time panic.

	// TxShaWitness should always calculate a witnessed hash.
	mtxCopy := msg.shallowCopyForSerializing(WitnessSigningMsgTxVersion())

	buf := bytes.NewBuffer(make([]byte, 0, mtxCopy.SerializeSize()))
	err := mtxCopy.Serialize(buf)
	if err != nil {
		panic("MsgTx failed serializing for TxShaWitnessSigning")
	}

	return chainhash.HashFuncH(buf.Bytes())
}

// TxShaWitnessValueSigning generates the Hash name for the transaction witness
// with BlockHeight and BlockIndex removed, allowing the signer to specify the
// ValueIn.
func (msg *MsgTx) TxShaWitnessValueSigning() chainhash.Hash {
	// Encode the transaction and calculate double sha256 on the result.
	// Ignore the error returns since the only way the encode could fail
	// is being out of memory or due to nil pointers, both of which would
	// cause a run-time panic.

	// TxShaWitness should always calculate a witnessed hash.
	mtxCopy := msg.shallowCopyForSerializing(WitnessValueSigningMsgTxVersion())

	buf := bytes.NewBuffer(make([]byte, 0, mtxCopy.SerializeSize()))
	err := mtxCopy.Serialize(buf)
	if err != nil {
		panic("MsgTx failed serializing for TxShaWitnessValueSigning")
	}

	return chainhash.HashFuncH(buf.Bytes())
}

// TxShaFull generates the Hash name for the transaction prefix || witness. It
// first obtains the hashes for both the transaction prefix and witness, then
// concatenates them and hashes these 64 bytes.
// Note that the inputs to the hashes, serialized prefix and serialized witnesses,
// have different uint32 versions because version is now actually two uint16s,
// with the last 16 bits referring to the serialization type. The first 16 bits
// refer to the actual version, and these must be the same in both serializations.
func (msg *MsgTx) TxShaFull() chainhash.Hash {
	concat := make([]byte, 64, 64)
	prefixHash := msg.TxSha()
	witnessHash := msg.TxShaWitness()
	copy(concat[0:32], prefixHash[:])
	copy(concat[32:64], witnessHash[:])

	return chainhash.HashFuncH(concat)
}

// TxShaLegacy generates the legacy transaction hash, for software
// compatibility.
func (msg *MsgTx) TxShaLegacy() chainhash.Hash {
	// Encode the transaction and calculate double sha256 on the result.
	// Ignore the error returns since the only way the encode could fail
	// is being out of memory or due to nil pointers, both of which would
	// cause a run-time panic.
	buf := bytes.NewBuffer(make([]byte, 0, msg.SerializeSize()))
	err := msg.LegacySerialize(buf)
	if err != nil {
		panic("MsgTx failed serializing for TxShaLegacy")
	}

	return chainhash.HashFuncH(buf.Bytes())
}

// Copy creates a deep copy of a transaction so that the original does not get
// modified when the copy is manipulated.
func (msg *MsgTx) Copy() *MsgTx {
	// Create new tx and start by copying primitive values and making space
	// for the transaction inputs and outputs.
	newTx := MsgTx{
		Version:  msg.Version,
		TxIn:     make([]*TxIn, 0, len(msg.TxIn)),
		TxOut:    make([]*TxOut, 0, len(msg.TxOut)),
		LockTime: msg.LockTime,
		Expiry:   msg.Expiry,
	}

	// Deep copy the old TxIn data.
	for _, oldTxIn := range msg.TxIn {
		// Deep copy the old previous outpoint.
		oldOutPoint := oldTxIn.PreviousOutPoint
		newOutPoint := OutPoint{}
		newOutPoint.Hash.SetBytes(oldOutPoint.Hash[:])
		newOutPoint.Index = oldOutPoint.Index
		newOutPoint.Tree = oldOutPoint.Tree

		// Deep copy the old signature script.
		var newScript []byte
		oldScript := oldTxIn.SignatureScript
		oldScriptLen := len(oldScript)
		if oldScriptLen > 0 {
			newScript = make([]byte, oldScriptLen, oldScriptLen)
			copy(newScript, oldScript[:oldScriptLen])
		}

		// Create new txIn with the deep copied data and append it to
		// new Tx.
		newTxIn := TxIn{
			PreviousOutPoint: newOutPoint,
			Sequence:         oldTxIn.Sequence,
			ValueIn:          oldTxIn.ValueIn,
			BlockHeight:      oldTxIn.BlockHeight,
			BlockIndex:       oldTxIn.BlockIndex,
			SignatureScript:  newScript,
		}
		newTx.TxIn = append(newTx.TxIn, &newTxIn)
	}

	// Deep copy the old TxOut data.
	for _, oldTxOut := range msg.TxOut {
		// Deep copy the old PkScript
		var newScript []byte
		oldScript := oldTxOut.PkScript
		oldScriptLen := len(oldScript)
		if oldScriptLen > 0 {
			newScript = make([]byte, oldScriptLen, oldScriptLen)
			copy(newScript, oldScript[:oldScriptLen])
		}

		// Create new txOut with the deep copied data and append it to
		// new Tx.
		newTxOut := TxOut{
			Value:    oldTxOut.Value,
			Version:  oldTxOut.Version,
			PkScript: newScript,
		}
		newTx.TxOut = append(newTx.TxOut, &newTxOut)
	}

	return &newTx
}

// decodePrefix decodes a transaction prefix and stores the contents
// in the embedded msgTx.
func (msg *MsgTx) decodePrefix(r io.Reader, pver uint32) error {
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	// Prevent more input transactions than could possibly fit into a
	// message.  It would be possible to cause memory exhaustion and panics
	// without a sane upper bound on this count.
	if count > uint64(maxTxInPerMessage) {
		str := fmt.Sprintf("too many input transactions to fit into "+
			"max message size [count %d, max %d]", count,
			maxTxInPerMessage)
		return messageError("MsgTx.decodePrefix", str)
	}

	// TxIns.
	txIns := make([]TxIn, count)
	msg.TxIn = make([]*TxIn, count)
	for i := uint64(0); i < count; i++ {
		ti := &txIns[i]
		err = readTxInPrefix(r, pver, msg.Version, ti)
		if err != nil {
			return err
		}
		msg.TxIn[i] = ti
	}

	count, err = ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	// Prevent more output transactions than could possibly fit into a
	// message.  It would be possible to cause memory exhaustion and panics
	// without a sane upper bound on this count.
	if count > uint64(maxTxOutPerMessage) {
		str := fmt.Sprintf("too many output transactions to fit into "+
			"max message size [count %d, max %d]", count,
			maxTxOutPerMessage)
		return messageError("MsgTx.decodePrefix", str)
	}

	// TxOuts.
	txOuts := make([]TxOut, count)
	msg.TxOut = make([]*TxOut, count)
	for i := uint64(0); i < count; i++ {
		to := &txOuts[i]
		err = readTxOut(r, pver, msg.Version, to)
		if err != nil {
			return err
		}
		msg.TxOut[i] = to
	}

	// Locktime and expiry.
	msg.LockTime, err = binarySerializer.Uint32(r, littleEndian)
	if err != nil {
		return err
	}

	msg.Expiry, err = binarySerializer.Uint32(r, littleEndian)
	if err != nil {
		return err
	}

	return nil
}

func (msg *MsgTx) decodeWitness(r io.Reader, pver uint32, isFull bool) error {
	// Witness only; generate the TxIn list and fill out only the
	// sigScripts.
	if !isFull {
		count, err := ReadVarInt(r, pver)
		if err != nil {
			return err
		}

		// Prevent more input transactions than could possibly fit into a
		// message.  It would be possible to cause memory exhaustion and panics
		// without a sane upper bound on this count.
		if count > uint64(maxTxInPerMessage) {
			str := fmt.Sprintf("too many input transactions to fit into "+
				"max message size [count %d, max %d]", count,
				maxTxInPerMessage)
			return messageError("MsgTx.decodeWitness", str)
		}

		txIns := make([]TxIn, count)
		msg.TxIn = make([]*TxIn, count)
		for i := uint64(0); i < count; i++ {
			ti := &txIns[i]
			err = readTxInWitness(r, pver, msg.Version, ti)
			if err != nil {
				return err
			}
			msg.TxIn[i] = ti
		}
		msg.TxOut = make([]*TxOut, 0)
	} else {
		// We're decoding witnesses from a full transaction, so read in
		// the number of signature scripts, check to make sure it's the
		// same as the number of TxIns we currently have, then fill in
		// the signature scripts.
		count, err := ReadVarInt(r, pver)
		if err != nil {
			return err
		}

		// Don't allow the deserializer to panic by accessing memory
		// that doesn't exist.
		if int(count) != len(msg.TxIn) {
			str := fmt.Sprintf("non equal witness and prefix txin quantities "+
				"(witness %v, prefix %v)", count,
				len(msg.TxIn))
			return messageError("MsgTx.decodeWitness", str)
		}

		// Prevent more input transactions than could possibly fit into a
		// message.  It would be possible to cause memory exhaustion and panics
		// without a sane upper bound on this count.
		if count > uint64(maxTxInPerMessage) {
			str := fmt.Sprintf("too many input transactions to fit into "+
				"max message size [count %d, max %d]", count,
				maxTxInPerMessage)
			return messageError("MsgTx.decodeWitness", str)
		}

		// Read in the witnesses, and copy them into the already generated
		// by decodePrefix TxIns.
		txIns := make([]TxIn, count)
		for i := uint64(0); i < count; i++ {
			ti := &txIns[i]
			err = readTxInWitness(r, pver, msg.Version, ti)
			if err != nil {
				return err
			}

			msg.TxIn[i].ValueIn = ti.ValueIn
			msg.TxIn[i].BlockHeight = ti.BlockHeight
			msg.TxIn[i].BlockIndex = ti.BlockIndex
			msg.TxIn[i].SignatureScript = ti.SignatureScript
		}
	}

	return nil
}

// decodeWitnessSigning decodes a witness for signing.
func (msg *MsgTx) decodeWitnessSigning(r io.Reader, pver uint32) error {
	// Witness only for signing; generate the TxIn list and fill out only the
	// sigScripts.
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	// Prevent more input transactions than could possibly fit into a
	// message.  It would be possible to cause memory exhaustion and panics
	// without a sane upper bound on this count.
	if count > uint64(maxTxInPerMessage) {
		str := fmt.Sprintf("too many input transactions to fit into "+
			"max message size [count %d, max %d]", count,
			maxTxInPerMessage)
		return messageError("MsgTx.decodeWitness", str)
	}

	txIns := make([]TxIn, count)
	msg.TxIn = make([]*TxIn, count)
	for i := uint64(0); i < count; i++ {
		ti := &txIns[i]
		err = readTxInWitnessSigning(r, pver, msg.Version, ti)
		if err != nil {
			return err
		}
		msg.TxIn[i] = ti
	}
	msg.TxOut = make([]*TxOut, 0)

	return nil
}

// decodeWitnessValueSigning decodes a witness for signing with value.
func (msg *MsgTx) decodeWitnessValueSigning(r io.Reader, pver uint32) error {
	// Witness only for signing; generate the TxIn list and fill out only the
	// sigScripts.
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	// Prevent more input transactions than could possibly fit into a
	// message.  It would be possible to cause memory exhaustion and panics
	// without a sane upper bound on this count.
	if count > uint64(maxTxInPerMessage) {
		str := fmt.Sprintf("too many input transactions to fit into "+
			"max message size [count %d, max %d]", count,
			maxTxInPerMessage)
		return messageError("MsgTx.decodeWitness", str)
	}

	txIns := make([]TxIn, count)
	msg.TxIn = make([]*TxIn, count)
	for i := uint64(0); i < count; i++ {
		ti := &txIns[i]
		err = readTxInWitnessValueSigning(r, pver, msg.Version, ti)
		if err != nil {
			return err
		}
		msg.TxIn[i] = ti
	}
	msg.TxOut = make([]*TxOut, 0)

	return nil
}

// BtcDecode decodes r using the decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
// See Deserialize for decoding transactions stored to disk, such as in a
// database, as opposed to decoding transactions from the wire.
func (msg *MsgTx) BtcDecode(r io.Reader, pver uint32) error {
	version, err := binarySerializer.Uint32(r, littleEndian)
	if err != nil {
		return err
	}
	msg.Version = int32(version)
	_, mType := msgTxVersionToVars(msg.Version)

	switch {
	case mType == TxSerializeNoWitness:
		err := msg.decodePrefix(r, pver)
		if err != nil {
			return err
		}

	case mType == TxSerializeOnlyWitness:
		err := msg.decodeWitness(r, pver, false)
		if err != nil {
			return err
		}

	case mType == TxSerializeWitnessSigning:
		err := msg.decodeWitnessSigning(r, pver)
		if err != nil {
			return err
		}

	case mType == TxSerializeWitnessValueSigning:
		err := msg.decodeWitnessValueSigning(r, pver)
		if err != nil {
			return err
		}

	case mType == TxSerializeFull:
		err := msg.decodePrefix(r, pver)
		if err != nil {
			return err
		}
		err = msg.decodeWitness(r, pver, true)
		if err != nil {
			return err
		}
	default:
		return messageError("MsgTx.BtcDecode", "unsupported transaction type")
	}

	return nil
}

// LegacyBtcDecode decodes r using the decred protocol encoding into the
// receiver. This is used for the decoding of legacy serialized transactions.
func (msg *MsgTx) LegacyBtcDecode(r io.Reader, pver uint32) error {
	version, err := binarySerializer.Uint32(r, littleEndian)
	if err != nil {
		return err
	}
	msg.Version = int32(version)

	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	// Prevent more input transactions than could possibly fit into a
	// message.  It would be possible to cause memory exhaustion and panics
	// without a sane upper bound on this count.
	if count > uint64(maxTxInPerMessage) {
		str := fmt.Sprintf("too many input transactions to fit into "+
			"max message size [count %d, max %d]", count,
			maxTxInPerMessage)
		return messageError("MsgTx.BtcDecode", str)
	}

	// Deserialize the inputs.
	txIns := make([]TxIn, count)
	msg.TxIn = make([]*TxIn, count)
	for i := uint64(0); i < count; i++ {
		ti := &txIns[i]
		err = legacyReadTxIn(r, pver, msg.Version, ti)
		if err != nil {
			return err
		}
		msg.TxIn[i] = ti
	}

	count, err = ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	// Prevent more output transactions than could possibly fit into a
	// message.  It would be possible to cause memory exhaustion and panics
	// without a sane upper bound on this count.
	if count > uint64(maxTxOutPerMessage) {
		str := fmt.Sprintf("too many output transactions to fit into "+
			"max message size [count %d, max %d]", count,
			maxTxOutPerMessage)
		return messageError("MsgTx.BtcDecode", str)
	}

	// Deserialize the outputs.
	txOuts := make([]TxOut, count)
	msg.TxOut = make([]*TxOut, count)
	for i := uint64(0); i < count; i++ {
		to := &txOuts[i]
		err = legacyReadTxOut(r, pver, msg.Version, to)
		if err != nil {
			return err
		}
		msg.TxOut[i] = to
	}

	msg.LockTime, err = binarySerializer.Uint32(r, littleEndian)
	if err != nil {
		return err
	}

	return nil
}

// Deserialize decodes a transaction from r into the receiver using a format
// that is suitable for long-term storage such as a database while respecting
// the Version field in the transaction.  This function differs from BtcDecode
// in that BtcDecode decodes from the Decred wire protocol as it was sent
// across the network.  The wire encoding can technically differ depending on
// the protocol version and doesn't even really need to match the format of a
// stored transaction at all.  As of the time this comment was written, the
// encoded transaction is the same in both instances, but there is a distinct
// difference and separating the two allows the API to be flexible enough to
// deal with changes.
func (msg *MsgTx) Deserialize(r io.Reader) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol version 0 and the stable long-term storage format.  As
	// a result, make use of BtcDecode.
	return msg.BtcDecode(r, 0)
}

// LegacyDeserialize decodes a transaction that has been encoded in the legacy
// Decred format.
func (msg *MsgTx) LegacyDeserialize(r io.Reader) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol version 0 and the stable long-term storage format.  As
	// a result, make use of BtcDecode.
	return msg.LegacyBtcDecode(r, 0)
}

// FromBytes deserializes a transaction byte slice.
func (msg *MsgTx) FromBytes(b []byte) error {
	r := bytes.NewReader(b)
	return msg.Deserialize(r)
}

// encodePrefix encodes a transaction prefix into a writer.
func (msg *MsgTx) encodePrefix(w io.Writer, pver uint32) error {
	count := uint64(len(msg.TxIn))
	err := WriteVarInt(w, pver, count)
	if err != nil {
		return err
	}

	for _, ti := range msg.TxIn {
		err = writeTxInPrefix(w, pver, msg.Version, ti)
		if err != nil {
			return err
		}
	}

	count = uint64(len(msg.TxOut))
	err = WriteVarInt(w, pver, count)
	if err != nil {
		return err
	}

	for _, to := range msg.TxOut {
		err = writeTxOut(w, pver, msg.Version, to)
		if err != nil {
			return err
		}
	}

	err = binarySerializer.PutUint32(w, littleEndian, msg.LockTime)
	if err != nil {
		return err
	}

	err = binarySerializer.PutUint32(w, littleEndian, msg.Expiry)
	if err != nil {
		return err
	}

	return nil
}

// encodeWitness encodes a transaction witness into a writer.
func (msg *MsgTx) encodeWitness(w io.Writer, pver uint32) error {
	count := uint64(len(msg.TxIn))
	err := WriteVarInt(w, pver, count)
	if err != nil {
		return err
	}

	for _, ti := range msg.TxIn {
		err = writeTxInWitness(w, pver, msg.Version, ti)
		if err != nil {
			return err
		}
	}

	return nil
}

// encodeWitnessSigning encodes a transaction witness into a writer for signing.
func (msg *MsgTx) encodeWitnessSigning(w io.Writer, pver uint32) error {
	count := uint64(len(msg.TxIn))
	err := WriteVarInt(w, pver, count)
	if err != nil {
		return err
	}

	for _, ti := range msg.TxIn {
		err = writeTxInWitnessSigning(w, pver, msg.Version, ti)
		if err != nil {
			return err
		}
	}

	return nil
}

// encodeWitnessValueSigning encodes a transaction witness into a writer for
// signing, with the value included.
func (msg *MsgTx) encodeWitnessValueSigning(w io.Writer, pver uint32) error {
	count := uint64(len(msg.TxIn))
	err := WriteVarInt(w, pver, count)
	if err != nil {
		return err
	}

	for _, ti := range msg.TxIn {
		err = writeTxInWitnessValueSigning(w, pver, msg.Version, ti)
		if err != nil {
			return err
		}
	}

	return nil
}

// BtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is part of the Message interface implementation.
// See Serialize for encoding transactions to be stored to disk, such as in a
// database, as opposed to encoding transactions for the wire.
func (msg *MsgTx) BtcEncode(w io.Writer, pver uint32) error {
	err := binarySerializer.PutUint32(w, littleEndian, uint32(msg.Version))
	if err != nil {
		return err
	}
	_, mType := msgTxVersionToVars(msg.Version)

	switch {
	case mType == TxSerializeNoWitness:
		err := msg.encodePrefix(w, pver)
		if err != nil {
			return err
		}

	case mType == TxSerializeOnlyWitness:
		err := msg.encodeWitness(w, pver)
		if err != nil {
			return err
		}

	case mType == TxSerializeWitnessSigning:
		err := msg.encodeWitnessSigning(w, pver)
		if err != nil {
			return err
		}

	case mType == TxSerializeWitnessValueSigning:
		err := msg.encodeWitnessValueSigning(w, pver)
		if err != nil {
			return err
		}

	case mType == TxSerializeFull:
		err := msg.encodePrefix(w, pver)
		if err != nil {
			return err
		}
		err = msg.encodeWitness(w, pver)
		if err != nil {
			return err
		}

	default:
		return messageError("MsgTx.BtcEncode", "unsupported transaction type")
	}

	return nil
}

// LegacyBtcEncode encodes the receiver to w using the Decred protocol encoding.
// This is for transactions encoded in the legacy encoding, for compatibility.
func (msg *MsgTx) LegacyBtcEncode(w io.Writer, pver uint32) error {
	err := binarySerializer.PutUint32(w, littleEndian, uint32(msg.Version))
	if err != nil {
		return err
	}

	count := uint64(len(msg.TxIn))
	err = WriteVarInt(w, pver, count)
	if err != nil {
		return err
	}

	for _, ti := range msg.TxIn {
		err = legacyWriteTxIn(w, pver, msg.Version, ti)
		if err != nil {
			return err
		}
	}

	count = uint64(len(msg.TxOut))
	err = WriteVarInt(w, pver, count)
	if err != nil {
		return err
	}

	for _, to := range msg.TxOut {
		err = legacyWriteTxOut(w, pver, msg.Version, to)
		if err != nil {
			return err
		}
	}

	err = binarySerializer.PutUint32(w, littleEndian, msg.LockTime)
	if err != nil {
		return err
	}

	return nil
}

// Serialize encodes the transaction to w using a format that suitable for
// long-term storage such as a database while respecting the Version field in
// the transaction.  This function differs from BtcEncode in that BtcEncode
// encodes the transaction to the decred wire protocol in order to be sent
// across the network.  The wire encoding can technically differ depending on
// the protocol version and doesn't even really need to match the format of a
// stored transaction at all.  As of the time this comment was written, the
// encoded transaction is the same in both instances, but there is a distinct
// difference and separating the two allows the API to be flexible enough to
// deal with changes.
func (msg *MsgTx) Serialize(w io.Writer) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol version 0 and the stable long-term storage format.  As
	// a result, make use of BtcEncode.
	return msg.BtcEncode(w, 0)
}

// LegacySerialize encodes the transaction in decred legacy format, for
// compatibility.
func (msg *MsgTx) LegacySerialize(w io.Writer) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol version 0 and the stable long-term storage format.  As
	// a result, make use of BtcEncode.
	return msg.LegacyBtcEncode(w, 0)
}

// Bytes returns the serialized form of the transaction in bytes.
func (msg *MsgTx) Bytes() ([]byte, error) {
	// Serialize the MsgTx.
	var w bytes.Buffer
	err := msg.Serialize(&w)
	if err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// BytesPrefix returns the serialized form of the transaction prefix in bytes.
func (msg *MsgTx) BytesPrefix() ([]byte, error) {
	mtxCopy := msg.shallowCopyForSerializing(NoWitnessMsgTxVersion())

	var w bytes.Buffer
	err := mtxCopy.Serialize(&w)
	if err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// BytesWitness returns the serialized form of the transaction prefix in bytes.
func (msg *MsgTx) BytesWitness() ([]byte, error) {
	mtxCopy := msg.shallowCopyForSerializing(WitnessOnlyMsgTxVersion())

	var w bytes.Buffer
	err := mtxCopy.Serialize(&w)
	if err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// SerializeSize returns the number of bytes it would take to serialize the
// the transaction.
func (msg *MsgTx) SerializeSize() int {
	_, mType := msgTxVersionToVars(msg.Version)

	// Unknown type return 0.
	n := 0
	switch {
	case mType == TxSerializeNoWitness:
		// Version 4 bytes + LockTime 4 bytes + Expiry 4 bytes +
		// Serialized varint size for the number of transaction
		// inputs and outputs.
		n = 12 + VarIntSerializeSize(uint64(len(msg.TxIn))) +
			VarIntSerializeSize(uint64(len(msg.TxOut)))

		for _, txIn := range msg.TxIn {
			n += txIn.SerializeSizePrefix()
		}
		for _, txOut := range msg.TxOut {
			n += txOut.SerializeSize()
		}

	case mType == TxSerializeOnlyWitness:
		// Version 4 bytes + Serialized varint size for the
		// number of transaction signatures.
		n = 4 + VarIntSerializeSize(uint64(len(msg.TxIn)))

		for _, txIn := range msg.TxIn {
			n += txIn.SerializeSizeWitness()
		}

	case mType == TxSerializeWitnessSigning:
		// Version 4 bytes + Serialized varint size for the
		// number of transaction signatures.
		n = 4 + VarIntSerializeSize(uint64(len(msg.TxIn)))

		for _, txIn := range msg.TxIn {
			n += txIn.SerializeSizeWitnessSigning()
		}

	case mType == TxSerializeWitnessValueSigning:
		// Version 4 bytes + Serialized varint size for the
		// number of transaction signatures.
		n = 4 + VarIntSerializeSize(uint64(len(msg.TxIn)))

		for _, txIn := range msg.TxIn {
			n += txIn.SerializeSizeWitnessValueSigning()
		}

	case mType == TxSerializeFull:
		// Version 4 bytes + LockTime 4 bytes + Expiry 4 bytes + Serialized
		// varint size for the number of transaction inputs (x2) and
		// outputs. The number of inputs is added twice because it's
		// encoded once in both the witness and the prefix.
		n = 12 + VarIntSerializeSize(uint64(len(msg.TxIn))) +
			VarIntSerializeSize(uint64(len(msg.TxIn))) +
			VarIntSerializeSize(uint64(len(msg.TxOut)))

		for _, txIn := range msg.TxIn {
			n += txIn.SerializeSizePrefix()
		}
		for _, txIn := range msg.TxIn {
			n += txIn.SerializeSizeWitness()
		}
		for _, txOut := range msg.TxOut {
			n += txOut.SerializeSize()
		}
	}

	return n
}

// LegacySerializeSize returns the number of bytes it would take to serialize
// the transaction.
func (msg *MsgTx) LegacySerializeSize() int {
	// Version 4 bytes + LockTime 4 bytes + Expiry 4 bytes + Serialized
	// varint size for the number of transaction inputs and outputs.
	n := 12 + VarIntSerializeSize(uint64(len(msg.TxIn))) +
		VarIntSerializeSize(uint64(len(msg.TxOut)))

	for _, txIn := range msg.TxIn {
		n += txIn.LegacySerializeSize()
	}

	for _, txOut := range msg.TxOut {
		n += txOut.SerializeSize()
	}

	return n
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgTx) Command() string {
	return CmdTx
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgTx) MaxPayloadLength(pver uint32) uint32 {
	return MaxBlockPayload
}

// PkScriptLocs returns a slice containing the start of each public key script
// within the raw serialized transaction.  The caller can easily obtain the
// length of each script by using len on the script available via the
// appropriate transaction output entry.
// TODO: Make this work for all serialization types, not just the full
// serialization type.
func (msg *MsgTx) PkScriptLocs() []int {
	// Return nil for witness-only tx.
	numTxOut := len(msg.TxOut)
	if numTxOut == 0 {
		return nil
	}

	// The starting offset in the serialized transaction of the first
	// transaction output is:
	//
	// Version 4 bytes + serialized varint size for the number of
	// transaction inputs and outputs + serialized size of each transaction
	// input.
	n := 4 + VarIntSerializeSize(uint64(len(msg.TxIn))) +
		VarIntSerializeSize(uint64(numTxOut))
	for _, txIn := range msg.TxIn {
		n += txIn.SerializeSizePrefix()
	}

	// Calculate and set the appropriate offset for each public key script.
	pkScriptLocs := make([]int, numTxOut)
	for i, txOut := range msg.TxOut {
		// The offset of the script in the transaction output is:
		//
		// Value 8 bytes + version 2 bytes + serialized varint size
		// for the length of PkScript.
		n += 8 + 2 + VarIntSerializeSize(uint64(len(txOut.PkScript)))
		pkScriptLocs[i] = n
		n += len(txOut.PkScript)
	}

	return pkScriptLocs
}

// LegacyPkScriptLocs returns a slice containing the start of each public key
// script within the raw serialized transaction.  The caller can easily obtain
// the length of each script by using len on the script available via the
// appropriate transaction output entry. This is for legacy decred format.
func (msg *MsgTx) LegacyPkScriptLocs() []int {
	numTxOut := len(msg.TxOut)
	if numTxOut == 0 {
		return nil
	}

	// The starting offset in the serialized transaction of the first
	// transaction output is:
	//
	// Version 4 bytes + serialized varint size for the number of
	// transaction inputs and outputs + serialized size of each transaction
	// input.
	n := 4 + VarIntSerializeSize(uint64(len(msg.TxIn))) +
		VarIntSerializeSize(uint64(numTxOut))
	for _, txIn := range msg.TxIn {
		n += txIn.LegacySerializeSize()
	}

	// Calculate and set the appropriate offset for each public key script.
	pkScriptLocs := make([]int, numTxOut)
	for i, txOut := range msg.TxOut {
		// The offset of the script in the transaction output is:
		//
		// Value 8 bytes + serialized varint size for the length of
		// PkScript.
		n += 8 + VarIntSerializeSize(uint64(len(txOut.PkScript)))
		pkScriptLocs[i] = n
		n += len(txOut.PkScript)
	}

	return pkScriptLocs
}

// NewMsgTx returns a new decred tx message that conforms to the Message
// interface.  The return instance has a default version of TxVersion and there
// are no transaction inputs or outputs.  Also, the lock time is set to zero
// to indicate the transaction is valid immediately as opposed to some time in
// future.
func NewMsgTx() *MsgTx {
	return &MsgTx{
		Version: DefaultMsgTxVersion(),
		TxIn:    make([]*TxIn, 0, defaultTxInOutAlloc),
		TxOut:   make([]*TxOut, 0, defaultTxInOutAlloc),
	}
}

// ReadOutPoint reads the next sequence of bytes from r as an OutPoint.
func ReadOutPoint(r io.Reader, pver uint32, version int32, op *OutPoint) error {
	_, err := io.ReadFull(r, op.Hash[:])
	if err != nil {
		return err
	}

	op.Index, err = binarySerializer.Uint32(r, littleEndian)
	if err != nil {
		return err
	}

	tree, err := binarySerializer.Uint8(r)
	if err != nil {
		return err
	}
	op.Tree = int8(tree)

	return nil
}

// WriteOutPoint encodes op to the decred protocol encoding for an OutPoint
// to w.
func WriteOutPoint(w io.Writer, pver uint32, version int32, op *OutPoint) error {
	_, err := w.Write(op.Hash[:])
	if err != nil {
		return err
	}

	err = binarySerializer.PutUint32(w, littleEndian, op.Index)
	if err != nil {
		return err
	}

	err = binarySerializer.PutUint8(w, uint8(op.Tree))
	if err != nil {
		return err
	}

	return nil
}

// legacyReadOutPoint reads the next sequence of bytes from r as a legacy
// Decred OutPoint.
func legacyReadOutPoint(r io.Reader, pver uint32, version int32,
	op *OutPoint) error {
	_, err := io.ReadFull(r, op.Hash[:])
	if err != nil {
		return err
	}

	op.Index, err = binarySerializer.Uint32(r, littleEndian)
	if err != nil {
		return err
	}

	return nil
}

// legacyWriteOutPoint encodes op to the decred protocol encoding for a legacy
// Decred OutPoint to w.
func legacyWriteOutPoint(w io.Writer, pver uint32, version int32,
	op *OutPoint) error {
	_, err := w.Write(op.Hash[:])
	if err != nil {
		return err
	}

	err = binarySerializer.PutUint32(w, littleEndian, op.Index)
	if err != nil {
		return err
	}

	return nil
}

// readTxInPrefix reads the next sequence of bytes from r as a transaction input
// (TxIn) in the transaction prefix.
func readTxInPrefix(r io.Reader, pver uint32, version int32, ti *TxIn) error {
	if version == WitnessOnlyMsgTxVersion() {
		return messageError("readTxInPrefix",
			"tried to read a prefix input for a witness only tx")
	}

	// Outpoint.
	var op OutPoint
	err := ReadOutPoint(r, pver, version, &op)
	if err != nil {
		return err
	}
	ti.PreviousOutPoint = op

	// Sequence.
	ti.Sequence, err = binarySerializer.Uint32(r, littleEndian)
	if err != nil {
		return err
	}

	return nil
}

// readTxInWitness reads the next sequence of bytes from r as a transaction input
// (TxIn) in the transaction witness.
func readTxInWitness(r io.Reader, pver uint32, version int32, ti *TxIn) error {
	// ValueIn.
	valueIn, err := binarySerializer.Uint64(r, littleEndian)
	if err != nil {
		return err
	}
	ti.ValueIn = int64(valueIn)

	// BlockHeight.
	ti.BlockHeight, err = binarySerializer.Uint32(r, littleEndian)
	if err != nil {
		return err
	}

	// BlockIndex.
	ti.BlockIndex, err = binarySerializer.Uint32(r, littleEndian)
	if err != nil {
		return err
	}

	// Signature script.
	ti.SignatureScript, err = ReadVarBytes(r, pver, MaxMessagePayload,
		"transaction input signature script")
	if err != nil {
		return err
	}

	return nil
}

// readTxInWitnessSigning reads a TxIn witness for signing.
func readTxInWitnessSigning(r io.Reader, pver uint32, version int32,
	ti *TxIn) error {
	var err error

	// Signature script.
	ti.SignatureScript, err = ReadVarBytes(r, pver, MaxMessagePayload,
		"transaction input signature script")
	if err != nil {
		return err
	}

	return nil
}

// readTxInWitnessValueSigning reads a TxIn witness for signing with value
// included.
func readTxInWitnessValueSigning(r io.Reader, pver uint32, version int32,
	ti *TxIn) error {
	// ValueIn.
	valueIn, err := binarySerializer.Uint64(r, littleEndian)
	if err != nil {
		return err
	}
	ti.ValueIn = int64(valueIn)

	// Signature script.
	ti.SignatureScript, err = ReadVarBytes(r, pver, MaxMessagePayload,
		"transaction input signature script")
	if err != nil {
		return err
	}

	return nil
}

// readTxInPrefix reads the next sequence of bytes from r as a transaction input
// (TxIn) in the transaction prefix.
func legacyReadTxIn(r io.Reader, pver uint32, version int32, ti *TxIn) error {
	var op OutPoint
	err := legacyReadOutPoint(r, pver, version, &op)
	if err != nil {
		return err
	}
	ti.PreviousOutPoint = op

	ti.SignatureScript, err = ReadVarBytes(r, pver, MaxMessagePayload,
		"transaction input signature script")
	if err != nil {
		return err
	}

	ti.Sequence, err = binarySerializer.Uint32(r, littleEndian)
	if err != nil {
		return err
	}

	return nil
}

// legacyWriteTxIn encodes ti to the decred protocol encoding for a transaction
// input (TxIn) to w for decred legacy format.
func legacyWriteTxIn(w io.Writer, pver uint32, version int32, ti *TxIn) error {
	err := legacyWriteOutPoint(w, pver, version, &ti.PreviousOutPoint)
	if err != nil {
		return err
	}

	err = WriteVarBytes(w, pver, ti.SignatureScript)
	if err != nil {
		return err
	}

	err = binarySerializer.PutUint32(w, littleEndian, ti.Sequence)
	if err != nil {
		return err
	}

	return nil
}

// writeTxInPrefixs encodes ti to the decred protocol encoding for a transaction
// input (TxIn) prefix to w.
func writeTxInPrefix(w io.Writer, pver uint32, version int32, ti *TxIn) error {
	err := WriteOutPoint(w, pver, version, &ti.PreviousOutPoint)
	if err != nil {
		return err
	}

	err = binarySerializer.PutUint32(w, littleEndian, ti.Sequence)
	if err != nil {
		return err
	}

	return nil
}

// writeTxWitness encodes ti to the decred protocol encoding for a transaction
// input (TxIn) witness to w.
func writeTxInWitness(w io.Writer, pver uint32, version int32, ti *TxIn) error {
	// ValueIn.
	err := binarySerializer.PutUint64(w, littleEndian, uint64(ti.ValueIn))
	if err != nil {
		return err
	}

	// BlockHeight.
	err = binarySerializer.PutUint32(w, littleEndian, ti.BlockHeight)
	if err != nil {
		return err
	}

	// BlockIndex.
	binarySerializer.PutUint32(w, littleEndian, ti.BlockIndex)
	if err != nil {
		return err
	}

	// Write the signature script.
	err = WriteVarBytes(w, pver, ti.SignatureScript)
	if err != nil {
		return err
	}

	return nil
}

// writeTxInWitnessSigning encodes ti to the decred protocol encoding for a
// transaction input (TxIn) witness to w for signing.
func writeTxInWitnessSigning(w io.Writer, pver uint32, version int32,
	ti *TxIn) error {
	var err error

	// Only write the signature script.
	err = WriteVarBytes(w, pver, ti.SignatureScript)
	if err != nil {
		return err
	}

	return nil
}

// writeTxInWitnessValueSigning encodes ti to the decred protocol encoding for a
// transaction input (TxIn) witness to w for signing with value included.
func writeTxInWitnessValueSigning(w io.Writer, pver uint32, version int32,
	ti *TxIn) error {
	var err error

	// ValueIn.
	err = binarySerializer.PutUint64(w, littleEndian, uint64(ti.ValueIn))
	if err != nil {
		return err
	}

	// Signature script.
	err = WriteVarBytes(w, pver, ti.SignatureScript)
	if err != nil {
		return err
	}

	return nil
}

// readTxOut reads the next sequence of bytes from r as a transaction output
// (TxOut).
func readTxOut(r io.Reader, pver uint32, version int32, to *TxOut) error {
	value, err := binarySerializer.Uint64(r, littleEndian)
	if err != nil {
		return err
	}
	to.Value = int64(value)

	to.Version, err = binarySerializer.Uint16(r, littleEndian)
	if err != nil {
		return err
	}

	to.PkScript, err = ReadVarBytes(r, pver, MaxMessagePayload,
		"transaction output public key script")
	if err != nil {
		return err
	}

	return nil
}

// writeTxOut encodes to into the decred protocol encoding for a transaction
// output (TxOut) to w.
func writeTxOut(w io.Writer, pver uint32, version int32, to *TxOut) error {
	err := binarySerializer.PutUint64(w, littleEndian, uint64(to.Value))
	if err != nil {
		return err
	}

	err = binarySerializer.PutUint16(w, littleEndian, to.Version)
	if err != nil {
		return err
	}

	err = WriteVarBytes(w, pver, to.PkScript)
	if err != nil {
		return err
	}
	return nil
}

// legacyReadTxOut reads the next sequence of bytes from r as a transaction output
// (TxOut) in legacy Decred format (for tests).
func legacyReadTxOut(r io.Reader, pver uint32, version int32, to *TxOut) error {
	value, err := binarySerializer.Uint64(r, littleEndian)
	if err != nil {
		return err
	}
	to.Value = int64(value)

	to.PkScript, err = ReadVarBytes(r, pver, MaxMessagePayload,
		"transaction output public key script")
	if err != nil {
		return err
	}

	return nil
}

// legacyWriteTxOut encodes to into the decred protocol encoding for a transaction
// output (TxOut) to w in legacy Decred format (for tests).
func legacyWriteTxOut(w io.Writer, pver uint32, version int32, to *TxOut) error {
	err := binarySerializer.PutUint64(w, littleEndian, uint64(to.Value))
	if err != nil {
		return err
	}

	err = WriteVarBytes(w, pver, to.PkScript)
	if err != nil {
		return err
	}
	return nil
}

// IsSupportedMsgTxVersion returns if a transaction version is supported or not.
// Currently, inclusion into the memory pool (and thus blocks) only supports
// the DefaultMsgTxVersion.
func IsSupportedMsgTxVersion(msgTx *MsgTx) bool {
	if msgTx.Version == DefaultMsgTxVersion() {
		return true
	}
	return false
}
