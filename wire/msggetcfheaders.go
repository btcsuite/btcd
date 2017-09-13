// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// MsgGetCFHeaders is a message similar to MsgGetHeaders, but for committed
// filter headers. It allows to set the FilterType field to get headers in the
// chain of basic (0x00) or extended (0x01) headers.
type MsgGetCFHeaders struct {
	ProtocolVersion    uint32
	BlockLocatorHashes []*chainhash.Hash
	HashStop           chainhash.Hash
	FilterType         uint8
}

// AddBlockLocatorHash adds a new block locator hash to the message.
func (msg *MsgGetCFHeaders) AddBlockLocatorHash(hash *chainhash.Hash) error {
	if len(msg.BlockLocatorHashes)+1 > MaxBlockLocatorsPerMsg {
		str := fmt.Sprintf("too many block locator hashes for message [max %v]",
			MaxBlockLocatorsPerMsg)
		return messageError("MsgGetCFHeaders.AddBlockLocatorHash", str)
	}

	msg.BlockLocatorHashes = append(msg.BlockLocatorHashes, hash)
	return nil
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgGetCFHeaders) BtcDecode(r io.Reader, pver uint32, _ MessageEncoding) error {
	err := readElement(r, &msg.ProtocolVersion)
	if err != nil {
		return err
	}

	// Read num block locator hashes and limit to max.
	count, err := ReadVarInt(r, pver)
	if err != nil {
		return err
	}
	if count > MaxBlockLocatorsPerMsg {
		str := fmt.Sprintf("too many block locator hashes for message "+
			"[count %v, max %v]", count, MaxBlockLocatorsPerMsg)
		return messageError("MsgGetHeaders.BtcDecode", str)
	}

	// Create a contiguous slice of hashes to deserialize into in order to
	// reduce the number of allocations.
	locatorHashes := make([]chainhash.Hash, count)
	msg.BlockLocatorHashes = make([]*chainhash.Hash, 0, count)
	for i := uint64(0); i < count; i++ {
		hash := &locatorHashes[i]
		err := readElement(r, hash)
		if err != nil {
			return err
		}
		msg.AddBlockLocatorHash(hash)
	}

	err = readElement(r, &msg.HashStop)
	if err != nil {
		return err
	}

	return readElement(r, &msg.FilterType)
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetCFHeaders) BtcEncode(w io.Writer, pver uint32, _ MessageEncoding) error {
	// Limit to max block locator hashes per message.
	count := len(msg.BlockLocatorHashes)
	if count > MaxBlockLocatorsPerMsg {
		str := fmt.Sprintf("too many block locator hashes for message "+
			"[count %v, max %v]", count, MaxBlockLocatorsPerMsg)
		return messageError("MsgGetHeaders.BtcEncode", str)
	}

	err := writeElement(w, msg.ProtocolVersion)
	if err != nil {
		return err
	}

	err = WriteVarInt(w, pver, uint64(count))
	if err != nil {
		return err
	}

	for _, hash := range msg.BlockLocatorHashes {
		err := writeElement(w, hash)
		if err != nil {
			return err
		}
	}

	err = writeElement(w, &msg.HashStop)
	if err != nil {
		return err
	}

	return writeElement(w, msg.FilterType)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetCFHeaders) Command() string {
	return CmdGetCFHeaders
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetCFHeaders) MaxPayloadLength(pver uint32) uint32 {
	// Version 4 bytes + num block locator hashes (varInt) + max allowed
	// block locators + hash stop + filter type 1 byte.
	return 4 + MaxVarIntPayload + (MaxBlockLocatorsPerMsg *
		chainhash.HashSize) + chainhash.HashSize + 1
}

// NewMsgGetCFHeaders returns a new bitcoin getcfheader message that conforms to
// the Message interface using the passed parameters and defaults for the
// remaining fields.
func NewMsgGetCFHeaders() *MsgGetCFHeaders {
	return &MsgGetCFHeaders{
		BlockLocatorHashes: make([]*chainhash.Hash, 0,
			MaxBlockLocatorsPerMsg),
	}
}
