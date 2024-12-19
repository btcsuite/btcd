package silentpayments

import "github.com/btcsuite/btcd/btcec/v2"

type OutputWithAddress struct {
	// Address is the address of the output.
	Address Address
	
	// OutputKey is the generated shared public key for the given address.
	OutputKey *btcec.PublicKey
}