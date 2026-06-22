// Copyright (c) 2026 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"encoding/binary"
	"fmt"

	"github.com/btcsuite/btcd/btcutil/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// IsHintsfileUnspendableOutput reports whether the given output should be
// skipped from the spendable-index numbering used by the swift sync
// hintsfile format.  The output is considered unspendable for hintsfile
// purposes if any of the following hold:
//
//   - The pkScript is longer than txscript.MaxScriptSize (10,000) bytes.
//   - The pkScript begins with OP_RETURN.
//   - The output is one of the two BIP-30 unspendable mainnet coinbase outputs.
//
// Both the hintsfile producer and the consumer MUST apply this identical
// filter, or the spendable indices recorded in the hintsfile will not match
// the indices computed when walking the block.
func IsHintsfileUnspendableOutput(net wire.BitcoinNet, blockHash *chainhash.Hash, height int32, txIndex int, txOut *wire.TxOut) bool {
	if isBIP30UnspendableCoinbaseOutput(net, blockHash, height, txIndex) {
		return true
	}

	pkScript := txOut.PkScript
	return len(pkScript) > txscript.MaxScriptSize ||
		(len(pkScript) > 0 && pkScript[0] == txscript.OP_RETURN)
}

// isSwiftSyncActive returns true if swift sync should be used for this height.
// Swift sync is active when:
//  1. Swift sync is enabled (a hintsfile was provided)
//  2. The block height is within the swift sync range
func (b *BlockChain) isSwiftSyncActive(height int32) bool {
	return b.swiftSyncEnabled &&
		b.swiftSync != nil &&
		height >= 1 &&
		height <= b.swiftSync.Height
}

// swiftSyncConnectTransactions adds unspent outputs to the UTXO cache using the
// per-block hints list parsed from the swift sync hintsfile.  Unlike normal
// connectTransactions, this does not mark inputs as spent: we are bootstrapping
// the UTXO set directly from the hints.
//
// For each transaction output we maintain a per-block spendable index that is
// incremented for every output that is NOT unspendable per the hintsfile rules
// (see IsHintsfileUnspendableOutput).  If the current spendable index appears in
// the block's unspent hints list, the output is added to the UTXO cache.
//
// When aggregate is true it also updates the swift sync aggregate that
// validates the hints: every spent input subtracts its outpoint and every
// spendable output that is not in the hints (i.e. spent within the synced
// range) adds its outpoint, both salted with the swift sync salt.  A
// created-and-spent output cancels, so the aggregate nets to zero at the
// boundary only when the hints are correct.  Pass false to rebuild the UTXO
// set without re-aggregating, e.g. on resume.
func (s *utxoCache) swiftSyncConnectTransactions(b *BlockChain,
	block *btcutil.Block, aggregate bool) error {

	height := block.Height()
	hints, err := b.swiftSync.HintsForBlock(height)
	if err != nil {
		return fmt.Errorf("swift sync: %w", err)
	}

	blockHash := block.Hash()
	net := b.chainParams.Net

	var (
		spendableIdx uint32
		hintCursor   int
	)
	for txIdx, tx := range block.Transactions() {
		isCoinBase := IsCoinBase(tx)
		prevOut := wire.OutPoint{Hash: *tx.Hash()}

		// Subtract every spent outpoint from the aggregate.  Coinbase
		// inputs reference the null outpoint and spend nothing.
		if aggregate && !isCoinBase {
			for _, txIn := range tx.MsgTx().TxIn {
				v := saltedOutpoint(&b.swiftSyncSalt, &txIn.PreviousOutPoint)
				b.swiftSyncAgg.Sub(&v)
			}
		}

		for txOutIdx, txOut := range tx.MsgTx().TxOut {
			if IsHintsfileUnspendableOutput(net, blockHash, height, txIdx, txOut) {
				continue
			}

			prevOut.Index = uint32(txOutIdx)
			unspent := hintCursor < len(hints) &&
				hints[hintCursor] == spendableIdx
			switch {
			case unspent:
				hintCursor++
				err := s.addTxOut(prevOut, txOut, isCoinBase, height)
				if err != nil {
					return err
				}

			case aggregate:
				// Spent within the synced range: add its outpoint so
				// the matching subtract at the spending input cancels.
				v := saltedOutpoint(&b.swiftSyncSalt, &prevOut)
				b.swiftSyncAgg.Add(&v)
			}

			spendableIdx++
		}
	}

	if hintCursor != len(hints) {
		return AssertError(fmt.Sprintf(
			"swift sync: block %d had %d unspent hints but consumed %d",
			height, len(hints), hintCursor,
		))
	}

	return nil
}

// saltedOutpoint returns the swift sync aggregate value for an outpoint: its
// 32-byte txid, 4-byte index, and the secret salt, concatenated and zero-padded
// to the accumulator width.  The salt is unknown to whoever produced the
// hintsfile, so the value cannot be predicted and a crafted hintsfile cannot
// pick offsetting wrong hints that cancel the aggregate.
func saltedOutpoint(salt *[32]byte, op *wire.OutPoint) [agg512Size]byte {
	var v [agg512Size]byte
	copy(v[0:32], op.Hash[:])
	binary.LittleEndian.PutUint32(v[32:36], op.Index)
	copy(v[36:68], salt[:])
	return v
}
