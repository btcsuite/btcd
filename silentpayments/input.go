package silentpayments

import (
	"bytes"
	"errors"
	"fmt"
	"slices"

	"github.com/btcsuite/btcd/address/v2"
	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

var (
	// TagBIP0352Inputs is the BIP-0352 tag for a inputs.
	TagBIP0352Inputs = []byte("BIP0352/Inputs")

	// TagBIP0352SharedSecret is the BIP-0352 tag for a shared secret.
	TagBIP0352SharedSecret = []byte("BIP0352/SharedSecret")

	// ErrNoInputs is returned if the number of compatible inputs provided
	// is zero.
	ErrNoInputs = errors.New("no inputs provided")

	// ErrInputKeyZero is returned if the sum of input keys is zero.
	ErrInputKeyZero = errors.New("sum of input keys is zero")

	// ErrSegWitVersionTooHigh is returned if an input spends an output
	// with a SegWit version greater than 1. Receivers skip any transaction
	// that spends such an output, so a silent payment can't be created.
	ErrSegWitVersionTooHigh = errors.New(
		"input with SegWit version > 1 not allowed",
	)

	// ErrInputKeyMismatch is returned if the private key of an input
	// doesn't match the public key committed to in the input's pkScript.
	ErrInputKeyMismatch = errors.New(
		"input private key doesn't match pkScript",
	)

	// ErrTooManyRecipientsPerGroup is returned if more than
	// MaxRecipientsPerGroup silent payment recipients share the same scan
	// key, in which case sending must fail.
	ErrTooManyRecipientsPerGroup = errors.New(
		"too many recipients sharing the same scan key",
	)
)

// Input describes a UTXO that should be spent in order to pay to one or
// multiple silent addresses.
type Input struct {
	// OutPoint is the outpoint of the UTXO.
	OutPoint wire.OutPoint

	// Utxo is script and amount of the UTXO.
	Utxo wire.TxOut

	// PrivKey is the private key of the input.
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
// transaction. The outputs are grouped by scan key, so callers must place them
// in the transaction by their OutputWithAddress.Address, not by their position
// in the returned slice (see AddressOutputKeys). This function can only be
// used if the private key is known by the component that creates the recipient
// address(es). In a setup where the private key is only known to an
// encapsulated signing component (e.g. a remote signing setup), the PSBT with
// DLEQ proof approach must be used.
func CreateOutputs(inputs []Input,
	recipients []Address) ([]OutputWithAddress, error) {

	if len(inputs) == 0 {
		return nil, ErrNoInputs
	}

	// We first sum up all the private keys of the inputs.
	//
	// Spec: Let a = a1 + a2 + ... + a_n, where each a_i has been negated if
	// necessary.
	var (
		sumKey              = new(btcec.ModNScalar)
		numCompatibleInputs uint
	)
	for idx, input := range inputs {
		a := &input.PrivKey

		// Receivers skip any transaction that spends a SegWit v2+
		// output, so we must check this for every input, including
		// those that are skipped for the shared secret derivation.
		//
		// Spec: Exclude inputs with SegWit version > 1.
		if err := checkSegWitVersion(input.Utxo.PkScript); err != nil {
			return nil, fmt.Errorf("input %d: %w", idx, err)
		}

		// If we should skip the input because it is a P2TR input with a
		// NUMS internal key, we can just continue here.
		if input.SkipInput {
			continue
		}

		compatible, script, err := InputCompatible(
			input.Utxo.PkScript, input.PrivKey.PubKey(),
		)
		if err != nil {
			return nil, fmt.Errorf("unable check input %d for "+
				"silent payment transaction compatibility: %w",
				idx, err)
		}

		// If an input is not supported, the BIP mentions that it should
		// be skipped. But in the end, at least one compatible input
		// must be present.
		if !compatible {
			continue
		}

		// For P2TR we need to take the even key.
		if script.Class() == txscript.WitnessV1TaprootTy {
			pubKeyBytes := a.PubKey().SerializeCompressed()
			if pubKeyBytes[0] == secp.PubKeyFormatCompressedOdd {
				a.Key.Negate()
			}
		}

		numCompatibleInputs++
		sumKey = sumKey.Add(&a.Key)
	}

	// Spec: At least one input MUST be from the _Inputs For Shared Secret
	// Derivation_ list.
	if numCompatibleInputs == 0 {
		return nil, ErrNoInputs
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
//
// NOTE: This only classifies the input by its script type and the public key
// committed in the script. It does NOT inspect the spending witness/scriptSig,
// so the BIP-0352 conditions that disqualify an otherwise-eligible input based
// on witness data (a P2TR input with a NUMS internal key, or a P2WPKH /
// P2SH-P2WPKH input with an uncompressed public key) must be detected by the
// caller and signalled via Input.SkipInput.
//
// For P2PKH, P2WPKH and P2TR inputs, ErrInputKeyMismatch is returned if the
// given public key doesn't match the one committed to in the pkScript, as the
// sender must hold the private key of each input used for the shared secret.
// A P2SH input that doesn't match the given key is reported as not compatible
// instead, since it can't be told apart from a P2SH script that isn't a nested
// P2WPKH (and therefore isn't eligible) from the pkScript alone.
//
// ErrSegWitVersionTooHigh is returned for an input that spends a SegWit v2+
// output, as receivers skip any transaction that spends one. Any other script
// that isn't in the BIP-0352 list of inputs for shared secret derivation is
// reported as not compatible without an error, as it may still be used to fund
// the transaction.
func InputCompatible(pkScript []byte,
	pubKey *btcec.PublicKey) (bool, txscript.PkScript, error) {

	if err := checkSegWitVersion(pkScript); err != nil {
		return false, txscript.PkScript{}, err
	}

	// Any UTXO with a known output script can be used to fund the
	// transaction, only the ones that aren't in the list of inputs for
	// shared secret derivation are left out of it. So a script we can't
	// parse (for example P2PK or bare multisig) isn't an error, it just
	// isn't compatible.
	script, err := txscript.ParsePkScript(pkScript)
	if err != nil {
		return false, txscript.PkScript{}, nil
	}

	pubKeyHash := address.Hash160(pubKey.SerializeCompressed())

	switch script.Class() {
	// P2PKH and P2WPKH are supported for compressed public keys only.
	case txscript.PubKeyHashTy, txscript.WitnessV0PubKeyHashTy:
		if !bytes.Contains(pkScript, pubKeyHash) {
			return false, script, ErrInputKeyMismatch
		}

		return true, script, nil

	// P2TR is supported, as long as the internal key isn't the NUMS key.
	// But we can't detect that without knowing the full witness, so we have
	// to rely on the caller to check that beforehand.
	//
	// Spec: For each taproot output spent the sending wallet MUST have
	// access to the private key corresponding to the taproot output key,
	// unless H is used as the internal public key.
	case txscript.WitnessV1TaprootTy:
		// The pkScript is OP_1 OP_DATA_32 <32-byte x-only output key>.
		outputKey := pkScript[2:]
		if !bytes.Equal(schnorr.SerializePubKey(pubKey), outputKey) {
			return false, script, ErrInputKeyMismatch
		}

		return true, script, nil

	// Only Nested P2WPKH (P2SH-P2WPKH) inputs are allowed.
	case txscript.ScriptHashTy:
		witnessScript, err := txscript.NewScriptBuilder().
			AddOp(txscript.OP_0).
			AddData(pubKeyHash).
			Script()
		if err != nil {
			return false, txscript.PkScript{}, fmt.Errorf("error "+
				"building witness script: %w", err)
		}

		hash := address.Hash160(witnessScript)
		return bytes.Contains(pkScript, hash), script, nil

	default:
		return false, script, nil
	}
}

// checkSegWitVersion returns ErrSegWitVersionTooHigh if the given pkScript is a
// witness program with a version greater than 1. Scripts that aren't witness
// programs (for example a bare OP_2 without a valid program push) pass.
func checkSegWitVersion(pkScript []byte) error {
	if !txscript.IsWitnessProgram(pkScript) {
		return nil
	}

	version, _, err := txscript.ExtractWitnessProgramInfo(pkScript)
	if err != nil {
		return err
	}

	if version > 1 {
		return ErrSegWitVersionTooHigh
	}

	return nil
}

// CalculateInputHashTweak calculates the input hash from the given input
// outpoint list and the sum public key A. The tweak can be used to tweak the
// share sum key to arrive at the final shared secret.
func CalculateInputHashTweak(inputs []wire.OutPoint,
	A *btcec.PublicKey) (*btcec.ModNScalar, error) {

	if len(inputs) == 0 {
		return nil, ErrNoInputs
	}

	// Now we need to choose the smallest outpoint lexicographically. We
	// don't sort the inputs, so the caller's slice isn't modified.
	//
	// Spec: Let input_hash = hashBIP0352/Inputs(outpointL || A), where
	// outpointL is the smallest outpoint lexicographically used in the
	// transaction and A = a·G.
	firstInput := slices.MinFunc(inputs, func(a, b wire.OutPoint) int {
		return bytes.Compare(serializeOutpoint(a), serializeOutpoint(b))
	})

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
	overflow := inputHashScalar.SetBytes((*[32]byte)(inputHash))

	// Spec: Fail if input_hash is 0 or is larger than or equal to the
	// secp256k1 group order. This mirrors the t_k validity check in
	// outputKey and is practically unreachable for a tagged hash.
	if overflow == 1 {
		return nil, fmt.Errorf("input hash overflow")
	}
	if inputHashScalar.IsZero() {
		return nil, fmt.Errorf("input hash is zero")
	}

	return &inputHashScalar, nil
}

// serializeOutpoint serializes an outpoint to a byte slice.
func serializeOutpoint(outpoint wire.OutPoint) []byte {
	var buf bytes.Buffer
	_ = wire.WriteOutPoint(&buf, 0, 0, &outpoint)
	return buf.Bytes()
}
