package silentpayments

import (
	"bytes"
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
// transaction. Recipients must be ordered by output index.
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

	inputOutpoints := make([]wire.OutPoint, 0, len(inputs))
	for _, input := range inputs {
		inputOutpoints = append(inputOutpoints, input.OutPoint)
	}

	// Create the tweak that will be used to tweak the share sum key to
	// arrive at the final shared secret.
	//
	// Spec: Let ecdh_shared_secret = input_hash·a·B_scan.
	inputHash, err := CalculateInputHashTweak(
		inputOutpoints, sumPrivKey.PubKey(),
	)
	if err != nil {
		return nil, err
	}

	return AddressOutputKeys(recipients, *sumKey, *inputHash)
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

// CalculateInputHashTweak calculates the input hash from the given input
// outpoint list and the sum public key A. The tweak can be used to tweak the
// share sum key to arrive at the final shared secret.
func CalculateInputHashTweak(inputs []wire.OutPoint,
	A *btcec.PublicKey) (*btcec.ModNScalar, error) {

	// Now we need to choose the smallest outpoint lexicographically. We can
	// do that by sorting the input outpoints.
	//
	// Spec: Let input_hash = hashBIP0352/Inputs(outpointL || A), where
	// outpointL is the smallest outpoint lexicographically used in the
	// transaction and A = a·G.
	sort.Slice(inputs, func(i, j int) bool {
		iBytes := serializeOutpoint(inputs[i])
		jBytes := serializeOutpoint(inputs[j])

		return bytes.Compare(iBytes[:], jBytes[:]) == -1
	})
	firstInput := inputs[0]

	var inputPayload bytes.Buffer
	err := wire.WriteOutPoint(&inputPayload, 0, 0, &firstInput)
	if err != nil {
		return nil, err
	}
	_, err = inputPayload.Write(A.SerializeCompressed())
	if err != nil {
		return nil, err
	}
	inputHash := chainhash.TaggedHash(
		TagBIP0352Inputs, inputPayload.Bytes(),
	)

	var inputHashScalar btcec.ModNScalar
	inputHashScalar.SetBytes((*[32]byte)(inputHash))

	return &inputHashScalar, nil
}

// serializeOutpoint serializes an outpoint to a byte slice.
func serializeOutpoint(outpoint wire.OutPoint) []byte {
	var buf bytes.Buffer
	_ = wire.WriteOutPoint(&buf, 0, 0, &outpoint)
	return buf.Bytes()
}
