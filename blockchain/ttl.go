// Copyright (c) 2015-2016 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package blockchain

import (
	"github.com/btcsuite/btcutil"
)

type ttlBlock struct {
	height  int32
	created []txoStart
}

type txoStart struct {
	createHeight int32
	stxoIdx      uint32
}

func GenTTL(block btcutil.Block, view *UtxoViewpoint, inskip, outskip []uint32) (*ttlBlock, error) {
	//var tBlock ttlBlock
	//var txoIdxForBlock, txInIdxForBlock uint32

	//for idx, tx := range block.Transactions() {
	//	for outIdx, txOut := range tx.MsgTx().TxOut {
	//		if len(outskip) > 0 && txoInBlock == outskip[0] {
	//			// skip inputs in the txin skiplist
	//			// fmt.Printf("skipping output %s:%d\n", txid.String(), txoInTx)
	//			outskip = outskip[1:]
	//			txoInBlock++
	//			continue
	//		}
	//		if isUnspendable(txo) {
	//			txoInBlock++
	//			continue
	//		}

	//		trb.newTxos = append(trb.newTxos,
	//			util.OutpointToBytes(wire.NewOutPoint(tx.Hash(), uint32(txoInTx))))
	//		txoInBlock++
	//	}

	//	for inIdx, txIn := range tx.MsgTx().TxIn {
	//		if txInIdxForBlock == 0 {
	//			txInIdxForBlock += uint32(len(tx.MsgTx().TxIn))
	//			break // skip coinbase input
	//		}
	//		if len(inskip) > 0 && txInIdxForBlock == inskip[0] {
	//			// skip inputs in the txin skiplist
	//			// fmt.Printf("skipping input %s\n", in.PreviousOutPoint.String())
	//			inskip = inskip[1:]
	//			txInIdxForBlock++
	//			continue
	//		}

	//		entry := view.entries[txIn.PreviousOutPoint]

	//		// append outpoint to slice
	//		trb.spentTxos = append(trb.spentTxos,
	//			util.OutpointToBytes(&in.PreviousOutPoint))
	//		// append start height to slice (get from rev data)
	//		trb.spentStartHeights = append(trb.spentStartHeights,
	//			bnr.Rev.Txs[txInBlock-1].TxIn[txinInTx].Height)

	//		txInIdxForBlock++
	//	}
	//}
	return nil, nil
}

// DedupeBlock takes a bitcoin block, and returns two int slices: the indexes of
// inputs, and idexes of outputs which can be removed.  These are indexes
// within the block as a whole, even the coinbase tx.
// So the coinbase tx in & output numbers affect the skip lists even though
// the coinbase ins/outs can never be deduped.  it's simpler that way.
//func DedupeBlock(blk *btcutil.Block) (inskip []uint32, outskip []uint32) {
//	var i uint32
//	// wire.Outpoints are comparable with == which is nice.
//	inmap := make(map[wire.OutPoint]uint32)
//
//	// go through txs then inputs building map
//	for cbif0, tx := range blk.Transactions() {
//		if cbif0 == 0 { // coinbase tx can't be deduped
//			i++ // coinbase has 1 input
//			continue
//		}
//		for _, in := range tx.MsgTx().TxIn {
//			inmap[in.PreviousOutPoint] = i
//			i++
//		}
//	}
//
//	i = 0
//	// start over, go through outputs finding skips
//	for cbif0, tx := range blk.Transactions() {
//		if cbif0 == 0 { // coinbase tx can't be deduped
//			i += uint32(len(tx.MsgTx().TxOut)) // coinbase can have multiple inputs
//			continue
//		}
//
//		for outidx, _ := range tx.MsgTx().TxOut {
//			op := wire.OutPoint{Hash: *tx.Hash(), Index: uint32(outidx)}
//			inpos, exists := inmap[op]
//			if exists {
//				inskip = append(inskip, inpos)
//				outskip = append(outskip, i)
//			}
//			i++
//		}
//	}
//
//	// sort inskip list, as it's built in order consumed not created
//	sortUint32s(inskip)
//	return
//}
//
//// it'd be cool if you just had .sort() methods on slices of builtin types...
//func sortUint32s(s []uint32) {
//	sort.Slice(s, func(a, b int) bool { return s[a] < s[b] })
//}
