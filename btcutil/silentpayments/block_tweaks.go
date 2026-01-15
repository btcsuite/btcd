package silentpayments

import (
	"bytes"
	"errors"
	"slices"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/ecdsa"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btclog"
)

var (
	// BIP0341NUMSPoint is an example NUMS point as defined in BIP-0341.
	BIP0341NUMSPoint = []byte{
		0x50, 0x92, 0x9b, 0x74, 0xc1, 0xa0, 0x49, 0x54,
		0xb7, 0x8b, 0x4b, 0x60, 0x35, 0xe9, 0x7a, 0x5e,
		0x07, 0x8a, 0x5a, 0x0f, 0x28, 0xec, 0x96, 0xd5,
		0x47, 0xbf, 0xee, 0x9a, 0xce, 0x80, 0x3a, 0xc0,
	}

	// ErrNonStandardScript is returned if the input script is non-standard
	// and, we cannot extract the public key.
	ErrNonStandardScript = errors.New(
		"non-standard script for input public key extraction",
	)

	// ErrInvalidTaprootWitness is returned if the taproot witness is
	// invalid for public key extraction.
	ErrInvalidTaprootWitness = errors.New(
		"invalid taproot witness for input public key extraction",
	)
)

// PrevOutFetcher is a function that can fetch the previous output script given
// an outpoint.
type PrevOutFetcher func(wire.OutPoint) ([]byte, error)

// SumInputPubKeys extracts and sums the public keys from the given transaction
// inputs. It supports P2PKH, P2WPKH, P2SH-P2WPKH, and P2TR input types,
// according to the BIP-0352 specification for allowed input types for Silent
// Payments. If no public keys could be extracted, nil is returned. This also
// means the transaction does not meet the Silent Payments criteria.
func SumInputPubKeys(tx *wire.MsgTx, getPrevOut PrevOutFetcher,
	logger btclog.Logger) *btcec.PublicKey {

	publicKeys := make([]*btcec.PublicKey, 0, len(tx.TxIn))
	for idx, txIn := range tx.TxIn {
		pubkey, err := PublicKeyFromInput(txIn, getPrevOut)
		if err != nil {
			if !errors.Is(err, ErrNonStandardScript) &&
				logger != nil {

				logger.Warnf("Error extracting public key "+
					"from input %d on tx %s: %v", idx,
					tx.TxHash(), err)
			}

			continue
		}

		publicKeys = append(publicKeys, pubkey)
	}

	return sumPubKeys(publicKeys)
}

// sumPubKeys takes a slice of public keys and returns their sum as a single
// public key.
func sumPubKeys(publicKeys []*btcec.PublicKey) *btcec.PublicKey {
	if len(publicKeys) == 0 {
		return nil
	}

	if len(publicKeys) == 1 {
		return publicKeys[0]
	}

	var keyJ btcec.JacobianPoint
	publicKeys[0].AsJacobian(&keyJ)

	for _, pubkey := range publicKeys[1:] {
		var pubkeyJ btcec.JacobianPoint
		pubkey.AsJacobian(&pubkeyJ)
		btcec.AddNonConst(&pubkeyJ, &keyJ, &keyJ)
	}

	keyJ.ToAffine()
	return btcec.NewPublicKey(&keyJ.X, &keyJ.Y)
}

// PublicKeyFromInput extracts the public key from the given transaction input.
// It supports P2PKH, P2WPKH, P2SH-P2WPKH, and P2TR input types, according to
// the BIP-0352 specification for allowed input types for Silent Payments.
func PublicKeyFromInput(txIn *wire.TxIn,
	getPrevOut PrevOutFetcher) (*btcec.PublicKey, error) {

	sigScriptLen := len(txIn.SignatureScript)
	witnessLen := len(txIn.Witness)

	// P2SH-P2WPKH is one of the script type we can determine with
	// reasonable certainty without looking at the previous output script.
	// The scriptSig contains the witness version and public key hash, while
	// the witness contains the signature and public key.
	//
	//    witness:      <signature> <33-byte-compressed-key>
	//    scriptSig:    <0 <20-byte-key-hash>>
	//                  (0x160014{20-byte-key-hash})
	//    scriptPubKey: HASH160 <20-byte-script-hash> EQUAL
	//                  (0xA914{20-byte-script-hash}87)
	if sigScriptLen > 0 && witnessLen == 2 {
		pubKeyBytes := txIn.Witness[1]
		if len(pubKeyBytes) != 33 {
			return nil, ErrNonStandardScript
		}
		return btcec.ParsePubKey(pubKeyBytes)
	}

	switch {
	// Shortcut for non-malleated P2PKH.
	case sigScriptLen > 0 &&
		txscript.IsPushOnlyScript(txIn.SignatureScript):

		// Since we checked that there are data pushes only, we can
		// extract the pushes as if it was a witness stack.
		witness, err := scriptAsWitness(txIn.SignatureScript)
		if err != nil {
			break
		}

		// If the witness looks like a P2PKH witness, we can extract
		// the public key directly.
		if isPayToWitnessPubKeyHashWitness(witness) {
			return btcec.ParsePubKey(witness[1])
		}

	// Shortcut for P2WPKH.
	case sigScriptLen == 0 && isPayToWitnessPubKeyHashWitness(txIn.Witness):
		return btcec.ParsePubKey(txIn.Witness[1])
	}

	// For everything else, we're going to need to look at the previous
	// output script to be certain.
	prevOutScript, err := getPrevOut(txIn.PreviousOutPoint)
	if err != nil {
		return nil, err
	}

	switch {
	case txscript.IsPayToPubKeyHash(prevOutScript):
		// P2PKH can also be identified by the signature script and the
		// absence of witness data alone.
		//
		//    scriptSig:    <signature> <33-byte-compressed-key>
		//    scriptPubKey: OP_DUP HASH160 <20-byte-key-hash>
		//                  OP_EQUALVERIFY OP_CHECKSIG
		//                  (0x76A914{20-byte-key-hash}88AC)
		// We must, however, make sure that we use the last push in the
		// scriptSig as the public key, in case the tx was malleated to
		// add extra data to the front of the scriptSig.
		if sigScriptLen < 2 || witnessLen != 0 {
			return nil, ErrNonStandardScript
		}

		targetHash := prevOutScript[3:23]
		for i := len(txIn.SignatureScript); i >= 0; i-- {
			if i-33 >= 0 {
				pubKey := txIn.SignatureScript[i-33 : i]
				pubKeyHash := btcutil.Hash160(pubKey)

				if bytes.Equal(targetHash, pubKeyHash) {
					return btcec.ParsePubKey(pubKey)
				}
			}
		}

		return nil, ErrNonStandardScript

	case txscript.IsPayToWitnessPubKeyHash(prevOutScript):
		// P2WPKH has no scriptSig, only witness data.
		//
		//    witness:      <signature> <33-byte-compressed-key>
		//    scriptSig:    (empty)
		//    scriptPubKey: 0 <20-byte-key-hash>
		//                  (0x0014{20-byte-key-hash})
		if witnessLen != 2 || len(txIn.Witness[1]) != 33 {
			return nil, ErrNonStandardScript
		}

		_, err := ecdsa.ParseSignature(txIn.Witness[0])
		if err != nil {
			return nil, ErrNonStandardScript
		}

		return btcec.ParsePubKey(txIn.Witness[1])

	case txscript.IsPayToTaproot(prevOutScript):
		// P2TR has no scriptSig, only witness data.
		// There are two cases, either the keypath spend or the
		// script path spend.
		//
		// Keypath spend:
		//
		//    witness:      <signature>
		//    scriptSig:    (empty)
		//    scriptPubKey: 1 <32-byte-x-only-key>
		//                  (0x5120{32-byte-x-only-key})
		//
		// Script path spend:
		//
		//    witness:      <optional witness items> <leaf script>
		//                  <control block>
		//    scriptSig:    (empty)
		//    scriptPubKey: 1 <32-byte-x-only-key>
		//                  (0x5120{32-byte-x-only-key})
		if witnessLen < 1 {
			return nil, ErrInvalidTaprootWitness
		}

		witness := slices.Clone(txIn.Witness)

		// Remove annex if present.
		if len(witness) > 1 && len(witness[len(witness)-1]) > 0 &&
			witness[len(witness)-1][0] == 0x50 {

			witness = witness[:len(witness)-1]
		}

		// Script path spend with NUMS internal key is not allowed.
		if len(witness) > 1 {
			controlBlock := witness[len(witness)-1]
			internalKey := controlBlock[1:33]
			if bytes.Equal(internalKey, BIP0341NUMSPoint) {
				return nil, ErrNonStandardScript
			}
		}

		// Extract the taproot key from the scriptPubKey.
		taprootKey := prevOutScript[2:]
		return schnorr.ParsePubKey(taprootKey)

	default:
		return nil, ErrNonStandardScript
	}
}

// OutputMatches checks whether the given target output key matches the output
// key derived from the given spend public key, label tweak, share sum,
// iterator k, and input hash. It returns true if a match is found, false
// otherwise.
func OutputMatches(targetOutputKey btcec.PublicKey,
	spendPubKey btcec.PublicKey, labelTweak *btcec.ModNScalar,
	shareSum btcec.PublicKey, k uint32,
	inputHash btcec.ModNScalar) (bool, error) {

	// We first try with no label tweak.
	tweakedSpendKey := LabelSpendKey(nil, &spendPubKey)
	outputKey, err := CreateOutputKey(
		shareSum, *tweakedSpendKey, k, inputHash,
	)
	if err != nil {
		return false, err
	}

	// Did we find a match without the label?
	if schnorrKeyEqual(&targetOutputKey, outputKey) {
		return true, nil
	}

	// Now we try with the label tweak.
	tweakedSpendKey = LabelSpendKey(labelTweak, &spendPubKey)
	outputKey, err = CreateOutputKey(
		shareSum, *tweakedSpendKey, k, inputHash,
	)
	if err != nil {
		return false, err
	}

	// Did we find a match with the label?
	if schnorrKeyEqual(&targetOutputKey, outputKey) {
		return true, nil
	}

	return false, nil
}

func schnorrKeyEqual(key1, key2 *btcec.PublicKey) bool {
	return bytes.Equal(
		schnorr.SerializePubKey(key1),
		schnorr.SerializePubKey(key2),
	)
}

func scriptAsWitness(sigScript []byte) (wire.TxWitness, error) {
	var witness wire.TxWitness

	const scriptVersion = 0
	tokenizer := txscript.MakeScriptTokenizer(scriptVersion, sigScript)
	for tokenizer.Next() {
		// All opcodes up to OP_16 are data push instructions.
		if tokenizer.Opcode() > txscript.OP_16 {
			return nil, ErrNonStandardScript
		}

		witness = append(witness, tokenizer.Data())
	}

	return witness, tokenizer.Err()
}

func isPayToWitnessPubKeyHashWitness(witness wire.TxWitness) bool {
	if len(witness) != 2 {
		return false
	}

	// First item must be an ECDSA signature.
	if _, err := ecdsa.ParseSignature(witness[0]); err != nil {
		return false
	}

	// Second item must be a 33-byte compressed public key.
	if _, err := btcec.ParsePubKey(witness[1]); err != nil {
		return false
	}

	return true
}
