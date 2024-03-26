package rpcclient

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestMatchErrStr checks that `matchErrStr` can correctly replace the dashes
// with spaces and turn title cases into lowercases for a given error and match
// it against the specified string pattern.
func TestMatchErrStr(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name        string
		bitcoindErr error
		matchStr    string
		matched     bool
	}{
		{
			name:        "error without dashes",
			bitcoindErr: errors.New("missing input"),
			matchStr:    "missing input",
			matched:     true,
		},
		{
			name:        "match str without dashes",
			bitcoindErr: errors.New("missing-input"),
			matchStr:    "missing input",
			matched:     true,
		},
		{
			name:        "error with dashes",
			bitcoindErr: errors.New("missing-input"),
			matchStr:    "missing input",
			matched:     true,
		},
		{
			name:        "match str with dashes",
			bitcoindErr: errors.New("missing-input"),
			matchStr:    "missing-input",
			matched:     true,
		},
		{
			name:        "error with title case and dash",
			bitcoindErr: errors.New("Missing-Input"),
			matchStr:    "missing input",
			matched:     true,
		},
		{
			name:        "match str with title case and dash",
			bitcoindErr: errors.New("missing-input"),
			matchStr:    "Missing-Input",
			matched:     true,
		},
		{
			name:        "unmatched error",
			bitcoindErr: errors.New("missing input"),
			matchStr:    "missingorspent",
			matched:     false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			matched := matchErrStr(tc.bitcoindErr, tc.matchStr)
			require.Equal(t, tc.matched, matched)
		})
	}
}

// TestMapRPCErr checks that `MapRPCErr` can correctly map a given error to
// the corresponding error in the `BtcdErrMap` or `BitcoindErrors` map.
func TestMapRPCErr(t *testing.T) {
	t.Parallel()

	require := require.New(t)

	// Get all known bitcoind errors.
	bitcoindErrors := make([]error, 0, errSentinel)
	for i := uint32(0); i < uint32(errSentinel); i++ {
		err := BitcoindRPCErr(i)
		bitcoindErrors = append(bitcoindErrors, err)
	}

	// An unknown error should be mapped to ErrUndefined.
	errUnknown := errors.New("unknown error")
	err := MapRPCErr(errUnknown)
	require.ErrorIs(err, ErrUndefined)

	// A known error should be mapped to the corresponding error in the
	// `BtcdErrMap` or `bitcoindErrors` map.
	for btcdErrStr, mappedErr := range BtcdErrMap {
		err := MapRPCErr(errors.New(btcdErrStr))
		require.ErrorIs(err, mappedErr)

		err = MapRPCErr(mappedErr)
		require.ErrorIs(err, mappedErr)
	}

	for _, bitcoindErr := range bitcoindErrors {
		err = MapRPCErr(bitcoindErr)
		require.ErrorIs(err, bitcoindErr)
	}
}

// TestBitcoindErrorSentinel checks that all defined BitcoindRPCErr errors are
// added to the method `Error`.
func TestBitcoindErrorSentinel(t *testing.T) {
	t.Parallel()

	rt := require.New(t)

	for i := uint32(0); i < uint32(errSentinel); i++ {
		err := BitcoindRPCErr(i)
		rt.NotEqualf(err.Error(), "unknown error", "error code %d is "+
			"not defined, make sure to update it inside the Error "+
			"method", i)
	}
}
