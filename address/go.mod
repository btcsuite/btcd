module github.com/btcsuite/btcd/address/v2

go 1.23.2

require (
	github.com/btcsuite/btcd/btcec/v2 v2.3.2
	github.com/btcsuite/btcd/chaincfg/v2 v2.0.0
	github.com/btcsuite/btcd/wire/v2 v2.0.0
	golang.org/x/crypto v0.40.0
)

require (
	github.com/btcsuite/btcd/chainhash/v2 v2.0.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	golang.org/x/sys v0.35.0 // indirect
)

// TODO(guggero): Remove this as soon as we have a tagged version of btcec.
replace github.com/btcsuite/btcd/btcec/v2 => ../btcec

// TODO(guggero): Remove this as soon as we have a tagged version of chaincfg.
replace github.com/btcsuite/btcd/chaincfg/v2 => ../chaincfg

// TODO(guggero): Remove this as soon as we have a tagged version of chainhash.
replace github.com/btcsuite/btcd/chainhash/v2 => ../chainhash

// TODO(guggero): Remove this as soon as we have a tagged version of wire.
replace github.com/btcsuite/btcd/wire/v2 => ../wire
