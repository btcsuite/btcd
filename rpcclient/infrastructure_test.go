package rpcclient

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestParseAddressString checks different variation of supported and
// unsupported addresses.
func TestParseAddressString(t *testing.T) {
	t.Parallel()

	// Using localhost only to avoid network calls.
	testCases := []struct {
		name          string
		addressString string
		expNetwork    string
		expAddress    string
		expErrStr     string
	}{
		{
			name:          "localhost",
			addressString: "localhost",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:0",
		},
		{
			name:          "localhost ip",
			addressString: "127.0.0.1",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:0",
		},
		{
			name:          "localhost ipv6",
			addressString: "::1",
			expNetwork:    "tcp",
			expAddress:    "[::1]:0",
		},
		{
			name:          "localhost and port",
			addressString: "localhost:80",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:80",
		},
		{
			name:          "localhost ipv6 and port",
			addressString: "[::1]:80",
			expNetwork:    "tcp",
			expAddress:    "[::1]:80",
		},
		{
			name:          "colon and port",
			addressString: ":80",
			expNetwork:    "tcp",
			expAddress:    ":80",
		},
		{
			name:          "colon only",
			addressString: ":",
			expNetwork:    "tcp",
			expAddress:    ":0",
		},
		{
			name:          "localhost and path",
			addressString: "localhost/path",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:0",
		},
		{
			name:          "localhost port and path",
			addressString: "localhost:80/path",
			expNetwork:    "tcp",
			expAddress:    "127.0.0.1:80",
		},
		{
			name:          "unix prefix",
			addressString: "unix://the/rest/of/the/path",
			expNetwork:    "unix",
			expAddress:    "the/rest/of/the/path",
		},
		{
			name:          "unix prefix",
			addressString: "unixpacket://the/rest/of/the/path",
			expNetwork:    "unixpacket",
			expAddress:    "the/rest/of/the/path",
		},
		{
			name:          "error http prefix",
			addressString: "http://localhost:1010",
			expErrStr:     "unsupported protocol in address",
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			addr, err := ParseAddressString(tc.addressString)
			if tc.expErrStr != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expErrStr)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.expNetwork, addr.Network())
			require.Equal(t, tc.expAddress, addr.String())
		})
	}
}
