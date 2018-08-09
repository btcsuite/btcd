module github.com/decred/dcrd/chaincfg

require (
	github.com/davecgh/go-spew v1.1.0
	github.com/decred/dcrd/chaincfg/chainhash v1.0.1
	github.com/decred/dcrd/wire v1.1.0
)

replace (
	github.com/decred/dcrd/chaincfg/chainhash => ./chainhash
	github.com/decred/dcrd/wire => ../wire
)
