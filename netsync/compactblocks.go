// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package netsync

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/btcsuite/btcd/blockchain"
	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

var (
	// ErrCompactBlockIncomplete signals that a compact block cannot yet be
	// reconstructed because transactions are still missing.
	ErrCompactBlockIncomplete = errors.New("compact block missing transactions")

	// ErrCompactBlockBadMerkleRoot signals that the reconstructed block's
	// merkle root does not match the compact block header.
	ErrCompactBlockBadMerkleRoot = errors.New("compact block merkle root mismatch")

	// ErrNilBlock signals that the provided block is nil.
	ErrNilBlock = errors.New("nil block")

	// ErrNoTransactions signals that a compact block requires transactions.
	ErrNoTransactions = errors.New("compact block requires transactions")

	// ErrNilCompactBlock signals that the provided compact block message is nil.
	ErrNilCompactBlock = errors.New("nil compact block")

	// ErrEmptyCompactBlock signals that a compact block has no transactions.
	ErrEmptyCompactBlock = errors.New("compact block has no transactions")

	// ErrCompactBlockTooManyTransactions signals that a compact block cannot
	// be represented within Bitcoin Core's 16-bit transaction index limit.
	ErrCompactBlockTooManyTransactions = errors.New(
		"compact block has too many transactions",
	)

	// ErrNilPrefilledTx signals that a prefilled transaction is nil.
	ErrNilPrefilledTx = errors.New("nil prefilled transaction")

	// ErrNilBlockTxnMsg signals that the provided blocktxn message is nil.
	ErrNilBlockTxnMsg = errors.New("nil blocktxn message")

	// ErrNilBlockTxnTx signals that a transaction within a blocktxn message
	// is nil.
	ErrNilBlockTxnTx = errors.New("nil blocktxn transaction")

	// ErrNilTx signals that the provided transaction is nil.
	ErrNilTx = errors.New("nil transaction")

	// ErrShortIDCountTooLow signals that the compact block short ID count is
	// lower than expected.
	ErrShortIDCountTooLow = errors.New("compact block short id count too low")

	// ErrShortIDCountTooHigh signals that the compact block short ID count is
	// higher than expected.
	ErrShortIDCountTooHigh = errors.New("compact block short id count too high")
)

// CompactBlockReconstructor tracks the state needed to reconstruct a compact
// block from a cmpctblock message using mempool transactions and an optional
// blocktxn response for any missing ones.
type CompactBlockReconstructor struct {
	header         wire.BlockHeader
	blockHash      chainhash.Hash
	missingIndexes []uint32
	txns           []*btcutil.Tx
}

// BuildCompactBlock builds a BIP 152 cmpctblock message from a full block.  If
// no prefilled indexes are provided, the coinbase transaction is prefilled.
func BuildCompactBlock(block *btcutil.Block, nonce uint64,
	prefilledIndexes []uint32, witness bool) (*wire.MsgCmpctBlock, error) {

	if block == nil {
		return nil, ErrNilBlock
	}

	txns := block.Transactions()
	if len(txns) == 0 {
		return nil, ErrNoTransactions
	}

	if len(txns) > math.MaxUint16 {
		return nil, ErrCompactBlockTooManyTransactions
	}


	if len(prefilledIndexes) == 0 {
		prefilledIndexes = []uint32{0}
	}

	prefilled, err := normalizePrefilledIndexes(prefilledIndexes, len(txns))
	if err != nil {
		return nil, err
	}

	header := &block.MsgBlock().Header
	msg := wire.NewMsgCmpctBlock(header, nonce)

	prefilledSet := make(map[uint32]struct{}, len(prefilled))
	for _, index := range prefilled {
		prefilledSet[index] = struct{}{}
	}

	for i, tx := range txns {
		index := uint32(i)
		if _, ok := prefilledSet[index]; ok {
			err := msg.AddPrefilledTxn(&wire.PrefilledTxn{
				Index: index,
				Tx:    tx.MsgTx(),
			})
			if err != nil {
				return nil, err
			}
			continue
		}

		shortID, err := CompactBlockShortID(header, nonce, tx, witness)
		if err != nil {
			return nil, err
		}
		if err := msg.AddShortID(shortID); err != nil {
			return nil, err
		}
	}

	return msg, nil
}

func normalizePrefilledIndexes(indexes []uint32, totalTxns int) ([]uint32, error) {

	normalized := make([]uint32, len(indexes))
	copy(normalized, indexes)
	sort.Slice(normalized, func(i, j int) bool {
		return normalized[i] < normalized[j]
	})

	for i, index := range normalized {
		if int(index) >= totalTxns {
			return nil, fmt.Errorf("prefilled index %d out of range",
				index)
		}
		if i > 0 && normalized[i-1] == index {
			return nil, fmt.Errorf("duplicate prefilled index %d",
				index)
		}
	}

	return normalized, nil
}

// NewCompactBlockReconstructor attempts to reconstruct the provided compact
// block with the supplied mempool transactions.
func NewCompactBlockReconstructor(msg *wire.MsgCmpctBlock,
	mempoolTxns []*btcutil.Tx,
	witness bool) (*CompactBlockReconstructor, error) {

	if msg == nil {
		return nil, ErrNilCompactBlock
	}

	totalTxns := len(msg.ShortIDs) + len(msg.PrefilledTxns)
	if totalTxns == 0 {
		return nil, ErrEmptyCompactBlock
	}
	if totalTxns > math.MaxUint16 {
		return nil, ErrCompactBlockTooManyTransactions
	}

	recon := &CompactBlockReconstructor{
		header:    msg.Header,
		blockHash: msg.Header.BlockHash(),
		txns:      make([]*btcutil.Tx, totalTxns),
	}

	prefilled := make(map[uint32]struct{}, len(msg.PrefilledTxns))
	for i, txn := range msg.PrefilledTxns {
		if txn == nil || txn.Tx == nil {
			return nil, ErrNilPrefilledTx
		}

		maxIndex := uint64(len(msg.ShortIDs) + i)
		if uint64(txn.Index) > maxIndex {
			return nil, fmt.Errorf("prefilled index %d out of range",
				txn.Index)
		}
		if _, ok := prefilled[txn.Index]; ok {
			return nil, fmt.Errorf("duplicate prefilled index %d",
				txn.Index)
		}

		prefilled[txn.Index] = struct{}{}
		recon.txns[txn.Index] = btcutil.NewTx(txn.Tx)
	}

	shortIDIndex, err := compactBlockShortIDIndexes(msg, totalTxns, prefilled)
	if err != nil {
		return nil, err
	}

	txIndex, err := buildShortIDTxIndex(
		&msg.Header, msg.Nonce, mempoolTxns, witness,
	)
	if err != nil {
		return nil, err
	}

	for shortID, blockIndex := range shortIDIndex {
		tx, ok := txIndex[shortID]
		if ok {
			recon.txns[blockIndex] = tx
		}
	}

	recon.refreshMissing()
	return recon, nil
}

// MissingIndexes returns the absolute transaction indexes still needed to
// reconstruct the block.
func (r *CompactBlockReconstructor) MissingIndexes() []uint32 {
	missing := make([]uint32, len(r.missingIndexes))
	copy(missing, r.missingIndexes)
	return missing
}

// IsComplete returns whether all transactions needed to reconstruct the block
// are available.
func (r *CompactBlockReconstructor) IsComplete() bool {
	return len(r.missingIndexes) == 0
}

// GetBlockTxnRequest returns a getblocktxn message for the currently missing
// transaction indexes.
func (r *CompactBlockReconstructor) GetBlockTxnRequest() *wire.MsgGetBlockTxn {
	msg := wire.NewMsgGetBlockTxn(&r.blockHash)
	for _, index := range r.missingIndexes {
		_ = msg.AddIndex(index)
	}

	return msg
}

// ProvideBlockTxn fills the missing transactions from a blocktxn message and
// returns the reconstructed block when complete.
func (r *CompactBlockReconstructor) ProvideBlockTxn(
	msg *wire.MsgBlockTxn) (*btcutil.Block, error) {

	if msg == nil {
		return nil, ErrNilBlockTxnMsg
	}
	if !msg.BlockHash.IsEqual(&r.blockHash) {
		return nil, fmt.Errorf("blocktxn hash %v does not match %v",
			msg.BlockHash, r.blockHash)
	}
	if len(msg.Transactions) != len(r.missingIndexes) {
		return nil, fmt.Errorf("blocktxn transaction count %d does not "+
			"match missing count %d", len(msg.Transactions),
			len(r.missingIndexes))
	}

	for i, msgTx := range msg.Transactions {
		if msgTx == nil {
			return nil, ErrNilBlockTxnTx
		}

		r.txns[r.missingIndexes[i]] = btcutil.NewTx(msgTx)
	}

	r.refreshMissing()
	return r.Block()
}

// Block returns the reconstructed block when complete.
func (r *CompactBlockReconstructor) Block() (*btcutil.Block, error) {
	if !r.IsComplete() {
		return nil, ErrCompactBlockIncomplete
	}

	msgBlock := &wire.MsgBlock{
		Header:       r.header,
		Transactions: make([]*wire.MsgTx, len(r.txns)),
	}
	for i, tx := range r.txns {
		msgBlock.Transactions[i] = tx.MsgTx()
	}

	blockTxns := make([]*btcutil.Tx, len(r.txns))
	copy(blockTxns, r.txns)
	merkleRoot := blockchain.CalcMerkleRoot(blockTxns, false)
	if !merkleRoot.IsEqual(&r.header.MerkleRoot) {
		return nil, ErrCompactBlockBadMerkleRoot
	}

	return btcutil.NewBlock(msgBlock), nil
}

func (r *CompactBlockReconstructor) refreshMissing() {
	r.missingIndexes = r.missingIndexes[:0]
	for i, tx := range r.txns {
		if tx == nil {
			r.missingIndexes = append(r.missingIndexes, uint32(i))
		}
	}
}
