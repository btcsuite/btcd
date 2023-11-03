package rpcclient

import "errors"

var (
	// ErrBitcoindVersion is returned when running against a bitcoind that
	// is older than the minimum version supported by the rpcclient.
	ErrBitcoindVersion = errors.New("bitcoind version too low")

	// ErrInvalidParam is returned when the caller provides an invalid
	// parameter to an RPC method.
	ErrInvalidParam = errors.New("invalid param")
)
