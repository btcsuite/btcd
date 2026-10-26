package threshold

import "github.com/btcsuite/btcd/btcec/v2"

type DKGOutput struct {
	SecShare        *btcec.PrivateKey
	ThresholdPubKey *btcec.PublicKey
	PubShares       []*btcec.PublicKey
}
