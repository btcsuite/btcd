// Copyright (c) 2013-2015 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire_test

import (
	"bytes"
	"testing"

	"github.com/btcsuite/btcd/wire"
)

func TestMemPool(t *testing.T) {
	pver := wire.ProtocolVersion

	// Ensure the command is expected value.
	wantCmd := "mempool"
	msg := wire.NewMsgMemPool()
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgMemPool: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value.
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
		t.Errorf("encode of MsgMemPool failed %v err <%v>", msg, err)
	}

	// Older protocol versions should fail encode since message didn't
	// exist yet.
	oldPver := wire.BIP0035Version - 1
	err = msg.BtcEncode(&buf, oldPver)
	if err == nil {
		s := "encode of MsgMemPool passed for old protocol version %v err <%v>"
		t.Errorf(s, msg, err)
	}

	// Test decode with latest protocol version.
	readmsg := wire.NewMsgMemPool()
	err = readmsg.BtcDecode(&buf, pver)
	if err != nil {
		t.Errorf("decode of MsgMemPool failed [%v] err <%v>", buf, err)
	}

	// Older protocol versions should fail decode since message didn't
	// exist yet.
	err = readmsg.BtcDecode(&buf, oldPver)
	if err == nil {
		s := "decode of MsgMemPool passed for old protocol version %v err <%v>"
		t.Errorf(s, msg, err)
	}

	return
}
