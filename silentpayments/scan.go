package silentpayments

import (
	"encoding/binary"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/v2/gcs"
	"github.com/btcsuite/btcd/btcutil/v2/gcs/builder"
	"github.com/btcsuite/btcd/chainhash/v2"
	"github.com/btcsuite/btcd/txscript/v2"
	"github.com/btcsuite/btcd/wire/v2"
	"github.com/btcsuite/btclog"
	secp "github.com/decred/dcrd/dcrec/secp256k1/v4"
)

// TransactionTweakData computes the Silent Payment tweak data for the given
// transaction. If the transaction does not contain any Taproot outputs, or if
// any of the inputs are unsupported for Silent Payments, a nil public key is
// returned.
func TransactionTweakData(tx *wire.MsgTx, fetcher PrevOutFetcher,
	log btclog.Logger) (*btcec.PublicKey, error) {

	// Only transactions with Taproot outputs can have Silent Payments.
	if !HasTaprootOutputs(tx) {
		return nil, nil
	}

	// We now compute the sum of all input public keys. If there is an
	// unsupported input
	inputSum := SumInputPubKeys(tx, fetcher, log)
	if inputSum == nil {
		return nil, nil
	}

	inputOutPoints := make([]wire.OutPoint, 0, len(tx.TxIn))
	for _, txIn := range tx.TxIn {
		inputOutPoints = append(inputOutPoints, txIn.PreviousOutPoint)
	}

	inputHash, err := CalculateInputHashTweak(inputOutPoints, inputSum)
	if err != nil {
		return nil, fmt.Errorf("error calculating input hash: %w", err)
	}

	// Calculate the actual tweak data for this transaction, which is:
	// tweak = input_hash * A
	var sumJ, tweakJ btcec.JacobianPoint
	inputSum.AsJacobian(&sumJ)
	btcec.ScalarMultNonConst(inputHash, &sumJ, &tweakJ)
	tweakJ.ToAffine()

	return btcec.NewPublicKey(&tweakJ.X, &tweakJ.Y), nil
}

// HasTaprootOutputs returns true if the given transaction has any Taproot
// outputs.
func HasTaprootOutputs(tx *wire.MsgTx) bool {
	for _, txOut := range tx.TxOut {
		if txscript.IsPayToTaproot(txOut.PkScript) {
			return true
		}
	}

	return false
}

// ScanAddress is a Silent Payment address along with the associated scan
// private key and cached label-tweaked spend public key.
type ScanAddress struct {
	// Address is the silent payment address.
	Address Address

	// ScanPrivateKey is the address' scan private key.
	ScanPrivateKey btcec.PrivateKey

	// LabelTweakedSpendKey is the address' label-tweaked spend public key
	// (the spend key with the label tweak applied, if present).
	LabelTweakedSpendKey btcec.PublicKey
}

// NewScanAddress creates a new ScanAddress from the given address and scan
// private key.
func NewScanAddress(addr Address, scanPrivateKey btcec.PrivateKey) ScanAddress {
	return ScanAddress{
		Address:              addr,
		ScanPrivateKey:       scanPrivateKey,
		LabelTweakedSpendKey: *addr.TweakedSpendKey(),
	}
}

// TransactionOutputKeysForFilter generates a list of all possible output keys
// for the given transaction by using the transaction's tweak and list of scan
// addresses. The addresses must include all labels the wallet is tracking,
// including the change label if desired. This function only generates keys for
// output index k = 0 with the intent of using these keys in combination with
// a compact block filter to determine if the transaction contains any outputs
// relevant to the wallet. If any outputs match, the wallet can then proceed to
// derive the actual output keys for all output indices after fetching the full
// block/transaction.
func TransactionOutputKeysForFilter(txTweak btcec.PublicKey,
	addresses []ScanAddress) ([]*btcec.PublicKey, error) {

	// As mentioned in the function comment, we only derive output keys for
	// output index k = 0 here. If an output with index 0 matches the
	// filter, we know the transaction is relevant, and we can derive the
	// rest of the output keys after fetching the full block/transaction.
	const k = 0

	// The ECDH shared secret depends on the scan private key and the
	// transaction tweak only, not on the (label-tweaked) spend key. All
	// addresses of a wallet normally share a single scan key, so we
	// compute the expensive scalar multiplication once and re-use it
	// across addresses, which roughly halves the cost of scanning a
	// transaction for the common base+change address pair.
	var (
		sharedSecret *btcec.PublicKey
		sharedForKey btcec.ModNScalar
	)

	result := make([]*btcec.PublicKey, 0, len(addresses))
	for _, recipient := range addresses {
		scanKey := recipient.ScanPrivateKey.Key
		if sharedSecret == nil || !sharedForKey.Equals(&scanKey) {
			// The txTweak is only input_hash·A, so we need to
			// multiply by b_scan to get the ecdh_shared_secret.
			//
			// Spec: Let ecdh_shared_secret = input_hash·A·b_scan.
			sharedSecret = ScalarMult(scanKey, &txTweak)
			sharedForKey = scanKey
		}

		outputKey, err := outputKey(
			*sharedSecret, k, recipient.LabelTweakedSpendKey,
		)
		if err != nil {
			return nil, fmt.Errorf("error generating output key: "+
				"%w", err)
		}

		result = append(result, outputKey)
	}

	return result, nil
}

// batchToAffine converts a slice of Jacobian points to affine coordinates
// (with normalized values, exactly like JacobianPoint.ToAffine) using a
// single modular inversion for the whole slice via Montgomery's trick,
// instead of one expensive inversion per point. A point at infinity yields
// an error, since no honestly derived scan value can be at infinity.
func batchToAffine(points []btcec.JacobianPoint) error {
	// prefix[i] holds z_0 * ... * z_(i-1), so that the single inverted
	// accumulator can be unrolled back into the individual 1/z_i values.
	prefix := make([]secp.FieldVal, len(points))

	var acc secp.FieldVal
	acc.SetInt(1)
	for i := range points {
		if points[i].Z.Normalize().IsZero() {
			return fmt.Errorf("point %d is at infinity", i)
		}

		prefix[i].Set(&acc)
		acc.Mul(&points[i].Z)
	}

	// One inversion for the whole batch: acc = 1 / (z_0 * ... * z_(n-1)).
	acc.Normalize().Inverse()

	for i := len(points) - 1; i >= 0; i-- {
		// 1/z_i = prefix_i * 1/(z_0*...*z_i); then strip z_i off the
		// accumulator for the next round: acc *= z_i.
		var zInv secp.FieldVal
		zInv.Mul2(&acc, &prefix[i]).Normalize()
		acc.Mul(&points[i].Z).Normalize()

		// Mirror ToAffine: X = X/z^2, Y = Y/z^3, Z = 1, normalized.
		var zInv2, zInv3 secp.FieldVal
		zInv2.SquareVal(&zInv)
		zInv3.Mul2(&zInv2, &zInv).Normalize()
		points[i].X.Mul(&zInv2).Normalize()
		points[i].Y.Mul(&zInv3).Normalize()
		points[i].Z.SetInt(1)
	}

	return nil
}

// TransactionOutputKeysForFilterBatch is the batch variant of
// TransactionOutputKeysForFilter: it derives the k = 0 candidate output
// keys of many transaction tweaks in one call, for a set of scan addresses
// that must all share the same scan private key. The result is a flat
// slice of len(txTweaks) * len(addresses) keys, grouped by tweak (all
// addresses of the first tweak first).
//
// Deriving candidates in batches is significantly cheaper than per tweak:
// the output tweak t_0 only depends on the shared secret, so its point is
// computed once per transaction and the per-address candidates become
// point additions; and all per-point modular inversions (the affine
// conversions of the shared secrets and of the candidate keys) collapse
// into two batched inversions for the entire call.
func TransactionOutputKeysForFilterBatch(addresses []ScanAddress,
	txTweaks []*btcec.PublicKey) ([]*btcec.PublicKey, error) {

	if len(txTweaks) == 0 {
		return nil, nil
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("no scan addresses provided")
	}

	// The whole point of batching is a single ECDH secret per tweak, so
	// all addresses must share the scan key it is derived from.
	scanKey := addresses[0].ScanPrivateKey.Key
	for _, recipient := range addresses[1:] {
		if !scanKey.Equals(&recipient.ScanPrivateKey.Key) {
			return nil, fmt.Errorf("all addresses must share " +
				"the same scan private key")
		}
	}

	// Spec: Let ecdh_shared_secret = input_hash·A·b_scan.
	//
	// The affine conversion of the results is deferred to a single
	// batched inversion below.
	sharedSecrets := make([]btcec.JacobianPoint, len(txTweaks))
	for i, txTweak := range txTweaks {
		var tweakJ btcec.JacobianPoint
		txTweak.AsJacobian(&tweakJ)
		btcec.ScalarMultNonConst(&scanKey, &tweakJ, &sharedSecrets[i])
	}
	if err := batchToAffine(sharedSecrets); err != nil {
		return nil, fmt.Errorf("shared secret: %w", err)
	}

	spendKeys := make([]btcec.JacobianPoint, len(addresses))
	for i, recipient := range addresses {
		recipient.LabelTweakedSpendKey.AsJacobian(&spendKeys[i])
	}

	const k = 0
	outputPayload := make([]byte, pubKeyLength+4)
	binary.BigEndian.PutUint32(outputPayload[pubKeyLength:], k)

	candidates := make(
		[]btcec.JacobianPoint, 0, len(txTweaks)*len(addresses),
	)
	for i := range sharedSecrets {
		// Serialize the (now affine) shared secret in compressed form
		// for the tagged hash.
		//
		// Spec: Let t_k = hashBIP0352/SharedSecret(
		//    serP(ecdh_shared_secret) || ser32(k)
		// )
		outputPayload[0] = secp.PubKeyFormatCompressedEven
		if sharedSecrets[i].Y.IsOdd() {
			outputPayload[0] = secp.PubKeyFormatCompressedOdd
		}
		sharedSecrets[i].X.PutBytesUnchecked(outputPayload[1:33])

		t := chainhash.TaggedHash(
			TagBIP0352SharedSecret, outputPayload,
		)

		var tScalar btcec.ModNScalar
		overflow := tScalar.SetBytes((*[32]byte)(t))

		// Spec: If t_k is not a valid tweak, i.e., if t_k = 0 or t_k
		// is larger or equal to the secp256k1 group order, fail.
		if overflow == 1 {
			return nil, fmt.Errorf("tagged hash overflow")
		}
		if tScalar.IsZero() {
			return nil, fmt.Errorf("tagged hash is zero")
		}

		// The tweak point t_0*G doesn't depend on the spend key, so
		// one base multiplication serves all addresses.
		var tweakPoint btcec.JacobianPoint
		btcec.ScalarBaseMultNonConst(&tScalar, &tweakPoint)

		// Spec: Let P_mn = B_m + t_k·G
		for j := range spendKeys {
			var candidate btcec.JacobianPoint
			btcec.AddNonConst(
				&tweakPoint, &spendKeys[j], &candidate,
			)
			candidates = append(candidates, candidate)
		}
	}

	// A candidate at infinity (spend key equal to the negated tweak
	// point, impossible for honestly derived keys) surfaces as an error
	// here, matching the per-tweak variant.
	if err := batchToAffine(candidates); err != nil {
		return nil, fmt.Errorf("output key: %w", err)
	}

	result := make([]*btcec.PublicKey, len(candidates))
	for i := range candidates {
		result[i] = btcec.NewPublicKey(
			&candidates[i].X, &candidates[i].Y,
		)
	}

	return result, nil
}

// MatchBlock matches the given list of Taproot output keys against the given
// compact block filter. This function expects all Silent Payment outputs to be
// derived/constructed for all eligible transactions in the block and for all
// labels the wallet is aware of. If any of the output keys match the filter,
// the function returns true.
func MatchBlock(blockFilter *gcs.Filter, blockHash *chainhash.Hash,
	trOutputKeys []*btcec.PublicKey) (bool, error) {

	// If the block filter was constructed with no data, we can shortcut
	// the matching process.
	if blockFilter.N() == 0 {
		return false, nil
	}

	watchList := make([][]byte, len(trOutputKeys))
	for idx, key := range trOutputKeys {
		watchList[idx] = append(
			[]byte{txscript.OP_1, txscript.OP_DATA_32},
			schnorr.SerializePubKey(key)...,
		)
	}

	key := builder.DeriveKey(blockHash)
	matched, err := blockFilter.MatchAny(key, watchList)
	if err != nil {
		return false, err
	}

	return matched, nil
}
