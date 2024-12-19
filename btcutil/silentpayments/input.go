package silentpayments

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

var (
	// TagBIP0352Inputs is the BIP-0352 tag for a inputs.
	TagBIP0352Inputs = []byte("BIP0352/Inputs")

	// TagBIP0352SharedSecret is the BIP-0352 tag for a shared secret.
	TagBIP0352SharedSecret = []byte("BIP0352/SharedSecret")
	
	// ErrInputKeyZero is returned if the sum of input keys is zero.
	ErrInputKeyZero = errors.New("sum of input keys is zero")
)

// Input describes a UTXO that should be spent in order to pay to one or
// multiple silent addresses.
type Input struct {
	// OutPoint is the outpoint of the UTXO.
	OutPoint wire.OutPoint

	// Utxo is script and amount of the UTXO.
	Utxo wire.TxOut

	// PrivKey is the private key of the input.
	// TODO(guggero): Find a way to do this in a remote signer setup where
	// we don't have access to the raw private key. We could restrict the
	// number of inputs to a single one, then we can do ECDH directly? Or
	// is there a PSBT protocol for this?
	PrivKey btcec.PrivateKey

	// SkipInput must be set to true if the input should be skipped because
	// it meets one of the following conditions:
	// - It is a P2TR input with a NUMS internal key.
	// - It is a P2WPKH input with an uncompressed public key.
	// - It is a P2SH (nested P2WPKH) input with an uncompressed public key.
	SkipInput bool
}

// CreateOutputs creates the outputs for a silent payment transaction. It
// returns the public keys of the outputs that should be used to create the
// transaction.
func CreateOutputs(inputs []Input,
	recipients []Address) ([]OutputWithAddress, error) {

	if len(inputs) == 0 {
		return nil, fmt.Errorf("no inputs provided")
	}

	// We first sum up all the private keys of the inputs.
	//
	// Spec: Let a = a1 + a2 + ... + a_n, where each a_i has been negated if
	// necessary.
	var sumKey = new(btcec.ModNScalar)
	for idx, input := range inputs {
		a := &input.PrivKey

		// If we should skip the input because it is a P2TR input with a
		// NUMS internal key, we can just continue here.
		if input.SkipInput {
			continue
		}

		ok, script, err := InputCompatible(input.Utxo.PkScript)
		if err != nil {
			return nil, fmt.Errorf("unable check input %d for "+
				"silent payment transaction compatibility: %w",
				idx, err)
		}

		if !ok {
			return nil, fmt.Errorf("input %d (%v) is not "+
				"compatible with silent payment transactions",
				idx, script.Class().String())
		}

		// For P2TR we need to take the even key.
		if script.Class() == txscript.WitnessV1TaprootTy {
			pubKeyBytes := a.PubKey().SerializeCompressed()
			if pubKeyBytes[0] == secp.PubKeyFormatCompressedOdd {
				a.Key.Negate()
			}
		}

		sumKey = sumKey.Add(&a.Key)
	}

	// Spec: If a = 0, fail.
	if sumKey.IsZero() {
		return nil, ErrInputKeyZero
	}
	sumPrivKey := btcec.PrivKeyFromScalar(sumKey)

	// Now we need to choose the smallest outpoint lexicographically. We can
	// do that by sorting the inputs.
	//
	// Spec: Let input_hash = hashBIP0352/Inputs(outpointL || A), where
	// outpointL is the smallest outpoint lexicographically used in the
	// transaction and A = a·G
	sort.Sort(sortableInputSlice(inputs))
	input := inputs[0]

	var inputPayload bytes.Buffer
	err := wire.WriteOutPoint(&inputPayload, 0, 0, &input.OutPoint)
	if err != nil {
		return nil, err
	}
	_, err = inputPayload.Write(sumPrivKey.PubKey().SerializeCompressed())
	if err != nil {
		return nil, err
	}
	inputHash := chainhash.TaggedHash(
		TagBIP0352Inputs, inputPayload.Bytes(),
	)

	// Create a copy of the sum key and tweak it with the input hash.
	//
	// Spec: Let ecdh_shared_secret = input_hash·a·B_scan.
	tweakedSumKey := *sumKey
	var tweakScalar btcec.ModNScalar
	tweakScalar.SetBytes((*[32]byte)(inputHash))
	tweakedSumKey = *(tweakedSumKey.Mul(&tweakScalar))

	// Spec: For each B_m in the group:
	results := make([]OutputWithAddress, 0, len(recipients))
	for _, recipients := range GroupByScanKey(recipients) {
		// We grouped by scan key before, so we can just take the first
		// one.
		scanPubKey := recipients[0].ScanKey

		for idx, recipient := range recipients {
			recipientSendKey := recipient.TweakedSpendKey()

			// TweakedSumKey is only input_hash·a, so we need to
			// multiply it by B_scan.
			//
			// Spec: Let ecdh_shared_secret = input_hash·a·B_scan.
			var scanKey, sharedSecret btcec.JacobianPoint
			scanPubKey.AsJacobian(&scanKey)
			btcec.ScalarMultNonConst(
				&tweakedSumKey, &scanKey, &sharedSecret,
			)
			sharedSecret.ToAffine()

			// Spec: Let tk = hashBIP0352/SharedSecret(
			//         serP(ecdh_shared_secret) || ser32(k))
			sharedSecretBytes := btcec.NewPublicKey(
				&sharedSecret.X, &sharedSecret.Y,
			).SerializeCompressed()

			outputPayload := make([]byte, pubKeyLength+4)
			copy(outputPayload[:], sharedSecretBytes)

			k := uint32(idx)
			binary.BigEndian.PutUint32(
				outputPayload[pubKeyLength:], k,
			)

			t := chainhash.TaggedHash(
				TagBIP0352SharedSecret, outputPayload,
			)

			var tScalar btcec.ModNScalar
			overflow := tScalar.SetBytes((*[32]byte)(t))

			// Spec: If tk is not valid tweak, i.e., if tk = 0 or tk
			// is larger or equal to the secp256k1 group order,
			// fail.
			if overflow == 1 {
				return nil, fmt.Errorf("tagged hash overflow")
			}
			if tScalar.IsZero() {
				return nil, fmt.Errorf("tagged hash is zero")
			}

			// Spec: Let Pmn = Bm + tk·G
			var sharedKey, sendKey btcec.JacobianPoint
			recipientSendKey.AsJacobian(&sendKey)
			btcec.ScalarBaseMultNonConst(&tScalar, &sharedKey)
			btcec.AddNonConst(&sendKey, &sharedKey, &sharedKey)
			sharedKey.ToAffine()

			results = append(results, OutputWithAddress{
				Address:   recipient,
				OutputKey: btcec.NewPublicKey(
					&sharedKey.X, &sharedKey.Y,
				),
			})
		}
	}

	return results, nil
}

// InputCompatible checks if a given pkScript is compatible with the silent
// payment protocol.
func InputCompatible(pkScript []byte) (bool, txscript.PkScript, error) {
	script, err := txscript.ParsePkScript(pkScript)
	if err != nil {
		return false, txscript.PkScript{}, fmt.Errorf("error parsing "+
			"pkScript: %w", err)
	}

	switch script.Class() {
	case txscript.PubKeyHashTy, txscript.WitnessV0PubKeyHashTy,
		txscript.WitnessV1TaprootTy:

		// These types are supported in any case.
		return true, script, nil

	case txscript.ScriptHashTy:
		// Only P2SH-P2WPKH is supported. Do we need further checks?
		// Or do we just assume Nested P2WPKH is the only active use
		// case of P2SH these days?
		return true, script, nil

	default:
		return false, script, nil
	}
}

// serializeOutpoint serializes an outpoint to a byte slice.
func serializeOutpoint(outpoint wire.OutPoint) []byte {
	var buf bytes.Buffer
	_ = wire.WriteOutPoint(&buf, 0, 0, &outpoint)
	return buf.Bytes()
}

// sortableInputSlice is a slice of inputs that can be sorted lexicographically.
type sortableInputSlice []Input

// Len returns the number of inputs in the slice.
func (s sortableInputSlice) Len() int { return len(s) }

// Swap swaps the inputs at the passed indices.
func (s sortableInputSlice) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Less returns whether the input at index i should be sorted before the input
// at index j lexicographically.
func (s sortableInputSlice) Less(i, j int) bool {
	// Input hashes are the same, so compare the index.
	iBytes := serializeOutpoint(s[i].OutPoint)
	jBytes := serializeOutpoint(s[j].OutPoint)

	return bytes.Compare(iBytes[:], jBytes[:]) == -1
}

func receiverPermutations(arr []Address) [][]Address {
	var helper func([]Address, int)
	var res [][]Address

	helper = func(arr []Address, n int) {
		if n == 1 {
			tmp := make([]Address, len(arr))
			copy(tmp, arr)
			res = append(res, tmp)
		} else {
			for i := 0; i < n; i++ {
				helper(arr, n-1)
				if n%2 == 1 {
					tmp := arr[i]
					arr[i] = arr[n-1]
					arr[n-1] = tmp
				} else {
					tmp := arr[0]
					arr[0] = arr[n-1]
					arr[n-1] = tmp
				}
			}
		}
	}
	helper(arr, len(arr))
	return res
}
