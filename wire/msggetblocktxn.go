// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/btcsuite/btcd/chainhash/v2"
)

// MsgGetBlockTxn implements the Message interface and represents a bitcoin
// getblocktxn message.  It requests transactions missing from a compact block
// reconstruction by absolute transaction index.  Indexes are encoded as
// uint32 values, callers serving the request must validate them against the
// requested block.
type MsgGetBlockTxn struct {
	BlockHash chainhash.Hash
	Indexes   []uint32
}

// AddIndex adds a transaction index to the message.
func (msg *MsgGetBlockTxn) AddIndex(index uint32) error {
	if len(msg.Indexes)+1 > maxTxPerBlock {
		str := fmt.Sprintf("too many indexes in message [max %v]",
			maxTxPerBlock)
		return messageError("MsgGetBlockTxn.AddIndex", str)
	}

	msg.Indexes = append(msg.Indexes, index)
	return nil
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgGetBlockTxn) BtcDecode(r io.Reader, pver uint32,
	enc MessageEncoding) error {
	if pver < ShortIdsBlocksVersion {
		str := fmt.Sprintf("getblocktxn message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgGetBlockTxn.BtcDecode", str)
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
		str := fmt.Sprintf("too many indexes for message "+
			"[count %v, max %v]", count, maxTxPerBlock)
		return messageError("MsgGetBlockTxn.BtcDecode", str)
	}

	indexCapacity := min(count, defaultTransactionAlloc)
	msg.Indexes = make([]uint32, 0, indexCapacity)
	var lastIndex uint64
	hasLastIndex := false
	for i := uint64(0); i < count; i++ {
		index, err := readDifferentialIndex(r, pver, buf, lastIndex,
			hasLastIndex)
		if err != nil {
			return err
		}
		lastIndex = index
		hasLastIndex = true
		msg.Indexes = append(msg.Indexes, uint32(index))
	}

	return nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetBlockTxn) BtcEncode(w io.Writer, pver uint32,
	enc MessageEncoding) error {
	if pver < ShortIdsBlocksVersion {
		str := fmt.Sprintf("getblocktxn message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgGetBlockTxn.BtcEncode", str)
	}

	if len(msg.Indexes) > maxTxPerBlock {
		str := fmt.Sprintf("too many indexes for message "+
			"[count %v, max %v]", len(msg.Indexes), maxTxPerBlock)
		return messageError("MsgGetBlockTxn.BtcEncode", str)
	}

	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	if _, err := w.Write(msg.BlockHash[:]); err != nil {
		return err
	}
	if err := WriteVarIntBuf(w, pver, uint64(len(msg.Indexes)), buf); err != nil {
		return err
	}

	var lastIndex uint64
	hasLastIndex := false
	for _, index := range msg.Indexes {
		nextIndex := uint64(index)
		if err := writeDifferentialIndex(w, pver, buf, nextIndex, lastIndex,
			hasLastIndex); err != nil {
			return err
		}
		lastIndex = nextIndex
		hasLastIndex = true
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetBlockTxn) Command() string {
	return CmdGetBlockTxn
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetBlockTxn) MaxPayloadLength(pver uint32) uint32 {
	return chainhash.HashSize + MaxVarIntPayload +
		(maxTxPerBlock * MaxVarIntPayload)
}

// NewMsgGetBlockTxn returns a new bitcoin getblocktxn message that conforms
// to the Message interface.  See MsgGetBlockTxn for details.
func NewMsgGetBlockTxn(blockHash *chainhash.Hash) *MsgGetBlockTxn {
	return &MsgGetBlockTxn{
		BlockHash: *blockHash,
		Indexes:   make([]uint32, 0, defaultTransactionAlloc),
	}
}
