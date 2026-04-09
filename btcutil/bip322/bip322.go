// Package bip322 implements generic message signing. For more details on
// BIP-322 see: https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki
package bip322

import (
	"bytes"
	"encoding/base64"
	"errors"
	"fmt"

	"github.com/btcsuite/btcd/btcutil/psbt"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
)

const (
	// TagBIP0322SignedMsg is the BIP-0322 tag for a signed message. It is
	// declared as an untyped string constant so it cannot be mutated at
	// runtime (which would silently break all sign/verify operations in
	// the same process).
	TagBIP0322SignedMsg = "BIP0322-signed-message"

	// PrefixSimple is the signature prefix for the "simple" variant of
	// BIP-322.
	PrefixSimple = "smp"

	// PrefixFull is the signature prefix for the "full" variant of BIP-322.
	PrefixFull = "ful"

	// PrefixProofOfFunds is the signature prefix for the "proof of funds"
	// variant of BIP-322.
	PrefixProofOfFunds = "pof"
)

var (
	// errSimpleSegWitOnly is returned when a non-native SegWit output is
	// attempted to be signed with the "simple" variant of BIP-322.
	errSimpleSegWitOnly = errors.New(
		"only native SegWit outputs (P2WPKH, P2WSH, P2TR) are " +
			"supported for the simple variant",
	)

	// b64Encode is a shortcut for the standard base64 encoding function.
	b64Encode = base64.StdEncoding.EncodeToString

	// b64Decode is a shortcut for the standard base64 decoding function.
	b64Decode = base64.StdEncoding.DecodeString
)

// buildToSpendTx constructs a transaction to spend an output using the
// specified message and output script. It computes the message hash,
// constructs the scriptSig, and creates the to_spend transaction, according to
// BIP-322.
func buildToSpendTx(message, outPkScript []byte) *wire.MsgTx {
	// Compute the message tagged hash:
	// SHA256(SHA256(tag) || SHA256(tag) || message).
	messageHash := *chainhash.TaggedHash(
		[]byte(TagBIP0322SignedMsg), message,
	)

	// Construct the scriptSig - OP_0 PUSH32[ message_hash ].
	scriptSig := append([]byte{0x00, 0x20}, messageHash[:]...)

	// Create to_spend transaction in accordance to BIP-322:
	// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki#full
	return &wire.MsgTx{
		Version:  0,
		LockTime: 0,
		TxIn: []*wire.TxIn{{
			PreviousOutPoint: wire.OutPoint{
				Index: 0xFFFFFFFF,
			},
			Sequence:        0,
			SignatureScript: scriptSig,
			Witness:         wire.TxWitness{},
		}},
		TxOut: []*wire.TxOut{{
			Value:    0,
			PkScript: outPkScript,
		}},
	}
}

// isNativeSegWitPkScript returns true iff pkScript is one of the native SegWit
// script types (P2WPKH, P2WSH, P2TR) supported by the BIP-322 "simple" variant.
func isNativeSegWitPkScript(pkScript []byte) bool {
	return txscript.IsPayToWitnessPubKeyHash(pkScript) ||
		txscript.IsPayToWitnessScriptHash(pkScript) ||
		txscript.IsPayToTaproot(pkScript)
}

// BuildToSignPacketSimple constructs a transaction template PSBT packet to
// prepare for signing, using the message and spend pkScript for the "simple"
// variant of the BIP-322 specification. It creates the to_sign transaction
// template, according to BIP-322. This can only be used for native SegWit
// outputs (P2WPKH, P2WSH, P2TR), and the spend pkScript is validated
// accordingly.
func BuildToSignPacketSimple(message, pkScript []byte) (*psbt.Packet, error) {
	// Enforce an inclusion list: only native SegWit outputs are valid for
	// the simple variant. Any other script type (legacy P2PKH/P2SH, bare
	// multisig, OP_RETURN, unknown future witness versions, etc.) must use
	// the full variant.
	if !isNativeSegWitPkScript(pkScript) {
		return nil, errSimpleSegWitOnly
	}

	return BuildToSignPacketFull(message, pkScript, 0, 0, 0), nil
}

// BuildToSignPacketFull constructs a transaction template PSBT packet to
// prepare for signing, using the message, spend pkScript, and tx parameters for
// the "full" variant of the BIP-322 specification. It creates the to_sign
// transaction template, according to BIP-322.
func BuildToSignPacketFull(message, spendPkScript []byte,
	txVersion int32, lockTime, sequence uint32) *psbt.Packet {

	spendTx := buildToSpendTx(message, spendPkScript)

	// Create to_sign transaction in accordance to BIP-322:
	// https://github.com/bitcoin/bips/blob/master/bip-0322.mediawiki#full
	packet := &psbt.Packet{
		UnsignedTx: &wire.MsgTx{
			Version:  txVersion,
			LockTime: lockTime,
			TxIn: []*wire.TxIn{{
				PreviousOutPoint: wire.OutPoint{
					Hash:  spendTx.TxHash(),
					Index: 0,
				},
				Sequence: sequence,
			}},
			TxOut: []*wire.TxOut{{
				Value:    0,
				PkScript: []byte{txscript.OP_RETURN},
			}},
		},
		Inputs: []psbt.PInput{{
			WitnessUtxo: &wire.TxOut{
				Value:    0,
				PkScript: spendPkScript,
			},
			NonWitnessUtxo: spendTx,
		}},
		Outputs: []psbt.POutput{{}},
	}

	// Legacy scripts can't have a witness UTXO, otherwise the PSBT
	// extraction will fail.
	if txscript.IsPayToPubKey(spendPkScript) ||
		txscript.IsPayToPubKeyHash(spendPkScript) {

		packet.Inputs[0].WitnessUtxo = nil
	}

	return packet
}

// SerializeTxWitness returns the wire witness stack as raw bytes.
func SerializeTxWitness(txWitness wire.TxWitness) ([]byte, error) {
	var witnessBytes bytes.Buffer
	err := psbt.WriteTxWitness(&witnessBytes, txWitness)
	if err != nil {
		return nil, fmt.Errorf("error serializing witness: %w", err)
	}

	return witnessBytes.Bytes(), nil
}

// SerializeSignature serializes the signature of a finalized PSBT packet of a
// BIP-322 to_sign transaction. According to the rules described in the BIP,
// this writes the signature as one of the three formats:
//  1. Simple (smp): Version and LockTime are 0, only one input with Sequence 0.
//  2. Full (ful): Version or LockTime or Sequence are non-zero, single input.
//  3. Proof of Funds (pof): Version or LockTime or Sequence are non-zero,
//     multiple inputs.
func SerializeSignature(finalizedPacket *psbt.Packet) (string, error) {
	if finalizedPacket == nil {
		return "", errors.New("nil packet")
	}

	// Prevent us from panicking on either if the length doesn't match.
	if len(finalizedPacket.Inputs) != len(finalizedPacket.UnsignedTx.TxIn) {
		return "", errors.New("input and txin length mismatch")
	}

	// At this point at least one input must be provided.
	if len(finalizedPacket.Inputs) == 0 {
		return "", errors.New("missing inputs")
	}

	// There is no exported IsFinalized function. But calling
	// MaybeFinalizeAll on an already finalized packet should not produce an
	// error if it's already finalized.
	if err := psbt.MaybeFinalizeAll(finalizedPacket); err != nil {
		return "", fmt.Errorf("packet must be finalizable: %w", err)
	}

	tx := finalizedPacket.UnsignedTx
	utxo := finalizedPacket.Inputs[0].WitnessUtxo
	if utxo == nil {
		if finalizedPacket.Inputs[0].NonWitnessUtxo == nil {
			return "", errors.New("missing utxo")
		}

		// The to_spend transaction must have exactly one output to be
		// a valid BIP-322 previous transaction to the to_sign's first
		// input.
		prevTx := finalizedPacket.Inputs[0].NonWitnessUtxo
		if len(prevTx.TxOut) != 1 {
			return "", errors.New("invalid non witness UTXO")
		}

		utxo = prevTx.TxOut[0]
	}

	// Detect the variant of the signature.
	switch {
	// Proof of Fund (pof) variant has multiple inputs.
	case len(finalizedPacket.Inputs) > 1:
		content, err := finalizedPacket.B64Encode()
		if err != nil {
			return "", fmt.Errorf("error encoding packet: %w", err)
		}

		return PrefixProofOfFunds + content, nil

	// Full (ful) variant has non-zero version, locktime, or sequence or a
	// non-native SegWit input.
	case tx.Version != 0 || tx.LockTime != 0 || tx.TxIn[0].Sequence != 0 ||
		!isNativeSegWitPkScript(utxo.PkScript):

		signedTx, err := psbt.Extract(finalizedPacket)
		if err != nil {
			return "", fmt.Errorf("error extracting packet: %w",
				err)
		}

		var signedTxBytes bytes.Buffer
		err = signedTx.Serialize(&signedTxBytes)
		if err != nil {
			return "", fmt.Errorf("error serializing signed tx: "+
				"%w", err)
		}

		content := b64Encode(signedTxBytes.Bytes())
		return PrefixFull + content, nil

	// The simple (smp) variant is used if the above cases don't match.
	default:
		signedTx, err := psbt.Extract(finalizedPacket)
		if err != nil {
			return "", fmt.Errorf("error extracting packet: %w",
				err)
		}

		witnessBytes, err := SerializeTxWitness(
			signedTx.TxIn[0].Witness,
		)
		if err != nil {
			return "", fmt.Errorf("error serializing witness: %w",
				err)
		}

		content := b64Encode(witnessBytes)
		return PrefixSimple + content, nil
	}
}
