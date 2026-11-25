package rpcclient

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// newNetworkInfoServer serves a bitcoind-style backend: getinfo is not found,
// and getnetworkinfo reports the given subversion and numeric version.
func newNetworkInfoServer(t *testing.T, subversion string,
	version int32) string {

	t.Helper()

	handler := http.HandlerFunc(func(w http.ResponseWriter,
		r *http.Request) {

		var req struct {
			Method string          `json:"method"`
			ID     json.RawMessage `json:"id"`
		}
		// The handler runs on the server's goroutine, where
		// t.FailNow (and so require) must not be called.
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "getinfo":
			fmt.Fprintf(w, `{"result":null,"error":{"code":-32601,`+
				`"message":"Method not found"},"id":%s}`,
				req.ID)

		case "getnetworkinfo":
			fmt.Fprintf(w, `{"result":{"version":%d,`+
				`"subversion":%q,"protocolversion":70016},`+
				`"error":null,"id":%s}`, version, subversion,
				req.ID)

		default:
			t.Errorf("unexpected method %q", req.Method)
		}
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return strings.TrimPrefix(server.URL, "http://")
}

// TestBackendVersionDetection checks that BackendVersion reads bitcoind's
// subversion string, and falls back to the numeric version for a backend that
// implements bitcoind's RPC interface under another name.
func TestBackendVersionDetection(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		subversion string
		version    int32
		want       BitcoindVersion
	}{
		{
			name:       "bitcoind",
			subversion: "/Satoshi:28.1.0/",
			version:    280100,
			want:       BitcoindPost25,
		},
		{
			name:       "bitcoind with a uacomment",
			subversion: "/Satoshi:23.0.0(comment)/",
			version:    230000,
			want:       BitcoindPre24,
		},
		{
			name:       "compatible backend at 28.0",
			subversion: "/satd:0.7.0/",
			version:    280000,
			want:       BitcoindPost25,
		},
		{
			name:       "compatible backend at 22.0",
			subversion: "/othernode:1.2.3/",
			version:    220000,
			want:       BitcoindPre24,
		},
	}

	for _, tc := range testCases {
		tc := tc

		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			host := newNetworkInfoServer(
				t, tc.subversion, tc.version,
			)
			client, err := New(&ConnConfig{
				Host:         host,
				User:         "user",
				Pass:         "pass",
				HTTPPostMode: true,
				DisableTLS:   true,
			}, nil)
			require.NoError(t, err)
			t.Cleanup(client.Shutdown)

			got, err := client.BackendVersion()
			require.NoError(t, err)
			require.Equal(t, tc.want, *got.(*BitcoindVersion))
		})
	}
}
