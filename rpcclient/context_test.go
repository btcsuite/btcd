package rpcclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestGetBlockCountWithContext_Success verifies that the context-
// aware GetBlockCountWithContext returns the correct block height.
func TestGetBlockCountWithContext_Success(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := `{"result":941874,"error":null,"id":1}`
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, resp)
		}),
	)
	defer server.Close()

	client := newTestHTTPClient(t, server)
	defer client.Shutdown()

	count, err := client.GetBlockCountWithContext(
		context.Background(),
	)
	require.NoError(t, err)
	require.Equal(t, int64(941874), count)
}

// TestGetBlockCountWithContext_CancelledContext verifies that a
// cancelled context stops the HTTP request and returns an error.
func TestGetBlockCountWithContext_CancelledContext(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Simulate slow RPC — delay longer than the
			// context timeout so cancellation is tested.
			select {
			case <-time.After(2 * time.Second):
			case <-r.Context().Done():
			}
		}),
	)
	defer server.Close()

	client := newTestHTTPClient(t, server)
	defer client.Shutdown()

	ctx, cancel := context.WithTimeout(
		context.Background(), 200*time.Millisecond,
	)
	defer cancel()

	start := time.Now()
	_, err := client.GetBlockCountWithContext(ctx)
	elapsed := time.Since(start)

	require.Error(t, err)
	// Should return within ~200ms, not 10s.
	require.Less(
		t, elapsed, 2*time.Second,
		"context cancellation should abort the request",
	)
}

// TestSendCmdWithContext_RetryRespectsCancel verifies that context
// cancellation during the retry backoff terminates immediately
// rather than waiting for all retry attempts.
func TestSendCmdWithContext_RetryRespectsCancel(t *testing.T) {
	t.Parallel()

	var attempts int
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			attempts++
			// Always return server error to trigger retry.
			w.WriteHeader(http.StatusInternalServerError)
		}),
	)
	defer server.Close()

	client := newTestHTTPClient(t, server)
	defer client.Shutdown()

	ctx, cancel := context.WithTimeout(
		context.Background(), 200*time.Millisecond,
	)
	defer cancel()

	start := time.Now()
	_, err := client.GetBlockCountWithContext(ctx)
	elapsed := time.Since(start)

	require.Error(t, err)
	// Without context, 10 retries with backoff would take
	// much longer. Context should cut it short.
	require.Less(
		t, elapsed, 2*time.Second,
		"context cancellation should stop retries promptly",
	)
}

// TestGetBlockCount_BackwardsCompatible verifies that the original
// GetBlockCount (without context) still works unchanged.
func TestGetBlockCount_BackwardsCompatible(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := `{"result":100,"error":null,"id":1}`
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, resp)
		}),
	)
	defer server.Close()

	client := newTestHTTPClient(t, server)
	defer client.Shutdown()

	count, err := client.GetBlockCount()
	require.NoError(t, err)
	require.Equal(t, int64(100), count)
}

// TestSendCmdWithContext_NilContext verifies that passing a nil
// request context field falls back to context.Background.
func TestSendCmdWithContext_NilCtxField(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := `{"result":500,"error":null,"id":1}`
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, resp)
		}),
	)
	defer server.Close()

	client := newTestHTTPClient(t, server)
	defer client.Shutdown()

	// SendCmd (no context) should still work — ctx field is nil,
	// handleSendPostMessage falls back to context.Background.
	count, err := client.GetBlockCount()
	require.NoError(t, err)
	require.Equal(t, int64(500), count)
}

// TestGetBlockCountWithContext_RPCError verifies that JSON-RPC
// errors are properly propagated through the context path.
func TestGetBlockCountWithContext_RPCError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			resp := map[string]interface{}{
				"result": nil,
				"error": map[string]interface{}{
					"code":    -1,
					"message": "method not found",
				},
				"id": 1,
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(resp)
		}),
	)
	defer server.Close()

	client := newTestHTTPClient(t, server)
	defer client.Shutdown()

	_, err := client.GetBlockCountWithContext(
		context.Background(),
	)
	require.Error(t, err)
	require.Contains(t, err.Error(), "method not found")
}

// newTestHTTPClient creates an rpcclient connected to the given
// httptest.Server in HTTP POST mode.
func newTestHTTPClient(
	t *testing.T, server *httptest.Server,
) *Client {

	t.Helper()

	// Strip "http://" prefix for ConnConfig.Host.
	host := strings.TrimPrefix(server.URL, "http://")

	client, err := New(&ConnConfig{
		Host:         host,
		User:         "user",
		Pass:         "pass",
		HTTPPostMode: true,
		DisableTLS:   true,
	}, nil)
	require.NoError(t, err)

	return client
}
