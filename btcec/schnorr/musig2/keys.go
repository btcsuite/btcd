// Copyright 2013-2022 The btcsuite developers

package musig2

import (
	"bytes"
	"sort"

	"github.com/btcsuite/btcd/btcec/v2"
	"github.com/btcsuite/btcd/btcec/v2/schnorr"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
)

var (
	// KeyAggTagList is the tagged hash tag used to compute the hash of the
	// list of sorted public keys.
	KeyAggTagList = []byte("KeyAgg list")

	// KeyAggTagCoeff is the tagged hash tag used to compute the key
	// aggregation coefficient for each key.
	KeyAggTagCoeff = []byte("KeyAgg coefficient")
)

// sortableKeys defines a type of slice of public keys that implements the sort
// interface for BIP 340 keys.
type sortableKeys []*btcec.PublicKey

// Less reports whether the element with index i must sort before the element
// with index j.
func (s sortableKeys) Less(i, j int) bool {
	keyIBytes := schnorr.SerializePubKey(s[i])
	keyJBytes := schnorr.SerializePubKey(s[j])

	return bytes.Compare(keyIBytes, keyJBytes) == -1
}

// Swap swaps the elements with indexes i and j.
func (s sortableKeys) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

// Len is the number of elements in the collection.
func (s sortableKeys) Len() int {
	return len(s)
}

// sortKeys takes a set of schnorr public keys and returns a new slice that is
// a copy of the keys sorted in lexicographical order bytes on the x-only
// pubkey serialization.
func sortKeys(keys []*btcec.PublicKey) []*btcec.PublicKey {
	keySet := sortableKeys(keys)
	if sort.IsSorted(keySet) {
		return keys
	}

	sort.Sort(keySet)
	return keySet
}

// keyHashFingerprint computes the tagged hash of the series of (sorted) public
// keys passed as input. This is used to compute the aggregation coefficient
// for each key. The final computation is:
//   * H(tag=KeyAgg list, pk1 || pk2..)
func keyHashFingerprint(keys []*btcec.PublicKey, sort bool) []byte {
	var keyBytes bytes.Buffer

	if sort {
		keys = sortKeys(keys)
	}

	for _, key := range keys {
		keyBytes.Write(schnorr.SerializePubKey(key))
	}

	h := chainhash.TaggedHash(KeyAggTagList, keyBytes.Bytes())
	return h[:]
}

// isSecondKey returns true if the passed public key is the second key in the
// (sorted) keySet passed.
func isSecondKey(keySet []*btcec.PublicKey, targetKey *btcec.PublicKey) bool {
	// For this comparison, we want to compare the raw serialized version
	// instead of the full pubkey, as it's possible we're dealing with a
	// pubkey that _actually_ has an odd y coordinate.
	equalBytes := func(a, b *btcec.PublicKey) bool {
		return bytes.Equal(
			schnorr.SerializePubKey(a),
			schnorr.SerializePubKey(b),
		)
	}

	for i := range keySet {
		if !equalBytes(keySet[i], keySet[0]) {
			return equalBytes(keySet[i], targetKey)
		}
	}

	return false
}

// aggregationCoefficient computes the key aggregation coefficient for the
// specified target key. The coefficient is computed as:
//  * H(tag=KeyAgg coefficient, keyHashFingerprint(pks) || pk)
func aggregationCoefficient(keySet []*btcec.PublicKey,
	targetKey *btcec.PublicKey, sort bool) *btcec.ModNScalar {

	var mu btcec.ModNScalar

	// If this is the second key, then this coefficient is just one.
	//
	// TODO(roasbeef): use intermediate cache to keep track of the second
	// key, can just store an index, otherwise this is O(n^2)
	if isSecondKey(keySet, targetKey) {
		return mu.SetInt(1)
	}

	// Otherwise, we'll compute the full finger print hash for this given
	// key and then use that to compute the coefficient tagged hash:
	//  * H(tag=KeyAgg coefficient, keyHashFingerprint(pks, pk) || pk)
	var coefficientBytes bytes.Buffer
	coefficientBytes.Write(keyHashFingerprint(keySet, sort))
	coefficientBytes.Write(schnorr.SerializePubKey(targetKey))

	muHash := chainhash.TaggedHash(KeyAggTagCoeff, coefficientBytes.Bytes())

	mu.SetByteSlice(muHash[:])

	return &mu
}

// TODO(roasbeef): make proper IsEven func

// AggregateKeys takes a list of possibly unsorted keys and returns a single
// aggregated key as specified by the musig2 key aggregation algorithm.
func AggregateKeys(keys []*btcec.PublicKey, sort bool) *btcec.PublicKey {
	// Sort the set of public key so we know we're working with them in
	// sorted order for all the routines below.
	if sort {
		keys = sortKeys(keys)
	}

	// For each key, we'll compute the intermediate blinded key: a_i*P_i,
	// where a_i is the aggregation coefficient for that key, and P_i is
	// the key itself, then accumulate that (addition) into the main final
	// key: P = P_1 + P_2 ... P_N.
	var finalKeyJ btcec.JacobianPoint
	for _, key := range keys {
		// Port the key over to Jacobian coordinates as we need it in
		// this format for the routines below.
		var keyJ btcec.JacobianPoint
		key.AsJacobian(&keyJ)

		// Compute the aggregation coefficient for the key, then
		// multiply it by the key itself: P_i' = a_i*P_i.
		var tweakedKeyJ btcec.JacobianPoint
		a := aggregationCoefficient(keys, key, sort)
		btcec.ScalarMultNonConst(a, &keyJ, &tweakedKeyJ)

		// Finally accumulate this into the final key in an incremental
		// fashion.
		btcec.AddNonConst(&finalKeyJ, &tweakedKeyJ, &finalKeyJ)
	}

	finalKeyJ.ToAffine()
	return btcec.NewPublicKey(&finalKeyJ.X, &finalKeyJ.Y)
}
