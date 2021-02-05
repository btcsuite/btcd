// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"fmt"
	"io"
)

// MsgNotFound defines a bitcoin notfound message which is sent in response to
// a getdata message if any of the requested data in not available on the peer.
// Each message is limited to a maximum number of inventory vectors, which is
// currently 50,000.
//
// Use the AddInvVect function to build up the list of inventory vectors when
// sending a notfound message to another peer.
type MsgNotFound struct {
	InvList []*InvVect
}

// AddInvVect adds an inventory vector to the message.
func (msg *MsgNotFound) AddInvVect(iv *InvVect) error {
	if len(msg.InvList)+1 > MaxInvPerMsg {
		str := fmt.Sprintf("too many invvect in message [max %v]",
			MaxInvPerMsg)
		return messageError("MsgNotFound.AddInvVect", str)
	}

	msg.InvList = append(msg.InvList, iv)
	return nil
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgNotFound) BtcDecode(r io.Reader, pver uint32, enc MessageEncoding) error {
	buf := binarySerializer.Borrow()
	count, err := ReadVarIntBuf(r, pver, buf)
	if err != nil {
		binarySerializer.Return(buf)
		return err
	}

	// Limit to max inventory vectors per message.
	if count > MaxInvPerMsg {
		binarySerializer.Return(buf)
		str := fmt.Sprintf("too many invvect in message [%v]", count)
		return messageError("MsgNotFound.BtcDecode", str)
	}

	// Create a contiguous slice of inventory vectors to deserialize into in
	// order to reduce the number of allocations.
	invList := make([]InvVect, count)
	msg.InvList = make([]*InvVect, 0, count)
	for i := uint64(0); i < count; i++ {
		iv := &invList[i]
		err := readInvVectBuf(r, pver, iv, buf)
		if err != nil {
			binarySerializer.Return(buf)
			return err
		}
		msg.AddInvVect(iv)
	}
	binarySerializer.Return(buf)

	return nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgNotFound) BtcEncode(w io.Writer, pver uint32, enc MessageEncoding) error {
	// Limit to max inventory vectors per message.
	count := len(msg.InvList)
	if count > MaxInvPerMsg {
		str := fmt.Sprintf("too many invvect in message [%v]", count)
		return messageError("MsgNotFound.BtcEncode", str)
	}

	buf := binarySerializer.Borrow()
	err := WriteVarIntBuf(w, pver, uint64(count), buf)
	if err != nil {
		binarySerializer.Return(buf)
		return err
	}

	for _, iv := range msg.InvList {
		err := writeInvVectBuf(w, pver, iv, buf)
		if err != nil {
			binarySerializer.Return(buf)
			return err
		}
	}
	binarySerializer.Return(buf)

	return nil
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgNotFound) Command() string {
	return CmdNotFound
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgNotFound) MaxPayloadLength(pver uint32) uint32 {
	// Max var int 9 bytes + max InvVects at 36 bytes each.
	// Num inventory vectors (varInt) + max allowed inventory vectors.
	return MaxVarIntPayload + (MaxInvPerMsg * maxInvVectPayload)
}

// NewMsgNotFound returns a new bitcoin notfound message that conforms to the
// Message interface.  See MsgNotFound for details.
func NewMsgNotFound() *MsgNotFound {
	return &MsgNotFound{
		InvList: make([]*InvVect, 0, defaultInvListAlloc),
	}
}
