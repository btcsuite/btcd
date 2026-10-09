package silentpayments

import (
	"encoding/binary"
	"fmt"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/chainhash/v2"
)

const (
	// MaxRecipientsPerGroup is the maximum number of silent payment
	// recipients (addresses sharing the same scan key) that may be grouped
	// together. Sending to a larger group must fail; the limit bounds the
	// receiver's scanning complexity and corresponds to the maximum number
	// of P2TR outputs that fit into a 100kvB transaction.
	//
	// Spec: K_max = 2323.
	MaxRecipientsPerGroup = 2323
)

// OutputWithAddress is a struct that holds the generated shared public key and
// silent payment address of an output.
type OutputWithAddress struct {
	// Address is the address of the output.
	Address Address

	// OutputKey is the generated shared public key for the given address.
	OutputKey *btcec.PublicKey
}

// AddressOutputKeys generates the actual on-chain output keys for the given
// addresses. The addresses need to be ordered by output index as they appear
// in the transaction.
func AddressOutputKeys(recipients []Address, sumKey btcec.ModNScalar,
	inputHash btcec.ModNScalar) ([]OutputWithAddress, error) {

	// Spec: For each B_m in the group:
	results := make([]OutputWithAddress, 0, len(recipients))
	for _, recipients := range GroupByScanKey(recipients) {
		// Spec: If any of the groups exceed the limit of K_max (=2323)
		// silent payment addresses, fail. This bounds the receiver's
		// scanning complexity for adversarial transactions.
		if len(recipients) > MaxRecipientsPerGroup {
			return nil, ErrTooManyRecipientsPerGroup
		}

		// We grouped by scan key before, so we can just take the first
		// one.
		scanPubKey := recipients[0].ScanKey

		for idx, recipient := range recipients {
			recipientSpendKey := recipient.TweakedSpendKey()

			shareSum := ScalarMult(sumKey, &scanPubKey)
			sharedKey, err := CreateOutputKey(
				*shareSum, *recipientSpendKey, uint32(idx),
				inputHash,
			)
			if err != nil {
				return nil, err
			}

			results = append(results, OutputWithAddress{
				Address:   recipient,
				OutputKey: sharedKey,
			})
		}
	}

	return results, nil
}

// CreateOutputKey creates a shared public key for the given share sum key,
// spend key, index and input hash.
func CreateOutputKey(shareSum, spendKey btcec.PublicKey, idx uint32,
	inputHash btcec.ModNScalar) (*btcec.PublicKey, error) {

	// The shareSum key is only a·B_scan, so we need to multiply it by
	// input_hash.
	//
	// Spec: Let ecdh_shared_secret = input_hash·a·B_scan.
	sharedSecret := ScalarMult(inputHash, &shareSum)

	return outputKey(*sharedSecret, idx, spendKey)
}

// outputKey derives the output key for a given shared secret, output index k,
// and label-tweaked spend public key.
func outputKey(sharedSecret btcec.PublicKey, k uint32,
	labelTweakedSpendPubKey btcec.PublicKey) (*btcec.PublicKey, error) {

	// Spec: Let t_k = hashBIP0352/SharedSecret(
	//    serP(ecdh_shared_secret) || ser32(k)
	// )
	outputPayload := make([]byte, pubKeyLength+4)
	copy(outputPayload[:], sharedSecret.SerializeCompressed())

	binary.BigEndian.PutUint32(outputPayload[pubKeyLength:], k)

	t := chainhash.TaggedHash(TagBIP0352SharedSecret, outputPayload)

	var tScalar btcec.ModNScalar
	overflow := tScalar.SetBytes((*[32]byte)(t))

	// Spec: If t_k is not a valid tweak, i.e., if t_k = 0 or t_k is larger
	// or equal to the secp256k1 group order, fail.
	if overflow == 1 {
		return nil, fmt.Errorf("tagged hash overflow")
	}
	if tScalar.IsZero() {
		return nil, fmt.Errorf("tagged hash is zero")
	}

	// Spec: Let P_mn = B_m + t_k·G
	return ScalarBaseMultAdd(tScalar, &labelTweakedSpendPubKey), nil
}
