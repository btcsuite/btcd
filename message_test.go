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

// TestMessage tests the Read/WriteMessage API.
func TestMessage(t *testing.T) {
	pver := btcwire.ProtocolVersion

	// Create the various types of messages to test.

	// MsgVersion.
	addrYou := &net.TCPAddr{IP: net.ParseIP("192.168.0.1"), Port: 8333}
	you, err := btcwire.NewNetAddress(addrYou, btcwire.SFNodeNetwork)
	if err != nil {
		t.Errorf("NewNetAddress: %v", err)
	}
	you.Timestamp = time.Time{} // Version message has zero value timestamp.
	addrMe := &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8333}
	me, err := btcwire.NewNetAddress(addrMe, btcwire.SFNodeNetwork)
	if err != nil {
		t.Errorf("NewNetAddress: %v", err)
	}
	me.Timestamp = time.Time{} // Version message has zero value timestamp.
	msgVersion := btcwire.NewMsgVersion(me, you, 123123, "/test:0.0.1/", 0)

	msgVerack := btcwire.NewMsgVerAck()
	msgGetAddr := btcwire.NewMsgGetAddr()
	msgAddr := btcwire.NewMsgAddr()
	msgGetBlocks := btcwire.NewMsgGetBlocks(&btcwire.ShaHash{})
	msgBlock := &blockOne
	msgInv := btcwire.NewMsgInv()
	msgGetData := btcwire.NewMsgGetData()
	msgNotFound := btcwire.NewMsgNotFound()
	msgTx := btcwire.NewMsgTx()
	msgPing := btcwire.NewMsgPing(123123)
	msgPong := btcwire.NewMsgPong(123123)
	msgGetHeaders := btcwire.NewMsgGetHeaders()
	msgHeaders := btcwire.NewMsgHeaders()

	tests := []struct {
		in     btcwire.Message    // Value to encode
		out    btcwire.Message    // Expected decoded value
		pver   uint32             // Protocol version for wire encoding
		btcnet btcwire.BitcoinNet // Network to use for wire encoding
	}{
		{msgVersion, msgVersion, pver, btcwire.MainNet},
		{msgVerack, msgVerack, pver, btcwire.MainNet},
		{msgGetAddr, msgGetAddr, pver, btcwire.MainNet},
		{msgAddr, msgAddr, pver, btcwire.MainNet},
		{msgGetBlocks, msgGetBlocks, pver, btcwire.MainNet},
		{msgBlock, msgBlock, pver, btcwire.MainNet},
		{msgInv, msgInv, pver, btcwire.MainNet},
		{msgGetData, msgGetData, pver, btcwire.MainNet},
		{msgNotFound, msgNotFound, pver, btcwire.MainNet},
		{msgTx, msgTx, pver, btcwire.MainNet},
		{msgPing, msgPing, pver, btcwire.MainNet},
		{msgPong, msgPong, pver, btcwire.MainNet},
		{msgGetHeaders, msgGetHeaders, pver, btcwire.MainNet},
		{msgHeaders, msgHeaders, pver, btcwire.MainNet},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		var buf bytes.Buffer
		err := btcwire.WriteMessage(&buf, test.in, test.pver, test.btcnet)
		if err != nil {
			t.Errorf("WriteMessage #%d error %v", i, err)
			continue
		}

		// Decode from wire format.
		rbuf := bytes.NewBuffer(buf.Bytes())
		msg, _, err := btcwire.ReadMessage(rbuf, test.pver, test.btcnet)
		if err != nil {
			t.Errorf("ReadMessage #%d error %v, msg %v", i, err,
				spew.Sdump(msg))
			continue
		}
		if !reflect.DeepEqual(msg, test.out) {
			t.Errorf("ReadMessage #%d\n got: %v want: %v", i,
				spew.Sdump(msg), spew.Sdump(test.out))
			continue
		}
	}
}
