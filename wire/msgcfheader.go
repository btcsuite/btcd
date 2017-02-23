// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"

	"github.com/btcsuite/fastsha256"
)

const (
	// MaxCFHeaderDataSize is the maximum byte size of a committed
	// filter header.
	MaxCFHeaderDataSize = fastsha256.Size
)

type MsgCFHeader struct {
	Data []byte
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgCFHeader) BtcDecode(r io.Reader, pver uint32) error {
	var err error
	msg.Data, err = ReadVarBytes(r, pver, MaxCFHeaderDataSize,
		"cf header data")
	return err
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgCFHeader) BtcEncode(w io.Writer, pver uint32) error {
	size := len(msg.Data)
	if size > MaxCFHeaderDataSize {
		str := fmt.Sprintf("cf header size too large for message "+
			"[size %v, max %v]", size, MaxCFHeaderDataSize)
		return messageError("MsgCFHeader.BtcEncode", str)
	}

	return WriteVarBytes(w, pver, msg.Data)
}

// Deserialize decodes a filter header from r into the receiver using a format
// that is suitable for long-term storage such as a database. This function
// differs from BtcDecode in that BtcDecode decodes from the bitcoin wire
// protocol as it was sent across the network.  The wire encoding can
// technically differ depending on the protocol version and doesn't even really
// need to match the format of a stored filter header at all. As of the time
// this comment was written, the encoded filter header is the same in both
// instances, but there is a distinct difference and separating the two allows
// the API to be flexible enough to deal with changes.
func (msg *MsgCFHeader) Deserialize(r io.Reader) error {
	// At the current time, there is no difference between the wire encoding
	// and the stable long-term storage format.  As a result, make use of
	// BtcDecode.
	return msg.BtcDecode(r, 0)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgCFHeader) Command() string {
	return CmdCFHeader
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgCFHeader) MaxPayloadLength(pver uint32) uint32 {
	return uint32(VarIntSerializeSize(MaxCFHeaderDataSize)) +
		MaxCFHeaderDataSize
}

// NewMsgCFHeader returns a new bitcoin cfheader message that conforms to
// the Message interface. See MsgCFHeader for details.
func NewMsgCFHeader(data []byte) *MsgCFHeader {
	return &MsgCFHeader{
		Data: data,
	}
}
