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

// TestHTTPURL pins down the URL strings produced by httpURL for each
// supported host shape. httpURL runs on every RPC and now uses a
// hand-rolled prefix check rather than delegating to ParseAddressString,
// so its output is exercised directly here.
func TestHTTPURL(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		host       string
		disableTLS bool
		expURL     string
	}{
		{
			name:       "unix socket",
			host:       "unix:///var/run/bitcoin/bitcoin.sock",
			disableTLS: true,
			expURL:     "http://unix",
		},
		{
			name:       "unixpacket socket",
			host:       "unixpacket:///var/run/bitcoin/bitcoin.sock",
			disableTLS: true,
			expURL:     "http://unix",
		},
		{
			name:       "ipv4 literal",
			host:       "127.0.0.1:8332",
			disableTLS: true,
			expURL:     "http://127.0.0.1:8332",
		},
		{
			name:       "ipv6 literal",
			host:       "[::1]:8332",
			disableTLS: true,
			expURL:     "http://[::1]:8332",
		},
		{
			name:       "hostname",
			host:       "localhost:8332",
			disableTLS: true,
			expURL:     "http://localhost:8332",
		},
		{
			name:       "empty host",
			host:       "",
			disableTLS: true,
			expURL:     "http://",
		},
		{
			name:       "tls hostname",
			host:       "bitcoind.example.com:8332",
			disableTLS: false,
			expURL:     "https://bitcoind.example.com:8332",
		},
		{
			name:       "tls unix socket",
			host:       "unix:///var/run/bitcoin/bitcoin.sock",
			disableTLS: false,
			expURL:     "https://unix",
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			cfg := &ConnConfig{
				Host:       tc.host,
				DisableTLS: tc.disableTLS,
			}
			require.Equal(t, tc.expURL, cfg.httpURL())
		})
	}
}

// TestHTTPURLWiring guards the construction-time wiring that copies
// (*ConnConfig).httpURL onto Client.httpURL when HTTPPostMode is set.
// TestHTTPURL covers the method itself; this test catches the case
// where a refactor of New silently drops the assignment, leaving
// Client.httpURL as the zero value.
func TestHTTPURLWiring(t *testing.T) {
	t.Parallel()

	cfg := &ConnConfig{
		Host:         "localhost:8332",
		HTTPPostMode: true,
		DisableTLS:   true,
		User:         "user",
		Pass:         "pass",
	}
	c, err := New(cfg, nil)
	require.NoError(t, err)
	defer c.Shutdown()

	require.Equal(t, "http://localhost:8332", c.httpURL)
}
