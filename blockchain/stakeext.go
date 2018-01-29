// Copyright (c) 2013-2014 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/database"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/dcrd/txscript"
)

// NextLotteryData returns the next tickets eligible for spending as SSGen
// on the top block.  It also returns the ticket pool size and the PRNG
// state checksum.
//
// This function is safe for concurrent access.
func (b *BlockChain) NextLotteryData() ([]chainhash.Hash, int, [6]byte, error) {
	b.chainLock.RLock()
	defer b.chainLock.RUnlock()

	return b.bestNode.stakeNode.Winners(), b.bestNode.stakeNode.PoolSize(),
		b.bestNode.stakeNode.FinalState(), nil
}

// lotteryDataForNode is a helper function that returns winning tickets
// along with the ticket pool size and PRNG checksum for a given node.
//
// This function is NOT safe for concurrent access and MUST be called
// with the chainLock held for writes.
func (b *BlockChain) lotteryDataForNode(node *blockNode) ([]chainhash.Hash, int, [6]byte, error) {
	if node.height < b.chainParams.StakeEnabledHeight {
		return []chainhash.Hash{}, 0, [6]byte{}, nil
	}
	stakeNode, err := b.fetchStakeNode(node)
	if err != nil {
		return []chainhash.Hash{}, 0, [6]byte{}, err
	}

	return stakeNode.Winners(), b.bestNode.stakeNode.PoolSize(),
		b.bestNode.stakeNode.FinalState(), nil
}

// lotteryDataForBlock takes a node block hash and returns the next tickets
// eligible for voting, the number of tickets in the ticket pool, and the
// final state of the PRNG.
//
// This function is NOT safe for concurrent access and must have the chainLock
// held for write access.
func (b *BlockChain) lotteryDataForBlock(hash *chainhash.Hash) ([]chainhash.Hash, int, [6]byte, error) {
	var node *blockNode
	if n := b.index.LookupNode(hash); n != nil {
		node = n
	} else {
		var err error
		node, err = b.findNode(hash, maxSearchDepth)
		if err != nil {
			return nil, 0, [6]byte{}, err
		}
	}

	winningTickets, poolSize, finalState, err := b.lotteryDataForNode(node)
	if err != nil {
		return nil, 0, [6]byte{}, err
	}

	return winningTickets, poolSize, finalState, nil
}

// LotteryDataForBlock returns lottery data for a given block in the block
// chain, including side chain blocks.
//
// It is safe for concurrent access.
// TODO An optimization can be added that only calls the read lock if the
//   block is not minMemoryStakeNodes blocks before the current best node.
//   This is because all the data for these nodes can be assumed to be
//   in memory.
func (b *BlockChain) LotteryDataForBlock(hash *chainhash.Hash) ([]chainhash.Hash, int, [6]byte, error) {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()

	return b.lotteryDataForBlock(hash)
}

// LiveTickets returns all currently live tickets from the stake database.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) LiveTickets() ([]chainhash.Hash, error) {
	b.chainLock.RLock()
	sn := b.bestNode.stakeNode
	b.chainLock.RUnlock()

	return sn.LiveTickets(), nil
}

// MissedTickets returns all currently missed tickets from the stake database.
//
// This function is NOT safe for concurrent access.
func (b *BlockChain) MissedTickets() ([]chainhash.Hash, error) {
	b.chainLock.RLock()
	sn := b.bestNode.stakeNode
	b.chainLock.RUnlock()

	return sn.MissedTickets(), nil
}

// TicketsWithAddress returns a slice of ticket hashes that are currently live
// corresponding to the given address.
//
// This function is safe for concurrent access.
func (b *BlockChain) TicketsWithAddress(address dcrutil.Address) ([]chainhash.Hash, error) {
	b.chainLock.RLock()
	sn := b.bestNode.stakeNode
	b.chainLock.RUnlock()

	tickets := sn.LiveTickets()

	var ticketsWithAddr []chainhash.Hash
	err := b.db.View(func(dbTx database.Tx) error {
		for _, hash := range tickets {
			utxo, err := dbFetchUtxoEntry(dbTx, &hash)
			if err != nil {
				return err
			}

			_, addrs, _, err :=
				txscript.ExtractPkScriptAddrs(txscript.DefaultScriptVersion,
					utxo.PkScriptByIndex(0), b.chainParams)
			if err != nil {
				return err
			}
			if addrs[0].EncodeAddress() == address.EncodeAddress() {
				ticketsWithAddr = append(ticketsWithAddr, hash)
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return ticketsWithAddr, nil
}

// CheckLiveTicket returns whether or not a ticket exists in the live ticket
// treap of the best node.
//
// This function is safe for concurrent access.
func (b *BlockChain) CheckLiveTicket(hash chainhash.Hash) bool {
	b.chainLock.RLock()
	sn := b.bestNode.stakeNode
	b.chainLock.RUnlock()

	return sn.ExistsLiveTicket(hash)
}

// CheckLiveTickets returns whether or not a slice of tickets exist in the live
// ticket treap of the best node.
//
// This function is safe for concurrent access.
func (b *BlockChain) CheckLiveTickets(hashes []chainhash.Hash) []bool {
	b.chainLock.RLock()
	sn := b.bestNode.stakeNode
	b.chainLock.RUnlock()

	existsSlice := make([]bool, len(hashes))
	for i := range hashes {
		existsSlice[i] = sn.ExistsLiveTicket(hashes[i])
	}

	return existsSlice
}

// CheckMissedTickets returns a slice of bools representing whether each ticket
// hash has been missed in the live ticket treap of the best node.
//
// This function is safe for concurrent access.
func (b *BlockChain) CheckMissedTickets(hashes []chainhash.Hash) []bool {
	b.chainLock.RLock()
	sn := b.bestNode.stakeNode
	b.chainLock.RUnlock()

	existsSlice := make([]bool, len(hashes))
	for i := range hashes {
		existsSlice[i] = sn.ExistsMissedTicket(hashes[i])
	}

	return existsSlice
}

// CheckExpiredTicket returns whether or not a ticket was ever expired.
//
// This function is safe for concurrent access.
func (b *BlockChain) CheckExpiredTicket(hash chainhash.Hash) bool {
	b.chainLock.RLock()
	sn := b.bestNode.stakeNode
	b.chainLock.RUnlock()

	return sn.ExistsExpiredTicket(hash)
}

// CheckExpiredTickets returns whether or not a ticket in a slice of
// tickets was ever expired.
//
// This function is safe for concurrent access.
func (b *BlockChain) CheckExpiredTickets(hashes []chainhash.Hash) []bool {
	b.chainLock.RLock()
	sn := b.bestNode.stakeNode
	b.chainLock.RUnlock()

	existsSlice := make([]bool, len(hashes))
	for i := range hashes {
		existsSlice[i] = sn.ExistsExpiredTicket(hashes[i])
	}

	return existsSlice
}

// TicketPoolValue returns the current value of all the locked funds in the
// ticket pool.
//
// This function is safe for concurrent access.  All live tickets are at least
// 256 blocks deep on mainnet, so the UTXO set should generally always have
// the asked for transactions.
func (b *BlockChain) TicketPoolValue() (dcrutil.Amount, error) {
	b.chainLock.RLock()
	sn := b.bestNode.stakeNode
	b.chainLock.RUnlock()

	var amt int64
	err := b.db.View(func(dbTx database.Tx) error {
		for _, hash := range sn.LiveTickets() {
			utxo, err := dbFetchUtxoEntry(dbTx, &hash)
			if err != nil {
				return err
			}

			amt += utxo.sparseOutputs[0].amount
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	return dcrutil.Amount(amt), nil
}
