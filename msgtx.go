// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire

import (
	"bytes"
	"io"
)

// MaxTxInSequenceNum is the maximum sequence number the sequence field
// of a transaction input can be.
const MaxTxInSequenceNum uint32 = 0xffffffff

// Outpoint defines a bitcoin data type that is used to track previous
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
func (tx *MsgTx) TxSha(pver uint32) (ShaHash, error) {
	var txsha ShaHash
	var wbuf bytes.Buffer
	err := tx.BtcEncode(&wbuf, pver)
	if err != nil {
		return txsha, err
	}
	txsha.SetBytes(DoubleSha256(wbuf.Bytes()))

	return txsha, nil
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

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
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

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgTx) Command() string {
	return cmdTx
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgTx) MaxPayloadLength(pver uint32) uint32 {
	return maxMessagePayload
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
