package peer

import (
	"sync/atomic"
	"testing"
)

// TestRecoverFromPanic verifies that recoverFromPanic catches a panic
// and disconnects the peer instead of crashing the process.
func TestRecoverFromPanic(t *testing.T) {
	t.Parallel()

	// Build a minimal Peer with a closed quit channel so
	// Disconnect() does not block or nil-deref.
	p := &Peer{
		quit: make(chan struct{}),
	}

	// Simulate a goroutine that panics.
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer p.recoverFromPanic()

		panic("test: crafted message decode")
	}()

	<-done

	// After recovery, the disconnect flag must be set.
	if atomic.LoadInt32(&p.disconnect) == 0 {
		t.Fatal("expected disconnect flag to be set " +
			"after panic recovery")
	}
}
