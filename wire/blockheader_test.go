// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/decred/dcrd/chaincfg/chainhash"
)

// TestBlockHeader tests the BlockHeader API.
func TestBlockHeader(t *testing.T) {
	nonce64, err := RandomUint64()
	if err != nil {
		t.Errorf("RandomUint64: Error generating nonce: %v", err)
	}
	nonce := uint32(nonce64)

	hash := mainNetGenesisHash
	merkleHash := mainNetGenesisMerkleRoot
	votebits := uint16(0x0000)
	bits := uint32(0x1d00ffff)
	finalState := [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	voters := uint16(0x0000)
	freshstake := uint8(0x00)
	revocations := uint8(0x00)
	poolsize := uint32(0)
	sbits := int64(0x0000000000000000)
	blockHeight := uint32(0)
	blockSize := uint32(0)
	stakeVersion := uint32(0xb0a710ad)
	extraData := [32]byte{}

	bh := NewBlockHeader(
		1, // verision
		&hash,
		&merkleHash,
		&merkleHash, // stakeRoot
		votebits,
		finalState,
		voters,
		freshstake,
		revocations,
		poolsize,
		bits,
		sbits,
		blockHeight,
		blockSize,
		nonce,
		extraData,
		stakeVersion,
	)

	// Ensure we get the same data back out.
	if !bh.PrevBlock.IsEqual(&hash) {
		t.Errorf("NewBlockHeader: wrong prev hash - got %v, want %v",
			spew.Sprint(bh.PrevBlock), spew.Sprint(hash))
	}
	if !bh.MerkleRoot.IsEqual(&merkleHash) {
		t.Errorf("NewBlockHeader: wrong merkle root - got %v, want %v",
			spew.Sprint(bh.MerkleRoot), spew.Sprint(merkleHash))
	}
	if !bh.StakeRoot.IsEqual(&merkleHash) {
		t.Errorf("NewBlockHeader: wrong merkle root - got %v, want %v",
			spew.Sprint(bh.MerkleRoot), spew.Sprint(merkleHash))
	}
	if bh.VoteBits != votebits {
		t.Errorf("NewBlockHeader: wrong bits - got %v, want %v",
			bh.Bits, bits)
	}
	if bh.FinalState != finalState {
		t.Errorf("NewBlockHeader: wrong bits - got %v, want %v",
			bh.Bits, bits)
	}
	if bh.Voters != voters {
		t.Errorf("NewBlockHeader: wrong bits - got %v, want %v",
			bh.Bits, bits)
	}
	if bh.FreshStake != freshstake {
		t.Errorf("NewBlockHeader: wrong bits - got %v, want %v",
			bh.Bits, bits)
	}
	if bh.Revocations != revocations {
		t.Errorf("NewBlockHeader: wrong bits - got %v, want %v",
			bh.Bits, bits)
	}
	if bh.PoolSize != poolsize {
		t.Errorf("NewBlockHeader: wrong PoolSize - got %v, want %v",
			bh.PoolSize, poolsize)
	}
	if bh.Bits != bits {
		t.Errorf("NewBlockHeader: wrong bits - got %v, want %v",
			bh.Bits, bits)
	}
	if bh.SBits != sbits {
		t.Errorf("NewBlockHeader: wrong bits - got %v, want %v",
			bh.Bits, bits)
	}
	if bh.Nonce != nonce {
		t.Errorf("NewBlockHeader: wrong nonce - got %v, want %v",
			bh.Nonce, nonce)
	}
	if bh.StakeVersion != stakeVersion {
		t.Errorf("NewBlockHeader: wrong stakeVersion - got %v, want %v",
			bh.StakeVersion, stakeVersion)
	}
}

// TestBlockHeaderWire tests the BlockHeader wire encode and decode for various
// protocol versions.
func TestBlockHeaderWire(t *testing.T) {
	nonce := uint32(123123) // 0x1e0f3
	pver := uint32(70001)

	// baseBlockHdr is used in the various tests as a baseline BlockHeader.
	bits := uint32(0x1d00ffff)
	baseBlockHdr := &BlockHeader{
		Version:      1,
		PrevBlock:    mainNetGenesisHash,
		MerkleRoot:   mainNetGenesisMerkleRoot,
		StakeRoot:    mainNetGenesisMerkleRoot,
		VoteBits:     uint16(0x0000),
		FinalState:   [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Voters:       uint16(0x0000),
		FreshStake:   uint8(0x00),
		Revocations:  uint8(0x00),
		PoolSize:     uint32(0x00000000),
		Timestamp:    time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Bits:         bits,
		SBits:        int64(0x0000000000000000),
		Nonce:        nonce,
		StakeVersion: uint32(0x0ddba110),
		Height:       uint32(0),
		Size:         uint32(0),
	}

	// baseBlockHdrEncoded is the wire encoded bytes of baseBlockHdr.
	baseBlockHdrEncoded := []byte{
		0x01, 0x00, 0x00, 0x00, // Version 1
		0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
		0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
		0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
		0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // PrevBlock
		0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
		0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
		0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
		0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a, // MerkleRoot
		0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
		0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
		0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
		0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a, // StakeRoot
		0x00, 0x00, // VoteBits
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // FinalState
		0x00, 0x00, // Voters
		0x00,                   // FreshStake
		0x00,                   // Revocations
		0x00, 0x00, 0x00, 0x00, //Poolsize
		0xff, 0xff, 0x00, 0x1d, // Bits
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SBits
		0x00, 0x00, 0x00, 0x00, // Height
		0x00, 0x00, 0x00, 0x00, // Size
		0x29, 0xab, 0x5f, 0x49, // Timestamp
		0xf3, 0xe0, 0x01, 0x00, // Nonce
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ExtraData
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0xa1, 0xdb, 0x0d, // StakeVersion
	}

	tests := []struct {
		in   *BlockHeader // Data to encode
		out  *BlockHeader // Expected decoded data
		buf  []byte       // Wire encoding
		pver uint32       // Protocol version for wire encoding
	}{
		// Latest protocol version.
		{
			baseBlockHdr,
			baseBlockHdr,
			baseBlockHdrEncoded,
			ProtocolVersion,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		// Former test (doesn't work because of capacity error)
		buf := bytes.NewBuffer(make([]byte, 0, MaxBlockHeaderPayload))
		err := writeBlockHeader(buf, test.pver, test.in)
		if err != nil {
			t.Errorf("writeBlockHeader #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("writeBlockHeader #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		buf.Reset()
		err = test.in.BtcEncode(buf, pver)
		if err != nil {
			t.Errorf("BtcEncode #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("BtcEncode #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode the block header from wire format.
		var bh BlockHeader
		rbuf := bytes.NewReader(test.buf)
		err = readBlockHeader(rbuf, test.pver, &bh)
		if err != nil {
			t.Errorf("readBlockHeader #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&bh, test.out) {
			t.Errorf("readBlockHeader #%d\n got: %s want: %s", i,
				spew.Sdump(&bh), spew.Sdump(test.out))
			continue
		}

		rbuf = bytes.NewReader(test.buf)
		err = bh.BtcDecode(rbuf, pver)
		if err != nil {
			t.Errorf("BtcDecode #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&bh, test.out) {
			t.Errorf("BtcDecode #%d\n got: %s want: %s", i,
				spew.Sdump(&bh), spew.Sdump(test.out))
			continue
		}
	}
}

// TestBlockHeaderSerialize tests BlockHeader serialize and deserialize.
func TestBlockHeaderSerialize(t *testing.T) {
	nonce := uint32(123123) // 0x1e0f3

	// baseBlockHdr is used in the various tests as a baseline BlockHeader.
	bits := uint32(0x1d00ffff)
	baseBlockHdr := &BlockHeader{
		Version:      1,
		PrevBlock:    mainNetGenesisHash,
		MerkleRoot:   mainNetGenesisMerkleRoot,
		StakeRoot:    mainNetGenesisMerkleRoot,
		VoteBits:     uint16(0x0000),
		FinalState:   [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Voters:       uint16(0x0000),
		FreshStake:   uint8(0x00),
		Revocations:  uint8(0x00),
		Timestamp:    time.Unix(0x495fab29, 0), // 2009-01-03 12:15:05 -0600 CST
		Bits:         bits,
		SBits:        int64(0x0000000000000000),
		Nonce:        nonce,
		StakeVersion: uint32(0x0ddba110),
		Height:       uint32(0),
		Size:         uint32(0),
	}

	// baseBlockHdrEncoded is the wire encoded bytes of baseBlockHdr.
	baseBlockHdrEncoded := []byte{
		0x01, 0x00, 0x00, 0x00, // Version 1
		0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
		0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
		0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
		0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // PrevBlock
		0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
		0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
		0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
		0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a, // MerkleRoot
		0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
		0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
		0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
		0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a, // StakeRoot
		0x00, 0x00, // VoteBits
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // FinalState
		0x00, 0x00, // Voters
		0x00,                   // FreshStake
		0x00,                   // Revocations
		0x00, 0x00, 0x00, 0x00, //Poolsize
		0xff, 0xff, 0x00, 0x1d, // Bits
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // SBits
		0x00, 0x00, 0x00, 0x00, // Height
		0x00, 0x00, 0x00, 0x00, // Size
		0x29, 0xab, 0x5f, 0x49, // Timestamp
		0xf3, 0xe0, 0x01, 0x00, // Nonce
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // ExtraData
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x10, 0xa1, 0xdb, 0x0d, // StakeVersion
	}

	tests := []struct {
		in  *BlockHeader // Data to encode
		out *BlockHeader // Expected decoded data
		buf []byte       // Serialized data
	}{
		{
			baseBlockHdr,
			baseBlockHdr,
			baseBlockHdrEncoded,
		},
	}

	t.Logf("Running %d tests", len(tests))
	buf := bytes.NewBuffer(make([]byte, 0, MaxBlockHeaderPayload))
	for i, test := range tests {
		// Clear existing contents.
		buf.Reset()

		// Serialize the block header.
		err := test.in.Serialize(buf)
		if err != nil {
			t.Errorf("Serialize #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("Serialize #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Deserialize the block header.
		var bh BlockHeader
		rbuf := bytes.NewReader(test.buf)
		err = bh.Deserialize(rbuf)
		if err != nil {
			t.Errorf("Deserialize #%d error %v", i, err)
			continue
		}
		if !reflect.DeepEqual(&bh, test.out) {
			t.Errorf("Deserialize #%d\n got: %s want: %s", i,
				spew.Sdump(&bh), spew.Sdump(test.out))
			continue
		}
	}
}

func TestBlockHeaderHashing(t *testing.T) {
	dummyHeader := "0000000049e0b48ade043f729d60095ed92642d96096fe6aba42f2eda" +
		"632d461591a152267dc840ff27602ce1968a81eb30a43423517207617a0150b56c4f72" +
		"b803e497f00000000000000000000000000000000000000000000000000000000000000" +
		"00010000000000000000000000b7000000ffff7f20204e0000000000005800000060010" +
		"0008b990956000000000000000000000000000000000000000000000000000000000000" +
		"0000000000000000ABCD"
	// This hash has reversed endianness compared to what chainhash spits out.
	hashStr := "0d40d58703482d81d711be0ffc1b313788d3c3937e1617e4876661d33a8c4c41"
	hashB, _ := hex.DecodeString(hashStr)
	hash, _ := chainhash.NewHash(hashB)

	vecH, _ := hex.DecodeString(dummyHeader)
	r := bytes.NewReader(vecH)
	var bh BlockHeader
	bh.Deserialize(r)
	hash2 := bh.BlockHash()

	if !hash2.IsEqual(hash) {
		t.Errorf("wrong block hash returned (want %v, got %v)", hash,
			hash2)
	}
}
