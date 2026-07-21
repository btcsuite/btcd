package main

import (
	"net"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/internal/inbound"
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

// TestInboundPeerReservation verifies that listener capacity is derived from
// the configured peer mode while connmgr retains its automatic target.
func TestInboundPeerReservation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		maxPeers       int
		permanentPeers int
		automatic      bool
		wantTarget     int
		wantReserved   int
		wantInbound    uint32
	}{
		{
			name: "connect only", maxPeers: 8,
			permanentPeers: 1, wantReserved: 1, wantInbound: 7,
		},
		{
			name: "connect only capped", maxPeers: 8,
			permanentPeers: 10,
			wantReserved:   8, wantInbound: 0,
		},
		{
			name: "simnet without peers", maxPeers: 8, wantReserved: 0,
			wantInbound: 8,
		},
		{
			name: "simnet with peers", maxPeers: 8, permanentPeers: 3,
			wantReserved: 3, wantInbound: 5,
		},
		{
			name: "automatic without add peers", maxPeers: 125,
			automatic: true, wantTarget: 8,
			wantReserved: 8, wantInbound: 117,
		},
		{
			name: "add peers below target", maxPeers: 125,
			permanentPeers: 3, automatic: true, wantTarget: 8,
			wantReserved: 11, wantInbound: 114,
		},
		{
			name: "add peers above target", maxPeers: 10,
			permanentPeers: 9, automatic: true, wantTarget: 1,
			wantReserved: 10, wantInbound: 0,
		},
		{
			name: "add peers at max peers", maxPeers: 10,
			permanentPeers: 10, automatic: true,
			wantReserved: 10, wantInbound: 0,
		},
		{
			name: "add peers above max peers", maxPeers: 10,
			permanentPeers: 12, automatic: true,
			wantReserved: 10, wantInbound: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			targetOutbound := targetOutboundPeers(
				test.maxPeers, test.permanentPeers,
				test.automatic,
			)
			require.Equal(t, test.wantTarget, targetOutbound)

			reserved := reservedOutboundPeers(
				test.maxPeers, targetOutbound,
				test.permanentPeers, test.automatic,
			)
			require.Equal(t, test.wantReserved, reserved)
			require.Equal(t, test.wantInbound, maxInboundPeers(
				test.maxPeers, reserved,
			))
		})
	}
}

// TestInboundPeerAdmissionSourceLimits verifies that loopback and whitelisted
// peers retain the ordinary pending-handshake and V2 source limits.
func TestInboundPeerAdmissionSourceLimits(t *testing.T) {
	_, whitelist, err := net.ParseCIDR("192.0.2.0/24")
	require.NoError(t, err)

	originalCfg := cfg
	t.Cleanup(func() {
		cfg = originalCfg
	})

	tests := []struct {
		name            string
		addr            net.Addr
		whitelists      []*net.IPNet
		wantWhitelisted bool
	}{
		{
			name: "loopback",
			addr: &net.TCPAddr{
				IP: net.ParseIP("127.0.0.2"), Port: 8333,
			},
		},
		{
			name: "whitelisted",
			addr: &net.TCPAddr{
				IP: net.ParseIP("192.0.2.1"), Port: 8333,
			},
			whitelists:      []*net.IPNet{whitelist},
			wantWhitelisted: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			cfg = &config{whitelists: test.whitelists}
			s := &server{inboundAdmission: inbound.New()}

			var releases []func()
			for i := 0; i < 20; i++ {
				whitelisted, release, _, err :=
					s.acquireInboundPeerAdmission(test.addr)
				if err != nil {
					break
				}

				require.Equal(
					t, test.wantWhitelisted, whitelisted,
				)
				releases = append(releases, release)
			}
			require.Less(t, len(releases), 20,
				"the source pending limit must reject a peer")
			for _, release := range releases {
				release()
			}

			var v2Rejections int
			for i := 0; i < 20; i++ {
				whitelisted, releaseSource, v2Admission, err :=
					s.acquireInboundPeerAdmission(test.addr)
				require.NoError(t, err)
				require.Equal(
					t, test.wantWhitelisted, whitelisted,
				)
				releaseSource()

				releaseV2, err := v2Admission.Acquire()
				if err != nil {
					v2Rejections++
					break
				}
				releaseV2()

				releaseV2, err = v2Admission.Acquire()
				require.NoError(t, err,
					"the second CPU phase must not consume "+
						"another rate token")
				releaseV2()
			}
			require.Equal(t, 1, v2Rejections,
				"the V2 source rate must reject a peer")
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
