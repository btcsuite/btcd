module github.com/btcsuite/btcd/wire

go 1.14

require (
	github.com/btcsuite/btcd/chaincfg/chainhash v0.1.0
	github.com/davecgh/go-spew v1.1.1
)

replace github.com/btcsuite/btcd/chaincfg/chainhash => ../chaincfg/chainhash
