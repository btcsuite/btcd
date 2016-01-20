// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package dcrutil

// bitflags contains a simple series of functions to handle bitwise boolean
// operations in uint16s.

// Flags16 is the type for 2 bytes of flags; not really used except in the
// declaration below.
type Flags16 uint16

// Flag fields for uint16.
const (
	FlagNone   Flags16 = 0
	BlockValid         = 1 << 0 // Describes whether TxTreeRegular is valid
	Flag01             = 1 << 1
	Flag02             = 1 << 2
	Flag03             = 1 << 3
	Flag04             = 1 << 4
	Flag05             = 1 << 5
	Flag06             = 1 << 6
	Flag07             = 1 << 7
	Flag08             = 1 << 8
	Flag09             = 1 << 9
	Flag10             = 1 << 10
	Flag11             = 1 << 11
	Flag12             = 1 << 12
	Flag13             = 1 << 13
	Flag14             = 1 << 14
	Flag15             = 1 << 15
)

// IsFlagSet16 returns true/false for a flag at flag field 'flag'.
func IsFlagSet16(flags uint16, flag uint16) bool {
	return flags&flag != 0
}

// SetFlag16 sets a bit flag at flag position 'flag' to bool 'b'.
func SetFlag16(flags *uint16, flag uint16, b bool) {
	if b {
		*flags = *flags | flag
	} else {
		*flags = *flags &^ flag
	}
}

// BoolArray16 is a bool array that is generated from a uint16 containing flags.
type BoolArray16 [16]bool

// GenerateBoolArray16 generates a BoolArray16 from a uint16 containing flags.
func GenerateBoolArray16(flags uint16) BoolArray16 {
	var ba BoolArray16

	for i := uint8(0); i < 16; i++ {
		ba[i] = (flags&(1<<i) != 0)
	}

	return ba
}
