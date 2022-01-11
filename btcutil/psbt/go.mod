module github.com/btcsuite/btcd/btcutil/psbt

go 1.17

require (
	github.com/btcsuite/btcd v0.20.1-beta
	github.com/btcsuite/btcd/btcutil v0.0.0-20190425235716-9e5f4b9a998d
	github.com/davecgh/go-spew v1.1.1
)

require (
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f // indirect
	github.com/btcsuite/btcutil v0.0.0-20190425235716-9e5f4b9a998d // indirect
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9 // indirect
)

replace github.com/btcsuite/btcd/btcutil => ../
