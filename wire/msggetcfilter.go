// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

type MsgGetCFilter struct {
	ProtocolVersion    uint32
	BlockHash          chainhash.Hash
}

func (msg *MsgGetCFilter) BtcDecode(r io.Reader, pver uint32) error {
	return readElement(r, &msg.BlockHash)
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetCFilter) BtcEncode(w io.Writer, pver uint32) error {
	return writeElement(w, &msg.BlockHash)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetCFilter) Command() string {
	return CmdGetCFilter
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetCFilter) MaxPayloadLength(pver uint32) uint32 {
	// Protocol version 4 bytes + block hash.
	return 4 + chainhash.HashSize
}

// NewMsgGetCFilter returns a new bitcoin getblocks message that conforms to
// the Message interface using the passed parameters and defaults for the
// remaining fields.
func NewMsgGetCFilter(blockHash *chainhash.Hash) *MsgGetCFilter {
	return &MsgGetCFilter{
		ProtocolVersion:    ProtocolVersion,
		BlockHash:          *blockHash,
	}
}
