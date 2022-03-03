module github.com/dashevo/dashd-go/btcutil

go 1.16

require (
	github.com/aead/siphash v1.0.1
	github.com/dashevo/dashd-go v0.23.3
	github.com/dashevo/dashd-go/btcec/v2 v2.0.5
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1
	github.com/kkdai/bstream v1.0.0
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292
)

replace (
	github.com/dashevo/dashd-go => ../
	github.com/dashevo/dashd-go/btcec/v2 => ../btcec
)
