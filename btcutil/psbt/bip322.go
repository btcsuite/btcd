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

// BuildToSignTx constructs a transaction to prepare for signing using the
// specified transaction ID, witness script, and additional parameters. It
// creates the to_sign transaction according to BIP-322. For more details on
// BIP-322, see: https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
//
// Parameters:
//   - toSpendTxId: The transaction ID of the output to spend.
//   - witnessScript: The witness script corresponding to the output being
//	spent.
//   - isRedeemScript: Whether the witness script should be used as the redeem
//	script.
//   - tapInternalKey: The internal key for Taproot if applicable, or nil if 
//	not used.
//
// Returns:
//   - *Packet: The constructed Partially Signed Bitcoin Transaction (PSBT).
//   - error: An error if occurred during the construction.
func BuildToSignTx(toSpendTxId string, witnessScript []byte,
	isRedeemScript bool, tapInternalKey []byte) (*Packet, error) {

	toSpendTxIdHash, err := chainhash.NewHashFromStr(toSpendTxId)
	if err != nil {
		return nil, err
	}

	// Create to_sign transaction in accordance to BIP-322:
	// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki#full
	toSignTx := &wire.MsgTx{
		Version:  0,
		LockTime: 0,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Hash:  *toSpendTxIdHash,
				Index: 0,
			},
			Sequence: 0,
		}},
		TxOut: []*wire.TxOut{{
			Value:    0,
			PkScript: []byte{txscript.OP_RETURN},
		}},
	}

	pInputs := []PInput{{
		WitnessUtxo: &wire.TxOut{
			Value:    0,
			PkScript: witnessScript,
		},
	}}

	pOutputs := []POutput{{}}

	psbt := &Packet{
		UnsignedTx: toSignTx,
		Inputs: pInputs,
		Outputs: pOutputs,
	}

	// Create an updater for the psbt. This also performs a sanity check.
	updater, err := NewUpdater(psbt)
	if err != nil {
		return nil, err
	}

	// Set redeemScript as witnessScript if isRedeemScript.
	if isRedeemScript {
		err = updater.AddInRedeemScript(witnessScript, 0)
		if err != nil {
			return nil, err
		}
	}

	// Set tapInternalKey if provided.
	if tapInternalKey != nil {
		err = updater.AddInTaprootInternalKey(tapInternalKey, 0)
		if err != nil {
			return nil, err
		}
	}

	return psbt, nil
}

// EncodeWitness encodes witness stack in a signed BIP-322 PSBT into
// its base-64 encoded format. For more details on
// BIP-322, see: https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
//
// Parameters:
//   - signedPsbt: The signed Partially Signed Bitcoin Transaction (PSBT).
//
// Returns:
//   - string: The base-64 encoded witness stack.
//   - error: An error if the witness data is empty.
func EncodeWitness(signedPsbt *Packet) (string, error) {
	witness := signedPsbt.Inputs[0].FinalScriptWitness
	if len(witness) > 0 {
		return base64.StdEncoding.EncodeToString(witness), nil
	} else {
		return "", errors.New("witness data is empty")
	}

}
