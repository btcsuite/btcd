// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"bytes"
	"github.com/conformal/btcwire"
	"io/ioutil"
	"testing"
)

// BenchmarkWriteVarInt1 performs a benchmark on how long it takes to write
// a single byte variable length integer.
func BenchmarkWriteVarInt1(b *testing.B) {
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteVarInt(ioutil.Discard, 0, 1)
	}
}

// BenchmarkWriteVarInt3 performs a benchmark on how long it takes to write
// a three byte variable length integer.
func BenchmarkWriteVarInt3(b *testing.B) {
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteVarInt(ioutil.Discard, 0, 65535)
	}
}

// BenchmarkWriteVarInt5 performs a benchmark on how long it takes to write
// a five byte variable length integer.
func BenchmarkWriteVarInt5(b *testing.B) {
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteVarInt(ioutil.Discard, 0, 4294967295)
	}
}

// BenchmarkWriteVarInt9 performs a benchmark on how long it takes to write
// a nine byte variable length integer.
func BenchmarkWriteVarInt9(b *testing.B) {
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteVarInt(ioutil.Discard, 0, 18446744073709551615)
	}
}

// BenchmarkReadVarInt1 performs a benchmark on how long it takes to read
// a single byte variable length integer.
func BenchmarkReadVarInt1(b *testing.B) {
	buf := []byte{0x01}
	for i := 0; i < b.N; i++ {
		btcwire.TstReadVarInt(bytes.NewBuffer(buf), 0)
	}
}

// BenchmarkReadVarInt3 performs a benchmark on how long it takes to read
// a three byte variable length integer.
func BenchmarkReadVarInt3(b *testing.B) {
	buf := []byte{0x0fd, 0xff, 0xff}
	for i := 0; i < b.N; i++ {
		btcwire.TstReadVarInt(bytes.NewBuffer(buf), 0)
	}
}

// BenchmarkReadVarInt5 performs a benchmark on how long it takes to read
// a five byte variable length integer.
func BenchmarkReadVarInt5(b *testing.B) {
	buf := []byte{0xfe, 0xff, 0xff, 0xff, 0xff}
	for i := 0; i < b.N; i++ {
		btcwire.TstReadVarInt(bytes.NewBuffer(buf), 0)
	}
}

// BenchmarkReadVarInt9 performs a benchmark on how long it takes to read
// a nine byte variable length integer.
func BenchmarkReadVarInt9(b *testing.B) {
	buf := []byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}
	for i := 0; i < b.N; i++ {
		btcwire.TstReadVarInt(bytes.NewBuffer(buf), 0)
	}
}

// BenchmarkReadVarStr4 performs a benchmark on how long it takes to read a
// four byte variable length string.
func BenchmarkReadVarStr4(b *testing.B) {
	buf := []byte{0x04, 't', 'e', 's', 't'}
	for i := 0; i < b.N; i++ {
		btcwire.TstReadVarString(bytes.NewBuffer(buf), 0)
	}
}

// BenchmarkReadVarStr10 performs a benchmark on how long it takes to read a
// ten byte variable length string.
func BenchmarkReadVarStr10(b *testing.B) {
	buf := []byte{0x0a, 't', 'e', 's', 't', '0', '1', '2', '3', '4', '5'}
	for i := 0; i < b.N; i++ {
		btcwire.TstReadVarString(bytes.NewBuffer(buf), 0)
	}
}

// BenchmarkWriteVarStr4 performs a benchmark on how long it takes to write a
// four byte variable length string.
func BenchmarkWriteVarStr4(b *testing.B) {
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteVarString(ioutil.Discard, 0, "test")
	}
}

// BenchmarkWriteVarStr10 performs a benchmark on how long it takes to write a
// ten byte variable length string.
func BenchmarkWriteVarStr10(b *testing.B) {
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteVarString(ioutil.Discard, 0, "test012345")
	}
}

// BenchmarkReadOutPoint performs a benchmark on how long it takes to read a
// transaction output point.
func BenchmarkReadOutPoint(b *testing.B) {
	buf := []byte{
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Previous output hash
		0xff, 0xff, 0xff, 0xff, // Previous output index
	}
	var op btcwire.OutPoint
	for i := 0; i < b.N; i++ {
		btcwire.TstReadOutPoint(bytes.NewBuffer(buf), 0, 0, &op)
	}
}

// BenchmarkWriteOutPoint performs a benchmark on how long it takes to write a
// transaction output point.
func BenchmarkWriteOutPoint(b *testing.B) {
	op := &btcwire.OutPoint{
		Hash:  btcwire.ShaHash{},
		Index: 0,
	}
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteOutPoint(ioutil.Discard, 0, 0, op)
	}
}

// BenchmarkReadTxOut performs a benchmark on how long it takes to read a
// transaction output.
func BenchmarkReadTxOut(b *testing.B) {
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
	var txOut btcwire.TxOut
	for i := 0; i < b.N; i++ {
		btcwire.TstReadTxOut(bytes.NewBuffer(buf), 0, 0, &txOut)
	}
}

// BenchmarkWriteTxOut performs a benchmark on how long it takes to write
// a transaction output.
func BenchmarkWriteTxOut(b *testing.B) {
	txOut := blockOne.Transactions[0].TxOut[0]
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteTxOut(ioutil.Discard, 0, 0, txOut)
	}
}

// BenchmarkReadTxIn performs a benchmark on how long it takes to read a
// transaction input.
func BenchmarkReadTxIn(b *testing.B) {
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
	var txIn btcwire.TxIn
	for i := 0; i < b.N; i++ {
		btcwire.TstReadTxIn(bytes.NewBuffer(buf), 0, 0, &txIn)
	}
}

// BenchmarkWriteTxIn performs a benchmark on how long it takes to write
// a transaction input.
func BenchmarkWriteTxIn(b *testing.B) {
	txIn := blockOne.Transactions[0].TxIn[0]
	for i := 0; i < b.N; i++ {
		btcwire.TstWriteTxIn(ioutil.Discard, 0, 0, txIn)
	}
}
