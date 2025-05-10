package rpcclient

import (
	"errors"
)

var (
	// ErrBackendVersion is returned when running against a bitcoind or
	// btcd that is older than the minimum version supported by the
	// rpcclient.
	ErrBackendVersion = errors.New("backend version too low")

	// ErrInvalidParam is returned when the caller provides an invalid
	// parameter to an RPC method.
	ErrInvalidParam = errors.New("invalid param")

	// ErrUndefined is used when an error returned is not recognized. We
	// should gradually increase our error types to avoid returning this
	// error.
	ErrUndefined = errors.New("undefined")
)
