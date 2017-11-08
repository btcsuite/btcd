// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package gcs

import (
	"io"
)

type bitWriter struct {
	bytes []byte
	p     *byte // Pointer to last byte
	next  byte  // Next bit to write or skip
}

// writeOne writes a one bit to the bit stream.
func (b *bitWriter) writeOne() {
	if b.next == 0 {
		b.bytes = append(b.bytes, 1<<7)
		b.p = &b.bytes[len(b.bytes)-1]
		b.next = 1 << 6
		return
	}

	*b.p |= b.next
	b.next >>= 1
}

// writeZero writes a zero bit to the bit stream.
func (b *bitWriter) writeZero() {
	if b.next == 0 {
		b.bytes = append(b.bytes, 0)
		b.p = &b.bytes[len(b.bytes)-1]
		b.next = 1 << 6
		return
	}

	b.next >>= 1
}

// writeNBits writes n number of LSB bits of data to the bit stream in big
// endian format.  Panics if n > 64.
func (b *bitWriter) writeNBits(data uint64, n uint) {
	if n > 64 {
		panic("gcs: cannot write more than 64 bits of a uint64")
	}

	data <<= 64 - n

	// If byte is partially written, fill the rest
	for n > 0 {
		if b.next == 0 {
			break
		}
		if data&(1<<63) != 0 {
			b.writeOne()
		} else {
			b.writeZero()
		}
		n--
		data <<= 1
	}

	if n == 0 {
		return
	}

	// Write 8 bits at a time.
	for n >= 8 {
		b.bytes = append(b.bytes, byte(data>>56))
		n -= 8
		data <<= 8
	}

	// Write the remaining bits.
	for n > 0 {
		if data&(1<<63) != 0 {
			b.writeOne()
		} else {
			b.writeZero()
		}
		n--
		data <<= 1
	}
}

type bitReader struct {
	bytes []byte
	next  byte // next bit to read in bytes[0]
}

func newBitReader(bitstream []byte) bitReader {
	return bitReader{
		bytes: bitstream,
		next:  1 << 7,
	}
}

// readUnary returns the number of unread sequential one bits before the next
// zero bit.  Errors with io.EOF if no zero bits are encountered.
func (b *bitReader) readUnary() (uint64, error) {
	var value uint64

	for {
		if len(b.bytes) == 0 {
			return value, io.EOF
		}

		for b.next != 0 {
			bit := b.bytes[0] & b.next
			b.next >>= 1
			if bit == 0 {
				return value, nil
			}
			value++
		}

		b.bytes = b.bytes[1:]
		b.next = 1 << 7
	}
}

// readNBits reads n number of LSB bits of data from the bit stream in big
// endian format.  Panics if n > 64.
func (b *bitReader) readNBits(n uint) (uint64, error) {
	if n > 64 {
		panic("gcs: cannot read more than 64 bits as a uint64")
	}

	if len(b.bytes) == 0 {
		return 0, io.EOF
	}

	var value uint64

	// If byte is partially read, read the rest
	if b.next != 1<<7 {
		for n > 0 {
			if b.next == 0 {
				b.next = 1 << 7
				b.bytes = b.bytes[1:]
				break
			}

			n--
			if b.bytes[0]&b.next != 0 {
				value |= 1 << n
			}
			b.next >>= 1
		}
	}

	if n == 0 {
		return value, nil
	}

	// Read 8 bits at a time.
	for n >= 8 {
		if len(b.bytes) == 0 {
			return 0, io.EOF
		}

		n -= 8
		value |= uint64(b.bytes[0]) << n
		b.bytes = b.bytes[1:]
	}

	if len(b.bytes) == 0 {
		if n != 0 {
			return 0, io.EOF
		}
		return value, nil
	}

	// Read the remaining bits.
	for n > 0 {
		if b.next == 0 {
			b.bytes = b.bytes[1:]
			if len(b.bytes) == 0 {
				return 0, io.EOF
			}
			b.next = 1 << 7
		}

		n--
		if b.bytes[0]&b.next != 0 {
			value |= 1 << n
		}
		b.next >>= 1
	}

	return value, nil
}
