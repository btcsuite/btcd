// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2016 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"sort"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	database "github.com/decred/dcrd/database2"
)

// GetNextWinningTickets returns the next tickets eligible for spending as SSGen
// on the top block. It also returns the ticket pool size.
// This function is NOT safe for concurrent access.
func (b *BlockChain) GetNextWinningTickets() ([]chainhash.Hash, int, [6]byte,
	error) {
	winningTickets, poolSize, finalState, _, err :=
		b.getWinningTicketsWithStore(b.bestNode)
	if err != nil {
		return nil, 0, [6]byte{}, err
	}

	return winningTickets, poolSize, finalState, nil
}

// getWinningTicketsWithStore is a helper function that returns winning tickets
// along with the ticket pool size and transaction store for the given node.
// Note that this function evaluates the lottery data predominantly for mining
// purposes; that is, it retrieves the lottery data which needs to go into
// the next block when mining on top of this block.
// This function is NOT safe for concurrent access.
func (b *BlockChain) getWinningTicketsWithStore(node *blockNode) ([]chainhash.Hash,
	int, [6]byte, TicketStore, error) {
	if node.height < b.chainParams.StakeEnabledHeight {
		return []chainhash.Hash{}, 0, [6]byte{}, nil, nil
	}

	evalLotteryWinners := false
	if node.height >= b.chainParams.StakeValidationHeight-1 {
		evalLotteryWinners = true
	}

	block, err := b.getBlockFromHash(node.hash)
	if err != nil {
		return nil, 0, [6]byte{}, nil, err
	}

	headerB, err := node.header.Bytes()
	if err != nil {
		return nil, 0, [6]byte{}, nil, err
	}

	ticketStore, err := b.fetchTicketStore(node)
	if err != nil {
		return nil, 0, [6]byte{}, nil,
			fmt.Errorf("Failed to generate ticket store for node %v; "+
				"error given: %v", node.hash, err)
	}

	if ticketStore != nil {
		view := NewUtxoViewpoint()
		view.SetBestHash(node.hash)
		view.SetStakeViewpoint(ViewpointPrevValidInitial)
		parent, err := b.getBlockFromHash(&node.header.PrevBlock)
		if err != nil {
			return nil, 0, [6]byte{}, nil, err
		}
		err = view.fetchInputUtxos(b.db, block, parent)
		if err != nil {
			return nil, 0, [6]byte{}, nil, err
		}
	}

	// Sort the entire list of tickets lexicographically by sorting
	// each bucket and then appending it to the list.
	tpdBucketMap := make(map[uint8][]*TicketPatchData)
	for _, tpd := range ticketStore {
		// Bucket does not exist.
		if _, ok := tpdBucketMap[tpd.td.Prefix]; !ok {
			tpdBucketMap[tpd.td.Prefix] = make([]*TicketPatchData, 1)
			tpdBucketMap[tpd.td.Prefix][0] = tpd
		} else {
			// Bucket exists.
			data := tpdBucketMap[tpd.td.Prefix]
			tpdBucketMap[tpd.td.Prefix] = append(data, tpd)
		}
	}
	totalTickets := 0
	var sortedSlice []*stake.TicketData
	for i := 0; i < stake.BucketsSize; i++ {
		ltb, err := b.GenerateLiveTicketBucket(ticketStore, tpdBucketMap,
			uint8(i))
		if err != nil {
			h := node.hash
			str := fmt.Sprintf("Failed to generate a live ticket bucket "+
				"to evaluate the lottery data for node %v, height %v! Error "+
				"given: %v",
				h,
				node.height,
				err.Error())
			return nil, 0, [6]byte{}, nil, fmt.Errorf(str)
		}
		mapLen := len(ltb)

		tempTdSlice := stake.NewTicketDataSlice(mapLen)
		itr := 0 // Iterator
		for _, td := range ltb {
			tempTdSlice[itr] = td
			itr++
			totalTickets++
		}
		sort.Sort(tempTdSlice)
		sortedSlice = append(sortedSlice, tempTdSlice...)
	}

	// Use the parent block's header to seed a PRNG that picks the
	// lottery winners.
	var winningTickets []chainhash.Hash
	var finalState [6]byte
	stateBuffer := make([]byte, 0,
		(b.chainParams.TicketsPerBlock+1)*chainhash.HashSize)
	if evalLotteryWinners {
		ticketsPerBlock := int(b.chainParams.TicketsPerBlock)
		prng := stake.NewHash256PRNG(headerB)
		ts, err := stake.FindTicketIdxs(int64(totalTickets), ticketsPerBlock, prng)
		if err != nil {
			return nil, 0, [6]byte{}, nil, err
		}
		for _, idx := range ts {
			winningTickets = append(winningTickets, sortedSlice[idx].SStxHash)
			stateBuffer = append(stateBuffer, sortedSlice[idx].SStxHash[:]...)
		}

		lastHash := prng.StateHash()
		stateBuffer = append(stateBuffer, lastHash[:]...)
		copy(finalState[:], chainhash.HashFuncB(stateBuffer)[0:6])
	}

	return winningTickets, totalTickets, finalState, ticketStore, nil
}

// getWinningTicketsInclStore is a helper function for block validation that
// returns winning tickets along with the ticket pool size and transaction
// store for the given node.
// Note that this function is used for finding the lottery data when
// evaluating a block that builds on a tip, not for mining.
// This function is NOT safe for concurrent access.
func (b *BlockChain) getWinningTicketsInclStore(node *blockNode,
	ticketStore TicketStore) ([]chainhash.Hash, int, [6]byte, error) {
	if node.height < b.chainParams.StakeEnabledHeight {
		return []chainhash.Hash{}, 0, [6]byte{}, nil
	}

	evalLotteryWinners := false
	if node.height >= b.chainParams.StakeValidationHeight-1 {
		evalLotteryWinners = true
	}

	parentHeaderB, err := node.parent.header.Bytes()
	if err != nil {
		return nil, 0, [6]byte{}, err
	}

	// Sort the entire list of tickets lexicographically by sorting
	// each bucket and then appending it to the list.
	tpdBucketMap := make(map[uint8][]*TicketPatchData)
	for _, tpd := range ticketStore {
		// Bucket does not exist.
		if _, ok := tpdBucketMap[tpd.td.Prefix]; !ok {
			tpdBucketMap[tpd.td.Prefix] = make([]*TicketPatchData, 1)
			tpdBucketMap[tpd.td.Prefix][0] = tpd
		} else {
			// Bucket exists.
			data := tpdBucketMap[tpd.td.Prefix]
			tpdBucketMap[tpd.td.Prefix] = append(data, tpd)
		}
	}
	totalTickets := 0
	var sortedSlice []*stake.TicketData
	for i := 0; i < stake.BucketsSize; i++ {
		ltb, err := b.GenerateLiveTicketBucket(ticketStore, tpdBucketMap,
			uint8(i))
		if err != nil {
			h := node.hash
			str := fmt.Sprintf("Failed to generate a live ticket bucket "+
				"to evaluate the lottery data for node %v, height %v! Error "+
				"given: %v",
				h,
				node.height,
				err.Error())
			return nil, 0, [6]byte{}, fmt.Errorf(str)
		}
		mapLen := len(ltb)

		tempTdSlice := stake.NewTicketDataSlice(mapLen)
		itr := 0 // Iterator
		for _, td := range ltb {
			tempTdSlice[itr] = td
			itr++
			totalTickets++
		}
		sort.Sort(tempTdSlice)
		sortedSlice = append(sortedSlice, tempTdSlice...)
	}

	// Use the parent block's header to seed a PRNG that picks the
	// lottery winners.
	var winningTickets []chainhash.Hash
	var finalState [6]byte
	stateBuffer := make([]byte, 0,
		(b.chainParams.TicketsPerBlock+1)*chainhash.HashSize)
	if evalLotteryWinners {
		ticketsPerBlock := int(b.chainParams.TicketsPerBlock)
		prng := stake.NewHash256PRNG(parentHeaderB)
		ts, err := stake.FindTicketIdxs(int64(totalTickets), ticketsPerBlock, prng)
		if err != nil {
			return nil, 0, [6]byte{}, err
		}
		for _, idx := range ts {
			winningTickets = append(winningTickets, sortedSlice[idx].SStxHash)
			stateBuffer = append(stateBuffer, sortedSlice[idx].SStxHash[:]...)
		}

		lastHash := prng.StateHash()
		stateBuffer = append(stateBuffer, lastHash[:]...)
		copy(finalState[:], chainhash.HashFuncB(stateBuffer)[0:6])
	}

	return winningTickets, totalTickets, finalState, nil
}

// GetWinningTickets takes a node block hash and returns the next tickets
// eligible for spending as SSGen.
//
// This function is safe for concurrent access.
func (b *BlockChain) GetWinningTickets(nodeHash chainhash.Hash) ([]chainhash.Hash,
	int, [6]byte, error) {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()

	var node *blockNode
	if n, exists := b.index[nodeHash]; exists {
		node = n
	} else {
		node, _ = b.findNode(&nodeHash)
	}

	if node == nil {
		return nil, 0, [6]byte{}, fmt.Errorf("node doesn't exist")
	}

	winningTickets, poolSize, finalState, _, err :=
		b.getWinningTicketsWithStore(node)
	if err != nil {
		return nil, 0, [6]byte{}, err
	}

	return winningTickets, poolSize, finalState, nil
}

// GetMissedTickets returns a list of currently missed tickets.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) GetMissedTickets() []chainhash.Hash {
	missedTickets := b.tmdb.GetTicketHashesForMissed()

	return missedTickets
}

// DB passes the pointer to the database. It is only to be used by testing.
func (b *BlockChain) DB() database.DB {
	return b.db
}

// TMDB passes the pointer to the ticket database. It is only to be used by
// testing.
func (b *BlockChain) TMDB() *stake.TicketDB {
	return b.tmdb
}
