// Copyright (c) 2013-2024 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"fmt"
	"io"
	"unicode/utf8"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// MessageHeaderSize is the number of bytes in a bitcoin message header.
// Bitcoin network (magic) 4 bytes + command 12 bytes + payload length 4 bytes +
// checksum 4 bytes.
const MessageHeaderSize = 24

// CommandSize is the fixed size of all commands in the common bitcoin message
// header.  Shorter commands must be zero padded.
const CommandSize = 12

// MaxMessagePayload is the maximum bytes a message can be regardless of other
// individual limits imposed by messages themselves.
const MaxMessagePayload = (1024 * 1024 * 32) // 32MB

// Commands used in bitcoin message headers which describe the type of message.
const (
	CmdVersion      = "version"
	CmdVerAck       = "verack"
	CmdGetAddr      = "getaddr"
	CmdAddr         = "addr"
	CmdAddrV2       = "addrv2"
	CmdGetBlocks    = "getblocks"
	CmdInv          = "inv"
	CmdGetData      = "getdata"
	CmdNotFound     = "notfound"
	CmdBlock        = "block"
	CmdTx           = "tx"
	CmdGetHeaders   = "getheaders"
	CmdHeaders      = "headers"
	CmdPing         = "ping"
	CmdPong         = "pong"
	CmdMemPool      = "mempool"
	CmdFilterAdd    = "filteradd"
	CmdFilterClear  = "filterclear"
	CmdFilterLoad   = "filterload"
	CmdMerkleBlock  = "merkleblock"
	CmdReject       = "reject"
	CmdSendHeaders  = "sendheaders"
	CmdFeeFilter    = "feefilter"
	CmdGetCFilters  = "getcfilters"
	CmdGetCFHeaders = "getcfheaders"
	CmdGetCFCheckpt = "getcfcheckpt"
	CmdCFilter      = "cfilter"
	CmdCFHeaders    = "cfheaders"
	CmdCFCheckpt    = "cfcheckpt"
	CmdSendAddrV2   = "sendaddrv2"
	CmdWTxIdRelay   = "wtxidrelay"
)

var (
	v2MessageIDs = map[uint8]string{
		1:  CmdAddr,
		2:  CmdBlock,
		5:  CmdFeeFilter,
		6:  CmdFilterAdd,
		7:  CmdFilterClear,
		8:  CmdFilterLoad,
		9:  CmdGetBlocks,
		11: CmdGetData,
		12: CmdGetHeaders,
		13: CmdHeaders,
		14: CmdInv,
		15: CmdMemPool,
		16: CmdMerkleBlock,
		17: CmdNotFound,
		18: CmdPing,
		19: CmdPong,
		21: CmdTx,
		22: CmdGetCFilters,
		23: CmdCFilter,
		24: CmdGetCFHeaders,
		25: CmdCFHeaders,
		26: CmdGetCFCheckpt,
		27: CmdCFCheckpt,
		28: CmdAddrV2,
	}

	v2Messages = map[string]uint8{
		CmdAddr:         1,
		CmdBlock:        2,
		CmdFeeFilter:    5,
		CmdFilterAdd:    6,
		CmdFilterClear:  7,
		CmdFilterLoad:   8,
		CmdGetBlocks:    9,
		CmdGetData:      11,
		CmdGetHeaders:   12,
		CmdHeaders:      13,
		CmdInv:          14,
		CmdMemPool:      15,
		CmdMerkleBlock:  16,
		CmdNotFound:     17,
		CmdPing:         18,
		CmdPong:         19,
		CmdTx:           21,
		CmdGetCFilters:  22,
		CmdCFilter:      23,
		CmdGetCFHeaders: 24,
		CmdCFHeaders:    25,
		CmdGetCFCheckpt: 26,
		CmdCFCheckpt:    27,
		CmdAddrV2:       28,
	}
)

// MessageEncoding represents the wire message encoding format to be used.
type MessageEncoding uint32

const (
	// BaseEncoding encodes all messages in the default format specified
	// for the Bitcoin wire protocol.
	BaseEncoding MessageEncoding = 1 << iota

	// WitnessEncoding encodes all messages other than transaction messages
	// using the default Bitcoin wire protocol specification. For transaction
	// messages, the new encoding format detailed in BIP0144 will be used.
	WitnessEncoding
)

// LatestEncoding is the most recently specified encoding for the Bitcoin wire
// protocol.
var LatestEncoding = WitnessEncoding

// ErrUnknownMessage is the error returned when decoding an unknown message.
var ErrUnknownMessage = fmt.Errorf("received unknown message")

// ErrInvalidHandshake is the error returned when a peer sends us a known
// message that does not belong in the version-verack handshake.
var ErrInvalidHandshake = fmt.Errorf("invalid message during handshake")

// Message is an interface that describes a bitcoin message.  A type that
// implements Message has complete control over the representation of its data
// and may therefore contain additional or fewer fields than those which
// are used directly in the protocol encoded message.
type Message interface {
	BtcDecode(io.Reader, uint32, MessageEncoding) error
	BtcEncode(io.Writer, uint32, MessageEncoding) error
	Command() string
	MaxPayloadLength(uint32) uint32
}

// makeEmptyMessage creates a message of the appropriate concrete type based
// on the command.
func makeEmptyMessage(command string) (Message, error) {
	var msg Message
	switch command {
	case CmdVersion:
		msg = &MsgVersion{}

	case CmdVerAck:
		msg = &MsgVerAck{}

	case CmdSendAddrV2:
		msg = &MsgSendAddrV2{}

	case CmdGetAddr:
		msg = &MsgGetAddr{}

	case CmdAddr:
		msg = &MsgAddr{}

	case CmdAddrV2:
		msg = &MsgAddrV2{}

	case CmdGetBlocks:
		msg = &MsgGetBlocks{}

	case CmdBlock:
		msg = &MsgBlock{}

	case CmdInv:
		msg = &MsgInv{}

	case CmdGetData:
		msg = &MsgGetData{}

	case CmdNotFound:
		msg = &MsgNotFound{}

	case CmdTx:
		msg = &MsgTx{}

	case CmdPing:
		msg = &MsgPing{}

	case CmdPong:
		msg = &MsgPong{}

	case CmdGetHeaders:
		msg = &MsgGetHeaders{}

	case CmdHeaders:
		msg = &MsgHeaders{}

	case CmdMemPool:
		msg = &MsgMemPool{}

	case CmdFilterAdd:
		msg = &MsgFilterAdd{}

	case CmdFilterClear:
		msg = &MsgFilterClear{}

	case CmdFilterLoad:
		msg = &MsgFilterLoad{}

	case CmdMerkleBlock:
		msg = &MsgMerkleBlock{}

	case CmdReject:
		msg = &MsgReject{}

	case CmdSendHeaders:
		msg = &MsgSendHeaders{}

	case CmdFeeFilter:
		msg = &MsgFeeFilter{}

	case CmdGetCFilters:
		msg = &MsgGetCFilters{}

	case CmdGetCFHeaders:
		msg = &MsgGetCFHeaders{}

	case CmdGetCFCheckpt:
		msg = &MsgGetCFCheckpt{}

	case CmdCFilter:
		msg = &MsgCFilter{}

	case CmdCFHeaders:
		msg = &MsgCFHeaders{}

	case CmdCFCheckpt:
		msg = &MsgCFCheckpt{}

	default:
		return nil, ErrUnknownMessage
	}
	return msg, nil
}

// messageHeader defines the header structure for all bitcoin protocol messages.
type messageHeader struct {
	magic    BitcoinNet // 4 bytes
	command  string     // 12 bytes
	length   uint32     // 4 bytes
	checksum [4]byte    // 4 bytes
}

// readMessageHeader reads a bitcoin message header from r.
func readMessageHeader(r io.Reader) (int, *messageHeader, error) {
	// Since readElements doesn't return the amount of bytes read, attempt
	// to read the entire header into a buffer first in case there is a
	// short read so the proper amount of read bytes are known.  This works
	// since the header is a fixed size.
	var headerBytes [MessageHeaderSize]byte
	n, err := io.ReadFull(r, headerBytes[:])
	if err != nil {
		return n, nil, err
	}
	hr := bytes.NewReader(headerBytes[:])

	// Create and populate a messageHeader struct from the raw header bytes.
	hdr := messageHeader{}
	var command [CommandSize]byte
	readElements(hr, &hdr.magic, &command, &hdr.length, &hdr.checksum)

	// Strip trailing zeros from command string.
	hdr.command = string(bytes.TrimRight(command[:], "\x00"))

	return n, &hdr, nil
}

// readPartialHeader reads a partial bitcon message header from r. It takes a
// prefix that contains the already-parsed bytes. This is needed in the case of
// a downgraded v2->v1 transport connection since we have already started
// parsing the header bytes when calling into this function.
func readPartialHeader(prefix []byte, r io.Reader) (int, *messageHeader,
	error) {

	// Fill out the messageHeader with the network magic from the prefix.
	hdr := messageHeader{}

	var command [CommandSize]byte
	prefixReader := bytes.NewReader(prefix[:])
	if err := readElements(prefixReader, &hdr.magic, &command); err != nil {
		return 0, nil, err
	}

	// Strip trailing zeros from command string.
	hdr.command = string(bytes.TrimRight(command[:], "\x00"))

	// Read the rest of the message header from the passed-in reader.
	if err := readElements(r, &hdr.length, &hdr.checksum); err != nil {
		return 0, nil, err
	}

	return MessageHeaderSize, &hdr, nil
}

// discardInput reads n bytes from reader r in chunks and discards the read
// bytes.  This is used to skip payloads when various errors occur and helps
// prevent rogue nodes from causing massive memory allocation through forging
// header length.
func discardInput(r io.Reader, n uint32) {
	maxSize := uint32(10 * 1024) // 10k at a time
	numReads := n / maxSize
	bytesRemaining := n % maxSize
	if n > 0 {
		buf := make([]byte, maxSize)
		for i := uint32(0); i < numReads; i++ {
			io.ReadFull(r, buf)
		}
	}
	if bytesRemaining > 0 {
		buf := make([]byte, bytesRemaining)
		io.ReadFull(r, buf)
	}
}

// WriteMessageN writes a bitcoin Message to w including the necessary header
// information and returns the number of bytes written.    This function is the
// same as WriteMessage except it also returns the number of bytes written.
func WriteMessageN(w io.Writer, msg Message, pver uint32, btcnet BitcoinNet) (int, error) {
	return WriteMessageWithEncodingN(w, msg, pver, btcnet, BaseEncoding)
}

// WriteMessage writes a bitcoin Message to w including the necessary header
// information.  This function is the same as WriteMessageN except it doesn't
// doesn't return the number of bytes written.  This function is mainly provided
// for backwards compatibility with the original API, but it's also useful for
// callers that don't care about byte counts.
func WriteMessage(w io.Writer, msg Message, pver uint32, btcnet BitcoinNet) error {
	_, err := WriteMessageN(w, msg, pver, btcnet)
	return err
}

// WriteV2MessageN writes a Message to the passed Writer using the bip324
// v2 encoding.
func WriteV2MessageN(w io.Writer, msg Message, pver uint32,
	encoding MessageEncoding) (int, error) {

	var totalBytes int

	cmd := msg.Command()
	if len(cmd) > CommandSize {
		str := fmt.Sprintf("command [%s] is too long [max %v]",
			cmd, CommandSize)
		return totalBytes, messageError("WriteMessage", str)
	}

	index, exists := v2Messages[cmd]
	if !exists {
		var command [CommandSize]byte
		copy(command[:], cmd)
		hw := bytes.NewBuffer(make([]byte, 0, CommandSize+1))
		writeElements(hw, byte(0x00), command)

		n, err := w.Write(hw.Bytes())
		if err != nil {
			return 0, err
		}

		totalBytes += n
	} else {
		hw := bytes.NewBuffer(make([]byte, 0, 1))
		writeElement(hw, index)

		n, err := w.Write(hw.Bytes())
		if err != nil {
			return 0, err
		}

		totalBytes += n
	}

	var bw bytes.Buffer
	err := msg.BtcEncode(&bw, pver, encoding)
	if err != nil {
		return totalBytes, err
	}

	payload := bw.Bytes()
	lenp := len(payload)

	// Enforce maximum overall message payload.
	if lenp > MaxMessagePayload {
		str := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload is %d bytes",
			lenp, MaxMessagePayload)
		return totalBytes, messageError("WriteMessage", str)
	}

	mpl := msg.MaxPayloadLength(pver)
	if uint32(lenp) > mpl {
		str := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload size for "+
			"messages of type [%s] is %d.", lenp, cmd, mpl)
		return totalBytes, messageError("WriteMessage", str)
	}

	if len(payload) > 0 {
		n, err := w.Write(payload)
		totalBytes += n

		return totalBytes, err
	}

	return totalBytes, nil
}

// WriteMessageWithEncodingN writes a bitcoin Message to w including the
// necessary header information and returns the number of bytes written.
// This function is the same as WriteMessageN except it also allows the caller
// to specify the message encoding format to be used when serializing wire
// messages.
func WriteMessageWithEncodingN(w io.Writer, msg Message, pver uint32,
	btcnet BitcoinNet, encoding MessageEncoding) (int, error) {

	totalBytes := 0

	// Enforce max command size.
	var command [CommandSize]byte
	cmd := msg.Command()
	if len(cmd) > CommandSize {
		str := fmt.Sprintf("command [%s] is too long [max %v]",
			cmd, CommandSize)
		return totalBytes, messageError("WriteMessage", str)
	}
	copy(command[:], []byte(cmd))

	// Encode the message payload.
	var bw bytes.Buffer
	err := msg.BtcEncode(&bw, pver, encoding)
	if err != nil {
		return totalBytes, err
	}
	payload := bw.Bytes()
	lenp := len(payload)

	// Enforce maximum overall message payload.
	if lenp > MaxMessagePayload {
		str := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload is %d bytes",
			lenp, MaxMessagePayload)
		return totalBytes, messageError("WriteMessage", str)
	}

	// Enforce maximum message payload based on the message type.
	mpl := msg.MaxPayloadLength(pver)
	if uint32(lenp) > mpl {
		str := fmt.Sprintf("message payload is too large - encoded "+
			"%d bytes, but maximum message payload size for "+
			"messages of type [%s] is %d.", lenp, cmd, mpl)
		return totalBytes, messageError("WriteMessage", str)
	}

	// Create header for the message.
	hdr := messageHeader{}
	hdr.magic = btcnet
	hdr.command = cmd
	hdr.length = uint32(lenp)
	copy(hdr.checksum[:], chainhash.DoubleHashB(payload)[0:4])

	// Encode the header for the message.  This is done to a buffer
	// rather than directly to the writer since writeElements doesn't
	// return the number of bytes written.
	hw := bytes.NewBuffer(make([]byte, 0, MessageHeaderSize))
	writeElements(hw, hdr.magic, command, hdr.length, hdr.checksum)

	// Write header.
	n, err := w.Write(hw.Bytes())
	totalBytes += n
	if err != nil {
		return totalBytes, err
	}

	// Only write the payload if there is one, e.g., verack messages don't
	// have one.
	if len(payload) > 0 {
		n, err = w.Write(payload)
		totalBytes += n
	}

	return totalBytes, err
}

// ReadV2MessageN takes the passed plaintext and attempts to construct a
// Message from the bytes using the bip324 v2 encoding.
func ReadV2MessageN(plaintext []byte, pver uint32, enc MessageEncoding) (
	Message, []byte, error) {

	if len(plaintext) == 0 {
		return nil, nil, fmt.Errorf("invalid plaintext length")
	}

	var msgCmd string

	// If the first byte is 0x00, read the next 12 bytes to determine what
	// message this is.
	if plaintext[0] == 0x00 {
		if len(plaintext) < CommandSize+1 {
			return nil, nil, fmt.Errorf("invalid plaintext length")
		}

		// Slice off the first 0x00 and the trailing 0x00 bytes.
		var command [CommandSize]byte
		copy(command[:], plaintext[1:CommandSize+1])

		msgCmd = string(bytes.TrimRight(command[:], "\x00"))

		plaintext = plaintext[CommandSize+1:]
	} else {
		// The first byte denotes what message this is.
		msgCmd = v2MessageIDs[plaintext[0]]

		plaintext = plaintext[1:]
	}

	msg, err := makeEmptyMessage(msgCmd)
	if err != nil {
		return nil, nil, err
	}

	mpl := msg.MaxPayloadLength(pver)
	if len(plaintext) > int(mpl) {
		return nil, nil, fmt.Errorf("payload exceeds max length")
	}

	buf := bytes.NewBuffer(plaintext)
	err = msg.BtcDecode(buf, pver, enc)
	if err != nil {
		return nil, nil, err
	}

	return msg, plaintext, nil
}

// ReadMessageWithEncodingN reads, validates, and parses the next bitcoin Message
// from r for the provided protocol version and bitcoin network.  It returns the
// number of bytes read in addition to the parsed Message and raw bytes which
// comprise the message.  This function is the same as ReadMessageN except it
// allows the caller to specify which message encoding is to to consult when
// decoding wire messages.
func ReadMessageWithEncodingN(r io.Reader, pver uint32, btcnet BitcoinNet,
	enc MessageEncoding) (int, Message, []byte, error) {

	totalBytes := 0
	n, hdr, err := readMessageHeader(r)
	totalBytes += n
	if err != nil {
		return totalBytes, nil, nil, err
	}

	return readMessageWithEncodingNInternal(
		r, pver, hdr, btcnet, enc, totalBytes,
	)
}

// ReadPartialMessageWithEncodingN is used in the case that we are expecting a
// v2 connection and then receive a v1 version header. In this case, we
// downgrade the implicit v2 connection to a v1 connection and must parse the
// rest of the version bytes and check it properly.
func ReadPartialMessageWithEncodingN(r io.Reader, pver uint32,
	btcnet BitcoinNet, enc MessageEncoding, prefix []byte) (int, Message,
	[]byte, error) {

	totalBytes := 0
	n, hdr, err := readPartialHeader(prefix, r)
	totalBytes += n
	if err != nil {
		return totalBytes, nil, nil, err
	}

	return readMessageWithEncodingNInternal(
		r, pver, hdr, btcnet, enc, totalBytes,
	)
}

// readMessageWithEncodingNInternal is used to deduplicate the code because we
// typically parse messages and headers all at once except in the case of a
// downgraded v2->v1 conncection.
func readMessageWithEncodingNInternal(r io.Reader, pver uint32,
	hdr *messageHeader, btcnet BitcoinNet, enc MessageEncoding,
	totalBytes int) (int, Message, []byte, error) {

	// Enforce maximum message payload.
	if hdr.length > MaxMessagePayload {
		str := fmt.Sprintf("message payload is too large - header "+
			"indicates %d bytes, but max message payload is %d "+
			"bytes.", hdr.length, MaxMessagePayload)
		return totalBytes, nil, nil, messageError("ReadMessage", str)

	}

	// Check for messages from the wrong bitcoin network.
	if hdr.magic != btcnet {
		discardInput(r, hdr.length)
		str := fmt.Sprintf("message from other network [%v]", hdr.magic)
		return totalBytes, nil, nil, messageError("ReadMessage", str)
	}

	// Check for malformed commands.
	command := hdr.command
	if !utf8.ValidString(command) {
		discardInput(r, hdr.length)
		str := fmt.Sprintf("invalid command %v", []byte(command))
		return totalBytes, nil, nil, messageError("ReadMessage", str)
	}

	// Create struct of appropriate message type based on the command.
	msg, err := makeEmptyMessage(command)
	if err != nil {
		// makeEmptyMessage can only return ErrUnknownMessage and it is
		// important that we bubble it up to the caller.
		discardInput(r, hdr.length)
		return totalBytes, nil, nil, err
	}

	// Check for maximum length based on the message type as a malicious client
	// could otherwise create a well-formed header and set the length to max
	// numbers in order to exhaust the machine's memory.
	mpl := msg.MaxPayloadLength(pver)
	if hdr.length > mpl {
		discardInput(r, hdr.length)
		str := fmt.Sprintf("payload exceeds max length - header "+
			"indicates %v bytes, but max payload size for "+
			"messages of type [%v] is %v.", hdr.length, command, mpl)
		return totalBytes, nil, nil, messageError("ReadMessage", str)
	}

	// Read payload.
	payload := make([]byte, hdr.length)
	n, err := io.ReadFull(r, payload)
	totalBytes += n
	if err != nil {
		return totalBytes, nil, nil, err
	}

	// Test checksum.
	checksum := chainhash.DoubleHashB(payload)[0:4]
	if !bytes.Equal(checksum, hdr.checksum[:]) {
		str := fmt.Sprintf("payload checksum failed - header "+
			"indicates %v, but actual checksum is %v.",
			hdr.checksum, checksum)
		return totalBytes, nil, nil, messageError("ReadMessage", str)
	}

	// Unmarshal message.  NOTE: This must be a *bytes.Buffer since the
	// MsgVersion BtcDecode function requires it.
	pr := bytes.NewBuffer(payload)
	err = msg.BtcDecode(pr, pver, enc)
	if err != nil {
		return totalBytes, nil, nil, err
	}

	return totalBytes, msg, payload, nil
}

// ReadMessageN reads, validates, and parses the next bitcoin Message from r for
// the provided protocol version and bitcoin network.  It returns the number of
// bytes read in addition to the parsed Message and raw bytes which comprise the
// message.  This function is the same as ReadMessage except it also returns the
// number of bytes read.
func ReadMessageN(r io.Reader, pver uint32, btcnet BitcoinNet) (int, Message, []byte, error) {
	return ReadMessageWithEncodingN(r, pver, btcnet, BaseEncoding)
}

// ReadMessage reads, validates, and parses the next bitcoin Message from r for
// the provided protocol version and bitcoin network.  It returns the parsed
// Message and raw bytes which comprise the message.  This function only differs
// from ReadMessageN in that it doesn't return the number of bytes read.  This
// function is mainly provided for backwards compatibility with the original
// API, but it's also useful for callers that don't care about byte counts.
func ReadMessage(r io.Reader, pver uint32, btcnet BitcoinNet) (Message, []byte, error) {
	_, msg, buf, err := ReadMessageN(r, pver, btcnet)
	return msg, buf, err
}
