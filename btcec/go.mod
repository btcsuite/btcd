module github.com/btcsuite/btcd/btcec/v2

go 1.23.2

require (
	github.com/btcsuite/btcd/chainhash/v2 v2.0.0
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0
	github.com/stretchr/testify v1.10.0
)

require (
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// TODO(guggero): Remove once we have a tagged version of the chainhash package.
replace github.com/btcsuite/btcd/chainhash/v2 => ../chainhash
