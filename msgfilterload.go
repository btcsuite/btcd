// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire

import (
	"fmt"
	"io"
)

type BloomUpdateType uint8

const (
	// BloomUpdateNone indicates the filter is not adjusted when a match is found.
	BloomUpdateNone BloomUpdateType = 0

	// BloomUpdateAll indicates if the filter matches any data element in a
	// scriptPubKey, the outpoint is serialized and inserted into the filter.
	BloomUpdateAll BloomUpdateType = 1

	// BloomUpdateP2PubkeyOnly indicates if the filter matches a data element in
	// a scriptPubkey and the script is of the standard payToPybKey or payToMultiSig,
	// the outpoint is inserted into the filter.
	BloomUpdateP2PubkeyOnly BloomUpdateType = 2
)

const (
	// MaxFilterLoadHashFuncs is the maximum number of hash functions to load
	// into the Bloom filter.
	MaxFilterLoadHashFuncs = 50

	// MaxFilterLoadFilterSize is the maximum size in bytes a filter may be.
	MaxFilterLoadFilterSize = 36000
)

// MsgFilterLoad implements the Message interface and represents a bitcoin filterload
// message which is used to reset a Bloom filter.
//
// This message was not added until protocol version BIP0037Version.
type MsgFilterLoad struct {
	Filter    []byte
	HashFuncs uint32
	Tweak     uint32
	Flags     BloomUpdateType
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgFilterLoad) BtcDecode(r io.Reader, pver uint32) error {
	if pver < BIP0037Version {
		str := fmt.Sprintf("filterload message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgFilterLoad.BtcDecode", str)
	}

	// Read num filter and limit to max.
	size, err := readVarInt(r, pver)
	if err != nil {
		return err
	}
	if size > MaxFilterLoadFilterSize {
		str := fmt.Sprintf("filterload filter size too large for message "+
			"[size %v, max %v]", size, MaxFilterLoadFilterSize)
		return messageError("MsgFilterLoad.BtcDecode", str)
	}

	msg.Filter = make([]byte, size)
	_, err = io.ReadFull(r, msg.Filter)
	if err != nil {
		return err
	}

	err = readElements(r, &msg.HashFuncs, &msg.Tweak, &msg.Flags)
	if err != nil {
		return err
	}

	if msg.HashFuncs > MaxFilterLoadHashFuncs {
		str := fmt.Sprintf("too many filter hash functions for message "+
			"[count %v, max %v]", msg.HashFuncs, MaxFilterLoadHashFuncs)
		return messageError("MsgFilterLoad.BtcDecode", str)
	}

	return nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgFilterLoad) BtcEncode(w io.Writer, pver uint32) error {
	if pver < BIP0037Version {
		str := fmt.Sprintf("filterload message invalid for protocol "+
			"version %d", pver)
		return messageError("MsgFilterLoad.BtcEncode", str)
	}

	size := len(msg.Filter)
	if size > MaxFilterLoadFilterSize {
		str := fmt.Sprintf("filterload filter size too large for message "+
			"[size %v, max %v]", size, MaxFilterLoadFilterSize)
		return messageError("MsgFilterLoad.BtcEncode", str)
	}

	if msg.HashFuncs > MaxFilterLoadHashFuncs {
		str := fmt.Sprintf("too many filter hash functions for message "+
			"[count %v, max %v]", msg.HashFuncs, MaxFilterLoadHashFuncs)
		return messageError("MsgFilterLoad.BtcEncode", str)
	}

	err := writeVarInt(w, pver, uint64(size))
	if err != nil {
		return err
	}
	err = writeElements(w, msg.Filter, msg.HashFuncs, msg.Tweak, msg.Flags)
	if err != nil {
		return err
	}

	return nil
}

// Serialize encodes the transaction to w using a format that suitable for
// long-term storage such as a database while respecting the Version field in
// the transaction.  This function differs from BtcEncode in that BtcEncode
// encodes the transaction to the bitcoin wire protocol in order to be sent
// across the network.  The wire encoding can technically differ depending on
// the protocol version and doesn't even really need to match the format of a
// stored transaction at all.  As of the time this comment was written, the
// encoded transaction is the same in both instances, but there is a distinct
// difference and separating the two allows the API to be flexible enough to
// deal with changes.
func (msg *MsgFilterLoad) Serialize(w io.Writer) error {
	// At the current time, there is no difference between the wire encoding
	// at protocol version 0 and the stable long-term storage format.  As
	// a result, make use of BtcEncode.
	return msg.BtcEncode(w, BIP0037Version)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgFilterLoad) Command() string {
	return cmdFilterLoad
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver.  This is part of the Message interface implementation.
func (msg *MsgFilterLoad) MaxPayloadLength(pver uint32) uint32 {
	return MaxVarIntPayload + MaxFilterLoadFilterSize + 4 + 4 + 1
}

// NewMsgFilterLoad returns a new bitcoin filterload message that conforms to the Message
// interface.  See MsgFilterLoad for details.
func NewMsgFilterLoad(filter []byte, hashFuncs uint32, tweak uint32, flags BloomUpdateType) *MsgFilterLoad {
	return &MsgFilterLoad{
		Filter:    filter,
		HashFuncs: hashFuncs,
		Tweak:     tweak,
		Flags:     flags,
	}
}
