package integration

import (
	"testing"
	"time"

	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/integration/rpctest"
	"github.com/btcsuite/btcd/rpcclient"
	"github.com/stretchr/testify/require"
)

// TestReorgFromForkPoint tests that when two nodes with a shared chain history
// diverge and reconnect, the shorter node correctly downloads blocks starting
// from the fork point.  This is a regression test for a bug where
// fetchHeaderBlocks started from bestState.Height + 1 instead of the fork
// point between the best chain and the best header chain, causing blocks
// between the fork point and the current height on the new chain to never be
// downloaded.
func TestReorgFromForkPoint(t *testing.T) {
	t.Parallel()

	const (
		sharedBlocks  = 10
		shorterBlocks = 5
		longerBlocks  = 20
	)
	var (
		sharedHeight  = int32(sharedBlocks)
		shorterHeight = sharedHeight + int32(shorterBlocks)
		longerHeight  = sharedHeight + int32(longerBlocks)
		forkBranchLen = int32(shorterBlocks)
	)

	longer, err := rpctest.New(&chaincfg.SimNetParams, nil, []string{}, "")
	require.NoError(t, err)
	require.NoError(t, longer.SetUp(false, 0))
	t.Cleanup(func() { require.NoError(t, longer.TearDown()) })

	shorter, err := rpctest.New(&chaincfg.SimNetParams, nil, []string{}, "")
	require.NoError(t, err)
	require.NoError(t, shorter.SetUp(false, 0))
	t.Cleanup(func() { require.NoError(t, shorter.TearDown()) })

	// Mine the shared chain on the longer node before connecting so that
	// it is "current" and can serve headers/blocks to the shorter node.
	_, err = longer.Client.Generate(sharedBlocks)
	require.NoError(t, err)

	// Connect and let the shorter node sync the shared chain.
	require.NoError(t, rpctest.ConnectNode(shorter, longer))
	require.NoError(t, rpctest.JoinNodes(
		[]*rpctest.Harness{longer, shorter}, rpctest.Blocks,
	))

	// Disconnect so they can mine independently.
	require.NoError(t, shorter.Client.AddNode(
		longer.P2PAddress(), rpcclient.ANRemove,
	))

	// Wait for both sides to see the disconnect.
	require.Eventually(t,
		func() bool {
			p1, err1 := longer.Client.GetPeerInfo()
			p2, err2 := shorter.Client.GetPeerInfo()

			return err1 == nil && err2 == nil &&
				len(p1) == 0 && len(p2) == 0
		},
		5*time.Second, 100*time.Millisecond,
	)

	// Mine divergent chains.  Both chains diverge at the shared block at
	// the shared height.
	//
	// Mine the shorter chain.
	_, err = shorter.Client.Generate(shorterBlocks)
	require.NoError(t, err)
	_, gotShorterHeight, err := shorter.Client.GetBestBlock()
	require.NoError(t, err)
	require.Equal(t, shorterHeight, gotShorterHeight)

	// Now the longer chain.
	_, err = longer.Client.Generate(longerBlocks)
	require.NoError(t, err)
	_, gotLongerHeight, err := longer.Client.GetBestBlock()
	require.NoError(t, err)
	require.Equal(t, longerHeight, gotLongerHeight)

	// Verify the chains actually diverged at the first block after the
	// fork point.
	forkDivergence := int64(sharedHeight + 1)
	longerHashAtFork, err := longer.Client.GetBlockHash(forkDivergence)
	require.NoError(t, err)
	shorterHashAtFork, err := shorter.Client.GetBlockHash(forkDivergence)
	require.NoError(t, err)
	require.NotEqual(t, longerHashAtFork, shorterHashAtFork,
		"chains should have diverged at height %d", forkDivergence)

	// Reconnect.  The shorter node should reorg to the longer chain.
	// This requires fetchHeaderBlocks to download blocks from the fork
	// point rather than from the shorter node's height, since blocks
	// after the fork point on the longer chain differ from the shorter's.
	require.NoError(t, rpctest.ConnectNode(shorter, longer))
	require.NoError(t, rpctest.JoinNodes(
		[]*rpctest.Harness{longer, shorter}, rpctest.Blocks,
	))

	// Both nodes should be on the same chain at the longer height.
	longerBestHash, gotLongerHeight, err := longer.Client.GetBestBlock()
	require.NoError(t, err)
	shorterBestHash, gotShorterHeight, err := shorter.Client.GetBestBlock()
	require.NoError(t, err)
	require.Equal(t, longerHeight, gotLongerHeight)
	require.Equal(t, longerHeight, gotShorterHeight)
	require.Equal(t, longerBestHash, shorterBestHash)

	// Verify the reorged node has the orphaned fork as a side chain.
	tips, err := shorter.Client.GetChainTips()
	require.NoError(t, err)
	require.Equal(t, 2, len(tips))

	var activeTip, forkTip *btcjson.GetChainTipsResult
	for _, tip := range tips {
		switch tip.Status {
		case "active":
			activeTip = tip
		case "valid-fork":
			forkTip = tip
		}
	}
	require.NotNil(t, activeTip)
	require.Equal(t, longerHeight, activeTip.Height)
	require.NotNil(t, forkTip)
	require.Equal(t, shorterHeight, forkTip.Height)
	require.Equal(t, forkBranchLen, forkTip.BranchLen)
}
