// Copyright (c) 2013-2014 Conformal Systems LLC.
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package btcwire

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

// Size of array used to store sha hashes.  See ShaHash.
const HashSize = 32

// MaxHashStringSize is the maximum length of a ShaHash hash string.
const MaxHashStringSize = HashSize * 2

// ErrHashStrSize describes an error that indicates the caller specified a hash
// string that has too many characters.
var ErrHashStrSize = fmt.Errorf("max hash length is %v chars", MaxHashStringSize)

// ShaHash is used in several of the bitcoin messages and common structures.  It
// typically represents the double sha256 of data.
type ShaHash [HashSize]byte

// String returns the ShaHash in the standard bitcoin big-endian form.
func (hash ShaHash) String() string {
	hashstr := ""
	for i := range hash {
		hashstr += fmt.Sprintf("%02x", hash[HashSize-1-i])
	}

	return hashstr
}

// Bytes returns the bytes which represent the hash as a byte slice.
func (hash *ShaHash) Bytes() []byte {
	newHash := make([]byte, HashSize)
	copy(newHash, hash[:])

	return newHash
}

// SetBytes sets the bytes which represent the hash.  An error is returned if
// the number of bytes passed in is not HashSize.
func (hash *ShaHash) SetBytes(newHash []byte) error {
	nhlen := len(newHash)
	if nhlen != HashSize {
		return fmt.Errorf("invalid sha length of %v, want %v", nhlen,
			HashSize)
	}
	copy(hash[:], newHash[0:HashSize])

	return nil
}

// IsEqual returns true if target is the same as hash.
func (hash *ShaHash) IsEqual(target *ShaHash) bool {
	return bytes.Equal(hash[:], target[:])
}

// NewShaHash returns a new ShaHash from a byte slice.  An error is returned if
// the number of bytes passed in is not HashSize.
func NewShaHash(newHash []byte) (*ShaHash, error) {
	var sh ShaHash
	err := sh.SetBytes(newHash)
	if err != nil {
		return nil, err
	}
	return &sh, err
}

// NewShaHashFromStr converts a hash string in the standard bitcoin big-endian
// form to a ShaHash (which is little-endian).
func NewShaHashFromStr(hash string) (*ShaHash, error) {
	// Return error if hash string is too long.
	if len(hash) > MaxHashStringSize {
		return nil, ErrHashStrSize
	}

	// Hex decoder expects the hash to be a multiple of two.
	if len(hash)%2 != 0 {
		hash = "0" + hash
	}

	// Convert string hash to bytes.
	buf, err := hex.DecodeString(hash)
	if err != nil {
		return nil, err
	}

	// The string was given in big-endian, so reverse the bytes to little
	// endian.
	blen := len(buf)
	for i := 0; i < blen/2; i++ {
		buf[i], buf[blen-1-i] = buf[blen-1-i], buf[i]
	}

	// Make sure the byte slice is the right length by appending zeros to
	// pad it out.
	pbuf := buf
	if HashSize-blen > 0 {
		pbuf = make([]byte, HashSize)
		copy(pbuf, buf)
	}

	// Create the sha hash using the byte slice and return it.
	return NewShaHash(pbuf)
}
