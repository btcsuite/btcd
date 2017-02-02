// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

type MsgGetCFilterHeader struct {
	ProtocolVersion    uint32
	BlockHash          chainhash.Hash
	Extended           bool
}

func (msg *MsgGetCFilterHeader) BtcDecode(r io.Reader, pver uint32) error {
	err := readElement(r, &msg.BlockHash)
	if err != nil {
		return err
	}
	return readElement(r, &msg.Extended)
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetCFilterHeader) BtcEncode(w io.Writer, pver uint32) error {
	err := writeElement(w, &msg.BlockHash)
	if err != nil {
		return err
	}
	return writeElement(w, &msg.Extended)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetCFilterHeader) Command() string {
	return CmdGetCFilterHeader
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetCFilterHeader) MaxPayloadLength(pver uint32) uint32 {
	// Protocol version 4 bytes + block hash + Extended flag.
	return 4 + chainhash.HashSize + 1
}

// NewMsgGetCFilterHeader returns a new bitcoin getcfilterheader message that
// conforms to the Message interface using the passed parameters and defaults
// for the remaining fields.
func NewMsgGetCFilterHeader(blockHash *chainhash.Hash, extended bool) *MsgGetCFilterHeader {
	return &MsgGetCFilterHeader{
		ProtocolVersion:     ProtocolVersion,
		BlockHash:          *blockHash,
		Extended:            extended,
	}
}
