// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"
)

const (
	// MaxCFilterDataSize is the maximum byte size of a committed filter.
	MaxCFilterDataSize = 65536
)
type MsgCFilter struct {
	Data []byte
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgCFilter) BtcDecode(r io.Reader, pver uint32) error {
	var err error
	msg.Data, err = ReadVarBytes(r, pver, MaxCFilterDataSize,
	    "cfilter data")
	return err
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgCFilter) BtcEncode(w io.Writer, pver uint32) error {
	size := len(msg.Data)
	if size > MaxCFilterDataSize {
		str := fmt.Sprintf("cfilter size too large for message "+
			"[size %v, max %v]", size, MaxCFilterDataSize)
		return messageError("MsgCFilter.BtcEncode", str)
	}

	return WriteVarBytes(w, pver, msg.Data)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgCFilter) Command() string {
	return CmdCFilter
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgCFilter) MaxPayloadLength(pver uint32) uint32 {
	return uint32(VarIntSerializeSize(MaxCFilterDataSize)) +
	    MaxCFilterDataSize
}

// NewMsgFilterAdd returns a new bitcoin filteradd message that conforms to the
// Message interface. See MsgCFilter for details.
func NewMsgCFilter(data []byte) *MsgCFilter {
	return &MsgCFilter{
		Data: data,
	}
}
