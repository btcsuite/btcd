// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire

import (
	"fmt"
	"io"
)

// maxFlagsPerMerkleBlock is the maximum number of flag bytes that could
// possibly fit into a merkle block.  Since each transaction is represented by
// a single bit, this is the max number of transactions per block divided by
// 8 bits per byte.  Then an extra one to cover partials.
const maxFlagsPerMerkleBlock = maxTxPerBlock / 8

// MsgMerkleBlock implements the Message interface and represents a bitcoin
// merkleblock message which is used to reset a Bloom filter.
//
// This message was not added until protocol version BIP0037Version.
type MsgMerkleBlock struct {
	Header       BlockHeader
	Transactions uint32
	Hashes       []*ShaHash
	Flags        []byte
}

// AddTxHash adds a new transaction hash to the message.
func (msg *MsgMerkleBlock) AddTxHash(hash *ShaHash) error {
	if len(msg.Hashes)+1 > maxTxPerBlock {
		str := fmt.Sprintf("too many tx hashes for message [max %v]",
			maxTxPerBlock)
		return messageError("MsgMerkleBlock.AddTxHash", str)
	}

	msg.Hashes = append(msg.Hashes, hash)
	return nil
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgMerkleBlock) BtcDecode(r io.Reader, pver uint32) error {
	if pver < BIP0037Version {
		str := fmt.Sprintf("merkleblock message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgMerkleBlock.BtcDecode", str)
	}

	err := readBlockHeader(r, pver, &msg.Header)
	if err != nil {
		return err
	}

	err = readElement(r, &msg.Transactions)
	if err != nil {
		return err
	}

	// Read num block locator hashes and limit to max.
	count, err := readVarInt(r, pver)
	if err != nil {
		return err
	}
	if count > maxTxPerBlock {
		str := fmt.Sprintf("too many transaction hashes for message "+
			"[count %v, max %v]", count, maxTxPerBlock)
		return messageError("MsgMerkleBlock.BtcDecode", str)
	}

	msg.Hashes = make([]*ShaHash, 0, count)
	for i := uint64(0); i < count; i++ {
		var sha ShaHash
		err := readElement(r, &sha)
		if err != nil {
			return err
		}
		msg.AddTxHash(&sha)
	}

	msg.Flags, err = readVarBytes(r, pver, maxFlagsPerMerkleBlock,
		"merkle block flags size")
	if err != nil {
		return err
	}

	return nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgMerkleBlock) BtcEncode(w io.Writer, pver uint32) error {
	if pver < BIP0037Version {
		str := fmt.Sprintf("merkleblock message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgMerkleBlock.BtcEncode", str)
	}

	// Read num transaction hashes and limit to max.
	numHashes := len(msg.Hashes)
	if numHashes > maxTxPerBlock {
		str := fmt.Sprintf("too many transaction hashes for message "+
			"[count %v, max %v]", numHashes, maxTxPerBlock)
		return messageError("MsgMerkleBlock.BtcDecode", str)
	}
	numFlagBytes := len(msg.Flags)
	if numFlagBytes > maxFlagsPerMerkleBlock {
		str := fmt.Sprintf("too many flag bytes for message [count %v, "+
			"max %v]", numFlagBytes, maxFlagsPerMerkleBlock)
		return messageError("MsgMerkleBlock.BtcDecode", str)
	}

	err := writeBlockHeader(w, pver, &msg.Header)
	if err != nil {
		return err
	}

	err = writeElement(w, msg.Transactions)
	if err != nil {
		return err
	}

	err = writeVarInt(w, pver, uint64(numHashes))
	if err != nil {
		return err
	}
	for _, hash := range msg.Hashes {
		err = writeElement(w, hash)
		if err != nil {
			return err
		}
	}

	err = writeVarInt(w, pver, uint64(numFlagBytes))
	if err != nil {
		return err
	}
	err = writeElement(w, msg.Flags)
	if err != nil {
		return err
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgMerkleBlock) Command() string {
	return CmdMerkleBlock
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgMerkleBlock) MaxPayloadLength(pver uint32) uint32 {
	return MaxBlockPayload
}

// NewMsgMerkleBlock returns a new bitcoin merkleblock message that conforms to
// the Message interface.  See MsgMerkleBlock for details.
func NewMsgMerkleBlock(bh *BlockHeader) *MsgMerkleBlock {
	return &MsgMerkleBlock{
		Header:       *bh,
		Transactions: 0,
		Hashes:       make([]*ShaHash, 0),
		Flags:        make([]byte, 0),
	}
}
