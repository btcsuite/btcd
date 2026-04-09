module github.com/btcsuite/btcd/bip322

go 1.25.0

require (
	github.com/btcsuite/btcd/address/v2 v2.0.0
	github.com/btcsuite/btcd/chaincfg/v2 v2.0.0
	github.com/btcsuite/btcd/chainhash/v2 v2.0.0
	github.com/btcsuite/btcd/psbt/v2 v2.0.0
	github.com/btcsuite/btcd/txscript/v2 v2.0.0
	github.com/btcsuite/btcd/wire/v2 v2.0.0
	github.com/stretchr/testify v1.11.1
)

require (
	github.com/btcsuite/btcd/btcec/v2 v2.5.0 // indirect
	github.com/btcsuite/btcd/btcutil/v2 v2.0.0 // indirect
	github.com/btcsuite/btclog v1.0.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/kcalvinalvin/anet v0.0.0-20251112173137-d8ddc1f6dbee // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	golang.org/x/crypto v0.51.0 // indirect
	golang.org/x/sys v0.44.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// TODO(guggero): Remove this once the BIP-322 PR is merged.
replace (
	github.com/btcsuite/btcd/psbt/v2 => ../psbt
	github.com/btcsuite/btcd/txscript/v2 => ../txscript
)
