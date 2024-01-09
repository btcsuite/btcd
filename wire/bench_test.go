// Copyright (c) 2013-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package wire

import (
	"bytes"
	"compress/bzip2"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"os"
	"testing"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

// genesisCoinbaseTx is the coinbase transaction for the genesis blocks for
// the main network, regression test network, and test network (version 3).
var genesisCoinbaseTx = MsgTx{
	Version: 1,
	TxIn: []*TxIn{
		{
			PreviousOutPoint: OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			SignatureScript: []byte{
				0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, 0x45, /* |.......E| */
				0x54, 0x68, 0x65, 0x20, 0x54, 0x69, 0x6d, 0x65, /* |The Time| */
				0x73, 0x20, 0x30, 0x33, 0x2f, 0x4a, 0x61, 0x6e, /* |s 03/Jan| */
				0x2f, 0x32, 0x30, 0x30, 0x39, 0x20, 0x43, 0x68, /* |/2009 Ch| */
				0x61, 0x6e, 0x63, 0x65, 0x6c, 0x6c, 0x6f, 0x72, /* |ancellor| */
				0x20, 0x6f, 0x6e, 0x20, 0x62, 0x72, 0x69, 0x6e, /* | on brin| */
				0x6b, 0x20, 0x6f, 0x66, 0x20, 0x73, 0x65, 0x63, /* |k of sec|*/
				0x6f, 0x6e, 0x64, 0x20, 0x62, 0x61, 0x69, 0x6c, /* |ond bail| */
				0x6f, 0x75, 0x74, 0x20, 0x66, 0x6f, 0x72, 0x20, /* |out for |*/
				0x62, 0x61, 0x6e, 0x6b, 0x73, /* |banks| */
			},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*TxOut{
		{
			Value: 0x12a05f200,
			PkScript: []byte{
				0x41, 0x04, 0x67, 0x8a, 0xfd, 0xb0, 0xfe, 0x55, /* |A.g....U| */
				0x48, 0x27, 0x19, 0x67, 0xf1, 0xa6, 0x71, 0x30, /* |H'.g..q0| */
				0xb7, 0x10, 0x5c, 0xd6, 0xa8, 0x28, 0xe0, 0x39, /* |..\..(.9| */
				0x09, 0xa6, 0x79, 0x62, 0xe0, 0xea, 0x1f, 0x61, /* |..yb...a| */
				0xde, 0xb6, 0x49, 0xf6, 0xbc, 0x3f, 0x4c, 0xef, /* |..I..?L.| */
				0x38, 0xc4, 0xf3, 0x55, 0x04, 0xe5, 0x1e, 0xc1, /* |8..U....| */
				0x12, 0xde, 0x5c, 0x38, 0x4d, 0xf7, 0xba, 0x0b, /* |..\8M...| */
				0x8d, 0x57, 0x8a, 0x4c, 0x70, 0x2b, 0x6b, 0xf1, /* |.W.Lp+k.| */
				0x1d, 0x5f, 0xac, /* |._.| */
			},
		},
	},
	LockTime: 0,
}

// BenchmarkWriteVarInt1 performs a benchmark on how long it takes to write
// a single byte variable length integer.
func BenchmarkWriteVarInt1(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		WriteVarInt(io.Discard, 0, 1)
	}
}

// BenchmarkWriteVarInt3 performs a benchmark on how long it takes to write
// a three byte variable length integer.
func BenchmarkWriteVarInt3(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		WriteVarInt(io.Discard, 0, 65535)
	}
}

// BenchmarkWriteVarInt5 performs a benchmark on how long it takes to write
// a five byte variable length integer.
func BenchmarkWriteVarInt5(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		WriteVarInt(io.Discard, 0, 4294967295)
	}
}

// BenchmarkWriteVarInt9 performs a benchmark on how long it takes to write
// a nine byte variable length integer.
func BenchmarkWriteVarInt9(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		WriteVarInt(io.Discard, 0, 18446744073709551615)
	}
}

// BenchmarkReadVarInt1 performs a benchmark on how long it takes to read
// a single byte variable length integer.
func BenchmarkReadVarInt1(b *testing.B) {
	b.ReportAllocs()

	buf := []byte{0x01}
	r := bytes.NewReader(buf)
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		ReadVarInt(r, 0)
	}
}

// BenchmarkReadVarInt3 performs a benchmark on how long it takes to read
// a three byte variable length integer.
func BenchmarkReadVarInt3(b *testing.B) {
	b.ReportAllocs()

	buf := []byte{0x0fd, 0xff, 0xff}
	r := bytes.NewReader(buf)
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		ReadVarInt(r, 0)
	}
}

// BenchmarkReadVarInt5 performs a benchmark on how long it takes to read
// a five byte variable length integer.
func BenchmarkReadVarInt5(b *testing.B) {
	b.ReportAllocs()

	buf := []byte{0xfe, 0xff, 0xff, 0xff, 0xff}
	r := bytes.NewReader(buf)
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		ReadVarInt(r, 0)
	}
}

// BenchmarkReadVarInt9 performs a benchmark on how long it takes to read
// a nine byte variable length integer.
func BenchmarkReadVarInt9(b *testing.B) {
	b.ReportAllocs()

	buf := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	r := bytes.NewReader(buf)
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		ReadVarInt(r, 0)
	}
}

// BenchmarkWriteVarIntBuf1 performs a benchmark on how long it takes to write
// a single byte variable length integer.
func BenchmarkWriteVarIntBuf1(b *testing.B) {
	b.ReportAllocs()

	buffer := binarySerializer.Borrow()
	for i := 0; i < b.N; i++ {
		WriteVarIntBuf(io.Discard, 0, 1, buffer)
	}
	binarySerializer.Return(buffer)
}

// BenchmarkWriteVarIntBuf3 performs a benchmark on how long it takes to write
// a three byte variable length integer.
func BenchmarkWriteVarIntBuf3(b *testing.B) {
	b.ReportAllocs()

	buffer := binarySerializer.Borrow()
	for i := 0; i < b.N; i++ {
		WriteVarIntBuf(io.Discard, 0, 65535, buffer)
	}
	binarySerializer.Return(buffer)
}

// BenchmarkWriteVarIntBuf5 performs a benchmark on how long it takes to write
// a five byte variable length integer.
func BenchmarkWriteVarIntBuf5(b *testing.B) {
	b.ReportAllocs()

	buffer := binarySerializer.Borrow()
	for i := 0; i < b.N; i++ {
		WriteVarIntBuf(io.Discard, 0, 4294967295, buffer)
	}
	binarySerializer.Return(buffer)
}

// BenchmarkWriteVarIntBuf9 performs a benchmark on how long it takes to write
// a nine byte variable length integer.
func BenchmarkWriteVarIntBuf9(b *testing.B) {
	b.ReportAllocs()

	buffer := binarySerializer.Borrow()
	for i := 0; i < b.N; i++ {
		WriteVarIntBuf(io.Discard, 0, 18446744073709551615, buffer)
	}
	binarySerializer.Return(buffer)
}

// BenchmarkReadVarIntBuf1 performs a benchmark on how long it takes to read
// a single byte variable length integer.
func BenchmarkReadVarIntBuf1(b *testing.B) {
	b.ReportAllocs()

	buffer := binarySerializer.Borrow()
	buf := []byte{0x01}
	r := bytes.NewReader(buf)
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		ReadVarIntBuf(r, 0, buffer)
	}
	binarySerializer.Return(buffer)
}

// BenchmarkReadVarIntBuf3 performs a benchmark on how long it takes to read
// a three byte variable length integer.
func BenchmarkReadVarIntBuf3(b *testing.B) {
	b.ReportAllocs()

	buffer := binarySerializer.Borrow()
	buf := []byte{0x0fd, 0xff, 0xff}
	r := bytes.NewReader(buf)
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		ReadVarIntBuf(r, 0, buffer)
	}
	binarySerializer.Return(buffer)
}

// BenchmarkReadVarIntBuf5 performs a benchmark on how long it takes to read
// a five byte variable length integer.
func BenchmarkReadVarIntBuf5(b *testing.B) {
	b.ReportAllocs()

	buffer := binarySerializer.Borrow()
	buf := []byte{0xfe, 0xff, 0xff, 0xff, 0xff}
	r := bytes.NewReader(buf)
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		ReadVarIntBuf(r, 0, buffer)
	}
	binarySerializer.Return(buffer)
}

// BenchmarkReadVarIntBuf9 performs a benchmark on how long it takes to read
// a nine byte variable length integer.
func BenchmarkReadVarIntBuf9(b *testing.B) {
	b.ReportAllocs()

	buffer := binarySerializer.Borrow()
	buf := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	r := bytes.NewReader(buf)
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		ReadVarIntBuf(r, 0, buffer)
	}
	binarySerializer.Return(buffer)
}

// BenchmarkReadVarStr4 performs a benchmark on how long it takes to read a
// four byte variable length string.
func BenchmarkReadVarStr4(b *testing.B) {
	b.ReportAllocs()

	buf := []byte{0x04, 't', 'e', 's', 't'}
	r := bytes.NewReader(buf)
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		ReadVarString(r, 0)
	}
}

// BenchmarkReadVarStr10 performs a benchmark on how long it takes to read a
// ten byte variable length string.
func BenchmarkReadVarStr10(b *testing.B) {
	b.ReportAllocs()

	buf := []byte{0x0a, 't', 'e', 's', 't', '0', '1', '2', '3', '4', '5'}
	r := bytes.NewReader(buf)
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		ReadVarString(r, 0)
	}
}

// BenchmarkWriteVarStr4 performs a benchmark on how long it takes to write a
// four byte variable length string.
func BenchmarkWriteVarStr4(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		WriteVarString(io.Discard, 0, "test")
	}
}

// BenchmarkWriteVarStr10 performs a benchmark on how long it takes to write a
// ten byte variable length string.
func BenchmarkWriteVarStr10(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		WriteVarString(io.Discard, 0, "test012345")
	}
}

// BenchmarkReadVarStrBuf4 performs a benchmark on how long it takes to read a
// four byte variable length string.
func BenchmarkReadVarStrBuf4(b *testing.B) {
	b.ReportAllocs()

	buffer := binarySerializer.Borrow()
	buf := []byte{0x04, 't', 'e', 's', 't'}
	r := bytes.NewReader(buf)
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		readVarStringBuf(r, 0, buffer)
	}
	binarySerializer.Return(buffer)
}

// BenchmarkReadVarStrBuf10 performs a benchmark on how long it takes to read a
// ten byte variable length string.
func BenchmarkReadVarStrBuf10(b *testing.B) {
	b.ReportAllocs()

	buffer := binarySerializer.Borrow()
	buf := []byte{0x0a, 't', 'e', 's', 't', '0', '1', '2', '3', '4', '5'}
	r := bytes.NewReader(buf)
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		readVarStringBuf(r, 0, buf)
	}
	binarySerializer.Return(buffer)
}

// BenchmarkWriteVarStrBuf4 performs a benchmark on how long it takes to write a
// four byte variable length string.
func BenchmarkWriteVarStrBuf4(b *testing.B) {
	b.ReportAllocs()

	buf := binarySerializer.Borrow()
	for i := 0; i < b.N; i++ {
		writeVarStringBuf(io.Discard, 0, "test", buf)
	}
	binarySerializer.Return(buf)
}

// BenchmarkWriteVarStrBuf10 performs a benchmark on how long it takes to write
// a ten byte variable length string.
func BenchmarkWriteVarStrBuf10(b *testing.B) {
	b.ReportAllocs()

	buf := binarySerializer.Borrow()
	for i := 0; i < b.N; i++ {
		writeVarStringBuf(io.Discard, 0, "test012345", buf)
	}
	binarySerializer.Return(buf)
}

// BenchmarkReadOutPoint performs a benchmark on how long it takes to read a
// transaction output point.
func BenchmarkReadOutPoint(b *testing.B) {
	b.ReportAllocs()

	buffer := binarySerializer.Borrow()
	buf := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Previous output hash
		0xff, 0xff, 0xff, 0xff, // Previous output index
	}
	r := bytes.NewReader(buf)
	var op OutPoint
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		readOutPointBuf(r, 0, 0, &op, buffer)
	}
	binarySerializer.Return(buffer)
}

// BenchmarkWriteOutPoint performs a benchmark on how long it takes to write a
// transaction output point.
func BenchmarkWriteOutPoint(b *testing.B) {
	b.ReportAllocs()

	op := &OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0,
	}
	for i := 0; i < b.N; i++ {
		WriteOutPoint(io.Discard, 0, 0, op)
	}
}

// BenchmarkWriteOutPointBuf performs a benchmark on how long it takes to write a
// transaction output point.
func BenchmarkWriteOutPointBuf(b *testing.B) {
	b.ReportAllocs()

	buf := binarySerializer.Borrow()
	op := &OutPoint{
		Hash:  chainhash.Hash{},
		Index: 0,
	}
	for i := 0; i < b.N; i++ {
		writeOutPointBuf(io.Discard, 0, 0, op, buf)
	}
	binarySerializer.Return(buf)
}

// BenchmarkReadTxOut performs a benchmark on how long it takes to read a
// transaction output.
func BenchmarkReadTxOut(b *testing.B) {
	b.ReportAllocs()

	buf := []byte{
		0x00, 0xf2, 0x05, 0x2a, 0x01, 0x00, 0x00, 0x00, // Transaction amount
		0x43, // Varint for length of pk script
		0x41, // OP_DATA_65
		0x04, 0x96, 0xb5, 0x38, 0xe8, 0x53, 0x51, 0x9c,
		0x72, 0x6a, 0x2c, 0x91, 0xe6, 0x1e, 0xc1, 0x16,
		0x00, 0xae, 0x13, 0x90, 0x81, 0x3a, 0x62, 0x7c,
		0x66, 0xfb, 0x8b, 0xe7, 0x94, 0x7b, 0xe6, 0x3c,
		0x52, 0xda, 0x75, 0x89, 0x37, 0x95, 0x15, 0xd4,
		0xe0, 0xa6, 0x04, 0xf8, 0x14, 0x17, 0x81, 0xe6,
		0x22, 0x94, 0x72, 0x11, 0x66, 0xbf, 0x62, 0x1e,
		0x73, 0xa8, 0x2c, 0xbf, 0x23, 0x42, 0xc8, 0x58,
		0xee, // 65-byte signature
		0xac, // OP_CHECKSIG
	}
	r := bytes.NewReader(buf)
	var txOut TxOut
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		ReadTxOut(r, 0, 0, &txOut)
	}
}

// BenchmarkReadTxOutBuf performs a benchmark on how long it takes to read a
// transaction output.
func BenchmarkReadTxOutBuf(b *testing.B) {
	b.ReportAllocs()

	scriptBuffer := scriptPool.Borrow()
	sbuf := scriptBuffer[:]
	buffer := binarySerializer.Borrow()
	buf := []byte{
		0x00, 0xf2, 0x05, 0x2a, 0x01, 0x00, 0x00, 0x00, // Transaction amount
		0x43, // Varint for length of pk script
		0x41, // OP_DATA_65
		0x04, 0x96, 0xb5, 0x38, 0xe8, 0x53, 0x51, 0x9c,
		0x72, 0x6a, 0x2c, 0x91, 0xe6, 0x1e, 0xc1, 0x16,
		0x00, 0xae, 0x13, 0x90, 0x81, 0x3a, 0x62, 0x7c,
		0x66, 0xfb, 0x8b, 0xe7, 0x94, 0x7b, 0xe6, 0x3c,
		0x52, 0xda, 0x75, 0x89, 0x37, 0x95, 0x15, 0xd4,
		0xe0, 0xa6, 0x04, 0xf8, 0x14, 0x17, 0x81, 0xe6,
		0x22, 0x94, 0x72, 0x11, 0x66, 0xbf, 0x62, 0x1e,
		0x73, 0xa8, 0x2c, 0xbf, 0x23, 0x42, 0xc8, 0x58,
		0xee, // 65-byte signature
		0xac, // OP_CHECKSIG
	}
	r := bytes.NewReader(buf)
	var txOut TxOut
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		readTxOutBuf(r, 0, 0, &txOut, buffer, sbuf)
	}
	binarySerializer.Return(buffer)
	scriptPool.Return(scriptBuffer)
}

// BenchmarkWriteTxOut performs a benchmark on how long it takes to write
// a transaction output.
func BenchmarkWriteTxOut(b *testing.B) {
	b.ReportAllocs()

	txOut := blockOne.Transactions[0].TxOut[0]
	for i := 0; i < b.N; i++ {
		WriteTxOut(io.Discard, 0, 0, txOut)
	}
}

// BenchmarkWriteTxOutBuf performs a benchmark on how long it takes to write
// a transaction output.
func BenchmarkWriteTxOutBuf(b *testing.B) {
	b.ReportAllocs()

	buf := binarySerializer.Borrow()
	txOut := blockOne.Transactions[0].TxOut[0]
	for i := 0; i < b.N; i++ {
		WriteTxOutBuf(io.Discard, 0, 0, txOut, buf)
	}
	binarySerializer.Return(buf)
}

// BenchmarkReadTxIn performs a benchmark on how long it takes to read a
// transaction input.
func BenchmarkReadTxIn(b *testing.B) {
	b.ReportAllocs()

	scriptBuffer := scriptPool.Borrow()
	sbuf := scriptBuffer[:]
	buffer := binarySerializer.Borrow()
	buf := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Previous output hash
		0xff, 0xff, 0xff, 0xff, // Previous output index
		0x07,                                     // Varint for length of signature script
		0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, // Signature script
		0xff, 0xff, 0xff, 0xff, // Sequence
	}
	r := bytes.NewReader(buf)
	var txIn TxIn
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		readTxInBuf(r, 0, 0, &txIn, buffer, sbuf)
	}
	binarySerializer.Return(buffer)
	scriptPool.Return(scriptBuffer)
}

// BenchmarkWriteTxIn performs a benchmark on how long it takes to write
// a transaction input.
func BenchmarkWriteTxIn(b *testing.B) {
	b.ReportAllocs()

	buf := binarySerializer.Borrow()
	txIn := blockOne.Transactions[0].TxIn[0]
	for i := 0; i < b.N; i++ {
		writeTxInBuf(io.Discard, 0, 0, txIn, buf)
	}
	binarySerializer.Return(buf)
}

// BenchmarkDeserializeTx performs a benchmark on how long it takes to
// deserialize a small transaction.
func BenchmarkDeserializeTxSmall(b *testing.B) {
	buf := []byte{
		0x01, 0x00, 0x00, 0x00, // Version
		0x01, // Varint for number of input transactions
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //  // Previous output hash
		0xff, 0xff, 0xff, 0xff, // Prevous output index
		0x07,                                     // Varint for length of signature script
		0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, // Signature script
		0xff, 0xff, 0xff, 0xff, // Sequence
		0x01,                                           // Varint for number of output transactions
		0x00, 0xf2, 0x05, 0x2a, 0x01, 0x00, 0x00, 0x00, // Transaction amount
		0x43, // Varint for length of pk script
		0x41, // OP_DATA_65
		0x04, 0x96, 0xb5, 0x38, 0xe8, 0x53, 0x51, 0x9c,
		0x72, 0x6a, 0x2c, 0x91, 0xe6, 0x1e, 0xc1, 0x16,
		0x00, 0xae, 0x13, 0x90, 0x81, 0x3a, 0x62, 0x7c,
		0x66, 0xfb, 0x8b, 0xe7, 0x94, 0x7b, 0xe6, 0x3c,
		0x52, 0xda, 0x75, 0x89, 0x37, 0x95, 0x15, 0xd4,
		0xe0, 0xa6, 0x04, 0xf8, 0x14, 0x17, 0x81, 0xe6,
		0x22, 0x94, 0x72, 0x11, 0x66, 0xbf, 0x62, 0x1e,
		0x73, 0xa8, 0x2c, 0xbf, 0x23, 0x42, 0xc8, 0x58,
		0xee,                   // 65-byte signature
		0xac,                   // OP_CHECKSIG
		0x00, 0x00, 0x00, 0x00, // Lock time
	}

	b.ReportAllocs()
	b.ResetTimer()

	r := bytes.NewReader(buf)
	var tx MsgTx
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		tx.Deserialize(r)
	}
}

// BenchmarkDeserializeTxLarge performs a benchmark on how long it takes to
// deserialize a very large transaction.
func BenchmarkDeserializeTxLarge(b *testing.B) {

	// tx bb41a757f405890fb0f5856228e23b715702d714d59bf2b1feb70d8b2b4e3e08
	// from the main block chain.
	fi, err := os.Open("testdata/megatx.bin.bz2")
	if err != nil {
		b.Fatalf("Failed to read transaction data: %v", err)
	}
	defer fi.Close()
	buf, err := ioutil.ReadAll(bzip2.NewReader(fi))
	if err != nil {
		b.Fatalf("Failed to read transaction data: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	r := bytes.NewReader(buf)
	var tx MsgTx
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		tx.Deserialize(r)
	}
}

func BenchmarkDeserializeBlock(b *testing.B) {
	buf, err := os.ReadFile(
		"testdata/block-00000000000000000021868c2cefc52a480d173c849412fe81c4e5ab806f94ab.blk",
	)
	if err != nil {
		b.Fatalf("Failed to read block data: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	r := bytes.NewReader(buf)
	var block MsgBlock
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		block.Deserialize(r)
	}
}

func BenchmarkSerializeBlock(b *testing.B) {
	buf, err := os.ReadFile(
		"testdata/block-00000000000000000021868c2cefc52a480d173c849412fe81c4e5ab806f94ab.blk",
	)
	if err != nil {
		b.Fatalf("Failed to read block data: %v", err)
	}

	var block MsgBlock
	err = block.Deserialize(bytes.NewReader(buf))
	if err != nil {
		panic(err.Error())
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		block.Serialize(io.Discard)
	}
}

// BenchmarkSerializeTx performs a benchmark on how long it takes to serialize
// a transaction.
func BenchmarkSerializeTx(b *testing.B) {
	b.ReportAllocs()

	tx := blockOne.Transactions[0]
	for i := 0; i < b.N; i++ {
		tx.Serialize(io.Discard)

	}
}

// BenchmarkSerializeTxSmall performs a benchmark on how long it takes to
// serialize a transaction.
func BenchmarkSerializeTxSmall(b *testing.B) {
	buf := []byte{
		0x01, 0x00, 0x00, 0x00, // Version
		0x01, // Varint for number of input transactions
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, //  // Previous output hash
		0xff, 0xff, 0xff, 0xff, // Prevous output index
		0x07,                                     // Varint for length of signature script
		0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, // Signature script
		0xff, 0xff, 0xff, 0xff, // Sequence
		0x01,                                           // Varint for number of output transactions
		0x00, 0xf2, 0x05, 0x2a, 0x01, 0x00, 0x00, 0x00, // Transaction amount
		0x43, // Varint for length of pk script
		0x41, // OP_DATA_65
		0x04, 0x96, 0xb5, 0x38, 0xe8, 0x53, 0x51, 0x9c,
		0x72, 0x6a, 0x2c, 0x91, 0xe6, 0x1e, 0xc1, 0x16,
		0x00, 0xae, 0x13, 0x90, 0x81, 0x3a, 0x62, 0x7c,
		0x66, 0xfb, 0x8b, 0xe7, 0x94, 0x7b, 0xe6, 0x3c,
		0x52, 0xda, 0x75, 0x89, 0x37, 0x95, 0x15, 0xd4,
		0xe0, 0xa6, 0x04, 0xf8, 0x14, 0x17, 0x81, 0xe6,
		0x22, 0x94, 0x72, 0x11, 0x66, 0xbf, 0x62, 0x1e,
		0x73, 0xa8, 0x2c, 0xbf, 0x23, 0x42, 0xc8, 0x58,
		0xee,                   // 65-byte signature
		0xac,                   // OP_CHECKSIG
		0x00, 0x00, 0x00, 0x00, // Lock time
	}

	var tx MsgTx
	tx.Deserialize(bytes.NewReader(buf))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tx.Serialize(io.Discard)
	}
}

// BenchmarkSerializeTxLarge performs a benchmark on how long it takes to
// serialize a transaction.
func BenchmarkSerializeTxLarge(b *testing.B) {
	// tx bb41a757f405890fb0f5856228e23b715702d714d59bf2b1feb70d8b2b4e3e08
	// from the main block chain.
	fi, err := os.Open("testdata/megatx.bin.bz2")
	if err != nil {
		b.Fatalf("Failed to read transaction data: %v", err)
	}
	defer fi.Close()
	buf, err := ioutil.ReadAll(bzip2.NewReader(fi))
	if err != nil {
		b.Fatalf("Failed to read transaction data: %v", err)
	}

	var tx MsgTx
	tx.Deserialize(bytes.NewReader(buf))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		tx.Serialize(io.Discard)
	}
}

// BenchmarkReadBlockHeader performs a benchmark on how long it takes to
// deserialize a block header.
func BenchmarkReadBlockHeader(b *testing.B) {
	b.ReportAllocs()

	buf := []byte{
		0x01, 0x00, 0x00, 0x00, // Version 1
		0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
		0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
		0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
		0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // PrevBlock
		0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
		0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
		0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
		0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a, // MerkleRoot
		0x29, 0xab, 0x5f, 0x49, // Timestamp
		0xff, 0xff, 0x00, 0x1d, // Bits
		0xf3, 0xe0, 0x01, 0x00, // Nonce
		0x00, // TxnCount Varint
	}
	r := bytes.NewReader(buf)
	var header BlockHeader
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		readBlockHeader(r, 0, &header)
	}
}

// BenchmarkReadBlockHeaderBuf performs a benchmark on how long it takes to
// deserialize a block header.
func BenchmarkReadBlockHeaderBuf(b *testing.B) {
	b.ReportAllocs()

	buffer := binarySerializer.Borrow()
	buf := []byte{
		0x01, 0x00, 0x00, 0x00, // Version 1
		0x6f, 0xe2, 0x8c, 0x0a, 0xb6, 0xf1, 0xb3, 0x72,
		0xc1, 0xa6, 0xa2, 0x46, 0xae, 0x63, 0xf7, 0x4f,
		0x93, 0x1e, 0x83, 0x65, 0xe1, 0x5a, 0x08, 0x9c,
		0x68, 0xd6, 0x19, 0x00, 0x00, 0x00, 0x00, 0x00, // PrevBlock
		0x3b, 0xa3, 0xed, 0xfd, 0x7a, 0x7b, 0x12, 0xb2,
		0x7a, 0xc7, 0x2c, 0x3e, 0x67, 0x76, 0x8f, 0x61,
		0x7f, 0xc8, 0x1b, 0xc3, 0x88, 0x8a, 0x51, 0x32,
		0x3a, 0x9f, 0xb8, 0xaa, 0x4b, 0x1e, 0x5e, 0x4a, // MerkleRoot
		0x29, 0xab, 0x5f, 0x49, // Timestamp
		0xff, 0xff, 0x00, 0x1d, // Bits
		0xf3, 0xe0, 0x01, 0x00, // Nonce
		0x00, // TxnCount Varint
	}
	r := bytes.NewReader(buf)
	var header BlockHeader
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		readBlockHeaderBuf(r, 0, &header, buffer)
	}
	binarySerializer.Return(buffer)
}

// BenchmarkWriteBlockHeader performs a benchmark on how long it takes to
// serialize a block header.
func BenchmarkWriteBlockHeader(b *testing.B) {
	b.ReportAllocs()

	header := blockOne.Header
	for i := 0; i < b.N; i++ {
		writeBlockHeader(io.Discard, 0, &header)
	}
}

// BenchmarkWriteBlockHeaderBuf performs a benchmark on how long it takes to
// serialize a block header.
func BenchmarkWriteBlockHeaderBuf(b *testing.B) {
	b.ReportAllocs()

	buf := binarySerializer.Borrow()
	header := blockOne.Header
	for i := 0; i < b.N; i++ {
		writeBlockHeaderBuf(io.Discard, 0, &header, buf)
	}
	binarySerializer.Return(buf)
}

// BenchmarkDecodeGetHeaders performs a benchmark on how long it takes to
// decode a getheaders message with the maximum number of block locator hashes.
func BenchmarkDecodeGetHeaders(b *testing.B) {
	b.ReportAllocs()

	// Create a message with the maximum number of block locators.
	pver := ProtocolVersion
	var m MsgGetHeaders
	for i := 0; i < MaxBlockLocatorsPerMsg; i++ {
		hash, err := chainhash.NewHashFromStr(fmt.Sprintf("%x", i))
		if err != nil {
			b.Fatalf("NewHashFromStr: unexpected error: %v", err)
		}
		m.AddBlockLocatorHash(hash)
	}

	// Serialize it so the bytes are available to test the decode below.
	var bb bytes.Buffer
	if err := m.BtcEncode(&bb, pver, LatestEncoding); err != nil {
		b.Fatalf("MsgGetHeaders.BtcEncode: unexpected error: %v", err)
	}
	buf := bb.Bytes()

	r := bytes.NewReader(buf)
	var msg MsgGetHeaders
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		msg.BtcDecode(r, pver, LatestEncoding)
	}
}

// BenchmarkDecodeHeaders performs a benchmark on how long it takes to
// decode a headers message with the maximum number of headers.
func BenchmarkDecodeHeaders(b *testing.B) {
	b.ReportAllocs()

	// Create a message with the maximum number of headers.
	pver := ProtocolVersion
	var m MsgHeaders
	for i := 0; i < MaxBlockHeadersPerMsg; i++ {
		hash, err := chainhash.NewHashFromStr(fmt.Sprintf("%x", i))
		if err != nil {
			b.Fatalf("NewHashFromStr: unexpected error: %v", err)
		}
		m.AddBlockHeader(NewBlockHeader(1, hash, hash, 0, uint32(i)))
	}

	// Serialize it so the bytes are available to test the decode below.
	var bb bytes.Buffer
	if err := m.BtcEncode(&bb, pver, LatestEncoding); err != nil {
		b.Fatalf("MsgHeaders.BtcEncode: unexpected error: %v", err)
	}
	buf := bb.Bytes()

	r := bytes.NewReader(buf)
	var msg MsgHeaders
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		msg.BtcDecode(r, pver, LatestEncoding)
	}
}

// BenchmarkDecodeGetBlocks performs a benchmark on how long it takes to
// decode a getblocks message with the maximum number of block locator hashes.
func BenchmarkDecodeGetBlocks(b *testing.B) {
	b.ReportAllocs()

	// Create a message with the maximum number of block locators.
	pver := ProtocolVersion
	var m MsgGetBlocks
	for i := 0; i < MaxBlockLocatorsPerMsg; i++ {
		hash, err := chainhash.NewHashFromStr(fmt.Sprintf("%x", i))
		if err != nil {
			b.Fatalf("NewHashFromStr: unexpected error: %v", err)
		}
		m.AddBlockLocatorHash(hash)
	}

	// Serialize it so the bytes are available to test the decode below.
	var bb bytes.Buffer
	if err := m.BtcEncode(&bb, pver, LatestEncoding); err != nil {
		b.Fatalf("MsgGetBlocks.BtcEncode: unexpected error: %v", err)
	}
	buf := bb.Bytes()

	r := bytes.NewReader(buf)
	var msg MsgGetBlocks
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		msg.BtcDecode(r, pver, LatestEncoding)
	}
}

// BenchmarkDecodeAddr performs a benchmark on how long it takes to decode an
// addr message with the maximum number of addresses.
func BenchmarkDecodeAddr(b *testing.B) {
	b.ReportAllocs()

	// Create a message with the maximum number of addresses.
	pver := ProtocolVersion
	ip := net.ParseIP("127.0.0.1")
	ma := NewMsgAddr()
	for port := uint16(0); port < MaxAddrPerMsg; port++ {
		ma.AddAddress(NewNetAddressIPPort(ip, port, SFNodeNetwork))
	}

	// Serialize it so the bytes are available to test the decode below.
	var bb bytes.Buffer
	if err := ma.BtcEncode(&bb, pver, LatestEncoding); err != nil {
		b.Fatalf("MsgAddr.BtcEncode: unexpected error: %v", err)
	}
	buf := bb.Bytes()

	r := bytes.NewReader(buf)
	var msg MsgAddr
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		msg.BtcDecode(r, pver, LatestEncoding)
	}
}

// BenchmarkDecodeInv performs a benchmark on how long it takes to decode an inv
// message with the maximum number of entries.
func BenchmarkDecodeInv(b *testing.B) {
	// Create a message with the maximum number of entries.
	pver := ProtocolVersion
	var m MsgInv
	for i := 0; i < MaxInvPerMsg; i++ {
		hash, err := chainhash.NewHashFromStr(fmt.Sprintf("%x", i))
		if err != nil {
			b.Fatalf("NewHashFromStr: unexpected error: %v", err)
		}
		m.AddInvVect(NewInvVect(InvTypeBlock, hash))
	}

	// Serialize it so the bytes are available to test the decode below.
	var bb bytes.Buffer
	if err := m.BtcEncode(&bb, pver, LatestEncoding); err != nil {
		b.Fatalf("MsgInv.BtcEncode: unexpected error: %v", err)
	}
	buf := bb.Bytes()

	b.ReportAllocs()
	b.ResetTimer()

	r := bytes.NewReader(buf)
	var msg MsgInv
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		msg.BtcDecode(r, pver, LatestEncoding)
	}
}

// BenchmarkDecodeNotFound performs a benchmark on how long it takes to decode
// a notfound message with the maximum number of entries.
func BenchmarkDecodeNotFound(b *testing.B) {
	b.ReportAllocs()

	// Create a message with the maximum number of entries.
	pver := ProtocolVersion
	var m MsgNotFound
	for i := 0; i < MaxInvPerMsg; i++ {
		hash, err := chainhash.NewHashFromStr(fmt.Sprintf("%x", i))
		if err != nil {
			b.Fatalf("NewHashFromStr: unexpected error: %v", err)
		}
		m.AddInvVect(NewInvVect(InvTypeBlock, hash))
	}

	// Serialize it so the bytes are available to test the decode below.
	var bb bytes.Buffer
	if err := m.BtcEncode(&bb, pver, LatestEncoding); err != nil {
		b.Fatalf("MsgNotFound.BtcEncode: unexpected error: %v", err)
	}
	buf := bb.Bytes()

	r := bytes.NewReader(buf)
	var msg MsgNotFound
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		msg.BtcDecode(r, pver, LatestEncoding)
	}
}

// BenchmarkDecodeMerkleBlock performs a benchmark on how long it takes to
// decode a reasonably sized merkleblock message.
func BenchmarkDecodeMerkleBlock(b *testing.B) {
	b.ReportAllocs()

	// Create a message with random data.
	pver := ProtocolVersion
	var m MsgMerkleBlock
	hash, err := chainhash.NewHashFromStr(fmt.Sprintf("%x", 10000))
	if err != nil {
		b.Fatalf("NewHashFromStr: unexpected error: %v", err)
	}
	m.Header = *NewBlockHeader(1, hash, hash, 0, uint32(10000))
	for i := 0; i < 105; i++ {
		hash, err := chainhash.NewHashFromStr(fmt.Sprintf("%x", i))
		if err != nil {
			b.Fatalf("NewHashFromStr: unexpected error: %v", err)
		}
		m.AddTxHash(hash)
		if i%8 == 0 {
			m.Flags = append(m.Flags, uint8(i))
		}
	}

	// Serialize it so the bytes are available to test the decode below.
	var bb bytes.Buffer
	if err := m.BtcEncode(&bb, pver, LatestEncoding); err != nil {
		b.Fatalf("MsgMerkleBlock.BtcEncode: unexpected error: %v", err)
	}
	buf := bb.Bytes()

	r := bytes.NewReader(buf)
	var msg MsgMerkleBlock
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Seek(0, 0)
		msg.BtcDecode(r, pver, LatestEncoding)
	}
}

// BenchmarkTxHash performs a benchmark on how long it takes to hash a
// transaction.
func BenchmarkTxHash(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		genesisCoinbaseTx.TxHash()
	}
}

// BenchmarkDoubleHashB performs a benchmark on how long it takes to perform a
// double hash returning a byte slice.
func BenchmarkDoubleHashB(b *testing.B) {
	b.ReportAllocs()

	var buf bytes.Buffer
	if err := genesisCoinbaseTx.Serialize(&buf); err != nil {
		b.Errorf("Serialize: unexpected error: %v", err)
		return
	}
	txBytes := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = chainhash.DoubleHashB(txBytes)
	}
}

// BenchmarkDoubleHashH performs a benchmark on how long it takes to perform
// a double hash returning a chainhash.Hash.
func BenchmarkDoubleHashH(b *testing.B) {
	b.ReportAllocs()

	var buf bytes.Buffer
	if err := genesisCoinbaseTx.Serialize(&buf); err != nil {
		b.Errorf("Serialize: unexpected error: %v", err)
		return
	}
	txBytes := buf.Bytes()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = chainhash.DoubleHashH(txBytes)
	}
}
