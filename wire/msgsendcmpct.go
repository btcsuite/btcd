// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"
)

// MsgSendCmpct implements the Message interface and represents a bitcoin
// sendcmpct message.  It is used to signal support for compact block relay and
// whether compact blocks should be used for high-bandwidth block
// announcements.
type MsgSendCmpct struct {
	Announce uint8
	Version  uint64
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgSendCmpct) BtcDecode(r io.Reader, pver uint32,
	enc MessageEncoding) error {
	if pver < ShortIdsBlocksVersion {
		str := fmt.Sprintf("sendcmpct message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgSendCmpct.BtcDecode", str)
	}

	if err := readElements(r, &msg.Announce, &msg.Version); err != nil {
		return err
	}

	if msg.Announce > 1 {
		str := "invalid sendcmpct announce field"
		return messageError("MsgSendCmpct.BtcDecode", str)
	}
	return nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgSendCmpct) BtcEncode(w io.Writer, pver uint32,
	enc MessageEncoding) error {
	if pver < ShortIdsBlocksVersion {
		str := fmt.Sprintf("sendcmpct message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgSendCmpct.BtcEncode", str)
	}

	if msg.Announce > 1 {
		str := "invalid sendcmpct announce field"
		return messageError("MsgSendCmpct.BtcEncode", str)
	}

	return writeElements(w, msg.Announce, msg.Version)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgSendCmpct) Command() string {
	return CmdSendCmpct
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgSendCmpct) MaxPayloadLength(pver uint32) uint32 {
	return 9
}

// NewMsgSendCmpct returns a new bitcoin sendcmpct message that conforms to the
// Message interface.  See MsgSendCmpct for details.
func NewMsgSendCmpct(announce uint8, version uint64) *MsgSendCmpct {
	return &MsgSendCmpct{
		Announce: announce,
		Version:  version,
	}
}
