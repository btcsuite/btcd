// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"bytes"
	"compress/bzip2"
	"encoding/gob"
	"fmt"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake/internal/tickettreap"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ffldb"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

const (
	// testDbType is the database backend type to use for the tests.
	testDbType = "ffldb"

	// testDbRoot is the root directory used to create all test databases.
	testDbRoot = "testdbs"
)

// copyNode copies a stake node so that it can be manipulated for tests.
func copyNode(n *Node) *Node {
	liveTickets := new(tickettreap.Immutable)
	n.liveTickets.ForEach(func(k tickettreap.Key, v *tickettreap.Value) bool {
		liveTickets.Put(k, v)
		return true
	})
	missedTickets := new(tickettreap.Immutable)
	n.missedTickets.ForEach(func(k tickettreap.Key, v *tickettreap.Value) bool {
		missedTickets.Put(k, v)
		return true
	})
	revokedTickets := new(tickettreap.Immutable)
	n.revokedTickets.ForEach(func(k tickettreap.Key, v *tickettreap.Value) bool {
		revokedTickets.Put(k, v)
		return true
	})
	databaseUndoUpdate := make(UndoTicketDataSlice, len(n.databaseUndoUpdate))
	copy(databaseUndoUpdate[:], n.databaseUndoUpdate[:])
	databaseBlockTickets := make([]chainhash.Hash, len(n.databaseBlockTickets))
	copy(databaseBlockTickets[:], n.databaseBlockTickets[:])
	nextWinners := make([]chainhash.Hash, len(n.nextWinners))
	copy(nextWinners[:], n.nextWinners[:])
	var finalState [6]byte
	copy(finalState[:], n.finalState[:])

	return &Node{
		height:               n.height,
		liveTickets:          liveTickets,
		missedTickets:        missedTickets,
		revokedTickets:       revokedTickets,
		databaseUndoUpdate:   databaseUndoUpdate,
		databaseBlockTickets: databaseBlockTickets,
		nextWinners:          nextWinners,
		finalState:           finalState,
		params:               n.params,
	}
}

// ticketsInBlock finds all the new tickets in the block.
func ticketsInBlock(bl *dcrutil.Block) []chainhash.Hash {
	tickets := make([]chainhash.Hash, 0)
	for _, stx := range bl.STransactions() {
		if DetermineTxType(stx.MsgTx()) == TxTypeSStx {
			h := stx.Hash()
			tickets = append(tickets, *h)
		}
	}

	return tickets
}

// ticketsSpentInBlock finds all the tickets spent in the block.
func ticketsSpentInBlock(bl *dcrutil.Block) []chainhash.Hash {
	tickets := make([]chainhash.Hash, 0)
	for _, stx := range bl.STransactions() {
		if DetermineTxType(stx.MsgTx()) == TxTypeSSGen {
			tickets = append(tickets, stx.MsgTx().TxIn[1].PreviousOutPoint.Hash)
		}
	}

	return tickets
}

// votesInBlock finds all the votes in the block.
func votesInBlock(bl *dcrutil.Block) []chainhash.Hash {
	votes := make([]chainhash.Hash, 0)
	for _, stx := range bl.STransactions() {
		if DetermineTxType(stx.MsgTx()) == TxTypeSSGen {
			h := stx.Hash()
			votes = append(votes, *h)
		}
	}

	return votes
}

// revokedTicketsInBlock finds all the revoked tickets in the block.
func revokedTicketsInBlock(bl *dcrutil.Block) []chainhash.Hash {
	tickets := make([]chainhash.Hash, 0)
	for _, stx := range bl.STransactions() {
		if DetermineTxType(stx.MsgTx()) == TxTypeSSRtx {
			tickets = append(tickets, stx.MsgTx().TxIn[0].PreviousOutPoint.Hash)
		}
	}

	return tickets
}

// compareTreap dumps two treaps into maps and compares them with deep equal.
func compareTreap(a *tickettreap.Immutable, b *tickettreap.Immutable) bool {
	aMap := make(map[tickettreap.Key]tickettreap.Value)
	a.ForEach(func(k tickettreap.Key, v *tickettreap.Value) bool {
		aMap[k] = *v
		return true
	})

	bMap := make(map[tickettreap.Key]tickettreap.Value)
	b.ForEach(func(k tickettreap.Key, v *tickettreap.Value) bool {
		bMap[k] = *v
		return true
	})

	return reflect.DeepEqual(aMap, bMap)
}

// nodesEqual does a cursory test to ensure that data returned from the API
// for any given node is equivalent.
func nodesEqual(a *Node, b *Node) error {
	if !reflect.DeepEqual(a.LiveTickets(), b.LiveTickets()) ||
		!compareTreap(a.liveTickets, b.liveTickets) {
		return fmt.Errorf("live tickets were not equal between nodes; "+
			"a: %v, b: %v", len(a.LiveTickets()), len(b.LiveTickets()))
	}
	badFlags := false
	a.liveTickets.ForEach(func(k tickettreap.Key, v *tickettreap.Value) bool {
		if v.Expired || v.Missed || v.Revoked || v.Spent {
			badFlags = true
		}
		return true
	})
	if badFlags {
		return fmt.Errorf("live ticket with bad flags in first treap")
	}
	b.liveTickets.ForEach(func(k tickettreap.Key, v *tickettreap.Value) bool {
		if v.Expired || v.Missed || v.Revoked || v.Spent {
			badFlags = true
		}
		return true
	})
	if badFlags {
		return fmt.Errorf("live ticket with bad flags in second treap")
	}
	if !reflect.DeepEqual(a.MissedTickets(), b.MissedTickets()) ||
		!compareTreap(a.missedTickets, b.missedTickets) {
		return fmt.Errorf("missed tickets were not equal between nodes; "+
			"a: %v, b: %v", len(a.MissedTickets()), len(b.MissedTickets()))
	}
	if !reflect.DeepEqual(a.RevokedTickets(), b.RevokedTickets()) ||
		!compareTreap(a.revokedTickets, b.revokedTickets) {
		return fmt.Errorf("revoked tickets were not equal between nodes; "+
			"a: %v, b: %v", len(a.RevokedTickets()), len(b.RevokedTickets()))
	}
	if !reflect.DeepEqual(a.NewTickets(), b.NewTickets()) {
		return fmt.Errorf("new tickets were not equal between nodes; "+
			"a: %v, b: %v", len(a.NewTickets()), len(b.NewTickets()))
	}
	if !reflect.DeepEqual(a.UndoData(), b.UndoData()) {
		return fmt.Errorf("undo data were not equal between nodes; "+
			"a: %v, b: %v", len(a.UndoData()), len(b.UndoData()))
	}
	if !reflect.DeepEqual(a.Winners(), b.Winners()) {
		return fmt.Errorf("winners were not equal between nodes; "+
			"a: %v, b: %v", a.Winners(), b.Winners())
	}
	if a.FinalState() != b.FinalState() {
		return fmt.Errorf("final state were not equal between nodes; "+
			"a: %x, b: %x", a.FinalState(), b.FinalState())
	}
	if a.PoolSize() != b.PoolSize() {
		return fmt.Errorf("pool size were not equal between nodes; "+
			"a: %x, b: %x", a.PoolSize(), b.PoolSize())
	}
	if !reflect.DeepEqual(a.SpentByBlock(), b.SpentByBlock()) {
		return fmt.Errorf("spentbyblock were not equal between nodes; "+
			"a: %x, b: %x", a.SpentByBlock(), b.SpentByBlock())
	}
	if !reflect.DeepEqual(a.MissedByBlock(), b.MissedByBlock()) {
		return fmt.Errorf("missedbyblock were not equal between nodes; "+
			"a: %x, b: %x", a.MissedByBlock(), b.MissedByBlock())
	}

	return nil
}

// findDifferences finds individual differences in two treaps and prints
// them.  For use in debugging.
func findDifferences(a *tickettreap.Immutable, b *tickettreap.Immutable) {
	aMap := make(map[tickettreap.Key]*tickettreap.Value)
	a.ForEach(func(k tickettreap.Key, v *tickettreap.Value) bool {
		aMap[k] = v
		return true
	})

	bMap := make(map[tickettreap.Key]*tickettreap.Value)
	b.ForEach(func(k tickettreap.Key, v *tickettreap.Value) bool {
		bMap[k] = v
		return true
	})

	for k, v := range aMap {
		h := chainhash.Hash(k)
		vB := bMap[k]
		if vB == nil {
			fmt.Printf("Second map missing key %v\n", h)
		} else {
			if *v != *vB {
				fmt.Printf("Second map val for %v is %v, first map %v\n", h,
					vB, v)
			}
		}
	}
	for k, v := range bMap {
		h := chainhash.Hash(k)
		vA := aMap[k]
		if vA == nil {
			fmt.Printf("First map missing key %v\n", h)
		} else {
			if *v != *vA {
				fmt.Printf("First map val for %v is %v, second map %v\n", h,
					vA, v)
			}
		}
	}
}

func TestTicketDBLongChain(t *testing.T) {
	// Declare some useful variables.
	testBCHeight := int64(1001)
	filename := filepath.Join("..", "/../blockchain/testdata", "testexpiry.bz2")
	fi, err := os.Open(filename)
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file.
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data.
	bcDecoder := gob.NewDecoder(bcBuf)
	testBlockchainBytes := make(map[int64][]byte)

	// Decode the blockchain into the map.
	if err := bcDecoder.Decode(&testBlockchainBytes); err != nil {
		t.Errorf("error decoding test blockchain")
	}
	testBlockchain := make(map[int64]*dcrutil.Block, len(testBlockchainBytes))
	for k, v := range testBlockchainBytes {
		bl, err := dcrutil.NewBlockFromBytes(v)
		if err != nil {
			t.Fatalf("couldn't decode block")
		}

		testBlockchain[k] = bl
	}

	// Connect to the best block (1001).
	bestNode := genesisNode(simNetParams)
	nodesForward := make([]*Node, testBCHeight+1)
	nodesForward[0] = bestNode
	for i := int64(1); i <= testBCHeight; i++ {
		block := testBlockchain[i]
		ticketsToAdd := make([]chainhash.Hash, 0)
		if i >= simNetParams.StakeEnabledHeight {
			matureHeight := (i - int64(simNetParams.TicketMaturity))
			ticketsToAdd = ticketsInBlock(testBlockchain[matureHeight])
		}
		header := block.MsgBlock().Header
		if int(header.PoolSize) != len(bestNode.LiveTickets()) {
			t.Errorf("bad number of live tickets: want %v, got %v",
				header.PoolSize, len(bestNode.LiveTickets()))
		}
		if header.FinalState != bestNode.FinalState() {
			t.Errorf("bad final state: want %x, got %x",
				header.FinalState, bestNode.FinalState())
		}

		// In memory addition test.
		bestNode, err = bestNode.ConnectNode(header,
			ticketsSpentInBlock(block), revokedTicketsInBlock(block),
			ticketsToAdd)
		if err != nil {
			t.Fatalf("couldn't connect node: %v", err.Error())
		}

		nodesForward[i] = bestNode
	}

	// Disconnect all the way back to the genesis block.
	for i := testBCHeight; i >= int64(1); i-- {
		parentBlock := testBlockchain[i-1]
		ticketsToAdd := make([]chainhash.Hash, 0)
		if i >= simNetParams.StakeEnabledHeight {
			matureHeight := (i - 1 - int64(simNetParams.TicketMaturity))
			ticketsToAdd = ticketsInBlock(testBlockchain[matureHeight])
		}
		header := parentBlock.MsgBlock().Header
		blockUndoData := nodesForward[i-1].UndoData()

		// In memory disconnection test.
		bestNode, err = bestNode.DisconnectNode(header, blockUndoData,
			ticketsToAdd, nil)
		if err != nil {
			t.Errorf(err.Error())
		}
	}

	// Test some accessory functions.
	accessoryTestNode := nodesForward[450]
	exists := accessoryTestNode.ExistsLiveTicket(accessoryTestNode.nextWinners[0])
	if !exists {
		t.Errorf("expected winner to exist in node live tickets")
	}
	missedExp := make([]chainhash.Hash, 0)
	accessoryTestNode.missedTickets.ForEach(func(k tickettreap.Key,
		v *tickettreap.Value) bool {
		if v.Expired {
			missedExp = append(missedExp, chainhash.Hash(k))
		}

		return true
	})
	revokedExp := make([]chainhash.Hash, 0)
	accessoryTestNode.revokedTickets.ForEach(func(k tickettreap.Key,
		v *tickettreap.Value) bool {
		if v.Expired {
			revokedExp = append(revokedExp, chainhash.Hash(k))
		}

		return true
	})
	exists = accessoryTestNode.ExistsMissedTicket(missedExp[0])
	if !exists {
		t.Errorf("expected expired and missed ticket to be missed")
	}
	exists = accessoryTestNode.ExistsExpiredTicket(missedExp[0])
	if !exists {
		t.Errorf("expected expired and missed ticket to be expired")
	}
	exists = accessoryTestNode.ExistsRevokedTicket(revokedExp[0])
	if !exists {
		t.Errorf("expected expired and revoked ticket to be revoked")
	}
	exists = accessoryTestNode.ExistsExpiredTicket(revokedExp[0])
	if !exists {
		t.Errorf("expected expired and revoked ticket to be expired")
	}
	exists = accessoryTestNode.ExistsExpiredTicket(
		accessoryTestNode.nextWinners[0])
	if exists {
		t.Errorf("live ticket was expired")
	}

	// ----------------------------------------------------------------------------
	// A longer, more strenuous test is given below. Uncomment to execute it.
	// ----------------------------------------------------------------------------

	/*
		// Create a new database to store the accepted stake node data into.
		dbName := "ffldb_staketest"
		dbPath := filepath.Join(testDbRoot, dbName)
		_ = os.RemoveAll(dbPath)
		testDb, err := database.Create(testDbType, dbPath, simNetParams.Net)
		if err != nil {
			t.Fatalf("error creating db: %v", err)
		}

		// Setup a teardown.
		defer os.RemoveAll(dbPath)
		defer os.RemoveAll(testDbRoot)
		defer testDb.Close()

		// Load the genesis block and begin testing exported functions.
		err = testDb.Update(func(dbTx database.Tx) error {
			var errLocal error
			bestNode, errLocal = InitDatabaseState(dbTx, simNetParams)
			if errLocal != nil {
				return errLocal
			}

			return nil
		})
		if err != nil {
			t.Fatalf(err.Error())
		}

		// Cache all of our nodes so that we can check them when we start
		// disconnecting and going backwards through the blockchain.
		nodesForward = make([]*Node, testBCHeight+1)
		loadedNodesForward := make([]*Node, testBCHeight+1)
		nodesForward[0] = bestNode
		loadedNodesForward[0] = bestNode
		err = testDb.Update(func(dbTx database.Tx) error {
			for i := int64(1); i <= testBCHeight; i++ {
				block := testBlockchain[i]
				ticketsToAdd := make([]chainhash.Hash, 0)
				if i >= simNetParams.StakeEnabledHeight {
					matureHeight := (i - int64(simNetParams.TicketMaturity))
					ticketsToAdd = ticketsInBlock(testBlockchain[matureHeight])
				}
				header := block.MsgBlock().Header
				if int(header.PoolSize) != len(bestNode.LiveTickets()) {
					t.Errorf("bad number of live tickets: want %v, got %v",
						header.PoolSize, len(bestNode.LiveTickets()))
				}
				if header.FinalState != bestNode.FinalState() {
					t.Errorf("bad final state: want %x, got %x",
						header.FinalState, bestNode.FinalState())
				}

				// In memory addition test.
				bestNode, err = bestNode.ConnectNode(header,
					ticketsSpentInBlock(block), revokedTicketsInBlock(block),
					ticketsToAdd)
				if err != nil {
					return fmt.Errorf("couldn't connect node: %v", err.Error())
				}

				// Write the new node to db.
				nodesForward[i] = bestNode
				blockHash := block.Hash()
				err := WriteConnectedBestNode(dbTx, bestNode, *blockHash)
				if err != nil {
					return fmt.Errorf("failure writing the best node: %v",
						err.Error())
				}

				// Reload the node from DB and make sure it's the same.
				blockHash := block.Hash()
				loadedNode, err := LoadBestNode(dbTx, bestNode.Height(),
					*blockHash, header, simNetParams)
				if err != nil {
					return fmt.Errorf("failed to load the best node: %v",
						err.Error())
				}
				err = nodesEqual(loadedNode, bestNode)
				if err != nil {
					return fmt.Errorf("loaded best node was not same as "+
						"in memory best node: %v", err.Error())
				}
				loadedNodesForward[i] = loadedNode
			}

			return nil
		})
		if err != nil {
			t.Fatalf(err.Error())
		}

		nodesBackward := make([]*Node, testBCHeight+1)
		nodesBackward[testBCHeight] = bestNode
		for i := testBCHeight; i >= int64(1); i-- {
			parentBlock := testBlockchain[i-1]
			ticketsToAdd := make([]chainhash.Hash, 0)
			if i >= simNetParams.StakeEnabledHeight {
				matureHeight := (i - 1 - int64(simNetParams.TicketMaturity))
				ticketsToAdd = ticketsInBlock(testBlockchain[matureHeight])
			}
			header := parentBlock.MsgBlock().Header
			blockUndoData := nodesForward[i-1].UndoData()
			formerBestNode := bestNode

			// In memory disconnection test.
			bestNode, err = bestNode.DisconnectNode(header, blockUndoData,
				ticketsToAdd, nil)
			if err != nil {
				t.Errorf(err.Error())
			}

			err = nodesEqual(bestNode, nodesForward[i-1])
			if err != nil {
				t.Errorf("non-equiv stake nodes at height %v: %v", i-1, err.Error())
			}

			// Try again using the database instead of the in memory
			// data to disconnect the node, too.
			var bestNodeUsingDB *Node
			err = testDb.View(func(dbTx database.Tx) error {
				bestNodeUsingDB, err = formerBestNode.DisconnectNode(header, nil,
					nil, dbTx)
				if err != nil {
					return err
				}

				return nil
			})
			if err != nil {
				t.Errorf("couldn't disconnect using the database: %v",
					err.Error())
			}
			err = nodesEqual(bestNode, bestNodeUsingDB)
			if err != nil {
				t.Errorf("non-equiv stake nodes using db when disconnecting: %v",
					err.Error())
			}

			// Write the new best node to the database.
			nodesBackward[i-1] = bestNode
			err = testDb.Update(func(dbTx database.Tx) error {
				nodesForward[i] = bestNode
				parentBlockHash := parentBlock.Hash()
				err := WriteDisconnectedBestNode(dbTx, bestNode,
					*parentBlockHash, formerBestNode.UndoData())
				if err != nil {
					return fmt.Errorf("failure writing the best node: %v",
						err.Error())
				}

				return nil
			})
			if err != nil {
				t.Errorf("%s", err.Error())
			}

			// Check the best node against the loaded best node from
			// the database after.
			err = testDb.View(func(dbTx database.Tx) error {
				parentBlockHash := parentBlock.Hash()
				loadedNode, err := LoadBestNode(dbTx, bestNode.Height(),
					*parentBlockHash, header, simNetParams)
				if err != nil {
					return fmt.Errorf("failed to load the best node: %v",
						err.Error())
				}
				err = nodesEqual(loadedNode, bestNode)
				if err != nil {
					return fmt.Errorf("loaded best node %v was not same as "+
						"in memory best node: %v", loadedNode.Height(), err.Error())
				}
				err = nodesEqual(loadedNode, loadedNodesForward[i-1])
				if err != nil {
					return fmt.Errorf("loaded best node %v was not same as "+
						"cached best node: %v", loadedNode.Height(), err.Error())
				}

				return nil
			})
			if err != nil {
				t.Errorf("%s", err.Error())
			}
		}
	*/
}

func TestTicketDBGeneral(t *testing.T) {
	// Declare some useful variables.
	testBCHeight := int64(168)
	filename := filepath.Join("..", "/../blockchain/testdata", "blocks0to168.bz2")
	fi, err := os.Open(filename)
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file.
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data.
	bcDecoder := gob.NewDecoder(bcBuf)
	testBlockchainBytes := make(map[int64][]byte)

	// Decode the blockchain into the map.
	if err := bcDecoder.Decode(&testBlockchainBytes); err != nil {
		t.Errorf("error decoding test blockchain")
	}
	testBlockchain := make(map[int64]*dcrutil.Block, len(testBlockchainBytes))
	for k, v := range testBlockchainBytes {
		bl, err := dcrutil.NewBlockFromBytes(v)
		if err != nil {
			t.Fatalf("couldn't decode block")
		}

		testBlockchain[k] = bl
	}

	// Create a new database to store the accepted stake node data into.
	dbName := "ffldb_staketest"
	dbPath := filepath.Join(testDbRoot, dbName)
	_ = os.RemoveAll(dbPath)
	testDb, err := database.Create(testDbType, dbPath, simNetParams.Net)
	if err != nil {
		t.Fatalf("error creating db: %v", err)
	}

	// Setup a teardown.
	defer os.RemoveAll(dbPath)
	defer os.RemoveAll(testDbRoot)
	defer testDb.Close()

	// Load the genesis block and begin testing exported functions.
	var bestNode *Node
	err = testDb.Update(func(dbTx database.Tx) error {
		var errLocal error
		bestNode, errLocal = InitDatabaseState(dbTx, simNetParams)
		return errLocal
	})
	if err != nil {
		t.Fatalf(err.Error())
	}

	// Cache all of our nodes so that we can check them when we start
	// disconnecting and going backwards through the blockchain.
	nodesForward := make([]*Node, testBCHeight+1)
	loadedNodesForward := make([]*Node, testBCHeight+1)
	nodesForward[0] = bestNode
	loadedNodesForward[0] = bestNode
	err = testDb.Update(func(dbTx database.Tx) error {
		for i := int64(1); i <= testBCHeight; i++ {
			block := testBlockchain[i]
			ticketsToAdd := make([]chainhash.Hash, 0)
			if i >= simNetParams.StakeEnabledHeight {
				matureHeight := (i - int64(simNetParams.TicketMaturity))
				ticketsToAdd = ticketsInBlock(testBlockchain[matureHeight])
			}
			header := block.MsgBlock().Header
			if int(header.PoolSize) != len(bestNode.LiveTickets()) {
				t.Errorf("bad number of live tickets: want %v, got %v",
					header.PoolSize, len(bestNode.LiveTickets()))
			}
			if header.FinalState != bestNode.FinalState() {
				t.Errorf("bad final state: want %x, got %x",
					header.FinalState, bestNode.FinalState())
			}

			// In memory addition test.
			bestNode, err = bestNode.ConnectNode(header,
				ticketsSpentInBlock(block), revokedTicketsInBlock(block),
				ticketsToAdd)
			if err != nil {
				return fmt.Errorf("couldn't connect node: %v", err.Error())
			}

			// Write the new node to db.
			nodesForward[i] = bestNode
			blockHash := block.Hash()
			err := WriteConnectedBestNode(dbTx, bestNode, *blockHash)
			if err != nil {
				return fmt.Errorf("failure writing the best node: %v",
					err.Error())
			}

			// Reload the node from DB and make sure it's the same.
			blockHash = block.Hash()
			loadedNode, err := LoadBestNode(dbTx, bestNode.Height(),
				*blockHash, header, simNetParams)
			if err != nil {
				return fmt.Errorf("failed to load the best node: %v",
					err.Error())
			}
			err = nodesEqual(loadedNode, bestNode)
			if err != nil {
				return fmt.Errorf("loaded best node was not same as "+
					"in memory best node: %v", err.Error())
			}
			loadedNodesForward[i] = loadedNode
		}

		return nil
	})
	if err != nil {
		t.Fatalf(err.Error())
	}

	nodesBackward := make([]*Node, testBCHeight+1)
	nodesBackward[testBCHeight] = bestNode
	for i := testBCHeight; i >= int64(1); i-- {
		parentBlock := testBlockchain[i-1]
		ticketsToAdd := make([]chainhash.Hash, 0)
		if i >= simNetParams.StakeEnabledHeight {
			matureHeight := (i - 1 - int64(simNetParams.TicketMaturity))
			ticketsToAdd = ticketsInBlock(testBlockchain[matureHeight])
		}
		header := parentBlock.MsgBlock().Header
		blockUndoData := nodesForward[i-1].UndoData()
		formerBestNode := bestNode

		// In memory disconnection test.
		bestNode, err = bestNode.DisconnectNode(header, blockUndoData,
			ticketsToAdd, nil)
		if err != nil {
			t.Fatalf(err.Error())
		}

		err = nodesEqual(bestNode, nodesForward[i-1])
		if err != nil {
			t.Errorf("non-equiv stake nodes at height %v: %v", i-1, err.Error())
		}

		// Try again using the database instead of the in memory
		// data to disconnect the node, too.
		var bestNodeUsingDB *Node
		err = testDb.View(func(dbTx database.Tx) error {
			// Negative test.
			bestNodeUsingDB, err = formerBestNode.DisconnectNode(header, nil,
				nil, nil)
			if err == nil && formerBestNode.height > 1 {
				return fmt.Errorf("expected error when no in memory data " +
					"or dbtx is passed")
			}

			bestNodeUsingDB, err = formerBestNode.DisconnectNode(header, nil,
				nil, dbTx)
			return err
		})
		if err != nil {
			t.Errorf("couldn't disconnect using the database: %v",
				err.Error())
		}
		err = nodesEqual(bestNode, bestNodeUsingDB)
		if err != nil {
			t.Errorf("non-equiv stake nodes using db when disconnecting: %v",
				err.Error())
		}

		// Write the new best node to the database.
		nodesBackward[i-1] = bestNode
		err = testDb.Update(func(dbTx database.Tx) error {
			nodesForward[i] = bestNode
			parentBlockHash := parentBlock.Hash()
			err := WriteDisconnectedBestNode(dbTx, bestNode,
				*parentBlockHash, formerBestNode.UndoData())
			if err != nil {
				return fmt.Errorf("failure writing the best node: %v",
					err.Error())
			}

			return nil
		})
		if err != nil {
			t.Errorf("%s", err.Error())
		}

		// Check the best node against the loaded best node from
		// the database after.
		err = testDb.View(func(dbTx database.Tx) error {
			parentBlockHash := parentBlock.Hash()
			loadedNode, err := LoadBestNode(dbTx, bestNode.Height(),
				*parentBlockHash, header, simNetParams)
			if err != nil {
				return fmt.Errorf("failed to load the best node: %v",
					err.Error())
			}
			err = nodesEqual(loadedNode, bestNode)
			if err != nil {
				return fmt.Errorf("loaded best node was not same as "+
					"in memory best node: %v", err.Error())
			}
			err = nodesEqual(loadedNode, loadedNodesForward[i-1])
			if err != nil {
				return fmt.Errorf("loaded best node was not same as "+
					"previously cached node: %v", err.Error())
			}

			return nil
		})
		if err != nil {
			t.Errorf("%s", err.Error())
		}
	}

	// Unit testing the in-memory implementation negatively.
	b161 := testBlockchain[161]
	b162 := testBlockchain[162]
	n162Test := copyNode(nodesForward[162])

	// No node.
	_, err = connectNode(nil, b162.MsgBlock().Header,
		n162Test.SpentByBlock(), revokedTicketsInBlock(b162),
		n162Test.NewTickets())
	if err == nil {
		t.Errorf("expect error for no node")
	}

	// Best node missing ticket in live ticket bucket to spend.
	n161Copy := copyNode(nodesForward[161])
	n161Copy.liveTickets.Delete(tickettreap.Key(n162Test.SpentByBlock()[0]))
	_, err = n161Copy.ConnectNode(b162.MsgBlock().Header,
		n162Test.SpentByBlock(), revokedTicketsInBlock(b162),
		n162Test.NewTickets())
	if err == nil || err.(RuleError).GetCode() != ErrMissingTicket {
		t.Errorf("unexpected wrong or no error for "+
			"Best node missing ticket in live ticket bucket to spend: %v", err)
	}

	// Duplicate best winners.
	n161Copy = copyNode(nodesForward[161])
	n162Copy := copyNode(nodesForward[162])
	n161Copy.nextWinners[0] = n161Copy.nextWinners[1]
	spentInBlock := n162Copy.SpentByBlock()
	spentInBlock[0] = spentInBlock[1]
	_, err = n161Copy.ConnectNode(b162.MsgBlock().Header,
		spentInBlock, revokedTicketsInBlock(b162),
		n162Test.NewTickets())
	if err == nil || err.(RuleError).GetCode() != ErrMissingTicket {
		t.Errorf("unexpected wrong or no error for "+
			"Best node missing ticket in live ticket bucket to spend: %v", err)
	}

	// Test for corrupted spentInBlock.
	someHash := chainhash.HashH([]byte{0x00})
	spentInBlock = n162Test.SpentByBlock()
	spentInBlock[4] = someHash
	_, err = nodesForward[161].ConnectNode(b162.MsgBlock().Header,
		spentInBlock, revokedTicketsInBlock(b162), n162Test.NewTickets())
	if err == nil || err.(RuleError).GetCode() != ErrUnknownTicketSpent {
		t.Errorf("unexpected wrong or no error for "+
			"Test for corrupted spentInBlock: %v", err)
	}

	// Corrupt winners.
	n161Copy = copyNode(nodesForward[161])
	n161Copy.nextWinners[4] = someHash
	_, err = n161Copy.ConnectNode(b162.MsgBlock().Header,
		spentInBlock, revokedTicketsInBlock(b162), n162Test.NewTickets())
	if err == nil || err.(RuleError).GetCode() != ErrMissingTicket {
		t.Errorf("unexpected wrong or no error for "+
			"Corrupt winners: %v", err)
	}

	// Unknown missed ticket.
	n162Copy = copyNode(nodesForward[162])
	spentInBlock = n162Copy.SpentByBlock()
	_, err = nodesForward[161].ConnectNode(b162.MsgBlock().Header,
		spentInBlock, append(revokedTicketsInBlock(b162), someHash),
		n162Copy.NewTickets())
	if err == nil || err.(RuleError).GetCode() != ErrMissingTicket {
		t.Errorf("unexpected wrong or no error for "+
			"Unknown missed ticket: %v", err)
	}

	// Insert a duplicate new ticket.
	spentInBlock = n162Test.SpentByBlock()
	newTicketsDup := []chainhash.Hash{someHash, someHash}
	_, err = nodesForward[161].ConnectNode(b162.MsgBlock().Header,
		spentInBlock, revokedTicketsInBlock(b162), newTicketsDup)
	if err == nil || err.(RuleError).GetCode() != ErrDuplicateTicket {
		t.Errorf("unexpected wrong or no error for "+
			"Insert a duplicate new ticket: %v", err)
	}

	// Impossible undo data for disconnecting.
	n161Copy = copyNode(nodesForward[161])
	n162Copy = copyNode(nodesForward[162])
	n162Copy.databaseUndoUpdate[0].Expired = false
	n162Copy.databaseUndoUpdate[0].Missed = false
	n162Copy.databaseUndoUpdate[0].Spent = false
	n162Copy.databaseUndoUpdate[0].Revoked = true
	_, err = n162Copy.DisconnectNode(b161.MsgBlock().Header,
		n161Copy.UndoData(), n161Copy.NewTickets(), nil)
	if err == nil {
		t.Errorf("unexpected wrong or no error for "+
			"Impossible undo data for disconnecting: %v", err)
	}

	// Missing undo data for disconnecting.
	n161Copy = copyNode(nodesForward[161])
	n162Copy = copyNode(nodesForward[162])
	n162Copy.databaseUndoUpdate = n162Copy.databaseUndoUpdate[0:3]
	_, err = n162Copy.DisconnectNode(b161.MsgBlock().Header,
		n161Copy.UndoData(), n161Copy.NewTickets(), nil)
	if err == nil {
		t.Errorf("unexpected wrong or no error for "+
			"Missing undo data for disconnecting: %v", err)
	}

	// Unknown undo data hash when disconnecting (missing).
	n161Copy = copyNode(nodesForward[161])
	n162Copy = copyNode(nodesForward[162])
	n162Copy.databaseUndoUpdate[0].TicketHash = someHash
	n162Copy.databaseUndoUpdate[0].Expired = false
	n162Copy.databaseUndoUpdate[0].Missed = true
	n162Copy.databaseUndoUpdate[0].Spent = false
	n162Copy.databaseUndoUpdate[0].Revoked = false
	_, err = n162Copy.DisconnectNode(b161.MsgBlock().Header,
		n161Copy.UndoData(), n161Copy.NewTickets(), nil)
	if err == nil || err.(RuleError).GetCode() != ErrMissingTicket {
		t.Errorf("unexpected wrong or no error for "+
			"Unknown undo data for disconnecting (missing): %v", err)
	}

	// Unknown undo data hash when disconnecting (revoked).
	n161Copy = copyNode(nodesForward[161])
	n162Copy = copyNode(nodesForward[162])
	n162Copy.databaseUndoUpdate[0].TicketHash = someHash
	n162Copy.databaseUndoUpdate[0].Expired = false
	n162Copy.databaseUndoUpdate[0].Missed = true
	n162Copy.databaseUndoUpdate[0].Spent = false
	n162Copy.databaseUndoUpdate[0].Revoked = true
	_, err = n162Copy.DisconnectNode(b161.MsgBlock().Header,
		n161Copy.UndoData(), n161Copy.NewTickets(), nil)
	if err == nil || err.(RuleError).GetCode() != ErrMissingTicket {
		t.Errorf("unexpected wrong or no error for "+
			"Unknown undo data for disconnecting (revoked): %v", err)
	}
}

// --------------------------------------------------------------------------------
// TESTING VARIABLES BEGIN HERE

// simNetPowLimit is the highest proof of work value a Decred block
// can have for the simulation test network.  It is the value 2^255 - 1.
var simNetPowLimit = new(big.Int).Sub(new(big.Int).Lsh(bigOne, 255), bigOne)

// SimNetParams defines the network parameters for the simulation test Decred
// network.  This network is similar to the normal test network except it is
// intended for private use within a group of individuals doing simulation
// testing.  The functionality is intended to differ in that the only nodes
// which are specifically specified are used to create the network rather than
// following normal discovery rules.  This is important as otherwise it would
// just turn into another public testnet.
var simNetParams = &chaincfg.Params{
	Name:        "simnet",
	Net:         wire.SimNet,
	DefaultPort: "18555",
	DNSSeeds:    nil, // NOTE: There must NOT be any seeds.

	// Chain parameters
	GenesisBlock:             &simNetGenesisBlock,
	GenesisHash:              &simNetGenesisHash,
	PowLimit:                 simNetPowLimit,
	PowLimitBits:             0x207fffff,
	ReduceMinDifficulty:      false,
	MinDiffReductionTime:     0, // Does not apply since ReduceMinDifficulty false
	GenerateSupported:        true,
	MaximumBlockSizes:        []int{1000000, 1310720},
	MaxTxSize:                1000000,
	TargetTimePerBlock:       time.Second,
	WorkDiffAlpha:            1,
	WorkDiffWindowSize:       8,
	WorkDiffWindows:          4,
	TargetTimespan:           time.Second * 8, // TimePerBlock * WindowSize
	RetargetAdjustmentFactor: 4,

	// Subsidy parameters.
	BaseSubsidy:              50000000000,
	MulSubsidy:               100,
	DivSubsidy:               101,
	SubsidyReductionInterval: 128,
	WorkRewardProportion:     6,
	StakeRewardProportion:    3,
	BlockTaxProportion:       1,

	// Checkpoints ordered from oldest to newest.
	Checkpoints: nil,

	// BIP0009 consensus rule change deployments.
	//
	// The miner confirmation window is defined as:
	//   target proof of work timespan / target proof of work spacing
	RuleChangeActivationQuorum:     160, // 10 % of RuleChangeActivationInterval * TicketsPerBlock
	RuleChangeActivationMultiplier: 3,   // 75%
	RuleChangeActivationDivisor:    4,
	RuleChangeActivationInterval:   320, // 320 seconds
	Deployments: map[uint32][]chaincfg.ConsensusDeployment{
		4: {{
			Vote: chaincfg.Vote{
				Id:          chaincfg.VoteIDMaxBlockSize,
				Description: "Change maximum allowed block size from 1MiB to 1.25MB",
				Mask:        0x0006, // Bits 1 and 2
				Choices: []chaincfg.Choice{{
					Id:          "abstain",
					Description: "abstain voting for change",
					Bits:        0x0000,
					IsIgnore:    true,
					IsNo:        false,
				}, {
					Id:          "no",
					Description: "reject changing max allowed block size",
					Bits:        0x0002, // Bit 1
					IsIgnore:    false,
					IsNo:        true,
				}, {
					Id:          "yes",
					Description: "accept changing max allowed block size",
					Bits:        0x0004, // Bit 2
					IsIgnore:    false,
					IsNo:        false,
				}},
			},
			StartTime:  0,             // Always available for vote
			ExpireTime: math.MaxInt64, // Never expires
		}},
	},

	// Enforce current block version once majority of the network has
	// upgraded.
	// 51% (51 / 100)
	// Reject previous block versions once a majority of the network has
	// upgraded.
	// 75% (75 / 100)
	BlockEnforceNumRequired: 51,
	BlockRejectNumRequired:  75,
	BlockUpgradeNumToCheck:  100,

	// Mempool parameters
	RelayNonStdTxs: true,

	// Address encoding magics
	NetworkAddressPrefix: "S",
	PubKeyAddrID:         [2]byte{0x27, 0x6f}, // starts with Sk
	PubKeyHashAddrID:     [2]byte{0x0e, 0x91}, // starts with Ss
	PKHEdwardsAddrID:     [2]byte{0x0e, 0x71}, // starts with Se
	PKHSchnorrAddrID:     [2]byte{0x0e, 0x53}, // starts with SS
	ScriptHashAddrID:     [2]byte{0x0e, 0x6c}, // starts with Sc
	PrivateKeyID:         [2]byte{0x23, 0x07}, // starts with Ps

	// BIP32 hierarchical deterministic extended key magics
	HDPrivateKeyID: [4]byte{0x04, 0x20, 0xb9, 0x03}, // starts with sprv
	HDPublicKeyID:  [4]byte{0x04, 0x20, 0xbd, 0x3d}, // starts with spub

	// BIP44 coin type used in the hierarchical deterministic path for
	// address generation.
	HDCoinType: 115, // ASCII for s

	// Decred PoS parameters
	MinimumStakeDiff:      20000,
	TicketPoolSize:        64,
	TicketsPerBlock:       5,
	TicketMaturity:        16,
	TicketExpiry:          384, // 6*TicketPoolSize
	CoinbaseMaturity:      16,
	SStxChangeMaturity:    1,
	TicketPoolSizeWeight:  4,
	StakeDiffAlpha:        1,
	StakeDiffWindowSize:   8,
	StakeDiffWindows:      8,
	MaxFreshStakePerBlock: 20,            // 4*TicketsPerBlock
	StakeEnabledHeight:    16 + 16,       // CoinbaseMaturity + TicketMaturity
	StakeValidationHeight: 16 + (64 * 2), // CoinbaseMaturity + TicketPoolSize*2
	StakeBaseSigScript:    []byte{0xde, 0xad, 0xbe, 0xef},

	// Decred organization related parameters
	OrganizationPkScript:        chaincfg.SimNetParams.OrganizationPkScript,
	OrganizationPkScriptVersion: chaincfg.SimNetParams.OrganizationPkScriptVersion,
	BlockOneLedger: []*chaincfg.TokenPayout{
		{Address: "Sshw6S86G2bV6W32cbc7EhtFy8f93rU6pae", Amount: 100000 * 1e8},
		{Address: "SsjXRK6Xz6CFuBt6PugBvrkdAa4xGbcZ18w", Amount: 100000 * 1e8},
		{Address: "SsfXiYkYkCoo31CuVQw428N6wWKus2ZEw5X", Amount: 100000 * 1e8},
	},
}

var bigOne = new(big.Int).SetInt64(1)

// simNetGenesisHash is the hash of the first block in the block chain for the
// simulation test network.
var simNetGenesisHash = simNetGenesisBlock.BlockHash()

// simNetGenesisMerkleRoot is the hash of the first transaction in the genesis
// block for the simulation test network.  It is the same as the merkle root for
// the main network.
var simNetGenesisMerkleRoot = genesisMerkleRoot

// genesisCoinbaseTx legacy is the coinbase transaction for the genesis blocks for
// the regression test network and test network.
var genesisCoinbaseTxLegacy = wire.MsgTx{
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			SignatureScript: []byte{
				0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, 0x45, /* |.......E| */
				0x54, 0x68, 0x65, 0x20, 0x54, 0x69, 0x6d, 0x65, /* |The Time| */
				0x73, 0x20, 0x30, 0x33, 0x2f, 0x4a, 0x61, 0x6e, /* |s 03/Jan| */
				0x2f, 0x32, 0x30, 0x30, 0x39, 0x20, 0x43, 0x68, /* |/2009 Ch| */
				0x61, 0x6e, 0x63, 0x65, 0x6c, 0x6c, 0x6f, 0x72, /* |ancellor| */
				0x20, 0x6f, 0x6e, 0x20, 0x62, 0x72, 0x69, 0x6e, /* | on brin| */
				0x6b, 0x20, 0x6f, 0x66, 0x20, 0x73, 0x65, 0x63, /* |k of sec|*/
				0x6f, 0x6e, 0x64, 0x20, 0x62, 0x61, 0x69, 0x6c, /* |ond bail| */
				0x6f, 0x75, 0x74, 0x20, 0x66, 0x6f, 0x72, 0x20, /* |out for |*/
				0x62, 0x61, 0x6e, 0x6b, 0x73, /* |banks| */
			},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value: 0x00000000,
			PkScript: []byte{
				0x41, 0x04, 0x67, 0x8a, 0xfd, 0xb0, 0xfe, 0x55, /* |A.g....U| */
				0x48, 0x27, 0x19, 0x67, 0xf1, 0xa6, 0x71, 0x30, /* |H'.g..q0| */
				0xb7, 0x10, 0x5c, 0xd6, 0xa8, 0x28, 0xe0, 0x39, /* |..\..(.9| */
				0x09, 0xa6, 0x79, 0x62, 0xe0, 0xea, 0x1f, 0x61, /* |..yb...a| */
				0xde, 0xb6, 0x49, 0xf6, 0xbc, 0x3f, 0x4c, 0xef, /* |..I..?L.| */
				0x38, 0xc4, 0xf3, 0x55, 0x04, 0xe5, 0x1e, 0xc1, /* |8..U....| */
				0x12, 0xde, 0x5c, 0x38, 0x4d, 0xf7, 0xba, 0x0b, /* |..\8M...| */
				0x8d, 0x57, 0x8a, 0x4c, 0x70, 0x2b, 0x6b, 0xf1, /* |.W.Lp+k.| */
				0x1d, 0x5f, 0xac, /* |._.| */
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// genesisMerkleRoot is the hash of the first transaction in the genesis block
// for the main network.
var genesisMerkleRoot = genesisCoinbaseTxLegacy.TxHash()

var regTestGenesisCoinbaseTx = wire.MsgTx{
	Version: 1,
	TxIn: []*wire.TxIn{
		{
			PreviousOutPoint: wire.OutPoint{
				Hash:  chainhash.Hash{},
				Index: 0xffffffff,
			},
			SignatureScript: []byte{
				0x04, 0xff, 0xff, 0x00, 0x1d, 0x01, 0x04, 0x45, /* |.......E| */
				0x54, 0x68, 0x65, 0x20, 0x54, 0x69, 0x6d, 0x65, /* |The Time| */
				0x73, 0x20, 0x30, 0x33, 0x2f, 0x4a, 0x61, 0x6e, /* |s 03/Jan| */
				0x2f, 0x32, 0x30, 0x30, 0x39, 0x20, 0x43, 0x68, /* |/2009 Ch| */
				0x61, 0x6e, 0x63, 0x65, 0x6c, 0x6c, 0x6f, 0x72, /* |ancellor| */
				0x20, 0x6f, 0x6e, 0x20, 0x62, 0x72, 0x69, 0x6e, /* | on brin| */
				0x6b, 0x20, 0x6f, 0x66, 0x20, 0x73, 0x65, 0x63, /* |k of sec|*/
				0x6f, 0x6e, 0x64, 0x20, 0x62, 0x61, 0x69, 0x6c, /* |ond bail| */
				0x6f, 0x75, 0x74, 0x20, 0x66, 0x6f, 0x72, 0x20, /* |out for |*/
				0x62, 0x61, 0x6e, 0x6b, 0x73, /* |banks| */
			},
			Sequence: 0xffffffff,
		},
	},
	TxOut: []*wire.TxOut{
		{
			Value:   0x00000000,
			Version: 0x0000,
			PkScript: []byte{
				0x41, 0x04, 0x67, 0x8a, 0xfd, 0xb0, 0xfe, 0x55, /* |A.g....U| */
				0x48, 0x27, 0x19, 0x67, 0xf1, 0xa6, 0x71, 0x30, /* |H'.g..q0| */
				0xb7, 0x10, 0x5c, 0xd6, 0xa8, 0x28, 0xe0, 0x39, /* |..\..(.9| */
				0x09, 0xa6, 0x79, 0x62, 0xe0, 0xea, 0x1f, 0x61, /* |..yb...a| */
				0xde, 0xb6, 0x49, 0xf6, 0xbc, 0x3f, 0x4c, 0xef, /* |..I..?L.| */
				0x38, 0xc4, 0xf3, 0x55, 0x04, 0xe5, 0x1e, 0xc1, /* |8..U....| */
				0x12, 0xde, 0x5c, 0x38, 0x4d, 0xf7, 0xba, 0x0b, /* |..\8M...| */
				0x8d, 0x57, 0x8a, 0x4c, 0x70, 0x2b, 0x6b, 0xf1, /* |.W.Lp+k.| */
				0x1d, 0x5f, 0xac, /* |._.| */
			},
		},
	},
	LockTime: 0,
	Expiry:   0,
}

// simNetGenesisBlock defines the genesis block of the block chain which serves
// as the public transaction ledger for the simulation test network.
var simNetGenesisBlock = wire.MsgBlock{
	Header: wire.BlockHeader{
		Version: 1,
		PrevBlock: chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}),
		MerkleRoot: simNetGenesisMerkleRoot,
		StakeRoot: chainhash.Hash([chainhash.HashSize]byte{ // Make go vet happy.
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
			0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		}),
		VoteBits:     uint16(0x0000),
		FinalState:   [6]byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00},
		Voters:       uint16(0x0000),
		FreshStake:   uint8(0x00),
		Revocations:  uint8(0x00),
		Timestamp:    time.Unix(1401292357, 0), // 2009-01-08 20:54:25 -0600 CST
		PoolSize:     uint32(0),
		Bits:         0x207fffff, // 545259519
		SBits:        int64(0x0000000000000000),
		Nonce:        0x00000000,
		StakeVersion: uint32(0),
		Height:       uint32(0),
	},
	Transactions:  []*wire.MsgTx{&regTestGenesisCoinbaseTx},
	STransactions: []*wire.MsgTx{},
}
