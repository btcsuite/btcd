// Copyright (c) 2017 The btcsuite developers
// Copyright (c) 2017 The Lightning Network Developers
// Copyright (c) 2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

/*
Package blockcf provides functions for building committed filters for blocks
using Golomb-coded sets in a way that is useful for light clients such as SPV
wallets.

Committed filters are a reversal of how bloom filters are typically used by a
light client: a consensus-validating full node commits to filters for every
block with a predetermined collision probability and light clients match against
the filters locally rather than uploading personal data to other nodes.  If a
filter matches, the light client should fetch the entire block and further
inspect it for relevant transactions.
*/
package blockcf

import (
	"encoding/binary"

	"github.com/decred/dcrd/blockchain/stake"
	"github.com/decred/dcrd/chaincfg/chainhash"
	"github.com/decred/dcrd/gcs"
	"github.com/decred/dcrd/txscript"
	"github.com/decred/dcrd/wire"
)

// P is the collision probability used for block committed filters (2^-20)
const P = 20

// Entries describes all of the filter entries used to create a GCS filter and
// provides methods for appending data structures found in blocks.
type Entries [][]byte

// AddOutPoint adds a serialized outpoint to an entries slice.
func (e *Entries) AddOutPoint(outpoint *wire.OutPoint) {
	entry := make([]byte, chainhash.HashSize+4)
	copy(entry, outpoint.Hash[:])
	binary.LittleEndian.PutUint32(entry[chainhash.HashSize:], outpoint.Index)

	*e = append(*e, entry)
}

// AddHash adds a hash to an entries slice.
func (e *Entries) AddHash(hash *chainhash.Hash) {
	*e = append(*e, hash[:])
}

// AddRegularPkScript adds the regular tx output script to an entries slice.
func (e *Entries) AddRegularPkScript(script []byte) {
	*e = append(*e, script)
}

// AddStakePkScript adds the output script without the stake opcode tag to an
// entries slice.
func (e *Entries) AddStakePkScript(script []byte) {
	*e = append(*e, script[1:])
}

// AddSigScript adds any data pushes of a signature script to an entries slice.
func (e *Entries) AddSigScript(script []byte) {
	// Ignore errors and add pushed data, if any
	pushes, err := txscript.PushedData(script)
	if err == nil && len(pushes) != 0 {
		*e = append(*e, pushes...)
	}
}

// Key creates a block committed filter key by truncating a block hash to the
// key size.
func Key(hash *chainhash.Hash) [gcs.KeySize]byte {
	var key [gcs.KeySize]byte
	copy(key[:], hash[:])
	return key
}

// Regular builds a regular GCS filter from a block.  A regular GCS filter will
// contain all the previous regular outpoints spent within a block, as well as
// the data pushes within all the outputs created within a block which can be
// spent by regular transactions.
func Regular(block *wire.MsgBlock) (*gcs.Filter, error) {
	var data Entries

	// Add "regular" data from stake transactions.  For each class of stake
	// transaction, the following data is committed to the regular filter:
	//
	//   ticket purchases:
	//     - all previous outpoints
	//     - all change output scripts
	//
	//   votes:
	//     - all OP_SSGEN-tagged output scripts (all outputs after the first
	//       two -- these describe the block voted on and the vote choices)
	//
	//   revocations:
	//     - all output scripts
	//
	// Because change outputs are required in a ticket purchase, even when
	// unused, a special case is made that excludes their commitment when the
	// output value is zero (provably unspendable).
	//
	// Output scripts are handled specially for stake transactions by slicing
	// off the stake opcode tag (OP_SS*).  This tag always appears as the first
	// byte of the script and removing it allows users of the filter to only
	// match against a normal P2PKH or P2SH script, instead of many extra
	// matches for each tag.
	for _, tx := range block.STransactions {
		switch stake.DetermineTxType(tx) {
		case stake.TxTypeSStx: // Ticket purchase
			for _, in := range tx.TxIn {
				data.AddOutPoint(&in.PreviousOutPoint)
			}
			for i := 2; i < len(tx.TxOut); i += 2 { // Iterate change outputs
				out := tx.TxOut[i]
				if out.Value != 0 {
					data.AddStakePkScript(out.PkScript)
				}
			}

		case stake.TxTypeSSGen: // Vote
			for _, out := range tx.TxOut[2:] { // Iterate generated coins
				data.AddStakePkScript(out.PkScript)
			}

		case stake.TxTypeSSRtx: // Revocation
			for _, out := range tx.TxOut {
				data.AddStakePkScript(out.PkScript)
			}
		}
	}

	// For regular transactions, all previous outpoints except the coinbase's
	// are committed, and all output scripts are committed.
	for i, tx := range block.Transactions {
		if i != 0 {
			for _, txIn := range tx.TxIn {
				data.AddOutPoint(&txIn.PreviousOutPoint)
			}
		}
		for _, txOut := range tx.TxOut {
			data.AddRegularPkScript(txOut.PkScript)
		}
	}

	// Create the key by truncating the block hash.
	blockHash := block.BlockHash()
	key := Key(&blockHash)

	return gcs.NewFilter(P, key, data)
}

// Extended builds an extended GCS filter from a block.  An extended filter
// supplements a regular basic filter by including all transaction hashes of
// regular and stake transactions, and adding the witness data (a.k.a. the
// signature script) found within every non-coinbase regular transaction.
func Extended(block *wire.MsgBlock) (*gcs.Filter, error) {
	var data Entries

	// For each stake transaction, commit the transaction hash.  If the
	// transaction is a ticket purchase, commit pushes from the signature script
	// (witness).
	for _, tx := range block.STransactions {
		txHash := tx.TxHash()
		data.AddHash(&txHash)

		if stake.IsSStx(tx) {
			for _, in := range tx.TxIn {
				data.AddSigScript(in.SignatureScript)
			}
		}
	}

	// For each regular transaction, commit the transaction hash.  For all
	// regular transactions except the coinbase, commit pushes to the signature
	// script (witness).
	coinbaseHash := block.Transactions[0].TxHash()
	data.AddHash(&coinbaseHash)
	for _, tx := range block.Transactions[1:] {
		txHash := tx.TxHash()
		data.AddHash(&txHash)

		for _, txIn := range tx.TxIn {
			if txIn.SignatureScript != nil {
				data.AddSigScript(txIn.SignatureScript)
			}
		}
	}

	// Create the key by truncating the block hash.
	blockHash := block.BlockHash()
	key := Key(&blockHash)

	return gcs.NewFilter(P, key, data)
}
