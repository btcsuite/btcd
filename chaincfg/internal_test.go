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
