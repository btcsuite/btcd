//go:build rpctest
// +build rpctest

package integration

import (
	"math/rand"
	"net"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

const (
	syncRaceIterations  = 1000
	syncRaceConcurrency = 300
	syncRaceRunDuration = 90 * time.Second
	syncRaceProofBlocks = 5
	syncRaceProofWait   = 8 * time.Second
)

// fakePeerConn connects to the node at nodeAddr, performs the
// minimum version/verack handshake so the node registers a peer
// (NewPeer) and then disconnects so the node runs DonePeer. This
// simulates attacker traffic: many connections that complete the
// handshake then drop, stressing the sync manager's ordering of
// NewPeer/DonePeer.
func fakePeerConn(nodeAddr string) error {
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

	for {
		msg, _, err := wire.ReadMessage(conn, wire.ProtocolVersion, wire.SimNet)
		if err != nil {
			return err
		}
		switch msg.(type) {
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
			// Handshake complete; close to trigger DonePeer.
			return nil

		default:
			// Ignore other messages (e.g. wtxidrelay) and keep reading.
		}
	}
}

// TestSyncManagerRaceCorruption stresses a single simnet node
// with many inbound connections that complete the version/verack
// handshake then disconnect. It then proves corruption: connect a
// fresh node that generates blocks; if the stressed node does not
// sync, it was stuck with a dead sync peer (the sync manager still
// has a dead peer as sync peer, so it ignores the new live one).
func TestSyncManagerRaceCorruption(t *testing.T) {
	stressedHarness, err := rpctest.New(&chaincfg.SimNetParams, nil, nil, "")
	require.NoError(t, err)
	require.NoError(t, stressedHarness.SetUp(true, 0))
	t.Cleanup(func() {
		require.NoError(t, stressedHarness.TearDown())
	})

	nodeAddr := stressedHarness.P2PAddress()
	deadline := time.Now().Add(syncRaceRunDuration)
	iter := 0
	var done int
	doneCh := make(chan struct{}, syncRaceConcurrency*2)

	for time.Now().Before(deadline) && iter < syncRaceIterations {
		for i := 0; i < syncRaceConcurrency; i++ {
			go func() {
				_ = fakePeerConn(nodeAddr)
				doneCh <- struct{}{}
			}()
		}
		for i := 0; i < syncRaceConcurrency; i++ {
			<-doneCh
			done++
		}
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
	// sending verack. This produces a peerDone without a preceding
	// peerAdd in the lifecycle channel.
	for i := 0; i < 50; i++ {
		conn, err := net.DialTimeout("tcp", nodeAddr, 5*time.Second)
		if err != nil {
			continue
		}

		_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

		nodeTCP, err := net.ResolveTCPAddr("tcp", nodeAddr)
		if err != nil {
			conn.Close()
			continue
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

		_ = wire.WriteMessage(
			conn, msgVersion, wire.ProtocolVersion, wire.SimNet,
		)

		// Close immediately without completing the handshake.
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

	t.Logf("node healthy after 50 pre-verack disconnects (height %d -> %d)",
		heightBefore, heightAfter)
}
