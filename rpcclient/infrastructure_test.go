package rpcclient

import (
	"strings"
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

// TestStripScheme verifies that scheme prefixes are correctly removed.
func TestStripScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"localhost:8332", "localhost:8332"},
		{"http://localhost:8332", "localhost:8332"},
		{"https://localhost:8332", "localhost:8332"},
		{"https://go.getblock.io/xxxx", "go.getblock.io/xxxx"},
		{"http://go.getblock.io/xxxx", "go.getblock.io/xxxx"},
		{"unix:///var/run/btcd.sock", "unix:///var/run/btcd.sock"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			require.Equal(t, tc.want, stripScheme(tc.input))
		})
	}
}

// TestExtractHostPort verifies that host:port is extracted from strings that
// may contain path components.
func TestExtractHostPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{"localhost:8332", "localhost:8332"},
		{"localhost", "localhost"},
		{"go.getblock.io/xxxx", "go.getblock.io"},
		{"go.getblock.io:443/xxxx", "go.getblock.io:443"},
		{"127.0.0.1:8332", "127.0.0.1:8332"},
		{"[::1]:8332", "[::1]:8332"},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			require.Equal(t, tc.want, extractHostPort(tc.input))
		})
	}
}

// TestHTTPURLWithSchemePrefix verifies that httpURL correctly handles Host
// values that include a scheme prefix (regression test for issue #2464).
func TestHTTPURLWithSchemePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		host   string
		tls    bool
		expect string
	}{
		{
			name:   "plain host",
			host:   "localhost:8332",
			tls:    false,
			expect: "http://localhost:8332",
		},
		{
			name:   "plain host with TLS",
			host:   "localhost:8332",
			tls:    true,
			expect: "https://localhost:8332",
		},
		{
			name:   "host with https prefix and path",
			host:   "https://go.getblock.io/xxxx",
			tls:    true,
			expect: "https://go.getblock.io/xxxx",
		},
		{
			name:   "host with http prefix and path",
			host:   "http://go.getblock.io/xxxx",
			tls:    false,
			expect: "http://go.getblock.io/xxxx",
		},
		{
			name:   "host with path but no scheme",
			host:   "go.getblock.io/xxxx",
			tls:    true,
			expect: "https://go.getblock.io/xxxx",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := &ConnConfig{
				Host:       tc.host,
				DisableTLS: !tc.tls,
			}
			got, err := config.httpURL()
			require.NoError(t, err)
			require.Equal(t, tc.expect, got)
		})
	}
}

// TestNewHTTPClientWithSchemePrefix verifies that newHTTPClient correctly
// handles Host values that include a scheme prefix (issue #2464).
func TestNewHTTPClientWithSchemePrefix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		host string
	}{
		{"plain host", "localhost:8332"},
		{"host with https prefix", "https://localhost:8332"},
		{"host with path", "https://go.getblock.io/xxxx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			config := &ConnConfig{
				Host:       tc.host,
				DisableTLS: strings.HasPrefix(tc.host, "http://"),
			}
			client, err := newHTTPClient(config)
			require.NoError(t, err)
			require.NotNil(t, client)
		})
	}
}
