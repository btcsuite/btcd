package netsync

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/mempool"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

// The package-level log variable is nil by default. Set it to the
// disabled logger so that log calls in the sync manager don't panic.
func init() {
	DisableLog()
}

// noopPeerNotifier is a no-op implementation of PeerNotifier for tests.
type noopPeerNotifier struct{}

func (noopPeerNotifier) AnnounceNewTransactions([]*mempool.TxDesc)            {}
func (noopPeerNotifier) UpdatePeerHeights(*chainhash.Hash, int32, *peer.Peer) {}
func (noopPeerNotifier) RelayInventory(*wire.InvVect, interface{})            {}
func (noopPeerNotifier) TransactionConfirmed(*btcutil.Tx)                     {}

// dbSetup is used to create a new db with the genesis block already inserted.
// It returns a teardown function the caller should invoke when done testing to
// clean up.  The database is stored under t.TempDir() which is automatically
// removed when the test finishes.
func dbSetup(t *testing.T, params *chaincfg.Params) (database.DB, func(), error) {
	dbPath := filepath.Join(t.TempDir(), "ffldb")
	db, err := database.Create("ffldb", dbPath, params.Net)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating db: %v", err)
	}

	teardown := func() {
		db.Close()
	}

	return db, teardown, nil
}

// chainSetup is used to create a new db and chain instance with the genesis
// block already inserted.  In addition to the new chain instance, it returns
// a teardown function the caller should invoke when done testing to clean up.
func chainSetup(t *testing.T, params *chaincfg.Params) (
	*blockchain.BlockChain, func(), error) {

	db, teardown, err := dbSetup(t, params)
	if err != nil {
		return nil, nil, err
	}

	// Copy the chain params to ensure any modifications the tests do to
	// the chain parameters do not affect the global instance.
	paramsCopy := *params

	// Deep-copy deployment starters/enders so that parallel tests don't
	// race on the shared blockClock field written by SynchronizeClock.
	for i := range paramsCopy.Deployments {
		d := &paramsCopy.Deployments[i]
		if s, ok := d.DeploymentStarter.(*chaincfg.MedianTimeDeploymentStarter); ok {
			d.DeploymentStarter = chaincfg.NewMedianTimeDeploymentStarter(
				s.StartTime())
		}
		if e, ok := d.DeploymentEnder.(*chaincfg.MedianTimeDeploymentEnder); ok {
			d.DeploymentEnder = chaincfg.NewMedianTimeDeploymentEnder(
				e.EndTime())
		}
	}

	// Create the main chain instance.
	chain, err := blockchain.New(&blockchain.Config{
		DB:          db,
		Checkpoints: paramsCopy.Checkpoints,
		ChainParams: &paramsCopy,
		TimeSource:  blockchain.NewMedianTime(),
		SigCache:    txscript.NewSigCache(1000),
	})
	if err != nil {
		teardown()
		err := fmt.Errorf("failed to create chain instance: %v", err)
		return nil, nil, err
	}
	return chain, teardown, nil
}

// loadHeaders loads headers from mainnet from 1 to 11.
func loadHeaders(t *testing.T) []*wire.BlockHeader {
	testFile := "blockheaders-mainnet-1-11.txt"
	filename := filepath.Join("testdata/", testFile)

	file, err := os.Open(filename)
	if err != nil {
		t.Fatal(err)
	}

	headers := make([]*wire.BlockHeader, 0, 10)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		b, err := hex.DecodeString(line)
		if err != nil {
			t.Fatalf("failed to read block headers from file %v", testFile)
		}

		r := bytes.NewReader(b)
		header := new(wire.BlockHeader)
		header.Deserialize(r)

		headers = append(headers, header)
	}

	return headers
}

func makeMockSyncManager(t *testing.T,
	params *chaincfg.Params) (*SyncManager, func()) {

	t.Helper()

	chain, tearDown, err := chainSetup(t, params)
	require.NoError(t, err)

	sm, err := New(&Config{
		PeerNotifier: noopPeerNotifier{},
		Chain:        chain,
		ChainParams:  params,
	})
	require.NoError(t, err)

	return sm, tearDown
}

func TestCheckHeadersList(t *testing.T) {
	// Set params to mainnet with a checkpoint at block 11.
	params := chaincfg.MainNetParams
	checkpointHeight := int32(11)
	checkpointHash, err := chainhash.NewHashFromStr(
		"0000000097be56d606cdd9c54b04d4747e957d3608abe69198c661f2add73073")
	if err != nil {
		t.Fatal(err)
	}
	params.Checkpoints = []chaincfg.Checkpoint{
		{
			Height: checkpointHeight,
			Hash:   checkpointHash,
		},
	}

	// Create mock SyncManager.
	sm, tearDown := makeMockSyncManager(t, &params)
	defer tearDown()

	// Setup SyncManager with headers processed.
	headers := loadHeaders(t)
	for _, header := range headers {
		isMainChain, err := sm.chain.ProcessBlockHeader(
			header, blockchain.BFNone, false)
		if err != nil {
			t.Fatal(err)
		}

		if !isMainChain {
			t.Fatalf("expected block header %v to be in the main chain",
				header.BlockHash())
		}
	}

	tests := []struct {
		hash              string
		isCheckpointBlock bool
		behaviorFlags     blockchain.BehaviorFlags
	}{
		{
			hash:              chaincfg.MainNetParams.GenesisHash.String(),
			isCheckpointBlock: false,
			behaviorFlags:     blockchain.BFFastAdd,
		},
		{
			// Block 10.
			hash:              "000000002c05cc2e78923c34df87fd108b22221ac6076c18f3ade378a4d915e9",
			isCheckpointBlock: false,
			behaviorFlags:     blockchain.BFFastAdd,
		},
		{
			// Block 11.
			hash:              "0000000097be56d606cdd9c54b04d4747e957d3608abe69198c661f2add73073",
			isCheckpointBlock: true,
			behaviorFlags:     blockchain.BFFastAdd,
		},
		{
			// Block 12.
			hash:              "0000000027c2488e2510d1acf4369787784fa20ee084c258b58d9fbd43802b5e",
			isCheckpointBlock: false,
			behaviorFlags:     blockchain.BFNone,
		},
	}

	for _, test := range tests {
		hash, err := chainhash.NewHashFromStr(test.hash)
		if err != nil {
			t.Errorf("NewHashFromStr: %v", err)
			continue
		}

		// Make sure that when the ibd mode is off, we always get
		// false and BFNone.
		sm.ibdMode = false
		isCheckpoint, gotFlags := sm.checkHeadersList(hash)
		require.Equal(t, false, isCheckpoint)
		require.Equal(t, blockchain.BFNone, gotFlags)

		// Now check that the test values are correct.
		sm.ibdMode = true
		isCheckpoint, gotFlags = sm.checkHeadersList(hash)
		require.Equal(t, test.isCheckpointBlock, isCheckpoint)
		require.Equal(t, test.behaviorFlags, gotFlags)
	}
}

func TestFetchHigherPeers(t *testing.T) {
	// Create mock SyncManager.
	sm, tearDown := makeMockSyncManager(t, &chaincfg.MainNetParams)
	defer tearDown()

	tests := []struct {
		peerHeights       []int32
		peerSyncCandidate []bool
		height            int32
		expectedCnt       int
	}{
		{
			peerHeights:       []int32{9, 10, 10, 10},
			peerSyncCandidate: []bool{true, true, true, true},
			height:            5,
			expectedCnt:       4,
		},

		{
			peerHeights:       []int32{9, 10, 10, 10},
			peerSyncCandidate: []bool{false, false, true, true},
			height:            5,
			expectedCnt:       2,
		},

		{
			peerHeights:       []int32{1, 100, 100, 100, 100},
			peerSyncCandidate: []bool{true, false, true, true, false},
			height:            100,
			expectedCnt:       0,
		},
	}

	for _, test := range tests {
		// Setup peers.
		sm.peerStates = make(map[*peer.Peer]*peerSyncState)
		for i, height := range test.peerHeights {
			peer := peer.NewInboundPeer(&peer.Config{})
			peer.UpdateLastBlockHeight(height)
			sm.peerStates[peer] = &peerSyncState{
				syncCandidate:   test.peerSyncCandidate[i],
				requestedTxns:   make(map[chainhash.Hash]struct{}),
				requestedBlocks: make(map[chainhash.Hash]struct{}),
			}
		}

		// Fetch higher peers and assert.
		peers := sm.fetchHigherPeers(test.height)
		require.Equal(t, test.expectedCnt, len(peers))

		for _, peer := range peers {
			state, found := sm.peerStates[peer]
			require.True(t, found)
			require.True(t, state.syncCandidate)
		}
	}
}

// mockTimeSource is used to trick the BlockChain instance to think that we're
// in the past.  This is so that we can force it to return true for isCurrent().
type mockTimeSource struct {
	adjustedTime time.Time
}

// AdjustedTime returns the internal adjustedTime.
//
// Part of the MedianTimeSource interface implementation.
func (m *mockTimeSource) AdjustedTime() time.Time {
	return m.adjustedTime
}

// AddTimeSample isn't relevant so we just leave it as emtpy.
//
// Part of the MedianTimeSource interface implementation.
func (m *mockTimeSource) AddTimeSample(id string, timeVal time.Time) {
	// purposely left empty
}

// Offset isn't relevant so we just return 0.
//
// Part of the MedianTimeSource interface implementation.
func (m *mockTimeSource) Offset() time.Duration {
	return 0
}

// TestBuildBlockRequestSkipsInflightBlocks verifies that buildBlockRequest
// does not re-request blocks that are already in sm.requestedBlocks.  When
// the pipeline refill threshold triggers fetchHeaderBlocks while blocks are
// still in-flight, re-requesting them causes the peer to send duplicates.
// The first copy gets processed (removing the hash from requestedBlocks),
// and the second copy then arrives as "unrequested", disconnecting the peer.
func TestBuildBlockRequestSkipsInflightBlocks(t *testing.T) {
	tests := []struct {
		name string
		// inflightHeights are the block heights already in
		// requestedBlocks before calling buildBlockRequest.
		inflightHeights []int32
		// wantRequestedHeights are the block heights that should
		// appear in the returned getdata message.
		wantRequestedHeights []int32
	}{
		{
			name:                 "no blocks in-flight requests all",
			inflightHeights:      nil,
			wantRequestedHeights: []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
		},
		{
			name:                 "all blocks in-flight requests none",
			inflightHeights:      []int32{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11},
			wantRequestedHeights: nil,
		},
		{
			name:                 "first 5 in-flight requests remaining 6",
			inflightHeights:      []int32{1, 2, 3, 4, 5},
			wantRequestedHeights: []int32{6, 7, 8, 9, 10, 11},
		},
		{
			name:                 "last 6 in-flight requests first 5",
			inflightHeights:      []int32{6, 7, 8, 9, 10, 11},
			wantRequestedHeights: []int32{1, 2, 3, 4, 5},
		},
		{
			name:                 "scattered in-flight requests gaps",
			inflightHeights:      []int32{2, 4, 6, 8, 10},
			wantRequestedHeights: []int32{1, 3, 5, 7, 9, 11},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			params := chaincfg.MainNetParams
			params.Checkpoints = nil
			sm, tearDown := makeMockSyncManager(t, &params)
			defer tearDown()

			// Process headers 1-11 so the header chain is
			// ahead of the block chain.
			headers := loadHeaders(t)
			for _, header := range headers {
				_, err := sm.chain.ProcessBlockHeader(
					header, blockchain.BFNone, false)
				require.NoError(t, err)
			}

			// Set up a disconnected peer as syncPeer.
			syncPeer := peer.NewInboundPeer(&peer.Config{})
			sm.syncPeer = syncPeer
			syncPeerState := &peerSyncState{
				requestedTxns:   make(map[chainhash.Hash]struct{}),
				requestedBlocks: make(map[chainhash.Hash]struct{}),
			}
			sm.peerStates[syncPeer] = syncPeerState

			// Pre-populate in-flight blocks.
			for _, h := range tc.inflightHeights {
				hash, err := sm.chain.HeaderHashByHeight(h)
				require.NoError(t, err)
				sm.requestedBlocks[*hash] = struct{}{}
				syncPeerState.requestedBlocks[*hash] = struct{}{}
			}

			gdmsg := sm.buildBlockRequest(syncPeer)

			// Collect the hashes from the getdata message.
			got := make(map[chainhash.Hash]struct{}, len(gdmsg.InvList))
			for _, iv := range gdmsg.InvList {
				got[iv.Hash] = struct{}{}
			}

			require.Equal(t, len(tc.wantRequestedHeights), len(gdmsg.InvList))
			for _, h := range tc.wantRequestedHeights {
				hash, err := sm.chain.HeaderHashByHeight(h)
				require.NoError(t, err)
				require.Contains(t, got, *hash,
					"block at height %d should be requested", h)
			}
			for _, h := range tc.inflightHeights {
				hash, err := sm.chain.HeaderHashByHeight(h)
				require.NoError(t, err)
				require.NotContains(t, got, *hash,
					"in-flight block at height %d should not be re-requested", h)
			}
		})
	}
}

func TestIsInIBDMode(t *testing.T) {
	tests := []struct {
		peerState  map[*peer.Peer]*peerSyncState
		params     *chaincfg.Params
		timesource *mockTimeSource
		isIBDMode  bool
	}{
		// Is not current, higher peers.
		{
			params: &chaincfg.MainNetParams,
			peerState: func() map[*peer.Peer]*peerSyncState {
				ps := make(map[*peer.Peer]*peerSyncState)
				peer := peer.NewInboundPeer(&peer.Config{})
				peer.UpdateLastBlockHeight(900_000)
				ps[peer] = &peerSyncState{
					syncCandidate:   true,
					requestedTxns:   make(map[chainhash.Hash]struct{}),
					requestedBlocks: make(map[chainhash.Hash]struct{}),
				}
				return ps
			}(),
			timesource: nil,
			isIBDMode:  true,
		},
		// Is not current, no higher peers.
		{
			params: &chaincfg.MainNetParams,
			peerState: func() map[*peer.Peer]*peerSyncState {
				ps := make(map[*peer.Peer]*peerSyncState)
				peer := peer.NewInboundPeer(&peer.Config{})
				peer.UpdateLastBlockHeight(0)
				ps[peer] = &peerSyncState{
					syncCandidate:   true,
					requestedTxns:   make(map[chainhash.Hash]struct{}),
					requestedBlocks: make(map[chainhash.Hash]struct{}),
				}
				return ps
			}(),
			timesource: nil,
			isIBDMode:  true,
		},
		// Is current, higher peers.
		{
			params: func() *chaincfg.Params {
				params := chaincfg.MainNetParams
				params.Checkpoints = nil
				return &params
			}(),
			peerState: func() map[*peer.Peer]*peerSyncState {
				ps := make(map[*peer.Peer]*peerSyncState)
				peer := peer.NewInboundPeer(&peer.Config{})
				peer.UpdateLastBlockHeight(900_000)
				ps[peer] = &peerSyncState{
					syncCandidate:   true,
					requestedTxns:   make(map[chainhash.Hash]struct{}),
					requestedBlocks: make(map[chainhash.Hash]struct{}),
				}
				return ps
			}(),
			timesource: &mockTimeSource{
				chaincfg.MainNetParams.GenesisBlock.Header.Timestamp,
			},
			isIBDMode: true,
		},
		// Is current, no higher peers.
		{
			params: func() *chaincfg.Params {
				params := chaincfg.MainNetParams
				params.Checkpoints = nil
				return &params
			}(),
			peerState: func() map[*peer.Peer]*peerSyncState {
				ps := make(map[*peer.Peer]*peerSyncState)
				peer := peer.NewInboundPeer(&peer.Config{})
				peer.UpdateLastBlockHeight(0)
				ps[peer] = &peerSyncState{
					syncCandidate:   true,
					requestedTxns:   make(map[chainhash.Hash]struct{}),
					requestedBlocks: make(map[chainhash.Hash]struct{}),
				}
				return ps
			}(),
			timesource: &mockTimeSource{
				chaincfg.MainNetParams.GenesisBlock.Header.Timestamp,
			},
			isIBDMode: false,
		},
	}

	for _, test := range tests {
		db, tearDown, err := dbSetup(t, test.params)
		if err != nil {
			tearDown()
			t.Fatal(err)
		}

		timesource := blockchain.NewMedianTime()
		if test.timesource != nil {
			timesource = test.timesource
		}

		// Create the main chain instance.
		chain, err := blockchain.New(&blockchain.Config{
			DB:          db,
			Checkpoints: test.params.Checkpoints,
			ChainParams: test.params,
			TimeSource:  timesource,
			SigCache:    txscript.NewSigCache(1000),
		})
		if err != nil {
			tearDown()
			t.Fatal(err)
		}
		sm, err := New(&Config{
			Chain:       chain,
			ChainParams: test.params,
		})
		if err != nil {
			tearDown()
			t.Fatal(err)
		}

		// Run test and assert.
		sm.peerStates = test.peerState
		got := sm.isInIBDMode()
		require.Equal(t, test.isIBDMode, got)
		tearDown()
	}
}

// createTestCoinbase creates a minimal coinbase transaction for the given
// block height.  The signature script encodes the height to ensure unique
// transaction hashes across blocks.
func createTestCoinbase(height int32, params *chaincfg.Params) *wire.MsgTx {
	tx := wire.NewMsgTx(wire.TxVersion)

	// Push the height as data to guarantee unique txids per block.
	sigScript := []byte{
		0x04,
		byte(height), byte(height >> 8),
		byte(height >> 16), byte(height >> 24),
	}

	tx.AddTxIn(&wire.TxIn{
		PreviousOutPoint: wire.OutPoint{
			Hash:  chainhash.Hash{},
			Index: wire.MaxPrevOutIndex,
		},
		SignatureScript: sigScript,
		Sequence:        wire.MaxTxInSequenceNum,
	})

	tx.AddTxOut(&wire.TxOut{
		Value:    blockchain.CalcBlockSubsidy(height, params),
		PkScript: []byte{txscript.OP_TRUE},
	})

	return tx
}

// solveTestBlock finds a nonce that satisfies the proof of work for the given
// header.  With regression test parameters the difficulty is minimal and a
// solution is found almost immediately.
func solveTestBlock(header *wire.BlockHeader, params *chaincfg.Params) bool {
	target := blockchain.CompactToBig(params.PowLimitBits)
	for nonce := uint32(0); nonce < math.MaxUint32; nonce++ {
		header.Nonce = nonce
		hash := header.BlockHash()
		if blockchain.HashToBig(&hash).Cmp(target) <= 0 {
			return true
		}
	}

	return false
}

// generateTestBlocks creates count valid blocks chaining from the genesis
// block of the given params.  Each block contains only a coinbase transaction.
func generateTestBlocks(
	t *testing.T, params *chaincfg.Params, count int) []*btcutil.Block {

	t.Helper()

	blocks := make([]*btcutil.Block, 0, count)
	prevHash := params.GenesisHash
	prevTime := params.GenesisBlock.Header.Timestamp

	for h := int32(1); h <= int32(count); h++ {
		cb := createTestCoinbase(h, params)
		merkleRoot := cb.TxHash()

		header := wire.BlockHeader{
			Version:    1,
			PrevBlock:  *prevHash,
			MerkleRoot: merkleRoot,
			Timestamp:  prevTime.Add(time.Minute),
			Bits:       params.PowLimitBits,
		}
		require.True(t, solveTestBlock(&header, params),
			"failed to solve block at height %d", h)

		msgBlock := &wire.MsgBlock{
			Header:       header,
			Transactions: []*wire.MsgTx{cb},
		}
		block := btcutil.NewBlock(msgBlock)
		blocks = append(blocks, block)

		bh := block.Hash()
		prevHash = bh
		prevTime = header.Timestamp
	}

	return blocks
}

// TestSyncStateMachine exercises the end-to-end IBD sync flow:
//
//	┌→ startSync
//	│      ↓
//	│  fetchHeaders
//	│      ↓
//	│  handleHeadersMsg
//	│      ↓
//	│  fetchHeaderBlocks ←┐
//	│      ↓              │ (refill)
//	│  handleBlockMsg ────┘──→ IBD complete
//	│
//	│  (stall detected at any phase above)
//	│      ↓
//	│  handleStallSample
//	│      ↓
//	└── handleDonePeerMsg
//
// It verifies that header processing transitions to block download, that the
// pipeline refill path in handleBlockMsg is exercised, and that IBD mode is
// properly cleared once the chain catches up to the best header.
//
// The "fresh ibd" case tests a complete sync from genesis: headers are fetched
// and then blocks are downloaded.
//
// The "stall before any headers" and "stall mid header download" cases test
// recovery when the sync peer stalls during header download.  A replacement
// peer delivers the remaining (or all) headers and then all blocks.
//
// The "headers complete peer stalls on blocks" case tests recovery when the
// sync peer delivers all headers but stalls before sending any blocks; a
// replacement peer downloads all blocks.
//
// The "stalled sync peer recovery" case tests recovery mid-block-download: a
// sync peer stops responding after some blocks, handleStallSample detects the
// inactivity, the stalled peer is disconnected, and a replacement peer
// finishes IBD.
//
// The "stall mid headers then stall on blocks" case combines both failure
// modes: one peer stalls during headers (peer 2 takes over and finishes
// headers), then peer 2 stalls during block download (peer 3 finishes blocks).
// This exercises recovery across three distinct peers.
func TestSyncStateMachine(t *testing.T) {
	t.Parallel()

	const testTotalBlocks = 2 * minInFlightBlocks

	tests := []struct {
		name        string
		totalBlocks int

		// stallHeadersAfter, when >= 0, triggers a stall during
		// header download: deliver this many headers, then stall
		// the sync peer and verify a replacement finishes header
		// download.  Set to -1 for no header stall.
		stallHeadersAfter int

		// stallAfter, when >= 0, triggers a stall during block
		// download: deliver all headers, then process this many
		// blocks before stalling.  Set to -1 for no block stall.
		stallAfter int
	}{
		{
			name:              "fresh ibd",
			totalBlocks:       testTotalBlocks,
			stallHeadersAfter: -1,
			stallAfter:        -1,
		},
		{
			name:              "stall before any headers",
			totalBlocks:       testTotalBlocks,
			stallHeadersAfter: 0,
			stallAfter:        -1,
		},
		{
			name:              "stall mid header download",
			totalBlocks:       testTotalBlocks,
			stallHeadersAfter: testTotalBlocks / 2,
			stallAfter:        -1,
		},
		{
			name:              "headers complete peer stalls on blocks",
			totalBlocks:       testTotalBlocks,
			stallHeadersAfter: -1,
			stallAfter:        0,
		},
		{
			name:              "stalled sync peer recovery",
			totalBlocks:       testTotalBlocks,
			stallHeadersAfter: -1,
			stallAfter:        5,
		},
		{
			name:              "stall mid headers then stall on blocks",
			totalBlocks:       testTotalBlocks,
			stallHeadersAfter: testTotalBlocks / 2,
			stallAfter:        5,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			params := chaincfg.RegressionNetParams
			params.Checkpoints = nil

			sm, tearDown := makeMockSyncManager(t, &params)
			defer tearDown()

			blocks := generateTestBlocks(t, &params, tc.totalBlocks)

			// Register a sync candidate and call startSync,
			// which activates IBD mode and sends getheaders.
			peer1 := startIBD(t, sm, tc.totalBlocks)

			if tc.stallHeadersAfter >= 0 {
				// Stall during header download;
				// replacement sends remaining headers.
				peer2 := newSyncCandidate(t, sm,
					int32(tc.totalBlocks))
				syncStalledHeaderRecovery(
					t, sm, peer1, peer2,
					blocks, tc.stallHeadersAfter,
					tc.totalBlocks,
				)

				if tc.stallAfter >= 0 {
					peer3 := newSyncCandidate(t, sm,
						int32(tc.totalBlocks))
					syncStalledPeerRecovery(
						t, sm, peer2,
						peer3, blocks,
						tc.stallAfter,
						tc.totalBlocks,
					)
				} else {
					syncProcessBlocks(t, sm,
						peer2, blocks,
						tc.totalBlocks)
				}
			} else {
				syncSendHeaders(t, sm, peer1,
					blocks, tc.totalBlocks)

				if tc.stallAfter >= 0 {
					peer2 := newSyncCandidate(t, sm,
						int32(tc.totalBlocks))
					syncStalledPeerRecovery(
						t, sm, peer1,
						peer2, blocks,
						tc.stallAfter,
						tc.totalBlocks,
					)
				} else {
					syncProcessBlocks(t, sm,
						peer1, blocks,
						tc.totalBlocks)
				}
			}
		})
	}
}

// newSyncCandidate creates and registers a sync-candidate peer at the
// given height without triggering startSync.
func newSyncCandidate(t *testing.T, sm *SyncManager,
	height int32) *peer.Peer {

	t.Helper()

	p := peer.NewInboundPeer(&peer.Config{
		ChainParams: sm.chainParams,
	})
	p.UpdateLastBlockHeight(height)
	sm.peerStates[p] = &peerSyncState{
		syncCandidate:   true,
		requestedTxns:   make(map[chainhash.Hash]struct{}),
		requestedBlocks: make(map[chainhash.Hash]struct{}),
	}
	return p
}

// assertIBDComplete verifies that IBD finished: chain height matches
// totalBlocks, ibdMode is off, and no blocks remain in-flight.
func assertIBDComplete(t *testing.T, sm *SyncManager,
	peerState *peerSyncState, totalBlocks int) {

	t.Helper()

	best := sm.chain.BestSnapshot()
	require.Equal(t, int32(totalBlocks), best.Height)
	require.False(t, sm.ibdMode,
		"ibdMode should be off after catching up")
	require.Empty(t, sm.requestedBlocks,
		"all requested blocks should be fulfilled")
	require.Empty(t, peerState.requestedBlocks,
		"peer should have no outstanding block requests")
}

// startIBD registers a sync peer and calls startSync, verifying that IBD
// mode is activated and the peer is selected.
func startIBD(t *testing.T, sm *SyncManager,
	peerHeight int) *peer.Peer {

	t.Helper()

	syncPeer := newSyncCandidate(t, sm, int32(peerHeight))

	sm.startSync()

	require.True(t, sm.syncPeer == syncPeer,
		"syncPeer should be set after startSync")
	require.True(t, sm.ibdMode, "ibdMode should be on")
	require.False(t, sm.lastProgressTime.IsZero(),
		"lastProgressTime should be set")

	return syncPeer
}

// syncSendHeaders delivers block headers to the sync manager and verifies
// that block requests are generated.
func syncSendHeaders(t *testing.T, sm *SyncManager,
	syncPeer *peer.Peer, blocks []*btcutil.Block, totalBlocks int) {

	t.Helper()

	// Record the progress time set by startIBD so we can verify
	// that handleHeadersMsg advances it.
	progressBefore := sm.lastProgressTime

	headers := wire.NewMsgHeaders()
	for _, block := range blocks {
		err := headers.AddBlockHeader(&block.MsgBlock().Header)
		require.NoError(t, err)
	}

	sm.handleHeadersMsg(&headersMsg{
		headers: headers,
		peer:    syncPeer,
	})

	_, bestHeaderHeight := sm.chain.BestHeader()
	require.Equal(t, int32(totalBlocks), bestHeaderHeight)

	require.True(t, sm.lastProgressTime.After(progressBefore),
		"handleHeadersMsg should update lastProgressTime")

	wantRequested := make(map[chainhash.Hash]struct{}, len(blocks))
	for _, block := range blocks {
		wantRequested[*block.Hash()] = struct{}{}
	}
	require.Equal(t, wantRequested, sm.requestedBlocks)
	require.Equal(t, wantRequested, sm.peerStates[syncPeer].requestedBlocks)
}

// syncProcessBlocks feeds all blocks to handleBlockMsg and verifies that IBD
// mode remains active until the final block, at which point IBD completes.
func syncProcessBlocks(t *testing.T, sm *SyncManager, syncPeer *peer.Peer,
	blocks []*btcutil.Block, totalBlocks int) {

	t.Helper()

	peerState := sm.peerStates[syncPeer]

	for i, block := range blocks {
		sm.handleBlockMsg(&blockMsg{
			block: block,
			peer:  syncPeer,
			reply: make(chan struct{}, 1),
		})

		if i < len(blocks)-1 {
			require.True(t, sm.ibdMode,
				"ibdMode should still be on at height %d", i+1)
		}
	}

	assertIBDComplete(t, sm, peerState, totalBlocks)
}

// syncStalledPeerRecovery processes stallAfter blocks from stalledPeer,
// triggers stall detection, verifies that stalledPeer is removed and
// replacementPeer takes over, then feeds remaining blocks and verifies
// IBD completes.
func syncStalledPeerRecovery(t *testing.T, sm *SyncManager,
	stalledPeer, replacementPeer *peer.Peer,
	blocks []*btcutil.Block, stallAfter, totalBlocks int) {

	t.Helper()

	// Process the first stallAfter blocks from the stalled peer.
	for _, block := range blocks[:stallAfter] {
		sm.handleBlockMsg(&blockMsg{
			block: block,
			peer:  stalledPeer,
			reply: make(chan struct{}, 1),
		})
	}

	best := sm.chain.BestSnapshot()
	require.Equal(t, int32(stallAfter), best.Height)
	require.True(t, sm.ibdMode)

	// Trigger stall detection.
	sm.lastProgressTime = time.Now().Add(
		-(maxStallDuration + time.Minute))
	sm.handleStallSample()

	// Verify that handleStallSample called Disconnect() on the
	// stalled peer (which closes p.quit, making WaitForDisconnect
	// return immediately).
	disconnected := make(chan struct{})
	go func() {
		stalledPeer.WaitForDisconnect()
		close(disconnected)
	}()
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("Disconnect() was not called on stalled peer")
	}

	// Snapshot the stalled peer's outstanding requested blocks before
	// disconnection so we can verify they are cleaned up.
	stalledState := sm.peerStates[stalledPeer]
	stalledRequested := make([]chainhash.Hash, 0, len(stalledState.requestedBlocks))
	for hash := range stalledState.requestedBlocks {
		stalledRequested = append(stalledRequested, hash)
	}
	require.NotEmpty(t, stalledRequested,
		"stalled peer should have outstanding requested blocks")

	// In production, Disconnect() triggers handleDonePeerMsg
	// asynchronously via the peer goroutine.  Call it directly to
	// complete the removal.  Note: handleDonePeerMsg first clears the
	// stalled peer's requested blocks from the global map via
	// clearRequestedState, then updateSyncPeer → startSync immediately
	// re-requests them for the replacement peer.
	sm.handleDonePeerMsg(stalledPeer)

	_, stalledTracked := sm.peerStates[stalledPeer]
	require.False(t, stalledTracked,
		"stalled peer should be removed")
	require.True(t, sm.syncPeer == replacementPeer,
		"replacement peer should take over as sync peer")
	require.True(t, sm.ibdMode)

	// Verify that the replacement peer re-requested the exact same
	// blocks that were outstanding from the stalled peer.
	replacementState := sm.peerStates[replacementPeer]
	require.Equal(t, len(stalledRequested),
		len(replacementState.requestedBlocks),
		"replacement peer should request same number of blocks")
	for _, hash := range stalledRequested {
		_, exists := replacementState.requestedBlocks[hash]
		require.True(t, exists,
			"block %v should be requested from replacement peer",
			hash)
	}

	// Feed remaining blocks from the replacement peer.
	for _, block := range blocks[stallAfter:] {
		sm.handleBlockMsg(&blockMsg{
			block: block,
			peer:  replacementPeer,
			reply: make(chan struct{}, 1),
		})
	}

	assertIBDComplete(t, sm, replacementState, totalBlocks)
}

// syncStalledHeaderRecovery simulates a stall during header download.
// It optionally delivers headersSent headers from stalledPeer, triggers stall
// detection, verifies that stalledPeer is removed and replacementPeer takes
// over, then delivers remaining headers and verifies block requests are
// generated.  The caller is responsible for the block-download phase.
func syncStalledHeaderRecovery(t *testing.T, sm *SyncManager,
	stalledPeer, replacementPeer *peer.Peer,
	blocks []*btcutil.Block, headersSent, totalBlocks int) {

	t.Helper()

	// Deliver partial headers from the stalled peer.  When
	// headersSent is 0, this is a no-op (peer stalls immediately).
	if headersSent > 0 {
		headers := wire.NewMsgHeaders()
		for _, block := range blocks[:headersSent] {
			err := headers.AddBlockHeader(
				&block.MsgBlock().Header)
			require.NoError(t, err)
		}

		sm.handleHeadersMsg(&headersMsg{
			headers: headers,
			peer:    stalledPeer,
		})

		_, bestHeaderHeight := sm.chain.BestHeader()
		require.Equal(t, int32(headersSent), bestHeaderHeight)
	}

	// No blocks should have been requested during header download
	// since the headers haven't caught up to the peer's height yet.
	require.Empty(t, sm.requestedBlocks,
		"no blocks should be requested during header download")

	// Trigger stall detection.
	sm.lastProgressTime = time.Now().Add(
		-(maxStallDuration + time.Minute))
	sm.handleStallSample()

	// Verify that handleStallSample called Disconnect() on the
	// stalled peer.
	disconnected := make(chan struct{})
	go func() {
		stalledPeer.WaitForDisconnect()
		close(disconnected)
	}()
	select {
	case <-disconnected:
	case <-time.After(time.Second):
		t.Fatal("Disconnect() was not called on stalled peer")
	}

	// Complete peer removal.  handleDonePeerMsg clears state and
	// triggers startSync which selects the replacement peer.
	sm.handleDonePeerMsg(stalledPeer)

	_, stalledTracked := sm.peerStates[stalledPeer]
	require.False(t, stalledTracked,
		"stalled peer should be removed")
	require.True(t, sm.syncPeer == replacementPeer,
		"replacement peer should take over as sync peer")
	require.True(t, sm.ibdMode)

	// Deliver remaining headers from the replacement peer.  When
	// headersSent is 0, this is all headers.
	remainingHeaders := wire.NewMsgHeaders()
	for _, block := range blocks[headersSent:] {
		err := remainingHeaders.AddBlockHeader(
			&block.MsgBlock().Header)
		require.NoError(t, err)
	}
	sm.handleHeadersMsg(&headersMsg{
		headers: remainingHeaders,
		peer:    replacementPeer,
	})

	_, bestHeaderHeight := sm.chain.BestHeader()
	require.Equal(t, int32(totalBlocks), bestHeaderHeight)

	// Verify all blocks were requested from the replacement.
	wantRequested := make(map[chainhash.Hash]struct{}, len(blocks))
	for _, block := range blocks {
		wantRequested[*block.Hash()] = struct{}{}
	}
	require.Equal(t, wantRequested, sm.requestedBlocks)
	replacementState := sm.peerStates[replacementPeer]
	require.Equal(t, wantRequested, replacementState.requestedBlocks)
}

// TestStartSyncBlockFallback verifies the startSync fallback path where
// headers are already caught up but the block chain lags behind.  In this
// case startSync should skip header download and directly request blocks.
func TestStartSyncBlockFallback(t *testing.T) {
	t.Parallel()

	params := chaincfg.RegressionNetParams
	params.Checkpoints = nil

	sm, tearDown := makeMockSyncManager(t, &params)
	defer tearDown()

	// Process headers so the header chain is at numBlocks while the
	// block chain stays at genesis.
	const numBlocks = 11
	blocks := generateTestBlocks(t, &params, numBlocks)
	for _, block := range blocks {
		_, err := sm.chain.ProcessBlockHeader(
			&block.MsgBlock().Header, blockchain.BFNone, false)
		require.NoError(t, err)
	}

	// Add a peer whose height equals the header height.
	// fetchHigherPeers(bestHeaderHeight) returns nothing because
	// the peer is not strictly higher than our headers.
	// fetchHigherPeers(bestBlockHeight=0) returns the peer.
	syncPeer := peer.NewInboundPeer(&peer.Config{})
	syncPeer.UpdateLastBlockHeight(int32(numBlocks))
	sm.peerStates[syncPeer] = &peerSyncState{
		syncCandidate:   true,
		requestedTxns:   make(map[chainhash.Hash]struct{}),
		requestedBlocks: make(map[chainhash.Hash]struct{}),
	}

	sm.startSync()

	require.NotNil(t, sm.syncPeer,
		"sync peer should be set for block download")
	require.NotEmpty(t, sm.requestedBlocks,
		"blocks should be requested via fetchHeaderBlocks")
}

// TestStallNoDisconnectAtSameHeight verifies that handleStallSample does
// not disconnect a sync peer whose advertised height equals our own.
func TestStallNoDisconnectAtSameHeight(t *testing.T) {
	t.Parallel()

	params := chaincfg.RegressionNetParams
	params.Checkpoints = nil

	sm, tearDown := makeMockSyncManager(t, &params)
	defer tearDown()

	p := peer.NewInboundPeer(&peer.Config{})
	p.UpdateLastBlockHeight(0) // Same height as our genesis chain.
	sm.peerStates[p] = &peerSyncState{}
	sm.syncPeer = p
	sm.ibdMode = true
	sm.lastProgressTime = time.Now().Add(
		-(maxStallDuration + time.Minute))

	sm.handleStallSample()

	_, tracked := sm.peerStates[p]
	require.True(t, tracked,
		"peer at same height should not be disconnected")
	require.Nil(t, sm.syncPeer,
		"we should have nil syncPeer after handleStallSample")
}

// TestStartSyncChainCurrent verifies that startSync does not set syncPeer
// or ibdMode when the chain is current and no peer is strictly higher.
// isInIBDMode sees IsCurrent()==true with no higher peers, returns false,
// and startSync exits immediately.
func TestStartSyncChainCurrent(t *testing.T) {
	t.Parallel()

	params := chaincfg.RegressionNetParams
	params.Checkpoints = nil

	sm, tearDown := makeMockSyncManager(t, &params)
	defer tearDown()

	// Mine a single block with a recent timestamp so
	// IsCurrent() returns true.
	cb := createTestCoinbase(1, &params)
	header := wire.BlockHeader{
		Version:    1,
		PrevBlock:  *params.GenesisHash,
		MerkleRoot: cb.TxHash(),
		Timestamp:  time.Now().Truncate(time.Second),
		Bits:       params.PowLimitBits,
	}
	require.True(t, solveTestBlock(&header, &params))

	block := btcutil.NewBlock(&wire.MsgBlock{
		Header:       header,
		Transactions: []*wire.MsgTx{cb},
	})
	_, _, err := sm.chain.ProcessBlock(block, blockchain.BFNone)
	require.NoError(t, err)
	require.True(t, sm.chain.IsCurrent())

	// Peer at our height — not higher.
	newSyncCandidate(t, sm, 1)

	sm.startSync()

	require.Nil(t, sm.syncPeer,
		"syncPeer should not be set when chain is already current")
	require.False(t, sm.ibdMode,
		"ibdMode should not be activated when chain is already current")
}
