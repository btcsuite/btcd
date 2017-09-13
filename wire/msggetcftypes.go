// Copyright (c) 2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import "io"

// MsgGetCFTypes is the getcftypes message.
type MsgGetCFTypes struct {
}

// BtcDecode decodes the receiver from w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetCFTypes) BtcDecode(r io.Reader, pver uint32, _ MessageEncoding) error {
	return nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetCFTypes) BtcEncode(w io.Writer, pver uint32, _ MessageEncoding) error {
	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetCFTypes) Command() string {
	return CmdGetCFTypes
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetCFTypes) MaxPayloadLength(pver uint32) uint32 {
	// Empty message.
	return 0
}

// NewMsgGetCFTypes returns a new bitcoin getcftypes message that conforms to
// the Message interface.
func NewMsgGetCFTypes() *MsgGetCFTypes {
	return &MsgGetCFTypes{}
}
