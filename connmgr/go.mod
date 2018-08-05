module github.com/decred/dcrd/connmgr

require (
	github.com/decred/dcrd/chaincfg v1.0.1
	github.com/decred/dcrd/wire v1.0.1
	github.com/decred/slog v1.0.0
)

replace (
	github.com/decred/dcrd/chaincfg => ../chaincfg
	github.com/decred/dcrd/chaincfg/chainhash => ../chaincfg/chainhash
	github.com/decred/dcrd/wire => ../wire
)
