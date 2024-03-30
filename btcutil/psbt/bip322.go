package psbt

import (
	"encoding/base64"
	"errors"

	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

// BuildToSpendTx constructs a transaction to spend an output using the 
// specified message and output script. It computes the message hash, 
// constructs the scriptSig, and creates the to_spend transaction according to
// BIP-322. For more details on BIP-322, see:
// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
//
// Parameters:
//   - message: The message to include in the transaction.
//   - outPkScript: The output script to spend.
//
// Returns:
//   - *wire.MsgTx: The constructed transaction.
//   - error: An error if occurred during the construction.
func BuildToSpendTx(message string, outPkScript []byte) (*wire.MsgTx, error) {
	// Compute the message tagged hash:
	// SHA256(SHA256(tag) || SHA256(tag) || message).
	messageHash := *chainhash.TaggedHash(
		chainhash.TagBIP0322SignedMsg, []byte(message),
	)

	// Construct the scriptSig - OP_0 PUSH32[ message_hash ].
	scriptSigPartOne := []byte{0x00, 0x20}

	// Convert messageHash to a byte slice.
	messageHashBytes := messageHash[:]

	// Create scriptSig with the length of scriptSigPartOne + messageHash.
	scriptSig := make([]byte, len(scriptSigPartOne)+len(messageHashBytes))

	// Copy scriptSigPartOne into scriptSig.
	copy(scriptSig, scriptSigPartOne)

	// Copy messageHash into scriptSig starting from
	// the end of scriptSigPartOne.
	copy(scriptSig[len(scriptSigPartOne):], messageHashBytes)

	// Create to_spend transaction in accordance to BIP-322:
	// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki#full
	toSpendTx := &wire.MsgTx{
		Version:  0,
		LockTime: 0,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Index: 0xFFFFFFFF,
			},
			Sequence: 0,
		}},
		TxOut: []*wire.TxOut{{
			Value:    0,
			PkScript: outPkScript,
		}},
	}

	pInputs := []PInput{{
		FinalScriptSig: scriptSig,
		WitnessScript:  []byte{},
	}}

	pOutputs := []POutput{{}}

	psbt := &Packet{
		UnsignedTx: toSpendTx,
		Inputs: pInputs,
		Outputs: pOutputs,
	}

	tx, err := Extract(psbt)
	if err != nil {
		return nil, err
	}

	return tx, nil
}
