package rpcclient

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseBitcoindVersion checks that the correct version from bitcoind's
// `getnetworkinfo` RPC call is parsed.
func TestParseBitcoindVersion(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name          string
		rpcVersion    string
		parsedVersion BackendVersion
	}{
		{
			name:          "parse version 0.19 and below",
			rpcVersion:    "/Satoshi:0.18.0/",
			parsedVersion: BitcoindPre19,
		},
		{
			name:          "parse version 0.19",
			rpcVersion:    "/Satoshi:0.19.0/",
			parsedVersion: BitcoindPre22,
		},
		{
			name:          "parse version 0.19 - 22.0",
			rpcVersion:    "/Satoshi:0.20.1/",
			parsedVersion: BitcoindPre22,
		},
		{
			name:          "parse version 22.0",
			rpcVersion:    "/Satoshi:22.0.0/",
			parsedVersion: BitcoindPre25,
		},
		{
			name:          "parse version 22.0 - 25.0",
			rpcVersion:    "/Satoshi:23.0.0/",
			parsedVersion: BitcoindPre25,
		},
		{
			name:          "parse version 25.0",
			rpcVersion:    "/Satoshi:25.0.0/",
			parsedVersion: BitcoindPost25,
		},
		{
			name:          "parse version 25.0 and above",
			rpcVersion:    "/Satoshi:26.0.0/",
			parsedVersion: BitcoindPost25,
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			version := parseBitcoindVersion(tc.rpcVersion)
			require.Equal(t, tc.parsedVersion, version)
		})
	}
}
