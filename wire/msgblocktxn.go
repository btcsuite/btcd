// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/btcsuite/btcd/chainhash/v2"
)

// MsgBlockTxn implements the Message interface and represents a bitcoin
// blocktxn message.  It provides transactions requested by a getblocktxn
// message.
type MsgBlockTxn struct {
	BlockHash    chainhash.Hash
	Transactions []*MsgTx
}

// AddTransaction adds a transaction to the message.
func (msg *MsgBlockTxn) AddTransaction(tx *MsgTx) error {
	if len(msg.Transactions)+1 > maxTxPerBlock {
		str := fmt.Sprintf("too many transactions in message [max %v]",
			maxTxPerBlock)
		return messageError("MsgBlockTxn.AddTransaction", str)
	}

	msg.Transactions = append(msg.Transactions, tx)
	return nil
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgBlockTxn) BtcDecode(r io.Reader, pver uint32,
	enc MessageEncoding) error {
	if pver < ShortIdsBlocksVersion {
		str := fmt.Sprintf("blocktxn message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgBlockTxn.BtcDecode", str)
	}

	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	if _, err := io.ReadFull(r, msg.BlockHash[:]); err != nil {
		return err
	}

	count, err := ReadVarIntBuf(r, pver, buf)
	if err != nil {
		return err
	}
	if count > maxTxPerBlock {
		str := fmt.Sprintf("too many transactions for message "+
			"[count %v, max %v]", count, maxTxPerBlock)
		return messageError("MsgBlockTxn.BtcDecode", str)
	}

	scriptBuf := scriptPool.Borrow()
	defer scriptPool.Return(scriptBuf)

	txCapacity := min(count, defaultTransactionAlloc)
	msg.Transactions = make([]*MsgTx, 0, txCapacity)
	for i := uint64(0); i < count; i++ {
		tx := MsgTx{}
		if err := tx.btcDecode(r, pver, enc, buf, scriptBuf[:]); err != nil {
			return err
		}
		msg.Transactions = append(msg.Transactions, &tx)
	}

	return nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgBlockTxn) BtcEncode(w io.Writer, pver uint32,
	enc MessageEncoding) error {
	if pver < ShortIdsBlocksVersion {
		str := fmt.Sprintf("blocktxn message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgBlockTxn.BtcEncode", str)
	}

	if len(msg.Transactions) > maxTxPerBlock {
		str := fmt.Sprintf("too many transactions for message "+
			"[count %v, max %v]", len(msg.Transactions), maxTxPerBlock)
		return messageError("MsgBlockTxn.BtcEncode", str)
	}

	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	if _, err := w.Write(msg.BlockHash[:]); err != nil {
		return err
	}
	if err := WriteVarIntBuf(w, pver, uint64(len(msg.Transactions)),
		buf); err != nil {
		return err
	}

	for _, tx := range msg.Transactions {
		if tx == nil {
			return messageError("MsgBlockTxn.BtcEncode", "nil transaction")
		}

		if err := tx.btcEncode(w, pver, enc, buf); err != nil {
			return err
		}
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgBlockTxn) Command() string {
	return CmdBlockTxn
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgBlockTxn) MaxPayloadLength(pver uint32) uint32 {
	return chainhash.HashSize + MaxVarIntPayload + MaxBlockPayload
}

// NewMsgBlockTxn returns a new bitcoin blocktxn message that conforms to the
// Message interface.  See MsgBlockTxn for details.
func NewMsgBlockTxn(blockHash *chainhash.Hash) *MsgBlockTxn {
	return &MsgBlockTxn{
		BlockHash:    *blockHash,
		Transactions: make([]*MsgTx, 0, defaultTransactionAlloc),
	}
}
