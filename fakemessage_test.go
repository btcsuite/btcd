// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"io"
)

// fakeMessage implements the btcwire.Message interface and is used to force
// errors.
type fakeMessage struct {
	command    string
	maxPayload uint32
}

// BtcDecode doesn't do anything.  It just satisfies the btcwire.Message
// interface.
func (msg *fakeMessage) BtcDecode(r io.Reader, pver uint32) error {
	return nil
}

// BtcEncode doesn't do anything.  It just satisfies the btcwire.Message
// interface.
func (msg *fakeMessage) BtcEncode(w io.Writer, pver uint32) error {
	return nil
}

// Command returns the command field of the fake message and satisfies the
// btcwire.Message interface.
func (msg *fakeMessage) Command() string {
	return msg.command
}

// Command returns the maxPayload field of the fake message and satisfies the
// btcwire.Message interface.
func (msg *fakeMessage) MaxPayloadLength(pver uint32) uint32 {
	return msg.maxPayload
}
