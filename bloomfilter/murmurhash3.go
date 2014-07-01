package bloomfilter

import (
	"encoding/binary"
)

// MurmurHash3 implements the MurmurHash3 algorithm.
func MurmurHash3(seed uint32, data []byte) uint32 {
	dataLen := len(data)
	h1 := seed
	c1 := uint32(0xcc9e2d51)
	c2 := uint32(0x1b873593)
	k1 := uint32(0)
	numBlocks := dataLen / 4

	// body
	for i := 0; i < numBlocks; i++ {
		k1 = binary.LittleEndian.Uint32(data[i * 4:])
		k1 *= c1
		k1 = (k1 << 15) | (k1 >> (32 - 15))
		k1 *= c2
		h1 ^= k1
		h1 = (h1 << 13) | (h1 >> (32 - 13))
		h1 = h1*5 + 0xe6546b64
	}

	// tail
	tailidx := numBlocks * 4
	k1 = 0

	switch dataLen & 3 {
	case 3:
		k1 ^= uint32(data[tailidx+2]) << 16
		fallthrough
	case 2:
		k1 ^= uint32(data[tailidx+1]) << 8
		fallthrough
	case 1:
		k1 ^= uint32(data[tailidx])
		k1 *= c1
		k1 = (k1 << 15) | (k1 >> (32 - 15))
		k1 *= c2
		h1 ^= k1
	}

	// Finalization
	h1 ^= uint32(dataLen)
	h1 ^= h1 >> 16
	h1 *= 0x85ebca6b
	h1 ^= h1 >> 13
	h1 *= 0xc2b2ae35
	h1 ^= h1 >> 16

	return h1
}
