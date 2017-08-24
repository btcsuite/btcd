// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

type MsgGetCFilter struct {
	BlockHash chainhash.Hash
	Extended  bool
}

func (msg *MsgGetCFilter) BtcDecode(r io.Reader, pver uint32, _ MessageEncoding) error {
	err := readElement(r, &msg.BlockHash)
	if err != nil {
		return err
	}
	return readElement(r, &msg.Extended)
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetCFilter) BtcEncode(w io.Writer, pver uint32, _ MessageEncoding) error {
	err := writeElement(w, &msg.BlockHash)
	if err != nil {
		return err
	}
	return writeElement(w, msg.Extended)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetCFilter) Command() string {
	return CmdGetCFilter
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetCFilter) MaxPayloadLength(pver uint32) uint32 {
	// Block hash + Extended flag.
	return chainhash.HashSize + 1
}

// NewMsgGetCFilter returns a new bitcoin getcfilter message that conforms to
// the Message interface using the passed parameters and defaults for the
// remaining fields.
func NewMsgGetCFilter(blockHash *chainhash.Hash, extended bool) *MsgGetCFilter {
	return &MsgGetCFilter{
		BlockHash: *blockHash,
		Extended:  extended,
	}
}
