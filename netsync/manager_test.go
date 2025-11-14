package netsync

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/database"
	_ "github.com/btcsuite/btcd/database/ffldb"
	"github.com/btcsuite/btcd/peer"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/stretchr/testify/require"
)

const (
	// testDbRoot is the root directory used to create all test databases.
	testDbRoot = "testdbs"
)

// filesExists returns whether or not the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// dbSetup is used to create a new db with the genesis block already inserted.
// It returns a teardown function the caller should invoke when done testing to
// clean up.
func dbSetup(dbName string, params *chaincfg.Params) (database.DB, func(), error) {
	var db database.DB
	var teardown func()
	// Create the root directory for test databases.
	if !fileExists(testDbRoot) {
		if err := os.MkdirAll(testDbRoot, 0700); err != nil {
			err := fmt.Errorf("unable to create test db "+
				"root: %v", err)
			return nil, nil, err
		}
	}

	// Create a new database to store the accepted blocks into.
	dbPath := filepath.Join(testDbRoot, dbName)
	_ = os.RemoveAll(dbPath)
	ndb, err := database.Create("ffldb", dbPath, params.Net)
	if err != nil {
		return nil, nil, fmt.Errorf("error creating db: %v", err)
	}
	db = ndb

	// Setup a teardown function for cleaning up.  This function is
	// returned to the caller to be invoked when it is done testing.
	teardown = func() {
		db.Close()
		os.RemoveAll(dbPath)
		os.RemoveAll(testDbRoot)
	}

	return db, teardown, nil
}

// chainSetup is used to create a new db and chain instance with the genesis
// block already inserted.  In addition to the new chain instance, it returns
// a teardown function the caller should invoke when done testing to clean up.
func chainSetup(dbName string, params *chaincfg.Params) (
	*blockchain.BlockChain, func(), error) {

	db, teardown, err := dbSetup(dbName, params)
	if err != nil {
		return nil, nil, err
	}

	// Copy the chain params to ensure any modifications the tests do to
	// the chain parameters do not affect the global instance.
	paramsCopy := *params

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
	testName string, params *chaincfg.Params) (*SyncManager, func()) {

	chain, tearDown, err := chainSetup(
		testName, params)
	if err != nil {
		t.Fatal(err)
	}

	sm, err := New(&Config{
		Chain:       chain,
		ChainParams: params,
	})
	if err != nil {
		t.Fatal(err)
	}
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
	sm, tearDown := makeMockSyncManager(t, "TestCheckHeadersList", &params)
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
	if err != nil {
		t.Fatal(err)
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
	sm, tearDown := makeMockSyncManager(t, "TestFetchHigherPeers",
		&chaincfg.MainNetParams)
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

	for i, test := range tests {
		db, tearDown, err := dbSetup(
			fmt.Sprintf("TestIsInIBDMode-%v", i),
			test.params)
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
