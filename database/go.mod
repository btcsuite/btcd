module github.com/decred/dcrd/database

require (
	github.com/btcsuite/goleveldb v1.0.0
	github.com/decred/dcrd/chaincfg v1.0.1
	github.com/decred/dcrd/chaincfg/chainhash v1.0.1
	github.com/decred/dcrd/dcrutil v1.0.0
	github.com/decred/dcrd/wire v1.0.1
	github.com/decred/slog v1.0.0
	github.com/jessevdk/go-flags v1.4.0
)

replace (
	github.com/decred/dcrd/chaincfg => ../chaincfg
	github.com/decred/dcrd/chaincfg/chainhash => ../chaincfg/chainhash
	github.com/decred/dcrd/dcrec => ../dcrec
	github.com/decred/dcrd/dcrec/edwards => ../dcrec/edwards
	github.com/decred/dcrd/dcrec/secp256k1 => ../dcrec/secp256k1
	github.com/decred/dcrd/dcrutil => ../dcrutil
	github.com/decred/dcrd/wire => ../wire
)
