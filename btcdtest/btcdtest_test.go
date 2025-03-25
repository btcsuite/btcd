package btcdtest

import (
	"testing"
)

func TestFail(t *testing.T) {
	for i := 0; i < 100; i++ {
		btcd := New(WithDebugLevel("trace"))
		btcd.Stop()
	}
}
