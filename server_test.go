package main

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/peer"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	// logRotator must be non-nil or any log write (e.g. from
	// OnVerAck's double-call guard) panics via logWriter.Write.
	initLogRotator(filepath.Join(os.TempDir(), "btcd-server-test.log"))
	os.Exit(m.Run())
}

// newTestServerPeer creates a minimal serverPeer suitable for unit
// tests that exercise the peer lifecycle logic without starting the
// full server.  The returned server's peerLifecycle channel is
// buffered so the handler never blocks during tests.
func newTestServerPeer(t *testing.T) (*server, *serverPeer) {
	t.Helper()

	s := &server{
		peerLifecycle: make(chan peerLifecycleEvent, 10),
	}
	sp := newServerPeer(s, false)
	sp.Peer = peer.NewInboundPeer(&peer.Config{
		ChainParams: &chaincfg.SimNetParams,
	})

	return s, sp
}

// recvLifecycleEvent reads a single event from the peerLifecycle
// channel or fails the test after a timeout.
func recvLifecycleEvent(
	t *testing.T, ch <-chan peerLifecycleEvent,
) peerLifecycleEvent {

	t.Helper()

	select {
	case ev := <-ch:
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for peerLifecycleEvent")
		return peerLifecycleEvent{}
	}
}

// TestOnVerAckDoubleCall verifies that calling OnVerAck twice on
// the same serverPeer does not panic. The double-call guard must
// log an error and leave verAckCh closed.
func TestOnVerAckDoubleCall(t *testing.T) {
	t.Parallel()

	_, sp := newTestServerPeer(t)
	var releases atomic.Uint32
	sp.releaseInboundHandshake = func() {
		releases.Add(1)
	}

	sp.OnVerAck(nil, nil)

	select {
	case <-sp.verAckCh:
	default:
		t.Fatal("verAckCh should be closed after first OnVerAck call")
	}

	require.NotPanics(t, func() {
		sp.OnVerAck(nil, nil)
	})

	select {
	case <-sp.verAckCh:
	default:
		t.Fatal("verAckCh should still be closed after second OnVerAck call")
	}
	require.Equal(t, uint32(1), releases.Load(),
		"the source-prefix slot must be released exactly once")
}

// TestHandshakeReleaseOnDisconnect verifies that a connection which
// disconnects before verack releases its source-prefix slot.
func TestHandshakeReleaseOnDisconnect(t *testing.T) {
	t.Parallel()

	s, sp := newTestServerPeer(t)
	var releases atomic.Uint32
	sp.releaseInboundHandshake = func() {
		releases.Add(1)
	}

	sp.Disconnect()
	go s.peerLifecycleHandler(sp)

	event := recvLifecycleEvent(t, s.peerLifecycle)
	require.Equal(t, peerDone, event.action)
	require.Equal(t, uint32(1), releases.Load())
}

// TestMaxInboundPeers verifies that automatic outbound capacity is reserved
// without underflow at small peer limits.
func TestMaxInboundPeers(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		maxPeers       int
		targetOutbound int
		want           uint32
	}{
		{name: "zero", maxPeers: 0, targetOutbound: 0, want: 0},
		{name: "all outbound", maxPeers: 8, targetOutbound: 8, want: 0},
		{name: "below outbound", maxPeers: 7, targetOutbound: 8, want: 0},
		{name: "one inbound", maxPeers: 9, targetOutbound: 8, want: 1},
		{name: "default", maxPeers: 125, targetOutbound: 8, want: 117},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.want, maxInboundPeers(
				test.maxPeers, test.targetOutbound,
			))
		})
	}
}

// TestPeerLifecycleOrdering verifies that when verack arrives before
// disconnect, peerLifecycleHandler emits peerAdd followed by peerDone
// on the peerLifecycle channel -- never out of order.
func TestPeerLifecycleOrdering(t *testing.T) {
	t.Parallel()

	s, sp := newTestServerPeer(t)

	// Simulate verack received before the handler starts.
	close(sp.verAckCh)

	go s.peerLifecycleHandler(sp)

	first := recvLifecycleEvent(t, s.peerLifecycle)
	require.Equal(t, peerAdd, first.action,
		"first lifecycle event must be peerAdd")
	require.Equal(t, sp, first.sp)

	// Trigger disconnect after peerAdd is observed.
	sp.Disconnect()

	second := recvLifecycleEvent(t, s.peerLifecycle)
	require.Equal(t, peerDone, second.action,
		"second lifecycle event must be peerDone")
	require.Equal(t, sp, second.sp)
}

// TestPeerLifecycleSimultaneousReady verifies that when both verAckCh
// and Peer.Done() are ready before the handler runs, the system stays
// stable: peerDone is always emitted, and if peerAdd is emitted it
// precedes peerDone. Go's select is nondeterministic so peerAdd may
// be skipped -- both outcomes are valid per documented behavior.
func TestPeerLifecycleSimultaneousReady(t *testing.T) {
	t.Parallel()

	const iterations = 100
	var addEmitted int

	for i := 0; i < iterations; i++ {
		s, sp := newTestServerPeer(t)

		close(sp.verAckCh)
		sp.Disconnect()

		go s.peerLifecycleHandler(sp)

		first := recvLifecycleEvent(t, s.peerLifecycle)
		if first.action == peerAdd {
			addEmitted++
			second := recvLifecycleEvent(t, s.peerLifecycle)
			assert.Equal(t, peerDone, second.action,
				"iteration %d: peerAdd must be "+
					"followed by peerDone", i)
		} else {
			assert.Equal(t, peerDone, first.action,
				"iteration %d: sole event must "+
					"be peerDone", i)
		}
	}

	t.Logf("peerAdd emitted in %d/%d iterations "+
		"(both outcomes are valid per documented behavior)",
		addEmitted, iterations)
}
