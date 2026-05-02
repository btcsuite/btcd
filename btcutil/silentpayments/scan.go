package silentpayments

import (
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/btcutil/gcs"
	"github.com/btcsuite/btcd/btcutil/gcs/builder"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/btcsuite/btcd/txscript"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btclog"
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

	result := make([]*btcec.PublicKey, 0, len(addresses))
	for _, recipient := range addresses {
		outputKey, err := outputKeyForTxTweak(
			txTweak, k, recipient.ScanPrivateKey,
			recipient.LabelTweakedSpendKey,
		)
		if err != nil {
			return nil, fmt.Errorf("error generating output key: "+
				"%w", err)
		}

		result = append(result, outputKey)
	}

	return result, nil
}

// outputKeyForTxTweak derives the output key for a given transaction tweak,
// output index k, scan private key, and label-tweaked spend public key.
func outputKeyForTxTweak(txTweak btcec.PublicKey, k uint32,
	scanPrivateKey btcec.PrivateKey,
	labelTweakedSpendPubKey btcec.PublicKey) (*btcec.PublicKey, error) {

	// The txTweak is only input_hash·A, so we need to multiply by b_scan
	// to get the ecdh_shared_secret.
	//
	// Spec: Let ecdh_shared_secret = input_hash·A·b_scan.
	sharedSecret := ScalarMult(scanPrivateKey.Key, &txTweak)

	return outputKey(*sharedSecret, k, labelTweakedSpendPubKey)
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
