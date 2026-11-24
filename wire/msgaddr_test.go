// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"io"
	"net"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
)

// TestAddr tests the MsgAddr API.
func TestAddr(t *testing.T) {
	pver := ProtocolVersion

	// Ensure the command is expected value.
	wantCmd := "addr"
	msg := NewMsgAddr()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgAddr: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Num addresses (varInt) + max allowed addresses.
	wantPayload := uint32(30009)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Ensure NetAddresses are added properly.
	tcpAddr := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	na := NewNetAddress(tcpAddr, SFNodeNetwork)
	err := msg.AddAddress(na)
	if err != nil {
		t.Errorf("AddAddress: %v", err)
	}
	if msg.AddrList[0] != na {
		t.Errorf("AddAddress: wrong address added - got %v, want %v",
			spew.Sprint(msg.AddrList[0]), spew.Sprint(na))
	}

	// Ensure the address list is cleared properly.
	msg.ClearAddresses()
	if len(msg.AddrList) != 0 {
		t.Errorf("ClearAddresses: address list is not empty - "+
			"got %v [%v], want %v", len(msg.AddrList),
			spew.Sprint(msg.AddrList[0]), 0)
	}

	// Ensure adding more than the max allowed addresses per message returns
	// error.
	for i := 0; i < MaxAddrPerMsg+1; i++ {
		err = msg.AddAddress(na)
	}
	if err == nil {
		t.Errorf("AddAddress: expected error on too many addresses " +
			"not received")
	}
	err = msg.AddAddresses(na)
	if err == nil {
		t.Errorf("AddAddresses: expected error on too many addresses " +
			"not received")
	}

	// Ensure max payload is expected value for protocol versions before
	// timestamp was added to NetAddress.
	// Num addresses (varInt) + max allowed addresses.
	pver = NetAddressTimeVersion - 1
	wantPayload = uint32(26009)
	maxPayload = msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Ensure max payload is expected value for protocol versions before
	// multiple addresses were allowed.
	// Num addresses (varInt) + a single net addresses.
	pver = MultipleAddressVersion - 1
	wantPayload = uint32(35)
	maxPayload = msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}
}

// TestAddrEncode checks that encoding an addr message produces the expected
// serialized output with different protocol versions, and numbers of addresses.
func TestAddrEncode(t *testing.T) {
	t.Parallel()

	pver := ProtocolVersion
	enc := BaseEncoding

	ts := time.Unix(0x00000001, 0)
	tsStr := "01000000"

	sf := SFNodeNetwork
	sfStr := "0100000000000000"

	port := uint16(0x0001)
	portStr := "0001"

	// ipv4 host.
	ipv4HostStr := "000000000000000000000000" + "00000001"
	ipv4Host := mustHex(ipv4HostStr)

	// ipv6 v4 mapped host.
	ipv6v4HostStr := "00000000000000000000FFFF00000001"
	ipv6v4Host := mustHex(ipv6v4HostStr)

	// ipv6 host.
	ipv6HostStr := "20010DB8000000000000000000000001"
	ipv6Host := mustHex(ipv6HostStr)

	// ipv6 onionCat torv2 host.
	onionCatPrefix := "FD87D87EEB43"
	ipv6OnionCatHostStr := onionCatPrefix + "00000000000000000001"
	ipv6OnionCatHost := mustHex(ipv6OnionCatHostStr)

	addresses0 := []*NetAddress{
		{
			Timestamp: ts,
			Services:  sf,
			IP:        ipv4Host,
			Port:      port,
		},
		{
			Timestamp: ts,
			Services:  sf,
			IP:        ipv6v4Host,
			Port:      port,
		},
		{
			Timestamp: ts,
			Services:  sf,
			IP:        ipv6Host,
			Port:      port,
		},
		{
			Timestamp: ts,
			Services:  sf,
			IP:        ipv6OnionCatHost,
			Port:      port,
		},
	}

	msgEncoded0 := mustHexAppend(
		"03", // addr count

		tsStr,
		sfStr,
		ipv4HostStr,
		portStr,

		tsStr,
		sfStr,
		ipv6v4HostStr,
		portStr,

		tsStr,
		sfStr,
		ipv6HostStr,
		portStr,

		// onionCat torv2 must be skipped (BIP-0155).
	)

	tests := []struct {
		name        string
		msg         *MsgAddr
		wantEncoded []byte
		pver        uint32
	}{
		{
			"latest protocol version with no addresses",
			&MsgAddr{},
			mustHex("00"), // length of zero.
			pver,
		},
		{
			"skip encoding torv2 addr between multiple addresses",
			&MsgAddr{AddrList: addresses0},
			msgEncoded0,
			pver,
		},
		{
			"protocol version MultipleAddressVersion-1 with no " +
				"addresses",
			&MsgAddr{},
			mustHex("00"), // length of zero.
			MultipleAddressVersion - 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var buf bytes.Buffer

			err := test.msg.BtcEncode(&buf, test.pver, enc)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !bytes.Equal(test.wantEncoded, buf.Bytes()) {
				t.Fatalf("bad encoding; \nwant: %x\n got: %x",
					test.wantEncoded, buf.Bytes())
			}
		})
	}
}

// TestAddrDecode checks that decoding an addr message from the wire results
// in the expected parsed data, with different protocol versions, and numbers of
// addresses.
func TestAddrDecode(t *testing.T) {
	t.Parallel()

	pver := ProtocolVersion
	enc := BaseEncoding

	ts := time.Unix(0x00000001, 0)
	tsStr := "01000000"

	sf := SFNodeNetwork
	sfStr := "0100000000000000"

	port := uint16(0x0001)
	portStr := "0001"

	// ipv4 host.
	ipv4HostStr := "000000000000000000000000" + "00000001"
	ipv4Host := mustHex(ipv4HostStr)

	// ipv6 v4 mapped host.
	ipv6v4HostStr := "00000000000000000000FFFF00000001"
	ipv6v4Host := mustHex(ipv6v4HostStr)

	// ipv6 host.
	ipv6HostStr := "20010DB8000000000000000000000001"
	ipv6Host := mustHex(ipv6HostStr)

	// ipv6 onionCat torv2 host.
	onionCatPrefix := "FD87D87EEB43"
	ipv6OnionCatHostStr := onionCatPrefix + "00000000000000000001"

	msgEncoded0 := mustHexAppend(
		"04", // addr count

		tsStr,
		sfStr,
		ipv4HostStr,
		portStr,

		tsStr,
		sfStr,
		ipv6v4HostStr,
		portStr,

		tsStr,
		sfStr,
		ipv6HostStr,
		portStr,

		tsStr,
		sfStr,
		ipv6OnionCatHostStr,
		portStr,
	)

	addresses0 := []*NetAddress{
		{
			Timestamp: ts,
			Services:  sf,
			IP:        ipv4Host,
			Port:      port,
		},
		{
			Timestamp: ts,
			Services:  sf,
			IP:        ipv6v4Host,
			Port:      port,
		},
		{
			Timestamp: ts,
			Services:  sf,
			IP:        ipv6Host,
			Port:      port,
		},

		// torv2 addresses must be skipped (BIP-0155).
	}

	tests := []struct {
		name    string
		encoded []byte
		wantMsg *MsgAddr
		pver    uint32
	}{
		{
			"latest protocol version with no addresses",
			mustHex("00"), // zero addresses.
			&MsgAddr{AddrList: []*NetAddress{}},
			pver,
		},
		{
			"skip decoding torv2 addr between multiple addresses",
			msgEncoded0,
			&MsgAddr{AddrList: addresses0},
			pver,
		},
		{
			"protocol version MultipleAddressVersion-1 with no " +
				"addresses",
			mustHex("00"), // zero addresses.
			&MsgAddr{AddrList: []*NetAddress{}},
			MultipleAddressVersion - 1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var msg MsgAddr
			err := msg.BtcDecode(
				bytes.NewReader(test.encoded), test.pver, enc,
			)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}

			if !reflect.DeepEqual(test.wantMsg, &msg) {
				t.Fatalf("unexpected msg: \nwant:\n%s\ngot:"+
					"\n%s", spew.Sdump(test.wantMsg),
					spew.Sdump(&msg))
			}
		})
	}
}

// TestAddrWireErrors performs negative tests against wire encode and decode
// of MsgAddr to confirm error paths work correctly.
func TestAddrWireErrors(t *testing.T) {
	pver := ProtocolVersion
	pverMA := MultipleAddressVersion
	wireErr := &MessageError{}

	// A couple of NetAddresses to use for testing.
	na := &NetAddress{
		Timestamp: time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	}
	na2 := &NetAddress{
		Timestamp: time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("192.168.0.1"),
		Port:      8334,
	}

	// Address message with multiple addresses.
	baseAddr := NewMsgAddr()
	baseAddr.AddAddresses(na, na2)
	baseAddrEncoded := []byte{
		0x02,                   // Varint for number of addresses
		0x29, 0xab, 0x5f, 0x49, // Timestamp
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01, // IP 127.0.0.1
		0x20, 0x8d, // Port 8333 in big-endian
		0x29, 0xab, 0x5f, 0x49, // Timestamp
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xff, 0xff, 0xc0, 0xa8, 0x00, 0x01, // IP 192.168.0.1
		0x20, 0x8e, // Port 8334 in big-endian

	}

	// Message that forces an error by having more than the max allowed
	// addresses.
	maxAddr := NewMsgAddr()
	for i := 0; i < MaxAddrPerMsg; i++ {
		maxAddr.AddAddress(na)
	}
	maxAddr.AddrList = append(maxAddr.AddrList, na)
	maxAddrEncoded := []byte{
		0xfd, 0x03, 0xe9, // Varint for number of addresses (1001)
	}

	tests := []struct {
		in       *MsgAddr        // Value to encode
		buf      []byte          // Wire encoding
		pver     uint32          // Protocol version for wire encoding
		enc      MessageEncoding // Message encoding format
		max      int             // Max size of fixed buffer to induce errors
		writeErr error           // Expected write error
		readErr  error           // Expected read error
	}{
		// Latest protocol version with intentional read/write errors.
		// Force error in addresses count
		{baseAddr, baseAddrEncoded, pver, BaseEncoding, 0, io.ErrShortWrite, io.EOF},
		// Force error in address list.
		{baseAddr, baseAddrEncoded, pver, BaseEncoding, 1, io.ErrShortWrite, io.EOF},
		// Force error with greater than max inventory vectors.
		{maxAddr, maxAddrEncoded, pver, BaseEncoding, 3, wireErr, wireErr},
		// Force error with greater than max inventory vectors for
		// protocol versions before multiple addresses were allowed.
		{maxAddr, maxAddrEncoded, pverMA - 1, BaseEncoding, 3, wireErr, wireErr},
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
		var msg MsgAddr
		r := newFixedReader(test.max, test.buf)
		err = msg.BtcDecode(r, test.pver, test.enc)
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
