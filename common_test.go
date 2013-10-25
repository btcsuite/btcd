// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire_test

import (
	"bytes"
	"fmt"
	"github.com/conformal/btcwire"
	"github.com/davecgh/go-spew/spew"
	"io"
	"reflect"
	"strings"
	"testing"
)

// fakeRandReader implements the io.Reader interface and is used to force
// errors in the RandomUint64 function.
type fakeRandReader struct {
	n   int
	err error
}

// Read returns the fake reader error and the lesser of the fake reader value
// and the length of p.
func (r *fakeRandReader) Read(p []byte) (int, error) {
	n := r.n
	if n > len(p) {
		n = len(p)
	}
	return n, r.err
}

// TestVarIntWire tests wire encode and decode for variable length integers.
func TestVarIntWire(t *testing.T) {
	pver := btcwire.ProtocolVersion

	tests := []struct {
		in   uint64 // Value to encode
		out  uint64 // Expected decoded value
		buf  []byte // Wire encoding
		pver uint32 // Protocol version for wire encoding
	}{
		// Latest protocol version.
		// Single byte
		{0, 0, []byte{0x00}, pver},
		// Max single byte
		{0xfc, 0xfc, []byte{0xfc}, pver},
		// Min 2-byte
		{0xfd, 0xfd, []byte{0xfd, 0x0fd, 0x00}, pver},
		// Max 2-byte
		{0xffff, 0xffff, []byte{0xfd, 0xff, 0xff}, pver},
		// Min 4-byte
		{0x10000, 0x10000, []byte{0xfe, 0x00, 0x00, 0x01, 0x00}, pver},
		// Max 4-byte
		{0xffffffff, 0xffffffff, []byte{0xfe, 0xff, 0xff, 0xff, 0xff}, pver},
		// Min 8-byte
		{
			0x100000000, 0x100000000,
			[]byte{0xff, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00},
			pver,
		},
		// Max 8-byte
		{
			0xffffffffffffffff, 0xffffffffffffffff,
			[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			pver,
		},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		var buf bytes.Buffer
		err := btcwire.TstWriteVarInt(&buf, test.pver, test.in)
		if err != nil {
			t.Errorf("writeVarInt #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("writeVarInt #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode from wire format.
		rbuf := bytes.NewBuffer(test.buf)
		val, err := btcwire.TstReadVarInt(rbuf, test.pver)
		if err != nil {
			t.Errorf("readVarInt #%d error %v", i, err)
			continue
		}
		if val != test.out {
			t.Errorf("readVarInt #%d\n got: %d want: %d", i,
				val, test.out)
			continue
		}
	}
}

// TestVarIntWireErrors performs negative tests against wire encode and decode
// of variable length integers to confirm error paths work correctly.
func TestVarIntWireErrors(t *testing.T) {
	pver := btcwire.ProtocolVersion

	tests := []struct {
		in       uint64 // Value to encode
		buf      []byte // Wire encoding
		pver     uint32 // Protocol version for wire encoding
		max      int    // Max size of fixed buffer to induce errors
		writeErr error  // Expected write error
		readErr  error  // Expected read error
	}{
		// Force errors on discriminant.
		{0, []byte{0x00}, pver, 0, io.ErrShortWrite, io.EOF},
		// Force errors on 2-byte read/write.
		{0xfd, []byte{0xfd}, pver, 2, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force errors on 4-byte read/write.
		{0x10000, []byte{0xfe}, pver, 2, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force errors on 8-byte read/write.
		{0x100000000, []byte{0xff}, pver, 2, io.ErrShortWrite, io.ErrUnexpectedEOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := btcwire.TstWriteVarInt(w, test.pver, test.in)
		if err != test.writeErr {
			t.Errorf("writeVarInt #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Decode from wire format.
		r := newFixedReader(test.max, test.buf)
		_, err = btcwire.TstReadVarInt(r, test.pver)
		if err != test.readErr {
			t.Errorf("readVarInt #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestVarStringWire tests wire encode and decode for variable length strings.
func TestVarStringWire(t *testing.T) {
	pver := btcwire.ProtocolVersion

	// str256 is a string that takes a 2-byte varint to encode.
	str256 := strings.Repeat("test", 64)

	tests := []struct {
		in   string // String to encode
		out  string // String to decoded value
		buf  []byte // Wire encoding
		pver uint32 // Protocol version for wire encoding
	}{
		// Latest protocol version.
		// Empty string
		{"", "", []byte{0x00}, pver},
		// Single byte varint + string
		{"Test", "Test", append([]byte{0x04}, []byte("Test")...), pver},
		// 2-byte varint + string
		{str256, str256, append([]byte{0xfd, 0x00, 0x01}, []byte(str256)...), pver},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		var buf bytes.Buffer
		err := btcwire.TstWriteVarString(&buf, test.pver, test.in)
		if err != nil {
			t.Errorf("writeVarString #%d error %v", i, err)
			continue
		}
		if !bytes.Equal(buf.Bytes(), test.buf) {
			t.Errorf("writeVarString #%d\n got: %s want: %s", i,
				spew.Sdump(buf.Bytes()), spew.Sdump(test.buf))
			continue
		}

		// Decode from wire format.
		rbuf := bytes.NewBuffer(test.buf)
		val, err := btcwire.TstReadVarString(rbuf, test.pver)
		if err != nil {
			t.Errorf("readVarString #%d error %v", i, err)
			continue
		}
		if val != test.out {
			t.Errorf("readVarString #%d\n got: %d want: %d", i,
				val, test.out)
			continue
		}
	}
}

// TestVarStringWireErrors performs negative tests against wire encode and
// decode of variable length strings to confirm error paths work correctly.
func TestVarStringWireErrors(t *testing.T) {
	pver := btcwire.ProtocolVersion

	// str256 is a string that takes a 2-byte varint to encode.
	str256 := strings.Repeat("test", 64)

	tests := []struct {
		in       string // Value to encode
		buf      []byte // Wire encoding
		pver     uint32 // Protocol version for wire encoding
		max      int    // Max size of fixed buffer to induce errors
		writeErr error  // Expected write error
		readErr  error  // Expected read error
	}{
		// Latest protocol version with intentional read/write errors.
		// Force errors on empty string.
		{"", []byte{0x00}, pver, 0, io.ErrShortWrite, io.EOF},
		// Force error on single byte varint + string.
		{"Test", []byte{0x04}, pver, 2, io.ErrShortWrite, io.ErrUnexpectedEOF},
		// Force errors on 2-byte varint + string.
		{str256, []byte{0xfd}, pver, 2, io.ErrShortWrite, io.ErrUnexpectedEOF},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Encode to wire format.
		w := newFixedWriter(test.max)
		err := btcwire.TstWriteVarString(w, test.pver, test.in)
		if err != test.writeErr {
			t.Errorf("writeVarString #%d wrong error got: %v, want: %v",
				i, err, test.writeErr)
			continue
		}

		// Decode from wire format.
		r := newFixedReader(test.max, test.buf)
		_, err = btcwire.TstReadVarString(r, test.pver)
		if err != test.readErr {
			t.Errorf("readVarString #%d wrong error got: %v, want: %v",
				i, err, test.readErr)
			continue
		}
	}
}

// TestVarStringOverflowErrors performs tests to ensure deserializing variable
// length strings intentionally crafted to use large values for the string
// length are handled properly.  This could otherwise potentially be used as an
// attack vector.
func TestVarStringOverflowErrors(t *testing.T) {
	pver := btcwire.ProtocolVersion

	tests := []struct {
		buf  []byte // Wire encoding
		pver uint32 // Protocol version for wire encoding
		err  error  // Expected error
	}{
		{[]byte{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff},
			pver, &btcwire.MessageError{}},
		{[]byte{0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x01},
			pver, &btcwire.MessageError{}},
	}

	t.Logf("Running %d tests", len(tests))
	for i, test := range tests {
		// Decode from wire format.
		rbuf := bytes.NewBuffer(test.buf)
		_, err := btcwire.TstReadVarString(rbuf, test.pver)
		if reflect.TypeOf(err) != reflect.TypeOf(test.err) {
			t.Errorf("readVarString #%d wrong error got: %v, "+
				"want: %v", i, err, reflect.TypeOf(test.err))
			continue
		}
	}

}

// TestRandomUint64 exercises the randomness of the random number generator on
// the system by ensuring the probability of the generated numbers.  If the RNG
// is evenly distributed as a proper cryptographic RNG should be, there really
// should only be 1 number < 2^56 in 2^8 tries for a 64-bit number.  However,
// use a higher number of 5 to really ensure the test doesn't fail unless the
// RNG is just horrendous.
func TestRandomUint64(t *testing.T) {
	tries := 1 << 8              // 2^8
	watermark := uint64(1 << 56) // 2^56
	maxHits := 5
	badRNG := "The random number generator on this system is clearly " +
		"terrible since we got %d values less than %d in %d runs " +
		"when only %d was expected"

	numHits := 0
	for i := 0; i < tries; i++ {
		nonce, err := btcwire.RandomUint64()
		if err != nil {
			t.Errorf("RandomUint64 iteration %d failed - err %v",
				i, err)
			return
		}
		if nonce < watermark {
			numHits++
		}
		if numHits > maxHits {
			str := fmt.Sprintf(badRNG, numHits, watermark, tries, maxHits)
			t.Errorf("Random Uint64 iteration %d failed - %v %v", i,
				str, numHits)
			return
		}
	}
}

// TestRandomUint64Errors uses a fake reader to force error paths to be executed
// and checks the results accordingly.
func TestRandomUint64Errors(t *testing.T) {
	// Test short reads.
	fr := &fakeRandReader{n: 2, err: nil}
	nonce, err := btcwire.TstRandomUint64(fr)
	if err != io.ErrShortBuffer {
		t.Errorf("TestRandomUint64Fails: Error not expected value of %v [%v]",
			io.ErrShortBuffer, err)
	}
	if nonce != 0 {
		t.Errorf("TestRandomUint64Fails: nonce is not 0 [%v]", nonce)
	}

	// Test err with full read.
	fr = &fakeRandReader{n: 20, err: io.ErrClosedPipe}
	nonce, err = btcwire.TstRandomUint64(fr)
	if err != io.ErrClosedPipe {
		t.Errorf("TestRandomUint64Fails: Error not expected value of %v [%v]",
			io.ErrClosedPipe, err)
	}
	if nonce != 0 {
		t.Errorf("TestRandomUint64Fails: nonce is not 0 [%v]", nonce)
	}
}
