// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// MsgGetCFilter implements the Message interface and represents a bitcoin
// getcfilter message. It is used to request a committed filter for a block.
type MsgGetCFilter struct {
	BlockHash  chainhash.Hash
	FilterType FilterType
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgGetCFilter) BtcDecode(r io.Reader, pver uint32, _ MessageEncoding) error {
	err := readElement(r, &msg.BlockHash)
	if err != nil {
		return err
	}
	return readElement(r, &msg.FilterType)
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetCFilter) BtcEncode(w io.Writer, pver uint32, _ MessageEncoding) error {
	err := writeElement(w, &msg.BlockHash)
	if err != nil {
		return err
	}
	return writeElement(w, msg.FilterType)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetCFilter) Command() string {
	return CmdGetCFilter
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetCFilter) MaxPayloadLength(pver uint32) uint32 {
	// Block hash + filter type.
	return chainhash.HashSize + 1
}

// NewMsgGetCFilter returns a new bitcoin getcfilter message that conforms to
// the Message interface using the passed parameters and defaults for the
// remaining fields.
func NewMsgGetCFilter(blockHash *chainhash.Hash,
	filterType FilterType) *MsgGetCFilter {
	return &MsgGetCFilter{
		BlockHash:  *blockHash,
		FilterType: filterType,
	}
}
