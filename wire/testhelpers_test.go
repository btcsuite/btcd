package wire

import (
	"encoding/hex"
)

// mustHex converts a hexadecimal string to a byte slice.
func mustHex(hexStr string) []byte {
	b, err := hex.DecodeString(hexStr)
	if err != nil {
		panic(err)
	}

	return b
}

// mustHexAppend converts hexadecimal strings to a single byte slice.
func mustHexAppend(hexStr ...string) []byte {
	var buf []byte
	for _, s := range hexStr {
		buf = append(buf, mustHex(s)...)
	}

	return buf
}
