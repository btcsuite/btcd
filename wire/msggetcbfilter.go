// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

type MsgGetCBFilter struct {
	ProtocolVersion    uint32
	BlockHash          chainhash.Hash
}

func (msg *MsgGetCBFilter) BtcDecode(r io.Reader, pver uint32) error {
	return readElement(r, &msg.BlockHash)
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetCBFilter) BtcEncode(w io.Writer, pver uint32) error {
	return writeElement(w, &msg.BlockHash)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetCBFilter) Command() string {
	return CmdGetCBFilter
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetCBFilter) MaxPayloadLength(pver uint32) uint32 {
	// Protocol version 4 bytes + block hash.
	return 4 + chainhash.HashSize
}

// NewMsgGetCBFilter returns a new bitcoin getblocks message that conforms to
// the Message interface using the passed parameters and defaults for the
// remaining fields.
func NewMsgGetCBFilter(blockHash *chainhash.Hash) *MsgGetCBFilter {
	return &MsgGetCBFilter{
		ProtocolVersion:    ProtocolVersion,
		BlockHash:          *blockHash,
	}
}
