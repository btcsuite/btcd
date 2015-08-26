// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"io"
)

// MsgGetMiningState implements the Message interface and represents a
// getminingstate message.  It is used to request the current mining state
// from a peer.
type MsgGetMiningState struct{}

// BtcDecode decodes r using the decred protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgGetMiningState) BtcDecode(r io.Reader, pver uint32) error {
	return nil
}

// BtcEncode encodes the receiver to w using the decred protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgGetMiningState) BtcEncode(w io.Writer, pver uint32) error {
	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgGetMiningState) Command() string {
	return CmdGetMiningState
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgGetMiningState) MaxPayloadLength(pver uint32) uint32 {
	return 0
}

// NewMsgGetMiningState returns a new decred pong message that conforms to the Message
// interface.  See MsgPong for details.
func NewMsgGetMiningState() *MsgGetMiningState {
	return &MsgGetMiningState{}
}
