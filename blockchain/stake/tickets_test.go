// Copyright (c) 2015-2017 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake

import (
	"bytes"
	"compress/bzip2"
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/decred/dcrd/blockchain/stake/internal/tickettreap"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ffldb"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/wire"
)

const (
	// testDbType is the database backend type to use for the tests.
	testDbType = "ffldb"

	// testDbRoot is the root directory used to create all test databases.
	testDbRoot = "testdbs"
)

// calcHash256PRNGIVFromHeader calculates the initialization vector for a
// Hash256PRNG instance based on using the serialized bytes for the provided
// header as a seed.
func calcHash256PRNGIVFromHeader(header *wire.BlockHeader) (chainhash.Hash, error) {
	hB, err := header.Bytes()
	if err != nil {
		return chainhash.Hash{}, err
	}
	return CalcHash256PRNGIV(hB), nil
}

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
	tickets := make([]chainhash.Hash, 0, bl.MsgBlock().Header.Voters)
	for _, stx := range bl.STransactions() {
		if DetermineTxType(stx.MsgTx()) == TxTypeSSGen {
			tickets = append(tickets, stx.MsgTx().TxIn[1].PreviousOutPoint.Hash)
		}
	}

	return tickets
}

// revokedTicketsInBlock finds all the revoked tickets in the block.
func revokedTicketsInBlock(bl *dcrutil.Block) []chainhash.Hash {
	tickets := make([]chainhash.Hash, 0, bl.MsgBlock().Header.Revocations)
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

func TestTicketDBLongChain(t *testing.T) {
	// Declare some useful variables.
	params := &chaincfg.SimNetParams
	testBCHeight := int64(1001)
	filename := filepath.Join("..", "/../blockchain/testdata", "testexpiry.bz2")
	fi, err := os.Open(filename)
	if err != nil {
		t.Fatalf("failed ot open test data: %v", err)
	}
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
	bestNode := genesisNode(params)
	nodesForward := make([]*Node, testBCHeight+1)
	nodesForward[0] = bestNode
	for i := int64(1); i <= testBCHeight; i++ {
		block := testBlockchain[i]
		ticketsToAdd := make([]chainhash.Hash, 0)
		if i >= params.StakeEnabledHeight {
			matureHeight := (i - int64(params.TicketMaturity))
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
		lotteryIV, err := calcHash256PRNGIVFromHeader(&header)
		if err != nil {
			t.Fatalf("failed to calc lottery IV: %v", err)
		}
		bestNode, err = bestNode.ConnectNode(lotteryIV,
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
		if i >= params.StakeEnabledHeight {
			matureHeight := (i - 1 - int64(params.TicketMaturity))
			ticketsToAdd = ticketsInBlock(testBlockchain[matureHeight])
		}
		header := parentBlock.MsgBlock().Header
		blockUndoData := nodesForward[i-1].UndoData()

		// In memory disconnection test.
		lotteryIV, err := calcHash256PRNGIVFromHeader(&header)
		if err != nil {
			t.Fatalf("failed to calc lottery IV: %v", err)
		}
		bestNode, err = bestNode.DisconnectNode(lotteryIV, blockUndoData,
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
		testDb, err := database.Create(testDbType, dbPath, params.Net)
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
			bestNode, errLocal = InitDatabaseState(dbTx, params)
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
				if i >= params.StakeEnabledHeight {
					matureHeight := (i - int64(params.TicketMaturity))
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
					*blockHash, header, params)
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
			if i >= params.StakeEnabledHeight {
				matureHeight := (i - 1 - int64(params.TicketMaturity))
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
					*parentBlockHash, header, params)
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
	params := &chaincfg.SimNetParams
	testBCHeight := int64(168)
	filename := filepath.Join("..", "/../blockchain/testdata", "blocks0to168.bz2")
	fi, err := os.Open(filename)
	if err != nil {
		t.Errorf("Failed to open %s: %v", filename, err)
	}
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
	testDb, err := database.Create(testDbType, dbPath, params.Net)
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
		bestNode, errLocal = InitDatabaseState(dbTx, params)
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
			if i >= params.StakeEnabledHeight {
				matureHeight := (i - int64(params.TicketMaturity))
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
			lotteryIV, err := calcHash256PRNGIVFromHeader(&header)
			if err != nil {
				return fmt.Errorf("failed to calc lottery IV: %v", err)
			}
			bestNode, err = bestNode.ConnectNode(lotteryIV,
				ticketsSpentInBlock(block), revokedTicketsInBlock(block),
				ticketsToAdd)
			if err != nil {
				return fmt.Errorf("couldn't connect node: %v", err.Error())
			}

			// Write the new node to db.
			nodesForward[i] = bestNode
			blockHash := block.Hash()
			err = WriteConnectedBestNode(dbTx, bestNode, *blockHash)
			if err != nil {
				return fmt.Errorf("failure writing the best node: %v",
					err.Error())
			}

			// Reload the node from DB and make sure it's the same.
			blockHash = block.Hash()
			loadedNode, err := LoadBestNode(dbTx, bestNode.Height(),
				*blockHash, header, params)
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
		if i >= params.StakeEnabledHeight {
			matureHeight := (i - 1 - int64(params.TicketMaturity))
			ticketsToAdd = ticketsInBlock(testBlockchain[matureHeight])
		}
		header := parentBlock.MsgBlock().Header
		blockUndoData := nodesForward[i-1].UndoData()
		formerBestNode := bestNode

		// In memory disconnection test.
		lotteryIV, err := calcHash256PRNGIVFromHeader(&header)
		if err != nil {
			t.Fatalf("failed to calc lottery IV: %v", err)
		}
		bestNode, err = bestNode.DisconnectNode(lotteryIV, blockUndoData,
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
			lotteryIV, err := calcHash256PRNGIVFromHeader(&header)
			if err != nil {
				return fmt.Errorf("failed to calc lottery IV: %v", err)
			}
			bestNodeUsingDB, err = formerBestNode.DisconnectNode(lotteryIV, nil,
				nil, nil)
			if err == nil && formerBestNode.height > 1 {
				return fmt.Errorf("expected error when no in memory data " +
					"or dbtx is passed")
			}

			bestNodeUsingDB, err = formerBestNode.DisconnectNode(lotteryIV, nil,
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
				*parentBlockHash, header, params)
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

	b161LotteryIV, err := calcHash256PRNGIVFromHeader(&b161.MsgBlock().Header)
	if err != nil {
		t.Fatalf("failed to calc lottery IV for b161: %v", err)
	}
	b162LotteryIV, err := calcHash256PRNGIVFromHeader(&b162.MsgBlock().Header)
	if err != nil {
		t.Fatalf("failed to calc lottery IV for b162: %v", err)
	}

	// No node.
	_, err = connectNode(nil, b162LotteryIV, n162Test.SpentByBlock(),
		revokedTicketsInBlock(b162), n162Test.NewTickets())
	if err == nil {
		t.Errorf("expect error for no node")
	}

	// Best node missing ticket in live ticket bucket to spend.
	n161Copy := copyNode(nodesForward[161])
	n161Copy.liveTickets.Delete(tickettreap.Key(n162Test.SpentByBlock()[0]))
	_, err = n161Copy.ConnectNode(b162LotteryIV, n162Test.SpentByBlock(),
		revokedTicketsInBlock(b162), n162Test.NewTickets())
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
	_, err = n161Copy.ConnectNode(b162LotteryIV, spentInBlock,
		revokedTicketsInBlock(b162), n162Test.NewTickets())
	if err == nil || err.(RuleError).GetCode() != ErrMissingTicket {
		t.Errorf("unexpected wrong or no error for "+
			"Best node missing ticket in live ticket bucket to spend: %v", err)
	}

	// Test for corrupted spentInBlock.
	someHash := chainhash.HashH([]byte{0x00})
	spentInBlock = n162Test.SpentByBlock()
	spentInBlock[4] = someHash
	_, err = nodesForward[161].ConnectNode(b162LotteryIV, spentInBlock,
		revokedTicketsInBlock(b162), n162Test.NewTickets())
	if err == nil || err.(RuleError).GetCode() != ErrUnknownTicketSpent {
		t.Errorf("unexpected wrong or no error for "+
			"Test for corrupted spentInBlock: %v", err)
	}

	// Corrupt winners.
	n161Copy = copyNode(nodesForward[161])
	n161Copy.nextWinners[4] = someHash
	_, err = n161Copy.ConnectNode(b162LotteryIV, spentInBlock,
		revokedTicketsInBlock(b162), n162Test.NewTickets())
	if err == nil || err.(RuleError).GetCode() != ErrMissingTicket {
		t.Errorf("unexpected wrong or no error for "+
			"Corrupt winners: %v", err)
	}

	// Unknown missed ticket.
	n162Copy = copyNode(nodesForward[162])
	spentInBlock = n162Copy.SpentByBlock()
	_, err = nodesForward[161].ConnectNode(b162LotteryIV, spentInBlock,
		append(revokedTicketsInBlock(b162), someHash), n162Copy.NewTickets())
	if err == nil || err.(RuleError).GetCode() != ErrMissingTicket {
		t.Errorf("unexpected wrong or no error for "+
			"Unknown missed ticket: %v", err)
	}

	// Insert a duplicate new ticket.
	spentInBlock = n162Test.SpentByBlock()
	newTicketsDup := []chainhash.Hash{someHash, someHash}
	_, err = nodesForward[161].ConnectNode(b162LotteryIV, spentInBlock,
		revokedTicketsInBlock(b162), newTicketsDup)
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

	_, err = n162Copy.DisconnectNode(b161LotteryIV, n161Copy.UndoData(),
		n161Copy.NewTickets(), nil)
	if err == nil {
		t.Errorf("unexpected wrong or no error for "+
			"Impossible undo data for disconnecting: %v", err)
	}

	// Missing undo data for disconnecting.
	n161Copy = copyNode(nodesForward[161])
	n162Copy = copyNode(nodesForward[162])
	n162Copy.databaseUndoUpdate = n162Copy.databaseUndoUpdate[0:3]
	_, err = n162Copy.DisconnectNode(b161LotteryIV, n161Copy.UndoData(),
		n161Copy.NewTickets(), nil)
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
	_, err = n162Copy.DisconnectNode(b161LotteryIV, n161Copy.UndoData(),
		n161Copy.NewTickets(), nil)
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
	_, err = n162Copy.DisconnectNode(b161LotteryIV, n161Copy.UndoData(),
		n161Copy.NewTickets(), nil)
	if err == nil || err.(RuleError).GetCode() != ErrMissingTicket {
		t.Errorf("unexpected wrong or no error for "+
			"Unknown undo data for disconnecting (revoked): %v", err)
	}
}
