// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"fmt"
	"io"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// defaultTransactionAlloc is the default size used for the backing array
// for transactions.  The transaction array will dynamically grow as needed, but
// this figure is intended to provide enough space for the number of
// transactions in the vast majority of blocks without needing to grow the
// backing array multiple times.
const defaultTransactionAlloc = 2048

// MaxBlocksPerMsg is the maximum number of blocks allowed per message.
const MaxBlocksPerMsg = 500

// MaxBlockPayloadV3 is the maximum bytes a block message can be in bytes as of
// version 3 of the protocol.
const MaxBlockPayloadV3 = 1000000 // Not actually 1MB which would be 1024 * 1024

// MaxBlockPayload is the maximum bytes a block message can be in bytes.
const MaxBlockPayload = 1310720 // 1.25MB

// MaxTxPerTxTree returns the maximum number of transactions that could possibly
// fit into a block per each merkle root for the given protocol version.
func MaxTxPerTxTree(pver uint32) uint64 {
	if pver <= 3 {
		return ((MaxBlockPayloadV3 / minTxPayload) / 2) + 1
	}

	return ((MaxBlockPayload / minTxPayload) / 2) + 1
}

// TxLoc holds locator data for the offset and length of where a transaction is
// located within a MsgBlock data buffer.
type TxLoc struct {
	TxStart int
	TxLen   int
}

// MsgBlock implements the Message interface and represents a decred
// block message.  It is used to deliver block and transaction information in
// response to a getdata message (MsgGetData) for a given block hash.
type MsgBlock struct {
	Header        BlockHeader
	Transactions  []*MsgTx
	STransactions []*MsgTx
}

// AddTransaction adds a transaction to the message.
func (msg *MsgBlock) AddTransaction(tx *MsgTx) error {
	msg.Transactions = append(msg.Transactions, tx)
	return nil

}

// AddSTransaction adds a stake transaction to the message.
func (msg *MsgBlock) AddSTransaction(tx *MsgTx) error {
	msg.STransactions = append(msg.STransactions, tx)
	return nil
}

// ClearTransactions removes all transactions from the message.
func (msg *MsgBlock) ClearTransactions() {
	msg.Transactions = make([]*MsgTx, 0, defaultTransactionAlloc)
}

// ClearSTransactions removes all stake transactions from the message.
func (msg *MsgBlock) ClearSTransactions() {
	msg.STransactions = make([]*MsgTx, 0, defaultTransactionAlloc)
}

// BtcDecode decodes r using the decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
// See Deserialize for decoding blocks stored to disk, such as in a database, as
// opposed to decoding blocks from the wire.
func (msg *MsgBlock) BtcDecode(r io.Reader, pver uint32) error {
	err := readBlockHeader(r, pver, &msg.Header)
	if err != nil {
		return err
	}

	txCount, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}

	// Prevent more transactions than could possibly fit into the regular
	// tx tree.
	// It would be possible to cause memory exhaustion and panics without
	// a sane upper bound on this count.
	maxTxPerTree := MaxTxPerTxTree(pver)
	if txCount > maxTxPerTree {
		str := fmt.Sprintf("too many transactions to fit into a block "+
			"[count %d, max %d]", txCount, maxTxPerTree)
		return messageError("MsgBlock.BtcDecode", str)
	}

	msg.Transactions = make([]*MsgTx, 0, txCount)
	for i := uint64(0); i < txCount; i++ {
		var tx MsgTx
		err := tx.BtcDecode(r, pver)
		if err != nil {
			return err
		}
		msg.Transactions = append(msg.Transactions, &tx)
	}

	// Prevent more transactions than could possibly fit into the stake
	// tx tree.
	// It would be possible to cause memory exhaustion and panics without
	// a sane upper bound on this count.
	stakeTxCount, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if stakeTxCount > maxTxPerTree {
		str := fmt.Sprintf("too many stransactions to fit into a block "+
			"[count %d, max %d]", stakeTxCount, maxTxPerTree)
		return messageError("MsgBlock.BtcDecode", str)
	}

	msg.STransactions = make([]*MsgTx, 0, stakeTxCount)
	for i := uint64(0); i < stakeTxCount; i++ {
		var tx MsgTx
		err := tx.BtcDecode(r, pver)
		if err != nil {
			return err
		}
		msg.STransactions = append(msg.STransactions, &tx)
	}

	return nil
}

// Deserialize decodes a block from r into the receiver using a format that is
// suitable for long-term storage such as a database while respecting the
// Version field in the block.  This function differs from BtcDecode in that
// BtcDecode decodes from the decred wire protocol as it was sent across the
// network.  The wire encoding can technically differ depending on the protocol
// version and doesn't even really need to match the format of a stored block at
// all.  As of the time this comment was written, the encoded block is the same
// in both instances, but there is a distinct difference and separating the two
// allows the API to be flexible enough to deal with changes.
func (msg *MsgBlock) Deserialize(r io.Reader) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol version 0 and the stable long-term storage format.  As
	// a result, make use of BtcDecode.
	return msg.BtcDecode(r, 0)
}

// FromBytes deserializes a transaction byte slice.
func (msg *MsgBlock) FromBytes(b []byte) error {
	r := bytes.NewReader(b)
	return msg.Deserialize(r)
}

// DeserializeTxLoc decodes r in the same manner Deserialize does, but it takes
// a byte buffer instead of a generic reader and returns a slice containing the
// start and length of each transaction within the raw data that is being
// deserialized.
func (msg *MsgBlock) DeserializeTxLoc(r *bytes.Buffer) ([]TxLoc, []TxLoc, error) {
	fullLen := r.Len()

	// At the current time, there is no difference between the wire encoding
	// at protocol version 0 and the stable long-term storage format.  As
	// a result, make use of existing wire protocol functions.
	err := readBlockHeader(r, 0, &msg.Header)
	if err != nil {
		return nil, nil, err
	}

	txCount, err := ReadVarInt(r, 0)
	if err != nil {
		return nil, nil, err
	}
	// Prevent more transactions than could possibly fit into a normal tx
	// tree.  It would be possible to cause memory exhaustion and panics
	// without a sane upper bound on this count.
	//
	// NOTE: This is using the constant for the latest protocol version
	// since it is in terms of the largest possible block size.
	maxTxPerTree := MaxTxPerTxTree(ProtocolVersion)
	if txCount > maxTxPerTree {
		str := fmt.Sprintf("too many transactions to fit into a block "+
			"[count %d, max %d]", txCount, maxTxPerTree)
		return nil, nil, messageError("MsgBlock.DeserializeTxLoc", str)
	}

	// Deserialize each transaction while keeping track of its location
	// within the byte stream.
	msg.Transactions = make([]*MsgTx, 0, txCount)
	txLocs := make([]TxLoc, txCount)
	for i := uint64(0); i < txCount; i++ {
		txLocs[i].TxStart = fullLen - r.Len()
		var tx MsgTx
		err := tx.Deserialize(r)
		if err != nil {
			return nil, nil, err
		}
		msg.Transactions = append(msg.Transactions, &tx)
		txLocs[i].TxLen = (fullLen - r.Len()) - txLocs[i].TxStart
	}

	stakeTxCount, err := ReadVarInt(r, 0)
	if err != nil {
		return nil, nil, err
	}

	// Prevent more transactions than could possibly fit into a stake tx
	// tree.  It would be possible to cause memory exhaustion and panics
	// without a sane upper bound on this count.
	//
	// NOTE: This is using the constant for the latest protocol version
	// since it is in terms of the largest possible block size.
	if stakeTxCount > maxTxPerTree {
		str := fmt.Sprintf("too many transactions to fit into a stake tx tree "+
			"[count %d, max %d]", stakeTxCount, maxTxPerTree)
		return nil, nil, messageError("MsgBlock.DeserializeTxLoc", str)
	}

	// Deserialize each transaction while keeping track of its location
	// within the byte stream.
	msg.STransactions = make([]*MsgTx, 0, stakeTxCount)
	sTxLocs := make([]TxLoc, stakeTxCount)
	for i := uint64(0); i < stakeTxCount; i++ {
		sTxLocs[i].TxStart = fullLen - r.Len()
		var tx MsgTx
		err := tx.Deserialize(r)
		if err != nil {
			return nil, nil, err
		}
		msg.STransactions = append(msg.STransactions, &tx)
		sTxLocs[i].TxLen = (fullLen - r.Len()) - sTxLocs[i].TxStart
	}

	return txLocs, sTxLocs, nil
}

// BtcEncode encodes the receiver to w using the decred protocol encoding.
// This is part of the Message interface implementation.
// See Serialize for encoding blocks to be stored to disk, such as in a
// database, as opposed to encoding blocks for the wire.
func (msg *MsgBlock) BtcEncode(w io.Writer, pver uint32) error {
	err := writeBlockHeader(w, pver, &msg.Header)
	if err != nil {
		return err
	}

	err = WriteVarInt(w, pver, uint64(len(msg.Transactions)))
	if err != nil {
		return err
	}

	for _, tx := range msg.Transactions {
		err = tx.BtcEncode(w, pver)
		if err != nil {
			return err
		}
	}

	err = WriteVarInt(w, pver, uint64(len(msg.STransactions)))
	if err != nil {
		return err
	}

	for _, tx := range msg.STransactions {
		err = tx.BtcEncode(w, pver)
		if err != nil {
			return err
		}
	}

	return nil
}

// Serialize encodes the block to w using a format that suitable for long-term
// storage such as a database while respecting the Version field in the block.
// This function differs from BtcEncode in that BtcEncode encodes the block to
// the decred wire protocol in order to be sent across the network.  The wire
// encoding can technically differ depending on the protocol version and doesn't
// even really need to match the format of a stored block at all.  As of the
// time this comment was written, the encoded block is the same in both
// instances, but there is a distinct difference and separating the two allows
// the API to be flexible enough to deal with changes.
func (msg *MsgBlock) Serialize(w io.Writer) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol version 0 and the stable long-term storage format.  As
	// a result, make use of BtcEncode.
	return msg.BtcEncode(w, 0)
}

// Bytes returns the serialized form of the block in bytes.
func (msg *MsgBlock) Bytes() ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, msg.SerializeSize()))
	err := msg.Serialize(buf)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// SerializeSize returns the number of bytes it would take to serialize the
// the block.
func (msg *MsgBlock) SerializeSize() int {
	// Check to make sure that all transactions have the correct
	// type and version to be included in a block.

	// Block header bytes + Serialized varint size for the number of
	// transactions + Serialized varint size for the number of
	// stake transactions

	n := blockHeaderLen + VarIntSerializeSize(uint64(len(msg.Transactions))) +
		VarIntSerializeSize(uint64(len(msg.STransactions)))

	for _, tx := range msg.Transactions {
		n += tx.SerializeSize()
	}

	for _, tx := range msg.STransactions {
		n += tx.SerializeSize()
	}

	return n
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgBlock) Command() string {
	return CmdBlock
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgBlock) MaxPayloadLength(pver uint32) uint32 {
	// Protocol version 3 and lower have a different max block payload.
	if pver <= 3 {
		return MaxBlockPayloadV3
	}

	// Block header at 80 bytes + transaction count + max transactions
	// which can vary up to the MaxBlockPayload (including the block header
	// and transaction count).
	return MaxBlockPayload
}

// BlockHash computes the block identifier hash for this block.
func (msg *MsgBlock) BlockHash() chainhash.Hash {
	return msg.Header.BlockHash()
}

// TxHashes returns a slice of hashes of all of transactions in this block.
func (msg *MsgBlock) TxHashes() []chainhash.Hash {
	hashList := make([]chainhash.Hash, 0, len(msg.Transactions))
	for _, tx := range msg.Transactions {
		hashList = append(hashList, tx.TxHash())
	}
	return hashList
}

// STxHashes returns a slice of hashes of all of stake transactions in this
// block.
func (msg *MsgBlock) STxHashes() []chainhash.Hash {
	hashList := make([]chainhash.Hash, 0, len(msg.STransactions))
	for _, tx := range msg.STransactions {
		hashList = append(hashList, tx.TxHash())
	}
	return hashList
}

// NewMsgBlock returns a new decred block message that conforms to the
// Message interface.  See MsgBlock for details.
func NewMsgBlock(blockHeader *BlockHeader) *MsgBlock {
	return &MsgBlock{
		Header:        *blockHeader,
		Transactions:  make([]*MsgTx, 0, defaultTransactionAlloc),
		STransactions: make([]*MsgTx, 0, defaultTransactionAlloc),
	}
}
