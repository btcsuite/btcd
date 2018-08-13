module github.com/decred/dcrd/gcs

require (
	github.com/dchest/blake256 v1.0.0
	github.com/dchest/siphash v1.2.0
	github.com/decred/dcrd/blockchain/stake v1.0.1
	github.com/decred/dcrd/chaincfg/chainhash v1.0.1
	github.com/decred/dcrd/txscript v1.0.1
	github.com/decred/dcrd/wire v1.1.0
)

replace (
	github.com/decred/dcrd/blockchain/stake => ../blockchain/stake
	github.com/decred/dcrd/chaincfg => ../chaincfg
	github.com/decred/dcrd/chaincfg/chainhash => ../chaincfg/chainhash
	github.com/decred/dcrd/database => ../database
	github.com/decred/dcrd/dcrec => ../dcrec
	github.com/decred/dcrd/dcrec/edwards => ../dcrec/edwards
	github.com/decred/dcrd/dcrec/secp256k1 => ../dcrec/secp256k1
	github.com/decred/dcrd/dcrutil => ../dcrutil
	github.com/decred/dcrd/txscript => ../txscript
	github.com/decred/dcrd/wire => ../wire
)
