package descriptors

import "strings"

// The descriptor checksum algorithm is specified in BIP380. It computes an
// 8-character checksum over the descriptor string (excluding the "#checksum"
// suffix) using a BCH code over a custom character grouping.
const (
	// checksumInputCharset is the set of characters allowed in a descriptor
	// string. A character's position in this set determines its value and
	// group for the checksum.
	checksumInputCharset = "0123456789()[],'/*abcdefgh@:$%{}IJKLMNOPQRST" +
		"UVWXYZ&+-.;<=>?!^_|~ijklmnopqrstuvwxyzABCDEFGH`#\"\\ "

	// checksumCharset is the set of characters the checksum itself is
	// encoded with (the bech32 character set).
	checksumCharset = "qpzry9x8gf2tvdw0s3jn54khce6mua7l"

	// checksumLength is the number of characters in a descriptor checksum.
	checksumLength = 8
)

// checksumPolyMod is the BCH code step used by the descriptor checksum, feeding
// one value into the running checksum accumulator.
func checksumPolyMod(c uint64, val int) uint64 {
	c0 := c >> 35
	c = ((c & 0x7ffffffff) << 5) ^ uint64(val)
	if c0&1 != 0 {
		c ^= 0xf5dee51989
	}
	if c0&2 != 0 {
		c ^= 0xa9fdca3312
	}
	if c0&4 != 0 {
		c ^= 0x1bab10e32d
	}
	if c0&8 != 0 {
		c ^= 0x3706b1677a
	}
	if c0&16 != 0 {
		c ^= 0x644d626ffd
	}
	return c
}

// descriptorChecksum computes the 8-character BIP380 checksum for the given
// descriptor string (which must not include a "#checksum" suffix). It returns
// an empty string if the descriptor contains a character outside the allowed
// input character set.
func descriptorChecksum(descriptor string) string {
	var (
		c        uint64 = 1
		cls      int
		clscount int
	)
	for _, ch := range descriptor {
		pos := strings.IndexRune(checksumInputCharset, ch)
		if pos < 0 {
			return ""
		}

		// Emit a symbol for the position inside the group for every
		// character.
		c = checksumPolyMod(c, pos&31)

		// Accumulate the group numbers, emitting an extra symbol for
		// every three characters.
		cls = cls*3 + (pos >> 5)
		clscount++
		if clscount == 3 {
			c = checksumPolyMod(c, cls)
			cls = 0
			clscount = 0
		}
	}
	if clscount > 0 {
		c = checksumPolyMod(c, cls)
	}

	// Shift further to determine the checksum, then flip a bit so that
	// appending zeroes changes the checksum.
	for j := 0; j < checksumLength; j++ {
		c = checksumPolyMod(c, 0)
	}
	c ^= 1

	var sb strings.Builder
	for j := 0; j < checksumLength; j++ {
		sb.WriteByte(checksumCharset[(c>>uint(5*(7-j)))&31])
	}
	return sb.String()
}
