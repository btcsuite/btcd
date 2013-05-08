// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
This test file is part of the btcwire package rather than than the
btcwire_test package so it can bridge access to the internals to properly test
cases which are either not possible or can't reliably be tested via the public
interface.  The functions are only exported while the tests are being run.
*/

package btcwire

import (
	"io"
)

// TstRandomUint64 makes the internal randomUint64 function available to the
// test package.
func TstRandomUint64(r io.Reader) (uint64, error) {
	return randomUint64(r)
}

// TstReadVarInt makes the internal readVarInt function available to the
// test package.
func TstReadVarInt(r io.Reader, pver uint32) (uint64, error) {
	return readVarInt(r, pver)
}

// TstWriteVarInt makes the internal writeVarInt function available to the
// test package.
func TstWriteVarInt(w io.Writer, pver uint32, val uint64) error {
	return writeVarInt(w, pver, val)
}

// TstReadVarString makes the internal readVarString function available to the
// test package.
func TstReadVarString(r io.Reader, pver uint32) (string, error) {
	return readVarString(r, pver)
}

// TstWriteVarString makes the internal writeVarString function available to the
// test package.
func TstWriteVarString(w io.Writer, pver uint32, str string) error {
	return writeVarString(w, pver, str)
}

// TstReadNetAddress makes the internal readNetAddress function available to
// the test package.
func TstReadNetAddress(r io.Reader, pver uint32, na *NetAddress, ts bool) error {
	return readNetAddress(r, pver, na, ts)
}

// TstWriteNetAddress makes the internal writeNetAddress function available to
// the test package.
func TstWriteNetAddress(w io.Writer, pver uint32, na *NetAddress, ts bool) error {
	return writeNetAddress(w, pver, na, ts)
}

// TstMaxNetAddressPayload makes the internal maxNetAddressPayload function
// available to the test package.
func TstMaxNetAddressPayload(pver uint32) uint32 {
	return maxNetAddressPayload(pver)
}

// TstReadInvVect makes the internal readInvVect function available to the test
// package.
func TstReadInvVect(r io.Reader, pver uint32, iv *InvVect) error {
	return readInvVect(r, pver, iv)
}

// TstWriteInvVect makes the internal writeInvVect function available to the
// test package.
func TstWriteInvVect(w io.Writer, pver uint32, iv *InvVect) error {
	return writeInvVect(w, pver, iv)
}

// TstReadBlockHeader makes the internal readBlockHeader function available to
// the test package.
func TstReadBlockHeader(r io.Reader, pver uint32, bh *BlockHeader) error {
	return readBlockHeader(r, pver, bh)
}

// TstWriteBlockHeader makes the internal writeBlockHeader function available to
// the test package.
func TstWriteBlockHeader(w io.Writer, pver uint32, bh *BlockHeader) error {
	return writeBlockHeader(w, pver, bh)
}

// TstReadMessage makes the internal readMessage function available to
// the test package.
func TstReadMessage(r io.Reader, pver uint32, hdr *messageHeader) (Message, []byte, error) {
	return readMessage(r, pver, hdr)
}

// TstReadMessageHeader makes the internal readMessageHeader function available
// to the test package.
func TstReadMessageHeader(r io.Reader) (*messageHeader, error) {
	return readMessageHeader(r)
}
