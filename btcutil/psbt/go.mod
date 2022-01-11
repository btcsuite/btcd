module github.com/btcsuite/btcd/btcutil/psbt

go 1.17

require (
	github.com/btcsuite/btcd v0.22.0-beta.0.20220111032746-97732e52810c
	github.com/btcsuite/btcd/btcutil v1.0.0
	github.com/davecgh/go-spew v1.1.1
)

require (
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f // indirect
	golang.org/x/crypto v0.0.0-20200622213623-75b288015ac9 // indirect
)

replace github.com/btcsuite/btcd/btcutil => ../

replace github.com/btcsuite/btcd => ../..
