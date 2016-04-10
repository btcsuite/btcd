package chaincfg

import (
	"testing"
)

func TestInvalidShaStr(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for invalid sha string, got nil")
		}
	}()
	newShaHashFromStr("banana")
}

// TestInvalidBigIntStrPanic ensures the newBigFromHex function panics when
// called with invalid hex.
func TestInvalidBigIntStrPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected panic for invalid big int string")
		}
	}()
	newBigFromHex("banana")
}
