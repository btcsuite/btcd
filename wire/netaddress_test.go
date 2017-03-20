// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
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

// TestNetAddress tests the NetAddress API.
func TestNetAddress(t *testing.T) {
	ip := net.ParseIP("127.0.0.1")
	port := 8333

	// Test NewNetAddress.
	tcpAddr := &net.TCPAddr{
		IP:   ip,
		Port: port,
	}
	na, err := NewNetAddress(tcpAddr, 0)
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
	if na.HasService(SFNodeNetwork) {
		t.Errorf("HasService: SFNodeNetwork service is set")
	}

	// Ensure adding the full service node flag works.
	na.AddService(SFNodeNetwork)
	if na.Services != SFNodeNetwork {
		t.Errorf("AddService: wrong services - got %v, want %v",
			na.Services, SFNodeNetwork)
	}
	if !na.HasService(SFNodeNetwork) {
		t.Errorf("HasService: SFNodeNetwork service not set")
	}

	// Ensure max payload is expected value for latest protocol version.
	pver := ProtocolVersion
	wantPayload := uint32(30)
	maxPayload := maxNetAddressPayload(ProtocolVersion)
	if maxPayload != wantPayload {
		t.Errorf("maxNetAddressPayload: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Check for expected failure on wrong address type.
	udpAddr := &net.UDPAddr{}
	_, err = NewNetAddress(udpAddr, 0)
	if err != ErrInvalidNetAddr {
		t.Errorf("NewNetAddress: expected error not received - "+
			"got %v, want %v", err, ErrInvalidNetAddr)
	}
}

// TestNetAddressWire tests the NetAddress wire encode and decode for various
// protocol versions and timestamp flag combinations.
func TestNetAddressWire(t *testing.T) {
	// baseNetAddr is used in the various tests as a baseline NetAddress.
	baseNetAddr := NetAddress{
		Timestamp: time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Services:  SFNodeNetwork,
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
		in   NetAddress // NetAddress to encode
		out  NetAddress // Expected decoded NetAddress
		ts   bool       // Include timestamp?
		buf  []byte     // Wire encoding
		pver uint32     // Protocol version for wire encoding
	}{
		// Latest protocol version without ts flag.
		{
			baseNetAddr,
			baseNetAddrNoTS,
			false,
			baseNetAddrNoTSEncoded,
			ProtocolVersion,
		},

		// Latest protocol version with ts flag.
		{
			baseNetAddr,
			baseNetAddr,
			true,
			baseNetAddrEncoded,
			ProtocolVersion,
		},
	}

	t.Logf("Running %d tests", len(tests))
	var buf bytes.Buffer
	for i, test := range tests {
		buf.Reset()
		// Encode to wire format.
		err := writeNetAddress(&buf, test.pver, &test.in, test.ts)
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
		var na NetAddress
		rbuf := bytes.NewReader(test.buf)
		err = readNetAddress(rbuf, test.pver, &na, test.ts)
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

// TestNetAddressWireErrors performs negative tests against wire encode and
// decode NetAddress to confirm error paths work correctly.
func TestNetAddressWireErrors(t *testing.T) {
	pver := ProtocolVersion

	// baseNetAddr is used in the various tests as a baseline NetAddress.
	baseNetAddr := NetAddress{
		Timestamp: time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Services:  SFNodeNetwork,
		IP:        net.ParseIP("127.0.0.1"),
		Port:      8333,
	}

	tests := []struct {
		in       *NetAddress // Value to encode
		buf      []byte      // Wire encoding
		pver     uint32      // Protocol version for wire encoding
		ts       bool        // Include timestamp flag
		max      int         // Max size of fixed buffer to induce errors
		writeErr error       // Expected write error
		readErr  error       // Expected read error
	}{
		// Latest protocol version with timestamp and intentional
		// read/write errors.
		// Force errors on timestamp.
		{&baseNetAddr, []byte{}, pver, true, 0, io.ErrShortWrite, io.EOF},
		// Force errors on services.
		{&baseNetAddr, []byte{}, pver, true, 4, io.ErrShortWrite, io.EOF},
		// Force errors on ip.
		{&baseNetAddr, []byte{}, pver, true, 12, io.ErrShortWrite, io.EOF},
		// Force errors on port.
		{&baseNetAddr, []byte{}, pver, true, 28, io.ErrShortWrite, io.EOF},

		// Latest protocol version with no timestamp and intentional
		// read/write errors.
		// Force errors on services.
		{&baseNetAddr, []byte{}, pver, false, 0, io.ErrShortWrite, io.EOF},
		// Force errors on ip.
		{&baseNetAddr, []byte{}, pver, false, 8, io.ErrShortWrite, io.EOF},
		// Force errors on port.
		{&baseNetAddr, []byte{}, pver, false, 24, io.ErrShortWrite, io.EOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := writeNetAddress(w, test.pver, test.in, test.ts)
		if err != test.writeErr {
			t.Errorf("writeNetAddress #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Decode from wire format.
		var na NetAddress
		r := newFixedReader(test.max, test.buf)
		err = readNetAddress(r, test.pver, &na, test.ts)
		if err != test.readErr {
			t.Errorf("readNetAddress #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}
