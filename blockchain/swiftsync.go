// Copyright (c) 2024 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/wire"
)

// isSwiftSyncActive returns true if swift sync should be used for this height.
// Swift sync is active when:
// 1. Swift sync is enabled (checkpoints are enabled and swift sync data exists)
// 2. The block height is within the swift sync range
func (b *BlockChain) isSwiftSyncActive(height int32) bool {
	return b.swiftSyncEnabled &&
		b.swiftSync != nil &&
		height >= 1 &&
		height <= b.swiftSync.Height
}

// getSwiftSyncBitmapBit returns true if the bit at index is set (output is spent).
func (b *BlockChain) getSwiftSyncBitmapBit(index uint64) bool {
	byteIdx := index / 8
	bitPos := index % 8
	if byteIdx >= uint64(len(b.swiftSync.Bitmap)) {
		return false
	}
	return (b.swiftSync.Bitmap[byteIdx] & (1 << bitPos)) != 0
}

// swiftSyncConnectTransactions adds unspent outputs to the UTXO cache using
// the swift sync bitmap. Unlike normal connectTransactions, this doesn't mark
// inputs as spent since we're bootstrapping the UTXO set directly from the
// bitmap.
//
// For each transaction output, we check the corresponding bit in the bitmap:
// - If bit is 0, the output is unspent and we add it to the UTXO cache
// - If bit is 1, the output was spent and we skip it
//
// The bit index is tracked across blocks and must be persisted for resumability.
func (s *utxoCache) swiftSyncConnectTransactions(b *BlockChain, block *btcutil.Block) error {
	for _, tx := range block.Transactions() {
		isCoinBase := IsCoinBase(tx)
		prevOut := wire.OutPoint{Hash: *tx.Hash()}

		for txOutIdx, txOut := range tx.MsgTx().TxOut {
			// Check bitmap - bit 0 means unspent, bit 1 means spent
			isSpent := b.getSwiftSyncBitmapBit(b.swiftSyncBitIdx)
			b.swiftSyncBitIdx++

			// Skip spent outputs
			if isSpent {
				continue
			}

			// Add unspent output to cache
			// addTxOut handles skipping unspendable outputs internally
			prevOut.Index = uint32(txOutIdx)
			err := s.addTxOut(prevOut, txOut, isCoinBase, block.Height())
			if err != nil {
				return err
			}
		}
	}
	return nil
}
