package rpcclient

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDisableAuth verifies that the DisableAuth field correctly controls
// whether the Authorization header is sent on RPC requests.
func TestDisableAuth(t *testing.T) {
	t.Parallel()

	t.Run("DisableAuth true omits Authorization header", func(t *testing.T) {
		t.Parallel()

		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			// Return a valid JSON-RPC response so the client doesn't retry.
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"result":null,"error":null,"id":1}`))
		}))
		defer srv.Close()

		addr := strings.TrimPrefix(srv.URL, "http://")
		client, err := New(&ConnConfig{
			Host:        addr,
			HTTPPostMode: true,
			DisableAuth: true,
			DisableTLS:  true,
		}, nil)
		require.NoError(t, err)
		defer client.Shutdown()

		// The client is now connected; issue a simple request to trigger
		// handleSendPostMessage.
		_, err = client.RawRequest("getblockchaininfo", nil)
		// We don't care if the RPC itself errors — we only care about
		// the Authorization header.
		_ = err

		require.Empty(t, gotAuth, "Authorization header should be empty when DisableAuth is true")
	})

	t.Run("DisableAuth false includes Authorization header", func(t *testing.T) {
		t.Parallel()

		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"result":null,"error":null,"id":1}`))
		}))
		defer srv.Close()

		addr := strings.TrimPrefix(srv.URL, "http://")
		client, err := New(&ConnConfig{
			Host:        addr,
			HTTPPostMode: true,
			DisableAuth: false,
			DisableTLS:  true,
			User:        "testuser",
			Pass:        "testpass",
		}, nil)
		require.NoError(t, err)
		defer client.Shutdown()

		_, err = client.RawRequest("getblockchaininfo", nil)
		_ = err

		expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("testuser:testpass"))
		require.Equal(t, expected, gotAuth, "Authorization header should be set when DisableAuth is false")
	})

	t.Run("DisableAuth default (zero value) includes Authorization header", func(t *testing.T) {
		t.Parallel()

		var gotAuth string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"result":null,"error":null,"id":1}`))
		}))
		defer srv.Close()

		addr := strings.TrimPrefix(srv.URL, "http://")
		client, err := New(&ConnConfig{
			Host:        addr,
			HTTPPostMode: true,
			// DisableAuth left as default (false)
			DisableTLS:  true,
			User:        "myuser",
			Pass:        "mypass",
		}, nil)
		require.NoError(t, err)
		defer client.Shutdown()

		_, err = client.RawRequest("getblockchaininfo", nil)
		_ = err

		expected := "Basic " + base64.StdEncoding.EncodeToString([]byte("myuser:mypass"))
		require.Equal(t, expected, gotAuth, "Authorization header should be set by default (DisableAuth is false)")
	})
}
