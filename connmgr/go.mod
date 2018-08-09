module github.com/decred/dcrd/connmgr

require (
	github.com/decred/dcrd/chaincfg v1.1.1
	github.com/decred/dcrd/wire v1.1.0
	github.com/decred/slog v1.0.0
)

replace (
	github.com/decred/dcrd/chaincfg => ../chaincfg
	github.com/decred/dcrd/chaincfg/chainhash => ../chaincfg/chainhash
	github.com/decred/dcrd/wire => ../wire
)
