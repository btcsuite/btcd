// Copyright (c) 2013, 2014 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package bloom

import (
	"encoding/binary"
)

// The following constants are used by the MurmurHash3 algorithm.
const (
	murmurC1 = 0xcc9e2d51
	murmurC2 = 0x1b873593
	murmurR1 = 15
	murmurR2 = 13
	murmurM  = 5
	murmurN  = 0xe6546b64
)

// MurmurHash3 implements a non-cryptographic hash function using the
// MurmurHash3 algorithm.  This implementation yields a 32-bit hash value which
// is suitable for general hash-based lookups.  The seed can be used to
// effectively randomize the hash function.  This makes it ideal for use in
// bloom filters which need multiple independent hash functions.
func MurmurHash3(seed uint32, data []byte) uint32 {
	dataLen := uint32(len(data))
	hash := seed
	k := uint32(0)
	numBlocks := dataLen / 4

	// Calculate the hash in 4-byte chunks.
	for i := uint32(0); i < numBlocks; i++ {
		k = binary.LittleEndian.Uint32(data[i*4:])
		k *= murmurC1
		k = (k << murmurR1) | (k >> (32 - murmurR1))
		k *= murmurC2

		hash ^= k
		hash = (hash << murmurR2) | (hash >> (32 - murmurR2))
		hash = hash*murmurM + murmurN
	}

	// Handle remaining bytes.
	tailIdx := numBlocks * 4
	k = 0

	switch dataLen & 3 {
	case 3:
		k ^= uint32(data[tailIdx+2]) << 16
		fallthrough
	case 2:
		k ^= uint32(data[tailIdx+1]) << 8
		fallthrough
	case 1:
		k ^= uint32(data[tailIdx])
		k *= murmurC1
		k = (k << murmurR1) | (k >> (32 - murmurR1))
		k *= murmurC2
		hash ^= k
	}

	// Finalization.
	hash ^= dataLen
	hash ^= hash >> 16
	hash *= 0x85ebca6b
	hash ^= hash >> 13
	hash *= 0xc2b2ae35
	hash ^= hash >> 16

	return hash
}
