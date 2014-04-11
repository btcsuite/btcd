// Copyright (c) 2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"bytes"
	"crypto/rand"
	"github.com/conformal/btcwire"
	"testing"
)

// TestMerkleBlock tests the MsgMerkleBlock API.
func TestMerkleBlock(t *testing.T) {
	pver := btcwire.ProtocolVersion
	pverOld := btcwire.BIP0037Version - 1

	// Block 1 header.
	prevHash := &blockOne.Header.PrevBlock
	merkleHash := &blockOne.Header.MerkleRoot
	bits := blockOne.Header.Bits
	nonce := blockOne.Header.Nonce
	bh := btcwire.NewBlockHeader(prevHash, merkleHash, bits, nonce)

	// Ensure the command is expected value.
	wantCmd := "merkleblock"
	msg := btcwire.NewMsgMerkleBlock(bh)
	if cmd := msg.Command(); cmd != wantCmd {
		t.Errorf("NewMsgBlock: wrong command - got %v want %v",
			cmd, wantCmd)
	}

	// Ensure max payload is expected value for latest protocol version.
	// Num addresses (varInt) + max allowed addresses.
	wantPayload := uint32(1000000)
	maxPayload := msg.MaxPayloadLength(pver)
	if maxPayload != wantPayload {
		t.Errorf("MaxPayloadLength: wrong max payload length for "+
			"protocol version %d - got %v, want %v", pver,
			maxPayload, wantPayload)
	}

	// Load maxTxPerBlock hashes
	data := make([]byte, 32)
	for i := 0; i < (btcwire.MaxBlockPayload/10)+1; i++ {
		rand.Read(data)
		hash, err := btcwire.NewShaHash(data)
		if err != nil {
			t.Errorf("NewShaHash failed: %v\n", err)
			return
		}

		if err = msg.AddTxHash(hash); err != nil {
			t.Errorf("AddTxHash failed: %v\n", err)
			return
		}
	}

	// Add one more Tx to test failure
	rand.Read(data)
	hash, err := btcwire.NewShaHash(data)
	if err != nil {
		t.Errorf("NewShaHash failed: %v\n", err)
		return
	}

	if err = msg.AddTxHash(hash); err == nil {
		t.Errorf("AddTxHash succeeded when it should have failed")
		return
	}

	// Test encode with latest protocol version.
	var buf bytes.Buffer
	err = msg.BtcEncode(&buf, pver)
	if err != nil {
		t.Errorf("encode of MsgMerkleBlock failed %v err <%v>", msg, err)
	}

	// Test encode with old protocol version.
	if err = msg.BtcEncode(&buf, pverOld); err == nil {
		t.Errorf("encode of MsgMerkleBlock succeeded with old protocol " +
			"version when it should have failed")
		return
	}

	// Test decode with latest protocol version.
	readmsg := btcwire.MsgMerkleBlock{}
	err = readmsg.BtcDecode(&buf, pver)
	if err != nil {
		t.Errorf("decode of MsgMerkleBlock failed [%v] err <%v>", buf, err)
	}

	// Test decode with old protocol version.
	if err = readmsg.BtcDecode(&buf, pverOld); err == nil {
		t.Errorf("decode of MsgMerkleBlock successed with old protocol " +
			"version when it should have failed")
		return
	}

	// Force extra hash to test maxTxPerBlock
	msg.Hashes = append(msg.Hashes, hash)
	err = msg.BtcEncode(&buf, pver)
	if err == nil {
		t.Errorf("encode of MsgMerkleBlock succeeded with too many tx hashes " +
			"when it should have failed")
		return
	}
}
