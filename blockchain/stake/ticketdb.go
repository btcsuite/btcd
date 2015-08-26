// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

//
// TODO: Consider adding the height where an SStx was missed for use as SSgen in
// SSRtx output[0] OP_RETURN?
//
// This file contains an in-memory database for storing information about tickets.
//
// There should be four major datasets:
// ticketMap			= Ticket map keyed for mature, available tickets by number.
// spentTicketMap 	= Ticket map keyed for tickets that are mature but invalid or
//                     spent, keyed by the block in which they were invalidated.
// missedTicketMap	= Ticket map keyed for SStx hash for tickets which had the
// 					  opportunity to be spent but were not.
// revokedTicketMap = Ticket map keyed for SStx hash for tickets which had the
// 					  opportunity to be spent but were not, and then were revoked.

package stake

import (
	"bytes"
	"encoding/gob"
	"errors"
	"fmt"
	"io/ioutil"
	"math"
	"math/big"
	"path/filepath"
	"sort"
	"sync"

	"github.com/decred/dcrd/blockchain/dbnamespace"
	"github.com/decred/dcrd/chaincfg"
	"github.com/decred/dcrd/chaincfg/chainhash"
	database "github.com/decred/dcrd/database2"
	"github.com/decred/dcrutil"
)

// BucketsSize is the number of pre-sort buckets for the in memory database of
// live tickets. This allows an approximately 1.5x-2.5x increase in sorting
// speed and easier/more efficient handling of new block insertion and
// evaluation of reorgs.
// TODO Storing the size of the buckets somewhere would make evaluation of
// blocks being added to HEAD extremely fast and should eventually be implemented.
// For example, when finding the tickets to use you can cycle through a struct
// for each bucket where the struct stores the number of tickets in the bucket,
// so you can easily find the index of the ticket you need without creating
// a giant slice and sorting it.
// Optimizations for reorganize are possible.
const BucketsSize = math.MaxUint8 + 1

// TicketData contains contextual information about tickets as indicated
// below.
// TODO Replace Missed/Expired bool with single byte bitflags.
type TicketData struct {
	SStxHash    chainhash.Hash
	Prefix      uint8 // Ticket hash prefix for pre-sort
	SpendHash   chainhash.Hash
	BlockHeight int64 // Block for where the original sstx was located
	Missed      bool  // Whether or not the ticket was spent
	Expired     bool  // Whether or not the ticket expired
}

// NewTicketData returns the a filled in new TicketData structure.
func NewTicketData(sstxHash chainhash.Hash,
	prefix uint8,
	spendHash chainhash.Hash,
	blockHeight int64,
	missed bool,
	expired bool) *TicketData {
	return &TicketData{sstxHash,
		prefix,
		spendHash,
		blockHeight,
		missed,
		expired}
}

// GobEncode serializes the TicketData struct into a gob for use in storage.
//
// This function is safe for concurrent access.
func (td *TicketData) GobEncode() ([]byte, error) {
	w := new(bytes.Buffer)
	encoder := gob.NewEncoder(w)

	err := encoder.Encode(td.SStxHash)
	if err != nil {
		return nil, err
	}
	err = encoder.Encode(td.Prefix)
	if err != nil {
		return nil, err
	}
	err = encoder.Encode(td.SpendHash)
	if err != nil {
		return nil, err
	}
	err = encoder.Encode(td.BlockHeight)
	if err != nil {
		return nil, err
	}
	err = encoder.Encode(td.Missed)
	if err != nil {
		return nil, err
	}
	err = encoder.Encode(td.Expired)
	if err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// GobDecode deserializes the TicketData struct into a gob for use in retrieval
// from storage.
func (td *TicketData) GobDecode(buf []byte) error {
	r := bytes.NewBuffer(buf)
	decoder := gob.NewDecoder(r)

	err := decoder.Decode(&td.SStxHash)
	if err != nil {
		return err
	}
	err = decoder.Decode(&td.Prefix)
	if err != nil {
		return err
	}
	err = decoder.Decode(&td.SpendHash)
	if err != nil {
		return err
	}
	err = decoder.Decode(&td.BlockHeight)
	if err != nil {
		return err
	}
	err = decoder.Decode(&td.Missed)
	if err != nil {
		return err
	}
	return decoder.Decode(&td.Expired)
}

// TicketDataSlice is a sortable data structure of pointers to TicketData.
type TicketDataSlice []*TicketData

// NewTicketDataSliceEmpty creates and returns a new, empty slice of
// TIcketData.
func NewTicketDataSliceEmpty() TicketDataSlice {
	var slice []*TicketData
	return TicketDataSlice(slice)
}

// NewTicketDataSlice creates and returns TicketData of the requested size.
func NewTicketDataSlice(size int) TicketDataSlice {
	slice := make([]*TicketData, size)
	return TicketDataSlice(slice)
}

// Less determines which of two *TicketData values is smaller; used for sort.
func (tds TicketDataSlice) Less(i, j int) bool {
	cmp := bytes.Compare(tds[i].SStxHash[:], tds[j].SStxHash[:])
	isISmaller := (cmp == -1)
	return isISmaller
}

// Swap swaps two *TicketData values.
func (tds TicketDataSlice) Swap(i, j int) { tds[i], tds[j] = tds[j], tds[i] }

// Len returns the length of the slice.
func (tds TicketDataSlice) Len() int { return len(tds) }

// SStxMemMap is a memory map of SStx keyed to the txHash.
type SStxMemMap map[chainhash.Hash]*TicketData

// TicketMaps is a struct of maps that encompass the four major buckets of the
// ticket in-memory database.
type TicketMaps struct {
	ticketMap        []SStxMemMap
	spentTicketMap   map[int64]SStxMemMap
	missedTicketMap  SStxMemMap
	revokedTicketMap SStxMemMap
}

// GobEncode serializes the TicketMaps struct into a gob for use in storage.
func (tm *TicketMaps) GobEncode() ([]byte, error) {
	w := new(bytes.Buffer)
	encoder := gob.NewEncoder(w)

	err := encoder.Encode(tm.ticketMap)
	if err != nil {
		return nil, err
	}
	err = encoder.Encode(tm.spentTicketMap)
	if err != nil {
		return nil, err
	}
	err = encoder.Encode(tm.missedTicketMap)
	if err != nil {
		return nil, err
	}
	err = encoder.Encode(tm.revokedTicketMap)
	if err != nil {
		return nil, err
	}
	return w.Bytes(), nil
}

// GobDecode deserializes the TicketMaps struct into a gob for use in retrieval
// from storage.
func (tm *TicketMaps) GobDecode(buf []byte) error {
	r := bytes.NewBuffer(buf)
	decoder := gob.NewDecoder(r)

	err := decoder.Decode(&tm.ticketMap)
	if err != nil {
		return err
	}
	err = decoder.Decode(&tm.spentTicketMap)
	if err != nil {
		return err
	}
	err = decoder.Decode(&tm.missedTicketMap)
	if err != nil {
		return err
	}
	return decoder.Decode(&tm.revokedTicketMap)
}

// TicketDB is the ticket in-memory database.
type TicketDB struct {
	mtx                sync.Mutex
	maps               TicketMaps
	database           database.DB
	chainParams        *chaincfg.Params
	StakeEnabledHeight int64
}

// bestChainState represents the data to be stored the database for the current
// best chain state.
type bestChainState struct {
	hash         chainhash.Hash
	height       uint32
	totalTxns    uint64
	totalSubsidy int64
	workSum      *big.Int
}

// deserializeBestChainState deserializes the passed serialized best chain
// state.  This is data stored in the chain state bucket and is updated after
// every block is connected or disconnected form the main chain.
// block.
func deserializeBestChainState(serializedData []byte) (bestChainState, error) {
	// Ensure the serialized data has enough bytes to properly deserialize
	// the hash, height, total transactions, total subsidy, current subsidy,
	// and work sum length.
	expectedMinLen := chainhash.HashSize + 4 + 8 + 8 + 4
	if len(serializedData) < expectedMinLen {
		return bestChainState{}, database.Error{
			ErrorCode: database.ErrCorruption,
			Description: fmt.Sprintf("corrupt best chain state size; min %v "+
				"got %v", expectedMinLen, len(serializedData)),
		}
	}

	state := bestChainState{}
	copy(state.hash[:], serializedData[0:chainhash.HashSize])
	offset := uint32(chainhash.HashSize)
	state.height = dbnamespace.ByteOrder.Uint32(serializedData[offset : offset+4])
	offset += 4
	state.totalTxns = dbnamespace.ByteOrder.Uint64(
		serializedData[offset : offset+8])
	offset += 8
	state.totalSubsidy = int64(dbnamespace.ByteOrder.Uint64(
		serializedData[offset : offset+8]))
	offset += 8
	workSumBytesLen := dbnamespace.ByteOrder.Uint32(
		serializedData[offset : offset+4])
	offset += 4

	// Ensure the serialized data has enough bytes to deserialize the work
	// sum.
	if uint32(len(serializedData[offset:])) < workSumBytesLen {
		return bestChainState{}, database.Error{
			ErrorCode: database.ErrCorruption,
			Description: fmt.Sprintf("corrupt work sum size; want %v "+
				"got %v", workSumBytesLen, uint32(len(serializedData[offset:]))),
		}
	}
	workSumBytes := serializedData[offset : offset+workSumBytesLen]
	state.workSum = new(big.Int).SetBytes(workSumBytes)

	return state, nil
}

// NewestSha returns the newest hash and height as recorded in the database
// of the blockchain.
func (tmdb *TicketDB) NewestSha() (*chainhash.Hash, int64, error) {
	var state bestChainState
	err := tmdb.database.View(func(dbTx database.Tx) error {
		// Fetch the stored chain state from the database metadata.
		// When it doesn't exist, it means the database hasn't been
		// initialized for use with chain yet, so break out now to allow
		// that to happen under a writable database transaction.
		serializedData := dbTx.Metadata().Get(dbnamespace.ChainStateKeyName)
		if serializedData == nil {
			return nil
		}

		var err error
		state, err = deserializeBestChainState(serializedData)
		if err != nil {
			return err
		}

		return nil
	})

	return &state.hash, int64(state.height), err
}

// FetchBlockShaByHeight queries the blockchain's database to find a block's hash
// for some height.
func (tmdb *TicketDB) FetchBlockShaByHeight(height int64) (*chainhash.Hash, error) {
	var hash chainhash.Hash
	err := tmdb.database.View(func(dbTx database.Tx) error {
		var serializedHeight [4]byte
		dbnamespace.ByteOrder.PutUint32(serializedHeight[:], uint32(height))

		meta := dbTx.Metadata()
		heightIndex := meta.Bucket(dbnamespace.HeightIndexBucketName)
		hashBytes := heightIndex.Get(serializedHeight[:])
		if hashBytes == nil {
			return fmt.Errorf("no block at height %d exists", height)
		}

		copy(hash[:], hashBytes)
		return nil
	})

	return &hash, err
}

// FetchBlockBySha fetches a block from a given hash using the blockchain
// database.
func (tmdb *TicketDB) FetchBlockBySha(hash *chainhash.Hash) (*dcrutil.Block, error) {
	var block *dcrutil.Block
	err := tmdb.database.View(func(dbTx database.Tx) error {
		rawBytes, err := dbTx.FetchBlock(hash)
		if err != nil {
			return err
		}
		block, err = dcrutil.NewBlockFromBytes(rawBytes)
		if err != nil {
			return err
		}

		return nil
	})

	block.SetHeight(int64(block.MsgBlock().Header.Height))

	return block, err
}

// Initialize allocates buckets for each ticket number in ticketMap and buckets
// for each height up to the declared height from 0. This should be called only
// when no suitable files exist to load the TicketDB from or when
// rescanTicketDB() is called.
// WARNING: Height should be 0 for all non-debug uses.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) Initialize(np *chaincfg.Params, db database.DB) error {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	tmdb.chainParams = np
	tmdb.database = db
	tmdb.maps.ticketMap = make([]SStxMemMap, BucketsSize, BucketsSize)
	tmdb.maps.spentTicketMap = make(map[int64]SStxMemMap)
	tmdb.maps.missedTicketMap = make(SStxMemMap)
	tmdb.maps.revokedTicketMap = make(SStxMemMap)

	tmdb.StakeEnabledHeight = np.StakeEnabledHeight

	// Fill in live ticket buckets.
	for i := 0; i < BucketsSize; i++ {
		tmdb.maps.ticketMap[uint8(i)] = make(SStxMemMap)
	}

	// Get the latest block height from the db.
	_, curHeight, err := tmdb.NewestSha()
	if err != nil {
		return err
	}
	log.Infof("Block ticket database initialized empty")

	if curHeight > 0 {
		log.Infof("Db non-empty, resyncing ticket DB")
		err := tmdb.RescanTicketDB()

		if err != nil {
			return err
		}
	}

	return nil
}

// maybeInsertBlock creates a new bucket in the spentTicketMap; this should be
// called this whenever we try to alter the spentTicketMap; this results in
// a lot of redundant calls, but I don't think they're expensive.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) maybeInsertBlock(height int64) {
	// Check if the bucket exists for the given height.
	if tmdb.maps.spentTicketMap[height] != nil {
		return
	}

	// If it doesn't exist, make it.
	tmdb.maps.spentTicketMap[height] = make(SStxMemMap)
	return
}

// getTopBlock is the internal function which implements the public
// GetTopBlock.  See the comment for GetTopBlock for more details.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) getTopBlock() int64 {
	// Discover the current height.
	topHeight := tmdb.StakeEnabledHeight
	for {
		if tmdb.maps.spentTicketMap[topHeight] == nil {
			topHeight--
			break
		}
		topHeight++
	}

	// If we aren't yet at a stake mature blockchain.
	if topHeight == (tmdb.StakeEnabledHeight - 1) {
		return int64(-1)
	}
	return topHeight
}

// GetTopBlock returns the top (current) block from a TicketDB.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) GetTopBlock() int64 {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	return tmdb.getTopBlock()
}

// LoadTicketDBs fetches the stored TicketDB and UsedTicketDB from the disk and
// stores them.
// Call this after the blockchain has been loaded into the daemon.
// TODO: Make a function that checks to see if the files exist before attempting
// to load them? Or do that elsewhere.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) LoadTicketDBs(tmsPath, tmsLoc string, np *chaincfg.Params,
	db database.DB) error {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	tmdb.chainParams = np
	tmdb.database = db

	tmdb.StakeEnabledHeight = np.StakeEnabledHeight

	filename := filepath.Join(tmsPath, tmsLoc)

	// Load the maps from disk.
	diskTicketMaps, errDiskTM := ioutil.ReadFile(filename)
	if errDiskTM != nil {
		return fmt.Errorf("TicketDB err @ loadTicketDBs: could not load " +
			"serialized ticketMaps from disk")
	}

	// Create buffer for maps and load the raw file into it.
	var loadedTicketMaps TicketMaps

	// Decode the maps from the buffer.
	errDeserialize := loadedTicketMaps.GobDecode(diskTicketMaps)
	if errDeserialize != nil {
		return fmt.Errorf("could not deserialize stored ticketMaps")
	}
	tmdb.maps = loadedTicketMaps

	// Get the latest block height from the database.
	_, curHeight, err := tmdb.NewestSha()
	if err != nil {
		return err
	}

	// Check and see if the height of spentTicketMap is the same as the current
	// height of the blockchain. If it isn't, spin up next DB for both from the
	// blockchain itself (rescanTicketDB).
	stmHeight := tmdb.getTopBlock()

	// The database chain is shorter than the ticket db chain, abort.
	if stmHeight > curHeight {
		return fmt.Errorf("Ticket DB err @ loadTicketDbs: there were more "+
			"blocks in the ticketDb (%v) than there were in the "+
			"main db (%v); try deleting the ticket database file",
			stmHeight, curHeight)
	}

	// The ticket db chain is shorter than the database chain, resync.
	if stmHeight < curHeight {
		log.Debugf("current height: %v, stm height %v", curHeight, stmHeight)
		log.Warnf("Accessory ticket database is desynced, " +
			"resyncing now")

		err := tmdb.rescanTicketDB()
		if err != nil {
			return err
		}
	}

	// If maps are empty pre-serializing, they'll be nil upon loading.
	// Check to make sure that no maps are nil; if they are, generate
	// them.
	if tmdb.maps.ticketMap == nil {
		tmdb.maps.ticketMap = make([]SStxMemMap, BucketsSize, BucketsSize)

		// Fill in live ticket buckets
		for i := uint8(0); ; i++ {
			tmdb.maps.ticketMap[i] = make(SStxMemMap)

			if i == math.MaxUint8 {
				break
			}
		}
	}

	if tmdb.maps.spentTicketMap == nil {
		tmdb.maps.spentTicketMap = make(map[int64]SStxMemMap)
	}

	if tmdb.maps.missedTicketMap == nil {
		tmdb.maps.missedTicketMap = make(SStxMemMap)
	}

	if tmdb.maps.revokedTicketMap == nil {
		tmdb.maps.revokedTicketMap = make(SStxMemMap)
	}

	return nil
}

// Store serializes and stores a TicketDB. Only intended to be called when shut
// down has been initiated and daemon network activity has ceased.
// TODO: Serialize in a way that is cross-platform instead of gob encoding.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) Store(tmsPath string, tmsLoc string) error {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	log.Infof("Storing the ticket database to disk")

	ticketMapsBytes, err := tmdb.maps.GobEncode()
	if err != nil {
		return fmt.Errorf("could not serialize ticketMaps: %v", err.Error())
	}

	filename := filepath.Join(tmsPath, tmsLoc)

	// Write the encoded ticketMap and spentTicketMap to disk
	if err := ioutil.WriteFile(filename, ticketMapsBytes, 0644); err != nil {
		return fmt.Errorf("could not write serialized "+
			"ticketMaps to disk: %v", err.Error())
	}

	return nil
}

// Close deletes a TicketDB and its contents. Intended to be called only when
// store() has first been called.
// Decred: In the daemon this is never called because it causes some problems
// with storage. As everything is a native Go structure in the first place,
// we don't really need this as far as I can tell, but I'll leave this in here
// in case a usage is found.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) Close() {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	return
}

// --------------------------------------------------------------------------------
// ! WARNING
// THESE ARE DIRECT MANIPULATION FUNCTIONS THAT SHOULD MAINLY BE USED INTERNALLY
// OR FOR FOR DEBUGGING PURPOSES ONLY.

// pushLiveTicket pushes a mature ticket into the ticketMap.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) pushLiveTicket(ticket *TicketData) error {
	// Make sure the ticket bucket exists; if it doesn't something has gone wrong
	// with the initialization
	if tmdb.maps.ticketMap[ticket.Prefix] == nil {
		return fmt.Errorf("TicketDB err @ pushLiveTicket: bucket for tickets "+
			"numbered %v missing", ticket.Prefix)
	}

	// Make sure the ticket isn't already in the map
	if tmdb.maps.ticketMap[ticket.Prefix][ticket.SStxHash] != nil {
		return fmt.Errorf("TicketDB err @ pushLiveTicket: ticket with hash %v "+
			"already exists", ticket.SStxHash)
	}

	// Always false going into live ticket map
	ticket.Missed = false

	// Put the ticket in its respective bucket in the map
	tmdb.maps.ticketMap[ticket.Prefix][ticket.SStxHash] = ticket

	return nil
}

// pushSpentTicket pushes a used ticket into the spentTicketMap.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) pushSpentTicket(spendHeight int64, ticket *TicketData) error {
	// Make sure there's a bucket in the map for used tickets
	tmdb.maybeInsertBlock(spendHeight)

	// Make sure the ticket isn't already in the map
	if tmdb.maps.spentTicketMap[spendHeight][ticket.SStxHash] != nil {
		return fmt.Errorf("TicketDB err @ pushSpentTicket: ticket with hash "+
			"%v already exists", ticket.SStxHash)
	}

	tmdb.maps.spentTicketMap[spendHeight][ticket.SStxHash] = ticket

	return nil
}

// pushMissedTicket pushes a used ticket into the spentTicketMap.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) pushMissedTicket(ticket *TicketData) error {
	// Make sure the map exists.
	if tmdb.maps.missedTicketMap == nil {
		return fmt.Errorf("TicketDB err @ pushMissedTicket: map missing")
	}

	// Make sure the ticket isn't already in the map.
	if tmdb.maps.missedTicketMap[ticket.SStxHash] != nil {
		return fmt.Errorf("TicketDB err @ pushMissedTicket: ticket with "+
			"hash %v already exists", ticket.SStxHash)
	}

	// Always true going into missedTicketMap.
	ticket.Missed = true

	tmdb.maps.missedTicketMap[ticket.SStxHash] = ticket

	return nil
}

// pushRevokedTicket pushes a used ticket into the spentTicketMap.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) pushRevokedTicket(ticket *TicketData) error {
	// Make sure the map exists.
	if tmdb.maps.revokedTicketMap == nil {
		return fmt.Errorf("TicketDB err @ pushRevokedTicket: map missing")
	}

	// Make sure the ticket isn't already in the map.
	if tmdb.maps.revokedTicketMap[ticket.SStxHash] != nil {
		return fmt.Errorf("TicketDB err @ pushRevokedTicket: ticket with "+
			"hash %v already exists", ticket.SStxHash)
	}

	// Always true going into revokedTicketMap.
	ticket.Missed = true

	tmdb.maps.revokedTicketMap[ticket.SStxHash] = ticket

	return nil
}

// removeLiveTicket removes live tickets that were added to the ticketMap from
// tickets maturing.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) removeLiveTicket(ticket *TicketData) error {
	// Make sure the ticket bucket exists; if it doesn't something has gone wrong
	// with the initialization
	if tmdb.maps.ticketMap[ticket.Prefix] == nil {
		return fmt.Errorf("TicketDB err @ removeLiveTicket: bucket for "+
			"tickets numbered %v missing", ticket.Prefix)
	}

	// Make sure the ticket itself exists
	if tmdb.maps.ticketMap[ticket.Prefix][ticket.SStxHash] == nil {
		return fmt.Errorf("TicketDB err @ removeLiveTicket: ticket %v to "+
			"delete does not exist!", ticket.SStxHash)
	}

	// Make sure that the tickets are identical in the unlikely case of a hash
	// collision
	if *tmdb.maps.ticketMap[ticket.Prefix][ticket.SStxHash] != *ticket {
		return fmt.Errorf("TicketDB err @ removeLiveTicket: ticket " +
			"hash duplicate, but non-identical data")
	}

	delete(tmdb.maps.ticketMap[ticket.Prefix], ticket.SStxHash)
	return nil
}

// removeSpentTicket removes spent tickets that were added to the spentTicketMap.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) removeSpentTicket(spendHeight int64, ticket *TicketData) error {
	// Make sure the height bucket exists; if it doesn't something has gone wrong
	// with the initialization
	if tmdb.maps.spentTicketMap[spendHeight] == nil {
		return fmt.Errorf("TicketDB err @ removeSpentTicket: bucket for "+
			"BlockHeight numbered %v missing", ticket.BlockHeight)
	}

	// Make sure the ticket itself exists
	if tmdb.maps.spentTicketMap[spendHeight][ticket.SStxHash] == nil {
		return fmt.Errorf("TicketDB err @ removeSpentTicket: ticket to "+
			"delete does not exist! %v", ticket.SStxHash)
	}

	// Make sure that the tickets are identical in the unlikely case of a hash
	// collision
	if *tmdb.maps.spentTicketMap[spendHeight][ticket.SStxHash] != *ticket {
		return fmt.Errorf("TicketDB err @ removeSpentTicket: ticket hash " +
			"duplicate, but non-identical data")
	}

	delete(tmdb.maps.spentTicketMap[spendHeight], ticket.SStxHash)
	return nil
}

// removeMissedTicket removes missed tickets that were added to the spentTicketMap.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) removeMissedTicket(ticket *TicketData) error {
	// Make sure the map exists.
	if tmdb.maps.missedTicketMap == nil {
		return fmt.Errorf("TicketDB err @ removeMissedTicket: map missing")
	}

	// Make sure the ticket exists
	if tmdb.maps.missedTicketMap[ticket.SStxHash] == nil {
		return fmt.Errorf("TicketDB err @ removeMissedTicket: ticket to "+
			"delete does not exist! %v", ticket.SStxHash)
	}

	// Make sure that the tickets are identical in the unlikely case of a hash
	// collision
	if *tmdb.maps.missedTicketMap[ticket.SStxHash] != *ticket {
		return fmt.Errorf("TicketDB err @ removeMissedTicket: ticket hash " +
			"duplicate, but non-identical data")
	}

	delete(tmdb.maps.missedTicketMap, ticket.SStxHash)
	return nil
}

// removeRevokedTicket removes missed tickets that were added to the
// revoked ticket map.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) removeRevokedTicket(ticket *TicketData) error {
	// Make sure the map exists.
	if tmdb.maps.revokedTicketMap == nil {
		return fmt.Errorf("TicketDB err @ removeRevokedTicket: map missing")
	}

	// Make sure the ticket exists.
	if tmdb.maps.revokedTicketMap[ticket.SStxHash] == nil {
		return fmt.Errorf("TicketDB err @ removeRevokedTicket: ticket to "+
			"delete does not exist! %v", ticket.SStxHash)
	}

	// Make sure that the tickets are identical in the unlikely case of a hash
	// collision.
	if *tmdb.maps.revokedTicketMap[ticket.SStxHash] != *ticket {
		return fmt.Errorf("TicketDB err @ removeRevokedTicket: ticket hash " +
			"duplicate, but non-identical data")
	}

	delete(tmdb.maps.revokedTicketMap, ticket.SStxHash)
	return nil
}

// removeSpentHeight removes a height bucket from the SpentTicketMap.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) removeSpentHeight(height int64) error {
	// Make sure the height exists
	if tmdb.maps.spentTicketMap[height] == nil {
		return fmt.Errorf("TicketDB err @ removeSpentHeight: height to "+
			"delete does not exist! %v", height)
	}

	delete(tmdb.maps.spentTicketMap, height)
	return nil
}

// DumpMapsPointer is a testing function that returns a pointer to
// the internally held maps. Used for testing.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) DumpMapsPointer() TicketMaps {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	return tmdb.maps
}

// END DIRECT ACCESS/DEBUG TOOLS.
// --------------------------------------------------------------------------------

// cloneSStxMemMap is a helper function to clone mem maps to avoid races.
func cloneSStxMemMap(mapToCopy SStxMemMap) SStxMemMap {
	newMemMap := make(SStxMemMap)

	for hash, ticket := range mapToCopy {
		newMemMap[hash] = NewTicketData(ticket.SStxHash,
			ticket.Prefix,
			ticket.SpendHash,
			ticket.BlockHeight,
			ticket.Missed,
			ticket.Expired)
	}

	return newMemMap
}

// CheckLiveTicket checks for the existence of a live ticket in ticketMap.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) CheckLiveTicket(txHash chainhash.Hash) (bool, error) {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	prefix := txHash[0]

	// Make sure the ticket bucket exists; if it doesn't something has gone wrong
	// with the initialization
	if tmdb.maps.ticketMap[prefix] == nil {
		return false, fmt.Errorf("TicketDB err @ checkLiveTicket: bucket for "+
			"tickets numbered %v missing", prefix)
	}

	if tmdb.maps.ticketMap[prefix][txHash] != nil {
		return true, nil
	}
	return false, nil
}

// CheckMissedTicket checks for the existence of a missed ticket in the missed
// ticket map. Assumes missedTicketMap is initialized.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) CheckMissedTicket(txHash chainhash.Hash) bool {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	if _, exists := tmdb.maps.missedTicketMap[txHash]; exists {
		return true
	}
	return false
}

// CheckRevokedTicket checks for the existence of a revoked ticket in the
// revoked ticket map. Assumes missedTicketMap is initialized.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) CheckRevokedTicket(txHash chainhash.Hash) bool {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	if _, exists := tmdb.maps.revokedTicketMap[txHash]; exists {
		return true
	}
	return false
}

// DumpLiveTickets duplicates the contents of a ticket bucket from the databases's
// ticketMap and returns them to the user.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) DumpLiveTickets(bucket uint8) (SStxMemMap, error) {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	tickets := make(SStxMemMap)

	// Make sure the ticket bucket exists; if it doesn't something has gone wrong
	// with the initialization
	if tmdb.maps.ticketMap[bucket] == nil {
		return nil, fmt.Errorf("TicketDB err @ DumpLiveTickets: bucket for "+
			"tickets numbered %v missing", bucket)
	}

	for _, ticket := range tmdb.maps.ticketMap[bucket] {
		tickets[ticket.SStxHash] = NewTicketData(ticket.SStxHash,
			ticket.Prefix,
			ticket.SpendHash,
			ticket.BlockHeight,
			ticket.Missed,
			ticket.Expired)
	}
	return tickets, nil
}

// DumpAllLiveTicketHashes duplicates the contents of a ticket bucket from the
// databases's ticketMap and returns them to the user.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) DumpAllLiveTicketHashes() ([]*chainhash.Hash, error) {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	var tickets []*chainhash.Hash

	for i := 0; i < BucketsSize; i++ {
		// Make sure the ticket bucket exists; if it doesn't something
		// has gone wrong with the initialization
		if tmdb.maps.ticketMap[i] == nil {
			return nil, fmt.Errorf("TicketDB err @ DumpLiveTickets: bucket for "+
				"tickets numbered %v missing", i)
		}

		for _, v := range tmdb.maps.ticketMap[i] {
			tickets = append(tickets, &v.SStxHash)
		}
	}

	return tickets, nil
}

// DumpSpentTickets duplicates the contents of a ticket bucket from the databases's
// spentTicketMap and returns them to the user.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) DumpSpentTickets(height int64) (SStxMemMap, error) {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	tickets := make(SStxMemMap)

	// Make sure the ticket bucket exists; if it doesn't something has gone wrong
	// with the initialization
	if tmdb.maps.spentTicketMap[height] == nil {
		return nil, fmt.Errorf("TicketDB err @ dumpSpentTickets: bucket for "+
			"tickets numbered %v missing", height)
	}

	for _, ticket := range tmdb.maps.spentTicketMap[height] {
		tickets[ticket.SStxHash] = NewTicketData(ticket.SStxHash,
			ticket.Prefix,
			ticket.SpendHash,
			ticket.BlockHeight,
			ticket.Missed,
			ticket.Expired)
	}
	return tickets, nil
}

// DumpMissedTickets duplicates the contents of a ticket bucket from the
// databases's missedTicketMap and returns them to the user.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) DumpMissedTickets() (SStxMemMap, error) {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	tickets := make(SStxMemMap)

	// Make sure the map is actually initialized.
	if tmdb.maps.missedTicketMap == nil {
		return nil, fmt.Errorf("TicketDB err @ missedTicketMap: map for " +
			"missed tickets uninitialized")
	}

	for _, ticket := range tmdb.maps.missedTicketMap {
		tickets[ticket.SStxHash] = NewTicketData(ticket.SStxHash,
			ticket.Prefix,
			ticket.SpendHash,
			ticket.BlockHeight,
			ticket.Missed,
			ticket.Expired)
	}
	return tickets, nil
}

// DumpRevokedTickets duplicates the contents of a ticket bucket from the
// databases's missedTicketMap and returns them to the user.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) DumpRevokedTickets() (SStxMemMap, error) {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	tickets := make(SStxMemMap)

	// Make sure the map is actually initialized.
	if tmdb.maps.revokedTicketMap == nil {
		return nil, fmt.Errorf("TicketDB err @ revokedTicketMap: map for " +
			"revoked tickets uninitialized")
	}

	for _, ticket := range tmdb.maps.revokedTicketMap {
		tickets[ticket.SStxHash] = NewTicketData(ticket.SStxHash,
			ticket.Prefix,
			ticket.SpendHash,
			ticket.BlockHeight,
			ticket.Missed,
			ticket.Expired)
	}
	return tickets, nil
}

// GetMissedTicket locates a missed ticket in the missed ticket database,
// duplicates the ticket data, and returns it.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) GetMissedTicket(hash chainhash.Hash) *TicketData {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	if ticket, exists := tmdb.maps.missedTicketMap[hash]; exists {
		return NewTicketData(ticket.SStxHash,
			ticket.Prefix,
			ticket.SpendHash,
			ticket.BlockHeight,
			ticket.Missed,
			ticket.Expired)
	}
	return nil
}

// GetRevokedTicket locates a revoked ticket in the revoked ticket database,
// duplicates the ticket data, and returns it.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) GetRevokedTicket(hash chainhash.Hash) *TicketData {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	if ticket, exists := tmdb.maps.revokedTicketMap[hash]; exists {
		return NewTicketData(ticket.SStxHash,
			ticket.Prefix,
			ticket.SpendHash,
			ticket.BlockHeight,
			ticket.Missed,
			ticket.Expired)
	}
	return nil
}

// GetLiveTicketBucketData creates a map of [int]int indicating the number
// of tickets in each bucket. Used for an RPC call.
func (tmdb *TicketDB) GetLiveTicketBucketData() map[int]int {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	ltbd := make(map[int]int)
	for i := 0; i < BucketsSize; i++ {
		ltbd[int(i)] = len(tmdb.maps.ticketMap[i])
	}

	return ltbd
}

// spendTickets transfers tickets from the ticketMap to the spentTicketMap. Useful
// when connecting blocks. Also pushes missed tickets to the missed ticket map.
// usedtickets is a map that contains all tickets that were actually used in SSGen
// votes; all other tickets are considered missed.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) spendTickets(parentBlock *dcrutil.Block,
	usedTickets map[chainhash.Hash]struct{},
	spendingHashes map[chainhash.Hash]chainhash.Hash) (SStxMemMap, error) {

	// If there is nothing being spent, break.
	if len(spendingHashes) < 1 {
		return nil, nil
	}

	// Make sure there's a bucket in the map for used tickets
	height := parentBlock.Height() + 1
	tmdb.maybeInsertBlock(height)

	tempTickets := make(SStxMemMap)

	// Sort the entire list of tickets lexicographically by sorting
	// each bucket and then appending it to the list.
	totalTickets := 0
	var sortedSlice []*TicketData
	for i := 0; i < BucketsSize; i++ {
		mapLen := len(tmdb.maps.ticketMap[i])
		totalTickets += mapLen
		tempTdSlice := NewTicketDataSlice(mapLen)
		itr := 0 // Iterator
		for _, td := range tmdb.maps.ticketMap[i] {
			tempTdSlice[itr] = td
			itr++
		}
		sort.Sort(tempTdSlice)
		sortedSlice = append(sortedSlice, tempTdSlice...)
	}

	// Use the parent block's header to seed a PRNG that picks the lottery winners.
	ticketsPerBlock := int(tmdb.chainParams.TicketsPerBlock)
	pbhB, err := parentBlock.MsgBlock().Header.Bytes()
	if err != nil {
		return nil, err
	}
	prng := NewHash256PRNG(pbhB)
	ts, err := FindTicketIdxs(int64(totalTickets), ticketsPerBlock, prng)
	if err != nil {
		return nil, err
	}
	ticketsToSpendOrMiss := make([]*TicketData, ticketsPerBlock, ticketsPerBlock)
	for i, idx := range ts {
		ticketsToSpendOrMiss[i] = sortedSlice[idx]
	}

	// Spend or miss these tickets by checking for their existence in the
	// passed usedtickets map.
	tixSpent := 0
	tixMissed := 0
	for _, ticket := range ticketsToSpendOrMiss {
		// Move the ticket from active tickets map into the used tickets map
		// if the ticket was spent.
		_, wasSpent := usedTickets[ticket.SStxHash]

		if wasSpent {
			ticket.Missed = false
			ticket.SpendHash = spendingHashes[ticket.SStxHash]
			err := tmdb.pushSpentTicket(height, ticket)
			if err != nil {
				return nil, err
			}
			err = tmdb.removeLiveTicket(ticket)
			if err != nil {
				return nil, err
			}
			tixSpent++
		} else { // Ticket missed being spent and --> false or nil
			ticket.Missed = true // TODO fix test failure @ L150 due to this
			err := tmdb.pushSpentTicket(height, ticket)
			if err != nil {
				return nil, err
			}
			err = tmdb.pushMissedTicket(ticket)
			if err != nil {
				return nil, err
			}
			err = tmdb.removeLiveTicket(ticket)
			if err != nil {
				return nil, err
			}
			tixMissed++
		}

		// Report on the spent and missed tickets for the block in debug.
		if ticket.Missed {
			log.Debugf("Ticket %v has been missed and expired from "+
				"the lottery pool as a missed ticket", ticket.SStxHash)
		} else {
			log.Debugf("Ticket %v was spent and removed from "+
				"the lottery pool", ticket.SStxHash)
		}

		// Add the ticket to the temporary tickets buffer for later use in
		// map restoration if needed.
		tempTickets[ticket.SStxHash] = ticket
	}

	// Some sanity checks.
	if tixSpent != len(usedTickets) {
		errStr := fmt.Sprintf("spendTickets error, an invalid number %v "+
			"tickets was spent, but %v many tickets should "+
			"have been spent!", tixSpent, len(usedTickets))
		return nil, errors.New(errStr)
	}

	if tixMissed != (ticketsPerBlock - len(usedTickets)) {
		errStr := fmt.Sprintf("spendTickets error, an invalid number %v "+
			"tickets was missed, but %v many tickets should "+
			"have been missed!", tixMissed, ticketsPerBlock-len(usedTickets))
		return nil, errors.New(errStr)
	}

	if (tixSpent + tixMissed) != ticketsPerBlock {
		errStr := fmt.Sprintf("spendTickets error, an invalid number %v "+
			"tickets was spent and missed, but TicketsPerBlock %v many "+
			"tickets should have been spent!", tixSpent, ticketsPerBlock)
		return nil, errors.New(errStr)
	}

	return tempTickets, nil
}

// expireTickets looks all tickets in the live ticket bucket at height
// TicketExpiry many blocks ago, and then any tickets now expiring to the missed
// tickets map.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) expireTickets(height int64) (SStxMemMap, error) {
	toExpireHeight := height - int64(tmdb.chainParams.TicketExpiry)
	if toExpireHeight < int64(tmdb.StakeEnabledHeight) {
		return nil, nil
	}

	expiredTickets := make(SStxMemMap)

	for i := 0; i < BucketsSize; i++ {
		for _, ticket := range tmdb.maps.ticketMap[i] {
			if ticket.BlockHeight == toExpireHeight {
				err := tmdb.pushSpentTicket(height, ticket)
				if err != nil {
					return nil, err
				}
				err = tmdb.pushMissedTicket(ticket)
				if err != nil {
					return nil, err
				}
				err = tmdb.removeLiveTicket(ticket)
				if err != nil {
					return nil, err
				}

				ticket.Expired = true
				expiredTickets[ticket.SStxHash] = ticket
			}
		}
	}

	return expiredTickets, nil
}

// revokeTickets takes a list of revoked tickets from SSRtx and removes them
// from the missedTicketMap, then returns all the killed tickets in a map.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) revokeTickets(
	revocations map[chainhash.Hash]struct{}) (SStxMemMap, error) {

	revokedTickets := make(SStxMemMap)

	for hash := range revocations {
		ticket := tmdb.maps.missedTicketMap[hash]

		if ticket == nil {
			return nil, errors.New("revokeTickets attempted to revoke ticket " +
				"not found in missedTicketsMap!")
		}
		revokedTickets[hash] = ticket

		err := tmdb.pushRevokedTicket(ticket)
		if err != nil {
			return nil, err
		}

		err = tmdb.removeMissedTicket(ticket)
		if err != nil {
			return nil, err
		}
	}

	return revokedTickets, nil
}

// unrevokeTickets takes a list of revoked tickets from SSRtx and moves them to
// the missedTicketMap, then returns all these tickets in a map.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) unrevokeTickets(height int64) (SStxMemMap, error) {
	// Get the block of interest.
	var hash, errHash = tmdb.FetchBlockShaByHeight(height)
	if errHash != nil {
		return nil, errHash
	}

	var block, errBlock = tmdb.FetchBlockBySha(hash)
	if errBlock != nil {
		return nil, errBlock
	}

	revocations := make(map[chainhash.Hash]bool)

	for _, staketx := range block.STransactions() {
		if is, _ := IsSSRtx(staketx); is {
			msgTx := staketx.MsgTx()
			sstxIn := msgTx.TxIn[0] // sstx input
			sstxHash := sstxIn.PreviousOutPoint.Hash

			revocations[sstxHash] = true
		}
	}

	unrevokedTickets := make(SStxMemMap)

	for hash := range revocations {
		ticket := tmdb.maps.revokedTicketMap[hash]

		if ticket == nil {
			return nil, errors.New("unrevokeTickets attempted to unrevoke " +
				"ticket not found in revokedTicketsMap!")
		}

		unrevokedTickets[hash] = ticket

		err := tmdb.pushMissedTicket(ticket)
		if err != nil {
			return nil, err
		}

		err = tmdb.removeRevokedTicket(ticket)
		if err != nil {
			return nil, err
		}
	}

	return unrevokedTickets, nil
}

// unspendTickets unspends all tickets that were previously spent at some height to
// the ticketMap; used for rolling back changes from the old main chain blocks when
// encountering and validating a fork.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) unspendTickets(height int64) (SStxMemMap, error) {
	tempTickets := make(SStxMemMap)

	for _, ticket := range tmdb.maps.spentTicketMap[height] {
		if ticket.Missed == true {
			err := tmdb.removeMissedTicket(ticket)
			if err != nil {
				return nil, err
			}

			// Marked that it was not missed or expired.
			ticket.Missed = false
			ticket.Expired = false
		}

		// Zero out the spend hash.
		ticket.SpendHash = chainhash.Hash{}

		// Add the ticket to the temporary tickets buffer for later use in
		// map restoration if needed.
		tempTickets[ticket.SStxHash] = ticket

		// Move the ticket from used tickets map into the active tickets map.
		err := tmdb.pushLiveTicket(ticket)
		if err != nil {
			return nil, err
		}

		// Delete the ticket from the spent ticket map.
		err = tmdb.removeSpentTicket(height, ticket)
		if err != nil {
			return nil, err
		}
	}

	// Delete the height itself from the spentTicketMap.
	err := tmdb.removeSpentHeight(height)
	if err != nil {
		return nil, err
	}

	return tempTickets, nil
}

// getNewTicketsFromHeight loads a block from leveldb and parses SStx from it using
// chain/stake's IsSStx function.
// This is intended to be used to get ticket numbers from the MAIN CHAIN as
// described in the DB.
// SIDE CHAIN evaluation should be instantiated in package:chain.
//
// This function MUST be called with the tmdb lock held (for reads).
func (tmdb *TicketDB) getNewTicketsFromHeight(height int64) (SStxMemMap, error) {
	if height < tmdb.StakeEnabledHeight {
		errStr := fmt.Sprintf("Tried to generate tickets for immature blockchain"+
			" at height %v", height)
		return nil, errors.New(errStr)
	}

	matureHeight := height - int64(tmdb.chainParams.TicketMaturity)

	var hash, errHash = tmdb.FetchBlockShaByHeight(matureHeight)
	if errHash != nil {
		return nil, errHash
	}

	var block, errBlock = tmdb.FetchBlockBySha(hash)
	if errBlock != nil {
		return nil, errBlock
	}

	// Create a map of ticketHash --> ticket to fill out
	tickets := make(SStxMemMap)

	stakeTransactions := block.STransactions()

	// Fill out the ticket data as best we can initially
	for _, staketx := range stakeTransactions {
		if is, _ := IsSStx(staketx); is {
			// Calculate the prefix for pre-sort.
			sstxHash := *staketx.Sha()

			ticket := new(TicketData)
			ticket.SStxHash = sstxHash
			ticket.Prefix = uint8(sstxHash[0])
			ticket.SpendHash = chainhash.Hash{} // Unspent at this point
			ticket.BlockHeight = height
			ticket.Missed = false
			ticket.Expired = false

			tickets[ticket.SStxHash] = ticket
		}
	}

	return tickets, nil
}

// pushMatureTicketsAtHeight matures tickets from TICKET_MATURITY blocks ago by
// looking them up in the database.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) pushMatureTicketsAtHeight(height int64) (SStxMemMap, error) {
	tempTickets := make(SStxMemMap)

	tickets, err := tmdb.getNewTicketsFromHeight(height)
	if err != nil {
		return nil, err
	}

	for _, ticket := range tickets {
		tempTickets[ticket.SStxHash] = ticket

		errPush := tmdb.pushLiveTicket(ticket)
		if errPush != nil {
			return nil, errPush
		}
	}

	return tempTickets, nil
}

// insertBlock is the internal function which implements the public
// InsertBlock.  See the comment for InsertBlock for more details.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) insertBlock(block *dcrutil.Block,
	parent *dcrutil.Block) (SStxMemMap, SStxMemMap, SStxMemMap, error) {

	height := block.Height()
	if height < tmdb.StakeEnabledHeight {
		return nil, nil, nil, nil
	}

	// Sanity check: Does the number of tickets in ticketMap equal the number
	// of tickets indicated in the header?
	poolSizeBlock := int(block.MsgBlock().Header.PoolSize)
	poolSize := 0
	for i := 0; i < BucketsSize; i++ {
		poolSize += len(tmdb.maps.ticketMap[i])
	}
	if poolSize != poolSizeBlock {
		return nil, nil, nil, fmt.Errorf("ticketpoolsize in block %v not "+
			"equal to the calculated ticketpoolsize, indicating database "+
			"corruption (got %v, want %v)",
			block.Sha(),
			poolSizeBlock,
			poolSize)
	}

	// Create the block in the spentTicketMap.
	tmdb.maybeInsertBlock(block.Height())

	// Iterate through all the SSGen (vote) tx in the block and add them to
	// a map of tickets that were actually used. The rest of the tickets in
	// the buckets were then considered missed --> missedTicketMap.
	// Note that it doesn't really matter what value you set usedTickets to,
	// it's just a map of tickets that were actually used in the block. It
	// would probably be more efficient to use an array.
	usedTickets := make(map[chainhash.Hash]struct{})
	spendingHashes := make(map[chainhash.Hash]chainhash.Hash)
	revocations := make(map[chainhash.Hash]struct{})

	for _, staketx := range block.STransactions() {
		if is, _ := IsSSGen(staketx); is {
			msgTx := staketx.MsgTx()
			sstxIn := msgTx.TxIn[1] // sstx input
			sstxHash := sstxIn.PreviousOutPoint.Hash

			usedTickets[sstxHash] = struct{}{}
			spendingHashes[sstxHash] = *staketx.Sha()
		}

		if is, _ := IsSSRtx(staketx); is {
			msgTx := staketx.MsgTx()
			sstxIn := msgTx.TxIn[0] // sstx input
			sstxHash := sstxIn.PreviousOutPoint.Hash

			revocations[sstxHash] = struct{}{}
		}
	}

	// Spend or miss all the necessary tickets and do some sanity checks.
	spentAndMissedTickets, err := tmdb.spendTickets(parent,
		usedTickets,
		spendingHashes)
	if err != nil {
		return nil, nil, nil, err
	}

	// Expire all old tickets, and stick them into the spent and missed ticket
	// map too.
	expiredTickets, err := tmdb.expireTickets(height)
	if err != nil {
		return nil, nil, nil, err
	}
	if len(expiredTickets) > 0 && len(spentAndMissedTickets) == 0 {
		return nil, nil, nil, fmt.Errorf("tried to expire tickets before " +
			"stake validation height! TicketExpiry may be too small")
	}
	if len(expiredTickets) > 0 {
		for hash, ticket := range expiredTickets {
			spentAndMissedTickets[hash] = ticket
		}
	}

	revokedTickets, err := tmdb.revokeTickets(revocations)
	if err != nil {
		return nil, nil, nil, err
	}

	newTickets, err := tmdb.pushMatureTicketsAtHeight(block.Height())
	if err != nil {
		return nil, nil, nil, err
	}

	log.Debugf("Connected block %v (height %v) to the ticket database",
		block.Sha(), block.Height())

	return cloneSStxMemMap(spentAndMissedTickets), cloneSStxMemMap(newTickets),
		cloneSStxMemMap(revokedTickets), nil
}

// InsertBlock is the main work horse for inserting blocks in the TicketDB.
// Warning: I think this and the remove block functions pass pointers
// back to TicketDB data. If you call this function and use the SStxMemMaps
// it returns you need to make sure you don't modify their contents
// externally. In the future consider passing by value if this causes
// a consensus failure somehow.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) InsertBlock(block, parent *dcrutil.Block) (SStxMemMap,
	SStxMemMap, SStxMemMap, error) {

	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	return tmdb.insertBlock(block, parent)
}

// unpushMatureTicketsAtHeight unmatures tickets from TICKET_MATURITY blocks ago by
// looking them up in the database.
//
// Safe for concurrent access (does not use TicketDB maps directly).
func (tmdb *TicketDB) unpushMatureTicketsAtHeight(height int64) (SStxMemMap,
	error) {

	tempTickets := make(SStxMemMap)

	tickets, err := tmdb.getNewTicketsFromHeight(height)
	if err != nil {
		return nil, err
	}

	for _, ticket := range tickets {
		tempTickets[ticket.SStxHash] = ticket

		errUnpush := tmdb.removeLiveTicket(ticket)
		if errUnpush != nil {
			return nil, errUnpush
		}
	}

	return tempTickets, nil
}

// removeBlockToHeight is the main work horse for removing blocks from the
// TicketDB. This function will remove all blocks until reaching the block at
// height.
//
// This function is unsafe for concurrent access.
func (tmdb *TicketDB) removeBlockToHeight(height int64) (map[int64]SStxMemMap,
	map[int64]SStxMemMap, map[int64]SStxMemMap, error) {
	if height < tmdb.StakeEnabledHeight {
		return nil, nil, nil, fmt.Errorf("TicketDB Error: tried to remove "+
			"blocks to before minimum maturation height (got %v, min %v)!",
			height, tmdb.StakeEnabledHeight)
	}

	// Discover the current height
	topHeight := tmdb.getTopBlock()

	if height >= topHeight {
		return nil, nil, nil, fmt.Errorf("TicketDB @ RemoveBlock: Tried to " +
			"remove blocks that are beyond or at the top block!")
	}

	// Create pseudo-DB maps of all the changes we're making
	unmaturedTicketMap := make(map[int64]SStxMemMap)
	unspentTicketMap := make(map[int64]SStxMemMap)
	unrevokedTicketMap := make(map[int64]SStxMemMap)

	// Iterates down from the top block, removing all changes that were made to
	// the stake db at that block, until it reaches the height specified.
	for curHeight := topHeight; curHeight > height; curHeight-- {
		unmaturedTicketMap[curHeight] = make(SStxMemMap)
		unspentTicketMap[curHeight] = make(SStxMemMap)
		unrevokedTicketMap[curHeight] = make(SStxMemMap)

		unmaturedTickets, err := tmdb.unpushMatureTicketsAtHeight(curHeight)
		if err != nil {
			return nil, nil, nil, err
		}
		unmaturedTicketMap[curHeight] = cloneSStxMemMap(unmaturedTickets)

		unrevokedTickets, err := tmdb.unrevokeTickets(curHeight)
		if err != nil {
			return nil, nil, nil, err
		}
		unrevokedTicketMap[curHeight] = cloneSStxMemMap(unrevokedTickets)

		// Note that unspendTickets below also deletes the block from the
		// spentTicketMap, and so updates our top block.
		unspentTickets, err := tmdb.unspendTickets(curHeight)
		if err != nil {
			return nil, nil, nil, err
		}
		unspentTicketMap[curHeight] = cloneSStxMemMap(unspentTickets)
	}

	return unmaturedTicketMap, unrevokedTicketMap, unspentTicketMap, nil
}

// RemoveBlockToHeight is the exported version of removeBlockToHeight.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) RemoveBlockToHeight(height int64) (map[int64]SStxMemMap,
	map[int64]SStxMemMap, map[int64]SStxMemMap, error) {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	return tmdb.removeBlockToHeight(height)
}

// rescanTicketDB is the internal function which implements the public
// RescanTicketDB.  See the comment for RescanTicketDB for more details.
//
// This function MUST be called with the tmdb lock held (for writes).
func (tmdb *TicketDB) rescanTicketDB() error {
	// Get the latest block height from the database.
	_, height, err := tmdb.NewestSha()
	if err != nil {
		return err
	}

	if height < tmdb.StakeEnabledHeight {
		return nil
	}

	// Find out what block the TMDB was previously synced to.
	curTmdbHeight := tmdb.getTopBlock()

	// If we're synced to some old height that the database
	// already has, begin resyncing from this height instead
	// of the genesis block.
	if curTmdbHeight > 0 {
		// The spent ticket map doesn't store the hashes of the
		// blocks, so instead review the listed votes. The votes
		// are dependent on the block hash of the previous block,
		// so if they line up we know the previous block before
		// curTmdbHeight is correct.
		spentBl, _ := tmdb.maps.spentTicketMap[curTmdbHeight]
		spendHashes := make(map[chainhash.Hash]struct{})
		for _, td := range spentBl {
			spendHashes[td.SpendHash] = struct{}{}
		}

		h, err := tmdb.FetchBlockShaByHeight(curTmdbHeight)
		if err != nil {
			return err
		}

		blCur, err := tmdb.FetchBlockBySha(h)
		if err != nil {
			return err
		}

		failedToFindBlock := false
		for _, stx := range blCur.STransactions() {
			if is, _ := IsSSGen(stx); is {
				voteHash := stx.Sha()
				_, ok := spendHashes[*voteHash]
				if !ok {
					failedToFindBlock = true
				}
			}
		}

		// Handle empty chain exception.
		if curTmdbHeight <= tmdb.chainParams.StakeEnabledHeight {
			failedToFindBlock = true
		}

		// We found a matching block in the database at
		// curTmdbHeight-1, so sync to it.
		if !failedToFindBlock {
			log.Infof("Found a previously good height %v in the old "+
				"stake database, attempting to sync to tip height %v "+
				"from it", curTmdbHeight, height)

			// Remove the top block if we can.
			_, _, _, err = tmdb.removeBlockToHeight(curTmdbHeight - 1)
			if err != nil {
				return err
			}

			// Reinsert the top block and sync to the best chain
			// height.
			for i := curTmdbHeight; i <= height; i++ {
				h, err := tmdb.FetchBlockShaByHeight(i)
				if err != nil {
					return err
				}

				bl, err := tmdb.FetchBlockBySha(h)
				if err != nil {
					return err
				}

				pa, err := tmdb.FetchBlockBySha(&bl.MsgBlock().Header.PrevBlock)
				if err != nil {
					return err
				}

				_, _, _, err = tmdb.insertBlock(bl, pa)
				if err != nil {
					return err
				}
			}

			return nil
		}
	}

	// We aren't able to resync from a previous height, so do the
	// entire thing from the genesis block.
	var freshTms TicketMaps
	freshTms.ticketMap = make([]SStxMemMap, BucketsSize, BucketsSize)
	freshTms.spentTicketMap = make(map[int64]SStxMemMap)
	freshTms.missedTicketMap = make(SStxMemMap)
	freshTms.revokedTicketMap = make(SStxMemMap)

	tmdb.maps = freshTms

	// Fill in live ticket buckets
	for i := 0; i < BucketsSize; i++ {
		tmdb.maps.ticketMap[i] = make(SStxMemMap)
	}

	// Tickets don't exist before StakeEnabledHeight
	for curHeight := tmdb.StakeEnabledHeight; curHeight <= height; curHeight++ {
		// Go through the winners and votes for each block and use those to spend
		// tickets in the ticket db.
		hash, errHash := tmdb.FetchBlockShaByHeight(curHeight)
		if errHash != nil {
			return errHash
		}

		block, errBlock := tmdb.FetchBlockBySha(hash)
		if errBlock != nil {
			return errBlock
		}

		parent, errBlock :=
			tmdb.FetchBlockBySha(&block.MsgBlock().Header.PrevBlock)
		if errBlock != nil {
			return errBlock
		}

		_, _, _, err = tmdb.insertBlock(block, parent)
		if err != nil {
			return err
		}
	}
	return nil
}

// RescanTicketDB rescans and regenerates both ticket memory maps starting from
// the genesis block and extending to the current block.  This uses a lot of
// memory because it doesn't kill the old buckets.
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) RescanTicketDB() error {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	return tmdb.rescanTicketDB()
}

// GetTicketHashesForMissed gets all the currently missed tickets, copies
// their hashes, and returns them.
// TODO: Is a pointer to a pointer for a slice really necessary?
//
// This function is safe for concurrent access.
func (tmdb *TicketDB) GetTicketHashesForMissed() []chainhash.Hash {
	tmdb.mtx.Lock()
	defer tmdb.mtx.Unlock()

	missedTickets := make([]chainhash.Hash, len(tmdb.maps.missedTicketMap))

	itr := 0
	for hash := range tmdb.maps.missedTicketMap {
		missedTickets[itr] = hash
		itr++
	}

	return missedTickets
}
