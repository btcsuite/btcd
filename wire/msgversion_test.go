// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
)

// TestVersion tests the MsgVersion API.
func TestVersion(t *testing.T) {
	pver := ProtocolVersion

	// Create version message data.
	lastBlock := int32(234234)
	tcpAddrMe := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	me := NewNetAddress(tcpAddrMe, SFNodeNetwork)
	tcpAddrYou := &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333}
	you := NewNetAddress(tcpAddrYou, SFNodeNetwork)
	nonce, err := RandomUint64()
	if err != nil {
		t.Errorf("RandomUint64: error generating nonce: %v", err)
	}

	// Ensure we get the correct data back out.
	msg := NewMsgVersion(me, you, nonce, lastBlock)
	if msg.ProtocolVersion != int32(pver) {
		t.Errorf("NewMsgVersion: wrong protocol version - got %v, want %v",
			msg.ProtocolVersion, pver)
	}
	if !reflect.DeepEqual(&msg.AddrMe, me) {
		t.Errorf("NewMsgVersion: wrong me address - got %v, want %v",
			spew.Sdump(&msg.AddrMe), spew.Sdump(me))
	}
	if !reflect.DeepEqual(&msg.AddrYou, you) {
		t.Errorf("NewMsgVersion: wrong you address - got %v, want %v",
			spew.Sdump(&msg.AddrYou), spew.Sdump(you))
	}
	if msg.Nonce != nonce {
		t.Errorf("NewMsgVersion: wrong nonce - got %v, want %v",
			msg.Nonce, nonce)
	}
	if msg.UserAgent != DefaultUserAgent {
		t.Errorf("NewMsgVersion: wrong user agent - got %v, want %v",
			msg.UserAgent, DefaultUserAgent)
	}
	if msg.LastBlock != lastBlock {
		t.Errorf("NewMsgVersion: wrong last block - got %v, want %v",
			msg.LastBlock, lastBlock)
	}
	if msg.DisableRelayTx {
		t.Errorf("NewMsgVersion: disable relay tx is not false by "+
			"default - got %v, want %v", msg.DisableRelayTx, false)
	}

	msg.AddUserAgent("myclient", "1.2.3", "optional", "comments")
	customUserAgent := DefaultUserAgent + "myclient:1.2.3(optional; comments)/"
	if msg.UserAgent != customUserAgent {
		t.Errorf("AddUserAgent: wrong user agent - got %s, want %s",
			msg.UserAgent, customUserAgent)
	}

	msg.AddUserAgent("mygui", "3.4.5")
	customUserAgent += "mygui:3.4.5/"
	if msg.UserAgent != customUserAgent {
		t.Errorf("AddUserAgent: wrong user agent - got %s, want %s",
			msg.UserAgent, customUserAgent)
	}

	// accounting for ":", "/"
	err = msg.AddUserAgent(strings.Repeat("t",
		MaxUserAgentLen-len(customUserAgent)-2+1), "")
	if _, ok := err.(*MessageError); !ok {
		t.Errorf("AddUserAgent: expected error not received "+
			"- got %v, want %T", err, MessageError{})

	}

	// Version message should not have any services set by default.
	if msg.Services != 0 {
		t.Errorf("NewMsgVersion: wrong default services - got %v, want %v",
			msg.Services, 0)

	}
	if msg.HasService(SFNodeNetwork) {
		t.Errorf("HasService: SFNodeNetwork service is set")
	}

	// Ensure the command is expected value.
	wantCmd := "version"
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgVersion: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value.
	// Protocol version 4 bytes + services 8 bytes + timestamp 8 bytes +
	// remote and local net addresses + nonce 8 bytes + length of user agent
	// (varInt) + max allowed user agent length + last block 4 bytes +
	// relay transactions flag 1 byte.
	wantPayload := uint32(358)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Ensure adding the full service node flag works.
	msg.AddService(SFNodeNetwork)
	if msg.Services != SFNodeNetwork {
		t.Errorf("AddService: wrong services - got %v, want %v",
			msg.Services, SFNodeNetwork)
	}
	if !msg.HasService(SFNodeNetwork) {
		t.Errorf("HasService: SFNodeNetwork service not set")
	}
}

// TestVersionWire tests the MsgVersion wire encode and decode for various
// protocol versions.
func TestVersionWire(t *testing.T) {
	// verRelayTxFalse and verRelayTxFalseEncoded is a version message as of
	// BIP0037Version with the transaction relay disabled.
	baseVersionBIP0037Copy := *baseVersionBIP0037
	verRelayTxFalse := &baseVersionBIP0037Copy
	verRelayTxFalse.DisableRelayTx = true
	verRelayTxFalseEncoded := make([]byte, len(baseVersionBIP0037Encoded))
	copy(verRelayTxFalseEncoded, baseVersionBIP0037Encoded)
	verRelayTxFalseEncoded[len(verRelayTxFalseEncoded)-1] = 0

	tests := []struct {
		in   *MsgVersion     // Message to encode
		out  *MsgVersion     // Expected decoded message
		buf  []byte          // Wire encoding
		pver uint32          // Protocol version for wire encoding
		enc  MessageEncoding // Message encoding format
	}{
		// Latest protocol version.
		{
			baseVersionBIP0037,
			baseVersionBIP0037,
			baseVersionBIP0037Encoded,
			ProtocolVersion,
			BaseEncoding,
		},

		// Protocol version BIP0037Version with relay transactions field
		// true.
		{
			baseVersionBIP0037,
			baseVersionBIP0037,
			baseVersionBIP0037Encoded,
			BIP0037Version,
			BaseEncoding,
		},

		// Protocol version BIP0037Version with relay transactions field
		// false.
		{
			verRelayTxFalse,
			verRelayTxFalse,
			verRelayTxFalseEncoded,
			BIP0037Version,
			BaseEncoding,
		},

		// Protocol version BIP0035Version.
		{
			baseVersion,
			baseVersion,
			baseVersionEncoded,
			BIP0035Version,
			BaseEncoding,
		},

		// Protocol version BIP0031Version.
		{
			baseVersion,
			baseVersion,
			baseVersionEncoded,
			BIP0031Version,
			BaseEncoding,
		},

		// Protocol version NetAddressTimeVersion.
		{
			baseVersion,
			baseVersion,
			baseVersionEncoded,
			NetAddressTimeVersion,
			BaseEncoding,
		},

		// Protocol version MultipleAddressVersion.
		{
			baseVersion,
			baseVersion,
			baseVersionEncoded,
			MultipleAddressVersion,
			BaseEncoding,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode the message to wire format.
		var buf bytes.Buffer
		err := test.in.BtcEncode(&buf, test.pver, test.enc)
		if err != nil {
			t.Errorf("BtcEncode #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("BtcEncode #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode the message from wire format.
		var msg MsgVersion
		rbuf := bytes.NewBuffer(test.buf)
		err = msg.BtcDecode(rbuf, test.pver, test.enc)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&msg, test.out) {
			t.Errorf("BtcDecode #%d\n got: %s want: %s", i,
				spew.Sdump(msg), spew.Sdump(test.out))
			continue
		}
	}
}

// TestVersionWireErrors performs negative tests against wire encode and
// decode of MsgGetHeaders to confirm error paths work correctly.
func TestVersionWireErrors(t *testing.T) {
	// Use protocol version 60002 specifically here instead of the latest
	// because the test data is using bytes encoded with that protocol
	// version.
	pver := uint32(60002)
	enc := BaseEncoding
	wireErr := &MessageError{}

	// Ensure calling MsgVersion.BtcDecode with a non *bytes.Buffer returns
	// error.
	fr := newFixedReader(0, []byte{})
	if err := baseVersion.BtcDecode(fr, pver, enc); err == nil {
		t.Errorf("Did not received error when calling " +
			"MsgVersion.BtcDecode with non *bytes.Buffer")
	}

	// Copy the base version and change the user agent to exceed max limits.
	bvc := *baseVersion
	exceedUAVer := &bvc
	newUA := "/" + strings.Repeat("t", MaxUserAgentLen-8+1) + ":0.0.1/"
	exceedUAVer.UserAgent = newUA

	// Encode the new UA length as a varint.
	var newUAVarIntBuf bytes.Buffer
	err := WriteVarInt(&newUAVarIntBuf, pver, uint64(len(newUA)))
	if err != nil {
		t.Errorf("WriteVarInt: error %v", err)
	}

	// Make a new buffer big enough to hold the base version plus the new
	// bytes for the bigger varint to hold the new size of the user agent
	// and the new user agent string.  Then stich it all together.
	newLen := len(baseVersionEncoded) - len(baseVersion.UserAgent)
	newLen = newLen + len(newUAVarIntBuf.Bytes()) - 1 + len(newUA)
	exceedUAVerEncoded := make([]byte, newLen)
	copy(exceedUAVerEncoded, baseVersionEncoded[0:80])
	copy(exceedUAVerEncoded[80:], newUAVarIntBuf.Bytes())
	copy(exceedUAVerEncoded[83:], []byte(newUA))
	copy(exceedUAVerEncoded[83+len(newUA):], baseVersionEncoded[97:100])

	tests := []struct {
		in       *MsgVersion     // Value to encode
		buf      []byte          // Wire encoding
		pver     uint32          // Protocol version for wire encoding
		enc      MessageEncoding // Message encoding format
		max      int             // Max size of fixed buffer to induce errors
		writeErr error           // Expected write error
		readErr  error           // Expected read error
	}{
		// Force error in protocol version.
		{baseVersion, baseVersionEncoded, pver, BaseEncoding, 0, io.ErrShortWrite, io.EOF},
		// Force error in services.
		{baseVersion, baseVersionEncoded, pver, BaseEncoding, 4, io.ErrShortWrite, io.EOF},
		// Force error in timestamp.
		{baseVersion, baseVersionEncoded, pver, BaseEncoding, 12, io.ErrShortWrite, io.EOF},
		// Force error in remote address.
		{baseVersion, baseVersionEncoded, pver, BaseEncoding, 20, io.ErrShortWrite, io.EOF},
		// Force error in local address.
		{baseVersion, baseVersionEncoded, pver, BaseEncoding, 47, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in nonce.
		{baseVersion, baseVersionEncoded, pver, BaseEncoding, 73, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in user agent length.
		{baseVersion, baseVersionEncoded, pver, BaseEncoding, 81, io.ErrShortWrite, io.EOF},
		// Force error in user agent.
		{baseVersion, baseVersionEncoded, pver, BaseEncoding, 82, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in last block.
		{baseVersion, baseVersionEncoded, pver, BaseEncoding, 98, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in relay tx - no read error should happen since
		// it's optional.
		{
			baseVersionBIP0037, baseVersionBIP0037Encoded,
			BIP0037Version, BaseEncoding, 101, io.ErrShortWrite, nil,
		},
		// Force error due to user agent too big
		{exceedUAVer, exceedUAVerEncoded, pver, BaseEncoding, newLen, wireErr, wireErr},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := test.in.BtcEncode(w, test.pver, test.enc)
		if reflect.TypeOf(err) != reflect.TypeOf(test.writeErr) {
			t.Errorf("BtcEncode #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		if _, ok := err.(*MessageError); !ok {
			if err != test.writeErr {
				t.Errorf("BtcEncode #%d wrong error got: %v, "+
					"want: %v", i, err, test.writeErr)
				continue
			}
		}

		// Decode from wire format.
		var msg MsgVersion
		buf := bytes.NewBuffer(test.buf[0:test.max])
		err = msg.BtcDecode(buf, test.pver, test.enc)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}

		// For errors which are not of type MessageError, check them for
		// equality.
		if _, ok := err.(*MessageError); !ok {
			if err != test.readErr {
				t.Errorf("BtcDecode #%d wrong error got: %v, "+
					"want: %v", i, err, test.readErr)
				continue
			}
		}
	}
}

// TestVersionOptionalFields performs tests to ensure that an encoded version
// messages that omit optional fields are handled correctly.
func TestVersionOptionalFields(t *testing.T) {
	// onlyRequiredVersion is a version message that only contains the
	// required versions and all other values set to their default values.
	onlyRequiredVersion := MsgVersion{
		ProtocolVersion: 60002,
		Services:        SFNodeNetwork,
		Timestamp:       time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
		AddrYou: NetAddress{
			Timestamp: time.Time{}, // Zero value -- no timestamp in version
			Services:  SFNodeNetwork,
			IP:        net.ParseIP("192.168.0.1"),
			Port:      8333,
		},
	}
	onlyRequiredVersionEncoded := make([]byte, len(baseVersionEncoded)-55)
	copy(onlyRequiredVersionEncoded, baseVersionEncoded)

	// addrMeVersion is a version message that contains all fields through
	// the AddrMe field.
	addrMeVersion := onlyRequiredVersion
	addrMeVersion.AddrMe = NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	}
	addrMeVersionEncoded := make([]byte, len(baseVersionEncoded)-29)
	copy(addrMeVersionEncoded, baseVersionEncoded)

	// nonceVersion is a version message that contains all fields through
	// the Nonce field.
	nonceVersion := addrMeVersion
	nonceVersion.Nonce = 123123 // 0x1e0f3
	nonceVersionEncoded := make([]byte, len(baseVersionEncoded)-21)
	copy(nonceVersionEncoded, baseVersionEncoded)

	// uaVersion is a version message that contains all fields through
	// the UserAgent field.
	uaVersion := nonceVersion
	uaVersion.UserAgent = "/btcdtest:0.0.1/"
	uaVersionEncoded := make([]byte, len(baseVersionEncoded)-4)
	copy(uaVersionEncoded, baseVersionEncoded)

	// lastBlockVersion is a version message that contains all fields
	// through the LastBlock field.
	lastBlockVersion := uaVersion
	lastBlockVersion.LastBlock = 234234 // 0x392fa
	lastBlockVersionEncoded := make([]byte, len(baseVersionEncoded))
	copy(lastBlockVersionEncoded, baseVersionEncoded)

	tests := []struct {
		msg  *MsgVersion     // Expected message
		buf  []byte          // Wire encoding
		pver uint32          // Protocol version for wire encoding
		enc  MessageEncoding // Message encoding format
	}{
		{
			&onlyRequiredVersion,
			onlyRequiredVersionEncoded,
			ProtocolVersion,
			BaseEncoding,
		},
		{
			&addrMeVersion,
			addrMeVersionEncoded,
			ProtocolVersion,
			BaseEncoding,
		},
		{
			&nonceVersion,
			nonceVersionEncoded,
			ProtocolVersion,
			BaseEncoding,
		},
		{
			&uaVersion,
			uaVersionEncoded,
			ProtocolVersion,
			BaseEncoding,
		},
		{
			&lastBlockVersion,
			lastBlockVersionEncoded,
			ProtocolVersion,
			BaseEncoding,
		},
	}

	for i, test := range tests {
		// Decode the message from wire format.
		var msg MsgVersion
		rbuf := bytes.NewBuffer(test.buf)
		err := msg.BtcDecode(rbuf, test.pver, test.enc)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&msg, test.msg) {
			t.Errorf("BtcDecode #%d\n got: %s want: %s", i,
				spew.Sdump(msg), spew.Sdump(test.msg))
			continue
		}
	}
}

// baseVersion is used in the various tests as a baseline MsgVersion.
var baseVersion = &MsgVersion{
	ProtocolVersion: 60002,
	Services:        SFNodeNetwork,
	Timestamp:       time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
	AddrYou: NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("192.168.0.1"),
		Port:      8333,
	},
	AddrMe: NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	},
	Nonce:     123123, // 0x1e0f3
	UserAgent: "/btcdtest:0.0.1/",
	LastBlock: 234234, // 0x392fa
}

// baseVersionEncoded is the wire encoded bytes for baseVersion using protocol
// version 60002 and is used in the various tests.
var baseVersionEncoded = []byte{
	0x62, 0xea, 0x00, 0x00, // Protocol version 60002
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
	0x29, 0xab, 0x5f, 0x49, 0x00, 0x00, 0x00, 0x00, // 64-bit Timestamp
	// AddrYou -- No timestamp for NetAddress in version message
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xc0, 0xa8, 0x00, 0x01, // IP 192.168.0.1
	0x20, 0x8d, // Port 8333 in big-endian
	// AddrMe -- No timestamp for NetAddress in version message
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01, // IP 127.0.0.1
	0x20, 0x8d, // Port 8333 in big-endian
	0xf3, 0xe0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, // Nonce
	0x10, // Varint for user agent length
	0x2f, 0x62, 0x74, 0x63, 0x64, 0x74, 0x65, 0x73,
	0x74, 0x3a, 0x30, 0x2e, 0x30, 0x2e, 0x31, 0x2f, // User agent
	0xfa, 0x92, 0x03, 0x00, // Last block
}

// baseVersionBIP0037 is used in the various tests as a baseline MsgVersion for
// BIP0037.
var baseVersionBIP0037 = &MsgVersion{
	ProtocolVersion: 70001,
	Services:        SFNodeNetwork,
	Timestamp:       time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
	AddrYou: NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("192.168.0.1"),
		Port:      8333,
	},
	AddrMe: NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	},
	Nonce:     123123, // 0x1e0f3
	UserAgent: "/btcdtest:0.0.1/",
	LastBlock: 234234, // 0x392fa
}

// baseVersionBIP0037Encoded is the wire encoded bytes for baseVersionBIP0037
// using protocol version BIP0037Version and is used in the various tests.
var baseVersionBIP0037Encoded = []byte{
	0x71, 0x11, 0x01, 0x00, // Protocol version 70001
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
	0x29, 0xab, 0x5f, 0x49, 0x00, 0x00, 0x00, 0x00, // 64-bit Timestamp
	// AddrYou -- No timestamp for NetAddress in version message
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0xc0, 0xa8, 0x00, 0x01, // IP 192.168.0.1
	0x20, 0x8d, // Port 8333 in big-endian
	// AddrMe -- No timestamp for NetAddress in version message
	0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01, // IP 127.0.0.1
	0x20, 0x8d, // Port 8333 in big-endian
	0xf3, 0xe0, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, // Nonce
	0x10, // Varint for user agent length
	0x2f, 0x62, 0x74, 0x63, 0x64, 0x74, 0x65, 0x73,
	0x74, 0x3a, 0x30, 0x2e, 0x30, 0x2e, 0x31, 0x2f, // User agent
	0xfa, 0x92, 0x03, 0x00, // Last block
	0x01, // Relay tx
}
