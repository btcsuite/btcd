package descriptors

import (
	"encoding/hex"

	"github.com/btcsuite/btcd/btcec/v2"
)

// testCompressedKeys returns n deterministic compressed-key encodings as hex
// strings. For n <= 255, these are distinct valid keys derived from scalars
// 1..n. Larger counts wrap the one-byte scalar, including zero, and are only
// suitable for tests that expect descriptor rejection.
func testCompressedKeys(n int) []string {
	keys := make([]string, n)
	for i := range keys {
		privBytes := make([]byte, 32)
		privBytes[31] = byte(i + 1)
		_, pub := btcec.PrivKeyFromBytes(privBytes)
		keys[i] = hex.EncodeToString(pub.SerializeCompressed())
	}

	return keys
}
