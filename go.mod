module github.com/btcsuite/btcd

go 1.23.2

require (
	github.com/btcsuite/btcd/address/v2 v2.0.0
	github.com/btcsuite/btcd/btcec/v2 v2.3.5
	github.com/btcsuite/btcd/btcutil/v2 v2.0.0
	github.com/btcsuite/btcd/chaincfg/v2 v2.0.0
	github.com/btcsuite/btcd/chainhash/v2 v2.0.0
	github.com/btcsuite/btcd/txscript/v2 v2.0.0
	github.com/btcsuite/btcd/v2transport v1.0.1
	github.com/btcsuite/btcd/wire/v2 v2.0.0
	github.com/btcsuite/btclog v1.0.0
	github.com/btcsuite/go-socks v0.0.0-20170105172521-4720035b7bfd
	github.com/btcsuite/websocket v0.0.0-20150119174127-31079b680792
	github.com/btcsuite/winsvc v1.0.0
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/lru v1.1.3
	github.com/gorilla/websocket v1.5.3
	github.com/jessevdk/go-flags v1.6.1
	github.com/jrick/logrotate v1.1.2
	github.com/stretchr/testify v1.10.0
	github.com/syndtr/goleveldb v1.0.1-0.20210819022825-2ae1ddf74ef7
	golang.org/x/crypto v0.40.0
	golang.org/x/sys v0.35.0
	pgregory.net/rapid v1.2.0
)

require (
	github.com/aead/siphash v1.0.1 // indirect
	github.com/decred/dcrd/crypto/blake256 v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.4.0 // indirect
	github.com/golang/snappy v1.0.0 // indirect
	github.com/kkdai/bstream v1.0.0 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/stretchr/objx v0.5.2 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

// TODO(guggero): Remove this as soon as we have a tagged version of address.
replace github.com/btcsuite/btcd/address/v2 => ./address

// TODO(guggero): Remove this as soon as we have a tagged version of btcec.
replace github.com/btcsuite/btcd/btcec/v2 => ./btcec

// TODO(guggero): Remove this as soon as we have a tagged version of btcutil.
replace github.com/btcsuite/btcd/btcutil/v2 => ./btcutil

// TODO(guggero): Remove this as soon as we have a tagged version of chaincfg.
replace github.com/btcsuite/btcd/chaincfg/v2 => ./chaincfg

// TODO(guggero): Remove this as soon as we have a tagged version of chainhash.
replace github.com/btcsuite/btcd/chainhash/v2 => ./chainhash

// TODO(guggero): Remove this as soon as we have a tagged version of txscript.
replace github.com/btcsuite/btcd/txscript/v2 => ./txscript

// TODO(guggero): Remove this as soon as we have a tagged version of wire.
replace github.com/btcsuite/btcd/wire/v2 => ./wire

// The retract statements below fixes an accidental push of the tags of a btcd
// fork.
retract (
	v0.18.1
	v0.18.0
	v0.17.1
	v0.17.0
	v0.16.5
	v0.16.4
	v0.16.3
	v0.16.2
	v0.16.1
	v0.16.0

	v0.15.2
	v0.15.1
	v0.15.0

	v0.14.7
	v0.14.6
	v0.14.6
	v0.14.5
	v0.14.4
	v0.14.3
	v0.14.2
	v0.14.1

	v0.14.0
	v0.13.0-beta2
	v0.13.0-beta
)
