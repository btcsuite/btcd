// Copyright (c) 2017 The btcsuite developers
// Copyright (c) 2017 The Lightning Network Developers
// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/decred/dcrd/chaincfg/chainhash"
)

// MsgGetCFilter implements the Message interface and represents a getcfilter
// message. It is used to request a committed filter for a block.
type MsgGetCFilter struct {
	BlockHash  chainhash.Hash
	FilterType FilterType
}

// BtcDecode decodes r using the wire protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgGetCFilter) BtcDecode(r io.Reader, pver uint32) error {
	if pver < NodeCFVersion {
		str := fmt.Sprintf("getcfilter message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgGetCFilter.BtcDecode", str)
	}

	err := readElement(r, &msg.BlockHash)
	if err != nil {
		return err
	}
	return readElement(r, (*uint8)(&msg.FilterType))
}

// BtcEncode encodes the receiver to w using the wire protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetCFilter) BtcEncode(w io.Writer, pver uint32) error {
	if pver < NodeCFVersion {
		str := fmt.Sprintf("getcfilter message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgGetCFilter.BtcEncode", str)
	}

	err := writeElement(w, &msg.BlockHash)
	if err != nil {
		return err
	}
	return binarySerializer.PutUint8(w, uint8(msg.FilterType))
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

// NewMsgGetCFilter returns a new getcfilter message that conforms to the
// Message interface using the passed parameters and defaults for the remaining
// fields.
func NewMsgGetCFilter(blockHash *chainhash.Hash, filterType FilterType) *MsgGetCFilter {
	return &MsgGetCFilter{
		BlockHash:  *blockHash,
		FilterType: filterType,
	}
}
