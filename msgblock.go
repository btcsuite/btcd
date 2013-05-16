// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire

import (
	"bytes"
	"io"
)

// MaxBlocksPerMsg is the maximum number of blocks allowed per message.
const MaxBlocksPerMsg = 500

// maxBlockPayload is the maximum bytes a block message can be.
const maxBlockPayload = (1024 * 1024) // 1MB

// TxLoc holds locator data for the offset and length of where a transaction is
// located within a MsgBlock data buffer.
type TxLoc struct {
	TxStart int
	TxLen   int
}

// MsgBlock implements the Message interface and represents a bitcoin
// block message.  It is used to deliver block and transaction information in
// response to a getdata message (MsgGetData) for a given block hash.
//
// NOTE: Unlike the other message types which contain slices, the number of
// transactions has a specific entry (Header.TxnCount) that must be kept in
// sync.  The AddTransaction and ClearTransactions functions properly sync the
// value, but if you are manually modifying the public members, you will need
// to ensure you update the Header.TxnCount when you add and remove
// transactions.
type MsgBlock struct {
	Header       BlockHeader
	Transactions []*MsgTx
}

// AddTransaction adds a transaction to the message and updates Header.TxnCount
// accordingly.
func (msg *MsgBlock) AddTransaction(tx *MsgTx) error {
	// TODO: Return error if adding the transaction would make the message
	// too large.
	msg.Transactions = append(msg.Transactions, tx)
	msg.Header.TxnCount = uint64(len(msg.Transactions))
	return nil

}

// ClearTransactions removes all transactions from the message and updates
// Header.TxnCount accordingly.
func (msg *MsgBlock) ClearTransactions() {
	msg.Transactions = []*MsgTx{}
	msg.Header.TxnCount = 0
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgBlock) BtcDecode(r io.Reader, pver uint32) error {
	err := readBlockHeader(r, pver, &msg.Header)
	if err != nil {
		return err
	}

	for i := uint64(0); i < msg.Header.TxnCount; i++ {
		tx := MsgTx{}
		err := tx.BtcDecode(r, pver)
		if err != nil {
			return err
		}
		msg.Transactions = append(msg.Transactions, &tx)
	}

	return nil
}

// BtcDecodeTxLoc decodes r using the bitcoin protocol encoding into the
// receiver and returns a slice containing the start and length of each
// transaction within the raw data.
func (msg *MsgBlock) BtcDecodeTxLoc(r *bytes.Buffer, pver uint32) ([]TxLoc, error) {
	var fullLen int
	fullLen = r.Len()

	err := readBlockHeader(r, pver, &msg.Header)
	if err != nil {
		return nil, err
	}

	var txLocs []TxLoc
	txLocs = make([]TxLoc, msg.Header.TxnCount)

	for i := uint64(0); i < msg.Header.TxnCount; i++ {
		txLocs[i].TxStart = fullLen - r.Len()
		tx := MsgTx{}
		err := tx.BtcDecode(r, pver)
		if err != nil {
			return nil, err
		}
		msg.Transactions = append(msg.Transactions, &tx)
		txLocs[i].TxLen = (fullLen - r.Len()) - txLocs[i].TxStart
	}

	return txLocs, nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgBlock) BtcEncode(w io.Writer, pver uint32) error {
	msg.Header.TxnCount = uint64(len(msg.Transactions))

	err := writeBlockHeader(w, pver, &msg.Header)
	if err != nil {
		return err
	}

	for _, tx := range msg.Transactions {
		err = tx.BtcEncode(w, pver)
		if err != nil {
			return err
		}
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgBlock) Command() string {
	return cmdBlock
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgBlock) MaxPayloadLength(pver uint32) uint32 {
	// Block header at 81 bytes + max transactions which can vary up to the
	// maxBlockPayload (including the block header).
	return maxBlockPayload
}

// BlockSha computes the block identifier hash for this block.
func (msg *MsgBlock) BlockSha(pver uint32) (ShaHash, error) {
	return msg.Header.BlockSha(pver)
}

// TxShas returns a slice of hashes of all of transactions in this block.
func (msg *MsgBlock) TxShas(pver uint32) ([]ShaHash, error) {
	var shaList []ShaHash
	for _, tx := range msg.Transactions {
		// Ignore error here since TxSha can't fail in the current
		// implementation except due to run-time panics.
		sha, _ := tx.TxSha(pver)
		shaList = append(shaList, sha)
	}
	return shaList, nil
}

// NewMsgBlock returns a new bitcoin block message that conforms to the
// Message interface.  See MsgBlock for details.
func NewMsgBlock(blockHeader *BlockHeader) *MsgBlock {
	return &MsgBlock{
		Header: *blockHeader,
	}
}
