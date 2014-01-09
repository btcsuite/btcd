// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire

import (
	"io"
)

// MsgAlert  implements the Message interface and defines a bitcoin alert
// message.
//
// This is a signed message that provides notifications that the client should
// display if the signature matches the key.  bitcoind/bitcoin-qt only checks
// against a signature from the core developers.
type MsgAlert struct {
	// PayloadBlob is the alert payload serialized as a string so that the
	// version can change but the Alert can still be passed on by older
	// clients.
	PayloadBlob string

	// Signature is the ECDSA signature of the message.
	Signature string
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgAlert) BtcDecode(r io.Reader, pver uint32) error {
	var err error
	msg.PayloadBlob, err = readVarString(r, pver)
	if err != nil {
		return err
	}
	msg.Signature, err = readVarString(r, pver)
	if err != nil {
		return err
	}

	return nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgAlert) BtcEncode(w io.Writer, pver uint32) error {
	var err error
	err = writeVarString(w, pver, msg.PayloadBlob)
	if err != nil {
		return err
	}
	err = writeVarString(w, pver, msg.Signature)
	if err != nil {
		return err
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgAlert) Command() string {
	return cmdAlert
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgAlert) MaxPayloadLength(pver uint32) uint32 {
	// Since this can vary depending on the message, make it the max
	// size allowed.
	return maxMessagePayload
}

// NewMsgAlert returns a new bitcoin alert message that conforms to the Message
// interface.  See MsgAlert for details.
func NewMsgAlert(payloadblob string, signature string) *MsgAlert {
	return &MsgAlert{
		PayloadBlob: payloadblob,
		Signature:   signature,
	}
}
