// Copyright (c) 2013-2017 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/peer"
	"github.com/stretchr/testify/require"
)

// TestParallelBlockFetchPartitionsWork verifies least-loaded assignment across
// three peers and that kicking a peer requeues its in-flight hashes for
// another peer (no stranded requests).
func TestParallelBlockFetchPartitionsWork(t *testing.T) {
	t.Parallel()

	params := chaincfg.RegressionNetParams
	params.Checkpoints = nil

	sm, tearDown := makeMockSyncManager(t, &params)
	defer tearDown()

	const totalBlocks = 48
	blocks := generateTestBlocks(t, &params, totalBlocks)

	peer1 := newSyncCandidate(t, sm, int32(totalBlocks))
	peer2 := newSyncCandidate(t, sm, int32(totalBlocks))
	peer3 := newSyncCandidate(t, sm, int32(totalBlocks))

	// Install headers so block fetch can proceed.
	for _, block := range blocks {
		header := &block.MsgBlock().Header
		_, err := sm.chain.ProcessBlockHeader(
			header, blockchain.BFNone, false)
		require.NoError(t, err)
	}

	sm.startParallelBlockFetch()
	require.Len(t, sm.blockPeers, 3)
	require.NotNil(t, sm.syncPeer)
	require.True(t, sm.ibdMode)

	assigned := make(map[chainhash.Hash]*peer.Peer)
	for _, p := range []*peer.Peer{peer1, peer2, peer3} {
		state := sm.peerStates[p]
		require.NotEmpty(t, state.requestedBlocks)
		for hash := range state.requestedBlocks {
			_, dup := assigned[hash]
			require.False(t, dup, "hash %v assigned to multiple peers", hash)
			assigned[hash] = p
			_, inGlobal := sm.requestedBlocks[hash]
			require.True(t, inGlobal)
		}
	}

	// Kick peer1 and ensure its hashes are reassigned, not stranded.
	state1 := sm.peerStates[peer1]
	kickedHashes := make([]chainhash.Hash, 0, len(state1.requestedBlocks))
	for hash := range state1.requestedBlocks {
		kickedHashes = append(kickedHashes, hash)
	}
	require.NotEmpty(t, kickedHashes)

	sm.kickBlockPeer(peer1, false, "test kick")
	require.False(t, sm.isBlockDownloadPeer(peer1))
	require.Empty(t, state1.requestedBlocks)

	for _, hash := range kickedHashes {
		_, onKicked := state1.requestedBlocks[hash]
		require.False(t, onKicked)

		found := false
		for p := range sm.blockPeers {
			if _, ok := sm.peerStates[p].requestedBlocks[hash]; ok {
				found = true
				require.NotEqual(t, peer1, p)
				_, inGlobal := sm.requestedBlocks[hash]
				require.True(t, inGlobal)
				break
			}
		}
		require.True(t, found,
			"requeued hash %v not assigned to a live peer", hash)
		_, stillPriority := sm.priorityBlocks[hash]
		require.False(t, stillPriority,
			"assigned priority hash %v should leave the front-of-line set", hash)
	}
}

// TestFindSlowBlockPeer requires a clear straggler before kicking.
func TestFindSlowBlockPeer(t *testing.T) {
	t.Parallel()

	params := chaincfg.RegressionNetParams
	params.Checkpoints = nil
	sm, tearDown := makeMockSyncManager(t, &params)
	defer tearDown()

	fast := newSyncCandidate(t, sm, 100)
	slow := newSyncCandidate(t, sm, 100)
	sm.blockPeers[fast] = struct{}{}
	sm.blockPeers[slow] = struct{}{}
	sm.peerStates[fast].bytesFetched = 1000
	sm.peerStates[slow].bytesFetched = 10

	require.Equal(t, slow, sm.findSlowBlockPeer())

	sm.peerStates[slow].bytesFetched = 500
	require.Nil(t, sm.findSlowBlockPeer())
}

// TestRequeuePeerBlockRequestsClearsPeerAndGlobal maps so IBD cannot stall
// with hashes stuck on a dead peer.
func TestRequeuePeerBlockRequestsClearsPeerAndGlobal(t *testing.T) {
	t.Parallel()

	params := chaincfg.RegressionNetParams
	sm, tearDown := makeMockSyncManager(t, &params)
	defer tearDown()

	p := newSyncCandidate(t, sm, 10)
	var h1, h2 chainhash.Hash
	h1[0], h2[0] = 1, 2
	sm.requestedBlocks[h1] = struct{}{}
	sm.requestedBlocks[h2] = struct{}{}
	sm.peerStates[p].requestedBlocks[h1] = struct{}{}
	sm.peerStates[p].requestedBlocks[h2] = struct{}{}

	n := sm.requeuePeerBlockRequests(sm.peerStates[p])
	require.Equal(t, 2, n)
	require.Empty(t, sm.peerStates[p].requestedBlocks)
	_, ok1 := sm.requestedBlocks[h1]
	_, ok2 := sm.requestedBlocks[h2]
	require.False(t, ok1)
	require.False(t, ok2)
	_, p1 := sm.priorityBlocks[h1]
	_, p2 := sm.priorityBlocks[h2]
	require.True(t, p1, "requeued hash should be at front of line")
	require.True(t, p2, "requeued hash should be at front of line")

	// Scoring window reset helper should not panic on empty pool timing.
	sm.lastScoreTime = time.Time{}
	sm.resetBlockPeerScores()
}
