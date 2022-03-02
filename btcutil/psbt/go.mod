module github.com/dashevo/dashd-go/btcutil/psbt

go 1.17

require (
	github.com/dashevo/dashd-go v0.0.0-00010101000000-000000000000
	github.com/dashevo/dashd-go/btcec/v2 v2.0.0-00010101000000-000000000000
	github.com/dashevo/dashd-go/btcutil v0.0.0-00010101000000-000000000000
	github.com/davecgh/go-spew v1.1.1
)

require (
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9 // indirect
)

replace github.com/dashevo/dashd-go/btcec/v2 => ../../btcec

replace github.com/dashevo/dashd-go/btcutil => ../

replace github.com/dashevo/dashd-go => ../..
