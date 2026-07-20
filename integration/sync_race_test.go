//go:build rpctest
// +build rpctest

package integration

import (
	"fmt"
	"math/rand"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/stretchr/testify/require"
)

const (
	syncRaceIterations           = 1000
	syncRaceConcurrency          = 300
	syncRaceHandshakeConcurrency = 8
	syncRaceRegistrationWait     = 15 * time.Second
	syncRaceRunDuration          = 90 * time.Second
	syncRaceProofBlocks          = 5
	syncRaceProofWait            = 8 * time.Second
)

// fakePeerConn connects to the node at nodeAddr, performs the
// minimum version/verack handshake so the node registers a peer
// (NewPeer) and then disconnects so the node runs DonePeer. This
// simulates attacker traffic: many connections that complete the
// handshake then drop, stressing the sync manager's ordering of
// NewPeer/DonePeer.
func fakePeerConn(
	nodeAddr string, handshakeSlots chan struct{}, ready chan<- struct{},
	disconnect <-chan struct{},
) error {

	select {
	case <-disconnect:
		return nil
	default:
	}

	select {
	case handshakeSlots <- struct{}{}:
	case <-disconnect:
		return nil
	}
	handshakeSlotHeld := true
	defer func() {
		if handshakeSlotHeld {
			<-handshakeSlots
		}
	}()

	select {
	case <-disconnect:
		return nil
	default:
	}

	conn, err := net.DialTimeout("tcp", nodeAddr, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(15 * time.Second))

	nodeTCP, err := net.ResolveTCPAddr("tcp", nodeAddr)
	if err != nil {
		return err
	}
	you := wire.NewNetAddress(
		nodeTCP, wire.SFNodeNetwork|wire.SFNodeWitness,
	)
	me := wire.NewNetAddress(
		&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		wire.SFNodeNetwork|wire.SFNodeWitness,
	)
	you.Timestamp = time.Time{}
	me.Timestamp = time.Time{}

	nonce := uint64(rand.Int63())
	msgVersion := wire.NewMsgVersion(me, you, nonce, 0)
	msgVersion.Services = wire.SFNodeNetwork | wire.SFNodeWitness

	err = wire.WriteMessage(
		conn, msgVersion, wire.ProtocolVersion, wire.SimNet,
	)
	if err != nil {
		return err
	}

	var pingNonce uint64
	var awaitingPong bool
	for {
		msg, _, err := wire.ReadMessage(conn, wire.ProtocolVersion, wire.SimNet)
		if err != nil {
			return err
		}
		switch msg := msg.(type) {
		case *wire.MsgVersion:
			// Node's version; send verack.
			err := wire.WriteMessage(
				conn, wire.NewMsgVerAck(),
				wire.ProtocolVersion, wire.SimNet,
			)
			if err != nil {
				return err
			}

		case *wire.MsgSendAddrV2:
			// Optional; keep reading.

		case *wire.MsgVerAck:
			// A pong proves the server processed our verack and
			// released its incomplete-handshake source slot.
			pingNonce = uint64(rand.Int63())
			awaitingPong = true
			err := wire.WriteMessage(
				conn, wire.NewMsgPing(pingNonce),
				wire.ProtocolVersion, wire.SimNet,
			)
			if err != nil {
				return err
			}

		case *wire.MsgPong:
			if !awaitingPong || msg.Nonce != pingNonce {
				continue
			}

			// Keep the registered peer open until all peers are
			// ready to disconnect together.
			<-handshakeSlots
			handshakeSlotHeld = false
			ready <- struct{}{}
			<-disconnect

			return nil

		default:
			// Ignore other messages (e.g. wtxidrelay) and keep reading.
		}
	}
}

// waitForConnectionCount waits for the server's registered peer count to
// reach the expected value.
func waitForConnectionCount(
	getConnectionCount func() rpcclient.FutureGetConnectionCountResult,
	want int64,
) error {

	deadline := time.NewTimer(syncRaceRegistrationWait)
	defer deadline.Stop()

	var got int64 = -1
	for {
		future := getConnectionCount()
		select {
		case response := <-future:
			// Put the response back into the buffered future so its
			// public Receive method retains the standard decoding and
			// error handling.
			future <- response

			var err error
			got, err = future.Receive()
			if err != nil {
				return err
			}

		case <-deadline.C:
			return fmt.Errorf(
				"timed out waiting for %d registered peers, "+
					"last count %d", want, got,
			)
		}

		if got == want {
			return nil
		}

		poll := time.NewTimer(10 * time.Millisecond)
		select {
		case <-poll.C:

		case <-deadline.C:
			if !poll.Stop() {
				<-poll.C
			}
			return fmt.Errorf(
				"timed out waiting for %d registered peers, "+
					"last count %d", want, got,
			)
		}
	}
}

// runFakePeerBatch registers one full wave of peers, disconnects them
// together, and waits for every worker and server-side peer removal.
func runFakePeerBatch(
	nodeAddr string,
	getConnectionCount func() rpcclient.FutureGetConnectionCountResult,
) error {

	handshakeSlots := make(
		chan struct{}, syncRaceHandshakeConcurrency,
	)
	ready := make(chan struct{}, syncRaceConcurrency)
	disconnect := make(chan struct{})
	errCh := make(chan error, syncRaceConcurrency)

	var disconnectOnce sync.Once
	disconnectAll := func() {
		disconnectOnce.Do(func() {
			close(disconnect)
		})
	}
	defer disconnectAll()

	for i := 0; i < syncRaceConcurrency; i++ {
		go func() {
			errCh <- fakePeerConn(
				nodeAddr, handshakeSlots, ready, disconnect,
			)
		}()
	}

	var firstErr error
	var results int
	for readyPeers := 0; readyPeers < syncRaceConcurrency &&
		firstErr == nil; {

		select {
		case <-ready:
			readyPeers++

		case err := <-errCh:
			results++
			if err == nil {
				firstErr = fmt.Errorf(
					"fake peer exited before disconnect barrier",
				)
				break
			}

			firstErr = fmt.Errorf(
				"fake peer failed before disconnect barrier: %w",
				err,
			)
		}
	}

	if firstErr == nil {
		err := waitForConnectionCount(
			getConnectionCount, syncRaceConcurrency,
		)
		if err != nil {
			firstErr = fmt.Errorf(
				"peer registration barrier: %w", err,
			)
		}
	}

	disconnectAll()
	for results < syncRaceConcurrency {
		err := <-errCh
		results++
		if firstErr == nil && err != nil {
			firstErr = err
		}
	}
	if firstErr != nil {
		return firstErr
	}

	err := waitForConnectionCount(getConnectionCount, 0)
	if err != nil {
		return fmt.Errorf("peer removal barrier: %w", err)
	}

	return nil
}

// TestSyncManagerRaceCorruption stresses a single simnet node
// with many inbound connections that complete the version/verack
// handshake then disconnect. It then proves corruption: connect a
// fresh node that generates blocks; if the stressed node does not
// sync, it was stuck with a dead sync peer (the sync manager still
// has a dead peer as sync peer, so it ignores the new live one).
func TestSyncManagerRaceCorruption(t *testing.T) {
	// This test deliberately opens more concurrent peers than the default
	// inbound limit. Raise the harness limit so admission does not dilute the
	// sync-manager lifecycle stress this test is intended to apply.
	stressedHarness, err := rpctest.New(
		&chaincfg.SimNetParams, nil, []string{"--maxpeers=400"}, "",
	)
	require.NoError(t, err)
	require.NoError(t, stressedHarness.SetUp(true, 0))
	t.Cleanup(func() {
		require.NoError(t, stressedHarness.TearDown())
	})

	nodeAddr := stressedHarness.P2PAddress()
	deadline := time.Now().Add(syncRaceRunDuration)
	iter := 0
	var done int

	for time.Now().Before(deadline) && iter < syncRaceIterations {
		err := runFakePeerBatch(
			nodeAddr, stressedHarness.Client.GetConnectionCountAsync,
		)
		require.NoError(t, err)

		done += syncRaceConcurrency
		iter += syncRaceConcurrency
	}

	// Prove corruption: connect a live node and generate blocks. If
	// the stressed node was corrupted (dead sync peer, 0 connected
	// peers), it will not sync from the new one.
	newHarness, err := rpctest.New(&chaincfg.SimNetParams, nil, nil, "")
	require.NoError(t, err)
	require.NoError(t, newHarness.SetUp(true, 0))
	defer func() { _ = newHarness.TearDown() }()

	require.NoError(t, rpctest.ConnectNode(stressedHarness, newHarness),
		"stressed node must connect to the new node")

	_, heightBefore, err := stressedHarness.Client.GetBestBlock()
	require.NoError(t, err)

	_, err = newHarness.Client.Generate(syncRaceProofBlocks)
	require.NoError(t, err)

	time.Sleep(syncRaceProofWait)

	_, heightAfter, err := stressedHarness.Client.GetBestBlock()
	require.NoError(t, err)

	expected := heightBefore + int32(syncRaceProofBlocks)
	require.GreaterOrEqualf(t, heightAfter, expected,
		"sync manager corruption after %d fake "+
			"peer cycles: node stuck with dead "+
			"sync peer (height %d -> %d)",
		done, heightBefore, heightAfter)

	t.Logf("completed %d fake peer cycles; "+
		"node synced (height %d -> %d)",
		done, heightBefore, heightAfter)
}

// dialAndSendVersion connects to nodeAddr and sends a version
// message, returning the open connection. The caller is
// responsible for closing it.
func dialAndSendVersion(
	t *testing.T, nodeAddr string,
) net.Conn {

	t.Helper()

	conn, err := net.DialTimeout("tcp", nodeAddr, 5*time.Second)
	require.NoError(t, err)

	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	nodeTCP, err := net.ResolveTCPAddr("tcp", nodeAddr)
	require.NoError(t, err)

	you := wire.NewNetAddress(
		nodeTCP, wire.SFNodeNetwork|wire.SFNodeWitness,
	)
	me := wire.NewNetAddress(
		&net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0},
		wire.SFNodeNetwork|wire.SFNodeWitness,
	)
	you.Timestamp = time.Time{}
	me.Timestamp = time.Time{}

	nonce := uint64(rand.Int63())
	msgVersion := wire.NewMsgVersion(me, you, nonce, 0)
	msgVersion.Services = wire.SFNodeNetwork | wire.SFNodeWitness

	err = wire.WriteMessage(
		conn, msgVersion, wire.ProtocolVersion, wire.SimNet,
	)
	require.NoError(t, err)

	return conn
}

// TestPreVerackDisconnect verifies that a peer disconnecting
// before completing the version/verack handshake does not corrupt
// the sync manager state. In this case only a peerDone event is
// produced (no peerAdd), since peerLifecycleHandler only sends
// peerAdd when verAckCh is closed. The node must remain healthy.
func TestPreVerackDisconnect(t *testing.T) {
	harness, err := rpctest.New(&chaincfg.SimNetParams, nil, nil, "")
	require.NoError(t, err)
	require.NoError(t, harness.SetUp(true, 0))
	t.Cleanup(func() { _ = harness.TearDown() })

	nodeAddr := harness.P2PAddress()

	// Connect and send version, then disconnect before receiving or
	// sending verack. This is expected to produce a peerDone without
	// a preceding peerAdd in the lifecycle channel.
	const preVerackAttempts = 50

	for i := 0; i < preVerackAttempts; i++ {
		conn := dialAndSendVersion(t, nodeAddr)
		conn.Close()
	}

	// Allow the node time to process all the disconnects.
	time.Sleep(2 * time.Second)

	// Verify the node is still healthy: connect a real peer, generate
	// blocks, and confirm the harness syncs them.
	helper, err := rpctest.New(&chaincfg.SimNetParams, nil, nil, "")
	require.NoError(t, err)
	require.NoError(t, helper.SetUp(true, 0))
	defer func() { _ = helper.TearDown() }()

	require.NoError(t, rpctest.ConnectNode(harness, helper))

	_, heightBefore, err := harness.Client.GetBestBlock()
	require.NoError(t, err)

	_, err = helper.Client.Generate(3)
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	_, heightAfter, err := harness.Client.GetBestBlock()
	require.NoError(t, err)

	require.GreaterOrEqual(t, heightAfter, heightBefore+3,
		"node failed to sync after pre-verack disconnects")

	t.Logf("node healthy after %d pre-verack disconnects "+
		"(height %d -> %d)",
		preVerackAttempts, heightBefore, heightAfter)
}
