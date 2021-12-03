module github.com/btcsuite/btcd/btcec/v2

go 1.17

require (
	github.com/btcsuite/btcd v0.22.0-beta.0.20220111032746-97732e52810c
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1
)

require github.com/decred/dcrd/crypto/blake256 v1.0.0 // indirect

// We depend on chainhash as is, so we need to replace to use the version of
// chainhash included in the version of btcd we're building in.
replace github.com/btcsuite/btcd => ../
