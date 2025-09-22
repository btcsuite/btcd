package netsync

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"testing"

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

		// Make sure that when the headers-first mode is off, we always get
		// false and BFNone.
		sm.headersFirstMode = false
		isCheckpoint, gotFlags := sm.checkHeadersList(hash)
		require.Equal(t, false, isCheckpoint)
		require.Equal(t, blockchain.BFNone, gotFlags)

		// Now check that the test values are correct.
		sm.headersFirstMode = true
		isCheckpoint, gotFlags = sm.checkHeadersList(hash)
		require.Equal(t, test.isCheckpointBlock, isCheckpoint)
		require.Equal(t, test.behaviorFlags, gotFlags)
	}
}
