// Copyright (c) 2013 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"math"
)

// Maximum payload size for a variable length integer.
const maxVarIntPayload = 9

// readElement reads the next sequence of bytes from r using little endian
// depending on the concrete type of element pointed to.
func readElement(r io.Reader, element interface{}) error {
	return binary.Read(r, binary.LittleEndian, element)
}

// readElements reads multiple items from r.  It is equivalent to multiple
// calls to readElement.
func readElements(r io.Reader, elements ...interface{}) error {
	for _, element := range elements {
		err := readElement(r, element)
		if err != nil {
			return err
		}
	}
	return nil
}

// writeElement writes the little endian representation of element to w.
func writeElement(w io.Writer, element interface{}) error {
	return binary.Write(w, binary.LittleEndian, element)
}

// writeElements writes multiple items to w.  It is equivalent to multiple
// calls to writeElement.
func writeElements(w io.Writer, elements ...interface{}) error {
	for _, element := range elements {
		err := writeElement(w, element)
		if err != nil {
			return err
		}
	}
	return nil
}

// readVarInt reads a variable length integer from r and returns it as a uint64.
func readVarInt(r io.Reader, pver uint32) (uint64, error) {
	b := make([]byte, 1)
	_, err := io.ReadFull(r, b)
	if err != nil {
		return 0, err
	}

	var rv uint64
	discriminant := uint8(b[0])
	switch discriminant {
	case 0xff:
		var u uint64
		err = binary.Read(r, binary.LittleEndian, &u)
		if err != nil {
			return 0, err
		}
		rv = u

	case 0xfe:
		var u uint32
		err = binary.Read(r, binary.LittleEndian, &u)
		if err != nil {
			return 0, err
		}
		rv = uint64(u)

	case 0xfd:
		var u uint16
		err = binary.Read(r, binary.LittleEndian, &u)
		if err != nil {
			return 0, err
		}
		rv = uint64(u)

	default:
		rv = uint64(discriminant)
	}

	return rv, nil
}

// writeVarInt serializes val to w using a variable number of bytes depending
// on its value.
func writeVarInt(w io.Writer, pver uint32, val uint64) error {
	if val > math.MaxUint32 {
		err := writeElements(w, []byte{0xff}, uint64(val))
		if err != nil {
			return err
		}
		return nil
	}
	if val > math.MaxUint16 {
		err := writeElements(w, []byte{0xfe}, uint32(val))
		if err != nil {
			return err
		}
		return nil
	}
	if val >= 0xfd {
		err := writeElements(w, []byte{0xfd}, uint16(val))
		if err != nil {
			return err
		}
		return nil
	}
	return writeElement(w, uint8(val))
}

// readVarString reads a variable length string from r and returns it as a Go
// string.  A varString is encoded as a varInt containing the length of the
// string, and the bytes that represent the string itself.
func readVarString(r io.Reader, pver uint32) (string, error) {
	slen, err := readVarInt(r, pver)
	if err != nil {
		return "", err
	}
	buf := make([]byte, slen)
	err = readElement(r, buf)
	if err != nil {
		return "", err
	}
	return string(buf), nil
}

// writeVarString serializes str to w as a varInt containing the length of the
// string followed by the bytes that represent the string itself.
func writeVarString(w io.Writer, pver uint32, str string) error {
	err := writeVarInt(w, pver, uint64(len(str)))
	if err != nil {
		return err
	}
	err = writeElement(w, []byte(str))
	if err != nil {
		return err
	}
	return nil
}

// randomUint64 returns a cryptographically random uint64 value.  This
// unexported version takes a reader primarily to ensure the error paths
// can be properly tested by passing a fake reader in the tests.
func randomUint64(r io.Reader) (uint64, error) {
	b := make([]byte, 8)
	n, err := r.Read(b)
	if n != len(b) {
		return 0, io.ErrShortBuffer
	}
	if err != nil {
		return 0, err
	}
	return binary.BigEndian.Uint64(b), nil
}

// RandomUint64 returns a cryptographically random uint64 value.
func RandomUint64() (uint64, error) {
	return randomUint64(rand.Reader)
}

// DoubleSha256 calculates sha256(sha256(b)) and returns the resulting bytes.
func DoubleSha256(b []byte) []byte {
	hasher := sha256.New()
	hasher.Write(b)
	sum := hasher.Sum(nil)
	hasher.Reset()
	hasher.Write(sum)
	return hasher.Sum(nil)
}
