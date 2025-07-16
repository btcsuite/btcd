// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"errors"
	"fmt"
	"io"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

const (
	// CFCheckptInterval is the gap (in number of blocks) between each
	// filter header checkpoint.
	CFCheckptInterval = 1000

	// maxCFHeadersLen is the max number of filter headers we will attempt
	// to decode.
	maxCFHeadersLen = 100000

	// maxCFCheckptPayload calculates the maximum reasonable payload size
	// for CF checkpoint messages.
	//
	// Calculation: 1 byte (filter type) + 32 bytes (stop hash) +
	// 5 bytes (max varint) + (maxCFHeadersLen * 32 bytes per hash)
	maxCFCheckptPayload = 1 + 32 + 5 + (maxCFHeadersLen * 32)
)

// ErrInsaneCFHeaderCount signals that we were asked to decode an
// unreasonable number of cfilter headers.
var ErrInsaneCFHeaderCount = errors.New(
	"refusing to decode unreasonable number of filter headers")

// MsgCFCheckpt implements the Message interface and represents a bitcoin
// cfcheckpt message.  It is used to deliver committed filter header information
// in response to a getcfcheckpt message (MsgGetCFCheckpt). See MsgGetCFCheckpt
// for details on requesting the headers.
type MsgCFCheckpt struct {
	FilterType    FilterType
	StopHash      chainhash.Hash
	FilterHeaders []*chainhash.Hash
}

// AddCFHeader adds a new committed filter header to the message.
func (msg *MsgCFCheckpt) AddCFHeader(header *chainhash.Hash) error {
	if len(msg.FilterHeaders) == cap(msg.FilterHeaders) {
		str := fmt.Sprintf("FilterHeaders has insufficient capacity for "+
			"additional header: len = %d", len(msg.FilterHeaders))
		return messageError("MsgCFCheckpt.AddCFHeader", str)
	}

	msg.FilterHeaders = append(msg.FilterHeaders, header)
	return nil
}

// BtcDecode decodes r using the bitcoin protocol encoding into the receiver.
// This is part of the Message interface implementation.
func (msg *MsgCFCheckpt) BtcDecode(r io.Reader, pver uint32, _ MessageEncoding) error {
	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	// Read filter type
	if _, err := io.ReadFull(r, buf[:1]); err != nil {
		return err
	}
	msg.FilterType = FilterType(buf[0])

	// Read stop hash
	if _, err := io.ReadFull(r, msg.StopHash[:]); err != nil {
		return err
	}

	// Read number of filter headers
	count, err := ReadVarIntBuf(r, pver, buf)
	if err != nil {
		return err
	}

	// Refuse to decode an insane number of cfheaders.
	if count > maxCFHeadersLen {
		return ErrInsaneCFHeaderCount
	}

	if count == 0 {
		msg.FilterHeaders = make([]*chainhash.Hash, 0)
		return nil
	}

	// Optimize memory allocation by creating a single backing array for
	// all hashes. This reduces GC pressure and improves cache locality.
	hashes := make([]chainhash.Hash, count)
	msg.FilterHeaders = make([]*chainhash.Hash, count)

	// Now we'll read all the hashes directly into the backing array we've
	// created above. We'll then point the underlying filter header hashes
	// into this backing array.
	for i := uint64(0); i < count; i++ {
		if _, err := io.ReadFull(r, hashes[i][:]); err != nil {
			return err
		}
		msg.FilterHeaders[i] = &hashes[i]
	}

	return nil
}

// BtcEncode encodes the receiver to w using the bitcoin protocol encoding.
// This is part of the Message interface implementation.
func (msg *MsgCFCheckpt) BtcEncode(w io.Writer, pver uint32, _ MessageEncoding) error {
	buf := binarySerializer.Borrow()
	defer binarySerializer.Return(buf)

	// Write filter type
	buf[0] = byte(msg.FilterType)
	if _, err := w.Write(buf[:1]); err != nil {
		return err
	}

	// Write stop hash
	if _, err := w.Write(msg.StopHash[:]); err != nil {
		return err
	}

	// Write length of FilterHeaders slice
	count := len(msg.FilterHeaders)
	err := WriteVarIntBuf(w, pver, uint64(count), buf)
	if err != nil {
		return err
	}

	for _, cfh := range msg.FilterHeaders {
		_, err := w.Write(cfh[:])
		if err != nil {
			return err
		}
	}

	return nil
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
func (msg *MsgCFCheckpt) Deserialize(r io.Reader) error {
	// At the current time, there is no difference between the wire encoding
	// and the stable long-term storage format.  As a result, make use of
	// BtcDecode.
	return msg.BtcDecode(r, 0, BaseEncoding)
}

// Command returns the protocol command string for the message.  This is part
// of the Message interface implementation.
func (msg *MsgCFCheckpt) Command() string {
	return CmdCFCheckpt
}

// MaxPayloadLength returns the maximum length the payload can be for the
// receiver. This is part of the Message interface implementation.
func (msg *MsgCFCheckpt) MaxPayloadLength(pver uint32) uint32 {
	// Use a more precise calculation based on the maximum number of
	// filter headers we support. No no reason to read more than we'll
	// process in BtcDecode.
	return maxCFCheckptPayload
}

// NewMsgCFCheckpt returns a new bitcoin cfheaders message that conforms to the
// Message interface. See MsgCFCheckpt for details.
func NewMsgCFCheckpt(filterType FilterType, stopHash *chainhash.Hash,
	headersCount int) *MsgCFCheckpt {

	// We pre-allocate with an exact capacity when count is known to avoid
	// slice growth during message construction.
	return &MsgCFCheckpt{
		FilterType:    filterType,
		StopHash:      *stopHash,
		FilterHeaders: make([]*chainhash.Hash, 0, headersCount),
	}
}
