package wire

import (
	"fmt"
	"io"
)

// MsgSendAddrV2 defines a bitcoin sendaddrv2 message which is used for a peer
// to signal support for receiving ADDRV2 messages (BIP155). It implements the
// Message interface.
//
// This message has no payload.
type MsgSendAddrV2 struct{}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgSendAddrV2) BtcDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	if pver < AddrV2Version {
		str := fmt.Sprintf("sendaddrv2 message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgSendAddrV2.BtcDecode", str)
	}

	return nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgSendAddrV2) BtcEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	if pver < AddrV2Version {
		str := fmt.Sprintf("sendaddrv2 message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgSendAddrV2.BtcEncode", str)
	}

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgSendAddrV2) Command() string {
	return CmdSendAddrV2
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgSendAddrV2) MaxPayloadLength(pver uint32) uint32 {
	return 0
}

// NewMsgSendAddrV2 returns a new bitcoin sendaddrv2 message that conforms to the
// Message interface.
func NewMsgSendAddrV2() *MsgSendAddrV2 {
	return &MsgSendAddrV2{}
}
