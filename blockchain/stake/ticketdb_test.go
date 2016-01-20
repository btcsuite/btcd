// Copyright (c) 2015 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package stake_test

import (
	"bytes"
	"compress/bzip2"
	"encoding/gob"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	_ "github.com/decred/dcrd/database/ldb"
	"github.com/decred/dcrd/wire"
	"github.com/decred/dcrutil"
)

// cloneTicketDB makes a deep copy of a ticket DB by
// serializing it to a gob and then deserializing it
// into an empty container.
func cloneTicketDB(tmdb *stake.TicketDB) (stake.TicketMaps, error) {
	mapsPointer := tmdb.DumpMapsPointer()
	mapsBytes, err := mapsPointer.GobEncode()
	if err != nil {
		return stake.TicketMaps{},
			fmt.Errorf("clone db error: could not serialize ticketMaps")
	}

	var mapsCopy stake.TicketMaps
	if err := mapsCopy.GobDecode(mapsBytes); err != nil {
		return stake.TicketMaps{},
			fmt.Errorf("clone db error: could not deserialize " +
				"ticketMaps")
	}

	return mapsCopy, nil
}

// hashInSlice returns whether a hash exists in a slice or not.
func hashInSlice(h *chainhash.Hash, list []*chainhash.Hash) bool {
	for _, hash := range list {
		if h.IsEqual(hash) {
			return true
		}
	}

	return false
}

func TestTicketDB(t *testing.T) {
	// Declare some useful variables
	testBCHeight := int64(168)

	// Set up a DB
	database, err := database.CreateDB("leveldb", "ticketdb_test")
	if err != nil {
		t.Errorf("Db create error: %v", err.Error())
	}

	// Make a new tmdb to fill with dummy live and used tickets
	var tmdb stake.TicketDB
	tmdb.Initialize(simNetParams, database)

	filename := filepath.Join("..", "/../blockchain/testdata", "blocks0to168.bz2")
	fi, err := os.Open(filename)
	bcStream := bzip2.NewReader(fi)
	defer fi.Close()

	// Create a buffer of the read file
	bcBuf := new(bytes.Buffer)
	bcBuf.ReadFrom(bcStream)

	// Create decoder from the buffer and a map to store the data
	bcDecoder := gob.NewDecoder(bcBuf)
	blockchain := make(map[int64][]byte)

	// Decode the blockchain into the map
	if err := bcDecoder.Decode(&blockchain); err != nil {
		t.Errorf("error decoding test blockchain")
	}

	var CopyOfMapsAtBlock50, CopyOfMapsAtBlock168 stake.TicketMaps
	var ticketsToSpendIn167 []chainhash.Hash
	var sortedTickets167 []*stake.TicketData

	for i := int64(0); i <= testBCHeight; i++ {
		block, err := dcrutil.NewBlockFromBytes(blockchain[i])
		if err != nil {
			t.Errorf("block deserialization error on block %v", i)
		}
		block.SetHeight(i)
		database.InsertBlock(block)
		tmdb.InsertBlock(block)

		if i == 50 {
			// Create snapshot of tmdb at block 50
			CopyOfMapsAtBlock50, err = cloneTicketDB(&tmdb)
			if err != nil {
				t.Errorf("db cloning at block 50 failure! %v", err)
			}
		}

		// Test to make sure that ticket selection is working correctly.
		if i == 167 {
			// Sort the entire list of tickets lexicographically by sorting
			// each bucket and then appending it to the list. Then store it
			// to use in the next block.
			totalTickets := 0
			sortedSlice := make([]*stake.TicketData, 0)
			for i := 0; i < stake.BucketsSize; i++ {
				tix, err := tmdb.DumpLiveTickets(uint8(i))
				if err != nil {
					t.Errorf("error dumping live tickets")
				}
				mapLen := len(tix)
				totalTickets += mapLen
				tempTdSlice := stake.NewTicketDataSlice(mapLen)
				itr := 0 // Iterator
				for _, td := range tix {
					tempTdSlice[itr] = td
					itr++
				}
				sort.Sort(tempTdSlice)
				sortedSlice = append(sortedSlice, tempTdSlice...)
			}
			sortedTickets167 = sortedSlice
		}

		if i == 168 {
			parentBlock, err := dcrutil.NewBlockFromBytes(blockchain[i-1])
			if err != nil {
				t.Errorf("block deserialization error on block %v", i-1)
			}
			pbhB, err := parentBlock.MsgBlock().Header.Bytes()
			if err != nil {
				t.Errorf("block header serialization error")
			}
			prng := stake.NewHash256PRNG(pbhB)
			ts, err := stake.FindTicketIdxs(int64(len(sortedTickets167)),
				int(simNetParams.TicketsPerBlock), prng)
			if err != nil {
				t.Errorf("failure on FindTicketIdxs")
			}
			for _, idx := range ts {
				ticketsToSpendIn167 =
					append(ticketsToSpendIn167, sortedTickets167[idx].SStxHash)
			}

			// Make sure that the tickets that were supposed to be spent or
			// missed were.
			spentTix, err := tmdb.DumpSpentTickets(i)
			if err != nil {
				t.Errorf("DumpSpentTickets failure")
			}
			for _, h := range ticketsToSpendIn167 {
				if _, ok := spentTix[h]; !ok {
					t.Errorf("missing ticket %v that should have been missed "+
						"or spent in block %v", h, i)
				}
			}

			// Create snapshot of tmdb at block 168
			CopyOfMapsAtBlock168, err = cloneTicketDB(&tmdb)
			if err != nil {
				t.Errorf("db cloning at block 168 failure! %v", err)
			}
		}
	}

	// Remove five blocks from HEAD~1
	_, _, _, err = tmdb.RemoveBlockToHeight(50)
	if err != nil {
		t.Errorf("error: %v", err)
	}

	// Test if the roll back was symmetric to the earlier snapshot
	if !reflect.DeepEqual(tmdb.DumpMapsPointer(), CopyOfMapsAtBlock50) {
		t.Errorf("The td did not restore to a previous block height correctly!")
	}

	// Test rescanning a ticket db
	err = tmdb.RescanTicketDB()
	if err != nil {
		t.Errorf("rescanticketdb err: %v", err.Error())
	}

	// Test if the db file storage was symmetric to the earlier snapshot
	if !reflect.DeepEqual(tmdb.DumpMapsPointer(), CopyOfMapsAtBlock168) {
		t.Errorf("The td did not rescan to HEAD correctly!")
	}

	err = os.Mkdir("testdata/", os.FileMode(0700))
	if err != nil {
		t.Error(err)
	}

	// Store the ticket db to disk
	err = tmdb.Store("testdata/", "testtmdb")
	if err != nil {
		t.Errorf("error: %v", err)
	}

	var tmdb2 stake.TicketDB
	err = tmdb2.LoadTicketDBs("testdata/", "testtmdb", simNetParams, database)
	if err != nil {
		t.Errorf("error: %v", err)
	}

	// Test if the db file storage was symmetric to previously rescanned one
	if !reflect.DeepEqual(tmdb.DumpMapsPointer(), tmdb2.DumpMapsPointer()) {
		t.Errorf("The td did not rescan to a previous block height correctly!")
	}

	tmdb2.Close()

	// Test dumping missing tickets from block 152
	missedIn152, _ := chainhash.NewHashFromStr(
		"84f7f866b0af1cc278cb8e0b2b76024a07542512c76487c83628c14c650de4fa")

	tmdb.RemoveBlockToHeight(152)

	missedTix, err := tmdb.DumpMissedTickets()
	if err != nil {
		t.Errorf("err dumping missed tix: %v", err.Error())
	}

	if _, exists := missedTix[*missedIn152]; !exists {
		t.Errorf("couldn't finding missed tx 1 %v in tmdb @ block 152!",
			missedIn152)
	}

	tmdb.RescanTicketDB()

	// Make sure that the revoked map contains the revoked tx
	revokedSlice := []*chainhash.Hash{missedIn152}

	revokedTix, err := tmdb.DumpRevokedTickets()
	if err != nil {
		t.Errorf("err dumping missed tix: %v", err.Error())
	}

	if len(revokedTix) != 1 {
		t.Errorf("revoked ticket map is wrong len, got %v, want %v",
			len(revokedTix), 1)
	}

	_, wasMissedIn152 := revokedTix[*revokedSlice[0]]
	ticketsRevoked := wasMissedIn152
	if !ticketsRevoked {
		t.Errorf("revoked ticket map did not include tickets missed in " +
			"block 152 and later revoked")
	}

	database.Close()
	tmdb.Close()

	os.RemoveAll("ticketdb_test")
	os.Remove("./ticketdb_test.ver")
	os.Remove("testdata/testtmdb")
	os.Remove("testdata")
}
