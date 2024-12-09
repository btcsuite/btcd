module v2transport

go 1.23.2

require (
	github.com/btcsuite/btcd v0.24.2
	github.com/btcsuite/btcd/btcec/v2 v2.3.4
	golang.org/x/crypto v0.25.0
)

require (
	github.com/btcsuite/btcd/chaincfg/chainhash v1.1.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	golang.org/x/sys v0.22.0 // indirect
)

replace github.com/btcsuite/btcd/btcec/v2 => ./../btcec
