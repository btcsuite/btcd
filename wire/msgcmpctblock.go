// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"
)

const (
	// ShortIDSize is the number of bytes used to encode a compact block
	// transaction short ID.
	ShortIDSize = 6
)

// ShortID identifies a transaction in a compact block.
type ShortID [ShortIDSize]byte

// PrefilledTxn houses a transaction included in full in a compact block along
// with its absolute index in the block.  The index is encoded as a uint32,
// callers reconstructing the block must validate that it is within the block.
type PrefilledTxn struct {
	Index uint32
	Tx    *MsgTx
}

// MsgCmpctBlock implements the Message interface and represents a bitcoin
// cmpctblock message.
type MsgCmpctBlock struct {
	Header        BlockHeader
	Nonce         uint64
	ShortIDs      []ShortID
	PrefilledTxns []*PrefilledTxn
}

// AddShortID adds a transaction short ID to the message.
func (msg *MsgCmpctBlock) AddShortID(shortID ShortID) error {
	if len(msg.ShortIDs)+1 > maxTxPerBlock {
		str := fmt.Sprintf("too many short ids in message [max %v]",
			maxTxPerBlock)
		return messageError("MsgCmpctBlock.AddShortID", str)
	}

	msg.ShortIDs = append(msg.ShortIDs, shortID)
	return nil
}

// AddPrefilledTxn adds a prefilled transaction to the message.
func (msg *MsgCmpctBlock) AddPrefilledTxn(txn *PrefilledTxn) error {
	if len(msg.PrefilledTxns)+1 > maxTxPerBlock {
		str := fmt.Sprintf("too many prefilled transactions in message "+
			"[max %v]", maxTxPerBlock)
		return messageError("MsgCmpctBlock.AddPrefilledTxn", str)
	}

	msg.PrefilledTxns = append(msg.PrefilledTxns, txn)
	return nil
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgCmpctBlock) BtcDecode(r io.Reader, pver uint32,
	enc MessageEncoding) error {
	if pver < ShortIdsBlocksVersion {
		str := fmt.Sprintf("cmpctblock message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgCmpctBlock.BtcDecode", str)
	}

	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	if err := readBlockHeaderBuf(r, pver, &msg.Header, buf); err != nil {
		return err
	}
	if err := readElement(r, &msg.Nonce); err != nil {
		return err
	}

	shortIDCount, err := ReadVarIntBuf(r, pver, buf)
	if err != nil {
		return err
	}
	if shortIDCount > maxTxPerBlock {
		str := fmt.Sprintf("too many short ids for message "+
			"[count %v, max %v]", shortIDCount, maxTxPerBlock)
		return messageError("MsgCmpctBlock.BtcDecode", str)
	}

	shortIDCapacity := min(shortIDCount, defaultTransactionAlloc)
	msg.ShortIDs = make([]ShortID, 0, shortIDCapacity)
	for i := uint64(0); i < shortIDCount; i++ {
		var shortID ShortID
		if _, err := io.ReadFull(r, shortID[:]); err != nil {
			return err
		}
		msg.ShortIDs = append(msg.ShortIDs, shortID)
	}

	prefilledCount, err := ReadVarIntBuf(r, pver, buf)
	if err != nil {
		return err
	}
	if prefilledCount > maxTxPerBlock {
		str := fmt.Sprintf("too many prefilled transactions for "+
			"message [count %v, max %v]", prefilledCount, maxTxPerBlock)
		return messageError("MsgCmpctBlock.BtcDecode", str)
	}

	scriptBuf := scriptPool.Borrow()
	defer scriptPool.Return(scriptBuf)

	prefilledCapacity := min(prefilledCount, uint64(defaultTransactionAlloc))
	msg.PrefilledTxns = make([]*PrefilledTxn, 0, prefilledCapacity)
	var lastIndex uint64
	hasLastIndex := false
	for i := uint64(0); i < prefilledCount; i++ {
		index, err := readDifferentialIndex(r, pver, buf, lastIndex,
			hasLastIndex)
		if err != nil {
			return err
		}
		lastIndex = index
		hasLastIndex = true

		tx := MsgTx{}
		if err := tx.btcDecode(r, pver, enc, buf, scriptBuf[:]); err != nil {
			return err
		}

		err = msg.AddPrefilledTxn(&PrefilledTxn{
			Index: uint32(index),
			Tx:    &tx,
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgCmpctBlock) BtcEncode(w io.Writer, pver uint32,
	enc MessageEncoding) error {
	if pver < ShortIdsBlocksVersion {
		str := fmt.Sprintf("cmpctblock message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgCmpctBlock.BtcEncode", str)
	}

	if len(msg.ShortIDs) > maxTxPerBlock {
		str := fmt.Sprintf("too many short ids for message "+
			"[count %v, max %v]", len(msg.ShortIDs), maxTxPerBlock)
		return messageError("MsgCmpctBlock.BtcEncode", str)
	}
	if len(msg.PrefilledTxns) > maxTxPerBlock {
		str := fmt.Sprintf("too many prefilled transactions for "+
			"message [count %v, max %v]", len(msg.PrefilledTxns),
			maxTxPerBlock)
		return messageError("MsgCmpctBlock.BtcEncode", str)
	}

	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	if err := writeBlockHeaderBuf(w, pver, &msg.Header, buf); err != nil {
		return err
	}
	if err := writeElement(w, msg.Nonce); err != nil {
		return err
	}
	if err := WriteVarIntBuf(w, pver, uint64(len(msg.ShortIDs)), buf); err != nil {
		return err
	}
	for _, shortID := range msg.ShortIDs {
		if _, err := w.Write(shortID[:]); err != nil {
			return err
		}
	}

	if err := WriteVarIntBuf(w, pver, uint64(len(msg.PrefilledTxns)),
		buf); err != nil {
		return err
	}

	var lastIndex uint64
	hasLastIndex := false
	for _, txn := range msg.PrefilledTxns {
		if txn == nil || txn.Tx == nil {
			return messageError("MsgCmpctBlock.BtcEncode",
				"nil prefilled transaction")
		}

		index := uint64(txn.Index)
		if err := writeDifferentialIndex(w, pver, buf, index, lastIndex,
			hasLastIndex); err != nil {
			return err
		}
		lastIndex = index
		hasLastIndex = true

		if err := txn.Tx.btcEncode(w, pver, enc, buf); err != nil {
			return err
		}
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgCmpctBlock) Command() string {
	return CmdCmpctBlock
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgCmpctBlock) MaxPayloadLength(pver uint32) uint32 {
	return MaxBlockHeaderPayload + 8 + MaxVarIntPayload +
		(maxTxPerBlock * ShortIDSize) + MaxVarIntPayload +
		(maxTxPerBlock * MaxVarIntPayload) + MaxBlockPayload
}

// NewMsgCmpctBlock returns a new bitcoin cmpctblock message that conforms to
// the Message interface.  See MsgCmpctBlock for details.
func NewMsgCmpctBlock(header *BlockHeader, nonce uint64) *MsgCmpctBlock {
	return &MsgCmpctBlock{
		Header:        *header,
		Nonce:         nonce,
		ShortIDs:      make([]ShortID, 0, defaultTransactionAlloc),
		PrefilledTxns: make([]*PrefilledTxn, 0, 1),
	}
}
