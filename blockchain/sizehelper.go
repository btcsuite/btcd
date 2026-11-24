// Copyright (c) 2023 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
package blockchain

// These constants are related to bitcoin.
const (
	// baseEntrySize is the allocation size of an UtxoEntry, the value is
	// calculated by running the following on a 64-bit system:
	//
	// 	roundupsize(unsafe.Sizeof(UtxoEntry{}))
	//
	// We add 8 because the size of the struct itself is 40 bytes, but we
	// have 8 bytes of overhead for each allocation (which uses a group of
	// 48 bytes in this case). We can reduce this in the future by
	// allocating in batch instead of one allocation per entry in the
	// utxocache.
	baseEntrySize = 40 + 8

	// pubKeyHashLen is the allocation size of a P2PKH script. We add 7
	// because the size of the slice itself is 25 bytes, but we have 7 bytes
	// of overhead for each allocation (uses a group of 32 bytes). We can
	// reduce this in the future by allocating in batch instead of one
	// allocation per entry in the utxocache.
	pubKeyHashLen = 25 + 7

	// avgEntrySize is how much we expect that each entry will allocate.
	// Since most txs are p2pkh, we can assume the entry to be more or less
	// the size of a p2pkh output.
	avgEntrySize = baseEntrySize + pubKeyHashLen

	// groupSize is the size of the map group for a
	// map[wire.OutPoint]*UtxoEntry. It can be calculated with:
	//
	// 	unsafe.Sizeof(struct{
	// 		slots [8]struct{
	//      		key wire.OutPoint
	//      		value *UtxoEntry
	//      	}
	//      	metadata uint64
	// 	}{})
	//
	groupSize = 392
)

const (
	// slotsPerGroup accounts for how many slots a map swiss table group
	// has.
	slotsPerGroup = 8

	// mapLoadFactorNum is the load factor numerator for the Go map.
	mapLoadFactorNum = 7

	// mapLoadFactorDen is the load factor denominator for the Go map.
	mapLoadFactorDen = 9
)

// The code here is shamelessly taken from the go runtime package. All the
// relevant code and variables are copied to here. These values are only
// correct for a 64 bit system.

const (
	_MaxSmallSize   = 32768
	smallSizeDiv    = 8
	smallSizeMax    = 1024
	largeSizeDiv    = 128
	_NumSizeClasses = 68
	_PageShift      = 13
	_PageSize       = 1 << _PageShift

	MaxUintptr = ^uintptr(0)

	// PtrSize is the size of a pointer in bytes - unsafe.Sizeof(uintptr(0))
	// but as an ideal constant. It is also the size of the machine's native
	// word size (that is, 4 on 32-bit systems, 8 on 64-bit).
	PtrSize = 4 << (^uintptr(0) >> 63)
)

var class_to_size = [_NumSizeClasses]uint16{
	0, 8, 16, 24, 32, 48, 64, 80, 96, 112, 128, 144, 160, 176, 192, 208,
	224, 240, 256, 288, 320, 352, 384, 416, 448, 480, 512, 576, 640, 704,
	768, 896, 1024, 1152, 1280, 1408, 1536, 1792, 2048, 2304, 2688, 3072,
	3200, 3456, 4096, 4864, 5376, 6144, 6528, 6784, 6912, 8192, 9472, 9728,
	10240, 10880, 12288, 13568, 14336, 16384, 18432, 19072, 20480, 21760,
	24576, 27264, 28672, 32768,
}

var size_to_class8 = [smallSizeMax/smallSizeDiv + 1]uint8{
	0, 1, 2, 3, 4, 5, 5, 6, 6, 7, 7, 8, 8, 9, 9, 10, 10, 11, 11, 12, 12, 13,
	13, 14, 14, 15, 15, 16, 16, 17, 17, 18, 18, 19, 19, 19, 19, 20, 20, 20,
	20, 21, 21, 21, 21, 22, 22, 22, 22, 23, 23, 23, 23, 24, 24, 24, 24, 25,
	25, 25, 25, 26, 26, 26, 26, 27, 27, 27, 27, 27, 27, 27, 27, 28, 28, 28,
	28, 28, 28, 28, 28, 29, 29, 29, 29, 29, 29, 29, 29, 30, 30, 30, 30, 30,
	30, 30, 30, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31, 31,
	31, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32, 32,
}

var size_to_class128 = [(_MaxSmallSize-smallSizeMax)/largeSizeDiv + 1]uint8{
	32, 33, 34, 35, 36, 37, 37, 38, 38, 39, 39, 40, 40, 40, 41, 41, 41, 42,
	43, 43, 44, 44, 44, 44, 44, 45, 45, 45, 45, 45, 45, 46, 46, 46, 46, 47,
	47, 47, 47, 47, 47, 48, 48, 48, 49, 49, 50, 51, 51, 51, 51, 51, 51, 51,
	51, 51, 51, 52, 52, 52, 52, 52, 52, 52, 52, 52, 52, 53, 53, 54, 54, 54,
	54, 55, 55, 55, 55, 55, 56, 56, 56, 56, 56, 56, 56, 56, 56, 56, 56, 57,
	57, 57, 57, 57, 57, 57, 57, 57, 57, 58, 58, 58, 58, 58, 58, 59, 59, 59,
	59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 59, 60, 60, 60, 60, 60,
	60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 60, 61, 61, 61, 61, 61, 62, 62,
	62, 62, 62, 62, 62, 62, 62, 62, 62, 63, 63, 63, 63, 63, 63, 63, 63, 63,
	63, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64, 64,
	64, 64, 64, 64, 64, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65, 65,
	65, 65, 65, 65, 65, 65, 65, 65, 66, 66, 66, 66, 66, 66, 66, 66, 66, 66,
	66, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67,
	67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67, 67,
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

// Returns size of the memory block that mallocgc will allocate if you ask for
// the size.
func roundupsize(size uintptr) uintptr {
	if size < _MaxSmallSize {
		if size <= smallSizeMax-8 {
			return uintptr(
				class_to_size[size_to_class8[divRoundUp(
					size, smallSizeDiv,
				)]],
			)
		} else {
			return uintptr(
				class_to_size[size_to_class128[divRoundUp(
					size-smallSizeMax,
					largeSizeDiv,
				)]],
			)
		}
	}
	if size+_PageSize < size {
		return size
	}
	return alignUp(size, _PageSize)
}

// alignUpPow2 align N to it's next power of two if it is not a power of 2.
func alignUpPow2(n uint64) uint64 {
	if n == 0 || n > 1<<63 {
		return 0
	}

	for i := uint64(0); i < 64; i++ {
		if n <= 1<<i {
			return 1 << i
		}
	}

	panic("unreachable")
}

// mapSize returns the expected memory usage for a map given its hint and group
// size. See groupSize to discover how to calculate the size of the map group.
func mapSize(hint int, groupSize uint64) uint64 {
	// Can be compared against the map implementation in
	// `go/src/internal/runtime/maps`
	const (
		maxAvgGroupLoad = 7
		maxTableSlots   = 1024

		sizeOfMap         = 8 + 8 + 8 + 8 + 1 + 1 + 1 + 1 + 4 + 8
		sizeOfTable       = 2 + 2 + 2 + 1 + 1 + 8 + 16
		sizeOfDirEntry    = 8 // pointer
		groupMetadataSize = 8 // uint64, 1 byte per entry
	)

	// If the hint is up to 8 entries, then the allocation happens on
	// demand.
	//
	// Only the map header is allocated, conservatively we assume that it
	// escapes to the heap.
	if hint <= slotsPerGroup || groupSize == 0 {
		return uint64(roundupsize(sizeOfMap))
	}

	targetSlots := (hint * slotsPerGroup) / maxAvgGroupLoad

	tableCount := (uint64(targetSlots) + maxTableSlots - 1) / maxTableSlots
	tableCount = alignUpPow2(tableCount)

	tableSlots := uint64(targetSlots) / tableCount
	tableSlots = alignUpPow2(tableSlots)

	tableGroups := tableSlots / slotsPerGroup
	tableGroupsSize := tableGroups * groupSize
	tableGroupsSize = uint64(roundupsize(uintptr(tableGroupsSize)))
	groupsSize := tableGroupsSize * tableCount

	tablePointersSize := tableCount * sizeOfDirEntry
	tablePointersSize = uint64(roundupsize(uintptr(tablePointersSize)))

	tableSize := uint64(roundupsize(sizeOfTable))
	tablesSize := tableCount * tableSize

	mapSize := uint64(roundupsize(sizeOfMap))

	estimatedSize := mapSize + tablePointersSize + tablesSize + groupsSize

	return estimatedSize
}

// hintForSize returns the map hint at the maximum load factor for maxSize,
// meaning that the returned hint allocates a map up to maxSize and hint >> 1
// allocates a map of nearly 50% of the size.
func hintForSize(maxSize, groupSize uint64) int {
	// start with a near guess.
	slots := (maxSize / groupSize) * slotsPerGroup
	slots = alignUpPow2(slots)

	hint := (slots * mapLoadFactorNum) / mapLoadFactorDen

	if hint == 0 || maxSize == 0 {
		return slotsPerGroup
	}

	// Double hint while it fits maxSize.
	for hint < 1<<62 {
		size := mapSize(int(hint), groupSize)

		if size <= maxSize {
			hint <<= 1
			continue
		}

		break
	}

	// Divide hint by 2 until it fits maxSize.
	for hint > 0 {
		size := mapSize(int(hint), groupSize)

		if size > maxSize {
			hint >>= 1
			continue
		}

		break
	}

	// Never return less than slotsPerGroup, as any map will allocate at
	// least a single group when setting a value.
	return max(int(hint), slotsPerGroup)
}
