// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"github.com/conformal/btcwire"
	"io"
)

// fakeMessage implements the btcwire.Message interface and is used to force
// encode errors in messages.
type fakeMessage struct {
	command        string
	payload        []byte
	forceEncodeErr bool
}

// BtcDecode doesn't do anything.  It just satisfies the btcwire.Message
// interface.
func (msg *fakeMessage) BtcDecode(r io.Reader, pver uint32) error {
	return nil
}

// BtcEncode writes the payload field of the fake message or forces an error
// if the forceEncodeErr flag of the fake message is set.  It also satisfies the
// btcwire.Message interface.
func (msg *fakeMessage) BtcEncode(w io.Writer, pver uint32) error {
	if msg.forceEncodeErr {
		err := &btcwire.MessageError{
			Func:        "fakeMessage.BtcEncode",
			Description: "intentional error",
		}
		return err
	}

	_, err := w.Write(msg.payload)
	return err
}

// Command returns the command field of the fake message and satisfies the
// btcwire.Message interface.
func (msg *fakeMessage) Command() string {
	return msg.command
}

// MaxPayloadLength simply returns 0.  It is only here to satisfy the
// btcwire.Message interface.
func (msg *fakeMessage) MaxPayloadLength(pver uint32) uint32 {
	return 0
}
