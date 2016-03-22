// Copyright (c) 2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.
package rpctest

import (
	"bytes"
	"math"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcwallet/waddrmgr"
)

func TestSetUp(t *testing.T) {
	// Create a new test instance.
	nodeTest, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Initiate setup, generated a chain of length 125.
	if err := nodeTest.SetUp(true, 25); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}

	// Chain length is 125, we should have 26 mature coinbase outputs.
	maxConfs := int32(math.MaxInt32)
	unspentOutputs, err := nodeTest.Wallet.ListUnspent(0, maxConfs, nil)
	if err != nil {
		t.Fatalf("unable to retrieve unspent outputs from the wallet: %v",
			err)
	}
	if len(unspentOutputs) != 26 {
		t.Fatalf("incorrect number of mature coinbases, have %v, should be %v",
			len(unspentOutputs), 26)
	}

	// Current tip should be at height 124.
	nodeInfo, err := nodeTest.Node.GetInfo()
	if err != nil {
		t.Fatalf("unable to execute getinfo on node: %v", err)
	}
	if nodeInfo.Blocks != 125 {
		t.Errorf("Chain height is %v, should be %v",
			nodeInfo.Blocks, 125)
	}

	nodeTest.TearDown()

	// Ensure all test directories have been deleted.
	if _, err := os.Stat(nodeTest.testNodeDir); err == nil {
		t.Errorf("Create test datadir was not deleted.")
	}
}

func TestCoinbaseSpend(t *testing.T) {
	// Create a new test instance.
	nodeTest, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer nodeTest.TearDown()

	// Initiate setup, generated a chain of length 125.
	if err := nodeTest.SetUp(true, 25); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}

	// Grab a fresh address from the wallet.
	addr, err := nodeTest.Wallet.NewAddress(waddrmgr.DefaultAccountNum)
	if err != nil {
		t.Fatalf("unable to get new address: %v", err)
	}

	// Next, send 5 BTC to this address, spending from one of our mature
	// coinbase outputs.
	addrScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		t.Fatalf("unable to generate pkscript to addr: %v", err)
	}
	output := wire.NewTxOut(5e8, addrScript)
	txid, err := nodeTest.CoinbaseSpend([]*wire.TxOut{output})
	if err != nil {
		t.Fatalf("coinbase spend failed: %v", err)
	}

	// Generate a single block, the transaction the wallet created should
	// be found in this block.
	blockHashes, err := nodeTest.Node.Generate(1)
	if err != nil {
		t.Fatalf("unable to generate single block: %v", err)
	}

	block, err := nodeTest.Node.GetBlock(blockHashes[0])
	if err != nil {
		t.Fatalf("unable to get block: %v", err)
	}

	minedTx := block.Transactions()[1]
	txSha := minedTx.Sha()
	if !bytes.Equal(txid[:], txSha.Bytes()[:]) {
		t.Fatalf("txid's don't match, %v vs %v", txSha, txid)
	}
}

func assertConnectedTo(t *testing.T, nodeA *Harness, nodeB *Harness) {

	nodePort := defaultP2pPort + (2 * nodeB.nodeNum)
	nodeAddr := net.JoinHostPort("127.0.0.1", strconv.Itoa(nodePort))

	nodeAPeers, err := nodeA.Node.GetPeerInfo()
	if err != nil {
		t.Fatalf("unable to get harness1's peer info")
	}

	addrFound := false
	for _, peerInfo := range nodeAPeers {
		if peerInfo.Addr == nodeAddr {
			addrFound = true
			break
		}
	}

	if !addrFound {
		t.Fatal("nodeA not connected to nodeB")
	}
}

func TestConnectNode(t *testing.T) {
	// Create two fresh test harnesses.
	harness1, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness1.SetUp(true, 25); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}
	defer harness1.TearDown()

	harness2, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness2.SetUp(true, 25); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}
	defer harness2.TearDown()

	// Establish a p2p connection from harness1 to harness2.
	if err := ConnectNode(harness1, harness2); err != nil {
		t.Fatalf("unable to connect harness1 to harness2: %v", err)
	}

	// harness1 should show up in harness2 peer's list, and vice verse.
	assertConnectedTo(t, harness1, harness2)
}

func TestTearDownAll(t *testing.T) {
	// Create two fresh test harnesses.
	harness1, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness1.SetUp(true, 25); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}

	harness2, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness2.SetUp(true, 25); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}

	// Tear down all currently active harnesses.
	if err := TearDownAll(); err != nil {
		t.Fatalf("unable to teardown all harnesses: %v", err)
	}

	// The global testInstances map should now be fully purged with no
	// active test harnesses remaining.
	if len(ActiveHarnesses()) != 0 {
		t.Fatalf("test harnesses still active after TestTearDownAll(): %v", err)
	}
}

func TestActiveHarnesses(t *testing.T) {
	// Create a single test harness.
	harness1, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer harness1.TearDown()

	// With the harness created above, a single harness should be detected
	// as active.
	numActiveHarnesses := len(ActiveHarnesses())
	if numActiveHarnesses != 1 {
		t.Fatalf("ActiveHarnesses not updated, should have length %v, "+
			"is instead %v", 1, numActiveHarnesses)
	}

	harness2, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer harness2.TearDown()

	// With the second harness created above, two harnesses should now be
	// considered active.
	numActiveHarnesses = len(ActiveHarnesses())
	if numActiveHarnesses != 2 {
		t.Fatalf("ActiveHarnesses not updated, should have length %v, "+
			"is instead %v", 2, numActiveHarnesses)
	}
}

func TestJoinMempools(t *testing.T) {
	// Create two test harnesses, starting at the same height.
	harness1, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness1.SetUp(true, 25); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}
	defer harness1.TearDown()
	harness2, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness2.SetUp(true, 25); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}
	defer harness2.TearDown()

	nodeSlice := []*Harness{harness1, harness2}

	// Both mempools should be considered synced as they are empty.
	// Therefore, this should return instantly.
	if err := JoinNodes(nodeSlice, Mempools); err != nil {
		t.Fatalf("unable to join node on block height: %v", err)
	}

	// Generate a coinbase spend to a new address within harness1's
	// mempool.
	addr, err := harness1.Wallet.NewAddress(waddrmgr.DefaultAccountNum)
	addrScript, err := txscript.PayToAddrScript(addr)
	if err != nil {
		t.Fatalf("unable to generate pkscript to addr: %v", err)
	}
	output := wire.NewTxOut(5e8, addrScript)
	if _, err = harness1.CoinbaseSpend([]*wire.TxOut{output}); err != nil {
		t.Fatalf("coinbase spend failed: %v", err)
	}

	poolsSynced := make(chan struct{})
	go func() {
		if err := JoinNodes(nodeSlice, Mempools); err != nil {
			t.Fatalf("unable to join node on node mempools: %v", err)
		}
		poolsSynced <- struct{}{}
	}()

	// This select case should fall through to the default as the goroutine
	// should be blocked on the JoinNodes calls.
	select {
	case <-poolsSynced:
		t.Fatalf("mempools detected as synced yet harness1 has a new tx")
	default:
	}

	// Establish an outbound connection from harness1 to harness2. After
	// the initial handshake both node should exchange inventory resulting
	// in a synced mempool.
	if err := ConnectNode(harness1, harness2); err != nil {
		t.Fatalf("unable to connect harness1 to harness2: %v", err)
	}

	// Select once again with a special timeout case after 1 minute. The
	// goroutine above should now be blocked on sending into the unbuffered
	// channel. The send should immediately suceed. In order to avoid the
	// test hanging indefinitely, a 1 minute timeout is in place.
	select {
	case <-poolsSynced:
		// fall through
	case <-time.After(time.Minute):
		t.Fatalf("block heights never detected as synced")
	}

}

func TestJoinBlocks(t *testing.T) {
	// Create two test harnesses, with one being 5 block ahead of the other
	// with respect to block height.
	harness1, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness1.SetUp(true, 30); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}
	defer harness1.TearDown()
	harness2, err := New(&chaincfg.SimNetParams, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness2.SetUp(true, 25); err != nil {
		t.Fatalf("unable to complete rpctest setup: %v", err)
	}
	defer harness2.TearDown()

	nodeSlice := []*Harness{harness1, harness2}
	blocksSynced := make(chan struct{})
	go func() {
		if err := JoinNodes(nodeSlice, Blocks); err != nil {
			t.Fatalf("unable to join node on block height: %v", err)
		}
		blocksSynced <- struct{}{}
	}()

	// This select case should fall through to the default as the goroutine
	// should be blocked on the JoinNodes calls.
	select {
	case <-blocksSynced:
		t.Fatalf("blocks detected as synced yet harness2 is 5 blocks behind")
	default:
	}

	// Extend harness2's chain by 5 blocks, this should cause JoinNodes to
	// finally unblock and return.
	if _, err := harness2.Node.Generate(5); err != nil {
		t.Fatalf("unable to generate blocks: %v", err)
	}

	// Select once again with a special timeout case after 1 minute. The
	// goroutine above should now be blocked on sending into the unbuffered
	// channel. The send should immediately suceed. In order to avoid the
	// test hanging indefinitely, a 1 minute timeout is in place.
	select {
	case <-blocksSynced:
		// fall through
	case <-time.After(time.Minute):
		t.Fatalf("block heights never detected as synced")
	}
}
