// Copyright (c) 2018 The btcsuite developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package psbt

// The Extractor requires provision of a single PSBT
// in which all necessary signatures are encoded, and
// uses it to construct a fully valid network serialized
// transaction.

import (
	"bytes"

	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
)

// Extract takes a finalized psbt.Packet and outputs a finalized transaction
// instance. Note that if the PSBT is in-complete, then an error
// ErrIncompletePSBT will be returned. As the extracted transaction has been
// fully finalized, it will be ready for network broadcast once returned.
func Extract(p *Packet) (*wire.MsgTx, error) {
	// If the packet isn't complete, then we'll return an error as it
	// doesn't have all the required witness data.
	if !p.IsComplete() {
		return nil, ErrIncompletePSBT
	}

	// First, we'll make a copy of the underlying unsigned transaction (the
	// initial template) so we don't mutate it during our activates below.
	finalTx := p.UnsignedTx.Copy()

	// For each input, we'll now populate any relevant witness and
	// sigScript data.
	for i, tin := range finalTx.TxIn {
		// We'll grab the corresponding internal packet input which
		// matches this materialized transaction input and emplace that
		// final sigScript (if present).
		pInput := p.Inputs[i]
		if pInput.FinalScriptSig != nil {
			tin.SignatureScript = pInput.FinalScriptSig
		}

		// Similarly, if there's a final witness, then we'll also need
		// to extract that as well, parsing the lower-level transaction
		// encoding.
		if pInput.FinalScriptWitness != nil {
			// PSBT_IN_FINAL_SCRIPTWITNESS stores the complete
			// scriptWitness for this transaction input. Decode that
			// single serialized witness stack into TxWitness below.
			witnessReader := bytes.NewReader(
				pInput.FinalScriptWitness,
			)

			// First extract the number of witness items encoded in
			// the serialized scriptWitness.
			witCount, err := wire.ReadVarInt(witnessReader, 0)
			if err != nil {
				return nil, err
			}

			// Allocate one slot per witness item, then read each
			// varint-prefixed item from the witness value. The value
			// must contain exactly this one stack, so the exhaustion
			// check below rejects trailing bytes.
			tin.Witness = make(wire.TxWitness, witCount)
			for j := uint64(0); j < witCount; j++ {
				wit, err := wire.ReadVarBytes(
					witnessReader, 0,
					txscript.MaxScriptSize, "witness",
				)
				if err != nil {
					return nil, err
				}
				tin.Witness[j] = wit
			}

			if err := assertFullyConsumed(witnessReader); err != nil {
				return nil, err
			}
		}
	}

	return finalTx, nil
}
