module github.com/dashevo/dashd-go/btcec/v2

go 1.17

require (
	github.com/dashevo/dashd-go v0.0.0-00010101000000-000000000000
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1
)

require github.com/decred/dcrd/crypto/blake256 v1.0.0 // indirect

// We depend on chainhash as is, so we need to replace to use the version of
// chainhash included in the version of btcd we're building in.
replace github.com/dashevo/dashd-go => ../
