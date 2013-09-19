// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire

import (
	"bytes"
	"io"
)

// TxVersion is the current latest supported transaction version.
const TxVersion = 1

// MaxTxInSequenceNum is the maximum sequence number the sequence field
// of a transaction input can be.
const MaxTxInSequenceNum uint32 = 0xffffffff

// OutPoint defines a bitcoin data type that is used to track previous
// transaction outputs.
type OutPoint struct {
	Hash  ShaHash
	Index uint32
}

// NewOutPoint returns a new bitcoin transaction outpoint point with the
// provided hash and index.
func NewOutPoint(hash *ShaHash, index uint32) *OutPoint {
	return &OutPoint{
		Hash:  *hash,
		Index: index,
	}
}

// TxIn defines a bitcoin transaction input.
type TxIn struct {
	PreviousOutpoint OutPoint
	SignatureScript  []byte
	Sequence         uint32
}

// NewTxIn returns a new bitcoin transaction input with the provided
// previous outpoint point and signature script with a default sequence of
// MaxTxInSequenceNum.
func NewTxIn(prevOut *OutPoint, signatureScript []byte) *TxIn {
	return &TxIn{
		PreviousOutpoint: *prevOut,
		SignatureScript:  signatureScript,
		Sequence:         MaxTxInSequenceNum,
	}
}

// TxOut defines a bitcoin transaction output.
type TxOut struct {
	Value    int64
	PkScript []byte
}

// NewTxOut returns a new bitcoin transaction output with the provided
// transaction value and public key script.
func NewTxOut(value int64, pkScript []byte) *TxOut {
	return &TxOut{
		Value:    value,
		PkScript: pkScript,
	}
}

// MsgTx implements the Message interface and represents a bitcoin tx message.
// It is used to deliver transaction information in response to a getdata
// message (MsgGetData) for a given transaction.
//
// Use the AddTxIn and AddTxOut functions to build up the list of transaction
// inputs and outputs.
type MsgTx struct {
	Version  uint32
	TxIn     []*TxIn
	TxOut    []*TxOut
	LockTime uint32
}

// AddTxIn adds a transaction input to the message.
func (msg *MsgTx) AddTxIn(ti *TxIn) {
	msg.TxIn = append(msg.TxIn, ti)
}

// AddTxOut adds a transaction output to the message.
func (msg *MsgTx) AddTxOut(to *TxOut) {
	msg.TxOut = append(msg.TxOut, to)
}

// TxSha generates the ShaHash name for the transaction.
func (tx *MsgTx) TxSha() (ShaHash, error) {
	// Encode the transaction and calculate double sha256 on the result.
	// Ignore the error returns since the only way the encode could fail
	// is being out of memory or due to nil pointers, both of which would
	// cause a run-time panic.  Also, SetBytes can't fail here due to the
	// fact DoubleSha256 always returns a []byte of the right size
	// regardless of input.
	var buf bytes.Buffer
	var sha ShaHash
	_ = tx.Serialize(&buf)
	_ = sha.SetBytes(DoubleSha256(buf.Bytes()))

	// Even though this function can't currently fail, it still returns
	// a potential error to help future proof the API should a failure
	// become possible.
	return sha, nil
}

// Copy creates a deep copy of a transaction so that the original does not get
// modified when the copy is manipulated.
func (tx *MsgTx) Copy() *MsgTx {
	// Create new tx and start by copying primitive values.
	newTx := MsgTx{
		Version:  tx.Version,
		LockTime: tx.LockTime,
	}

	// Deep copy the old TxIn data.
	for _, oldTxIn := range tx.TxIn {
		// Deep copy the old previous outpoint.
		oldOutPoint := oldTxIn.PreviousOutpoint
		newOutPoint := OutPoint{}
		newOutPoint.Hash.SetBytes(oldOutPoint.Hash[:])
		newOutPoint.Index = oldOutPoint.Index

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
			PreviousOutpoint: newOutPoint,
			SignatureScript:  newScript,
			Sequence:         oldTxIn.Sequence,
		}
		newTx.TxIn = append(newTx.TxIn, &newTxIn)
	}

	// Deep copy the old TxOut data.
	for _, oldTxOut := range tx.TxOut {
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
			PkScript: newScript,
		}
		newTx.TxOut = append(newTx.TxOut, &newTxOut)
	}

	return &newTx
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
// See Deserialize for decoding transactions stored to disk, such as in a
// database, as opposed to decoding transactions from the wire.
func (msg *MsgTx) BtcDecode(r io.Reader, pver uint32) error {
	err := readElement(r, &msg.Version)
	if err != nil {
		return err
	}

	count, err := readVarInt(r, pver)
	if err != nil {
		return err
	}

	for i := uint64(0); i < count; i++ {
		ti := TxIn{}
		err = readTxIn(r, pver, msg.Version, &ti)
		if err != nil {
			return err
		}
		msg.TxIn = append(msg.TxIn, &ti)
	}

	count, err = readVarInt(r, pver)
	if err != nil {
		return err
	}

	for i := uint64(0); i < count; i++ {
		to := TxOut{}
		err = readTxOut(r, pver, msg.Version, &to)
		if err != nil {
			return err
		}
		msg.TxOut = append(msg.TxOut, &to)
	}

	err = readElement(r, &msg.LockTime)
	if err != nil {
		return err
	}

	return nil
}

// Deserialize decodes a transaction from r into the receiver using a format
// that is suitable for long-term storage such as a database while respecting
// the Version field in the transaction.  This function differs from BtcDecode
// in that BtcDecode decodes from the bitcoin wire protocol as it was sent
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

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
// See Serialize for encoding transactions to be stored to disk, such as in a
// database, as opposed to encoding transactions for the wire.
func (msg *MsgTx) BtcEncode(w io.Writer, pver uint32) error {
	err := writeElement(w, msg.Version)
	if err != nil {
		return err
	}

	count := uint64(len(msg.TxIn))
	err = writeVarInt(w, pver, count)
	if err != nil {
		return err
	}

	for _, ti := range msg.TxIn {
		err = writeTxIn(w, pver, msg.Version, ti)
		if err != nil {
			return err
		}
	}

	count = uint64(len(msg.TxOut))
	err = writeVarInt(w, pver, count)
	if err != nil {
		return err
	}

	for _, to := range msg.TxOut {
		err = writeTxOut(w, pver, to)
		if err != nil {
			return err
		}
	}

	err = writeElement(w, msg.LockTime)
	if err != nil {
		return err
	}

	return nil
}

// Serialize encodes the transaction to w using a format that suitable for
// long-term storage such as a database while respecting the Version field in
// the transaction.  This function differs from BtcEncode in that BtcEncode
// encodes the transaction to the bitcoin wire protocol in order to be sent
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

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgTx) Command() string {
	return cmdTx
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgTx) MaxPayloadLength(pver uint32) uint32 {
	return MaxBlockPayload
}

// NewMsgTx returns a new bitcoin tx message that conforms to the Message
// interface.  The return instance has a default version of TxVersion and there
// are no transaction inputs or outputs.  Also, the lock time is set to zero
// to indicate the transaction is valid immediately as opposed to some time in
// future.
func NewMsgTx() *MsgTx {
	return &MsgTx{Version: TxVersion}
}

// readOutPoint reads the next sequence of bytes from r as an OutPoint.
func readOutPoint(r io.Reader, pver uint32, version uint32, op *OutPoint) error {
	err := readElements(r, &op.Hash, &op.Index)
	if err != nil {
		return err
	}
	return nil
}

// writeOutPoint encodes op to the bitcoin protocol encoding for an OutPoint
// to w.
func writeOutPoint(w io.Writer, pver uint32, version uint32, op *OutPoint) error {
	err := writeElements(w, op.Hash, op.Index)
	if err != nil {
		return err
	}
	return nil
}

// readTxIn reads the next sequence of bytes from r as a transaction input
// (TxIn).
func readTxIn(r io.Reader, pver uint32, version uint32, ti *TxIn) error {
	op := OutPoint{}
	err := readOutPoint(r, pver, version, &op)
	if err != nil {
		return err
	}
	ti.PreviousOutpoint = op

	count, err := readVarInt(r, pver)
	if err != nil {
		return err
	}

	b := make([]byte, count)
	err = readElement(r, b)
	if err != nil {
		return err
	}
	ti.SignatureScript = b

	err = readElement(r, &ti.Sequence)
	if err != nil {
		return err
	}

	return nil
}

// writeTxIn encodes ti to the bitcoin protocol encoding for a transaction
// input (TxIn) to w.
func writeTxIn(w io.Writer, pver uint32, version uint32, ti *TxIn) error {
	err := writeOutPoint(w, pver, version, &ti.PreviousOutpoint)
	if err != nil {
		return err
	}

	slen := uint64(len(ti.SignatureScript))
	err = writeVarInt(w, pver, slen)
	if err != nil {
		return err
	}

	b := []byte(ti.SignatureScript)
	_, err = w.Write(b)
	if err != nil {
		return err
	}

	err = writeElement(w, &ti.Sequence)
	if err != nil {
		return err
	}

	return nil
}

// readTxOut reads the next sequence of bytes from r as a transaction output
// (TxOut).
func readTxOut(r io.Reader, pver uint32, version uint32, to *TxOut) error {
	err := readElement(r, &to.Value)
	if err != nil {
		return err
	}

	slen, err := readVarInt(r, pver)
	if err != nil {
		return err
	}

	b := make([]byte, slen)
	err = readElement(r, b)
	if err != nil {
		return err
	}
	to.PkScript = b

	return nil
}

// writeTxOut encodes to into the bitcoin protocol encoding for a transaction
// output (TxOut) to w.
func writeTxOut(w io.Writer, pver uint32, to *TxOut) error {
	err := writeElement(w, to.Value)
	if err != nil {
		return err
	}

	pkLen := uint64(len(to.PkScript))
	err = writeVarInt(w, pver, pkLen)
	if err != nil {
		return err
	}

	err = writeElement(w, to.PkScript)
	if err != nil {
		return err
	}

	return nil
}
