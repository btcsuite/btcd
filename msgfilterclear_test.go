// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"bytes"
	"github.com/conformal/btcwire"
	"testing"
)

// TestFilterCLearLatest tests the MsgFilterClear API against the latest protocol version.
func TestFilterClearLatest(t *testing.T) {
	pver := btcwire.ProtocolVersion

	msg := btcwire.NewMsgFilterClear()

	// Ensure the command is expected value.
	wantCmd := "filterclear"
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgFilterClear: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	wantPayload := uint32(0)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, pver)
	if err != nil {
		t.Errorf("encode of MsgFilterClear failed %v err <%v>", msg, err)
	}

	// Test decode with latest protocol version.
	readmsg := btcwire.NewMsgFilterClear()
	err = readmsg.BtcDecode(&buf, pver)
	if err != nil {
		t.Errorf("decode of MsgFilterClear failed [%v] err <%v>", buf, err)
	}

	return
}

// TestFilterClearCrossProtocol tests the MsgFilterClear API when encoding with the latest
// protocol version and decoded with BIP0031Version.
func TestFilterClearCrossProtocol(t *testing.T) {
	msg := btcwire.NewMsgFilterClear()

	// Encode with old protocol version.
	var buf bytes.Buffer
	err := msg.BtcEncode(&buf, btcwire.BIP0037Version-1)
	if err == nil {
		t.Errorf("encode of MsgFilterClear succeeded when it shouldn't have %v",
			msg)
	}

	// Decode with old protocol version.
	readmsg := btcwire.NewMsgFilterClear()
	err = readmsg.BtcDecode(&buf, btcwire.BIP0031Version)
	if err == nil {
		t.Errorf("decode of MsgFilterClear succeeded when it shouldn't have %v",
			msg)
	}
}
