module github.com/dashevo/dashd-go

require (
	github.com/btcsuite/btclog v0.0.0-20170628155309-84c8d2346e9f
	github.com/btcsuite/go-socks v0.0.0-20170105172521-4720035b7bfd
	github.com/btcsuite/goleveldb v1.0.0
	github.com/btcsuite/websocket v0.0.0-20150119174127-31079b680792
	github.com/btcsuite/winsvc v1.0.0
	github.com/dashevo/dashd-go/btcec/v2 v2.0.5
	github.com/dashevo/dashd-go/btcutil v1.1.1
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/lru v1.0.0
	github.com/jessevdk/go-flags v1.4.0
	github.com/jrick/logrotate v1.0.0
	golang.org/x/crypto v0.0.0-20220214200702-86341886e292
)

require (
	github.com/aead/siphash v1.0.1 // indirect
	github.com/btcsuite/snappy-go v1.0.0 // indirect
	github.com/decred/dcrd/dcrec/secp256k1/v4 v4.0.1 // indirect
	github.com/kkdai/bstream v1.0.0 // indirect
)

replace (
	github.com/dashevo/dashd-go/btcec/v2 => ./btcec
	github.com/dashevo/dashd-go/btcutil => ./btcutil
)

retract (
	v0.22.0-beta
	v0.2.0
	v0.1.4
	v0.1.3
	v0.1.2
	v0.1.1
	v0.1.0
)

go 1.17
