// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire_test

import (
	"bytes"
	"io"
	"net"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/btcsuite/btcd/wire"
	"github.com/davecgh/go-spew/spew"
)

// TestVersion tests the MsgVersion API.
func TestVersion(t *testing.T) {
	pver := wire.ProtocolVersion

	// Create version message data.
	lastBlock := int32(234234)
	tcpAddrMe := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	me, err := wire.NewNetAddress(tcpAddrMe, wire.SFNodeNetwork)
	if err != nil {
		t.Errorf("NewNetAddress: %v", err)
	}
	tcpAddrYou := &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333}
	you, err := wire.NewNetAddress(tcpAddrYou, wire.SFNodeNetwork)
	if err != nil {
		t.Errorf("NewNetAddress: %v", err)
	}
	nonce, err := wire.RandomUint64()
	if err != nil {
		t.Errorf("RandomUint64: error generating nonce: %v", err)
	}

	// Ensure we get the correct data back out.
	msg := wire.NewMsgVersion(me, you, nonce, lastBlock)
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
	if msg.UserAgent != wire.DefaultUserAgent {
		t.Errorf("NewMsgVersion: wrong user agent - got %v, want %v",
			msg.UserAgent, wire.DefaultUserAgent)
	}
	if msg.LastBlock != lastBlock {
		t.Errorf("NewMsgVersion: wrong last block - got %v, want %v",
			msg.LastBlock, lastBlock)
	}
	if msg.DisableRelayTx != false {
		t.Errorf("NewMsgVersion: disable relay tx is not false by "+
			"default - got %v, want %v", msg.DisableRelayTx, false)
	}

	msg.AddUserAgent("myclient", "1.2.3", "optional", "comments")
	customUserAgent := wire.DefaultUserAgent + "myclient:1.2.3(optional; comments)/"
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
		wire.MaxUserAgentLen-len(customUserAgent)-2+1), "")
	if _, ok := err.(*wire.MessageError); !ok {
		t.Errorf("AddUserAgent: expected error not received "+
			"- got %v, want %T", err, wire.MessageError{})

	}

	// Version message should not have any services set by default.
	if msg.Services != 0 {
		t.Errorf("NewMsgVersion: wrong default services - got %v, want %v",
			msg.Services, 0)

	}
	if msg.HasService(wire.SFNodeNetwork) {
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
	wantPayload := uint32(2102)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Ensure adding the full service node flag works.
	msg.AddService(wire.SFNodeNetwork)
	if msg.Services != wire.SFNodeNetwork {
		t.Errorf("AddService: wrong services - got %v, want %v",
			msg.Services, wire.SFNodeNetwork)
	}
	if !msg.HasService(wire.SFNodeNetwork) {
		t.Errorf("HasService: SFNodeNetwork service not set")
	}

	// Use a fake connection.
	conn := &fakeConn{localAddr: tcpAddrMe, remoteAddr: tcpAddrYou}
	msg, err = wire.NewMsgVersionFromConn(conn, nonce, lastBlock)
	if err != nil {
		t.Errorf("NewMsgVersionFromConn: %v", err)
	}

	// Ensure we get the correct connection data back out.
	if !msg.AddrMe.IP.Equal(tcpAddrMe.IP) {
		t.Errorf("NewMsgVersionFromConn: wrong me ip - got %v, want %v",
			msg.AddrMe.IP, tcpAddrMe.IP)
	}
	if !msg.AddrYou.IP.Equal(tcpAddrYou.IP) {
		t.Errorf("NewMsgVersionFromConn: wrong you ip - got %v, want %v",
			msg.AddrYou.IP, tcpAddrYou.IP)
	}

	// Use a fake connection with local UDP addresses to force a failure.
	conn = &fakeConn{
		localAddr:  &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333},
		remoteAddr: tcpAddrYou,
	}
	msg, err = wire.NewMsgVersionFromConn(conn, nonce, lastBlock)
	if err != wire.ErrInvalidNetAddr {
		t.Errorf("NewMsgVersionFromConn: expected error not received "+
			"- got %v, want %v", err, wire.ErrInvalidNetAddr)
	}

	// Use a fake connection with remote UDP addresses to force a failure.
	conn = &fakeConn{
		localAddr:  tcpAddrMe,
		remoteAddr: &net.UDPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333},
	}
	msg, err = wire.NewMsgVersionFromConn(conn, nonce, lastBlock)
	if err != wire.ErrInvalidNetAddr {
		t.Errorf("NewMsgVersionFromConn: expected error not received "+
			"- got %v, want %v", err, wire.ErrInvalidNetAddr)
	}

	return
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
		in   *wire.MsgVersion // Message to encode
		out  *wire.MsgVersion // Expected decoded message
		buf  []byte           // Wire encoding
		pver uint32           // Protocol version for wire encoding
	}{
		// Latest protocol version.
		{
			baseVersionBIP0037,
			baseVersionBIP0037,
			baseVersionBIP0037Encoded,
			wire.ProtocolVersion,
		},

		// Protocol version BIP0037Version with relay transactions field
		// true.
		{
			baseVersionBIP0037,
			baseVersionBIP0037,
			baseVersionBIP0037Encoded,
			wire.BIP0037Version,
		},

		// Protocol version BIP0037Version with relay transactions field
		// false.
		{
			verRelayTxFalse,
			verRelayTxFalse,
			verRelayTxFalseEncoded,
			wire.BIP0037Version,
		},

		// Protocol version BIP0035Version.
		{
			baseVersion,
			baseVersion,
			baseVersionEncoded,
			wire.BIP0035Version,
		},

		// Protocol version BIP0031Version.
		{
			baseVersion,
			baseVersion,
			baseVersionEncoded,
			wire.BIP0031Version,
		},

		// Protocol version NetAddressTimeVersion.
		{
			baseVersion,
			baseVersion,
			baseVersionEncoded,
			wire.NetAddressTimeVersion,
		},

		// Protocol version MultipleAddressVersion.
		{
			baseVersion,
			baseVersion,
			baseVersionEncoded,
			wire.MultipleAddressVersion,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode the message to wire format.
		var buf bytes.Buffer
		err := test.in.BtcEncode(&buf, test.pver)
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
		var msg wire.MsgVersion
		rbuf := bytes.NewBuffer(test.buf)
		err = msg.BtcDecode(rbuf, test.pver)
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
	wireErr := &wire.MessageError{}

	// Ensure calling MsgVersion.BtcDecode with a non *bytes.Buffer returns
	// error.
	fr := newFixedReader(0, []byte{})
	if err := baseVersion.BtcDecode(fr, pver); err == nil {
		t.Errorf("Did not received error when calling " +
			"MsgVersion.BtcDecode with non *bytes.Buffer")
	}

	// Copy the base version and change the user agent to exceed max limits.
	bvc := *baseVersion
	exceedUAVer := &bvc
	newUA := "/" + strings.Repeat("t", wire.MaxUserAgentLen-8+1) + ":0.0.1/"
	exceedUAVer.UserAgent = newUA

	// Encode the new UA length as a varint.
	var newUAVarIntBuf bytes.Buffer
	err := wire.TstWriteVarInt(&newUAVarIntBuf, pver, uint64(len(newUA)))
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
		in       *wire.MsgVersion // Value to encode
		buf      []byte           // Wire encoding
		pver     uint32           // Protocol version for wire encoding
		max      int              // Max size of fixed buffer to induce errors
		writeErr error            // Expected write error
		readErr  error            // Expected read error
	}{
		// Force error in protocol version.
		{baseVersion, baseVersionEncoded, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error in services.
		{baseVersion, baseVersionEncoded, pver, 4, io.ErrShortWrite, io.EOF},
		// Force error in timestamp.
		{baseVersion, baseVersionEncoded, pver, 12, io.ErrShortWrite, io.EOF},
		// Force error in remote address.
		{baseVersion, baseVersionEncoded, pver, 20, io.ErrShortWrite, io.EOF},
		// Force error in local address.
		{baseVersion, baseVersionEncoded, pver, 47, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in nonce.
		{baseVersion, baseVersionEncoded, pver, 73, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in user agent length.
		{baseVersion, baseVersionEncoded, pver, 81, io.ErrShortWrite, io.EOF},
		// Force error in user agent.
		{baseVersion, baseVersionEncoded, pver, 82, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in last block.
		{baseVersion, baseVersionEncoded, pver, 98, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force error in relay tx - no read error should happen since
		// it's optional.
		{
			baseVersionBIP0037, baseVersionBIP0037Encoded,
			wire.BIP0037Version, 101, io.ErrShortWrite, nil,
		},
		// Force error due to user agent too big
		{exceedUAVer, exceedUAVerEncoded, pver, newLen, wireErr, wireErr},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := test.in.BtcEncode(w, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.writeErr) {
			t.Errorf("BtcEncode #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// For errors which are not of type wire.MessageError, check
		// them for equality.
		if _, ok := err.(*wire.MessageError); !ok {
			if err != test.writeErr {
				t.Errorf("BtcEncode #%d wrong error got: %v, "+
					"want: %v", i, err, test.writeErr)
				continue
			}
		}

		// Decode from wire format.
		var msg wire.MsgVersion
		buf := bytes.NewBuffer(test.buf[0:test.max])
		err = msg.BtcDecode(buf, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.readErr) {
			t.Errorf("BtcDecode #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}

		// For errors which are not of type wire.MessageError, check
		// them for equality.
		if _, ok := err.(*wire.MessageError); !ok {
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
	onlyRequiredVersion := wire.MsgVersion{
		ProtocolVersion: 60002,
		Services:        wire.SFNodeNetwork,
		Timestamp:       time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
		AddrYou: wire.NetAddress{
			Timestamp: time.Time{}, // Zero value -- no timestamp in version
			Services:  wire.SFNodeNetwork,
			IP:        net.ParseIP("192.168.0.1"),
			Port:      8333,
		},
	}
	onlyRequiredVersionEncoded := make([]byte, len(baseVersionEncoded)-55)
	copy(onlyRequiredVersionEncoded, baseVersionEncoded)

	// addrMeVersion is a version message that contains all fields through
	// the AddrMe field.
	addrMeVersion := onlyRequiredVersion
	addrMeVersion.AddrMe = wire.NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Services:  wire.SFNodeNetwork,
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
		msg  *wire.MsgVersion // Expected message
		buf  []byte           // Wire encoding
		pver uint32           // Protocol version for wire encoding
	}{
		{
			&onlyRequiredVersion,
			onlyRequiredVersionEncoded,
			wire.ProtocolVersion,
		},
		{
			&addrMeVersion,
			addrMeVersionEncoded,
			wire.ProtocolVersion,
		},
		{
			&nonceVersion,
			nonceVersionEncoded,
			wire.ProtocolVersion,
		},
		{
			&uaVersion,
			uaVersionEncoded,
			wire.ProtocolVersion,
		},
		{
			&lastBlockVersion,
			lastBlockVersionEncoded,
			wire.ProtocolVersion,
		},
	}

	for i, test := range tests {
		// Decode the message from wire format.
		var msg wire.MsgVersion
		rbuf := bytes.NewBuffer(test.buf)
		err := msg.BtcDecode(rbuf, test.pver)
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
var baseVersion = &wire.MsgVersion{
	ProtocolVersion: 60002,
	Services:        wire.SFNodeNetwork,
	Timestamp:       time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
	AddrYou: wire.NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Services:  wire.SFNodeNetwork,
		IP:        net.ParseIP("192.168.0.1"),
		Port:      8333,
	},
	AddrMe: wire.NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Services:  wire.SFNodeNetwork,
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
var baseVersionBIP0037 = &wire.MsgVersion{
	ProtocolVersion: 70001,
	Services:        wire.SFNodeNetwork,
	Timestamp:       time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST)
	AddrYou: wire.NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Services:  wire.SFNodeNetwork,
		IP:        net.ParseIP("192.168.0.1"),
		Port:      8333,
	},
	AddrMe: wire.NetAddress{
		Timestamp: time.Time{}, // Zero value -- no timestamp in version
		Services:  wire.SFNodeNetwork,
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
