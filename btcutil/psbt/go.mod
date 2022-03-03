module github.com/dashevo/dashd-go/btcutil/psbt

go 1.17

require (
	github.com/dashevo/dashd-go v0.23.4
	github.com/dashevo/dashd-go/btcec/v2 v2.0.5
	github.com/dashevo/dashd-go/btcutil v1.1.1
	github.com/davecgh/go-spew v1.1.1
)

require (
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292 // indirect
)

replace github.com/dashevo/dashd-go/btcec/v2 => ../../btcec

replace github.com/dashevo/dashd-go/btcutil => ../

replace github.com/dashevo/dashd-go => ../..
