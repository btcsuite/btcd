// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"bytes"
	"github.com/conformal/btcwire"
	"github.com/davecgh/go-spew/spew"
	"net"
	"reflect"
	"testing"
	"time"
)

// TestNetAddress tests the NetAddress API.
func TestNetAddress(t *testing.T) {
	ip := net.ParseIP("127.0.0.1")
	port := 8333

	// Test NewNetAddress.
	tcpAddr := &net.TCPAddr{
		IP:   ip,
		Port: port,
	}
	na, err := btcwire.NewNetAddress(tcpAddr, 0)
	if err != nil {
		t.Errorf("NewNetAddress: %v", err)
	}

	// Ensure we get the same ip, port, and services back out.
	if !na.IP.Equal(ip) {
		t.Errorf("NetNetAddress: wrong ip - got %v, want %v", na.IP, ip)
	}
	if na.Port != uint16(port) {
		t.Errorf("NetNetAddress: wrong port - got %v, want %v", na.Port,
			port)
	}
	if na.Services != 0 {
		t.Errorf("NetNetAddress: wrong services - got %v, want %v",
			na.Services, 0)
	}
	if na.HasService(btcwire.SFNodeNetwork) {
		t.Errorf("HasService: SFNodeNetwork service is set")
	}

	// Ensure adding the full service node flag works.
	na.AddService(btcwire.SFNodeNetwork)
	if na.Services != btcwire.SFNodeNetwork {
		t.Errorf("AddService: wrong services - got %v, want %v",
			na.Services, btcwire.SFNodeNetwork)
	}
	if !na.HasService(btcwire.SFNodeNetwork) {
		t.Errorf("HasService: SFNodeNetwork service not set")
	}

	// Ensure max payload is expected value for latest protocol version.
	pver := btcwire.ProtocolVersion
	wantPayload := uint32(30)
	maxPayload := btcwire.TstMaxNetAddressPayload(btcwire.ProtocolVersion)
	if maxPayload != wantPayload {
		t.Errorf("maxNetAddressPayload: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Protocol version before NetAddressTimeVersion when timestamp was
	// added.  Ensure max payload is expected value for it.
	pver = btcwire.NetAddressTimeVersion - 1
	wantPayload = 26
	maxPayload = btcwire.TstMaxNetAddressPayload(pver)
	if maxPayload != wantPayload {
		t.Errorf("maxNetAddressPayload: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Check for expected failure on wrong address type.
	udpAddr := &net.UDPAddr{}
	_, err = btcwire.NewNetAddress(udpAddr, 0)
	if err != btcwire.ErrInvalidNetAddr {
		t.Errorf("NewNetAddress: expected error not received - "+
			"got %v, want %v", err, btcwire.ErrInvalidNetAddr)
	}
}

// TestNetAddressWire tests the NetAddress wire encode and decode for various
// protocol versions and timestamp flag combinations.
func TestNetAddressWire(t *testing.T) {
	// baseNetAddr is used in the various tests as a baseline NetAddress.
	baseNetAddr := btcwire.NetAddress{
		Timestamp: time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Services:  btcwire.SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	}

	// baseNetAddrNoTS is baseNetAddr with a zero value for the timestamp.
	baseNetAddrNoTS := baseNetAddr
	baseNetAddrNoTS.Timestamp = time.Time{}

	// baseNetAddrEncoded is the wire encoded bytes of baseNetAddr.
	baseNetAddrEncoded := []byte{
		0x29, 0xab, 0x5f, 0x49, // Timestamp
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01, // IP 127.0.0.1
		0x20, 0x8d, // Port 8333 in big-endian
	}

	// baseNetAddrNoTSEncoded is the wire encoded bytes of baseNetAddrNoTS.
	baseNetAddrNoTSEncoded := []byte{
		// No timestamp
		0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SFNodeNetwork
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x01, // IP 127.0.0.1
		0x20, 0x8d, // Port 8333 in big-endian
	}

	tests := []struct {
		in   btcwire.NetAddress // NetAddress to encode
		out  btcwire.NetAddress // Expected decoded NetAddress
		ts   bool               // Include timestamp?
		buf  []byte             // Wire encoding
		pver uint32             // Protocol version for wire encoding
	}{
		// Latest protocol version without ts flag.
		{
			baseNetAddr,
			baseNetAddrNoTS,
			false,
			baseNetAddrNoTSEncoded,
			btcwire.ProtocolVersion,
		},

		// Latest protocol version with ts flag.
		{
			baseNetAddr,
			baseNetAddr,
			true,
			baseNetAddrEncoded,
			btcwire.ProtocolVersion,
		},

		// Protocol version NetAddressTimeVersion without ts flag.
		{
			baseNetAddr,
			baseNetAddrNoTS,
			false,
			baseNetAddrNoTSEncoded,
			btcwire.NetAddressTimeVersion,
		},

		// Protocol version NetAddressTimeVersion with ts flag.
		{
			baseNetAddr,
			baseNetAddr,
			true,
			baseNetAddrEncoded,
			btcwire.NetAddressTimeVersion,
		},

		// Protocol version NetAddressTimeVersion-1 without ts flag.
		{
			baseNetAddr,
			baseNetAddrNoTS,
			false,
			baseNetAddrNoTSEncoded,
			btcwire.NetAddressTimeVersion - 1,
		},

		// Protocol version NetAddressTimeVersion-1 with timestamp.
		// Even though the timestamp flag is set, this shouldn't have a
		// timestamp since it is a protocol version before it was
		// added.
		{
			baseNetAddr,
			baseNetAddrNoTS,
			true,
			baseNetAddrNoTSEncoded,
			btcwire.NetAddressTimeVersion - 1,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		var buf bytes.Buffer
		err := btcwire.TstWriteNetAddress(&buf, test.pver, &test.in, test.ts)
		if err != nil {
			t.Errorf("writeNetAddress #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("writeNetAddress #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode the message from wire format.
		var na btcwire.NetAddress
		rbuf := bytes.NewBuffer(test.buf)
		err = btcwire.TstReadNetAddress(rbuf, test.pver, &na, test.ts)
		if err != nil {
			t.Errorf("readNetAddress #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(na, test.out) {
			t.Errorf("readNetAddress #%d\n got: %s want: %s", i,
				spew.Sdump(na), spew.Sdump(test.out))
			continue
		}
	}
}
