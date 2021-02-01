// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/btcsuite/btcutil"
	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/btcacc"
)

// UtreexoViewpoint is the compact state of the chainstate using the utreexo accumulator
type UtreexoViewpoint struct {
	accumulator accumulator.Pollard
}

// Modify takes an ublock and adds the utxos and deletes the stxos from the utreexo state
func (uview *UtreexoViewpoint) Modify(ub *btcutil.UBlock) error {
	// Grab all the sstxo indexes of the same block spends
	// inskip is all the txIns that reference a txOut in the same block
	// outskip is all the txOuts that are referenced by a txIn in the same block
	inskip, outskip := ub.Block().DedupeBlock()

	// grab the "nl" (numLeaves) which is number of all the utxos currently in the
	// utreexo accumulator. h is the height of the utreexo accumulator
	nl, h := uview.accumulator.ReconstructStats()

	// ProofSanity checks the consistency of a UBlock. It checks that there are
	// enough proofs for all the referenced txOuts and that the these proofs are
	// for that txOut
	err := ub.ProofSanity(inskip, nl, h)
	if err != nil {
		return err
	}

	// IngestBatchProof first checks that the utreexo proofs are valid. If it is valid,
	// it readys the utreexo accumulator for additions/deletions.
	err = uview.accumulator.IngestBatchProof(ub.MsgUBlock().UtreexoData.AccProof)
	if err != nil {
		return err
	}

	// Remember is used to keep some utxos that will be spent in the near future
	// so that the node won't have to re-download those UTXOs over the wire.
	remember := make([]bool, len(ub.MsgUBlock().UtreexoData.TxoTTLs))
	for i, ttl := range ub.MsgUBlock().UtreexoData.TxoTTLs {
		// If the time-to-live value is less than the chosen amount of blocks
		// then remember it.
		remember[i] = ttl < uview.accumulator.Lookahead
	}

	// Make the now verified utxos into 32 byte leaves ready to be added into the
	// utreexo accumulator.
	leaves := BlockToAddLeaves(ub.Block(), remember, outskip, ub.MsgUBlock().UtreexoData.Height)

	// Add the utxos into the accumulator
	err = uview.accumulator.Modify(leaves, ub.MsgUBlock().UtreexoData.AccProof.Targets)
	if err != nil {
		return err
	}

	return nil
}

// BlockToAddLeaves turns all the new utxos in the block into "leaves" which are 32 byte
// hashes that are ready to be added into the utreexo accumulator. Unspendables and
// same block spends are excluded.
func BlockToAddLeaves(blk *btcutil.Block,
	remember []bool, skiplist []uint32,
	height int32) (leaves []accumulator.Leaf) {

	var txonum uint32
	// bh := bl.Blockhash
	for coinbaseif0, tx := range blk.Transactions() {
		// cache txid aka txhash
		for i, out := range tx.MsgTx().TxOut {
			// Skip all the OP_RETURNs
			if isUnspendable(out) {
				txonum++
				continue
			}
			// Skip txos on the skip list
			if len(skiplist) > 0 && skiplist[0] == txonum {
				skiplist = skiplist[1:]
				txonum++
				continue
			}

			var l btcacc.LeafData
			// TODO put blockhash back in -- leaving empty for now!
			// l.BlockHash = bh
			l.TxHash = btcacc.Hash(*tx.Hash())
			l.Index = uint32(i)
			l.Height = height
			if coinbaseif0 == 0 {
				l.Coinbase = true
			}
			l.Amt = out.Value
			l.PkScript = out.PkScript
			uleaf := accumulator.Leaf{Hash: l.LeafHash()}
			if uint32(len(remember)) > txonum {
				uleaf.Remember = remember[txonum]
			}
			leaves = append(leaves, uleaf)
			txonum++
		}
	}
	return
}

// UBlockToStxos extracts all the referenced SpentTxOuts in the block to the stxos
func UBlockToStxos(ublock *btcutil.UBlock, stxos *[]SpentTxOut) error {
	// First, add all the referenced inputs
	for _, ustxo := range ublock.MsgUBlock().UtreexoData.Stxos {
		stxo := SpentTxOut{
			Amount:     ustxo.Amt,
			PkScript:   ustxo.PkScript,
			Height:     ustxo.Height,
			IsCoinBase: ustxo.Coinbase,
		}
		*stxos = append(*stxos, stxo)
	}

	// grab all sstxo indexes for all the same block spends
	// Since the udata excludes any same block spends, this step is necessary
	_, outskip := ublock.Block().DedupeBlock()

	// Go through all the transactions and find the same block spends
	// Add the txOuts of these spends to stxos
	var txonum uint32
	for coinbaseif0, tx := range ublock.Block().MsgBlock().Transactions {
		for _, txOut := range tx.TxOut {
			// Skip all the OP_RETURNs
			if isUnspendable(txOut) {
				txonum++
				continue
			}
			// Skip txos on the skip list
			if len(outskip) > 0 && outskip[0] == txonum {
				//fmt.Println("ADD:", txonum)
				stxo := SpentTxOut{
					Amount:     txOut.Value,
					PkScript:   txOut.PkScript,
					Height:     ublock.Block().Height(),
					IsCoinBase: coinbaseif0 == 0,
				}
				*stxos = append(*stxos, stxo)
				outskip = outskip[1:]
				txonum++
				continue
			}
			txonum++
		}
	}

	return nil
}

// NewUtreexoViewpoint returns an empty UtreexoViewpoint
func NewUtreexoViewpoint() *UtreexoViewpoint {
	return &UtreexoViewpoint{
		accumulator: accumulator.Pollard{},
	}
}
