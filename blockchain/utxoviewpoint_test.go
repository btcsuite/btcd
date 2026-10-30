package blockchain

import "testing"

// TestUtxoEntryMemoryUsage asserts that the memory usage of an UtxoEntry is
// being calculated correctly, that is, it accounts for the allocation overhead
// of the scriptPubKey byte slice.
func TestUtxoEntryMemoryUsage(t *testing.T) {
	tests := []struct {
		name             string
		scriptPubKeySize uintptr
		wantSize         uintptr
	}{
		{"0 byte scriptPubKey", 0, baseEntrySize},
		{"1 byte scriptPubKey", 1, baseEntrySize + 8},
		{"25 byte scriptPubKey", 25, baseEntrySize + 32},
		{"65 byte scriptPubKey", 65, baseEntrySize + 80},
	}

	for _, test := range tests {
		entry := UtxoEntry{
			pkScript: make([]byte, test.scriptPubKeySize),
		}

		memUsage := entry.memoryUsage()
		if test.wantSize != uintptr(memUsage) {
			t.Errorf("Fail test %s; wantMemUsage %d got %d",
				test.name, test.wantSize, memUsage)
		}
	}
}
