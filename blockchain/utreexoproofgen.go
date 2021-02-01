// Copyright (c) 2013-2018 The btcsuite developers
// Copyright (c) 2015-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcutil"
	"github.com/mit-dci/utreexo/accumulator"
	"github.com/mit-dci/utreexo/btcacc"
)

const (
	// lookahead is the max amount that the utreexo bridgenode should
	// generate time-to-live values for an individual txo
	// During the initial block download, a utreexo bridgenode will
	// hold this many blocks in memory to update the ttl values
	lookahead = 1000
)

// UtreexoBridgeState is the utreexo accumulator state for the bridgenode
type UtreexoBridgeState struct {
	forest *accumulator.Forest
}

// NewUtreexoBridgeState returns a utreexo accumulator state in ram
// TODO: support on disk options
func NewUtreexoBridgeState() *UtreexoBridgeState {
	// Default to ram for now
	return &UtreexoBridgeState{
		forest: accumulator.NewForest(nil, false, "", 0),
	}
}

// RestoreUtreexoBridgeState reads the utreexo bridgestate files on disk and returns
// the initialized UtreexoBridgeState.
func RestoreUtreexoBridgeState(utreexoBSPath string) (*UtreexoBridgeState, error) {
	miscPath := filepath.Join(utreexoBSPath, "miscforestfile.dat")
	miscFile, err := os.Open(miscPath)
	if err != nil {
		return nil, err
	}
	forestPath := filepath.Join(utreexoBSPath, "forestdata.dat")
	fFile, err := os.Open(forestPath)
	if err != nil {
		return nil, err
	}

	f, err := accumulator.RestoreForest(miscFile, fFile, true, false, "", 0)
	if err != nil {
		return nil, err
	}
	return &UtreexoBridgeState{forest: f}, nil
}

// WriteUtreexoBridgeState flushes the current in-ram UtreexoBridgeState to disk.
// This function is meant to be called during shutdown.
func (b *BlockChain) WriteUtreexoBridgeState(utreexoBSPath string) error {
	b.chainLock.Lock()
	defer b.chainLock.Unlock()

	// Tells connectBlock to not update the stateSnapshot
	b.utreexoQuit = true

	// Check and make directory if it doesn't exist
	if _, err := os.Stat(utreexoBSPath); os.IsNotExist(err) {
		os.MkdirAll(utreexoBSPath, 0700)
	}
	miscPath := filepath.Join(utreexoBSPath, "miscforestfile.dat")
	miscFile, err := os.OpenFile(miscPath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		fmt.Println(err)
		return err
	}
	err = b.UtreexoBS.forest.WriteMiscData(miscFile)
	if err != nil {
		return err
	}

	forestPath := filepath.Join(utreexoBSPath, "forestdata.dat")
	fFile, err := os.OpenFile(forestPath, os.O_RDWR|os.O_CREATE, 0755)
	if err != nil {
		return err
	}
	err = b.UtreexoBS.forest.WriteForestToDisk(fFile, true, false)
	if err != nil {
		return err
	}

	log.Infof("Gracefully wrote the UtreexoBridgeState to the disk")

	return nil
}

// UpdateUtreexoBS takes in a non-utreexo Bitcoin block and adds/deletes the txos
// from the passed in block from the UtreexoBridgeState. It returns a utreexo proof
// so that utreexocsns can verify.
func (b *BlockChain) UpdateUtreexoBS(block *btcutil.Block, stxos []SpentTxOut) (*btcacc.UData, error) {
	if block.Height() == 0 {
		return nil, nil
	}
	inskip, outskip := block.DedupeBlock()
	dels, err := blockToDelLeaves(stxos, block, inskip)
	if err != nil {
		return nil, err
	}

	adds := blockToAddLeaves(block, nil, outskip)

	ud, err := btcacc.GenUData(dels, b.UtreexoBS.forest, block.Height())
	if err != nil {
		return nil, err
	}

	// TODO don't ignore undoblock
	_, err = b.UtreexoBS.forest.Modify(adds, ud.AccProof.Targets)
	if err != nil {
		return nil, err
	}

	return &ud, nil
}

// blockToDelLeaves takes a non-utreexo block and stxos and turns the block into
// leaves that are to be deleted from the UtreexoBridgeState.
func blockToDelLeaves(stxos []SpentTxOut, block *btcutil.Block, inskip []uint32) (delLeaves []btcacc.LeafData, err error) {
	var blockInputs int
	var blockInIdx uint32
	for idx, tx := range block.Transactions() {
		if idx == 0 {
			blockInIdx++ // coinbase always has 1 input
			continue
		}
		idx--

		for _, txIn := range tx.MsgTx().TxIn {
			blockInputs++
			// Skip txos on the skip list
			if len(inskip) > 0 && inskip[0] == blockInIdx {
				inskip = inskip[1:]
				blockInIdx++
				continue
			}

			var leaf = btcacc.LeafData{
				// TODO add blockhash in. Left out for compatibility with utreexo master branch
				//BlockHash: *block.Hash(),
				// TODO change this to chainhash.Hash
				TxHash: btcacc.Hash(txIn.PreviousOutPoint.Hash),
				Index:  uint32(txIn.PreviousOutPoint.Index),
				// NOTE blockInIdx is needed for determining skips. So you
				// would really need to variables but you can do this -1
				// since coinbase tx doesn't have an stxo
				Height:   stxos[blockInIdx-1].Height,
				Coinbase: stxos[blockInIdx-1].IsCoinBase,
				Amt:      stxos[blockInIdx-1].Amount,
				PkScript: stxos[blockInIdx-1].PkScript,
			}

			delLeaves = append(delLeaves, leaf)
			blockInIdx++
		}
	}

	// just an assertion to check the code is correct. Should never happen
	if blockInputs != len(stxos) {
		return nil, fmt.Errorf(
			"block height: %v, hash:%x, has %v txs but %v stxos",
			block.Height(), block.Hash(), len(block.Transactions()), len(stxos))
	}

	return
}

// blockToAddLeaves takes a non-utreexo block and stxos and turns the block into
// leaves that are to be added to the UtreexoBridgeState.
func blockToAddLeaves(block *btcutil.Block, remember []bool, outskip []uint32) (leaves []accumulator.Leaf) {
	var txonum uint32
	for coinbase, tx := range block.Transactions() {
		for outIdx, txOut := range tx.MsgTx().TxOut {
			// Skip all the OP_RETURNs
			if isUnspendable(txOut) {
				txonum++
				continue
			}

			// Skip txos on the skip list
			if len(outskip) > 0 && outskip[0] == txonum {
				outskip = outskip[1:]
				txonum++
				continue
			}

			var leaf = btcacc.LeafData{
				// TODO add blockhash in. Left out for compatibility with utreexo master branch
				//BlockHash: *block.Hash(),
				// TODO change this to chainhash.Hash
				TxHash:   btcacc.Hash(*tx.Hash()),
				Index:    uint32(outIdx),
				Height:   block.Height(),
				Coinbase: coinbase == 0,
				Amt:      txOut.Value,
				PkScript: txOut.PkScript,
			}

			uleaf := accumulator.Leaf{Hash: leaf.LeafHash()}

			if len(remember) > int(txonum) {
				uleaf.Remember = remember[txonum]
			}

			leaves = append(leaves, uleaf)
			txonum++
		}
	}

	return
}

// isUnspendable determines whether a tx is spendable or not.
// returns true if spendable, false if unspendable.
//
// NOTE: for utreexo, we're keeping our own isUnspendable function that has the
// same behavior as the bitcoind code. There are some utxos that btcd will mark
// unspendable that bitcoind will not and vise versa.
func isUnspendable(o *wire.TxOut) bool {
	switch {
	case len(o.PkScript) > 10000: //len 0 is OK, spendable
		return true
	case len(o.PkScript) > 0 && o.PkScript[0] == 0x6a: // OP_RETURN is 0x6a
		return true
	default:
		return false
	}
}
