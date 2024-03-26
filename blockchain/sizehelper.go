// Copyright (c) 2023 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
package blockchain

import (
	"math"
)

// These constants are related to bitcoin.
const (
	// outpointSize is the size of an outpoint.
	//
	// This value is calculated by running the following:
	//	unsafe.Sizeof(wire.OutPoint{})
	outpointSize = 36

	// uint64Size is the size of an uint64 allocated in memory.
	uint64Size = 8

	// bucketSize is the size of the bucket in the cache map.  Exact
	// calculation is (16 + keysize*8 + valuesize*8) where for the map of:
	// map[wire.OutPoint]*UtxoEntry would have a keysize=36 and valuesize=8.
	//
	// https://github.com/golang/go/issues/34561#issuecomment-536115805
	bucketSize = 16 + uint64Size*outpointSize + uint64Size*uint64Size

	// This value is calculated by running the following on a 64-bit system:
	//   unsafe.Sizeof(UtxoEntry{})
	baseEntrySize = 40

	// pubKeyHashLen is the length of a P2PKH script.
	pubKeyHashLen = 25

	// avgEntrySize is how much each entry we expect it to be.  Since most
	// txs are p2pkh, we can assume the entry to be more or less the size
	// of a p2pkh tx.  We add on 7 to make it 32 since 64 bit systems will
	// align by 8 bytes.
	avgEntrySize = baseEntrySize + (pubKeyHashLen + 7)
)

// The code here is shamelessly taken from the go runtime package.  All the relevant
// code and variables are copied to here.  These values are only correct for a 64 bit
// system.

const (
	_MaxSmallSize   = 32768
	smallSizeDiv    = 8
	smallSizeMax    = 1024
	largeSizeDiv    = 128
	_NumSizeClasses = 68
	_PageShift      = 13
	_PageSize       = 1 << _PageShift

	MaxUintptr = ^uintptr(0)

	// Maximum number of key/elem pairs a bucket can hold.
	bucketCntBits = 3
	bucketCnt     = 1 << bucketCntBits

	// Maximum average load of a bucket that triggers growth is 6.5.
	// Represent as loadFactorNum/loadFactorDen, to allow integer math.
	loadFactorNum = 13
	loadFactorDen = 2

	// _64bit = 1 on 64-bit systems, 0 on 32-bit systems
	_64bit = 1 << (^uintptr(0) >> 63) / 2

	// PtrSize is the size of a pointer in bytes - unsafe.Sizeof(uintptr(0))
	// but as an ideal constant. It is also the size of the machine's native
	// word size (that is, 4 on 32-bit systems, 8 on 64-bit).
	PtrSize = 4 << (^uintptr(0) >> 63)

	// heapAddrBits is the number of bits in a heap address that's actually
	// available for memory allocation.
	//
	// NOTE (guggero): For 64-bit systems, we just assume 40 bits of address
	// space available, as that seems to be the lowest common denominator.
	// See heapAddrBits in runtime/malloc.go of the standard library for
	// more details
	heapAddrBits = 32 + (_64bit * 8)

	// maxAlloc is the maximum size of an allocation on the heap.
	//
	// NOTE(guggero): With the somewhat simplified heapAddrBits calculation
	// above, this will currently limit the maximum allocation size of the
	// UTXO cache to around 300GiB on 64-bit systems. This should be more
	// than enough for the foreseeable future, but if we ever need to
	// increase it, we should probably use the same calculation as the
	// standard library.
	maxAlloc = (1 << heapAddrBits) - (1-_64bit)*1
)

var class_to_size = [_NumSizeClasses]uint16{0, 8, 16, 24, 32, 48, 64, 80, 96, 112, 128, 144, 160, 176, 192, 208, 224, 240, 256, 288, 320, 352, 384, 416, 448, 480, 512, 576, 640, 704, 768, 896, 1024, 1152, 1280, 1408, 1536, 1792, 2048, 2304, 2688, 3072, 3200, 3456, 4096, 4864, 5376, 6144, 6528, 6784, 6912, 8192, 9472, 9728, 10240, 10880, 12288, 13568, 14336, 16384, 18432, 19072, 20480, 21760, 24576, 27264, 28672, 32768}
var size_to_class8 = [smallSizeMax/smallSizeDiv + 1]uint8{0, 1, 2, 3, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13, 13, 14, 14, 15, 15, 16, 16, 17, 17, 18, 18, 19, 19, 19, 19, 20, 20, 20, 20, 21, 21, 21, 21, 22, 22, 22, 22, 23, 23, 23, 23, 24, 24, 24, 24, 25, 25, 25, 25, 26, 26, 26, 26, 27, 27, 27, 27, 27, 27, 27, 27, 28, 28, 28, 28, 28, 28, 28, 28, 29, 29, 29, 29, 29, 29, 29, 29, 30, 30, 30, 30, 30, 30, 30, 30, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32}
var size_to_class128 = [(_MaxSmallSize-smallSizeMax)/largeSizeDiv + 1]uint8{32, 33, 34, 35, 36, 37, 37, 38, 38, 39, 39, 40, 40, 40, 41, 41, 41, 42, 43, 43, 44, 44, 44, 44, 44, 45, 45, 45, 45, 45, 45, 46, 46, 46, 46, 47, 47, 47, 47, 47, 47, 48, 48, 48, 49, 49, 50, 51, 51, 51, 51, 51, 51, 51, 51, 51, 51, 52, 52, 52, 52, 52, 52, 52, 52, 52, 52, 53, 53, 54, 54, 54, 54, 55, 55, 55, 55, 55, 56, 56, 56, 56, 56, 56, 56, 56, 56, 56, 56, 57, 57, 57, 57, 57, 57, 57, 57, 57, 57, 58, 58, 58, 58, 58, 58, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 61, 61, 61, 61, 61, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 62, 63, 63, 63, 63, 63, 63, 63, 63, 63, 63, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67}

// calculateRoughMapSize returns a close enough estimate of the
// total memory allocated by a map.
// hint should be the same value as the number you give when
// making a map with the following syntax: make(map[k]v, hint)
//
// bucketsize is (16 + keysize*8 + valuesize*8).  For a map of:
// map[int64]int64, keysize=8 and valuesize=8.  There are edge cases
// where the bucket size is different that I can't find the source code
// for. https://github.com/golang/go/issues/34561#issuecomment-536115805
//
// I suspect it's because of alignment and how the compiler handles it but
// when compared with how much the compiler allocates, it's a couple hundred
// bytes off.
func calculateRoughMapSize(hint int, bucketSize uintptr) int {
	// This code is copied from makemap() in runtime/map.go.
	//
	// TODO check once in a while to see if this algorithm gets
	// changed.
	mem, overflow := mulUintptr(uintptr(hint), uintptr(bucketSize))
	if overflow || mem > maxAlloc {
		hint = 0
	}

	// Find the size parameter B which will hold the requested # of elements.
	// For hint < 0 overLoadFactor returns false since hint < bucketCnt.
	B := uint8(0)
	for overLoadFactor(hint, B) {
		B++
	}

	// This code is copied from makeBucketArray() in runtime/map.go.
	//
	// TODO check once in a while to see if this algorithm gets
	// changed.
	//
	// For small b, overflow buckets are unlikely.
	// Avoid the overhead of the calculation.
	base := bucketShift(B)
	numBuckets := base
	if B >= 4 {
		// Add on the estimated number of overflow buckets
		// required to insert the median number of elements
		// used with this value of b.
		numBuckets += bucketShift(B - 4)
		sz := bucketSize * numBuckets
		up := roundupsize(sz)
		if up != sz {
			numBuckets = up / bucketSize
		}
	}
	total, _ := mulUintptr(bucketSize, numBuckets)

	if base != numBuckets {
		// Add 24 for mapextra struct overhead. Refer to
		// runtime/map.go in the std library for the struct.
		total += 24
	}

	// 48 is the number of bytes needed for the map header in a
	// 64 bit system. Refer to hmap in runtime/map.go in the go
	// standard library.
	total += 48
	return int(total)
}

// calculateMinEntries returns the minimum number of entries that will make the
// map allocate the given total bytes.  -1 on the returned entry count will
// make the map allocate half as much total bytes (for returned entry count that's
// greater than 0).
func calculateMinEntries(totalBytes int, bucketSize int) int {
	// 48 is the number of bytes needed for the map header in a
	// 64 bit system. Refer to hmap in runtime/map.go in the go
	// standard library.
	totalBytes -= 48

	numBuckets := totalBytes / bucketSize
	B := uint8(math.Log2(float64(numBuckets)))
	if B < 4 {
		switch B {
		case 0:
			return 0
		case 1:
			return 9
		case 2:
			return 14
		default:
			return 27
		}
	}

	B -= 1

	return (int(loadFactorNum * (bucketShift(B) / loadFactorDen))) + 1
}

// mulUintptr returns a * b and whether the multiplication overflowed.
// On supported platforms this is an intrinsic lowered by the compiler.
func mulUintptr(a, b uintptr) (uintptr, bool) {
	if a|b < 1<<(4*PtrSize) || a == 0 {
		return a * b, false
	}
	overflow := b > MaxUintptr/a
	return a * b, overflow
}

// divRoundUp returns ceil(n / a).
func divRoundUp(n, a uintptr) uintptr {
	// a is generally a power of two. This will get inlined and
	// the compiler will optimize the division.
	return (n + a - 1) / a
}

// alignUp rounds n up to a multiple of a. a must be a power of 2.
func alignUp(n, a uintptr) uintptr {
	return (n + a - 1) &^ (a - 1)
}

// Returns size of the memory block that mallocgc will allocate if you ask for the size.
func roundupsize(size uintptr) uintptr {
	if size < _MaxSmallSize {
		if size <= smallSizeMax-8 {
			return uintptr(class_to_size[size_to_class8[divRoundUp(size, smallSizeDiv)]])
		} else {
			return uintptr(class_to_size[size_to_class128[divRoundUp(size-smallSizeMax, largeSizeDiv)]])
		}
	}
	if size+_PageSize < size {
		return size
	}
	return alignUp(size, _PageSize)
}

// overLoadFactor reports whether count items placed in 1<<B buckets is over loadFactor.
func overLoadFactor(count int, B uint8) bool {
	return count > bucketCnt && uintptr(count) > loadFactorNum*(bucketShift(B)/loadFactorDen)
}

// bucketShift returns 1<<b, optimized for code generation.
func bucketShift(b uint8) uintptr {
	// Masking the shift amount allows overflow checks to be elided.
	return uintptr(1) << (b & (8*8 - 1))
}
