package util

import "github.com/btcsuite/btcd/btcec/v2"

func PubKeyIdx(pubKey *btcec.PublicKey, pubKeys []*btcec.PublicKey) (
	int, bool) {

	for i, pk := range pubKeys {
		if pk.IsEqual(pubKey) {
			return i, true
		}
	}

	return -1, false
}
