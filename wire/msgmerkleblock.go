// Copyright (c) 2014-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// maxFlagsPerMerkleBlock is the maximum number of flag bytes that could
// possibly fit into a merkle block of the given protocol version.
func maxFlagsPerMerkleBlock(pver uint32) uint32 {
	// Each transaction is represented by a single bit, so the result is the
	// max number of transactions per block divided by 8 bits per byte.
	// Then an extra one to cover partials.
	return uint32(MaxTxPerTxTree(ProtocolVersion)/8) + 1
}

// MsgMerkleBlock implements the Message interface and represents a decred
// merkleblock message which is used to reset a Bloom filter.
//
// This message was not added until protocol version BIP0037Version.
type MsgMerkleBlock struct {
	Header        BlockHeader
	Transactions  uint32
	Hashes        []*chainhash.Hash
	STransactions uint32
	SHashes       []*chainhash.Hash
	Flags         []byte
}

// AddTxHash adds a new transaction hash to the message.
func (msg *MsgMerkleBlock) AddTxHash(hash *chainhash.Hash) error {
	maxTxPerTree := MaxTxPerTxTree(ProtocolVersion)
	if uint64(len(msg.Hashes)+1) > maxTxPerTree {
		str := fmt.Sprintf("too many tx hashes for message [max %v]",
			maxTxPerTree)
		return messageError("MsgMerkleBlock.AddTxHash", str)
	}

	msg.Hashes = append(msg.Hashes, hash)
	return nil
}

// AddSTxHash adds a new stake transaction hash to the message.
func (msg *MsgMerkleBlock) AddSTxHash(hash *chainhash.Hash) error {
	maxTxPerTree := MaxTxPerTxTree(ProtocolVersion)
	if uint64(len(msg.SHashes)+1) > maxTxPerTree {
		str := fmt.Sprintf("too many tx hashes for message [max %v]",
			maxTxPerTree)
		return messageError("MsgMerkleBlock.AddSTxHash", str)
	}

	msg.SHashes = append(msg.SHashes, hash)
	return nil
}

// BtcDecode decodes r using the decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgMerkleBlock) BtcDecode(r io.Reader, pver uint32) error {
	err := readBlockHeader(r, pver, &msg.Header)
	if err != nil {
		return err
	}

	err = readElement(r, &msg.Transactions)
	if err != nil {
		return err
	}

	// Read num block locator hashes and limit to max.
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	maxTxPerTree := MaxTxPerTxTree(pver)
	if count > maxTxPerTree {
		str := fmt.Sprintf("too many transaction hashes for message "+
			"[count %v, max %v]", count, maxTxPerTree)
		return messageError("MsgMerkleBlock.BtcDecode", str)
	}

	// Create a contiguous slice of hashes to deserialize into in order to
	// reduce the number of allocations.
	hashes := make([]chainhash.Hash, count)
	msg.Hashes = make([]*chainhash.Hash, 0, count)
	for i := uint64(0); i < count; i++ {
		hash := &hashes[i]
		err := readElement(r, hash)
		if err != nil {
			return err
		}
		msg.AddTxHash(hash)
	}

	err = readElement(r, &msg.STransactions)
	if err != nil {
		return err
	}

	// Read num block locator hashes for stake and limit to max.
	scount, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if scount > maxTxPerTree {
		str := fmt.Sprintf("too many stransaction hashes for message "+
			"[count %v, max %v]", scount, maxTxPerTree)
		return messageError("MsgMerkleBlock.BtcDecode", str)
	}

	hashes = make([]chainhash.Hash, count)
	msg.SHashes = make([]*chainhash.Hash, 0, scount)
	for i := uint64(0); i < scount; i++ {
		hash := &hashes[i]
		err := readElement(r, hash)
		if err != nil {
			return err
		}
		msg.AddSTxHash(hash)
	}

	msg.Flags, err = ReadVarBytes(r, pver, maxFlagsPerMerkleBlock(pver),
		"merkle block flags size")
	return err
}

// BtcEncode encodes the receiver to w using the decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgMerkleBlock) BtcEncode(w io.Writer, pver uint32) error {
	// Read num transaction hashes and limit to max.
	numHashes := uint64(len(msg.Hashes))
	maxTxPerTree := MaxTxPerTxTree(pver)
	if numHashes > maxTxPerTree {
		str := fmt.Sprintf("too many transaction hashes for message "+
			"[count %v, max %v]", numHashes, maxTxPerTree)
		return messageError("MsgMerkleBlock.BtcDecode", str)
	}
	// Read num stake transaction hashes and limit to max.
	numSHashes := uint64(len(msg.SHashes))
	if numSHashes > maxTxPerTree {
		str := fmt.Sprintf("too many stake transaction hashes for message "+
			"[count %v, max %v]", numHashes, maxTxPerTree)
		return messageError("MsgMerkleBlock.BtcDecode", str)
	}

	numFlagBytes := uint32(len(msg.Flags))
	maxFlags := maxFlagsPerMerkleBlock(pver)
	if numFlagBytes > maxFlags {
		str := fmt.Sprintf("too many flag bytes for message [count %v, "+
			"max %v]", numFlagBytes, maxFlags)
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

	err = WriteVarInt(w, pver, numHashes)
	if err != nil {
		return err
	}
	for _, hash := range msg.Hashes {
		err = writeElement(w, hash)
		if err != nil {
			return err
		}
	}

	err = writeElement(w, msg.STransactions)
	if err != nil {
		return err
	}

	err = WriteVarInt(w, pver, numSHashes)
	if err != nil {
		return err
	}
	for _, hash := range msg.SHashes {
		err = writeElement(w, hash)
		if err != nil {
			return err
		}
	}

	return WriteVarBytes(w, pver, msg.Flags)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgMerkleBlock) Command() string {
	return CmdMerkleBlock
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgMerkleBlock) MaxPayloadLength(pver uint32) uint32 {
	// Protocol version 3 and lower have a different max block payload.
	if pver <= 3 {
		return MaxBlockPayloadV3
	}

	return MaxBlockPayload
}

// NewMsgMerkleBlock returns a new decred merkleblock message that conforms to
// the Message interface.  See MsgMerkleBlock for details.
func NewMsgMerkleBlock(bh *BlockHeader) *MsgMerkleBlock {
	return &MsgMerkleBlock{
		Header:       *bh,
		Transactions: 0,
		Hashes:       make([]*chainhash.Hash, 0),
		SHashes:      make([]*chainhash.Hash, 0),
		Flags:        make([]byte, 0),
	}
}
